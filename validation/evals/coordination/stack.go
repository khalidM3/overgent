package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stickguy/stickguy/internal/app"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/stickguy/stickguy/internal/onboarding"
)

type backendProcess struct {
	command *exec.Cmd
	done    chan error
	logPath string

	environmentPath     string
	environmentSnapshot []byte
	environmentExisted  bool
}

// setAsideDeveloperEnvironment removes a pre-existing convex/.env.local for
// the duration of the run. A configured deployment in that file overrides
// CONVEX_AGENT_MODE=anonymous, silently attaching the suite to a real cloud
// deployment, which the loopback-only gate then refuses. The original file
// contents are restored in stop.
func (process *backendProcess) setAsideDeveloperEnvironment(path string) error {
	process.environmentPath = path
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("snapshot developer convex environment: %w", err)
	}
	process.environmentSnapshot = contents
	process.environmentExisted = true
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("set aside developer convex environment: %w", err)
	}
	return nil
}

func (process *backendProcess) restoreDeveloperEnvironment() {
	if !process.environmentExisted {
		return
	}
	_ = os.WriteFile(process.environmentPath, process.environmentSnapshot, 0o600)
	process.environmentExisted = false
}

func startBackend(ctx context.Context, repositoryRoot, temporaryRoot string) (*backendProcess, string, error) {
	logPath := filepath.Join(temporaryRoot, "backend.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("create backend log: %w", err)
	}
	command := exec.CommandContext(ctx, "pnpm", "dev:backend")
	command.Dir = repositoryRoot
	command.Env = append(os.Environ(), "CI=true", "CONVEX_AGENT_MODE=anonymous")
	command.Stdout, command.Stderr = logFile, logFile
	// The pnpm wrapper spawns the actual convex-local backend. Interrupting
	// only the wrapper leaks the backend and its port; signal the whole
	// process group on stop instead.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	environmentPath := filepath.Join(repositoryRoot, "convex", ".env.local")
	process := &backendProcess{command: command, done: make(chan error, 1), logPath: logPath}
	if err := process.setAsideDeveloperEnvironment(environmentPath); err != nil {
		_ = logFile.Close()
		return nil, "", err
	}
	if err := command.Start(); err != nil {
		process.restoreDeveloperEnvironment()
		_ = logFile.Close()
		return nil, "", fmt.Errorf("start loopback backend: %w", err)
	}
	go func() {
		process.done <- command.Wait()
		_ = logFile.Close()
	}()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case waitErr := <-process.done:
			return nil, "", fmt.Errorf("loopback backend exited before readiness: %v: %s", waitErr, tailFile(logPath, 4000))
		default:
		}
		siteURL, readErr := readLoopbackSiteURL(environmentPath)
		if readErr == nil && backendReady(ctx, siteURL) {
			return process, siteURL, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	process.stop()
	return nil, "", fmt.Errorf("loopback backend did not become ready: %s", tailFile(logPath, 4000))
}

func (process *backendProcess) stop() {
	if process == nil || process.command == nil || process.command.Process == nil {
		return
	}
	_ = syscall.Kill(-process.command.Process.Pid, syscall.SIGINT)
	select {
	case <-process.done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-process.command.Process.Pid, syscall.SIGKILL)
		select {
		case <-process.done:
		case <-time.After(2 * time.Second):
		}
	}
	process.restoreDeveloperEnvironment()
}

func readLoopbackSiteURL(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || key != "CONVEX_SITE_URL" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "'\"")
		parsed, parseErr := url.Parse(value)
		if parseErr != nil || parsed.Scheme != "http" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", errors.New("anonymous backend produced an invalid HTTP actions origin")
		}
		if parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" {
			return "", errors.New("coordination eval refuses a non-loopback backend")
		}
		return strings.TrimRight(value, "/"), nil
	}
	return "", errors.New("CONVEX_SITE_URL is missing")
}

func backendReady(ctx context.Context, siteURL string) bool {
	requestContext, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, siteURL+"/v1/device/bootstrap", nil)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false
	}
	response.Body.Close()
	// The listener can answer 404 before Convex has loaded the Stickguy
	// functions. The real bootstrap route deterministically answers 401 without
	// a device credential, which proves the HTTP actions are ready.
	return response.StatusCode == http.StatusUnauthorized
}

