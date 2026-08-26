package main

import (
	"context"
	"crypto/rand"
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
	"io"
	"os"
	"os/signal"
	"syscall"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

type versionInfo struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuildTime     string `json:"buildTime"`
	SchemaMinimum int    `json:"schemaMinimum"`
	SchemaMaximum int    `json:"schemaMaximum"`
}

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 2 && args[0] == "version" && args[1] == "--json" {
		return json.NewEncoder(os.Stdout).Encode(versionInfo{version, commit, buildTime, 1, 1})
	}
	fs := flag.NewFlagSet("stickguy", flag.ContinueOnError)
	root := fs.String("config-root", "", "isolated per-user state root")
	apiBase := fs.String("api", "https://api.stickguy.dev", "Stickguy API origin")
	if e := fs.Parse(args); e != nil {
		return e
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("usage: stickguy [--config-root <dir>] create|join|dashboard|mcp|setup|service|workspace|intent|pause|resume|doctor|scan")
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
			return e
		}
		input, readErr := io.ReadAll(io.LimitReader(os.Stdin, agentactivity.MaxInputBytes+1))
		if readErr != nil {
			return nil
		}
		event, parseErr := agentactivity.Parse(*vendor, input)
		if parseErr != nil {
			return nil
		}
		response, callErr := daemon.Call(ctx, paths.Socket, daemon.Request{
			Method: "agent_event", AgentVendor: event.Vendor, AgentCWD: event.CWD,
			AgentWorkstreamID: event.WorkstreamID, AgentSessionAlias: event.SessionAlias,
			AgentEvent: event.Kind, AgentStatus: event.Status, AgentAction: event.Action,
			AgentTool: event.Tool, AgentType: event.AgentType, AgentSubagentAlias: event.SubagentAlias,
			AgentPaths:          event.CandidatePaths,
			AgentTranscriptPath: event.TranscriptPath, AgentVendorSessionID: event.VendorSessionID,
		})
		if callErr != nil || !response.OK {
			return nil
		}
		return nil
	case "setup":
		if len(rest) < 2 || !map[string]bool{"codex": true, "claude": true, "status": true, "remove": true}[rest[1]] {
			return errors.New("setup requires codex, claude, status, or remove")
		}
		setupFlags := flag.NewFlagSet("setup "+rest[1], flag.ContinueOnError)
		projectRoot := setupFlags.String("project-root", ".", "trusted coding-agent project root")
		agent := setupFlags.String("agent", "codex", "coding agent for status/remove: codex or claude")
		development := setupFlags.Bool("development", false, "explicitly install the local development MCP adapter")
		if e = setupFlags.Parse(rest[2:]); e != nil {
			return e
		}
		if (rest[1] == "codex" || rest[1] == "claude") && !*development {
			return errors.New("coding-agent setup is withheld: L5 real-client validation narrowed; use Git/manual coordination fallback")
		}
		executable, executableErr := os.Executable()
		if executableErr != nil {
			return executableErr
		}
		selected := rest[1]
		if selected == "status" || selected == "remove" {
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
			return errors.New("service requires run or status")
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
			return printCall(ctx, paths.Socket, daemon.Request{Method: "health"})
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
	}
	return errors.New("unsupported command")
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
