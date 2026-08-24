package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
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
type presenceSender interface {
	Heartbeat(context.Context, string, string) error
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
	go s.heartbeatLoop(ctx)
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
	failures := 0
	t := time.NewTimer(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if s.flush(ctx) {
				failures = 0
			} else {
				failures++
			}
			t.Reset(retryDelay(failures))
		}
	}
}
func (s *Service) flush(ctx context.Context) bool {
	pending, err := s.store.Pending(ctx)
	if err != nil {
		return false
	}
	if len(pending) == 0 {
		return true
	}
	ws, err := s.store.Workspaces(ctx)
	if err != nil {
		return false
	}
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
				return false
			}
			for _, event := range window {
				if e := s.store.Ack(ctx, event.ID); e != nil {
					return false
				}
			}
			events = events[n:]
		}
	}
	return true
}

func retryDelay(failures int) time.Duration {
	if failures <= 0 {
		return 500 * time.Millisecond
	}
	base := 500 * time.Millisecond
	for i := 1; i < failures && base < 30*time.Second; i++ {
		base *= 2
	}
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return base
	}
	// Jitter within 80-120% prevents synchronized reconnect storms.
	permille := 800 + binary.BigEndian.Uint64(random[:])%401
	return time.Duration(int64(base) * int64(permille) / 1000)
}

func (s *Service) heartbeatLoop(ctx context.Context) {
	presence, ok := s.sender.(presenceSender)
	if !ok {
		return
	}
	s.sendHeartbeats(ctx, presence)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendHeartbeats(ctx, presence)
		}
	}
}

func (s *Service) sendHeartbeats(ctx context.Context, presence presenceSender) {
	workspaces, err := s.store.Workspaces(ctx)
	if err != nil {
		return
	}
	for _, workspace := range workspaces {
		state := "active"
		if workspace.Paused {
			state = "paused"
		}
		_ = presence.Heartbeat(ctx, workspace.ID, state)
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
	case "intent":
		if e := validateIntent(q); e != nil {
			return daemon.Response{Error: e.Error()}
		}
		workstreamID := workspaceWorkstream(s.cfg, q.WorkspaceID)
		if workstreamID == "" {
			return daemon.Response{Error: "workspace not found"}
		}
		payload := map[string]any{"workstreamId": workstreamID, "title": q.Title, "intendedOutcome": q.IntendedOutcome}
		if q.ApproachSummary != "" {
			payload["approachSummary"] = q.ApproachSummary
		}
		if len(q.Components) > 0 {
			payload["components"] = q.Components
		}
		if len(q.Contracts) > 0 {
			payload["contracts"] = q.Contracts
		}
		if len(q.AnticipatedPaths) > 0 {
			payload["anticipatedPaths"] = q.AnticipatedPaths
		}
		if len(q.PlanItemIDs) > 0 {
			payload["planItemIds"] = q.PlanItemIDs
		}
		if e := s.store.EnqueueEvent(ctx, q.WorkspaceID, newID("evt_"), "manual", "workstream.intent_reported", payload); e != nil {
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

func workspaceWorkstream(cfg config.Config, workspaceID string) string {
	for _, workspace := range cfg.Workspaces {
		if workspace.ID == workspaceID {
			return workspace.WorkstreamID
		}
	}
	return ""
}

func validateIntent(q daemon.Request) error {
	if q.WorkspaceID == "" {
		return errors.New("workspace id required")
	}
	if len(q.Title) < 1 || len(q.Title) > 160 {
		return errors.New("intent title must be 1-160 characters")
	}
	if len(q.IntendedOutcome) < 1 || len(q.IntendedOutcome) > 2000 {
		return errors.New("intended outcome must be 1-2000 characters")
	}
	if len(q.ApproachSummary) > 2000 {
		return errors.New("approach summary exceeds 2000 characters")
	}
	if len(q.Components) > 32 || len(q.Contracts) > 32 || len(q.AnticipatedPaths) > 100 || len(q.PlanItemIDs) > 32 {
		return errors.New("intent list exceeds contract limit")
	}
	for _, values := range []struct {
		name  string
		items []string
		max   int
	}{{"component", q.Components, 160}, {"contract", q.Contracts, 160}, {"anticipated path", q.AnticipatedPaths, 512}, {"plan item id", q.PlanItemIDs, 128}} {
		for _, item := range values.items {
			if len(item) < 1 || len(item) > values.max || strings.ContainsAny(item, "\r\n\x00") {
				return fmt.Errorf("intent %s must be 1-%d safe characters", values.name, values.max)
			}
		}
	}
	return nil
}
func Register(ctx context.Context, root, apiBaseURL, deviceID string, w config.Workspace) error {
	if apiBaseURL == "" {
		return fmt.Errorf("hosted API base URL is required")
	}
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
	if cfg.APIBaseURL != "" && cfg.APIBaseURL != apiBaseURL {
		return fmt.Errorf("hosted API base URL does not match registered service")
	}
	cfg.APIBaseURL = apiBaseURL
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
