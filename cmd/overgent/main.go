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
	"github.com/khalidM3/overgent/internal/activation"
	"github.com/khalidM3/overgent/internal/adapterrepair"
	"github.com/khalidM3/overgent/internal/agentactivity"
	"github.com/khalidM3/overgent/internal/app"
	"github.com/khalidM3/overgent/internal/claudesetup"
	"github.com/khalidM3/overgent/internal/cliui"
	"github.com/khalidM3/overgent/internal/codexsetup"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/cursorsetup"
	"github.com/khalidM3/overgent/internal/daemon"
	"github.com/khalidM3/overgent/internal/hosted"
	coordinationmcp "github.com/khalidM3/overgent/internal/mcp"
	"github.com/khalidM3/overgent/internal/onboarding"
	servicemanager "github.com/khalidM3/overgent/internal/service"
	updateclient "github.com/khalidM3/overgent/internal/update"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
		renderCLIError(e, os.Stderr)
		os.Exit(1)
	}
}
func run(args []string) error {
	fs := flag.NewFlagSet("overgent", flag.ContinueOnError)
	root := fs.String("config-root", "", "isolated per-user state root")
	apiBase := fs.String("api", "https://api.overgent.com", "Overgent API origin")
	noColor := fs.Bool("no-color", false, "never emit ANSI color")
	noInput := fs.Bool("no-input", false, "never prompt; fail instead of asking")
	if e := fs.Parse(args); e != nil {
		return e
	}
	setPresentation(*noColor, *noInput)
	rest := fs.Args()
	// Help, completion, and version answer from the static command catalogue.
	// They resolve before configuration so they stay correct on a brand-new,
	// offline, or partially broken installation (cli-experience.md section 11).
	if len(rest) > 0 {
		switch rest[0] {
		case "help":
			return runHelp(rest[1:], os.Stdout)
		case "completion":
			return runCompletion(rest[1:], os.Stdout)
		case "version":
			return runVersion(rest[1:], os.Stdout, os.Stderr)
		}
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
	if len(rest) == 0 {
		return runContextualRoot(ctx, paths, os.Stdout)
	}
	switch rest[0] {
	case "init":
		return runInit(rest[1:], *root, *apiBase, flagProvided(fs, "api"), os.Stdin, os.Stdout, os.Stderr, run)
	case "projects":
		return runProjects(ctx, paths, rest[1:])
	case "status":
		for index, arg := range rest[1:] {
			if arg == "--watch" {
				watchArgs := append([]string(nil), rest[1:index+1]...)
				watchArgs = append(watchArgs, rest[index+2:]...)
				return runWatch(ctx, paths, watchArgs, os.Stdin, os.Stdout, os.Stderr)
			}
		}
		return runStatus(ctx, paths, rest[1:])
	case "watch":
		return runWatch(ctx, paths, rest[1:], os.Stdin, os.Stdout, os.Stderr)
	case "privacy":
		return runPrivacy(ctx, paths, rest[1:], os.Stdout, os.Stderr)
	case "create":
		createFlags := flag.NewFlagSet("create", flag.ContinueOnError)
		label := createFlags.String("label", "", "Project label")
		deviceLabel := createFlags.String("device-label", "", "device label shared with Project members")
		repository := createFlags.String("root", ".", "Git repository root")
		local := createFlags.Bool("local", false, "create the Project on this Mac's bundled backend; nothing leaves this computer")
		jsonOutput := createFlags.Bool("json", false, "emit stable JSON")
		if e = createFlags.Parse(rest[1:]); e != nil {
			return e
		}
		// --local names where the Project lives, and so does --api. Accepting
		// both would mean silently ignoring one of the two things the member
		// said about where their coordination data goes.
		if *local && flagProvided(fs, "api") {
			return errors.New("create accepts --local or --api, not both")
		}
		if *local {
			endpoint, localErr := backendEnsure(ctx, paths)
			if localErr != nil {
				return localErr
			}
			*apiBase = endpoint.SiteOrigin
		} else if validated, originErr := onboarding.ValidateAPIOrigin(*apiBase); originErr != nil {
			return originErr
		} else {
			*apiBase = validated
		}
		// The backend named here is the Project's, not the profile's: a local
		// Project and a team Project sit side by side (ADR-074). The flow
		// reuses this profile's device identity for that backend when it has
		// one, and mints a new one when this is a server it has never used.
		cfg, configErr := config.Load(paths)
		if configErr != nil {
			return configErr
		}
		if repositoryErr := repositoryAvailable(cfg, *repository); repositoryErr != nil {
			return repositoryErr
		}
		service := onboarding.New(cfg.BackendTarget(*apiBase))
		result, createErr := service.CreateOnNewBackend(ctx, onboarding.Options{
			ConfigRoot: *root, RepositoryRoot: *repository, ProjectLabel: *label,
			DeviceLabel: *deviceLabel, AppVersion: "overgent/" + version, SkipInvite: *local,
		})
		if createErr != nil {
			return createErr
		}
		if cliui.IsTerminal(os.Stdout) && !*jsonOutput {
			mode := "team"
			if *local {
				mode = "local"
			}
			if _, printErr := fmt.Fprintf(os.Stdout, "Project created · %s\n\nRepository registered. Run `overgent status` to check coordination.\n", mode); printErr != nil {
				return printErr
			}
			if result.JoinCode != "" {
				_, printErr := fmt.Fprintf(os.Stdout, "Invite: %s\n", result.JoinCode)
				return printErr
			}
			return nil
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			SchemaVersion int    `json:"schemaVersion"`
			ProjectID     string `json:"projectId"`
			DeviceID      string `json:"deviceId"`
			WorkspaceID   string `json:"workspaceId"`
			WorkstreamID  string `json:"workstreamId"`
			JoinCode      string `json:"joinCode"`
		}{cliOutputSchemaVersion, result.ProjectID, result.DeviceID, result.WorkspaceID, result.WorkstreamID, result.JoinCode})
	case "join":
		joinFlags := flag.NewFlagSet("join", flag.ContinueOnError)
		deviceLabel := joinFlags.String("device-label", "", "device label shared with Project members")
		repository := joinFlags.String("root", ".", "Git repository root")
		jsonOutput := joinFlags.Bool("json", false, "emit stable JSON")
		if e = joinFlags.Parse(rest[1:]); e != nil {
			return e
		}
		if joinFlags.NArg() != 1 {
			return errors.New("join requires one invite link or code")
		}
		// An https invite link names the server the Project lives on, which is
		// the whole point of pasting a link rather than a bare code: a member
		// on a purely local profile can join a friend's team Project without
		// being asked which server it is on. A bare code or a deep link names
		// no server, so --api (or its default) decides.
		_, inviteOrigin, inviteErr := onboarding.ParseInviteCode(joinFlags.Arg(0))
		if inviteErr != nil {
			return inviteErr
		}
		if inviteOrigin != "" {
			if flagProvided(fs, "api") && strings.TrimRight(*apiBase, "/") != inviteOrigin {
				return errors.New("this invite link names a different server than --api; pass the bare invite code to use --api")
			}
			*apiBase = inviteOrigin
		}
		validatedAPI, originErr := onboarding.ValidateAPIOrigin(*apiBase)
		if originErr != nil {
			return originErr
		}
		*apiBase = validatedAPI
		cfg, configErr := config.Load(paths)
		if configErr != nil {
			return configErr
		}
		if repositoryErr := repositoryAvailable(cfg, *repository); repositoryErr != nil {
			return repositoryErr
		}
		options := onboarding.Options{ConfigRoot: *root, RepositoryRoot: *repository, DeviceLabel: *deviceLabel, AppVersion: "overgent/" + version}
		result, joinErr := onboarding.New(cfg.BackendTarget(*apiBase)).JoinOnNewBackend(ctx, options, joinFlags.Arg(0))
		if joinErr != nil {
			return joinErr
		}
		if cliui.IsTerminal(os.Stdout) && !*jsonOutput {
			_, printErr := fmt.Fprintln(os.Stdout, "Project joined.\n\nRepository registered. Run `overgent status` to check coordination.")
			return printErr
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			SchemaVersion int    `json:"schemaVersion"`
			ProjectID     string `json:"projectId"`
			DeviceID      string `json:"deviceId"`
			WorkspaceID   string `json:"workspaceId"`
			WorkstreamID  string `json:"workstreamId"`
		}{cliOutputSchemaVersion, result.ProjectID, result.DeviceID, result.WorkspaceID, result.WorkstreamID})
	case "reset":
		// Recovery for a device whose credential a backend no longer accepts -
		// revoked by an owner, or unknown to the deployment. It is scoped to
		// one backend, because a profile now holds several and a revoked team
		// Project says nothing about the local Project beside it. --all is the
		// whole-profile form.
		resetFlags := flag.NewFlagSet("reset", flag.ContinueOnError)
		force := resetFlags.Bool("force", false, "clear the local enrollment even if the credential could not be verified")
		backendID := resetFlags.String("backend", "", "backend id to reset; see overgent backend list")
		all := resetFlags.Bool("all", false, "reset every backend on this profile")
		if e = resetFlags.Parse(rest[1:]); e != nil {
			return e
		}
		cfg, configErr := config.Load(paths)
		if configErr != nil {
			return configErr
		}
		if *all && *backendID != "" {
			return errors.New("reset accepts --backend or --all, not both")
		}
		var outcomes []onboarding.ResetOutcome
		var resetErr error
		switch {
		case *all:
			outcomes, resetErr = onboarding.ResetAll(ctx, *root, *force)
		default:
			backend, resolveErr := resolveBackend(cfg, *backendID)
			if resolveErr != nil {
				return resolveErr
			}
			var outcome onboarding.ResetOutcome
			outcome, resetErr = onboarding.New(backend).Reset(ctx, *root, *force)
			outcomes = []onboarding.ResetOutcome{outcome}
		}
		if resetErr != nil {
			return resetErr
		}
		reported := make([]resetReport, 0, len(outcomes))
		for _, outcome := range outcomes {
			reported = append(reported, resetReport{string(outcome.Status), outcome.BackendID, outcome.APIBaseURL, outcome.DeviceID, outcome.ClearedWorkspaces, outcome.CredentialDeleted})
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"backends": reported})
	case "open", "dashboard":
		// One destination command. `dashboard` stays as a compatible alias that
		// routes here rather than to a second implementation (ADR-080).
		openFlags := flag.NewFlagSet(rest[0], flag.ContinueOnError)
		projectID := openFlags.String("project", "", "Project id")
		// The desktop app embeds the same dashboard build the browser serves, and
		// it keeps the activation handoff inside its own webview instead of
		// stranding a tab (ADR-057). So the app is the default destination and
		// the browser is the fallback; --web forces the browser for SSH, headless
		// use, or when the app is not the surface a member wants.
		web := openFlags.Bool("web", false, "open the browser dashboard instead of the app")
		if e = openFlags.Parse(rest[1:]); e != nil {
			return e
		}
		// `dashboard` opened a browser before `open` existed. Keeping that exact
		// behavior is what makes the alias compatible rather than merely
		// accepted: a script that calls it still gets the surface it expects.
		if rest[0] == "dashboard" {
			*web = true
		}
		cfg, loadErr := config.Load(paths)
		if loadErr != nil {
			return loadErr
		}
		if *projectID == "" {
			selected, selectErr := selectDashboardProject(ctx, cfg, os.Stdin, os.Stdout)
			if selectErr != nil {
				return selectErr
			}
			*projectID = selected
		}
		if !*web {
			if appErr := activation.OpenApp(ctx, *projectID); appErr == nil {
				return nil
			}
			// The app is not installed or could not be reached. Falling through
			// to the browser is the recovery, and it is announced rather than
			// silent: a member who expected the app should know why a tab
			// opened instead.
			fmt.Fprintln(os.Stderr, "Overgent app unavailable; opening the browser dashboard instead.")
		}
		// The dashboard opens against the backend this Project lives on, not a
		// profile-wide origin: two Projects on one Mac can be served by two
		// different servers.
		backend, bound := cfg.BackendForProject(*projectID)
		if !bound || backend.DeviceID == "" {
			return errors.New("dashboard requires an enrolled Project")
		}
		token, credentialErr := credential.Get(ctx, backend.DeviceID)
		if credentialErr != nil {
			return credentialErr
		}
		client, clientErr := hosted.New(backend.APIBaseURL, token)
		if clientErr != nil {
			return clientErr
		}
		ticket, ticketErr := client.CreateDashboardTicket(ctx, *projectID)
		if ticketErr != nil {
			return ticketErr
		}
		return activation.Open(ctx, backend.APIBaseURL, ticket.Ticket)
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
		if len(rest) < 2 || !map[string]bool{"codex": true, "claude": true, "cursor": true, "status": true, "remove": true, "remove-all": true, "reconnect": true, "repair": true}[rest[1]] {
			return errors.New("setup requires codex, claude, cursor, status, reconnect, repair, remove, or remove-all")
		}
		// repair adopts bindings an earlier Overgent left behind across every
		// registered repository, which is what the desktop app does on launch.
		// It exists here for headless recovery and support, and never creates a
		// binding: an agent that was never connected stays unconnected.
		if rest[1] == "repair" {
			if len(rest) != 2 {
				return errors.New("setup repair accepts no arguments")
			}
			executable, executableErr := os.Executable()
			if executableErr != nil {
				return executableErr
			}
			cfg, configErr := config.Load(paths)
			if configErr != nil {
				return configErr
			}
			roots := make([]string, 0, len(cfg.Workspaces))
			for _, workspace := range cfg.Workspaces {
				roots = append(roots, workspace.Root)
			}
			adopted := []adapterrepair.Outcome{}
			for _, outcome := range adapterrepair.Run(*root, executable, roots) {
				if outcome.Err != nil {
					return fmt.Errorf("repair %s in %s: %w", outcome.Vendor, outcome.Root, outcome.Err)
				}
				adopted = append(adopted, outcome)
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"adopted": adopted})
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
			return app.Run(ctx, *root, app.NewHostedSenders())
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
	case "backend":
		return runBackend(ctx, paths, rest[1:])
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
			if len(cfg.Backends) == 0 || len(cfg.Workspaces) == 0 {
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
			// A second root joins a Project that already exists, so it takes
			// that Project's backend rather than the profile's: after ADR-074
			// there is no such thing as the profile's backend.
			backend, bound := cfg.BackendForProject(*project)
			if !bound {
				return errors.New("selected Project has no backend on this profile")
			}
			*member, *device, *apiBase = source.MemberID, backend.DeviceID, backend.APIBaseURL
			request := daemon.Request{Method: "add_development_workspace", WorkspaceID: *id, ProjectID: *project, WorkstreamID: *workstream, MemberID: *member, SessionID: *session, Root: *repo, APIBaseURL: *apiBase, DeviceID: *device}
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
	case "ai":
		return runAI(ctx, paths, rest[1:], os.Stdin, os.Stdout)
	case "doctor":
		if err := printCall(ctx, paths.Socket, daemon.Request{Method: "doctor"}); err != nil {
			return err
		}
		return printAIDoctor(ctx, paths, os.Stdout)
	case "scan":
		return printCall(ctx, paths.Socket, daemon.Request{Method: rest[0]})
	case "diagnostics":
		return writeDiagnostics(ctx, paths)
	case "update":
		updateFlags := flag.NewFlagSet("update", flag.ContinueOnError)
		manifestURL := updateFlags.String("manifest", "https://releases.overgent.com/current/update-manifest.json", "signed update metadata URL")
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
	return unknownCommandError(rest[0])
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
type resetReport struct {
	Credential        string `json:"credential"`
	BackendID         string `json:"backendId,omitempty"`
	APIBaseURL        string `json:"apiBaseUrl,omitempty"`
	DeviceID          string `json:"deviceId,omitempty"`
	ClearedWorkspaces int    `json:"clearedWorkspaces"`
	CredentialDeleted bool   `json:"credentialDeleted"`
}

// repositoryAvailable refuses a repository that is already connected. It is
// checked before anything reaches the network so a mistake cannot spend an
// invite or create a Project nothing will be registered against.
func repositoryAvailable(cfg config.Config, repository string) error {
	root, err := filepath.Abs(repository)
	if err != nil {
		return err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil {
		root = resolved
	}
	for _, workspace := range cfg.Workspaces {
		if workspace.Root == root {
			return errors.New("this repository is already connected to a Project")
		}
	}
	return nil
}

// resolveBackend picks the backend a command names. A profile with exactly one
// backend does not have to name it; a profile with several must, because
// guessing which enrollment to erase is not a guess worth making.
func resolveBackend(cfg config.Config, backendID string) (config.Backend, error) {
	if backendID != "" {
		backend, known := cfg.BackendByID(backendID)
		if !known {
			return config.Backend{}, fmt.Errorf("no backend %s on this profile", backendID)
		}
		return backend, nil
	}
	switch len(cfg.Backends) {
	case 0:
		return config.Backend{}, errors.New("this profile has no enrolled backend")
	case 1:
		return cfg.Backends[0], nil
	default:
		return config.Backend{}, errors.New("this profile has more than one backend; pass --backend <id> or --all")
	}
}

// flagProvided reports whether the member actually typed this flag. Every flag
// here has a default, so comparing against the default cannot tell "not given"
// from "given the same value".
func flagProvided(set *flag.FlagSet, name string) bool {
	provided := false
	set.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}
