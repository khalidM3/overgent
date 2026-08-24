package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	if len(Hash(m.Entries)) != 64 {
		t.Fatal("expected sha256")
	}
	a, _ := Fingerprint(context.Background(), Runner{}, r, "prj_a")
	b, _ := Fingerprint(context.Background(), Runner{}, r, "prj_b")
	if a == b {
		t.Fatal("registration scope missing")
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