func tailFile(path string, limit int) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "backend log unavailable"
	}
	if len(contents) > limit {
		contents = contents[len(contents)-limit:]
	}
	return strings.TrimSpace(string(contents))
}

type fixtureRepository struct {
	worktreeA string
	worktreeB string
}

func materializeFixture(ctx context.Context, fixtureRoot, destination, repositoryKey string) (fixtureRepository, error) {
	repository := filepath.Join(destination, "repository")
	if err := copyTree(fixtureRoot, repository); err != nil {
		return fixtureRepository{}, err
	}
	commands := [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "coordination-eval@example.invalid"},
		{"config", "user.name", "Stickguy Coordination Eval"},
		{"remote", "add", "origin", "https://example.invalid/stickguy/coordination-eval-" + strings.ToLower(repositoryKey) + ".git"},
		{"add", "."},
		{"commit", "-q", "-m", "coordination fixture baseline"},
	}
	for _, arguments := range commands {
		if _, err := runGit(ctx, repository, arguments...); err != nil {
			return fixtureRepository{}, err
		}
	}
	worktreeA := filepath.Join(destination, "agent-a")
	worktreeB := filepath.Join(destination, "agent-b")
	if _, err := runGit(ctx, repository, "worktree", "add", "-q", "-b", "eval-agent-a", worktreeA, "main"); err != nil {
		return fixtureRepository{}, err
	}
	if _, err := runGit(ctx, repository, "worktree", "add", "-q", "-b", "eval-agent-b", worktreeB, "main"); err != nil {
		return fixtureRepository{}, err
	}
	return fixtureRepository{worktreeA: worktreeA, worktreeB: worktreeB}, nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func runGit(ctx context.Context, repository string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repository}, arguments...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

type memoryCredentials struct {
	mu     sync.Mutex
	values map[string]string
}

func (credentials *memoryCredentials) Put(_ context.Context, account, secret string) error {
	credentials.mu.Lock()
	defer credentials.mu.Unlock()
	credentials.values[account] = secret
	return nil
}

func (credentials *memoryCredentials) Delete(_ context.Context, account string) error {
	credentials.mu.Lock()
	defer credentials.mu.Unlock()
	delete(credentials.values, account)
	return nil
}

func (credentials *memoryCredentials) get(account string) string {
	credentials.mu.Lock()
	defer credentials.mu.Unlock()
	return credentials.values[account]
}

type liveSender struct {
	client *hosted.Client
	mu     sync.Mutex
	last   error
}

func (sender *liveSender) record(err error) error {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	sender.last = err
	return err
}

func (sender *liveSender) lastError() error {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.last
}

func (sender *liveSender) Send(ctx context.Context, _ string, batch []byte) error {
	ack, err := sender.client.PublishBatch(ctx, batch)
	if err != nil {
		return sender.record(fmt.Errorf("publish %s: %w", batchKinds(batch), err))
	}
	if len(ack.AcceptedEventIDs) == 0 || ack.Cursor == "" {
		return sender.record(errors.New("hosted acknowledgement was incomplete"))
	}
	return sender.record(nil)
}

func batchKinds(batch []byte) string {
	var value struct {
		Events []struct {
			Type string `json:"type"`
		} `json:"events"`
	}
	if json.Unmarshal(batch, &value) != nil {
		return "undecodable event batch"
	}
	kinds := make([]string, 0, len(value.Events))
	for _, event := range value.Events {
		kinds = append(kinds, event.Type)
	}
	return strings.Join(kinds, ",")
}

func (sender *liveSender) Heartbeat(ctx context.Context, workspaceID, state string) error {
	return sender.client.Heartbeat(ctx, workspaceID, state)
}

func (sender *liveSender) CreateBrief(ctx context.Context, workstreamID, trigger, cursor string, budget int) (hosted.CoordinationBrief, error) {
	return sender.client.CreateBrief(ctx, workstreamID, trigger, cursor, budget)
}

