package codexsetup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// trustedForTest makes unit tests deterministic and offline. Trust repair
// spawns a Codex child process, so no test may reach the real implementation.
func trustedForTest(t *testing.T) {
	t.Helper()
	original := inspectTrust
	inspectTrust = func(_ Manager, _ context.Context, _, _ string, _ bool) TrustReport {
		return TrustReport{Method: TrustMethodAppServer, Total: 9, Trusted: 9}
	}
	t.Cleanup(func() { inspectTrust = original })
}

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
	trustedForTest(t)
	manager := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "state"), Executable: "/usr/local/bin/overgent", CodexHome: t.TempDir()}
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
	// A different managed command is a different profile, so it gets its own
	// Codex home; switching profiles in place is Rebind's job, not Setup's.
	portable := Manager{ProjectRoot: project, Portable: true, CodexHome: t.TempDir()}
	if _, err := portable.Setup(); err != nil {
		t.Fatal(err)
	}
	managed, _ := os.ReadFile(path)
	if strings.Contains(string(managed), manager.ConfigRoot) || !strings.Contains(string(managed), "command = \"overgent\"") || !strings.Contains(string(managed), "args = [\"mcp\"]") {
		t.Fatalf("portable config contains machine state or lacks PATH command: %q", managed)
	}
	if _, err := portable.Remove(); err != nil {
		t.Fatal(err)
	}
}

func TestOtherProfileIsExplicitAndRebindPreservesUnrelatedConfig(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("model = \"fixture\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	executable := "/usr/local/bin/overgent"
	codexHome := t.TempDir()
	trustedForTest(t)
	oldProfile := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "old"), Executable: executable, CodexHome: codexHome}
	newProfile := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "shared"), Executable: executable, CodexHome: codexHome}
	if _, err := oldProfile.Setup(); err != nil {
		t.Fatal(err)
	}
	status, err := newProfile.Status()
	if err != nil || status.Binding != "other_profile" || status.Configured || status.PreviousProfile == "" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	if _, err = newProfile.Setup(); err == nil || !strings.Contains(err.Error(), "explicit reconnect") {
		t.Fatalf("ordinary setup should not detach another profile: %v", err)
	}
	beforeConfig, _ := os.ReadFile(path)
	hookPath := filepath.Join(codexHome, "hooks.json")
	beforeHooks, _ := os.ReadFile(hookPath)
	originalRebind := rebindHooks
	rebindHooks = func(string, string) error { return errors.New("synthetic hook failure") }
	if _, err = newProfile.Rebind(); err == nil {
		t.Fatal("synthetic hook failure did not fail reconnect")
	}
	rebindHooks = originalRebind
	afterConfig, _ := os.ReadFile(path)
	afterHooks, _ := os.ReadFile(hookPath)
	if string(afterConfig) != string(beforeConfig) || string(afterHooks) != string(beforeHooks) {
		t.Fatal("failed reconnect did not restore both Codex files")
	}
	if _, err = newProfile.Rebind(); err != nil {
		t.Fatal(err)
	}
	status, err = newProfile.Status()
	if err != nil || !status.Configured || status.Binding != "current" || status.Hooks != "active" {
		t.Fatalf("rebound status=%#v err=%v", status, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "model = \"fixture\"\n") || strings.Contains(string(data), oldProfile.ConfigRoot) || !strings.Contains(string(data), newProfile.ConfigRoot) {
		t.Fatalf("rebind damaged unrelated config or retained old profile: %s", data)
	}
}
