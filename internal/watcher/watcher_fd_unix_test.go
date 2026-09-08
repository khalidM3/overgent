//go:build unix

package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

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
