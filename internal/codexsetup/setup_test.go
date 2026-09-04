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
	// The other profile has to look alive for this test to be about anything:
	// an Overgent binary that still exists, and a profile directory holding an
	// enrolled device. A binding nothing is using any more is adopted without
	// asking, which TestAbandonedBindingIsAdoptedWithoutAsking covers.
	executable := liveExecutable(t)
	codexHome := t.TempDir()
	trustedForTest(t)
	oldProfile := Manager{ProjectRoot: project, ConfigRoot: liveProfile(t), Executable: executable, CodexHome: codexHome}
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

// liveExecutable is an Overgent binary that exists and is runnable, so a
// binding pointing at it is not abandoned merely because its file is gone.
func liveExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overgent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// liveProfile is an Overgent profile directory holding an enrolled device, so
// a binding that names it is a real decision rather than a leftover.
func liveProfile(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "live")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"version":1,"deviceId":"dev_live"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// A binding left behind by an earlier Overgent - here a former product name,
// which is the exact shape the Stickguy rename left in every member's
// user-level Codex hooks - is adopted by an ordinary Setup, with no reconnect
// button and no decision presented to anyone.
func TestAbandonedBindingIsAdoptedWithoutAsking(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	executable := liveExecutable(t)
	codexHome := t.TempDir()
	trustedForTest(t)

	legacyRoot := filepath.Join(t.TempDir(), "Stickguy")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "config.json"), []byte(`{"version":1,"deviceId":"dev_legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := Manager{ProjectRoot: project, ConfigRoot: legacyRoot, Executable: executable, CodexHome: codexHome}
	if _, err := legacy.Setup(); err != nil {
		t.Fatal(err)
	}

	current := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "Overgent"), Executable: executable, CodexHome: codexHome}
	status, err := current.Setup()
	if err != nil {
		t.Fatalf("setup refused an abandoned binding: %v", err)
	}
	if !status.Configured || status.Binding != "current" {
		t.Fatalf("adopted status=%#v", status)
	}
	after, err := current.Status()
	if err != nil || !after.Configured || after.Binding != "current" || after.Hooks != "active" {
		t.Fatalf("status after adoption=%#v err=%v", after, err)
	}
	hooks, err := os.ReadFile(filepath.Join(codexHome, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(hooks), legacyRoot) {
		t.Fatalf("legacy hook command survived adoption: %s", hooks)
	}
}

// Repair fixes what an earlier build left behind and nothing else: an agent
// that was never connected here stays unconnected, because connecting one is
// the member's decision.
func TestRepairAdoptsWithoutConnectingAnythingNew(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	executable := liveExecutable(t)
	codexHome := t.TempDir()
	trustedForTest(t)

	current := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "Overgent"), Executable: executable, CodexHome: codexHome}
	if _, err := current.Repair(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("repair created an MCP binding for an agent that was never connected")
	}
	if _, err := os.Stat(filepath.Join(codexHome, "hooks.json")); !os.IsNotExist(err) {
		t.Fatal("repair installed hooks for an agent that was never connected")
	}
}
