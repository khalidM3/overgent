package watcher

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	w      *fsnotify.Watcher
	delay  time.Duration
	scan   func(context.Context, bool)
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func New(delay time.Duration, scan func(context.Context, bool)) (*Watcher, error) {
	w, e := fsnotify.NewWatcher()
	if e != nil {
		return nil, e
	}
	return &Watcher{w: w, delay: delay, scan: scan, timers: map[string]*time.Timer{}}, nil
}
func (w *Watcher) Add(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			if filepath.Base(p) == ".git" {
				return filepath.SkipDir
			}
			return w.w.Add(p)
		}
		return nil
	})
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
					_ = w.Add(ev.Name)
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
