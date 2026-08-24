package claudesetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupStatusRemovalMergeAndRefuseDrift(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".mcp.json")
	original := []byte(`{"other":{"preserved":true},"mcpServers":{"fixture":{"type":"http","url":"https://example.invalid/mcp"}}}`)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	manager := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "state"), Executable: "/usr/local/bin/stickguy"}
	if got, err := manager.Setup(); err != nil || !got.Configured || got.Approval != "required_by_claude" {
		t.Fatal(got, err)
	}
	first, _ := os.ReadFile(path)
	if _, err := manager.Setup(); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatal("setup was not byte-idempotent")
	}
	var document map[string]any
	if err := json.Unmarshal(second, &document); err != nil || document["other"].(map[string]any)["preserved"] != true {
		t.Fatalf("unrelated JSON not preserved: %#v err=%v", document, err)
	}
	servers := document["mcpServers"].(map[string]any)
	servers["stickguy"].(map[string]any)["command"] = "/drifted"
	drifted, _ := json.Marshal(document)
	if err := os.WriteFile(path, drifted, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Remove(); err == nil {
		t.Fatal("drifted entry removed")
	}
	if err := os.WriteFile(path, second, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Remove(); err != nil {
		t.Fatal(err)
	}
	remaining, _ := os.ReadFile(path)
	if err := json.Unmarshal(remaining, &document); err != nil {
		t.Fatal(err)
	}
	servers = document["mcpServers"].(map[string]any)
	if _, exists := servers["stickguy"]; exists || servers["fixture"] == nil || document["other"] == nil {
		t.Fatalf("removal damaged unrelated config: %#v", document)
	}
	if _, err := manager.Remove(); err != nil {
		t.Fatal(err)
	}
	portable := Manager{ProjectRoot: project, Portable: true}
	if _, err := portable.Setup(); err != nil {
		t.Fatal(err)
	}
	managed, _ := os.ReadFile(path)
	if strings.Contains(string(managed), manager.ConfigRoot) || !strings.Contains(string(managed), `"command": "stickguy"`) || !strings.Contains(string(managed), `"mcp"`) {
		t.Fatalf("portable config contains machine state or lacks PATH command: %s", managed)
	}
	if _, err := portable.Remove(); err != nil {
		t.Fatal(err)
	}
}
