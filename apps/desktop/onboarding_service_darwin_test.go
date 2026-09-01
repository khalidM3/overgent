//go:build darwin

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stickguy/stickguy/internal/codexsetup"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/store"
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
	// Named rather than counted: this assertion was left at 2 when the Cursor
	// adapter landed, and a bare count says nothing about which one went
	// missing when it next changes.
	var names []string
	for _, adapter := range state.Adapters {
		names = append(names, adapter.Name)
	}
	if !slices.Equal(names, []string{"Codex", "Claude Code", "Cursor"}) {
		t.Fatalf("adapters=%v", names)
	}
}

// isolateCodex redirects Codex state and Codex discovery. Codex hooks install
// at the user layer and trust repair spawns Codex, so a test without this
// writes into the contributor's real Codex configuration.
func isolateCodex(t *testing.T) {
	t.Helper()
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("STICKGUY_CODEX_EXECUTABLE", filepath.Join(t.TempDir(), "absent-codex"))
}

func TestOnboardingDetectsAndReconnectsAnotherManagedProfile(t *testing.T) {
	isolateCodex(t)
	sharedRoot, oldRoot, repository := t.TempDir(), t.TempDir(), t.TempDir()
	repository, _ = filepath.EvalSymlinks(repository)
	paths, err := config.Resolve(sharedRoot)
	if err != nil {
		t.Fatal(err)
	}
	workspace := config.Workspace{ID: "wsp_test", ProjectID: "prj_test", WorkstreamID: "wrk_test", Root: repository}
	if err = config.Save(paths, config.Config{Version: 1, APIBaseURL: "https://example.convex.site", DeviceID: "dev_test", Workspaces: []config.Workspace{workspace}}); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (codexsetup.Manager{ProjectRoot: repository, ConfigRoot: oldRoot, Executable: executable}).Setup(); err != nil {
		t.Fatal(err)
	}
	service := &OnboardingService{configRoot: sharedRoot, apiBaseURL: "https://example.convex.site", cliBinary: executable}
	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	codex := state.Adapters[0]
	if codex.Binding != "other_profile" || codex.Configured || !codex.ReconnectAllowed {
		t.Fatalf("other profile was not actionable: %#v", codex)
	}
	if _, err = service.ReconnectAdapter(repository, "codex"); err != nil {
		t.Fatal(err)
	}
	state, err = service.State()
	if err != nil {
		t.Fatal(err)
	}
	codex = state.Adapters[0]
	if !codex.Configured || codex.Binding != "current" || !codex.RestartRequired || codex.RuntimeVerified {
		t.Fatalf("reconnected state is dishonest: %#v", codex)
	}
}

func TestOnboardingRequiresLiveEventBeforeAdapterReady(t *testing.T) {
	isolateCodex(t)
	root, repository := t.TempDir(), t.TempDir()
	repository, _ = filepath.EvalSymlinks(repository)
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace := config.Workspace{ID: "wsp_test", ProjectID: "prj_test", WorkstreamID: "wrk_test", Root: repository}
	if err = config.Save(paths, config.Config{Version: 1, APIBaseURL: "https://example.convex.site", DeviceID: "dev_test", Workspaces: []config.Workspace{workspace}}); err != nil {
		t.Fatal(err)
	}
	executable, _ := os.Executable()
	if _, err = (codexsetup.Manager{ProjectRoot: repository, ConfigRoot: root, Executable: executable}).Setup(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(paths.DB)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.RecordAgentObservation(context.Background(), workspace.ID, "codex", time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	service := &OnboardingService{configRoot: root, apiBaseURL: "https://example.convex.site", cliBinary: executable}
	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Adapters[0].RuntimeVerified || state.Adapters[0].RestartRequired {
		t.Fatalf("live event did not verify adapter: %#v", state.Adapters[0])
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
