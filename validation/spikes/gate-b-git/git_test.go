package gatebgit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

var testRunner Runner

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.name", "Gate B Fixture")
	git(t, root, "config", "user.email", "fixture.invalid@example.invalid")
	write(t, root, ".gitignore", "ignored/\n")
	write(t, root, "tracked.txt", "baseline\n")
	write(t, root, "rename-source.txt", "rename me without changing content\n")
	write(t, root, "delete-me.txt", "delete me\n")
	git(t, root, "add", ".gitignore", "tracked.txt", "rename-source.txt", "delete-me.txt")
	git(t, root, "commit", "-m", "synthetic baseline")
	return root
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := testRunner.Git(context.Background(), root, args...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func paths(entries []Entry) []string {
	out := make([]string, len(entries))
	for i := range entries {
		out[i] = entries[i].Path
	}
	return out
}

func findEntry(t *testing.T, entries []Entry, path string) Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Path == path {
			return entry
		}
	}
	t.Fatalf("entry %q not found in %#v", path, entries)
	return Entry{}
}

func TestManifest_AllWorktreeStatesAndIgnored(t *testing.T) {
	root := newRepo(t)
	baseline, err := CaptureBaseline(context.Background(), testRunner, root)
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "tracked.txt", "unstaged\n")
	write(t, root, "staged.txt", "staged\n")
	git(t, root, "add", "staged.txt")
	write(t, root, "untracked.txt", "untracked\n")
	git(t, root, "mv", "rename-source.txt", "renamed.txt")
	if err := os.Remove(filepath.Join(root, "delete-me.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "ignored/secret-shaped.txt", "must not be observed\n")

	manifest, err := Observe(context.Background(), testRunner, root, baseline)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"delete-me.txt", "renamed.txt", "staged.txt", "tracked.txt", "untracked.txt"}
	if got := paths(manifest.Entries); !reflect.DeepEqual(got, wantPaths) {
		t.Fatalf("paths = %#v, want %#v", got, wantPaths)
	}
	if got := findEntry(t, manifest.Entries, "renamed.txt"); got.Status != "renamed" || got.OldPath != "rename-source.txt" {
		t.Fatalf("rename entry = %#v", got)
	}
	if strings.Contains(strings.Join(paths(manifest.Entries), "\n"), "ignored") {
		t.Fatal("ignored path was observed")
	}
}

