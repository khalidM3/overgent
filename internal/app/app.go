package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/stickguy/stickguy/internal/store"
	"github.com/stickguy/stickguy/internal/watcher"
)

type Sender interface {
	Send(context.Context, string, []byte) error
}
type presenceSender interface {
	Heartbeat(context.Context, string, string) error
}
type briefProvider interface {
	CreateBrief(context.Context, string, string, string, int) (hosted.CoordinationBrief, error)
}
type Service struct {
	paths       config.Paths
	store       *store.Store
	cfg         config.Config
	sender      Sender
	mu          sync.Mutex
	scans, boot int64
	watch       *watcher.Watcher
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
	s.watch = watch
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
		paused := 0
		for _, workspace := range w {
			if workspace.Paused {
				paused++
			}
		}
		return daemon.Response{OK: true, Data: map[string]any{"status": "ok", "bootCount": s.boot, "workspaces": len(w), "pausedWorkspaces": paused, "pending": len(p), "scans": s.scans, "pid": os.Getpid()}}
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
	case "begin_work", "update_intent", "check_coordination", "report_checkpoint", "acknowledge_context", "finish_work", "report_event":
		return s.handleLifecycle(ctx, q)
	case "scan":
		s.scanAll(ctx)
		return daemon.Response{OK: true}
	case "add_development_workspace":
		workspace, err := s.addDevelopmentWorkspace(ctx, q)
		if err != nil {
			return daemon.Response{Error: err.Error()}
		}
		return daemon.Response{OK: true, Data: workspace}
	default:
		return daemon.Response{Error: "unsupported method"}
	}
}

func (s *Service) addDevelopmentWorkspace(ctx context.Context, q daemon.Request) (config.Workspace, error) {
	if q.Root == "" || q.ProjectID == "" || q.WorkspaceID == "" || q.WorkstreamID == "" || q.MemberID == "" || q.SessionID == "" {
		return config.Workspace{}, errors.New("development workspace fields are required")
	}
	for label, value := range map[string]struct{ value, pattern string }{
		"Project": {q.ProjectID, `^prj_[a-z0-9_]{1,80}$`}, "workspace": {q.WorkspaceID, `^wsp_[a-z0-9_]{1,123}$`},
		"workstream": {q.WorkstreamID, `^wrk_[a-z0-9_]{1,80}$`}, "member": {q.MemberID, `^mem_[a-z0-9_]{1,123}$`},
		"session": {q.SessionID, `^ses_[a-z0-9_]{1,123}$`},
	} {
		if !regexp.MustCompile(value.pattern).MatchString(value.value) {
			return config.Workspace{}, fmt.Errorf("invalid %s ID", label)
		}
	}
	absRoot, err := filepath.Abs(q.Root)
	if err != nil {
		return config.Workspace{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return config.Workspace{}, fmt.Errorf("resolve workspace root symlinks: %w", err)
	}
	baseline, err := git.CaptureBaseline(ctx, git.Runner{}, canonicalRoot)
	if err != nil {
		return config.Workspace{}, err
	}
	fingerprint, err := git.Fingerprint(ctx, git.Runner{}, canonicalRoot, q.ProjectID)
	if err != nil {
		return config.Workspace{}, fmt.Errorf("fingerprint workspace repository: %w", err)
	}
	workspace := config.Workspace{ID: q.WorkspaceID, ProjectID: q.ProjectID, WorkstreamID: q.WorkstreamID, MemberID: q.MemberID, SessionID: q.SessionID, Root: canonicalRoot, Baseline: baseline, Fingerprint: fingerprint}

	s.mu.Lock()
	defer s.mu.Unlock()
	var projectMemberFound bool
	for _, existing := range s.cfg.Workspaces {
		if existing.ID == workspace.ID || existing.Root == workspace.Root {
			return config.Workspace{}, errors.New("workspace already registered")
		}
		if existing.ProjectID == workspace.ProjectID && existing.MemberID == workspace.MemberID {
			projectMemberFound = true
		}
	}
	if !projectMemberFound {
		return config.Workspace{}, errors.New("development workspace must reuse an enrolled Project member")
	}
	if s.watch == nil {
		return config.Workspace{}, errors.New("workspace watcher is unavailable")
	}
	if err = s.watch.Add(workspace.Root); err != nil {
		return config.Workspace{}, fmt.Errorf("watch development workspace: %w", err)
	}
	previous := s.cfg
	next := s.cfg
	next.Workspaces = append(append([]config.Workspace(nil), s.cfg.Workspaces...), workspace)
	if err = config.Save(s.paths, next); err != nil {
		return config.Workspace{}, err
	}
	if err = s.store.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: s.cfg.DeviceID, SessionID: workspace.SessionID, Root: workspace.Root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		_ = config.Save(s.paths, previous)
		return config.Workspace{}, err
	}
	s.cfg = next
	return workspace, nil
}

