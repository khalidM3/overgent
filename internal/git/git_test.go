package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineToCurrentIncludesCommittedAndWorktreeStates(t *testing.T) {
	r := repo(t)
	base, _ := CaptureBaseline(context.Background(), Runner{}, r)
	write(t, r, "committed.txt", "x")
	gitcmd(t, r, "add", ".")
	gitcmd(t, r, "commit", "-m", "committed")
	gitcmd(t, r, "mv", "base.txt", "renamed.txt")
	if e := os.Remove(filepath.Join(r, "committed.txt")); e != nil {
		t.Fatal(e)
	}
	write(t, r, "unstaged.txt", "x")
	write(t, r, "staged.txt", "x")
	gitcmd(t, r, "add", "staged.txt")
	write(t, r, "staged.txt", "changed after staging")
	write(t, r, "untracked.txt", "x")
	write(t, r, "ignored.txt", "x")
	write(t, r, ".gitignore", "ignored.txt\n")
	m, e := Observe(context.Background(), Runner{}, r, base)
	if e != nil {
		t.Fatal(e)
	}
	names := map[string]States{}
	for _, v := range m.Entries {
		names[v.Path] = v.States
	}
	for _, n := range []string{"committed.txt", "renamed.txt", "unstaged.txt", "staged.txt", "untracked.txt", ".gitignore"} {
		if names[n] == (States{}) {
			t.Errorf("missing %s: %#v", n, m.Entries)
		}
	}
	if names["ignored.txt"] != (States{}) {
		t.Fatal("ignored path leaked")
	}
	if got := names["committed.txt"]; got.Baseline == nil || got.Baseline.Status != "added" || got.Worktree == nil || got.Worktree.Status != "deleted" {
		t.Fatalf("committed/worktree states were not preserved independently: %#v", got)
	}
	if got := names["staged.txt"]; got.Index == nil || got.Index.Status != "added" || got.Worktree == nil || got.Worktree.Status != "modified" {
		t.Fatalf("index/worktree states were not preserved independently: %#v", got)
	}
}
func TestRejectsSymlinkEscape(t *testing.T) {
	r := repo(t)
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(r, "escape")); e != nil {
		t.Fatal(e)
	}
	if _, e := normalize(r, "escape/secret.txt"); e == nil {
		t.Fatal("expected symlink escape rejection")
	}
	if _, e := normalize(r, string([]byte{0xff})); e == nil {
		t.Fatal("expected invalid UTF-8 rejection")
	}
	if _, e := normalize(r, strings.Repeat("a", 513)); e == nil {
		t.Fatal("expected oversized path rejection")
	}
}
func TestCommittedThousandPathsAndFingerprint(t *testing.T) {
	r := repo(t)
	base, _ := CaptureBaseline(context.Background(), Runner{}, r)
	for i := range 1000 {
		write(t, r, fmt.Sprintf("bulk/%04d.txt", i), "synthetic")
	}
	gitcmd(t, r, "add", ".")
	gitcmd(t, r, "commit", "-m", "bulk")
	m, e := Observe(context.Background(), Runner{}, r, base)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Entries) != 1000 {
		t.Fatalf("got %d", len(m.Entries))
	}
	hash, e := Hash(m.Entries)
	if e != nil || len(hash) != 64 {
		t.Fatal("expected sha256")
	}
}

func TestCanonicalCrossLanguageHashAndOrdering(t *testing.T) {
	entries := []Entry{
		{Path: "a.ts", States: States{Baseline: &Change{Status: "modified"}, Index: &Change{Status: "renamed", OldPath: "old-a.ts"}, Worktree: &Change{Status: "modified"}}},
		{Path: "z.ts", States: States{Worktree: &Change{Status: "untracked"}}},
	}
	got, e := Hash(entries)
	if e != nil || got != "cb3fc754d48edb8d7be868df86d249942d8832811e0af83fb2f24f022328ea4d" {
		t.Fatalf("cross-language hash=%q err=%v", got, e)
	}
	if _, e = Hash([]Entry{entries[1], entries[0]}); e == nil {
		t.Fatal("unordered paths accepted")
	}
	if _, e = Hash([]Entry{entries[0], entries[0]}); e == nil {
		t.Fatal("duplicate paths accepted")
	}
}

