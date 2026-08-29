package hookconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallStatusRemovePreservesUnrelatedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"permissions":{"allow":["Read"]},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"other-hook"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	command, err := Command("/Applications/Stick Guy/bin/stickguy", "/tmp/state root", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(path, command); err != nil {
		t.Fatal(err)
	}
	if ok, err := Status(path, command); err != nil || !ok {
		t.Fatalf("status=%v err=%v", ok, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "other-hook") || !strings.Contains(string(data), "permissions") {
		t.Fatalf("unrelated settings lost: %s", data)
	}
	if err := Remove(path, command); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "other-hook") || strings.Contains(string(data), "agent-hook") {
		t.Fatalf("remove result: %s", data)
	}
}

func TestDriftFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	command, _ := Command("/a/stickguy", "/state", "codex")
	if err := Install(path, command); err != nil {
		t.Fatal(err)
	}
	other, _ := Command("/b/stickguy", "/state", "codex")
	if err := Install(path, other); err == nil {
		t.Fatal("expected drift error")
	}
}

func TestInspectAndRebindOtherProfilePreservesUnrelatedHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	oldCommand, _ := Command("/Applications/Stickguy Dev.app/bin/stickguy", "/tmp/old profile", "codex")
	newCommand, _ := Command("/Applications/Stickguy Dev.app/bin/stickguy", "/tmp/shared profile", "codex")
	if err := Install(path, oldCommand); err != nil {
		t.Fatal(err)
	}
	document, hooks, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	groups, _ := groupsFor(hooks["Stop"])
	groups = append(groups, group{Hooks: []handler{{Type: "command", Command: "other-hook"}}})
	hooks["Stop"], _ = json.Marshal(groups)
	if err = write(path, document, hooks); err != nil {
		t.Fatal(err)
	}
	inspection, err := Inspect(path, newCommand)
	if err != nil || inspection.State != BindingOtherProfile || inspection.ExistingCommand != oldCommand {
		t.Fatalf("inspection=%#v err=%v", inspection, err)
	}
	if err = Rebind(path, newCommand); err != nil {
		t.Fatal(err)
	}
	inspection, err = Inspect(path, newCommand)
	if err != nil || inspection.State != BindingCurrent {
		t.Fatalf("rebound inspection=%#v err=%v", inspection, err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "/tmp/old profile") || !strings.Contains(string(data), "/tmp/shared profile") || !strings.Contains(string(data), "other-hook") {
		t.Fatalf("rebound hooks lost or retained wrong state: %s", data)
	}
}

func TestInstallAutomaticallyRepairsPartialCurrentBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	command, _ := Command("/Applications/Stickguy Dev.app/bin/stickguy", "/tmp/shared profile", "codex")
	if err := Install(path, command); err != nil {
		t.Fatal(err)
	}
	document, hooks, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	delete(hooks, "SessionStart")
	if err = write(path, document, hooks); err != nil {
		t.Fatal(err)
	}
	if inspection, inspectErr := Inspect(path, command); inspectErr != nil || inspection.State != BindingPartial {
		t.Fatalf("partial inspection=%#v err=%v", inspection, inspectErr)
	}
	if err = Install(path, command); err != nil {
		t.Fatal(err)
	}
	if inspection, inspectErr := Inspect(path, command); inspectErr != nil || inspection.State != BindingCurrent {
		t.Fatalf("repaired inspection=%#v err=%v", inspection, inspectErr)
	}
}

func TestInjectionBoundariesAreSynchronousAndBounded(t *testing.T) {
	for _, vendor := range []string{"claude", "codex"} {
		command, _ := Command("/a/stickguy", "/state", vendor)
		for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
			configured := expected(event, command).Hooks[0]
			if configured.Async || configured.Timeout != 2 {
				t.Fatalf("%s %s handler=%#v", vendor, event, configured)
			}
		}
		configured := expected("PostToolUse", command).Hooks[0]
		if !configured.Async || configured.Timeout != 5 {
			t.Fatalf("%s observation handler=%#v", vendor, configured)
		}
	}
}

func TestInstallMigratesOnlyRecognizedLegacyInjectionHandlers(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	command, _ := Command("/a/stickguy", "/state", "codex")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{"hooks": map[string]any{"SessionStart": []group{{Hooks: []handler{legacyExpected("SessionStart", command)}}}}}
	encoded, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	inspection, err := Inspect(path, command)
	if err != nil || inspection.State != BindingPartial {
		t.Fatalf("legacy inspection=%#v err=%v", inspection, err)
	}
	if err = Install(path, command); err != nil {
		t.Fatal(err)
	}
	inspection, err = Inspect(path, command)
	if err != nil || inspection.State != BindingCurrent {
		t.Fatalf("migrated inspection=%#v err=%v", inspection, err)
	}
}

// The grant must be narrow: exactly Stickguy's tools, nothing a member wrote
// disturbed, and a teardown that withdraws precisely what it granted.
func TestToolApprovalIsNarrowAndReversible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.local.json")
	if err := os.WriteFile(path, []byte(`{"permissions":{"allow":["Bash(git status)"],"deny":["Bash(rm:*)"]},"model":"opus"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AllowTools(path, StickguyMCPTools); err != nil {
		t.Fatal(err)
	}
	document := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	permissions := document["permissions"].(map[string]any)
	allow := permissions["allow"].([]any)
	if len(allow) != 1+len(StickguyMCPTools) {
		t.Fatalf("allow=%v", allow)
	}
	if allow[0] != "Bash(git status)" {
		t.Fatalf("a member's own permission was disturbed: %v", allow)
	}
	for _, entry := range allow {
		if text := entry.(string); text != "Bash(git status)" && !strings.HasPrefix(text, "mcp__stickguy__") {
			t.Fatalf("granted something outside Stickguy's own tools: %q", text)
		}
	}
	if permissions["deny"] == nil || document["model"] != "opus" {
		t.Fatalf("unrelated settings were lost: %v", document)
	}

	// Granting twice must not duplicate entries.
	if err = AllowTools(path, StickguyMCPTools); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	document = map[string]any{}
	_ = json.Unmarshal(data, &document)
	if again := document["permissions"].(map[string]any)["allow"].([]any); len(again) != 1+len(StickguyMCPTools) {
		t.Fatalf("re-granting duplicated entries: %v", again)
	}

	if err = DisallowTools(path, StickguyMCPTools); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	document = map[string]any{}
	_ = json.Unmarshal(data, &document)
	permissions = document["permissions"].(map[string]any)
	remaining := permissions["allow"].([]any)
	if len(remaining) != 1 || remaining[0] != "Bash(git status)" {
		t.Fatalf("teardown did not withdraw exactly what it granted: %v", remaining)
	}
	if permissions["deny"] == nil || document["model"] != "opus" {
		t.Fatalf("teardown lost unrelated settings: %v", document)
	}
}
