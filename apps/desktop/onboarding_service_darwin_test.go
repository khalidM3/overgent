//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Agent install locations are the most platform-shaped thing in this package:
// two of the three vendors ship a CLI inside a .app bundle that is never on a
// GUI application's PATH.
func TestAgentDetectionCoversStandardMacAndNVMInstallLocations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	claude := filepath.Join(home, ".nvm", "versions", "node", "v20.19.0", "bin", "claude")
	if err := os.MkdirAll(filepath.Dir(claude), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claude, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, ok := agentExecutable("claude"); !ok || got != claude {
		t.Fatalf("Claude detection=(%q,%v)", got, ok)
	}
	codex := filepath.Join(home, "Applications", "ChatGPT.app", "Contents", "Resources", "codex")
	if err := os.MkdirAll(filepath.Dir(codex), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codex, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, ok := agentExecutable("codex"); !ok || got != codex {
		t.Fatalf("Codex detection=(%q,%v)", got, ok)
	}
}
