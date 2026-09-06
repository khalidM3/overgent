package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/cliui"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/daemon"
	"github.com/khalidM3/overgent/internal/hosted"
)

func TestSelectStatusWorkspaceUsesLongestContainingRoot(t *testing.T) {
	outer := filepath.Join(t.TempDir(), "repo")
	inner := filepath.Join(outer, "packages", "api")
	cfg := config.Config{Workspaces: []config.Workspace{
		{ID: "wsp_outer", ProjectID: "prj_outer", Root: outer},
		{ID: "wsp_inner", ProjectID: "prj_inner", Root: inner},
	}}
	workspace, err := selectStatusWorkspace(cfg, "", func() (string, error) { return filepath.Join(inner, "src"), nil })
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ID != "wsp_inner" {
		t.Fatalf("workspace = %q, want nested workspace", workspace.ID)
	}
}

func TestSelectStatusWorkspaceNeverGuessesAmbiguousRoot(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{Workspaces: []config.Workspace{
		{ID: "wsp_a", ProjectID: "prj_a", Root: root},
		{ID: "wsp_b", ProjectID: "prj_b", Root: root},
	}}
	_, err := selectStatusWorkspace(cfg, "", func() (string, error) { return root, nil })
	// The refusal must name the recovery rather than silently picking a side.
	if err == nil || !strings.Contains(err.Error(), "more than one Project") || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("error = %v", err)
	}
}

// A device with nothing registered is a first-run state, not a wrong-directory
// state: pointing it at `--project ID` would name an id that cannot exist.
func TestSelectStatusWorkspaceSendsAFirstRunDeviceToEnrollment(t *testing.T) {
	root := t.TempDir()
	_, err := selectStatusWorkspace(config.Config{}, "", func() (string, error) { return root, nil })
	if err == nil || !strings.Contains(err.Error(), "overgent init") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "--project") {
		t.Fatalf("first-run recovery offered a Project id that cannot exist: %v", err)
	}
}

func TestRunStatusJSONIsProjectAwareAndStable(t *testing.T) {
	state, repo := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths, err := config.Resolve(state)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Single("https://api.example.com", "dev_1", []config.Workspace{{ID: "wsp_1", ProjectID: "prj_1", Root: repo}})
	if err = config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cli := statusCLI{
		getwd: func() (string, error) { return filepath.Join(repo, "child"), nil },
		call: func(context.Context, string, daemon.Request) (daemon.Response, error) {
			return daemon.Response{OK: true, Data: map[string]any{
				"status":   "ok",
				"backends": []map[string]any{{"id": cfg.Backends[0].ID, "credential": "ok"}},
			}}, nil
		},
		bootstrap: func(context.Context, config.Backend) (hosted.Bootstrap, error) {
			return hosted.Bootstrap{Projects: []hosted.Project{{ID: "prj_1", Label: "Payments"}}}, nil
		},
		stdout: &stdout,
		stderr: &stderr,
	}
	if err = runStatusWithCLI(context.Background(), paths, []string{"--json"}, cli); err != nil {
		t.Fatal(err)
	}
	var got statusOutput
	if err = json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 1 || got.Project.ID != "prj_1" || got.Project.Label != "Payments" || got.Repository.Root != repo {
		t.Fatalf("status = %+v", got)
	}
	if got.Service.State != "running" || got.Sync.State != "available" {
		t.Fatalf("health = %+v %+v", got.Service, got.Sync)
	}
}

func TestRunStatusReportsOfflineWithoutInventingProjectActivity(t *testing.T) {
	state, repo := t.TempDir(), t.TempDir()
	paths, _ := config.Resolve(state)
	cfg := config.Single("http://127.0.0.1:43103", "dev_1", []config.Workspace{{ID: "wsp_1", ProjectID: "prj_1", Root: repo}})
	if err := config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cli := statusCLI{
		getwd: func() (string, error) { return repo, nil },
		call: func(context.Context, string, daemon.Request) (daemon.Response, error) {
			return daemon.Response{}, errors.New("offline")
		},
		bootstrap: func(context.Context, config.Backend) (hosted.Bootstrap, error) {
			return hosted.Bootstrap{}, errors.New("offline")
		},
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		ui:     cliui.Options{Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: 80},
	}
	if err := runStatusWithCLI(context.Background(), paths, nil, cli); err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	for _, want := range []string{"prj_1", "local Project", "Service     - stopped", "Sync        - unavailable"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output %q missing %q", text, want)
		}
	}
	if !strings.Contains(text, "NEEDS YOU") || !strings.Contains(text, "- unavailable") {
		t.Fatalf("output did not state needs-you coverage: %s", text)
	}
	// The offline case must never read as an all-clear.
	if strings.Contains(text, "Nothing needs you") {
		t.Fatalf("an offline status reported an all-clear: %s", text)
	}
	if !strings.Contains(text, "could not check everything") {
		t.Fatalf("offline status did not say the check was incomplete: %s", text)
	}
}

