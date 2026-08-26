//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stickguy/stickguy/internal/config"
)

func TestOnboardingStateReadsOnlyBoundedLocalMetadata(t *testing.T) {
	root := t.TempDir()
	repository := t.TempDir()
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Version: 1, APIBaseURL: "http://127.0.0.1:3211", DeviceID: "dev_test", Workspaces: []config.Workspace{{ID: "wsp_test", ProjectID: "prj_test", WorkstreamID: "wrk_test", Root: repository}}}
	if err = config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	service := &OnboardingService{configRoot: root, apiBaseURL: cfg.APIBaseURL, activationBaseURL: "http://127.0.0.1:5173/api"}
	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enrolled || state.ProjectID != "prj_test" || state.RepositoryRoot != repository || state.RepositoryLabel != filepath.Base(repository) {
		t.Fatalf("unexpected bounded state: %+v", state)
	}
	if len(state.Adapters) != 2 {
		t.Fatalf("adapter count=%d", len(state.Adapters))
	}
}

func TestLinkedWorktreeValidationUsesGitIdentityWithoutMutation(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	linked := filepath.Join(t.TempDir(), "linked")
	unrelated := filepath.Join(t.TempDir(), "unrelated")
	for _, root := range []string{repository, unrelated} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "init", "-q")
		runGit(t, root, "config", "user.name", "Stickguy Test")
		runGit(t, root, "config", "user.email", "stickguy-test@example.invalid")
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("synthetic\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", "README.md")
		runGit(t, root, "commit", "-qm", "fixture")
	}
	runGit(t, repository, "worktree", "add", "-q", "-b", "fixture-linked", linked)
	if err := requireLinkedWorktree(repository, linked); err != nil {
		t.Fatalf("linked worktree rejected: %v", err)
	}
	if err := requireLinkedWorktree(repository, unrelated); err == nil {
		t.Fatal("unrelated repository accepted")
	}
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

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
