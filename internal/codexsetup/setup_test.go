package codexsetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupStatusRemovalPreserveUnrelatedConfigAndRefuseDrift(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, ".codex")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	original := "model = \"fixture\"\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "state"), Executable: "/usr/local/bin/stickguy"}
	if status, err := manager.Setup(); err != nil || !status.Configured || status.Hooks != "active" {
		t.Fatal(status, err)
	}
	first, _ := os.ReadFile(path)
	if _, err := manager.Setup(); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) || !strings.HasPrefix(string(second), original) {
		t.Fatal("setup was not byte-idempotent or preserved")
	}
	if status, err := manager.Status(); err != nil || !status.Configured {
		t.Fatal(status, err)
	}
	drifted := strings.Replace(string(second), "tool_timeout_sec = 60", "tool_timeout_sec = 61", 1)
	if err := os.WriteFile(path, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Remove(); err == nil {
		t.Fatal("drifted block removed")
	}
	if err := os.WriteFile(path, second, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Remove(); err != nil {
		t.Fatal(err)
	}
	remaining, _ := os.ReadFile(path)
	if string(remaining) != original {
		t.Fatalf("unrelated config changed: %q", remaining)
	}
	if _, err := manager.Remove(); err != nil {
		t.Fatal(err)
	}
	portable := Manager{ProjectRoot: project, Portable: true}
	if _, err := portable.Setup(); err != nil {
		t.Fatal(err)
	}
	managed, _ := os.ReadFile(path)
	if strings.Contains(string(managed), manager.ConfigRoot) || !strings.Contains(string(managed), "command = \"stickguy\"") || !strings.Contains(string(managed), "args = [\"mcp\"]") {
		t.Fatalf("portable config contains machine state or lacks PATH command: %q", managed)
	}
	if _, err := portable.Remove(); err != nil {
		t.Fatal(err)
	}
}