func TestFingerprintRemoteSemanticsAndLinkedWorktree(t *testing.T) {
	ctx := context.Background()
	withoutRemote := repo(t)
	if _, e := Fingerprint(ctx, Runner{}, withoutRemote, "prj_fixture"); e == nil || !strings.Contains(e.Error(), "found none") {
		t.Fatalf("no-remote outcome: %v", e)
	}
	r1 := repo(t)
	r2 := repo(t)
	gitcmd(t, r1, "remote", "add", "origin", "https://github.com/Stickguy/Fixture.git")
	gitcmd(t, r2, "remote", "add", "origin", "git@github.com:Stickguy/Fixture.git")
	f1, e := Fingerprint(ctx, Runner{}, r1, "prj_fixture")
	if e != nil {
		t.Fatal(e)
	}
	f2, e := Fingerprint(ctx, Runner{}, r2, "prj_fixture")
	if e != nil || f1 != f2 {
		t.Fatalf("cross-device remote identity mismatch: %q %q %v", f1, f2, e)
	}
	otherProject, e := Fingerprint(ctx, Runner{}, r2, "prj_other")
	if e != nil || otherProject == f2 {
		t.Fatal("explicit Project registration missing from fingerprint")
	}
	linked := filepath.Join(t.TempDir(), "linked")
	gitcmd(t, r2, "worktree", "add", "-q", "-b", "linked-fixture", linked)
	linkedFingerprint, e := Fingerprint(ctx, Runner{}, linked, "prj_fixture")
	if e != nil || linkedFingerprint != f2 {
		t.Fatalf("linked worktree fingerprint mismatch: %q %q %v", linkedFingerprint, f2, e)
	}
	mainCommon, _ := CommonDir(ctx, Runner{}, r2)
	linkedCommon, _ := CommonDir(ctx, Runner{}, linked)
	if mainCommon != linkedCommon {
		t.Fatalf("linked worktree local association mismatch: %q %q", mainCommon, linkedCommon)
	}
	gitcmd(t, r2, "remote", "add", "mirror", "https://example.invalid/other/repo.git")
	if _, e := Fingerprint(ctx, Runner{}, r2, "prj_fixture"); e == nil || !strings.Contains(e.Error(), "multiple distinct remotes") {
		t.Fatalf("multiple-remote outcome: %v", e)
	}
}

func TestSHA256ObjectFormatBaselineAndHead(t *testing.T) {
	r := filepath.Join(t.TempDir(), "sha256-repo")
	cmd := exec.Command("git", "init", "--object-format=sha256", "-q", r)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if b, e := cmd.CombinedOutput(); e != nil {
		message := strings.ToLower(string(b))
		if strings.Contains(message, "unknown option") || strings.Contains(message, "unsupported") || strings.Contains(message, "not support") {
			t.Skipf("installed Git explicitly lacks SHA-256 object format: %v: %s", e, strings.TrimSpace(string(b)))
		}
		t.Fatalf("initialize SHA-256 repository: %v: %s", e, b)
	}
	gitcmd(t, r, "config", "user.email", "fixture@example.invalid")
	gitcmd(t, r, "config", "user.name", "Fixture")
	write(t, r, "base.txt", "base")
	gitcmd(t, r, "add", ".")
	gitcmd(t, r, "commit", "-q", "-m", "base")
	baseline, e := CaptureBaseline(context.Background(), Runner{}, r)
	if e != nil || len(baseline) != 64 {
		t.Fatalf("SHA-256 baseline=%q err=%v", baseline, e)
	}
	write(t, r, "committed.txt", "fixture")
	gitcmd(t, r, "add", ".")
	gitcmd(t, r, "commit", "-q", "-m", "current")
	m, e := Observe(context.Background(), Runner{}, r, baseline)
	if e != nil || len(m.Head) != 64 || len(m.Entries) != 1 || m.Entries[0].States.Baseline == nil {
		t.Fatalf("SHA-256 observation: head=%q entries=%#v err=%v", m.Head, m.Entries, e)
	}
}
func repo(t *testing.T) string {
	t.Helper()
	r := t.TempDir()
	gitcmd(t, r, "init", "-q")
	gitcmd(t, r, "config", "user.email", "fixture@example.invalid")
	gitcmd(t, r, "config", "user.name", "Fixture")
	write(t, r, "base.txt", "base")
	gitcmd(t, r, "add", ".")
	gitcmd(t, r, "commit", "-q", "-m", "base")
	return r
}
func write(t *testing.T, r, p, v string) {
	t.Helper()
	p = filepath.Join(r, p)
	if e := os.MkdirAll(filepath.Dir(p), 0o700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(v), 0o600); e != nil {
		t.Fatal(e)
	}
}
func gitcmd(t *testing.T, r string, a ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", r}, a...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("git %v: %v %s", a, e, b)
	}
}
