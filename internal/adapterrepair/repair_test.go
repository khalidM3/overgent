package adapterrepair

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/codexsetup"
)

// The exact shape the Stickguy rename left on every member's Mac: a user-level
// Codex hook file bound to a former product name, which made a brand-new
// Project on a brand-new repository report that Codex belonged to somebody else.
func TestRunAdoptsALeftoverFromAFormerProductName(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "overgent")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	codexHome := t.TempDir()
	legacyRoot := filepath.Join(t.TempDir(), "Stickguy")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "config.json"), []byte(`{"version":1,"deviceId":"dev_legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := codexsetup.Manager{ProjectRoot: project, ConfigRoot: legacyRoot, Executable: executable, CodexHome: codexHome}
	if _, err := legacy.Setup(); err != nil {
		t.Fatal(err)
	}

	current := filepath.Join(t.TempDir(), "Overgent")
	t.Setenv("CODEX_HOME", codexHome)
	outcomes := Run(current, executable, []string{project})

	adopted := false
	for _, outcome := range outcomes {
		if outcome.Err != nil {
			t.Fatalf("repair reported an error: %v", outcome)
		}
		if outcome.Vendor == "codex" && outcome.Adopted {
			adopted = true
		}
	}
	if !adopted {
		t.Fatalf("codex leftover was not adopted: %#v", outcomes)
	}
	hooks, err := os.ReadFile(filepath.Join(codexHome, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(hooks), legacyRoot) || !strings.Contains(string(hooks), current) {
		t.Fatalf("hooks were not moved onto this profile: %s", hooks)
	}
}

// A repository where nobody connected anything is left exactly as it was, and
// the pass reports nothing at all so the interface has nothing to say about it.
func TestRunIsSilentAndCreatesNothingWhenThereIsNothingToRepair(t *testing.T) {
	project := t.TempDir()
	executable := filepath.Join(t.TempDir(), "overgent")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", t.TempDir())
	if outcomes := Run(filepath.Join(t.TempDir(), "Overgent"), executable, []string{project}); len(outcomes) != 0 {
		t.Fatalf("a clean repository produced outcomes: %#v", outcomes)
	}
	for _, path := range []string{".codex/config.toml", ".mcp.json", ".claude/settings.local.json", ".cursor/hooks.json"} {
		if _, err := os.Stat(filepath.Join(project, path)); !os.IsNotExist(err) {
			t.Fatalf("repair created %s for an agent that was never connected", path)
		}
	}
}