type mcpOutput struct {
	ProjectID      string                    `json:"projectId"`
	WorkspaceID    string                    `json:"workspaceId"`
	WorkstreamID   string                    `json:"workstreamId"`
	Duplicate      bool                      `json:"duplicate"`
	IntentRevision int64                     `json:"intentRevision"`
	Brief          *hosted.CoordinationBrief `json:"brief"`
	Degraded       bool                      `json:"degraded"`
	Degradation    string                    `json:"degradation"`
}

type mcpAgent struct {
	session *sdkmcp.ClientSession
}

func connectMCP(ctx context.Context, binary, stateRoot, workingDirectory, name string) (*mcpAgent, error) {
	command := exec.Command(binary, "--config-root", stateRoot, "mcp")
	command.Dir = workingDirectory
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: name, Version: "coordination-eval/v1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect %s MCP session: %w", name, err)
	}
	return &mcpAgent{session: session}, nil
}

func (agent *mcpAgent) call(ctx context.Context, name string, arguments map[string]any) (mcpOutput, error) {
	result, err := agent.session.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return mcpOutput{}, fmt.Errorf("MCP %s: %w", name, err)
	}
	if result == nil || result.IsError {
		return mcpOutput{}, fmt.Errorf("MCP %s returned a tool error", name)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return mcpOutput{}, fmt.Errorf("encode MCP %s output: %w", name, err)
	}
	var output mcpOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		return mcpOutput{}, fmt.Errorf("decode MCP %s output: %w", name, err)
	}
	return output, nil
}

func (agent *mcpAgent) close() {
	if agent != nil && agent.session != nil {
		_ = agent.session.Close()
	}
}

type evaluationEnvironment struct {
	ctx         context.Context
	cancel      context.CancelFunc
	serviceDone chan error
	stateRoot   string
	client      *hosted.Client
	sender      *liveSender
	projectID   string
	scenarios   map[string]scenarioPair
}

type scenarioPair struct {
	repository fixtureRepository
	workspaceA config.Workspace
	workspaceB config.Workspace
}

type scenarioEnvironment struct {
	ctx        context.Context
	stateRoot  string
	repository fixtureRepository
	workspaceA config.Workspace
	workspaceB config.Workspace
	client     *hosted.Client
	sender     *liveSender
	agentA     *mcpAgent
	agentB     *mcpAgent
	projectID  string

	// readerWorkstreamA is the stable hook-session workstream identity for
	// workspace A's scripted agent. A real vendor session keeps one hashed
	// workstream for its whole life (ADR-033), so read-set evidence and the
	// findings it produces target this identity, not a fresh id per event.
	readerWorkstreamA string
}

func (environment *scenarioEnvironment) readerWorkstream() string {
	if environment.readerWorkstreamA == "" {
		environment.readerWorkstreamA = newPublicID("wrk_agent_")
	}
	return environment.readerWorkstreamA
}

