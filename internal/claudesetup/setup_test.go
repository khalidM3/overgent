package claudesetup

import (
	"encoding/json"
	"errors"
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
	manager := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "state"), Executable: "/usr/local/bin/overgent"}
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
	servers["overgent"].(map[string]any)["command"] = "/drifted"
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
	if _, exists := servers["overgent"]; exists || servers["fixture"] == nil || document["other"] == nil {
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
	if strings.Contains(string(managed), manager.ConfigRoot) || !strings.Contains(string(managed), `"command": "overgent"`) || !strings.Contains(string(managed), `"mcp"`) {
		t.Fatalf("portable config contains machine state or lacks PATH command: %s", managed)
	}
	if _, err := portable.Remove(); err != nil {
		t.Fatal(err)
	}
}

func TestOtherProfileRequiresExplicitRebindAndPreservesJSON(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"other":{"preserved":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// The other profile has to look alive for this test to be about anything: a
	// binary that still exists and a profile holding an enrolled device. A
	// binding nothing is using any more is adopted without asking, which
	// TestAbandonedBindingIsAdoptedWithoutAsking covers.
	executable := liveExecutable(t)
	oldProfile := Manager{ProjectRoot: project, ConfigRoot: liveProfile(t), Executable: executable}
	newProfile := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "shared"), Executable: executable}
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
	hookPath := filepath.Join(project, ".claude", "settings.local.json")
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
		t.Fatal("failed reconnect did not restore both Claude files")
	}
	if _, err = newProfile.Rebind(); err != nil {
		t.Fatal(err)
	}
	status, err = newProfile.Status()
	if err != nil || !status.Configured || status.Binding != "current" {
		t.Fatalf("rebound status=%#v err=%v", status, err)
	}
	data, _ := os.ReadFile(path)
	var document map[string]any
	if err = json.Unmarshal(data, &document); err != nil || document["other"].(map[string]any)["preserved"] != true || strings.Contains(string(data), oldProfile.ConfigRoot) || !strings.Contains(string(data), newProfile.ConfigRoot) {
		t.Fatalf("rebind damaged unrelated JSON or retained old profile: %s err=%v", data, err)
	}
}

// B1: a development install checked without --config-root is compared against
// the portable binding a released install uses, so it correctly reports
// other_profile — while naming the member's own profile, which reads as an
// alarm. The reply has to say which profile it compared against, or the honest
// answer is indistinguishable from a real cross-profile conflict.
func TestStatusNamesTheProfileItComparedAgainst(t *testing.T) {
	project := t.TempDir()
	executable := filepath.Join(t.TempDir(), "overgent")
	profile := filepath.Join(t.TempDir(), "profile")
	installed := Manager{ProjectRoot: project, ConfigRoot: profile, Executable: executable}
	if _, err := installed.Setup(); err != nil {
		t.Fatal(err)
	}
	if status, err := installed.Status(); err != nil || status.CheckedProfile != profile {
		t.Fatalf("status=%#v err=%v, want checkedProfile %q", status, err, profile)
	}

	// The same machine, asked the way the released install would be asked.
	portable := Manager{ProjectRoot: project, ConfigRoot: profile, Executable: executable, Portable: true}
	status, err := portable.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Binding != "other_profile" {
		t.Fatalf("binding=%q, want other_profile", status.Binding)
	}
	if status.CheckedProfile != "portable" {
		t.Fatalf("checkedProfile=%q, want portable", status.CheckedProfile)
	}
	// Both halves of the explanation must be present: what was asked about, and
	// what is actually bound.
	if status.PreviousProfile != profile {
		t.Fatalf("previousProfile=%q, want %q", status.PreviousProfile, profile)
	}
}

func liveExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overgent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

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

// A binding left behind by an earlier Overgent - here a former product name -
// is adopted by an ordinary Setup, with no reconnect button and no decision
// presented to anyone.
func TestAbandonedBindingIsAdoptedWithoutAsking(t *testing.T) {
	project := t.TempDir()
	executable := liveExecutable(t)
	legacyRoot := filepath.Join(t.TempDir(), "Stickguy")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "config.json"), []byte(`{"version":1,"deviceId":"dev_legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := Manager{ProjectRoot: project, ConfigRoot: legacyRoot, Executable: executable}
	if _, err := legacy.Setup(); err != nil {
		t.Fatal(err)
	}
	current := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "Overgent"), Executable: executable}
	status, err := current.Setup()
	if err != nil {
		t.Fatalf("setup refused an abandoned binding: %v", err)
	}
	if !status.Configured || status.Binding != "current" {
		t.Fatalf("adopted status=%#v", status)
	}
	after, err := current.Status()
	if err != nil || !after.Configured || after.Binding != "current" {
		t.Fatalf("status after adoption=%#v err=%v", after, err)
	}
	settings, err := os.ReadFile(filepath.Join(project, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(settings), legacyRoot) {
		t.Fatalf("legacy hook command survived adoption: %s", settings)
	}
}

// Repair fixes what an earlier build left behind and nothing else.
func TestRepairAdoptsWithoutConnectingAnythingNew(t *testing.T) {
	project := t.TempDir()
	current := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "Overgent"), Executable: liveExecutable(t)}
	if _, err := current.Repair(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("repair created an MCP binding for an agent that was never connected")
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "settings.local.json")); !os.IsNotExist(err) {
		t.Fatal("repair installed hooks for an agent that was never connected")
	}
}
