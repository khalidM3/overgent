package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/khalidM3/overgent/internal/cliui"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/daemon"
	"github.com/khalidM3/overgent/internal/hosted"
)

const cliOutputSchemaVersion = 1

type projectListItem struct {
	ID         string   `json:"id"`
	Label      string   `json:"label,omitempty"`
	Mode       string   `json:"mode"`
	Backend    string   `json:"backend"`
	Workspaces []string `json:"workspaces"`
	Reachable  bool     `json:"reachable"`
}

type projectsOutput struct {
	SchemaVersion int               `json:"schemaVersion"`
	Projects      []projectListItem `json:"projects"`
}

type statusOutput struct {
	SchemaVersion int           `json:"schemaVersion"`
	Project       statusProject `json:"project"`
	Repository    statusRepo    `json:"repository"`
	Service       statusService `json:"service"`
	Sync          statusSync    `json:"sync"`
	NeedsYou      attention     `json:"needsYou"`
}

type statusProject struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Mode  string `json:"mode"`
}

type statusRepo struct {
	WorkspaceID string `json:"workspaceId"`
	Root        string `json:"root"`
}

type statusService struct {
	State string `json:"state"`
}

type statusSync struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

type statusCLI struct {
	getwd     func() (string, error)
	call      daemonCaller
	bootstrap func(context.Context, config.Backend) (hosted.Bootstrap, error)
	changes   func(context.Context, config.Backend, string) (hosted.ChangePage, error)
	stdout    io.Writer
	stderr    io.Writer
	// ui carries the color, Unicode, and width decision. Tests inject it so
	// rendered output never depends on the locale or terminal running the
	// suite (cli-experience.md section 12).
	ui cliui.Options
}

// terminal binds the injected presentation options to this command's streams.
func (cli statusCLI) terminal() cliui.Terminal {
	options := cli.ui
	options.Out, options.Err = cli.stdout, cli.stderr
	return cliui.NewTerminal(options)
}

func defaultStatusCLI() statusCLI {
	return statusCLI{
		ui:    presentationOptions(),
		getwd: os.Getwd,
		call:  daemon.Call,
		bootstrap: func(ctx context.Context, backend config.Backend) (hosted.Bootstrap, error) {
			token, err := credential.Get(ctx, backend.DeviceID)
			if err != nil {
				return hosted.Bootstrap{}, err
			}
			client, err := hosted.New(backend.APIBaseURL, token)
			if err != nil {
				return hosted.Bootstrap{}, err
			}
			return client.Bootstrap(ctx)
		},
		changes: func(ctx context.Context, backend config.Backend, projectID string) (hosted.ChangePage, error) {
			client, err := backendClient(ctx, backend)
			if err != nil {
				return hosted.ChangePage{}, err
			}
			return client.ProjectChanges(ctx, projectID)
		},
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

// backendClient builds an authenticated client for one Project's backend. Two
// Projects on one Mac can live on two servers, so the credential is always
// looked up per backend rather than per profile.
func backendClient(ctx context.Context, backend config.Backend) (*hosted.Client, error) {
	token, err := credential.Get(ctx, backend.DeviceID)
	if err != nil {
		return nil, err
	}
	return hosted.New(backend.APIBaseURL, token)
}

// viewerWorkstreams collects every workstream this member owns for a Project on
// this device. "Needs you" is scoped to the member, not to one checkout, so a
// finding that lands on a second worktree of the same Project still counts.
func viewerWorkstreams(cfg config.Config, projectID string) map[string]bool {
	mine := map[string]bool{}
	for _, workspace := range cfg.Workspaces {
		if workspace.ProjectID == projectID && workspace.WorkstreamID != "" {
			mine[workspace.WorkstreamID] = true
		}
	}
	return mine
}

// runProjects is the read-only Projects entry point used by the root command.
func runProjects(ctx context.Context, paths config.Paths, args []string) error {
	return runProjectsWithCLI(ctx, paths, args, defaultStatusCLI())
}

func runProjectsWithCLI(ctx context.Context, paths config.Paths, args []string, cli statusCLI) error {
	flags := flag.NewFlagSet("projects", flag.ContinueOnError)
	flags.SetOutput(cli.stderr)
	jsonOutput := flags.Bool("json", false, "emit stable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("projects accepts no positional arguments")
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}

	labels, reachable := projectLabels(ctx, cfg, cli.bootstrap)
	items := make([]projectListItem, 0, len(cfg.Projects))
	for _, project := range cfg.Projects {
		backend, bound := cfg.BackendForProject(project.ID)
		mode, origin := "unknown", ""
		if bound {
			mode, origin = backend.Kind, backend.APIBaseURL
		}
		var roots []string
		for _, workspace := range cfg.Workspaces {
			if workspace.ProjectID == project.ID {
				roots = append(roots, workspace.Root)
			}
		}
		sort.Strings(roots)
		items = append(items, projectListItem{ID: project.ID, Label: labels[project.ID], Mode: mode, Backend: origin, Workspaces: roots, Reachable: reachable[project.ID]})
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i].Label, items[j].Label
		if left == "" {
			left = items[i].ID
		}
		if right == "" {
			right = items[j].ID
		}
		return left < right
	})
	if *jsonOutput {
		return json.NewEncoder(cli.stdout).Encode(projectsOutput{SchemaVersion: cliOutputSchemaVersion, Projects: items})
	}
	if len(items) == 0 {
		_, err = fmt.Fprintln(cli.stdout, "No Projects are registered on this device.\n\nNext: run `overgent init` inside a Git repository.")
		return err
	}
	terminal := cli.terminal()
	_, _ = fmt.Fprintf(cli.stdout, "%s  %s\n%s\n\n", terminal.Style(cliui.StyleBold, "PROJECTS"), terminal.Style(cliui.StyleMuted, fmt.Sprintf("%d configured", len(items))), terminal.Style(cliui.StyleMuted, "Registered on this device"))
	for _, item := range items {
		name := item.Label
		if name == "" {
			name = item.ID
		}
		connectivity := terminal.Style(cliui.StyleMuted, "unavailable")
		if item.Reachable {
			connectivity = terminal.Style(cliui.StyleLive, terminal.Symbol("●", "*")) + " reachable"
		}
		_, _ = fmt.Fprintf(cli.stdout, "%s\n  %s  %s\n", terminal.Style(cliui.StyleBold, name), terminal.Style(cliui.StyleMuted, item.Mode), connectivity)
		for _, root := range item.Workspaces {
			_, _ = fmt.Fprintf(cli.stdout, "  %s\n", terminal.Style(cliui.StyleMuted, root))
		}
		_, _ = fmt.Fprintln(cli.stdout)
	}
	return nil
}

