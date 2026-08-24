package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	git "github.com/stickguy/stickguy/internal/git"
	"github.com/stickguy/stickguy/internal/store"
	"github.com/stickguy/stickguy/internal/watcher"
)

type Sender interface {
	Send(context.Context, []store.QueueEvent) error
}
type Service struct {
	paths       config.Paths
	store       *store.Store
	cfg         config.Config
	sender      Sender
	mu          sync.Mutex
	scans, boot int64
}

func Run(ctx context.Context, root string, sender Sender) error {
	paths, e := config.Resolve(root)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(paths.Root, 0o700); e != nil {
		return fmt.Errorf("create service state directory: %w", e)
	}
	if e = os.Chmod(paths.Root, 0o700); e != nil {
		return fmt.Errorf("secure service state directory: %w", e)
	}
	lock, e := daemon.Acquire(paths.Lock)
	if e != nil {
		return e
	}
	defer lock.Close()
	cfg, e := config.Load(paths)
	if e != nil {
		return e
	}
	sdb, e := store.Open(paths.DB)
	if e != nil {
		return e
	}
	defer sdb.Close()
	s := &Service{paths: paths, store: sdb, cfg: cfg, sender: sender}
	s.boot, _ = sdb.Boot(ctx)
	for _, w := range cfg.Workspaces {
		if e = sdb.UpsertWorkspace(ctx, store.Workspace{ID: w.ID, ProjectID: w.ProjectID, WorkstreamID: w.WorkstreamID, Root: w.Root, Baseline: w.Baseline}); e != nil {
			return e
		}
	}
	watch, e := watcher.New(250*time.Millisecond, func(c context.Context, _ bool) { s.scanAll(c) })
	if e != nil {
		return e
	}
	for _, w := range cfg.Workspaces {
		if e = watch.Add(w.Root); e != nil {
			return fmt.Errorf("watch %s: %w", w.ID, e)
		}
	}
	go watch.Run(ctx)
	s.scanAll(ctx)
	go s.flushLoop(ctx)
	return daemon.Serve(ctx, paths.Socket, s.handle)
}
func (s *Service) scanAll(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.cfg.Workspaces {
		m, e := git.Observe(ctx, git.Runner{}, w.Root, w.Baseline)
		if e != nil {
			continue
		}
		_, h, b, e := s.store.ActiveManifest(ctx, w.ID)
		if e == nil {
			var old []git.Entry
			_ = json.Unmarshal(b, &old)
			if h == git.Hash(m.Entries) {
				continue
			}
		} else if e != sql.ErrNoRows {
			continue
		}
		_, _ = s.store.PublishManifest(ctx, w.ID, git.Hash(m.Entries), m.Entries, "evt_"+uuid.NewString())
		s.scans++
	}
}
func (s *Service) flushLoop(ctx context.Context) {
	if s.sender == nil {
		return
	}
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.flush(ctx)
		}
	}
}
func (s *Service) flush(ctx context.Context) {
	pending, _ := s.store.Pending(ctx)
	if len(pending) == 0 {
		return
	}
	ws, _ := s.store.Workspaces(ctx)
	paused := map[string]bool{}
	for _, w := range ws {
		paused[w.ID] = w.Paused
	}
	var send []store.QueueEvent
	for _, e := range pending {
		if !paused[e.WorkspaceID] {
			send = append(send, e)
		}
	}
	if len(send) == 0 {
		return
	}
	if s.sender.Send(ctx, send) == nil {
		for _, e := range send {
			_ = s.store.Ack(ctx, e.ID)
		}
	}
}
func (s *Service) handle(ctx context.Context, q daemon.Request) daemon.Response {
	switch q.Method {
	case "health", "doctor":
		w, _ := s.store.Workspaces(ctx)
		p, _ := s.store.Pending(ctx)
		return daemon.Response{OK: true, Data: map[string]any{"status": "ok", "bootCount": s.boot, "workspaces": len(w), "pending": len(p), "scans": s.scans, "pid": os.Getpid()}}
	case "pause", "resume":
		if e := s.store.SetPaused(ctx, q.WorkspaceID, q.Method == "pause"); e != nil {
			return daemon.Response{Error: e.Error()}
		}
		return daemon.Response{OK: true}
	case "scan":
		s.scanAll(ctx)
		return daemon.Response{OK: true}
	default:
		return daemon.Response{Error: "unsupported method"}
	}
}
func Register(ctx context.Context, root string, w config.Workspace) error {
	paths, e := config.Resolve(root)
	if e != nil {
		return e
	}
	lock, e := daemon.Acquire(paths.Lock)
	if e != nil {
		return fmt.Errorf("register workspace: %w", e)
	}
	defer lock.Close()
	cfg, e := config.Load(paths)
	if e != nil {
		return e
	}
	for _, v := range cfg.Workspaces {
		if v.ID == w.ID || v.Root == w.Root {
			return fmt.Errorf("workspace already registered")
		}
	}
	baseline, e := git.CaptureBaseline(ctx, git.Runner{}, w.Root)
	if e != nil {
		return e
	}
	w.Baseline = baseline
	cfg.Workspaces = append(cfg.Workspaces, w)
	return config.Save(paths, cfg)
}
