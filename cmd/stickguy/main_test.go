package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVersionEnvelope(t *testing.T) {
	b, err := json.Marshal(versionInfo{Version: "dev", Commit: "abc", BuildTime: "now", SchemaMinimum: 1, SchemaMaximum: 1})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["schemaMinimum"] != float64(1) || got["schemaMaximum"] != float64(1) {
		t.Fatalf("unexpected protocol range: %s", b)
	}
}

func TestAgentHookEmitsExactVendorJSONOnlyWithPendingContext(t *testing.T) {
	for _, vendor := range []string{"claude", "codex"} {
		for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
			root := t.TempDir()
			input := []byte(`{"session_id":"fixture-session","cwd":"` + root + `","hook_event_name":"` + event + `","prompt":"continue"}`)
			var output bytes.Buffer
			calls := 0
			call := func(_ context.Context, _ string, request daemon.Request) (daemon.Response, error) {
				calls++
				if request.Method == "agent_event" {
					return daemon.Response{OK: true}, nil
				}
				return daemon.Response{OK: true, Data: map[string]any{"additionalContext": "Coordination update: re-read backend.Refresh."}}, nil
			}
			if err := runAgentHook(context.Background(), "unused", vendor, bytes.NewReader(input), &output, call); err != nil {
				t.Fatal(err)
			}
			want := `{"hookSpecificOutput":{"hookEventName":"` + event + `","additionalContext":"Coordination update: re-read backend.Refresh."}}` + "\n"
			if output.String() != want || calls != 2 {
				t.Fatalf("%s %s output=%q calls=%d", vendor, event, output.String(), calls)
			}
		}
	}
}

func TestAgentHookPreservesEmptyObservationOutput(t *testing.T) {
	root := t.TempDir()
	input := []byte(`{"session_id":"fixture-session","cwd":"` + root + `","hook_event_name":"UserPromptSubmit","prompt":"continue"}`)
	var output bytes.Buffer
	call := func(_ context.Context, _ string, request daemon.Request) (daemon.Response, error) {
		if request.Method == "agent_event" {
			return daemon.Response{OK: true}, nil
		}
		return daemon.Response{OK: true, Data: map[string]any{}}, nil
	}
	if err := runAgentHook(context.Background(), "unused", "claude", bytes.NewReader(input), &output, call); err != nil || output.Len() != 0 {
		t.Fatalf("err=%v output=%q", err, output.String())
	}
}

func TestAgentHookLoopbackDeliversOnceAndRevisedItemAgain(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	socket := shortTestSocket(t)
	revision := 1
	delivered := map[int]bool{}
	var deliveryMu sync.Mutex
	go func() {
		_ = daemon.Serve(ctx, socket, func(_ context.Context, request daemon.Request) daemon.Response {
			if request.Method == "agent_event" {
				return daemon.Response{OK: true}
			}
			deliveryMu.Lock()
			defer deliveryMu.Unlock()
			if request.Method != "agent_injection" || delivered[revision] {
				return daemon.Response{OK: true, Data: map[string]any{}}
			}
			delivered[revision] = true
			return daemon.Response{OK: true, Data: map[string]any{"additionalContext": "Coordination update: backend.Refresh revision changed."}}
		})
	}()
	waitForSocket(t, socket)
	root := t.TempDir()
	input := []byte(`{"session_id":"fixture-session","cwd":"` + root + `","hook_event_name":"UserPromptSubmit","prompt":"continue"}`)
	invoke := func() string {
		var output bytes.Buffer
		if err := runAgentHook(context.Background(), socket, "claude", bytes.NewReader(input), &output, daemon.Call); err != nil {
			t.Fatal(err)
		}
		return output.String()
	}
	if first := invoke(); !strings.Contains(first, "backend.Refresh") {
		t.Fatalf("first output=%q", first)
	}
	if second := invoke(); second != "" {
		t.Fatalf("same revision output=%q", second)
	}
	deliveryMu.Lock()
	revision = 2
	deliveryMu.Unlock()
	if third := invoke(); !strings.Contains(third, "backend.Refresh") {
		t.Fatalf("revised output=%q", third)
	}
}

func TestAgentHookSlowServiceFailsOpenWithoutOutput(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	serviceContext, cancelService := context.WithCancel(context.Background())
	defer cancelService()
	socket := shortTestSocket(t)
	go func() {
		_ = daemon.Serve(serviceContext, socket, func(context.Context, daemon.Request) daemon.Response {
			time.Sleep(500 * time.Millisecond)
			return daemon.Response{OK: true}
		})
	}()
	waitForSocket(t, socket)
	root := t.TempDir()
	input := []byte(`{"session_id":"fixture-session","cwd":"` + root + `","hook_event_name":"UserPromptSubmit","prompt":"continue"}`)
	var output bytes.Buffer
	hookContext, cancelHook := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancelHook()
	started := time.Now()
	if err := runAgentHook(hookContext, socket, "codex", bytes.NewReader(input), &output, daemon.Call); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 || time.Since(started) > 400*time.Millisecond {
		t.Fatalf("output=%q elapsed=%s", output.String(), time.Since(started))
	}
}

func waitForSocket(t *testing.T, socket string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("socket %s was not ready", socket)
}

func shortTestSocket(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/private/tmp", "sg-hook-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return filepath.Join(directory, "service.sock")
}

