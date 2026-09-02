package watcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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

// openDescriptors counts this process's open descriptors by probing each
// number with fcntl. Reading /dev/fd instead undercounts on macOS and would
// leave the leak assertions vacuous.
func openDescriptors(t *testing.T) int {
	t.Helper()
	n := 0
	for fd := 0; fd < 16384; fd++ {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
			n++
		}
	}
	return n
}

// A watched directory pins descriptors for itself and, on kqueue platforms,
// every file inside it until the watch ends. Ignored trees must therefore
// never be watched at all: an unpruned node_modules once pinned ~46k
// descriptors and exhausted the system file table (2026-09-01).
func TestAddPrunesIgnoredTreesAndReleasesNoDescriptorsToThem(t *testing.T) {
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "src"), 0o700); e != nil {
		t.Fatal(e)
	}
	for d := range 20 {
		dir := filepath.Join(root, "junk", fmt.Sprintf("dep%02d", d))
		if e := os.MkdirAll(dir, 0o700); e != nil {
			t.Fatal(e)
		}
		for f := range 20 {
			if e := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d", f)), []byte("x"), 0o600); e != nil {
				t.Fatal(e)
			}
		}
	}
	before := openDescriptors(t)
	w, e := New(time.Hour, func(context.Context, bool) {})
	if e != nil {
		t.Fatal(e)
	}
	defer w.w.Close()
	if e = w.Add(root, fakeIgnorer{names: map[string]bool{"junk": true}}); e != nil {
		t.Fatal(e)
	}
	if delta := openDescriptors(t) - before; delta > 50 {
		t.Fatalf("watching a pruned tree pinned %d descriptors; the ignored tree is being watched", delta)
	}
	list := w.w.WatchList()
	if !slices.Contains(list, filepath.Join(root, "src")) {
		t.Fatalf("source directory not watched: %v", list)
	}
	for _, p := range list {
		if strings.Contains(p, "junk") {
			t.Fatalf("ignored directory watched: %s", p)
		}
	}
}

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