type lifecycleResult struct {
	Duplicate      bool                      `json:"duplicate"`
	IntentRevision int64                     `json:"intentRevision,omitempty"`
	Brief          *hosted.CoordinationBrief `json:"brief,omitempty"`
	Degraded       bool                      `json:"degraded"`
	Degradation    string                    `json:"degradation,omitempty"`
}

func (s *Service) handleLifecycle(ctx context.Context, q daemon.Request) daemon.Response {
	workstreamID := workspaceWorkstream(s.cfg, q.WorkspaceID)
	if workstreamID == "" {
		return daemon.Response{Error: "workspace not found"}
	}
	result := lifecycleResult{}
	trigger := ""
	var publication *store.LifecyclePublication
	switch q.Method {
	case "begin_work", "update_intent":
		if err := validateLifecycleIntent(q); err != nil {
			return daemon.Response{Error: err.Error()}
		}
		payload := intentPayload(workstreamID, q)
		publication = &store.LifecyclePublication{WorkspaceID: q.WorkspaceID, Method: q.Method, IdempotencyKey: q.IdempotencyKey, Source: "mcp", Kind: "workstream.intent_reported", Payload: payload, IncrementIntentRevision: true}
		if q.Method == "update_intent" {
			expected := q.Revision
			publication.ExpectedIntentRevision = &expected
		} else {
			trigger = "begin"
		}
	case "check_coordination":
		trigger = q.Trigger
		if trigger == "" {
			trigger = "before_broad_edit"
		}
		if !validBriefTrigger(trigger) || q.ApproximateTokenBudget < 0 || q.ApproximateTokenBudget > 800 || q.ApproximateTokenBudget > 0 && q.ApproximateTokenBudget < 128 || len(q.SinceCursor) > 512 {
			return daemon.Response{Error: "invalid coordination brief request"}
		}
	case "report_checkpoint":
		if err := validateCheckpoint(q); err != nil {
			return daemon.Response{Error: err.Error()}
		}
		payload := map[string]any{"checkpointId": q.CheckpointID, "workstreamId": workstreamID, "summary": q.Summary}
		if len(q.Discoveries) > 0 {
			payload["discoveries"] = q.Discoveries
		}
		if q.ManifestRevision > 0 {
			payload["relatedManifestRevision"] = q.ManifestRevision
		}
		if q.BriefID != "" {
			payload["basedOnBriefId"] = q.BriefID
		}
		if len(q.Verification) > 0 {
			payload["verification"] = verificationPayload(q.Verification)
		}
		publication = &store.LifecyclePublication{WorkspaceID: q.WorkspaceID, Method: q.Method, IdempotencyKey: q.CheckpointID, Source: "mcp", Kind: "workstream.checkpoint_reported", Payload: payload}
		trigger = "checkpoint"
	case "acknowledge_context":
		if !validContractID(q.BriefID) || len(q.ConsideredItemIDs) < 1 || len(q.ConsideredItemIDs) > 64 {
			return daemon.Response{Error: "invalid context acknowledgement"}
		}
		for _, itemID := range q.ConsideredItemIDs {
			if !validContractID(itemID) {
				return daemon.Response{Error: "invalid context acknowledgement item"}
			}
		}
		key := q.IdempotencyKey
		if key == "" {
			key = q.BriefID
		}
		publication = &store.LifecyclePublication{WorkspaceID: q.WorkspaceID, Method: q.Method, IdempotencyKey: key, Source: "mcp", Kind: "context.acknowledged", Payload: map[string]any{"briefId": q.BriefID, "consideredItemIds": q.ConsideredItemIDs}}
	case "finish_work":
		if err := validateIdempotency(q.IdempotencyKey); err != nil {
			return daemon.Response{Error: err.Error()}
		}
		if len(q.Outcome) < 1 || len(q.Outcome) > 2000 || len(q.Summary) < 1 || len(q.Summary) > 2000 || q.ManifestRevision < 0 || q.BriefID != "" && !validContractID(q.BriefID) {
			return daemon.Response{Error: "finish outcome and summary are required and bounded"}
		}
		if err := validateVerification(q.Verification); err != nil {
			return daemon.Response{Error: err.Error()}
		}
		checkpointSum := sha256.Sum256([]byte("stickguy.finish-checkpoint.v1\x00" + q.WorkspaceID + "\x00" + q.IdempotencyKey))
		checkpoint := map[string]any{"checkpointId": fmt.Sprintf("chk_finish_%x", checkpointSum[:12]), "workstreamId": workstreamID, "summary": q.Summary}
		if q.ManifestRevision > 0 {
			checkpoint["relatedManifestRevision"] = q.ManifestRevision
		}
		if q.BriefID != "" {
			checkpoint["basedOnBriefId"] = q.BriefID
		}
		if len(q.Verification) > 0 {
			checkpoint["verification"] = verificationPayload(q.Verification)
		}
		publication = &store.LifecyclePublication{
			WorkspaceID: q.WorkspaceID, Method: q.Method, IdempotencyKey: q.IdempotencyKey, Source: "mcp",
			Kind: "workstream.checkpoint_reported", Payload: checkpoint,
			Additional: []store.LifecycleEvent{
				{Kind: "activity.reported", Payload: map[string]any{"kind": "completion", "summary": q.Outcome}},
				{Kind: "workstream.status_changed", Payload: map[string]any{"workstreamId": workstreamID, "status": "done"}},
			},
		}
		trigger = "finish"
	case "report_event":
		if err := validateIdempotency(q.IdempotencyKey); err != nil {
			return daemon.Response{Error: err.Error()}
		}
		if !map[string]bool{"decision": true, "completion": true, "blocker": true}[q.Kind] || len(q.Summary) < 1 || len(q.Summary) > 2000 {
			return daemon.Response{Error: "invalid bounded activity event"}
		}
		publication = &store.LifecyclePublication{WorkspaceID: q.WorkspaceID, Method: q.Method, IdempotencyKey: q.IdempotencyKey, Source: "mcp", Kind: "activity.reported", Payload: map[string]any{"kind": q.Kind, "summary": q.Summary}}
	}
	if publication != nil {
		revision, duplicate, err := s.store.PublishLifecycle(ctx, *publication)
		if err != nil {
			return daemon.Response{Error: err.Error()}
		}
		result.IntentRevision, result.Duplicate = revision, duplicate
	}
	if trigger != "" {
		budget := int(q.ApproximateTokenBudget)
		if budget == 0 {
			budget = 400
		}
		provider, ok := s.sender.(briefProvider)
		if !ok {
			result.Degraded, result.Degradation = true, "hosted_coordination_unavailable"
		} else if brief, err := provider.CreateBrief(ctx, workstreamID, trigger, q.SinceCursor, budget); err != nil {
			result.Degraded, result.Degradation = true, "hosted_coordination_unavailable"
		} else {
			result.Brief = &brief
		}
	}
	return daemon.Response{OK: true, Data: result}
}

