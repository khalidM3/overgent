package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestDebounceAndExplicitFullRescan(t *testing.T) {
	root := t.TempDir()
	var n atomic.Int64
	w, e := New(80*time.Millisecond, func(context.Context, bool) { n.Add(1) })
	if e != nil {
		t.Fatal(e)
	}
	if e = w.Add(root, nil); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	for i := range 20 {
		p := filepath.Join(root, "f")
		if e = os.WriteFile(p, []byte{byte(i)}, 0o600); e != nil {
			t.Fatal(e)
		}
	}
	time.Sleep(250 * time.Millisecond)
	if got := n.Load(); got != 1 {
		t.Fatalf("debounced scans=%d", got)
	}
	w.Rescan(ctx)
	if n.Load() != 2 {
		t.Fatal("full rescan not invoked")
	}
}

// fakeIgnorer ignores every path whose base name matches one of names.
type fakeIgnorer struct{ names map[string]bool }

func (f fakeIgnorer) Prune(p string) bool                      { return f.names[filepath.Base(p)] }
func (f fakeIgnorer) Ignored(_ context.Context, p string) bool { return f.names[filepath.Base(p)] }

func TestDirectoryCreatedAtRuntimeIsVettedBeforeWatching(t *testing.T) {
	root := t.TempDir()
	w, e := New(time.Hour, func(context.Context, bool) {})
	if e != nil {
		t.Fatal(e)
	}
	if e = w.Add(root, fakeIgnorer{names: map[string]bool{"junk": true}}); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	if e = os.Mkdir(filepath.Join(root, "junk"), 0o700); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(filepath.Join(root, "keep"), 0o700); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !slices.Contains(w.w.WatchList(), filepath.Join(root, "keep")) {
		if time.Now().After(deadline) {
			t.Fatalf("new source directory never watched: %v", w.w.WatchList())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if slices.Contains(w.w.WatchList(), filepath.Join(root, "junk")) {
		t.Fatal("ignored directory created at runtime was watched")
	}
}

func TestWatchBudgetStopsRegistrationInsteadOfExhaustingDescriptors(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a", "b", "c", "d"} {
		if e := os.Mkdir(filepath.Join(root, d), 0o700); e != nil {
			t.Fatal(e)
		}
	}
	w, e := New(time.Hour, func(context.Context, bool) {})
	if e != nil {
		t.Fatal(e)
	}
	defer w.w.Close()
	w.maxDirs = 2
	if e = w.Add(root, nil); !errors.Is(e, ErrWatchBudget) {
		t.Fatalf("expected watch budget error, got %v", e)
	}
	if got := len(w.w.WatchList()); got != 2 {
		t.Fatalf("watched %d directories under a budget of 2", got)
	}
}

// The Watcher only ever grew before Remove existed, so a repository
// disconnected while the service was running kept a descriptor on every
// directory in it — and kept waking the scanner on edits to a repository
// Overgent no longer coordinates — until the service was restarted. Releasing
// one root must leave the root beside it whole, and must return the budget it
// was holding so a later Add can use it.
func TestRemoveReleasesOneRootAndReturnsItsBudget(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for _, dir := range []string{filepath.Join(first, "src"), filepath.Join(second, "src")} {
		if e := os.Mkdir(dir, 0o700); e != nil {
			t.Fatal(e)
		}
	}
	w, e := New(time.Hour, func(context.Context, bool) {})
	if e != nil {
		t.Fatal(e)
	}
	defer w.w.Close()
	if e = w.Add(first, nil); e != nil {
		t.Fatal(e)
	}
	if e = w.Add(second, nil); e != nil {
		t.Fatal(e)
	}
	watched := w.watched
	if released := w.Remove(first); released != 2 {
		t.Fatalf("released %d directories, want the root and its one child", released)
	}
	if w.watched != watched-2 {
		t.Fatalf("watched budget = %d, want %d", w.watched, watched-2)
	}
	for _, path := range w.w.WatchList() {
		if path == first || filepath.Dir(path) == first {
			t.Fatalf("%s is still watched after its root was removed", path)
		}
	}
	if len(w.w.WatchList()) != 2 {
		t.Fatalf("the root beside it lost watches: %v", w.w.WatchList())
	}
	if w.ignorerFor(filepath.Join(first, "src")) != nil {
		t.Fatal("a removed root still claims paths under it")
	}
	if released := w.Remove(first); released != 0 {
		t.Fatalf("removing an already-removed root released %d", released)
	}
}
