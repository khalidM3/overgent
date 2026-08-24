package hookconfig

import (
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
