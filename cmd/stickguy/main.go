package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/stickguy/stickguy/internal/activation"
	"github.com/stickguy/stickguy/internal/agentactivity"
	"github.com/stickguy/stickguy/internal/app"
	"github.com/stickguy/stickguy/internal/claudesetup"
	"github.com/stickguy/stickguy/internal/codexsetup"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/credential"
	"github.com/stickguy/stickguy/internal/daemon"
	"github.com/stickguy/stickguy/internal/hosted"
	coordinationmcp "github.com/stickguy/stickguy/internal/mcp"
	"github.com/stickguy/stickguy/internal/onboarding"
	servicemanager "github.com/stickguy/stickguy/internal/service"
	updateclient "github.com/stickguy/stickguy/internal/update"
	"io"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
	// updatePublicKey is an Ed25519 public key encoded as standard base64. The
	// release workflow injects it with -ldflags; development binaries refuse
	// remote updates instead of trusting unsigned metadata.
	updatePublicKey = ""
)

type versionInfo struct {
	Version        string `json:"version"`
	Commit         string `json:"commit"`
	BuildTime      string `json:"buildTime"`
	SchemaMinimum  int    `json:"schemaMinimum"`
	SchemaMaximum  int    `json:"schemaMaximum"`
	ArtifactSHA256 string `json:"artifactSha256"`
}

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 2 && args[0] == "version" && args[1] == "--json" {
		identity, _ := executableIdentity()
		return json.NewEncoder(os.Stdout).Encode(versionInfo{version, commit, buildTime, 1, 1, identity})
	}
	fs := flag.NewFlagSet("stickguy", flag.ContinueOnError)
	root := fs.String("config-root", "", "isolated per-user state root")
	apiBase := fs.String("api", "https://api.stickguy.dev", "Stickguy API origin")
	if e := fs.Parse(args); e != nil {
		return e
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("usage: stickguy [--config-root <dir>] create|join|dashboard|mcp|setup|service|workspace|intent|pause|resume|doctor|diagnostics|scan|update")
	}
	customConfigRoot := *root != ""
	if *root == "" {
		var err error
		*root, err = config.DefaultRoot()
		if err != nil {
			return err
		}
	}
	paths, e := config.Resolve(*root)
	if e != nil {
		if len(rest) > 0 && rest[0] == "agent-hook" {
			return nil
		}
		return e
	}
	ctx := context.Background()
	switch rest[0] {
	case "create":
		createFlags := flag.NewFlagSet("create", flag.ContinueOnError)
		label := createFlags.String("label", "", "Project label")
		deviceLabel := createFlags.String("device-label", "", "device label shared with Project members")
		repository := createFlags.String("root", ".", "Git repository root")
		if e = createFlags.Parse(rest[1:]); e != nil {
			return e
		}
		service := onboarding.New(*apiBase)
		result, createErr := service.Create(ctx, onboarding.Options{ConfigRoot: *root, RepositoryRoot: *repository, APIBaseURL: *apiBase, ProjectLabel: *label, DeviceLabel: *deviceLabel})
		if createErr != nil {
			return createErr
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			ProjectID    string `json:"projectId"`
			DeviceID     string `json:"deviceId"`
			WorkspaceID  string `json:"workspaceId"`
			WorkstreamID string `json:"workstreamId"`
			JoinCode     string `json:"joinCode"`
		}{result.ProjectID, result.DeviceID, result.WorkspaceID, result.WorkstreamID, result.JoinCode})
	case "join":
		joinFlags := flag.NewFlagSet("join", flag.ContinueOnError)
		deviceLabel := joinFlags.String("device-label", "", "device label shared with Project members")
		repository := joinFlags.String("root", ".", "Git repository root")
		if e = joinFlags.Parse(rest[1:]); e != nil {
			return e
		}
		if joinFlags.NArg() != 1 {
			return errors.New("join requires one invite code")
		}
		service := onboarding.New(*apiBase)
		result, joinErr := service.Join(ctx, onboarding.Options{ConfigRoot: *root, RepositoryRoot: *repository, APIBaseURL: *apiBase, DeviceLabel: *deviceLabel}, joinFlags.Arg(0))
		if joinErr != nil {
			return joinErr
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			ProjectID    string `json:"projectId"`
			DeviceID     string `json:"deviceId"`
			WorkspaceID  string `json:"workspaceId"`
			WorkstreamID string `json:"workstreamId"`
		}{result.ProjectID, result.DeviceID, result.WorkspaceID, result.WorkstreamID})
	case "dashboard":
		dashboardFlags := flag.NewFlagSet("dashboard", flag.ContinueOnError)
		projectID := dashboardFlags.String("project", "", "Project id")
		if e = dashboardFlags.Parse(rest[1:]); e != nil {
			return e
		}
		cfg, loadErr := config.Load(paths)
		if loadErr != nil {
			return loadErr
		}
		if *projectID == "" || cfg.DeviceID == "" || cfg.APIBaseURL == "" {
			return errors.New("dashboard requires an enrolled service and project id")
		}
		token, credentialErr := credential.Get(ctx, cfg.DeviceID)
		if credentialErr != nil {
			return credentialErr
		}
		client, clientErr := hosted.New(cfg.APIBaseURL, token)
		if clientErr != nil {
			return clientErr
		}
		ticket, ticketErr := client.CreateDashboardTicket(ctx, *projectID)
		if ticketErr != nil {
			return ticketErr
		}
		return activation.Open(ctx, cfg.APIBaseURL, ticket.Ticket)
	case "mcp":
		if len(rest) != 1 {
			return errors.New("mcp accepts no arguments")
		}
		return coordinationmcp.Run(ctx, *root)
	case "agent-hook":
		hookFlags := flag.NewFlagSet("agent-hook", flag.ContinueOnError)
		vendor := hookFlags.String("vendor", "", "supported coding-agent vendor")
		if e = hookFlags.Parse(rest[1:]); e != nil {
			return nil
		}
		return runAgentHook(ctx, paths.Socket, *vendor, os.Stdin, os.Stdout, daemon.Call)
	case "setup":
		if len(rest) < 2 || !map[string]bool{"codex": true, "claude": true, "status": true, "remove": true, "reconnect": true}[rest[1]] {
			return errors.New("setup requires codex, claude, status, reconnect, or remove")
		}
		setupFlags := flag.NewFlagSet("setup "+rest[1], flag.ContinueOnError)
		projectRoot := setupFlags.String("project-root", ".", "trusted coding-agent project root")
		agent := setupFlags.String("agent", "codex", "coding agent for status/remove: codex or claude")
		development := setupFlags.Bool("development", false, "explicitly install the local development MCP adapter")
		if e = setupFlags.Parse(rest[2:]); e != nil {
			return e
		}
		executable, executableErr := os.Executable()
		if executableErr != nil {
			return executableErr
		}
		selected := rest[1]
		if selected == "status" || selected == "remove" || selected == "reconnect" {
			selected = *agent
		}
		if selected != "codex" && selected != "claude" {
			return errors.New("setup agent must be codex or claude")
		}
		var status any
		var setupErr error
		if selected == "codex" {
			manager := codexsetup.Manager{ProjectRoot: *projectRoot, ConfigRoot: *root, Executable: executable, Portable: !customConfigRoot && !*development}
			switch rest[1] {
			case "codex":
				status, setupErr = manager.Setup()
			case "status":
				status, setupErr = manager.Status()
			case "remove":
				status, setupErr = manager.Remove()
			case "reconnect":
				status, setupErr = manager.Rebind()
			default:
				return errors.New("setup command and agent do not match")
			}
		} else {
			manager := claudesetup.Manager{ProjectRoot: *projectRoot, ConfigRoot: *root, Executable: executable, Portable: !customConfigRoot && !*development}
			switch rest[1] {
			case "claude":
				status, setupErr = manager.Setup()
			case "status":
				status, setupErr = manager.Status()
			case "remove":
				status, setupErr = manager.Remove()
			case "reconnect":
				status, setupErr = manager.Rebind()
			default:
				return errors.New("setup command and agent do not match")
			}
		}
		if setupErr != nil {
			return setupErr
		}
		return json.NewEncoder(os.Stdout).Encode(status)
	case "service":
		if len(rest) != 2 {
			return errors.New("service requires run, install, start, stop, status, or remove")
		}
		if rest[1] == "run" {
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			sender, senderErr := app.NewHostedSender(ctx, *root)
			if senderErr != nil {
				return senderErr
			}
			return app.Run(ctx, *root, sender)
		}
		if rest[1] == "status" {
			if response, callErr := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "health"}); callErr == nil && response.OK {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{"service": "running", "health": response.Data})
			}
		}
		executable, executableErr := os.Executable()
		if executableErr != nil {
			return executableErr
		}
		executable, executableErr = filepath.EvalSymlinks(executable)
		if executableErr != nil {
			return fmt.Errorf("resolve service executable: %w", executableErr)
		}
		home, uid, accountErr := currentAccount()
		if accountErr != nil {
			return accountErr
		}
		manager := servicemanager.Manager{Executable: executable, ConfigRoot: paths.Root, Home: home, UID: uid}
		switch rest[1] {
		case "install":
			return manager.Install(ctx)
		case "start":
			return manager.Start(ctx)
		case "stop":
			return manager.Stop(ctx)
		case "remove":
			return manager.Remove(ctx)
		case "status":
			status, statusErr := manager.Status(ctx)
			if statusErr != nil {
				return statusErr
			}
			return json.NewEncoder(os.Stdout).Encode(status)
		}
	case "workspace":
		if len(rest) >= 2 && rest[1] == "list" {
			cfg, loadErr := config.Load(paths)
			if loadErr != nil {
				return loadErr
			}
			return json.NewEncoder(os.Stdout).Encode(cfg.Workspaces)
		}
		if len(rest) < 2 || rest[1] != "add" {
			break
		}
		wf := flag.NewFlagSet("workspace add", flag.ContinueOnError)
		id := wf.String("id", "", "")
		project := wf.String("project", "", "")
		workstream := wf.String("workstream", "", "")
		member := wf.String("member", "", "")
		device := wf.String("device", "", "")
		session := wf.String("session", "", "")
		apiBase := wf.String("api", "", "")
		repo := wf.String("root", "", "")
		development := wf.Bool("development", false, "derive a second local workstream from the enrolled development profile")
		if e = wf.Parse(rest[2:]); e != nil {
			return e
		}
		if *development {
			if *repo == "" {
				return errors.New("development workspace add requires root")
			}
			cfg, loadErr := config.Load(paths)
			if loadErr != nil {
				return loadErr
			}
			if cfg.DeviceID == "" || cfg.APIBaseURL == "" || len(cfg.Workspaces) == 0 {
				return errors.New("development profile is not enrolled; run stickguy create first")
			}
			projectIDs := map[string]bool{}
			for _, existing := range cfg.Workspaces {
				projectIDs[existing.ProjectID] = true
			}
			if *project == "" {
				if len(projectIDs) != 1 {
					return errors.New("multiple Projects are enrolled; specify --project")
				}
				for value := range projectIDs {
					*project = value
				}
			}
			var source config.Workspace
			for _, existing := range cfg.Workspaces {
				if existing.ProjectID == *project {
					source = existing
					break
				}
			}
			if source.ID == "" {
				return errors.New("selected Project is not enrolled")
			}
			var idErr error
			if *id, idErr = devID("wsp_"); idErr == nil {
				*workstream, idErr = devID("wrk_")
			}
			if idErr == nil {
				*session, idErr = devID("ses_")
			}
			if idErr != nil {
				return idErr
			}
			*member, *device, *apiBase = source.MemberID, cfg.DeviceID, cfg.APIBaseURL
			request := daemon.Request{Method: "add_development_workspace", WorkspaceID: *id, ProjectID: *project, WorkstreamID: *workstream, MemberID: *member, SessionID: *session, Root: *repo}
			response, callErr := daemon.Call(ctx, paths.Socket, request)
			if callErr == nil {
				if !response.OK {
					return errors.New(response.Error)
				}
				return json.NewEncoder(os.Stdout).Encode(response.Data)
			}
			workspace := config.Workspace{ID: *id, ProjectID: *project, WorkstreamID: *workstream, MemberID: *member, SessionID: *session, Root: *repo}
			if addErr := app.Register(ctx, *root, *apiBase, *device, workspace); addErr != nil {
				return fmt.Errorf("add stopped-service development workspace after IPC unavailable: %w", addErr)
			}
			return json.NewEncoder(os.Stdout).Encode(workspace)
		}
		if *id == "" || *project == "" || *workstream == "" || *member == "" || *device == "" || *session == "" || *apiBase == "" || *repo == "" {
			return errors.New("workspace add requires id, project, workstream, member, device, session, api, root")
		}
		return app.Register(ctx, *root, *apiBase, *device, config.Workspace{ID: *id, ProjectID: *project, WorkstreamID: *workstream, MemberID: *member, SessionID: *session, Root: *repo})
	case "pause", "resume":
		pf := flag.NewFlagSet(rest[0], flag.ContinueOnError)
		id := pf.String("workspace", "", "")
		if e = pf.Parse(rest[1:]); e != nil {
			return e
		}
		if *id == "" {
			return errors.New("workspace id required")
		}
		return printCall(ctx, paths.Socket, daemon.Request{Method: rest[0], WorkspaceID: *id})
	case "intent":
		intentFlags := flag.NewFlagSet("intent", flag.ContinueOnError)
		workspaceID := intentFlags.String("workspace", "", "workspace id")
		title := intentFlags.String("title", "", "short workstream title")
		outcome := intentFlags.String("outcome", "", "intended outcome")
		approach := intentFlags.String("approach", "", "optional approach summary")
		if e = intentFlags.Parse(rest[1:]); e != nil {
			return e
		}
		if *workspaceID == "" || *title == "" || *outcome == "" {
			return errors.New("intent requires workspace, title, and outcome")
		}
		return printCall(ctx, paths.Socket, daemon.Request{Method: "intent", WorkspaceID: *workspaceID, Title: *title, IntendedOutcome: *outcome, ApproachSummary: *approach})
	case "doctor", "scan":
		return printCall(ctx, paths.Socket, daemon.Request{Method: rest[0]})
	case "diagnostics":
		return writeDiagnostics(ctx, paths)
	case "update":
		updateFlags := flag.NewFlagSet("update", flag.ContinueOnError)
		manifestURL := updateFlags.String("manifest", "https://github.com/stickguy/stickguy/releases/latest/download/update-manifest.json", "signed update metadata URL")
		if e = updateFlags.Parse(rest[1:]); e != nil {
			return e
		}
		executable, executableErr := os.Executable()
		if executableErr != nil {
			return executableErr
		}
		executable, executableErr = filepath.EvalSymlinks(executable)
		if executableErr != nil {
			return executableErr
		}
		if updateFlags.NArg() == 1 && updateFlags.Arg(0) == "rollback" {
			result, rollbackErr := updateclient.Rollback(executable)
			if rollbackErr != nil {
				return rollbackErr
			}
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		if updateFlags.NArg() != 0 {
			return errors.New("update accepts no argument or rollback")
		}
		publicKey, keyErr := updateclient.ParsePublicKey(updatePublicKey)
		if keyErr != nil {
			return keyErr
		}
		client := updateclient.Client{PublicKey: publicKey}
		manifest, manifestErr := client.FetchManifest(ctx, *manifestURL)
		if manifestErr != nil {
			return manifestErr
		}
		if manifest.Version == version {
			return json.NewEncoder(os.Stdout).Encode(updateclient.Result{Version: version, Updated: false})
		}
		result, applyErr := client.Apply(ctx, manifest, executable)
		if applyErr != nil {
			return applyErr
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	return errors.New("unsupported command")
}

type daemonCaller func(context.Context, string, daemon.Request) (daemon.Response, error)

func runAgentHook(ctx context.Context, socket, vendor string, stdin io.Reader, stdout io.Writer, call daemonCaller) error {
	hookContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	input, err := io.ReadAll(io.LimitReader(stdin, agentactivity.MaxInputBytes+1))
	if err != nil {
		return nil
	}
	event, err := agentactivity.Parse(vendor, input)
	if err != nil {
		return nil
	}
	request := daemon.Request{
		AgentVendor: event.Vendor, AgentCWD: event.CWD,
		AgentWorkstreamID: event.WorkstreamID, AgentSessionAlias: event.SessionAlias,
		AgentEvent: event.Kind, AgentStatus: event.Status, AgentAction: event.Action,
		AgentTool: event.Tool, AgentType: event.AgentType, AgentSubagentAlias: event.SubagentAlias,
		AgentPaths:          event.CandidatePaths,
		AgentTranscriptPath: event.TranscriptPath, AgentVendorSessionID: event.VendorSessionID,
	}
	// Observation and delivery get separate budgets inside the overall hook
	// deadline. Sharing one budget let a slow observation — a cold service or a
	// slow branch read — consume the whole window, so the turn that most needed
	// a correction was the turn least likely to receive one.
	request.Method = "agent_event"
	observeContext, cancelObserve := context.WithTimeout(hookContext, 700*time.Millisecond)
	_, err = call(observeContext, socket, request)
	cancelObserve()
	if event.Kind != "SessionStart" && event.Kind != "UserPromptSubmit" {
		return nil
	}
	// A failed observation never suppresses a pending correction: the event is
	// already durable service-side, and only the acknowledgement was lost.
	request.Method = "agent_injection"
	injectContext, cancelInject := context.WithTimeout(hookContext, 1200*time.Millisecond)
	response, err := call(injectContext, socket, request)
	cancelInject()
	if err != nil || !response.OK {
		return nil
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil {
		return nil
	}
	var injection struct {
		AdditionalContext string `json:"additionalContext"`
	}
	if err = json.Unmarshal(encoded, &injection); err != nil || injection.AdditionalContext == "" {
		return nil
	}
	type hookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	}
	output, err := json.Marshal(struct {
		HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
	}{HookSpecificOutput: hookSpecificOutput{HookEventName: event.Kind, AdditionalContext: injection.AdditionalContext}})
	if err != nil {
		return nil
	}
	_, _ = stdout.Write(append(output, '\n'))
	return nil
}

func devID(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate development identifier: %w", err)
	}
	return prefix + hex.EncodeToString(value[:]), nil
}
func printCall(ctx context.Context, socket string, q daemon.Request) error {
	r, e := daemon.Call(ctx, socket, q)
	if e != nil {
		return e
	}
	if !r.OK {
		return errors.New(r.Error)
	}
	return json.NewEncoder(os.Stdout).Encode(r)
}

