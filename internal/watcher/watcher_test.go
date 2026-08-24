package watcher

import (
	"context"
	"os"
	"path/filepath"
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
	if e = w.Add(root); e != nil {
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
