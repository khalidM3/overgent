package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/khalidM3/overgent/internal/agentactivity"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/daemon"
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
	t.Setenv("OVERGENT_CODEX_EXECUTABLE", filepath.Join(t.TempDir(), "absent-codex"))
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
		if err != nil || !strings.Contains(string(contents), state) || !strings.Contains(string(contents), "overgent") {
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
		if err != nil || !strings.Contains(string(contents), "overgent") || !strings.Contains(string(contents), state) {
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
		if readErr == nil && strings.Contains(string(contents), "overgent") {
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

// Cursor's hook response is its own shape: `env` publishes the session-scoped
// workspace root from sessionStart, and `additional_context` carries a
// correction into the turn that triggered the hook. Neither is Claude's
// `hookSpecificOutput`, so the two paths are asserted separately.
func TestCursorHookPublishesTheWorkspaceRootAndPushesContext(t *testing.T) {
	root := t.TempDir()
	input := []byte(`{"conversation_id":"conv-1","session_id":"sess-1","hook_event_name":"sessionStart","workspace_roots":["` + root + `"]}`)
	var output bytes.Buffer
	var methods []string
	call := func(_ context.Context, _ string, request daemon.Request) (daemon.Response, error) {
		methods = append(methods, request.Method)
		if request.AgentVendor != "cursor" || request.AgentEvent != "SessionStart" {
			t.Fatalf("unexpected request: %+v", request)
		}
		if request.Method == "agent_event" {
			// The service answers with the root it actually resolved, which is
			// what later hooks in this session are pinned to.
			return daemon.Response{OK: true, Data: map[string]any{"accepted": true, "workspaceRoot": root}}, nil
		}
		return daemon.Response{OK: true, Data: map[string]any{"additionalContext": "Coordination update: Refresh changed."}}, nil
	}
	if err := runCursorHook(context.Background(), "unused", "sessionStart", bytes.NewReader(input), &output, call); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not JSON: %q", output.String())
	}
	env, ok := decoded["env"].(map[string]any)
	if !ok || env["OVERGENT_CURSOR_WORKSPACE_ROOT"] != root {
		t.Fatalf("sessionStart did not publish the workspace root: %q", output.String())
	}
	if decoded["additional_context"] != "Coordination update: Refresh changed." {
		t.Fatalf("correction was not pushed into the turn: %q", output.String())
	}
	if strings.Contains(output.String(), "hookSpecificOutput") {
		t.Fatalf("Cursor must not be handed Claude's response shape: %q", output.String())
	}
	if len(methods) != 2 || methods[0] != "agent_event" || methods[1] != "agent_injection" {
		t.Fatalf("unexpected call sequence: %v", methods)
	}
}

// afterFileEdit carries no workspace root and no event name, so the session
// variable and the installed --event are the only things that can place it.
// It is also not an injection point: it must observe and stay silent.
func TestCursorEditHookUsesTheSessionVariableAndEmitsNothing(t *testing.T) {
	root := t.TempDir()
	t.Setenv(agentactivity.CursorWorkspaceRootEnv, root)
	input := []byte(`{"conversation_id":"conv-1","file_path":"` + filepath.Join(root, "frontend", "session.ts") + `"}`)
	var output bytes.Buffer
	var methods []string
	call := func(_ context.Context, _ string, request daemon.Request) (daemon.Response, error) {
		methods = append(methods, request.Method)
		if request.AgentCWD != root || request.AgentEvent != "PostToolUse" || request.AgentTool != "edit" {
			t.Fatalf("unexpected request: %+v", request)
		}
		return daemon.Response{OK: true, Data: map[string]any{"accepted": true, "workspaceRoot": root}}, nil
	}
	if err := runCursorHook(context.Background(), "unused", "afterFileEdit", bytes.NewReader(input), &output, call); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("an observation-only hook wrote output: %q", output.String())
	}
	if len(methods) != 1 || methods[0] != "agent_event" {
		t.Fatalf("unexpected call sequence: %v", methods)
	}
}

// Nothing Overgent cannot do may reach a Cursor turn (ADR-017), so an
// unreachable service, an unknown event, and unreadable input all produce no
// output and no error.
func TestCursorHookFailsOpenWithoutOutput(t *testing.T) {
	root := t.TempDir()
	valid := `{"conversation_id":"conv-1","hook_event_name":"sessionStart","workspace_roots":["` + root + `"]}`
	failing := func(_ context.Context, _ string, _ daemon.Request) (daemon.Response, error) {
		return daemon.Response{}, context.DeadlineExceeded
	}
	for name, input := range map[string]string{"valid": valid, "malformed": "{not json", "empty": ""} {
		var output bytes.Buffer
		if err := runCursorHook(context.Background(), "unused", "sessionStart", strings.NewReader(input), &output, failing); err != nil {
			t.Fatalf("%s: hook returned an error: %v", name, err)
		}
		if output.Len() != 0 {
			t.Fatalf("%s: hook wrote output despite failure: %q", name, output.String())
		}
	}
	var output bytes.Buffer
	if err := runCursorHook(context.Background(), "unused", "beforeShellExecution", strings.NewReader(valid), &output, failing); err != nil || output.Len() != 0 {
		t.Fatalf("an unconfigured event produced %q / %v", output.String(), err)
	}
}
