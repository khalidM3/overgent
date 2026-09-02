package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoresSnapshotPrunesIgnoredTreesAndFiles(t *testing.T) {
	r := repo(t)
	write(t, r, ".gitignore", "node_modules/\n*.log\ndist/\n")
	write(t, r, "node_modules/pkg/a.js", "x")
	write(t, r, "debug.log", "x")
	write(t, r, "src/main.go", "package main")
	ig, e := NewIgnores(context.Background(), Runner{}, r)
	if e != nil {
		t.Fatal(e)
	}
	for path, want := range map[string]bool{
		filepath.Join(r, "node_modules"):        true,
		filepath.Join(r, "node_modules", "pkg"): true,
		filepath.Join(r, "debug.log"):           true,
		filepath.Join(r, "src"):                 false,
		filepath.Join(r, "base.txt"):            false,
		r:                                       false,
		filepath.Join(t.TempDir(), "outside"):   false,
	} {
		if got := ig.Prune(path); got != want {
			t.Errorf("Prune(%s)=%v want %v", path, got, want)
		}
	}
}

func TestIgnoresResolvesPathsCreatedAfterSnapshot(t *testing.T) {
	r := repo(t)
	write(t, r, ".gitignore", "dist/\n")
	ig, e := NewIgnores(context.Background(), Runner{}, r)
	if e != nil {
		t.Fatal(e)
	}
	dist := filepath.Join(r, "dist")
	if e = os.Mkdir(dist, 0o700); e != nil {
		t.Fatal(e)
	}
	src := filepath.Join(r, "src")
	if e = os.Mkdir(src, 0o700); e != nil {
		t.Fatal(e)
	}
	if ig.Prune(dist) {
		t.Fatal("snapshot cannot know a directory created after it")
	}
	if !ig.Ignored(context.Background(), dist) {
		t.Fatal("check-ignore fallback missed a freshly created ignored directory")
	}
	if ig.Ignored(context.Background(), src) {
		t.Fatal("source directory misreported as ignored")
	}
	// Second lookups answer from cache; equal verdicts prove the cache holds
	// the same truth rather than a stale or inverted one.
	if !ig.Ignored(context.Background(), dist) || ig.Ignored(context.Background(), src) {
		t.Fatal("cached verdicts diverged from first sight")
	}
}