func newEvaluationEnvironment(ctx context.Context, fixtureRoot, siteURL, temporaryRoot string) (*evaluationEnvironment, error) {
	stateRoot := filepath.Join(temporaryRoot, "state")
	credentials := &memoryCredentials{values: map[string]string{}}
	onboardingService := onboarding.Service{
		Client:   func(token string) (onboarding.API, error) { return hosted.New(siteURL, token) },
		Creds:    credentials,
		Register: app.Register,
	}
	pairs := map[string]scenarioPair{}
	for _, definition := range scenarioDefinitions {
		scenarioRoot := filepath.Join(temporaryRoot, "scenario-"+strings.ToLower(definition.ID))
		if err := os.MkdirAll(scenarioRoot, 0o700); err != nil {
			return nil, err
		}
		repository, err := materializeFixture(ctx, fixtureRoot, filepath.Join(scenarioRoot, "fixture"), definition.ID)
		if err != nil {
			return nil, fmt.Errorf("materialize scenario %s fixture: %w", definition.ID, err)
		}
		pairs[definition.ID] = scenarioPair{repository: repository}
	}
	first := pairs[scenarioDefinitions[0].ID]
	created, err := onboardingService.Create(ctx, onboarding.Options{
		ConfigRoot: stateRoot, RepositoryRoot: first.repository.worktreeA, APIBaseURL: siteURL,
		ProjectLabel: "Coordination eval", DeviceLabel: "Scripted agents",
	})
	if err != nil {
		return nil, fmt.Errorf("create coordination evaluation Project: %w", err)
	}
	paths, err := config.Resolve(stateRoot)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return nil, fmt.Errorf("load coordination evaluation profile: %w", err)
	}
	if len(cfg.Workspaces) != 1 {
		return nil, fmt.Errorf("load coordination evaluation profile: got %d workspaces, want 1", len(cfg.Workspaces))
	}
	memberID := cfg.Workspaces[0].MemberID
	first.workspaceA = cfg.Workspaces[0]
	pairs[scenarioDefinitions[0].ID] = first
	for _, definition := range scenarioDefinitions {
		pair := pairs[definition.ID]
		roots := []string{pair.repository.worktreeA, pair.repository.worktreeB}
		start := 0
		if definition.ID == scenarioDefinitions[0].ID {
			start = 1
		}
		for index := start; index < len(roots); index++ {
			workspace := config.Workspace{
				ID: newPublicID("wsp_eval_"), ProjectID: created.ProjectID,
				WorkstreamID: newPublicID("wrk_eval_"), MemberID: memberID,
				SessionID: newPublicID("ses_eval_"), Root: roots[index],
			}
			if err := app.Register(ctx, stateRoot, siteURL, created.DeviceID, workspace); err != nil {
				return nil, fmt.Errorf("register scenario %s workspace %d: %w", definition.ID, index, err)
			}
			if index == 0 {
				pair.workspaceA = workspace
			} else {
				pair.workspaceB = workspace
			}
		}
		pairs[definition.ID] = pair
	}
	client, err := hosted.New(siteURL, credentials.get(created.DeviceID))
	if err != nil {
		return nil, err
	}
	serviceContext, cancel := context.WithCancel(ctx)
	sender := &liveSender{client: client}
	environment := &evaluationEnvironment{
		ctx: serviceContext, cancel: cancel, serviceDone: make(chan error, 1), stateRoot: stateRoot,
		client: client, sender: sender, projectID: created.ProjectID, scenarios: pairs,
	}
	go func() { environment.serviceDone <- app.Run(serviceContext, stateRoot, sender) }()
	if err := waitService(serviceContext, paths.Socket, environment.serviceDone); err != nil {
		environment.stop()
		return nil, fmt.Errorf("start coordination evaluation service: %w", err)
	}
	if err := environment.waitForQueue(20 * time.Second); err != nil {
		environment.stop()
		return nil, err
	}
	return environment, nil
}

func (environment *evaluationEnvironment) openScenario(binary, scenarioID string) (*scenarioEnvironment, error) {
	pair, ok := environment.scenarios[scenarioID]
	if !ok {
		return nil, fmt.Errorf("unknown scenario %s", scenarioID)
	}
	scenario := &scenarioEnvironment{
		ctx: environment.ctx, stateRoot: environment.stateRoot, repository: pair.repository,
		workspaceA: pair.workspaceA, workspaceB: pair.workspaceB, client: environment.client, sender: environment.sender,
		projectID: environment.projectID,
	}
	var err error
	scenario.agentA, err = connectMCP(environment.ctx, binary, environment.stateRoot, pair.repository.worktreeA, "scripted-agent-a-"+strings.ToLower(scenarioID))
	if err == nil {
		scenario.agentB, err = connectMCP(environment.ctx, binary, environment.stateRoot, pair.repository.worktreeB, "scripted-agent-b-"+strings.ToLower(scenarioID))
	}
	if err != nil {
		scenario.stop()
		return nil, err
	}
	return scenario, nil
}

func (environment *evaluationEnvironment) stop() {
	if environment == nil {
		return
	}
	if environment.cancel != nil {
		environment.cancel()
		select {
		case <-environment.serviceDone:
		case <-time.After(5 * time.Second):
		}
	}
}

func (environment *evaluationEnvironment) waitForQueue(timeout time.Duration) error {
	return waitForQueue(environment.ctx, environment.stateRoot, timeout, environment.sender)
}

func (environment *scenarioEnvironment) stop() {
	if environment == nil {
		return
	}
	environment.agentA.close()
	environment.agentB.close()
}

