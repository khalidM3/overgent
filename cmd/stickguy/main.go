package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/stickguy/stickguy/internal/activation"
	"github.com/stickguy/stickguy/internal/app"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/credential"
	"github.com/stickguy/stickguy/internal/daemon"
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/stickguy/stickguy/internal/onboarding"
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
		return errors.New("usage: stickguy [--config-root <dir>] service run|status | workspace add | pause|resume | doctor | scan")
	}
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
		if e = wf.Parse(rest[2:]); e != nil {
			return e
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
