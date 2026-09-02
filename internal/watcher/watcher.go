package watcher

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Ignorer prunes paths from observation. Prune must be cheap — it is asked
// once per directory of a full tree walk. Ignored may consult the repository
// and is used only for paths that appear while the watcher is running.
type Ignorer interface {
	Prune(path string) bool
	Ignored(ctx context.Context, path string) bool
}

// maxWatchedDirectories caps how many directories one Watcher will register.
// On macOS every watched directory pins an open file descriptor — and the
// kqueue backend pins one more per file inside it — for the life of the watch,
// so an unbounded watch over a pathological tree exhausts the system file
// table for every process on the machine. Hitting the cap degrades to partial
// observation of the deepest paths instead.
const maxWatchedDirectories = 4096

// ErrWatchBudget reports that the directory cap was reached. Directories
// registered before the cap keep working.
var ErrWatchBudget = errors.New("watch budget exhausted; observing a partial tree")

type Watcher struct {
	w      *fsnotify.Watcher
	delay  time.Duration
	scan   func(context.Context, bool)
	mu     sync.Mutex
	timers map[string]*time.Timer
	// roots pairs each watched root with its pruning rules so directories
	// created later are vetted with the rules of the workspace they belong to.
	roots   map[string]Ignorer
	watched int
	maxDirs int
}

func New(delay time.Duration, scan func(context.Context, bool)) (*Watcher, error) {
	w, e := fsnotify.NewWatcher()
	if e != nil {
		return nil, e
	}
	return &Watcher{w: w, delay: delay, scan: scan, timers: map[string]*time.Timer{}, roots: map[string]Ignorer{}, maxDirs: maxWatchedDirectories}, nil
}

// Add watches root and its non-ignored descendants. ignore may be nil, which
// watches everything except .git; real workspaces must pass their repository's
// rules or ignored build trees pin a descriptor per file until the service
// exits.
func (w *Watcher) Add(root string, ignore Ignorer) error {
	w.mu.Lock()
	w.roots[root] = ignore
	w.mu.Unlock()
	prune := func(string) bool { return false }
	if ignore != nil {
		prune = ignore.Prune
	}
	return w.addTree(root, prune)
}

func (w *Watcher) addTree(dir string, prune func(string) bool) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			return nil
		}
		if filepath.Base(p) == ".git" || prune(p) {
			return filepath.SkipDir
		}
		w.mu.Lock()
		if w.watched >= w.maxDirs {
			w.mu.Unlock()
			return ErrWatchBudget
		}
		w.watched++
		w.mu.Unlock()
		return w.w.Add(p)
	})
}

// addCreated brings a directory that appeared at runtime under watch, unless
// the workspace it belongs to ignores it. A freshly created ignored tree (an
// install materializing node_modules) is skipped after a single check on its
// top directory, before anything below it is walked.
func (w *Watcher) addCreated(ctx context.Context, dir string) {
	ignore := w.ignorerFor(dir)
	if ignore == nil {
		_ = w.addTree(dir, func(string) bool { return false })
		return
	}
	if ignore.Ignored(ctx, dir) {
		return
	}
	_ = w.addTree(dir, func(p string) bool { return p != dir && ignore.Ignored(ctx, p) })
}

// ignorerFor resolves the rules of the innermost registered root containing
// path, so nested development workspaces answer with their own repository.
func (w *Watcher) ignorerFor(path string) Ignorer {
	w.mu.Lock()
	defer w.mu.Unlock()
	roots := make([]string, 0, len(w.roots))
	for r := range w.roots {
		roots = append(roots, r)
	}
	sort.Slice(roots, func(i, j int) bool { return len(roots[i]) > len(roots[j]) })
	for _, r := range roots {
		if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
			return w.roots[r]
		}
	}
	return nil
}

func (w *Watcher) Run(ctx context.Context) error {
	defer w.w.Close()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-w.w.Events:
			if ev.Op&fsnotify.Create != 0 {
				if st, e := os.Stat(ev.Name); e == nil && st.IsDir() {
					w.addCreated(ctx, ev.Name)
				}
			}
			w.schedule(ctx, ev.Op&fsnotify.Create != 0)
		case e := <-w.w.Errors:
			if e != nil {
				w.schedule(ctx, true)
			}
		}
	}
}
func (w *Watcher) schedule(ctx context.Context, full bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t := w.timers["all"]; t != nil {
		t.Stop()
	}
	w.timers["all"] = time.AfterFunc(w.delay, func() { w.scan(ctx, full) })
}
func (w *Watcher) Rescan(ctx context.Context) { w.scan(ctx, true) }