// runStatus is the read-only, current-workspace status entry point used by the
// root command. It intentionally reports only facts available from config and
// the daemon health API; sessions and findings are not invented here.
func runStatus(ctx context.Context, paths config.Paths, args []string) error {
	return runStatusWithCLI(ctx, paths, args, defaultStatusCLI())
}

func runStatusWithCLI(ctx context.Context, paths config.Paths, args []string, cli statusCLI) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(cli.stderr)
	jsonOutput := flags.Bool("json", false, "emit stable JSON")
	projectID := flags.String("project", "", "select a Project by id")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("status accepts no positional arguments")
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	workspace, err := selectStatusWorkspace(cfg, *projectID, cli.getwd)
	if err != nil {
		return err
	}
	backend, bound := cfg.BackendForWorkspace(workspace)
	mode := "unknown"
	if bound {
		mode = backend.Kind
	}

	label := ""
	if bound {
		lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		bootstrap, lookupErr := cli.bootstrap(lookupCtx, backend)
		cancel()
		if lookupErr == nil {
			for _, project := range bootstrap.Projects {
				if project.ID == workspace.ProjectID {
					label = project.Label
					break
				}
			}
		}
	}
	out := statusOutput{
		SchemaVersion: cliOutputSchemaVersion,
		Project:       statusProject{ID: workspace.ProjectID, Label: label, Mode: mode},
		Repository:    statusRepo{WorkspaceID: workspace.ID, Root: workspace.Root},
		Service:       statusService{State: "stopped"},
		Sync:          statusSync{State: "unavailable", Reason: "local service is not running"},
	}
	healthCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	response, callErr := cli.call(healthCtx, paths.Socket, daemon.Request{Method: "health"})
	cancel()
	if callErr == nil && response.OK {
		out.Service.State = "running"
		out.Sync = syncFromHealth(response.Data, backend, len(cfg.Backends))
	} else if callErr == nil {
		out.Service.State = "degraded"
		out.Sync = statusSync{State: "unavailable", Reason: "local service health check failed"}
	}

	// Both evidence sources are optional and independently degradable. Whether
	// each answered is passed through, so an unreachable one becomes a named
	// gap instead of an absence of work (cli-experience.md section 6).
	var sessions []watchedSession
	sessionCtx, cancelSessions := context.WithTimeout(ctx, 1200*time.Millisecond)
	activity, activityErr := cli.call(sessionCtx, paths.Socket, daemon.Request{Method: "project_activity", WorkspaceID: workspace.ID})
	cancelSessions()
	sessionsOK := activityErr == nil && activity.OK
	if sessionsOK {
		sessions = decodeWatchedSessions(activity.Data)
	}
	var findings []map[string]any
	findingsOK := false
	if bound && cli.changes != nil {
		findingCtx, cancelFindings := context.WithTimeout(ctx, 2*time.Second)
		page, changeErr := cli.changes(findingCtx, backend, workspace.ProjectID)
		cancelFindings()
		if changeErr == nil {
			findings, findingsOK = page.Items, true
		}
	}
	out.NeedsYou = evaluateAttention(findings, findingsOK, sessions, sessionsOK, viewerWorkstreams(cfg, workspace.ProjectID))

	if *jsonOutput {
		return json.NewEncoder(cli.stdout).Encode(out)
	}
	terminal := cli.terminal()
	name := out.Project.Label
	if name == "" {
		name = out.Project.ID
	}
	modeLabel := terminal.Style(cliui.StyleMuted, out.Project.Mode+" Project")
	_, _ = fmt.Fprintf(cli.stdout, "%s\n%s\n\n", terminal.Style(cliui.StyleBold, name), modeLabel)
	stateStyle := func(state string) string {
		switch state {
		case "running", "available":
			return terminal.Style(cliui.StyleLive, terminal.Symbol("●", "*")+" "+state)
		case "degraded":
			return terminal.Style(cliui.StyleAlert, terminal.Symbol("!", "!")+" "+state)
		default:
			return terminal.Style(cliui.StyleMuted, terminal.Symbol("○", "-")+" "+state)
		}
	}
	syncValue := stateStyle(out.Sync.State)
	if out.Sync.Reason != "" {
		syncValue += terminal.Style(cliui.StyleMuted, " — "+out.Sync.Reason)
	}
	if err = cliui.WriteFields(cli.stdout, terminal.Width(), []cliui.Field{{Label: "Service", Value: stateStyle(out.Service.State)}, {Label: "Repository", Value: out.Repository.Root}, {Label: "Sync", Value: syncValue}}); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cli.stdout)
	if err = writeAttention(cli.stdout, terminal, out.NeedsYou); err != nil {
		return err
	}
	return writeElsewhereCount(cli.stdout, terminal, out.NeedsYou.Elsewhere)
}