// isolateCodex points Codex discovery and Codex state at throwaway locations.
// Codex hooks install at the user layer, and trust repair spawns Codex, so a
// test that skips this writes into the contributor's real Codex configuration.
func isolateCodex(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("STICKGUY_CODEX_EXECUTABLE", filepath.Join(t.TempDir(), "absent-codex"))
	return home
}

func TestDevelopmentAgentSetupIsExplicitAndUsesBuiltExecutable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	isolateCodex(t)
	for _, agent := range []string{"codex", "claude"} {
		project, state := t.TempDir(), t.TempDir()
		if err := run([]string{"--config-root", state, "setup", agent, "--development", "--project-root", project}); err != nil {
			t.Fatalf("%s development setup: %v", agent, err)
		}
		path := filepath.Join(project, ".mcp.json")
		if agent == "codex" {
			path = filepath.Join(project, ".codex", "config.toml")
		}
		contents, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(contents), state) || !strings.Contains(string(contents), "stickguy") {
			t.Fatalf("%s development config path=%s err=%v contents=%q", agent, path, err, contents)
		}
	}
}

func TestProductionAgentSetupUsesCurrentProfile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	isolateCodex(t)
	for _, agent := range []string{"codex", "claude"} {
		state, project := t.TempDir(), t.TempDir()
		if err := run([]string{"--config-root", state, "setup", agent, "--project-root", project}); err != nil {
			t.Fatalf("%s setup: %v", agent, err)
		}
		path := filepath.Join(project, ".mcp.json")
		if agent == "codex" {
			path = filepath.Join(project, ".codex", "config.toml")
		}
		contents, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(contents), "stickguy") || !strings.Contains(string(contents), state) {
			t.Fatalf("%s production config path=%s err=%v contents=%q", agent, path, err, contents)
		}
	}
}

func TestDiagnosticsDoctorSummaryRejectsProhibitedData(t *testing.T) {
	secret := "sk_live_abcdefghijklmno"
	repository := "/Users/person/private-repository"
	report := safeDoctorSummary(map[string]any{
		"status": "ok", "workspaces": 2, "pending": int64(3),
		"projectId": "prj_private", "repositoryRoot": repository,
		"environment": "DATABASE_URL=postgres://private", "token": secret,
		"commandOutput": "all private rows",
	})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, prohibited := range []string{"prj_private", repository, "DATABASE_URL", secret, "commandOutput", "private rows"} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("diagnostics disclosed %q in %s", prohibited, text)
		}
	}
	if !strings.Contains(text, `"status":"ok"`) || !strings.Contains(text, `"workspaces":2`) {
		t.Fatalf("diagnostics omitted safe health fields: %s", text)
	}
}

func TestRemoveAllAgentBindingsUsesManagedRemoval(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	codexHome := isolateCodex(t)
	state, project := t.TempDir(), t.TempDir()
	for _, agent := range []string{"codex", "claude"} {
		if err := run([]string{"--config-root", state, "setup", agent, "--project-root", project}); err != nil {
			t.Fatalf("setup %s: %v", agent, err)
		}
	}
	paths, err := config.Resolve(state)
	if err != nil {
		t.Fatal(err)
	}
	if err = config.Save(paths, config.Config{Version: 1, Workspaces: []config.Workspace{{ID: "wsp_test", Root: project}}}); err != nil {
		t.Fatal(err)
	}
	if err = run([]string{"--config-root", state, "setup", "remove-all"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(project, ".codex", "config.toml"), filepath.Join(project, ".mcp.json"), filepath.Join(codexHome, "hooks.json"), filepath.Join(project, ".claude", "settings.local.json")} {
		contents, readErr := os.ReadFile(path)
		if readErr == nil && strings.Contains(string(contents), "stickguy") {
			t.Fatalf("managed binding remained in %s: %s", path, contents)
		}
	}
}

// B5: `create` on a profile that has already enrolled used to mint a second
// device credential, which would strand the Projects the first one owns, so the
// only way to add a Project was an undocumented `workspace add`. An enrolled
// profile now takes the additional-Project path instead.
func TestCreateReusesAnEnrolledDeviceAndRefusesAConnectedRepository(t *testing.T) {
	paths, err := config.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := enrolledDevice(paths, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if fresh.deviceID != "" {
		t.Fatalf("a profile that never enrolled must take first enrollment: %#v", fresh)
	}

	connected := t.TempDir()
	if resolved, resolveErr := filepath.EvalSymlinks(connected); resolveErr == nil {
		connected = resolved
	}
	if err = config.Save(paths, config.Config{
		Version: 1, APIBaseURL: "https://enrolled.example", DeviceID: "dev_existing",
		Workspaces: []config.Workspace{{ID: "wsp_a", Root: connected, ProjectID: "prj_a"}},
	}); err != nil {
		t.Fatal(err)
	}

	// The API origin comes from the enrolled configuration, because the extra
	// Project must be created on the backend that issued the reused credential.
	state, err := enrolledDevice(paths, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if state.deviceID != "dev_existing" || state.apiBaseURL != "https://enrolled.example" {
		t.Fatalf("state=%#v", state)
	}

	if _, err = enrolledDevice(paths, connected); err == nil {
		t.Fatal("a repository already connected to a Project must be refused")
	}
}
