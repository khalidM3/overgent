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
	"github.com/overgent/overgent/internal/activation"
	"github.com/overgent/overgent/internal/agentactivity"
	"github.com/overgent/overgent/internal/app"
	"github.com/overgent/overgent/internal/claudesetup"
	"github.com/overgent/overgent/internal/codexsetup"
	"github.com/overgent/overgent/internal/config"
	"github.com/overgent/overgent/internal/credential"
	"github.com/overgent/overgent/internal/cursorsetup"
	"github.com/overgent/overgent/internal/daemon"
	"github.com/overgent/overgent/internal/hosted"
	coordinationmcp "github.com/overgent/overgent/internal/mcp"
	"github.com/overgent/overgent/internal/onboarding"
	servicemanager "github.com/overgent/overgent/internal/service"
	updateclient "github.com/overgent/overgent/internal/update"
	"io"
	"os"
	"os/exec"
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
	fs := flag.NewFlagSet("overgent", flag.ContinueOnError)
	root := fs.String("config-root", "", "isolated per-user state root")
	apiBase := fs.String("api", "https://api.overgent.com", "Overgent API origin")
	if e := fs.Parse(args); e != nil {
		return e
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("usage: overgent [--config-root <dir>] create|join|reset|dashboard|mcp|setup|service|workspace|intent|pause|resume|focus|unfocus|doctor|diagnostics|scan|update")
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
		// A profile that has already enrolled has a device identity, and one
		// per-user service keeps one identity across all of its Projects. Minting
		// a second credential here would strand the Projects the first one owns,
		// so an enrolled profile takes the additional-Project path instead of
		// failing and leaving `workspace add` as the only undocumented way
		// through. The desktop app has always done this; the CLI now matches it.
		existing, existingErr := enrolledDevice(paths, *repository)
		if existingErr != nil {
			return existingErr
		}
		var result onboarding.Result
		var createErr error
		if existing.deviceID != "" {
			token, tokenErr := credential.Get(ctx, existing.deviceID)
			if tokenErr != nil {
				return fmt.Errorf("read existing device credential: %w", tokenErr)
			}
			service := onboarding.New(existing.apiBaseURL)
			result, createErr = service.CreateAdditional(ctx, onboarding.Options{ConfigRoot: *root, RepositoryRoot: *repository, APIBaseURL: existing.apiBaseURL, ProjectLabel: *label, DeviceLabel: *deviceLabel, AppVersion: "overgent/" + version}, existing.deviceID, token)
		} else {
			service := onboarding.New(*apiBase)
			result, createErr = service.Create(ctx, onboarding.Options{ConfigRoot: *root, RepositoryRoot: *repository, APIBaseURL: *apiBase, ProjectLabel: *label, DeviceLabel: *deviceLabel, AppVersion: "overgent/" + version})
		}
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
			return errors.New("join requires one invite link or code")
		}
		service := onboarding.New(*apiBase)
		result, joinErr := service.Join(ctx, onboarding.Options{ConfigRoot: *root, RepositoryRoot: *repository, APIBaseURL: *apiBase, DeviceLabel: *deviceLabel, AppVersion: "overgent/" + version}, joinFlags.Arg(0))
		if joinErr != nil {
			return joinErr
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			ProjectID    string `json:"projectId"`
			DeviceID     string `json:"deviceId"`
			WorkspaceID  string `json:"workspaceId"`
			WorkstreamID string `json:"workstreamId"`
		}{result.ProjectID, result.DeviceID, result.WorkspaceID, result.WorkstreamID})
	case "reset":
		// Recovery for a device whose credential the hosted API no longer
		// accepts - revoked by an owner, or unknown to the deployment. The
		// desktop app offers the same action; this is the headless path.
		resetFlags := flag.NewFlagSet("reset", flag.ContinueOnError)
		force := resetFlags.Bool("force", false, "clear the local enrollment even if the credential could not be verified")
		if e = resetFlags.Parse(rest[1:]); e != nil {
			return e
		}
		outcome, resetErr := onboarding.New(*apiBase).Reset(ctx, *root, *force)
		if resetErr != nil {
			return resetErr
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			Credential        string `json:"credential"`
			DeviceID          string `json:"deviceId,omitempty"`
			ClearedWorkspaces int    `json:"clearedWorkspaces"`
			CredentialDeleted bool   `json:"credentialDeleted"`
		}{string(outcome.Status), outcome.DeviceID, outcome.ClearedWorkspaces, outcome.CredentialDeleted})
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
		// Cursor's afterFileEdit and beforeSubmitPrompt payloads carry no
		// hook_event_name, so the managed command states which event it was
		// installed for. Claude and Codex name the event in the payload and
		// ignore this flag.
		event := hookFlags.String("event", "", "vendor hook event name, for vendors whose payload omits it")
		if e = hookFlags.Parse(rest[1:]); e != nil {
			return nil
		}
		if *vendor == "cursor" {
			return runCursorHook(ctx, paths.Socket, *event, os.Stdin, os.Stdout, daemon.Call)
		}
		return runAgentHook(ctx, paths.Socket, *vendor, os.Stdin, os.Stdout, daemon.Call)
	case "setup":
		if len(rest) < 2 || !map[string]bool{"codex": true, "claude": true, "cursor": true, "status": true, "remove": true, "remove-all": true, "reconnect": true}[rest[1]] {
			return errors.New("setup requires codex, claude, cursor, status, reconnect, remove, or remove-all")
		}
		if rest[1] == "remove-all" {
			if len(rest) != 2 {
				return errors.New("setup remove-all accepts no arguments")
			}
			return removeAllAgentBindings(*root, paths, !customConfigRoot)
		}
		setupFlags := flag.NewFlagSet("setup "+rest[1], flag.ContinueOnError)
		projectRoot := setupFlags.String("project-root", ".", "trusted coding-agent project root")
		agent := setupFlags.String("agent", "codex", "coding agent for status/remove: codex, claude, or cursor")
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
		if selected != "codex" && selected != "claude" && selected != "cursor" {
			return errors.New("setup agent must be codex, claude, or cursor")
		}
		var status any
		var setupErr error
		if selected == "cursor" {
			manager := cursorsetup.Manager{ProjectRoot: *projectRoot, ConfigRoot: *root, Executable: executable, Portable: !customConfigRoot && !*development}
			switch rest[1] {
			case "cursor":
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
		} else if selected == "codex" {
			manager := codexsetup.Manager{ProjectRoot: *projectRoot, ConfigRoot: *root, Executable: executable, Portable: !customConfigRoot && !*development, Version: version}
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
				return errors.New("development profile is not enrolled; run overgent create first")
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
		id := pf.String("workspace", "", "workspace id")
		project := pf.String("project", "", "project id: every workspace registered to it on this device")
		if e = pf.Parse(rest[1:]); e != nil {
			return e
		}
		if *id == "" && *project == "" {
			return errors.New("workspace or project id required")
		}
		if *id != "" && *project != "" {
			return errors.New("name a workspace or a project, not both")
		}
		return printCall(ctx, paths.Socket, daemon.Request{Method: rest[0], WorkspaceID: *id, ProjectID: *project})
	case "focus", "unfocus":
		// Focus is the inbound control: it stops coordination being injected
		// into one agent session's turns and changes nothing about what this
		// device publishes. It always expires.
		ff := flag.NewFlagSet(rest[0], flag.ContinueOnError)
		session := ff.String("session", "", "agent session workstream id")
		minutes := ff.Int64("minutes", 0, "how long to stay quiet; default 60, maximum 480")
		if e = ff.Parse(rest[1:]); e != nil {
			return e
		}
		if *session == "" {
			return errors.New("session id required")
		}
		if *minutes < 0 {
			return errors.New("minutes must not be negative")
		}
		return printCall(ctx, paths.Socket, daemon.Request{Method: rest[0], AgentWorkstreamID: *session, FocusSeconds: *minutes * 60})
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
		manifestURL := updateFlags.String("manifest", "https://github.com/overgent/overgent/releases/latest/download/update-manifest.json", "signed update metadata URL")
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
			if activateErr := activateUpdatedExecutable(ctx, executable, paths); activateErr != nil {
				return fmt.Errorf("rollback executable was restored but did not become healthy: %w", activateErr)
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
		if activateErr := activateUpdatedExecutable(ctx, executable, paths); activateErr != nil {
			if _, rollbackErr := updateclient.Rollback(executable); rollbackErr != nil {
				return fmt.Errorf("updated executable failed validation (%v) and rollback failed: %w", activateErr, rollbackErr)
			}
			_ = restartInstalledService(ctx, executable, paths)
			return fmt.Errorf("updated executable failed validation and was rolled back: %w", activateErr)
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
	// PostToolUse is the only boundary that can reach an agent working
	// autonomously through a long turn before its work lands (B28). Claude
	// renders hookSpecificOutput.additionalContext there; other vendors do
	// not, so for them mid-turn stays a no-op. The daemon rate-limits the
	// fetch and restricts mid-turn payloads to coordination_required items.
	midTurnBoundary := event.Kind == "PostToolUse" && event.Vendor == "claude"
	if event.Kind != "SessionStart" && event.Kind != "UserPromptSubmit" && !midTurnBoundary {
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

// runCursorHook is Cursor's hook entry point. It is separate from runAgentHook
// because three things differ, none of them cosmetic:
//
//   - stdin is streamed rather than buffered. beforeReadFile sends the whole
//     file being read, and buffering it against MaxInputBytes would reject every
//     read of a file over 256 KiB, silently emptying the read set while the
//     session still reported `observed` coverage.
//   - the event name comes from the installed command, because afterFileEdit and
//     beforeSubmitPrompt carry none.
//   - the response shape is Cursor's `additional_context`/`env`, not Claude's
//     `hookSpecificOutput`.
//
// Like every other hook path it fails open: every error returns nil, so Cursor's
// turn proceeds unchanged whatever Overgent could not do (ADR-017).
func runCursorHook(ctx context.Context, socket, event string, stdin io.Reader, stdout io.Writer, call daemonCaller) error {
	hookContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if !agentactivity.SupportedCursorEvent(event) {
		return nil
	}
	// The workspace root Overgent published from this session's sessionStart.
	// afterFileEdit and beforeSubmitPrompt report no root of their own, so
	// without this they cannot be attributed to a repository at all.
	parsed, err := agentactivity.ParseCursor(event, stdin, os.Getenv(agentactivity.CursorWorkspaceRootEnv))
	if err != nil {
		return nil
	}
	request := daemon.Request{
		AgentVendor: parsed.Vendor, AgentCWD: parsed.CWD, AgentCandidateRoots: parsed.CandidateRoots,
		AgentWorkstreamID: parsed.WorkstreamID, AgentSessionAlias: parsed.SessionAlias,
		AgentEvent: parsed.Kind, AgentStatus: parsed.Status, AgentAction: parsed.Action,
		AgentTool: parsed.Tool, AgentPaths: parsed.CandidatePaths,
		AgentSessionTitle: parsed.SessionTitle, AgentVendorSessionID: parsed.VendorSessionID,
	}
	request.Method = "agent_event"
	observeContext, cancelObserve := context.WithTimeout(hookContext, 700*time.Millisecond)
	observed, observeErr := call(observeContext, socket, request)
	cancelObserve()

	output := map[string]any{}
	// Only sessionStart can publish session-scoped variables, and it publishes
	// the root the service actually resolved rather than the one Cursor
	// reported, so a multi-root workspace pins the registered repository for the
	// rest of the session.
	if event == "sessionStart" && observeErr == nil && observed.OK {
		if root := resolvedWorkspaceRoot(observed.Data); root != "" {
			output["env"] = map[string]string{agentactivity.CursorWorkspaceRootEnv: root}
		}
	}
	if parsed.Kind == "SessionStart" || parsed.Kind == "UserPromptSubmit" {
		request.Method = "agent_injection"
		injectContext, cancelInject := context.WithTimeout(hookContext, 1200*time.Millisecond)
		response, injectErr := call(injectContext, socket, request)
		cancelInject()
		if injectErr == nil && response.OK {
			if encoded, marshalErr := json.Marshal(response.Data); marshalErr == nil {
				var injection struct {
					AdditionalContext string `json:"additionalContext"`
				}
				if json.Unmarshal(encoded, &injection) == nil && injection.AdditionalContext != "" {
					output["additional_context"] = injection.AdditionalContext
				}
			}
		}
	}
	if len(output) == 0 {
		return nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return nil
	}
	_, _ = stdout.Write(append(encoded, '\n'))
	return nil
}

func resolvedWorkspaceRoot(data any) string {
	encoded, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	var accepted struct {
		WorkspaceRoot string `json:"workspaceRoot"`
	}
	if json.Unmarshal(encoded, &accepted) != nil {
		return ""
	}
	return accepted.WorkspaceRoot
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
		report["service"] = safeDoctorSummary(response.Data)
	} else {
		report["service"] = map[string]any{"status": "unavailable"}
	}
	// Project IDs, repository paths, environment values, credentials, event
	// payloads, command output, and raw errors are deliberately absent.
	return json.NewEncoder(os.Stdout).Encode(report)
}

func safeDoctorSummary(value any) map[string]any {
	safe := map[string]any{"status": "unavailable"}
	object, ok := value.(map[string]any)
	if !ok {
		return safe
	}
	allowedStrings := []string{"status", "lastPublishError"}
	allowedNumbers := []string{"bootCount", "workspaces", "pausedWorkspaces", "focusedSessions", "pending", "quarantined", "scans", "scanCycles"}
	for _, key := range allowedStrings {
		if item, ok := object[key].(string); ok && len(item) <= 40 && (key != "lastPublishError" || isDegradedReason(item)) {
			safe[key] = item
		}
	}
	for _, key := range allowedNumbers {
		if item, ok := object[key].(float64); ok && item >= 0 {
			safe[key] = item
		} else if item, ok := object[key].(int); ok && item >= 0 {
			safe[key] = item
		} else if item, ok := object[key].(int64); ok && item >= 0 {
			safe[key] = item
		}
	}
	return safe
}

func isDegradedReason(value string) bool {
	switch value {
	case "", "not_configured", "quota", "provider_error", "offline", "paused", "rejected":
		return true
	default:
		return false
	}
}

func removeAllAgentBindings(configRoot string, paths config.Paths, portable bool) error {
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	roots := map[string]bool{}
	for _, workspace := range cfg.Workspaces {
		if filepath.IsAbs(workspace.Root) {
			roots[workspace.Root] = true
		}
	}
	for root := range roots {
		if _, err = (codexsetup.Manager{ProjectRoot: root, ConfigRoot: configRoot, Executable: executable, Portable: portable}).Remove(); err != nil {
			return fmt.Errorf("remove managed Codex binding from %s: %w", root, err)
		}
		if _, err = (claudesetup.Manager{ProjectRoot: root, ConfigRoot: configRoot, Executable: executable, Portable: portable}).Remove(); err != nil {
			return fmt.Errorf("remove managed Claude binding from %s: %w", root, err)
		}
		if _, err = (cursorsetup.Manager{ProjectRoot: root, ConfigRoot: configRoot, Executable: executable, Portable: portable}).Remove(); err != nil {
			return fmt.Errorf("remove managed Cursor binding from %s: %w", root, err)
		}
	}
	// Codex hooks live at the user layer and are shared by every project, so
	// they are torn down once, after the last project binding is gone.
	if err = (codexsetup.Manager{ConfigRoot: configRoot, Executable: executable, Portable: portable}).RemoveHooks(); err != nil {
		return fmt.Errorf("remove managed Codex activity hooks: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"removed": true, "workspaceRoots": len(roots)})
}

func activateUpdatedExecutable(ctx context.Context, executable string, paths config.Paths) error {
	validationCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(validationCtx, executable, "version", "--json")
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("start updated executable: %w", err)
	}
	var info versionInfo
	if err = json.Unmarshal(output, &info); err != nil || info.Version == "" || info.ArtifactSHA256 == "" {
		return errors.New("updated executable returned invalid version identity")
	}
	return restartInstalledService(ctx, executable, paths)
}

func restartInstalledService(ctx context.Context, executable string, paths config.Paths) error {
	home, uid, err := currentAccount()
	if err != nil {
		return err
	}
	manager := servicemanager.Manager{Executable: executable, ConfigRoot: paths.Root, Home: home, UID: uid}
	status, err := manager.Status(ctx)
	if err != nil || !status.Installed {
		return nil
	}
	if err = manager.Install(ctx); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, callErr := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "health"})
		if callErr == nil && response.OK {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("updated service did not become healthy")
}

// enrolledState describes an already-enrolled profile. A zero deviceID means
// this profile has never enrolled and the caller should run first enrollment.
type enrolledState struct {
	deviceID   string
	apiBaseURL string
}

// enrolledDevice reports the device identity a profile has already enrolled, and
// refuses a repository that is already connected. The API origin is read from
// the enrolled configuration rather than the flag, because an additional Project
// has to be created on the same backend that issued the credential being reused.
func enrolledDevice(paths config.Paths, repository string) (enrolledState, error) {
	cfg, err := config.Load(paths)
	if err != nil {
		return enrolledState{}, err
	}
	if cfg.DeviceID == "" || cfg.APIBaseURL == "" {
		return enrolledState{}, nil
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return enrolledState{}, err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil {
		root = resolved
	}
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			return enrolledState{}, errors.New("this repository is already connected to a Project")
		}
	}
	return enrolledState{deviceID: cfg.DeviceID, apiBaseURL: cfg.APIBaseURL}, nil
}