func projectLabels(ctx context.Context, cfg config.Config, fetch func(context.Context, config.Backend) (hosted.Bootstrap, error)) (map[string]string, map[string]bool) {
	labels, reachable := map[string]string{}, map[string]bool{}
	for _, backend := range cfg.Backends {
		lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		bootstrap, err := fetch(lookupCtx, backend)
		cancel()
		if err != nil {
			continue
		}
		for _, project := range bootstrap.Projects {
			labels[project.ID], reachable[project.ID] = project.Label, true
		}
	}
	return labels, reachable
}

func selectStatusWorkspace(cfg config.Config, projectID string, getwd func() (string, error)) (config.Workspace, error) {
	if projectID != "" {
		var matches []config.Workspace
		for _, workspace := range cfg.Workspaces {
			if workspace.ProjectID == projectID {
				matches = append(matches, workspace)
			}
		}
		if len(matches) == 0 {
			return config.Workspace{}, fmt.Errorf("Project %q has no registered checkout on this device.\n\nNext: run `overgent projects` to see what is registered", projectID)
		}
		if len(matches) > 1 {
			return config.Workspace{}, fmt.Errorf("Project %q has several registered checkouts on this device.\n\nNext: run the command from inside the checkout you mean", projectID)
		}
		return matches[0], nil
	}
	cwd, err := getwd()
	if err != nil {
		return config.Workspace{}, fmt.Errorf("resolve current directory: %w", err)
	}
	cwd, err = canonicalPath(cwd)
	if err != nil {
		return config.Workspace{}, err
	}
	bestLength := -1
	var matches []config.Workspace
	for _, workspace := range cfg.Workspaces {
		root, rootErr := canonicalPath(workspace.Root)
		if rootErr != nil || !pathContains(root, cwd) {
			continue
		}
		length := len(root)
		if length > bestLength {
			bestLength, matches = length, []config.Workspace{workspace}
		} else if length == bestLength {
			matches = append(matches, workspace)
		}
	}
	if len(matches) == 0 {
		if len(cfg.Workspaces) == 0 {
			return config.Workspace{}, errors.New("no Project is registered on this device.\n\nNext: run `overgent init` inside a Git repository")
		}
		return config.Workspace{}, errors.New("this directory is not registered with a Project.\n\nNext: run `overgent projects` to list them, then re-run with `--project ID`, or change into a registered repository")
	}
	project := matches[0].ProjectID
	for _, match := range matches[1:] {
		if match.ProjectID != project {
			return config.Workspace{}, errors.New("this directory is registered with more than one Project.\n\nNext: choose one with `--project ID`; run `overgent projects` to list them")
		}
	}
	return matches[0], nil
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repository path: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}

func pathContains(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func syncFromHealth(data any, backend config.Backend, backendCount int) statusSync {
	encoded, err := json.Marshal(data)
	if err != nil {
		return statusSync{State: "degraded", Reason: "service returned unreadable health"}
	}
	var health struct {
		LastPublishError string                            `json:"lastPublishError"`
		Backends         []struct{ ID, Credential string } `json:"backends"`
	}
	if json.Unmarshal(encoded, &health) != nil {
		return statusSync{State: "degraded", Reason: "service returned unreadable health"}
	}
	for _, item := range health.Backends {
		if item.ID != backend.ID {
			continue
		}
		switch item.Credential {
		case "ok":
			if health.LastPublishError != "" && backendCount == 1 {
				return statusSync{State: "degraded", Reason: health.LastPublishError}
			}
			return statusSync{State: "available"}
		case "revoked":
			return statusSync{State: "degraded", Reason: "device access was revoked"}
		case "unknown":
			return statusSync{State: "degraded", Reason: "device credential is unavailable"}
		default:
			return statusSync{State: "degraded", Reason: "backend credential has not been verified"}
		}
	}
	return statusSync{State: "degraded", Reason: "backend health is unavailable"}
}
