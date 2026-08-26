//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/stickguy/stickguy/internal/activation"
	"github.com/stickguy/stickguy/internal/claudesetup"
	"github.com/stickguy/stickguy/internal/codexsetup"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/credential"
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/stickguy/stickguy/internal/onboarding"
	"github.com/stickguy/stickguy/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
	"unicode/utf8"
)

type AdapterState struct {
	Name             string `json:"name"`
	Installed        bool   `json:"installed"`
	Configured       bool   `json:"configured"`
	Fidelity         string `json:"fidelity"`
	Detail           string `json:"detail"`
	Binding          string `json:"binding"`
	PreviousProfile  string `json:"previousProfile,omitempty"`
	CurrentProfile   string `json:"currentProfile"`
	RuntimeVerified  bool   `json:"runtimeVerified"`
	RestartRequired  bool   `json:"restartRequired"`
	ReconnectAllowed bool   `json:"reconnectAllowed"`
}

type OnboardingState struct {
	Available       bool           `json:"available"`
	Enrolled        bool           `json:"enrolled"`
	ProjectID       string         `json:"projectId"`
	RepositoryRoot  string         `json:"repositoryRoot"`
	RepositoryLabel string         `json:"repositoryLabel"`
	DeviceLabel     string         `json:"deviceLabel"`
	APIBaseURL      string         `json:"apiBaseUrl"`
	Adapters        []AdapterState `json:"adapters"`
	Limitation      string         `json:"limitation"`
}

type EnrollmentRequest struct {
	RepositoryRoot string `json:"repositoryRoot"`
	ProjectLabel   string `json:"projectLabel"`
	DeviceLabel    string `json:"deviceLabel"`
	DisplayName    string `json:"displayName"`
	JoinCode       string `json:"joinCode"`
	EnableCodex    bool   `json:"enableCodex"`
	EnableClaude   bool   `json:"enableClaude"`
}

type EnrollmentResult struct {
	ProjectID string   `json:"projectId"`
	JoinCode  string   `json:"joinCode"`
	Warnings  []string `json:"warnings"`
}

type OnboardingService struct {
	configRoot, apiBaseURL, activationBaseURL, cliBinary string
}

func newOnboardingService() *OnboardingService {
	root := desktopConfigRoot()
	return &OnboardingService{configRoot: root, apiBaseURL: desktopAPIBaseURL(), activationBaseURL: desktopActivationBaseURL(), cliBinary: desktopCLIBinary()}
}

func (service *OnboardingService) ChooseRepository() (string, error) {
	selected, err := application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Choose the Git repository Stickguy should coordinate").
		PromptForSingleSelection()
	if err != nil || selected == "" {
		return selected, err
	}
	return canonicalRepository(selected)
}

func (service *OnboardingService) State() (OnboardingState, error) {
	state := OnboardingState{Available: desktopDevelopment, APIBaseURL: service.apiBaseURL, DeviceLabel: defaultDeviceLabel(), Limitation: "Start new Codex or Claude Code sessions in this repository after connecting an adapter. Existing sessions must restart once so the agent can load the Project hooks."}
	if service.configRoot == "" {
		return state, errors.New("local Stickguy configuration is unavailable")
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return state, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return state, err
	}
	if len(cfg.Workspaces) == 0 {
		state.Adapters = service.adapterStates(nil)
		return state, nil
	}
	projects := map[string]struct{}{}
	for _, workspace := range cfg.Workspaces {
		projects[workspace.ProjectID] = struct{}{}
	}
	if len(projects) != 1 {
		return state, errors.New("desktop onboarding cannot safely select among multiple enrolled Projects yet")
	}
	state.Enrolled = true
	state.ProjectID = cfg.Workspaces[0].ProjectID
	state.RepositoryRoot = cfg.Workspaces[0].Root
	state.RepositoryLabel = filepath.Base(cfg.Workspaces[0].Root)
	roots := make([]string, 0, len(cfg.Workspaces))
	for _, workspace := range cfg.Workspaces {
		roots = append(roots, workspace.Root)
	}
	state.Adapters = service.adapterStates(roots)
	return state, nil
}

func (service *OnboardingService) CreateProject(request EnrollmentRequest) (EnrollmentResult, error) {
	return service.enroll(request, true)
}

func (service *OnboardingService) JoinProject(request EnrollmentRequest) (EnrollmentResult, error) {
	return service.enroll(request, false)
}