func (environment *scenarioEnvironment) forceScan() error {
	paths, err := config.Resolve(environment.stateRoot)
	if err != nil {
		return err
	}
	response, err := daemon.Call(environment.ctx, paths.Socket, daemon.Request{Method: "scan"})
	if err == nil {
		if !response.OK {
			return fmt.Errorf("force service scan: response=%s", response.Error)
		}
		return nil
	}
	// The daemon keeps handling the scan after its fixed RPC read deadline.
	// Waiting for the queue avoids stacking a second full scan behind it and
	// proves the service became responsive after publishing pending evidence.
	if drainErr := environment.waitForQueue(8 * time.Second); drainErr != nil {
		return fmt.Errorf("force service scan timed out (%v), then queue did not drain: %w", err, drainErr)
	}
	return nil
}

func (environment *scenarioEnvironment) hookRead(workspace config.Workspace, vendor, relativePath string) error {
	paths, err := config.Resolve(environment.stateRoot)
	if err != nil {
		return err
	}
	response, err := daemon.Call(environment.ctx, paths.Socket, daemon.Request{
		Method: "agent_event", AgentVendor: vendor, AgentCWD: workspace.Root,
		// Hook sessions are independent first-class workstreams (ADR-033) with
		// one stable identity per session; stale-assumption findings target it.
		AgentWorkstreamID: environment.readerWorkstream(), AgentSessionAlias: vendor + "-a1b2c3",
		AgentEvent: "PostToolUse", AgentStatus: "active", AgentAction: "read " + relativePath,
		AgentTool: "Read", AgentPaths: []string{filepath.Join(workspace.Root, relativePath)},
	})
	if err != nil {
		return fmt.Errorf("publish hook-shaped read event: %w", err)
	}
	if !response.OK {
		return fmt.Errorf("publish hook-shaped read event: %s", response.Error)
	}
	return nil
}

func (environment *scenarioEnvironment) waitForQueue(timeout time.Duration) error {
	return waitForQueue(environment.ctx, environment.stateRoot, timeout, environment.sender)
}

func waitForQueue(ctx context.Context, stateRoot string, timeout time.Duration, sender *liveSender) error {
	paths, err := config.Resolve(stateRoot)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		response, callErr := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "doctor"})
		if callErr == nil && response.OK {
			if data, ok := response.Data.(map[string]any); ok {
				if pending, ok := data["pending"].(float64); ok && pending == 0 {
					return nil
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if sender != nil && sender.lastError() != nil {
		return fmt.Errorf("local event queue did not drain: %w", sender.lastError())
	}
	return errors.New("local event queue did not drain")
}

func (environment *scenarioEnvironment) findings() ([]actualFinding, error) {
	page, err := environment.client.ProjectChanges(environment.ctx, environment.projectID)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(page.Items)
	if err != nil {
		return nil, err
	}
	var findings []actualFinding
	if err := json.Unmarshal(encoded, &findings); err != nil {
		return nil, err
	}
	// A scenario's own workstreams are its two MCP workstreams plus the hook
	// session workstream that carries its read set. Contract findings target
	// the reading session (ADR-033/048), so excluding it would hide exactly
	// the evidence scenarios A and C assert on.
	own := []string{environment.workspaceA.WorkstreamID, environment.workspaceB.WorkstreamID}
	if environment.readerWorkstreamA != "" {
		own = append(own, environment.readerWorkstreamA)
	}
	relevant := make([]actualFinding, 0, len(findings))
	for _, finding := range findings {
		for _, workstream := range own {
			if includesAll(finding.WorkstreamIDs, []string{workstream}) {
				relevant = append(relevant, finding)
				break
			}
		}
	}
	return relevant, nil
}

func waitService(ctx context.Context, socket string, serviceDone <-chan error) error {
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-serviceDone:
			if err == nil {
				return errors.New("service exited before its socket became healthy")
			}
			return fmt.Errorf("service exited before readiness: %w", err)
		default:
		}
		response, err := daemon.Call(ctx, socket, daemon.Request{Method: "health"})
		if err == nil && response.OK {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("service socket did not become healthy")
}

func newPublicID(prefix string) string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(value)
}

func writeFixtureFile(root, relativePath, contents string) error {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(contents), 0o600)
}