func TestManifest_LocalCommitAfterBaselineOnCleanWorktree(t *testing.T) {
	root := newRepo(t)
	baseline, _ := CaptureBaseline(context.Background(), testRunner, root)
	write(t, root, "local-only.txt", "content stays local\n")
	git(t, root, "add", "local-only.txt")
	git(t, root, "commit", "-m", "local commit after workstream baseline")
	if status := git(t, root, "status", "--porcelain"); status != "" {
		t.Fatalf("fixture worktree not clean: %q", status)
	}
	manifest, err := Observe(context.Background(), testRunner, root, baseline)
	if err != nil {
		t.Fatal(err)
	}
	entry := findEntry(t, manifest.Entries, "local-only.txt")
	if entry.Status != "added" || manifest.BaselineState != BaselineAncestor {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestBaseline_BranchSwitchDivergenceAndDetachedHead(t *testing.T) {
	root := newRepo(t)
	git(t, root, "switch", "-c", "agent-work")
	write(t, root, "agent.txt", "agent branch\n")
	git(t, root, "add", "agent.txt")
	git(t, root, "commit", "-m", "agent branch commit")
	baseline, _ := CaptureBaseline(context.Background(), testRunner, root)
	git(t, root, "switch", "main")
	write(t, root, "main.txt", "main branch\n")
	git(t, root, "add", "main.txt")
	git(t, root, "commit", "-m", "diverge main")
	state, err := ClassifyBaseline(context.Background(), testRunner, root, baseline)
	if err != nil || state != BaselineDiverged {
		t.Fatalf("state = %q, err = %v", state, err)
	}
	git(t, root, "checkout", "--detach", "HEAD")
	manifest, err := Observe(context.Background(), testRunner, root, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.BaselineState != BaselineDiverged || manifest.Head == "" {
		t.Fatalf("detached manifest = %#v", manifest)
	}
}

func TestBaseline_RebaseRewritesCapturedCommitAsDiverged(t *testing.T) {
	root := newRepo(t)
	git(t, root, "switch", "-c", "agent-work")
	write(t, root, "agent.txt", "pre-rebase agent commit\n")
	git(t, root, "add", "agent.txt")
	git(t, root, "commit", "-m", "agent work before rebase")
	baseline, _ := CaptureBaseline(context.Background(), testRunner, root)
	git(t, root, "switch", "main")
	write(t, root, "main.txt", "upstream commit\n")
	git(t, root, "add", "main.txt")
	git(t, root, "commit", "-m", "upstream advances")
	git(t, root, "switch", "agent-work")
	git(t, root, "rebase", "main")
	state, err := ClassifyBaseline(context.Background(), testRunner, root, baseline)
	if err != nil || state != BaselineDiverged {
		t.Fatalf("post-rebase baseline state = %q, err = %v", state, err)
	}
}

func TestRepositoryIdentity_NoRemoteMultipleRemoteAndWorktree(t *testing.T) {
	root := newRepo(t)
	id, err := IdentifyRepository(context.Background(), testRunner, root)
	if err != nil || id.Classification != "no_remote_requires_registration" {
		t.Fatalf("no-remote identity = %#v, err = %v", id, err)
	}
	git(t, root, "remote", "add", "origin", "https://user:token@example.com/Team/Repo.git?credential=forbidden")
	id, err = IdentifyRepository(context.Background(), testRunner, root)
	if err != nil || id.RemoteIdentity != "example.com/Team/Repo" || strings.Contains(id.RemoteIdentity, "token") {
		t.Fatalf("remote identity = %#v, err = %v", id, err)
	}
	git(t, root, "remote", "add", "mirror", "git@mirror.example:Team/Other.git")
	id, err = IdentifyRepository(context.Background(), testRunner, root)
	if err != nil || id.Classification != "multiple_distinct_remotes_require_registration" {
		t.Fatalf("multi-remote identity = %#v, err = %v", id, err)
	}
	worktree := filepath.Join(t.TempDir(), "linked")
	git(t, root, "worktree", "add", "--detach", worktree, "HEAD")
	linked, err := IdentifyRepository(context.Background(), testRunner, worktree)
	if err != nil || linked.CommonDir != id.CommonDir {
		t.Fatalf("linked identity = %#v, primary = %#v, err = %v", linked, id, err)
	}
}

func TestManifest_ThousandCommittedPathsChunkHashAtomicityAndResources(t *testing.T) {
	root := newRepo(t)
	baseline, _ := CaptureBaseline(context.Background(), testRunner, root)
	for i := range 1000 {
		write(t, root, fmt.Sprintf("generated/path-%04d.txt", i), "synthetic\n")
	}
	git(t, root, "add", "generated")
	git(t, root, "commit", "-m", "one thousand locally committed paths")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	started := time.Now()
	manifest, err := Observe(context.Background(), testRunner, root, baseline)
	elapsed := time.Since(started)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 1000 {
		t.Fatalf("entry count = %d, want 1000", len(manifest.Entries))
	}
	chunks := Chunk(manifest.Entries, 200)
	if len(chunks) != 5 {
		t.Fatalf("chunk count = %d", len(chunks))
	}
	assembler := NewAssembler()
	for i := 0; i < 4; i++ {
		assembler.Stage(2, i, chunks[i])
	}
	if err := assembler.Activate(2, 5, HashEntries(manifest.Entries)); err == nil {
		t.Fatal("incomplete revision activated")
	}
	if revision, entries := assembler.Active(); revision != 0 || entries != nil {
		t.Fatalf("partial revision became visible: revision=%d entries=%d", revision, len(entries))
	}
	assembler.Stage(2, 4, chunks[4])
	if err := assembler.Activate(2, 5, HashEntries(manifest.Entries)); err != nil {
		t.Fatal(err)
	}
	revision, active := assembler.Active()
	if revision != 2 || len(active) != 1000 {
		t.Fatalf("active revision=%d entries=%d", revision, len(active))
	}
	allocDelta := after.TotalAlloc - before.TotalAlloc
	t.Logf("resource_evidence manifest_paths=1000 chunks=5 content_hash=%s observe_elapsed=%s total_alloc_delta_bytes=%d", HashEntries(manifest.Entries), elapsed, allocDelta)
}

func TestPathNormalization_MaliciousNamesAndSymlinkEscape(t *testing.T) {
	root := newRepo(t)
	baseline, _ := CaptureBaseline(context.Background(), testRunner, root)
	malicious := []string{"-leading-option.txt", "line\nbreak.txt", "semi;colon.txt", "dollar$(touch nope).txt", "space name.txt"}
	for _, name := range malicious {
		write(t, root, name, "synthetic\n")
	}
	manifest, err := Observe(context.Background(), testRunner, root, baseline)
	if err != nil {
		t.Fatal(err)
	}
	got := paths(manifest.Entries)
	sort.Strings(malicious)
	if !reflect.DeepEqual(got, malicious) {
		t.Fatalf("malicious-name paths changed: got %#v want %#v", got, malicious)
	}
	if _, err := NormalizeObservedPath(root, "../escape"); err == nil {
		t.Fatal("parent escape accepted")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "outside-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeObservedPath(root, "outside-link"); err == nil {
		t.Fatal("symlink escape accepted")
	}
	if _, err := NormalizeObservedPath(root, "outside-link/not-created.txt"); err == nil {
		t.Fatal("symlink-parent escape accepted for nonexistent terminal")
	}
}

func TestBaseline_RejectsOptionLikeOrSymbolicRefs(t *testing.T) {
	root := newRepo(t)
	for _, malicious := range []string{"--output=/tmp/forbidden", "HEAD", "main;touch nope", strings.Repeat("a", 39)} {
		if _, err := ClassifyBaseline(context.Background(), testRunner, root, malicious); err == nil {
			t.Fatalf("baseline %q was accepted", malicious)
		}
	}
}

func TestWatcherCoalescingAndOverflow(t *testing.T) {
	events := make([]EventKind, 100)
	if got := Coalesce(events); got.Full || got.Reasons != 100 {
		t.Fatalf("rapid edit coalescing = %#v", got)
	}
	events = append(events, EventOverflow)
	if got := Coalesce(events); !got.Full || got.Reasons != 101 {
		t.Fatalf("overflow coalescing = %#v", got)
	}
}

func TestCanonicalFixtureShapeContainsNoContent(t *testing.T) {
	entries := []Entry{{Path: "new/name.txt", Status: "renamed", OldPath: "old/name.txt"}, {Path: "untracked.txt", Status: "untracked"}}
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"content", "diff", "blob", "patch"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("canonical fixture includes forbidden field %q: %s", forbidden, b)
		}
	}
}
