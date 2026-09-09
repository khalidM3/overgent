package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khalidM3/overgent/internal/codexsetup"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/store"
)

func TestOnboardingStateReadsOnlyBoundedLocalMetadata(t *testing.T) {
	root := t.TempDir()
	repository := t.TempDir()
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Single("http://127.0.0.1:3211", "dev_test", []config.Workspace{{ID: "wsp_test", ProjectID: "prj_test", WorkstreamID: "wrk_test", Root: repository}})
	if err = config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	service := &OnboardingService{configRoot: root, apiBaseURL: "http://127.0.0.1:3211"}
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
	t.Setenv("OVERGENT_CODEX_EXECUTABLE", filepath.Join(t.TempDir(), "absent-codex"))
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
	if err = config.Save(paths, config.Single("https://example.convex.site", "dev_test", []config.Workspace{workspace})); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The other profile has to look alive, or the launch-time repair adopts it
	// without asking - which is the point of that pass and is covered by
	// TestLaunchRepairAdoptsALeftoverProfileWithoutAsking below.
	oldPaths, err := config.Resolve(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = config.Save(oldPaths, config.Single("https://example.convex.site", "dev_other", nil)); err != nil {
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
	if err = config.Save(paths, config.Single("https://example.convex.site", "dev_test", []config.Workspace{workspace})); err != nil {
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
		runGit(t, root, "config", "user.name", "Overgent Test")
		runGit(t, root, "config", "user.email", "overgent-test@example.invalid")
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

// The failure two beta testers hit on day one: a user-level Codex hook file left
// bound to a former product name made a fresh install on a fresh repository
// report Codex as belonging to somebody else, with the only working control a
// text link inside the adapter row. Opening the app now fixes it, and the
// adapter row never mentions it.
func TestLaunchRepairAdoptsALeftoverProfileWithoutAsking(t *testing.T) {
	isolateCodex(t)
	repository := t.TempDir()
	repository, _ = filepath.EvalSymlinks(repository)
	currentRoot := filepath.Join(t.TempDir(), "Overgent")
	legacyRoot := filepath.Join(t.TempDir(), "Stickguy")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyPaths, err := config.Resolve(legacyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = config.Save(legacyPaths, config.Single("https://example.convex.site", "dev_legacy", nil)); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (codexsetup.Manager{ProjectRoot: repository, ConfigRoot: legacyRoot, Executable: executable}).Setup(); err != nil {
		t.Fatal(err)
	}

	paths, err := config.Resolve(currentRoot)
	if err != nil {
		t.Fatal(err)
	}
	workspace := config.Workspace{ID: "wsp_test", ProjectID: "prj_test", WorkstreamID: "wrk_test", Root: repository}
	if err = config.Save(paths, config.Single("https://example.convex.site", "dev_test", []config.Workspace{workspace})); err != nil {
		t.Fatal(err)
	}

	service := &OnboardingService{configRoot: currentRoot, apiBaseURL: "https://example.convex.site", cliBinary: executable}
	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	codex := state.Adapters[0]
	if codex.Binding != "current" || !codex.Configured {
		t.Fatalf("opening the app did not adopt the leftover: %#v", codex)
	}
	if codex.ReconnectAllowed {
		t.Fatalf("a leftover was still presented as a decision: %#v", codex)
	}
}

// Deleting a Project on the server left this Mac still connected to it: the
// window listed it, its repository went on being watched and publishing, and
// re-connecting that repository was refused as already connected. The window
// only stopped listing it after a quit and relaunch, and the registration
// survived even that. This is the call that closes it, on the path a member
// with no running service actually takes.
func TestDisconnectProjectForgetsOneProjectAndFreesItsRepository(t *testing.T) {
	isolateCodex(t)
	root := t.TempDir()
	gone, kept := t.TempDir(), t.TempDir()
	gone, _ = filepath.EvalSymlinks(gone)
	kept, _ = filepath.EvalSymlinks(kept)
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Single("http://127.0.0.1:3211", "dev_test", []config.Workspace{
		{ID: "wsp_gone", ProjectID: "prj_gone", WorkstreamID: "wrk_gone", Root: gone},
		{ID: "wsp_kept", ProjectID: "prj_kept", WorkstreamID: "wrk_kept", Root: kept},
	})
	if err = config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	service := &OnboardingService{configRoot: root, apiBaseURL: "http://127.0.0.1:3211"}

	// No service is running here, so this is the stopped-service fallback: the
	// profile is edited directly rather than through IPC.
	state, err := service.DisconnectProject("prj_gone")
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range state.Projects {
		if project.ProjectID == "prj_gone" {
			t.Fatal("the disconnected Project is still in this Mac's list")
		}
	}
	if len(state.Projects) != 1 || state.Projects[0].ProjectID != "prj_kept" {
		t.Fatalf("projects = %+v", state.Projects)
	}

	// The repository being free again is the point: this is the state that made
	// "this repository is already connected to a Project" name a Project the
	// member had just deleted.
	if _, err = service.addProject(EnrollmentRequest{RepositoryRoot: gone, JoinCode: "inv_abc.secret"}, false, true); err != nil && strings.Contains(err.Error(), "already connected") {
		t.Fatalf("the disconnected repository is still held: %v", err)
	}
	if _, err = service.addProject(EnrollmentRequest{RepositoryRoot: kept, JoinCode: "inv_abc.secret"}, false, true); err == nil || !strings.Contains(err.Error(), "already connected") {
		t.Fatalf("a repository still connected to a Project must be refused: %v", err)
	}
	if _, err = service.DisconnectProject("prj_gone"); err == nil {
		t.Fatal("disconnecting a Project this Mac no longer holds must be refused")
	}
}
