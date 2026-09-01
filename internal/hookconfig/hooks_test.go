package hookconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyHandler is the shape Stickguy wrote before injection boundaries were
// bounded and before the Codex SessionEnd ceiling was honoured. It lives here
// now because it is only interesting as an input to repair.
func legacyHandler(event, command string) handler {
	return handler{Type: "command", Command: command, Async: event != "SessionEnd", Timeout: 5}
}

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

func TestInstallMigratesLegacyInjectionHandlers(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	command, _ := Command("/a/stickguy", "/state", "codex")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{"hooks": map[string]any{"SessionStart": []group{{Hooks: []handler{legacyHandler("SessionStart", command)}}}}}
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

// Codex silently lowers a SessionEnd timeout above its ceiling and warns while
// doing it, so the value written has to be the ceiling itself. Claude has no
// such cap and keeps the ordinary observation budget.
func TestCodexSessionEndTimeoutMatchesTheCodexCeiling(t *testing.T) {
	codex, _ := Command("/a/stickguy", "/state", "codex")
	configured := expected("SessionEnd", codex).Hooks[0]
	if configured.Async || configured.Timeout != codexSessionEndTimeout {
		t.Fatalf("codex SessionEnd handler=%#v", configured)
	}
	claude, _ := Command("/a/stickguy", "/state", "claude")
	configured = expected("SessionEnd", claude).Hooks[0]
	if configured.Async || configured.Timeout != 5 {
		t.Fatalf("claude SessionEnd handler=%#v", configured)
	}
}

// A member who installed before the ceiling was honoured has a 5s SessionEnd on
// disk. That is the shape legacyExpected describes, so setup must repair it in
// place rather than refusing the file as drift and leaving the warning running.
func TestInstallRepairsAnOverLongCodexSessionEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	command, _ := Command("/a/stickguy", "/state", "codex")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := map[string]any{"hooks": map[string]any{"SessionEnd": []group{{Hooks: []handler{legacyHandler("SessionEnd", command)}}}}}
	encoded, _ := json.Marshal(stale)
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	inspection, err := Inspect(path, command)
	if err != nil || inspection.State != BindingPartial {
		t.Fatalf("stale inspection=%#v err=%v", inspection, err)
	}
	if err = Install(path, command); err != nil {
		t.Fatal(err)
	}
	if ok, statusErr := Status(path, command); statusErr != nil || !ok {
		t.Fatalf("repaired status=%v err=%v", ok, statusErr)
	}
	data, _ := os.ReadFile(path)
	var document struct {
		Hooks map[string][]group `json:"hooks"`
	}
	if err = json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if written := document.Hooks["SessionEnd"][0].Hooks[0].Timeout; written != codexSessionEndTimeout {
		t.Fatalf("SessionEnd timeout on disk = %d, want %d: %s", written, codexSessionEndTimeout, data)
	}
}

// The friend's case from the closed test: he edited the SessionEnd timeout in
// his own hooks.json to silence Codex's clamp warning, and Stickguy answered
// "managed Stickguy activity hook drifted" on a row whose only affordance,
// reconnect, is offered for another profile and so never appeared. A handler
// carrying this profile's exact command is ours whatever its tuning says.
func TestMemberEditedTuningIsRepairedNotRefused(t *testing.T) {
	for _, edited := range []int{4, 30, 0} {
		path := filepath.Join(t.TempDir(), "hooks.json")
		command, _ := Command("/a/stickguy", "/state", "codex")
		if err := Install(path, command); err != nil {
			t.Fatal(err)
		}
		rewriteSessionEndTimeout(t, path, edited)

		inspection, err := Inspect(path, command)
		if err != nil {
			t.Fatalf("timeout %d: inspect refused a hook carrying our own command: %v", edited, err)
		}
		if inspection.State != BindingPartial {
			t.Fatalf("timeout %d: state=%q, want %q", edited, inspection.State, BindingPartial)
		}
		if err = Install(path, command); err != nil {
			t.Fatalf("timeout %d: install refused to repair: %v", edited, err)
		}
		if ok, statusErr := Status(path, command); statusErr != nil || !ok {
			t.Fatalf("timeout %d: repaired status=%v err=%v", edited, ok, statusErr)
		}
	}
}

// Ownership is still decided by the command alone. A hook belonging to another
// profile must never be rewritten, however plausible its tuning looks.
func TestAnotherProfilesHookIsStillRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	mine, _ := Command("/a/stickguy", "/state", "codex")
	theirs, _ := Command("/a/stickguy", "/other state", "codex")
	if err := Install(path, theirs); err != nil {
		t.Fatal(err)
	}
	rewriteSessionEndTimeout(t, path, 4)
	inspection, err := Inspect(path, mine)
	if err == nil && inspection.State == BindingCurrent {
		t.Fatalf("another profile's hook was adopted: %#v", inspection)
	}
	before, _ := os.ReadFile(path)
	if err = Install(path, mine); err == nil {
		t.Fatal("expected Install to refuse another profile's hook")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("refused install still wrote the file:\n%s\n%s", before, after)
	}
}

func rewriteSessionEndTimeout(t *testing.T, path string, timeout int) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	var hooks map[string][]group
	if err = json.Unmarshal(document["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	hooks["SessionEnd"][0].Hooks[0].Timeout = timeout
	if document["hooks"], err = json.Marshal(hooks); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
}
