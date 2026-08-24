package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedProjectConfiguration(t *testing.T) {
	config, err := os.ReadFile(".codex/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	for _, want := range []string{"[mcp_servers.stickguy_gate_a]", `command = "/private/tmp/stickguy-gate-a/bin/gate-a-sdk-mcp"`, "startup_timeout_sec = 5", "tool_timeout_sec = 5"} {
		if !strings.Contains(text, want) {
			t.Errorf("config missing %q", want)
		}
	}
	for _, forbidden := range []string{"env =", "env_vars", "bearer", "http_headers", "notify", "otel"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("config contains forbidden field %q", forbidden)
		}
	}
	var hooks struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	b, err := os.ReadFile(".codex/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks.Hooks) != 2 || hooks.Hooks["SessionStart"] == nil || hooks.Hooks["SubagentStart"] == nil {
		t.Fatalf("only SessionStart/SubagentStart are permitted, got %v", hooks.Hooks)
	}
	for _, forbidden := range []string{"UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "SessionEnd", "SubagentStop"} {
		if hooks.Hooks[forbidden] != nil {
			t.Fatalf("content/control hook configured: %s", forbidden)
		}
	}
}

func TestInstructionsFrontLoadPrivacyAndLifecycle(t *testing.T) {
	if len(instructions) < 512 {
		t.Fatalf("instructions should exercise the 512-character boundary: %d", len(instructions))
	}
	front := instructions[:512]
	for _, want := range []string{"advisory", "begin_work", "check_coordination", "report_checkpoint", "finish_work", "Never send source", "prompts", "transcripts", "secrets"} {
		if !strings.Contains(front, want) {
			t.Errorf("first 512 characters missing %q", want)
		}
	}
}

func TestMCPInitializeListAndLifecycle(t *testing.T) {
	root := t.TempDir()
	registryPath := writeRegistry(t, []workspace{{Root: root, ProjectID: "prj_fixture", WorkspaceID: "wsp_fixture", WorkstreamID: "wrk_fixture"}})
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	s := &server{registryPath: registryPath, seen: make(map[string]struct{})}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"begin_work","arguments":{"idempotency_key":"begin-1"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"check_coordination","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"report_checkpoint","arguments":{"summary":"bounded fixture checkpoint"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"finish_work","arguments":{"idempotency_key":"finish-1"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := s.run(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("got %d response lines", len(lines))
	}
	for _, line := range lines {
		var got map[string]any
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatal(err)
		}
		if got["error"] != nil {
			t.Fatalf("unexpected error: %s", line)
		}
	}
	if !strings.Contains(lines[0], `"instructions"`) || !strings.Contains(lines[1], `"begin_work"`) {
		t.Fatalf("missing initialization/tool evidence")
	}
}

func TestWorkspaceResolutionAndAmbiguity(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	registryPath := writeRegistry(t, []workspace{{Root: root, WorkspaceID: "wsp_one"}})
	got, err := resolveWorkspace(registryPath, nested)
	if err != nil || got.WorkspaceID != "wsp_one" {
		t.Fatalf("resolve: %#v, %v", got, err)
	}
	registryPath = writeRegistry(t, []workspace{{Root: root}, {Root: nested}})
	if _, err := resolveWorkspace(registryPath, nested); err == nil || err.Error() != "workspace_registration_ambiguous" {
		t.Fatalf("expected explicit ambiguity, got %v", err)
	}
}

func TestRejectsContentBearingArgumentsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	registryPath := writeRegistry(t, []workspace{{Root: root, ProjectID: "p", WorkspaceID: "w", WorkstreamID: "s"}})
	old, _ := os.Getwd()
	_ = os.Chdir(root)
	t.Cleanup(func() { _ = os.Chdir(old) })
	s := &server{registryPath: registryPath, seen: make(map[string]struct{})}
	bad, _ := json.Marshal(map[string]any{"name": "report_checkpoint", "arguments": map[string]any{"transcript": "forbidden"}})
	if _, err := s.call(bad); err == nil {
		t.Fatal("expected prohibited argument rejection")
	}
	good, _ := json.Marshal(map[string]any{"name": "begin_work", "arguments": map[string]any{"idempotency_key": "same"}})
	if _, err := s.call(good); err != nil {
		t.Fatal(err)
	}
	got, err := s.call(good)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	if !bytes.Contains(b, []byte(`"duplicate":true`)) {
		t.Fatalf("expected duplicate marker: %s", b)
	}
}

func writeRegistry(t *testing.T, workspaces []workspace) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "registry.json")
	b, err := json.Marshal(registry{Workspaces: workspaces})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