func intentPayload(workstreamID string, q daemon.Request) map[string]any {
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
	return payload
}

func validateLifecycleIntent(q daemon.Request) error {
	if err := validateIdempotency(q.IdempotencyKey); err != nil {
		return err
	}
	if q.Method == "update_intent" && q.Revision < 1 {
		return errors.New("update_intent requires the current positive revision")
	}
	return validateIntent(q)
}

func validateIdempotency(key string) error {
	if len(key) < 1 || len(key) > 128 || strings.ContainsAny(key, "\r\n\x00") {
		return errors.New("idempotency key must be 1-128 safe characters")
	}
	return nil
}

func validBriefTrigger(trigger string) bool {
	return map[string]bool{"begin": true, "before_broad_edit": true, "checkpoint": true, "refresh": true, "finish": true, "manual": true}[trigger]
}

func validateCheckpoint(q daemon.Request) error {
	if !regexp.MustCompile(`^chk_[A-Za-z0-9_-]{1,80}$`).MatchString(q.CheckpointID) || len(q.Summary) < 1 || len(q.Summary) > 2000 || len(q.Discoveries) > 32 || len(q.Verification) > 32 || q.ManifestRevision < 0 || q.BriefID != "" && !validContractID(q.BriefID) {
		return errors.New("invalid bounded checkpoint")
	}
	for _, discovery := range q.Discoveries {
		if len(discovery) < 1 || len(discovery) > 500 {
			return errors.New("checkpoint discovery exceeds limit")
		}
	}
	return validateVerification(q.Verification)
}

func validContractID(value string) bool {
	return regexp.MustCompile(`^[a-z][a-z0-9_]{2,127}$`).MatchString(value)
}

func validateVerification(values []daemon.VerificationSummary) error {
	if len(values) > 32 {
		return errors.New("too many verification summaries")
	}
	for _, verification := range values {
		if !map[string]bool{"not_run": true, "running": true, "passed": true, "failed": true, "unknown": true}[verification.State] || len(verification.CheckKind) < 1 || len(verification.CheckKind) > 80 || len(verification.Label) < 1 || len(verification.Label) > 160 || len(verification.Summary) > 500 || len(verification.AffectedComponent) > 160 || verification.ManifestRevision < 0 {
			return errors.New("invalid structured verification summary")
		}
		if verification.ObservedAt != "" {
			if _, err := time.Parse(time.RFC3339Nano, verification.ObservedAt); err != nil {
				return errors.New("verification observed_at must be RFC3339")
			}
		}
	}
	return nil
}

func verificationPayload(values []daemon.VerificationSummary) []map[string]any {
	out := make([]map[string]any, len(values))
	for i, value := range values {
		item := map[string]any{"state": value.State, "checkKind": value.CheckKind, "label": value.Label, "summary": value.Summary, "source": "mcp", "observedAt": value.ObservedAt}
		if value.AffectedComponent != "" {
			item["affectedComponent"] = value.AffectedComponent
		}
		if value.ManifestRevision > 0 {
			item["manifestRevision"] = value.ManifestRevision
		}
		out[i] = item
	}
	return out
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
