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
