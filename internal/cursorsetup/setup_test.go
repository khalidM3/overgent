package cursorsetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/agentactivity"
)

func managerFor(t *testing.T, project string) Manager {
	t.Helper()
	return Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "state"), Executable: "/usr/local/bin/overgent"}
}

func loadHooks(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSetupWritesEveryInstalledEventWithFailClosedFalse(t *testing.T) {
	project := t.TempDir()
	manager := managerFor(t, project)
	status, err := manager.Setup()
	if err != nil || !status.Configured || status.Binding != "current" {
		t.Fatal(status, err)
	}
	if !strings.HasSuffix(status.ConfigPath, filepath.Join(".cursor", "hooks.json")) {
		t.Fatalf("configuration went to the wrong file: %s", status.ConfigPath)
	}
	document := loadHooks(t, status.ConfigPath)
	if document["version"] != float64(schemaVersion) {
		t.Fatalf("schema version is %v", document["version"])
	}
	hooks, ok := document["hooks"].(map[string]any)
	if !ok || len(hooks) != len(agentactivity.CursorEvents) {
		t.Fatalf("unexpected hook map: %v", document["hooks"])
	}
	for _, event := range agentactivity.CursorEvents {
		entries, entriesOK := hooks[event].([]any)
		if !entriesOK || len(entries) != 1 {
			t.Fatalf("%s: unexpected handlers %v", event, hooks[event])
		}
		entry := entries[0].(map[string]any)
		// Overgent must never block, delay, or fail a Cursor turn (ADR-017), so
		// this is written explicitly rather than left to a vendor default.
		if entry["failClosed"] != false {
			t.Fatalf("%s: failClosed is %v; Overgent must never block a turn", event, entry["failClosed"])
		}
		if entry["type"] != "command" {
			t.Fatalf("%s: handler type is %v", event, entry["type"])
		}
		// Two events name themselves nowhere in their payload, so every command
		// states which event it was installed for.
		command, _ := entry["command"].(string)
		if !strings.HasSuffix(command, " agent-hook --vendor cursor --event "+event) {
			t.Fatalf("%s: command does not declare its own event: %q", event, command)
		}
	}
	if status, err = manager.Status(); err != nil || !status.Configured || status.Binding != "current" {
		t.Fatal(status, err)
	}
}

func TestSetupPreservesUnrelatedKeysHooksAndUndecodableEvents(t *testing.T) {
	project := t.TempDir()
	directory := filepath.Join(project, ".cursor")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "hooks.json")
	original := `{"version":1,"experimental":{"keep":true},"hooks":{"sessionStart":[{"command":"/usr/local/bin/lint","type":"command","timeout":9}],"beforeShellExecution":"member-shape-this-package-cannot-model"}}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := managerFor(t, project)
	if _, err := manager.Setup(); err != nil {
		t.Fatal(err)
	}
	document := loadHooks(t, path)
	if experimental, ok := document["experimental"].(map[string]any); !ok || experimental["keep"] != true {
		t.Fatalf("an unrelated top-level key was lost: %v", document["experimental"])
	}
	hooks := document["hooks"].(map[string]any)
	if hooks["beforeShellExecution"] != "member-shape-this-package-cannot-model" {
		t.Fatalf("an event this package cannot decode was rewritten: %v", hooks["beforeShellExecution"])
	}
	entries := hooks["sessionStart"].([]any)
	if len(entries) != 2 {
		t.Fatalf("the member's own sessionStart hook was not preserved alongside Overgent's: %v", entries)
	}
	if entries[0].(map[string]any)["command"] != "/usr/local/bin/lint" {
		t.Fatalf("the member's hook was displaced: %v", entries[0])
	}

	// Removal restores the file to exactly what the member had, including the
	// undecodable event and the unrelated key.
	if _, err := manager.Remove(); err != nil {
		t.Fatal(err)
	}
	after := loadHooks(t, path)
	afterHooks := after["hooks"].(map[string]any)
	if len(afterHooks) != 2 || len(afterHooks["sessionStart"].([]any)) != 1 {
		t.Fatalf("removal did not leave unrelated configuration intact: %v", afterHooks)
	}
	if afterHooks["beforeShellExecution"] != "member-shape-this-package-cannot-model" {
		t.Fatalf("removal rewrote an event it could not decode: %v", afterHooks["beforeShellExecution"])
	}
	if experimental, ok := after["experimental"].(map[string]any); !ok || experimental["keep"] != true {
		t.Fatal("removal dropped an unrelated top-level key")
	}
}

// An event whose handlers Overgent wrote and then emptied must actually
// disappear, rather than being restored from the bytes it was read from.
func TestRemovalDeletesEventsOvergentFullyOwned(t *testing.T) {
	project := t.TempDir()
	manager := managerFor(t, project)
	if _, err := manager.Setup(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, ".cursor", "hooks.json")
	if _, err := manager.Remove(); err != nil {
		t.Fatal(err)
	}
	if hooks := loadHooks(t, path)["hooks"].(map[string]any); len(hooks) != 0 {
		t.Fatalf("removal left Overgent hooks behind: %v", hooks)
	}
}

func TestAnotherProfileIsRecognizedAndOnlyMovedByExplicitReconnect(t *testing.T) {
	project := t.TempDir()
	other := managerFor(t, project)
	if _, err := other.Setup(); err != nil {
		t.Fatal(err)
	}
	mine := managerFor(t, project)
	status, err := mine.Status()
	if err != nil || status.Binding != "other_profile" || status.PreviousProfile == "" {
		t.Fatal(status, err)
	}
	if _, err = mine.Setup(); err == nil {
		t.Fatal("Setup must refuse to take over another profile's binding silently")
	}
	if status, err = mine.Rebind(); err != nil || !status.Configured || status.Binding != "current" {
		t.Fatal(status, err)
	}
	if status, err = mine.Status(); err != nil || status.Binding != "current" {
		t.Fatal(status, err)
	}
	if status, err = other.Status(); err != nil || status.Binding != "other_profile" {
		t.Fatal(status, err)
	}
}

func TestRebindRestoresTheOriginalDocumentWhenTheWriteFails(t *testing.T) {
	project := t.TempDir()
	directory := filepath.Join(project, ".cursor")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	other := managerFor(t, project)
	if _, err := other.Setup(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "hooks.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A read-only directory makes the atomic write fail after the snapshot is
	// taken, which is the failure the rollback exists for.
	if err = os.Chmod(directory, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o755) })
	if _, err = managerFor(t, project).Rebind(); err == nil {
		t.Fatal("a failed rebind must report the failure")
	}
	if err = os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("a failed rebind left the document changed:\n%s\n%s", before, after)
	}
}

func TestUnknownDriftRefusesEveryOperation(t *testing.T) {
	for name, contents := range map[string]string{
		"unsupported schema version": `{"version":2,"hooks":{}}`,
		"managed command drifted":    `{"version":1,"hooks":{"stop":[{"command":"unexpected agent-hook --vendor cursor --event stop","type":"command","timeout":5,"failClosed":false}]}}`,
		"managed hook on an unknown event": `{"version":1,"hooks":{"beforeShellExecution":[` +
			`{"command":"'overgent' agent-hook --vendor cursor --event beforeShellExecution","type":"command","timeout":5,"failClosed":false}]}}`,
	} {
		project := t.TempDir()
		directory := filepath.Join(project, ".cursor")
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, "hooks.json")
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		manager := managerFor(t, project)
		if _, err := manager.Status(); err == nil {
			t.Fatalf("%s: Status must fail closed", name)
		}
		if _, err := manager.Setup(); err == nil {
			t.Fatalf("%s: Setup must refuse to overwrite", name)
		}
		if _, err := manager.Remove(); err == nil {
			t.Fatalf("%s: Remove must refuse to guess", name)
		}
		unchanged, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(unchanged) != contents {
			t.Fatalf("%s: a refused operation still modified the file", name)
		}
	}
}

func TestSetupIsIdempotent(t *testing.T) {
	project := t.TempDir()
	manager := managerFor(t, project)
	if _, err := manager.Setup(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, ".cursor", "hooks.json")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Setup(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("a repeated Setup changed the document:\n%s\n%s", first, second)
	}
}

func TestPartialInstallIsReportedAndRepaired(t *testing.T) {
	project := t.TempDir()
	manager := managerFor(t, project)
	if _, err := manager.Setup(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, ".cursor", "hooks.json")
	document := loadHooks(t, path)
	hooks := document["hooks"].(map[string]any)
	delete(hooks, "beforeReadFile")
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status()
	if err != nil || status.Configured || status.Binding != "partial" {
		t.Fatal(status, err)
	}
	if status, err = manager.Setup(); err != nil || !status.Configured {
		t.Fatal(status, err)
	}
	if status, err = manager.Status(); err != nil || status.Binding != "current" {
		t.Fatal(status, err)
	}
}

// File presence is not evidence that Cursor will run what was written, so the
// reported approval state says so rather than implying a working binding.
func TestApprovalIsReportedAsUnverified(t *testing.T) {
	project := t.TempDir()
	status, err := managerFor(t, project).Setup()
	if err != nil {
		t.Fatal(err)
	}
	if status.Approval != "unverified_by_cursor" {
		t.Fatalf("approval is %q; a configured file must not be reported as a proven binding", status.Approval)
	}
}