func (service *OnboardingService) enroll(request EnrollmentRequest, create bool) (EnrollmentResult, error) {
	root, err := canonicalRepository(request.RepositoryRoot)
	if err != nil {
		return EnrollmentResult{}, err
	}
	request.RepositoryRoot = root
	request.DeviceLabel = boundedLabel(request.DeviceLabel, defaultDeviceLabel())
	// An empty display name is passed through as empty so the member is asked to
	// choose one rather than silently inheriting this machine's hostname.
	displayName, err := boundedDisplayName(request.DisplayName)
	if err != nil {
		return EnrollmentResult{}, err
	}
	request.DisplayName = displayName
	request.ProjectLabel = boundedLabel(request.ProjectLabel, filepath.Base(root))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	options := onboarding.Options{ConfigRoot: service.configRoot, RepositoryRoot: root, APIBaseURL: service.apiBaseURL, ProjectLabel: request.ProjectLabel, DeviceLabel: request.DeviceLabel, DisplayName: request.DisplayName}
	flow := onboarding.New(service.apiBaseURL)
	var result onboarding.Result
	if create {
		result, err = flow.Create(ctx, options)
	} else {
		result, err = flow.Join(ctx, options, strings.TrimSpace(request.JoinCode))
	}
	if err != nil {
		return EnrollmentResult{}, err
	}
	warnings := append([]string{}, service.configureAdapters(root, request.EnableCodex, request.EnableClaude)...)
	return EnrollmentResult{ProjectID: result.ProjectID, JoinCode: result.JoinCode, Warnings: warnings}, nil
}

func (service *OnboardingService) ConfigureAdapters(repositoryRoot string, enableCodex, enableClaude bool) ([]AdapterState, error) {
	root, err := canonicalRepository(repositoryRoot)
	if err != nil {
		return nil, err
	}
	warnings := service.configureAdapters(root, enableCodex, enableClaude)
	states := service.adapterStates([]string{root})
	if len(warnings) > 0 {
		return states, errors.New(strings.Join(warnings, "; "))
	}
	return states, nil
}

func (service *OnboardingService) ReconnectAdapter(repositoryRoot, agent string) (AdapterState, error) {
	root, err := canonicalRepository(repositoryRoot)
	if err != nil {
		return AdapterState{}, err
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return AdapterState{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return AdapterState{}, err
	}
	registered := false
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			registered = true
			break
		}
	}
	if !registered {
		return AdapterState{}, errors.New("adapter reconnect requires a repository registered to this Project")
	}
	executable, err := service.resolveCLI()
	if err != nil {
		return AdapterState{}, err
	}
	switch agent {
	case "codex":
		if _, err = (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Rebind(); err != nil {
			return AdapterState{}, err
		}
		if err = service.clearAgentRuntimeVerification(cfg, root, "codex"); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[0], nil
	case "claude":
		if _, err = (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Rebind(); err != nil {
			return AdapterState{}, err
		}
		if err = service.clearAgentRuntimeVerification(cfg, root, "claude"); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[1], nil
	default:
		return AdapterState{}, errors.New("agent must be codex or claude")
	}
}

func (service *OnboardingService) clearAgentRuntimeVerification(cfg config.Config, root, vendor string) error {
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return err
	}
	db, err := store.Open(paths.DB)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			return db.ClearAgentObservation(ctx, workspace.ID, vendor)
		}
	}
	return nil
}

