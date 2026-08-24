package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	Send(context.Context, string, []byte) error
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
		if e = sdb.UpsertWorkspace(ctx, store.Workspace{ID: w.ID, ProjectID: w.ProjectID, WorkstreamID: w.WorkstreamID, MemberID: w.MemberID, DeviceID: cfg.DeviceID, SessionID: w.SessionID, Root: w.Root, Baseline: w.Baseline, Fingerprint: w.Fingerprint}); e != nil {
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
		contentHash, e := git.Hash(m.Entries)
		if e != nil {
			continue
		}
		_, h, _, e := s.store.ActiveManifest(ctx, w.ID)
		if e == nil {
			if h == contentHash {
				continue
			}
		} else if e != sql.ErrNoRows {
			continue
		}
		if _, e = s.store.PublishManifest(ctx, store.ManifestPublication{WorkspaceID: w.ID, ManifestID: newID("mft_"), Baseline: m.Baseline, Head: m.Head, Hash: contentHash, Entries: m.Entries, EventID: newID("evt_")}); e == nil {
			s.scans++
		}
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
	groups := map[string][]store.QueueEvent{}
	var order []string
	for _, e := range pending {
		if !paused[e.WorkspaceID] {
			if groups[e.WorkspaceID] == nil {
				order = append(order, e.WorkspaceID)
			}
			groups[e.WorkspaceID] = append(groups[e.WorkspaceID], e)
		}
	}
	for _, workspaceID := range order {
		events := groups[workspaceID]
		for len(events) > 0 {
			n := min(100, len(events))
			window := events[:n]
			batch, e := store.Batch(window)
			if e != nil || s.sender.Send(ctx, workspaceID, batch) != nil {
				break
			}
			for _, event := range window {
				_ = s.store.Ack(ctx, event.ID)
			}
			events = events[n:]
		}
	}
}

func newID(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")
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
func Register(ctx context.Context, root, deviceID string, w config.Workspace) error {
	for label, value := range map[string]struct {
		value, pattern string
	}{
		"Project":    {w.ProjectID, `^prj_[a-z0-9_]{1,80}$`},
		"workspace":  {w.ID, `^wsp_[a-z0-9_]{1,123}$`},
		"workstream": {w.WorkstreamID, `^wrk_[a-z0-9_]{1,80}$`},
		"member":     {w.MemberID, `^mem_[a-z0-9_]{1,123}$`},
		"device":     {deviceID, `^dev_[a-z0-9_]{1,123}$`},
		"session":    {w.SessionID, `^ses_[a-z0-9_]{1,123}$`},
	} {
		if !regexp.MustCompile(value.pattern).MatchString(value.value) {
			return fmt.Errorf("invalid %s ID", label)
		}
	}
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
	if cfg.DeviceID != "" && cfg.DeviceID != deviceID {
		return fmt.Errorf("device ID does not match registered service device")
	}
	cfg.DeviceID = deviceID
	absRoot, e := filepath.Abs(w.Root)
	if e != nil {
		return fmt.Errorf("resolve workspace root: %w", e)
	}
	w.Root, e = filepath.EvalSymlinks(absRoot)
	if e != nil {
		return fmt.Errorf("resolve workspace root symlinks: %w", e)
	}
	info, e := os.Stat(w.Root)
	if e != nil || !info.IsDir() {
		return fmt.Errorf("workspace root is not a directory")
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
	w.Fingerprint, e = git.Fingerprint(ctx, git.Runner{}, w.Root, w.ProjectID)
	if e != nil {
		return fmt.Errorf("fingerprint workspace repository: %w", e)
	}
	cfg.Workspaces = append(cfg.Workspaces, w)
	return config.Save(paths, cfg)
}