// The whole path, from config through both evidence sources to rendered text.
func TestRunStatusAnswersNeedsYouFromRoutedFindingsAndWaitingSessions(t *testing.T) {
	state, repo := t.TempDir(), t.TempDir()
	paths, _ := config.Resolve(state)
	cfg := config.Single("http://127.0.0.1:43103", "dev_1", []config.Workspace{{ID: "wsp_1", ProjectID: "prj_1", WorkstreamID: "wrk_me", Root: repo}})
	if err := config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cli := statusCLI{
		getwd: func() (string, error) { return repo, nil },
		call: func(_ context.Context, _ string, request daemon.Request) (daemon.Response, error) {
			if request.Method == "project_activity" {
				return daemon.Response{OK: true, Data: map[string]any{"sessions": []map[string]any{
					{"Vendor": "codex", "WorkstreamID": "wrk_me", "Status": "waiting"},
					{"Vendor": "claude", "WorkstreamID": "wrk_me", "Status": "active"},
				}}}, nil
			}
			return daemon.Response{OK: true, Data: map[string]any{}}, nil
		},
		bootstrap: func(context.Context, config.Backend) (hosted.Bootstrap, error) {
			return hosted.Bootstrap{}, errors.New("offline")
		},
		changes: func(context.Context, config.Backend, string) (hosted.ChangePage, error) {
			return hosted.ChangePage{Items: []map[string]any{
				{"id": "fnd_mine", "state": "open", "delivery": "next_turn", "severity": "high", "reason": "Two sessions are editing auth.go", "workstreamIds": []any{"wrk_me"}},
				{"id": "fnd_theirs", "state": "open", "delivery": "next_turn", "severity": "critical", "reason": "Someone else's collision", "workstreamIds": []any{"wrk_other"}},
			}}, nil
		},
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		ui:     cliui.Options{Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: 80},
	}
	if err := runStatusWithCLI(context.Background(), paths, nil, cli); err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	for _, want := range []string{"NEEDS YOU", "Two sessions are editing auth.go", "waiting on you", "ELSEWHERE", "1 other finding"} {
		if !strings.Contains(text, want) {
			t.Errorf("status missing %q:\n%s", want, text)
		}
	}
	// A finding on another member's workstream is Elsewhere, never Needs you.
	if strings.Contains(text[:strings.Index(text, "ELSEWHERE")], "Someone else's collision") {
		t.Errorf("another member's finding was routed to this viewer:\n%s", text)
	}
	if strings.Contains(text, "Nothing needs you") {
		t.Errorf("status claimed clear while work was routed:\n%s", text)
	}
}

func TestRunProjectsKeepsConfiguredProjectsWhenBackendIsOffline(t *testing.T) {
	state, repo := t.TempDir(), t.TempDir()
	paths, _ := config.Resolve(state)
	cfg := config.Single("https://api.example.com", "dev_1", []config.Workspace{{ID: "wsp_1", ProjectID: "prj_1", Root: repo}})
	if err := config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cli := statusCLI{
		bootstrap: func(context.Context, config.Backend) (hosted.Bootstrap, error) {
			return hosted.Bootstrap{}, errors.New("offline")
		},
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		ui:     cliui.Options{Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: 80},
	}
	if err := runProjectsWithCLI(context.Background(), paths, []string{"--json"}, cli); err != nil {
		t.Fatal(err)
	}
	var got projectsOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Projects) != 1 || got.Projects[0].ID != "prj_1" || got.Projects[0].Reachable {
		t.Fatalf("projects = %+v", got.Projects)
	}
}