func (service *OnboardingService) ConnectAgentWorktree(repositoryRoot, agent string) (AdapterState, error) {
	root, err := canonicalRepository(repositoryRoot)
	if err != nil {
		return AdapterState{}, err
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return AdapterState{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return AdapterState{}, err
	}
	if len(cfg.Workspaces) == 0 {
		return AdapterState{}, errors.New("create or join a Project before assigning agent worktrees")
	}
	if root == cfg.Workspaces[0].Root {
		return AdapterState{}, errors.New("choose a distinct linked worktree; the enrolled checkout already has combined observation")
	}
	if err = requireLinkedWorktree(cfg.Workspaces[0].Root, root); err != nil {
		return AdapterState{}, err
	}
	agent = strings.ToLower(strings.TrimSpace(agent))
	if agent != "codex" && agent != "claude" {
		return AdapterState{}, errors.New("agent must be codex or claude")
	}
	executable, err := service.resolveCLI()
	if err != nil {
		return AdapterState{}, err
	}
	if agent == "codex" {
		if status, statusErr := (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status(); statusErr == nil && status.Configured {
			return AdapterState{}, errors.New("this worktree is already assigned to Claude Code; choose a different linked worktree for Codex attribution")
		}
	} else if status, statusErr := (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status(); statusErr == nil && status.Configured {
		return AdapterState{}, errors.New("this worktree is already assigned to Codex; choose a different linked worktree for Claude attribution")
	}
	registered := false
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			registered = true
			break
		}
	}
	if !registered {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "--config-root", service.configRoot, "workspace", "add", "--development", "--root", root)
		output, commandErr := command.CombinedOutput()
		if commandErr != nil {
			message := strings.TrimSpace(string(output))
			if len(message) > 300 {
				message = message[:300]
			}
			if message == "" {
				message = commandErr.Error()
			}
			return AdapterState{}, fmt.Errorf("register linked worktree through the running service: %s", message)
		}
	}
	switch agent {
	case "codex":
		if _, err = (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[0], nil
	case "claude":
		if _, err = (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			return AdapterState{}, err
		}
		return service.adapterStates([]string{root})[1], nil
	}
	return AdapterState{}, errors.New("unreachable agent selection")
}

func (service *OnboardingService) OpenLiveProject(projectID string) (string, error) {
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return "", err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return "", err
	}
	found := false
	for _, workspace := range cfg.Workspaces {
		if workspace.ProjectID == projectID {
			found = true
			break
		}
	}
	if !found || cfg.DeviceID == "" {
		return "", errors.New("Project is not enrolled on this device")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	token, err := credential.Get(ctx, cfg.DeviceID)
	if err != nil {
		return "", err
	}
	client, err := hosted.New(cfg.APIBaseURL, token)
	if err != nil {
		return "", err
	}
	ticket, err := client.CreateDashboardTicket(ctx, projectID)
	if err != nil {
		return "", err
	}
	handoff, err := activation.Start(service.activationBaseURL, ticket.Ticket)
	if err != nil {
		return "", err
	}
	go func() {
		waitContext, stop := context.WithTimeout(context.Background(), 35*time.Second)
		defer stop()
		_ = handoff.Wait(waitContext)
	}()
	return handoff.URL(), nil
}

func (service *OnboardingService) configureAdapters(root string, codex, claude bool) []string {
	if !codex && !claude {
		return nil
	}
	executable, err := service.resolveCLI()
	if err != nil {
		return []string{err.Error()}
	}
	var warnings []string
	if codex {
		if _, err = (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			warnings = append(warnings, "Codex setup: "+err.Error())
		}
	}
	if claude {
		if _, err = (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Setup(); err != nil {
			warnings = append(warnings, "Claude setup: "+err.Error())
		}
	}
	return warnings
}

func (service *OnboardingService) adapterStates(roots []string) []AdapterState {
	executable, cliErr := service.resolveCLI()
	currentProfile := filepath.Base(service.configRoot)
	states := []AdapterState{
		{Name: "Codex", Binding: "not_configured", CurrentProfile: currentProfile, Fidelity: "Live sessions + bounded title intent + tools + safe paths", Detail: "Not connected to this Project yet."},
		{Name: "Claude Code", Binding: "not_configured", CurrentProfile: currentProfile, Fidelity: "Live sessions + bounded title intent + tools + safe paths", Detail: "Not connected to this Project yet."},
	}
	for index, command := range []string{"codex", "claude"} {
		_, states[index].Installed = agentExecutable(command)
	}
	if len(roots) == 0 || cliErr != nil {
		return states
	}
	for _, root := range roots {
		if status, statusErr := (codexsetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status(); statusErr == nil {
			applyAdapterBinding(&states[0], status.Configured, status.Binding, status.PreviousProfile)
		} else if !states[0].Configured {
			states[0].Binding = "drifted"
			states[0].Detail = "Configuration needs review: " + statusErr.Error()
		}
		if status, statusErr := (claudesetup.Manager{ProjectRoot: root, ConfigRoot: service.configRoot, Executable: executable}).Status(); statusErr == nil {
			applyAdapterBinding(&states[1], status.Configured, status.Binding, status.PreviousProfile)
		} else if !states[1].Configured {
			states[1].Binding = "drifted"
			states[1].Detail = "Configuration needs review: " + statusErr.Error()
		}
	}
	for index, vendor := range []string{"codex", "claude"} {
		states[index].RuntimeVerified = service.agentRuntimeVerified(roots, vendor)
		states[index].RestartRequired = states[index].Configured && !states[index].RuntimeVerified
		states[index].ReconnectAllowed = states[index].Binding == "other_profile"
		if states[index].RuntimeVerified {
			states[index].Detail = "Verified by a live session event from this Project. Relevant briefs use MCP pull."
		} else if states[index].RestartRequired {
			states[index].Detail = "Configured for this Project. Restart the agent, then start a new task in this repository to verify the connection."
		}
	}
	return states
}

func applyAdapterBinding(state *AdapterState, configured bool, binding, previousProfile string) {
	state.Configured = state.Configured || configured
	if bindingPriority(binding) < bindingPriority(state.Binding) {
		return
	}
	state.Binding = binding
	if previousProfile != "" {
		state.PreviousProfile = filepath.Base(previousProfile)
	}
	switch binding {
	case "other_profile":
		state.Detail = "Connected to a different Stickguy profile. Reconnect explicitly to move only Stickguy-managed configuration to this Project."
	case "partial":
		state.Detail = "Stickguy found an incomplete connection and can repair the managed entries automatically."
	case "not_configured":
		state.Detail = "Not connected to this Project yet."
	}
}

func bindingPriority(binding string) int {
	switch binding {
	case "drifted":
		return 5
	case "other_profile":
		return 4
	case "partial":
		return 3
	case "current":
		return 2
	default:
		return 1
	}
}

func (service *OnboardingService) agentRuntimeVerified(roots []string, vendor string) bool {
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return false
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return false
	}
	db, err := store.Open(paths.DB)
	if err != nil {
		return false
	}
	defer db.Close()
	allowed := map[string]bool{}
	for _, root := range roots {
		allowed[root] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, workspace := range cfg.Workspaces {
		if !allowed[workspace.Root] {
			continue
		}
		if _, observed, observationErr := db.AgentObserved(ctx, workspace.ID, vendor); observationErr == nil && observed {
			return true
		}
	}
	return false
}

func requireLinkedWorktree(primary, candidate string) error {
	primaryCommon, err := gitCommonDirectory(primary)
	if err != nil {
		return fmt.Errorf("inspect enrolled repository: %w", err)
	}
	candidateCommon, err := gitCommonDirectory(candidate)
	if err != nil {
		return fmt.Errorf("inspect selected repository: %w", err)
	}
	if primaryCommon != candidateCommon {
		return errors.New("selected directory must be an existing linked worktree of the enrolled Git repository")
	}
	return nil
}

func gitCommonDirectory(root string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", errors.New("directory is not a Git worktree")
	}
	value := strings.TrimSpace(string(output))
	if value == "" || len(value) > 4096 {
		return "", errors.New("Git common directory is invalid")
	}
	return filepath.EvalSymlinks(value)
}

func (service *OnboardingService) resolveCLI() (string, error) {
	if service.cliBinary != "" {
		absolute, err := filepath.Abs(service.cliBinary)
		if err == nil {
			if info, statErr := os.Stat(absolute); statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return absolute, nil
			}
		}
		return "", errors.New("Stickguy development CLI is missing; restart pnpm dev so it can be rebuilt")
	}
	value, err := exec.LookPath("stickguy")
	if err != nil {
		return "", errors.New("Stickguy CLI is not installed; agent setup is unavailable")
	}
	return filepath.Abs(value)
}

func agentExecutable(command string) (string, bool) {
	if value, err := exec.LookPath(command); err == nil && executableFile(value) {
		return value, true
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	var candidates []string
	switch command {
	case "codex":
		candidates = []string{
			filepath.Join(home, ".local", "bin", "codex"),
			filepath.Join(home, ".codex", "bin", "codex"),
			filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex"),
			filepath.Join(home, "Applications", "ChatGPT.app", "Contents", "Resources", "codex"),
			"/Applications/Codex.app/Contents/Resources/codex",
			"/Applications/ChatGPT.app/Contents/Resources/codex",
		}
	case "claude":
		candidates = []string{
			filepath.Join(home, ".local", "bin", "claude"),
			filepath.Join(home, ".npm-global", "bin", "claude"),
		}
		nvmCandidates, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "claude"))
		candidates = append(candidates, nvmCandidates...)
	default:
		return "", false
	}
	for _, candidate := range candidates {
		if executableFile(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func canonicalRepository(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("choose a repository")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve repository: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonicalize repository: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("repository is not a directory")
	}
	return resolved, nil
}

func defaultDeviceLabel() string {
	host, _ := os.Hostname()
	return boundedLabel(host, "This Mac")
}

// boundedDisplayName enforces the ADR-035 identity rules before any network
// call so the desktop reports a clear reason instead of a hosted error code.
func boundedDisplayName(value string) (string, error) {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "", nil
	}
	if utf8.RuneCountInString(value) < 2 || utf8.RuneCountInString(value) > 60 {
		return "", errors.New("choose a display name between 2 and 60 characters")
	}
	if strings.ContainsRune(value, '@') {
		return "", errors.New("choose a display name; an email address cannot be your Project identity")
	}
	return value, nil
}

func boundedLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	characters := []rune(value)
	if len(characters) > 80 {
		value = string(characters[:80])
	}
	return value
}

// SessionDetail exposes the caller's own local session content to the embedded
// dashboard. Content is read on demand from the vendor transcript and is never
// uploaded; sharing it with the Project stays a separate, explicit choice.
func (service *OnboardingService) SessionDetail(workstreamID string) (SessionDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return newDaemonService().SessionDetail(ctx, workstreamID)
}