func currentAccount() (string, int, error) {
	account, err := user.Current()
	if err != nil {
		return "", 0, fmt.Errorf("resolve current user: %w", err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid <= 0 || !filepath.IsAbs(account.HomeDir) {
		return "", 0, errors.New("current user has invalid home or uid")
	}
	return account.HomeDir, uid, nil
}

func executableIdentity() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, io.LimitReader(file, 250<<20)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeDiagnostics(ctx context.Context, paths config.Paths) error {
	report := map[string]any{"schemaVersion": 1, "version": version, "commit": commit, "buildTime": buildTime, "platform": runtime.GOOS + "/" + runtime.GOARCH}
	if identity, err := executableIdentity(); err == nil {
		report["artifactSha256"] = identity
	}
	if cfg, err := config.Load(paths); err == nil {
		report["configVersion"] = cfg.Version
		report["workspaceCount"] = len(cfg.Workspaces)
	} else {
		report["configState"] = "unreadable"
	}
	if stat, err := os.Stat(paths.DB); err == nil {
		report["databaseBytes"] = stat.Size()
	} else if os.IsNotExist(err) {
		report["databaseState"] = "absent"
	} else {
		report["databaseState"] = "unreadable"
	}
	if response, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "doctor"}); err == nil && response.OK {
		report["service"] = response.Data
	} else {
		report["service"] = map[string]any{"status": "unavailable"}
	}
	// Project IDs, repository paths, environment values, credentials, event
	// payloads, command output, and raw errors are deliberately absent.
	return json.NewEncoder(os.Stdout).Encode(report)
}
