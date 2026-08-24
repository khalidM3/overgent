package adapterconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClaudeInstallRemovePreservesUnrelatedSettings(t *testing.T) {
	original := []byte(`{"permissions":{"allow":["Read"]},"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"existing-tool"}]}]}}`)
	installed, err := InstallClaude(original)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installed), managedPrefix+"SessionStart") || !strings.Contains(string(installed), "existing-tool") {
		t.Fatalf("managed or unrelated hook missing: %s", installed)
	}
	removed, err := RemoveClaude(installed)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(removed, &document); err != nil {
		t.Fatal(err)
	}
	permissions := document["permissions"].(map[string]any)
	if len(permissions["allow"].([]any)) != 1 || strings.Contains(string(removed), managedPrefix) || !strings.Contains(string(removed), "existing-tool") {
		t.Fatalf("remove changed unrelated settings or retained managed data: %s", removed)
	}
}

func TestClaudeConfigDriftFailsClosed(t *testing.T) {
	input := []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"stickguy activity-hook Unexpected"}]}]}}`)
	if _, err := InstallClaude(input); err == nil || !strings.Contains(err.Error(), "drifted") {
		t.Fatalf("install drift error = %v", err)
	}
	if _, err := RemoveClaude(input); err == nil || !strings.Contains(err.Error(), "drifted") {
		t.Fatalf("remove drift error = %v", err)
	}
}

func TestClaudeInstallIsExplicitAndDoesNotChangePermissions(t *testing.T) {
	input := []byte(`{"permissions":{"deny":["Bash"]}}`)
	installed, err := InstallClaude(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installed), `"deny": [`) || !strings.Contains(string(installed), `"Bash"`) {
		t.Fatalf("permissions changed: %s", installed)
	}
	if _, err := InstallClaude(installed); err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("duplicate install error = %v", err)
	}
}
