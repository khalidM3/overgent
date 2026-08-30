package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/stickguy/stickguy/internal/agentactivity"
	"github.com/stickguy/stickguy/internal/codexappserver"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	git "github.com/stickguy/stickguy/internal/git"
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/stickguy/stickguy/internal/sessiontranscript"
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
type collaborationProvider interface {
	Collaboration(context.Context, string) (hosted.CollaborationSnapshot, error)
}

const (
	injectionFetchTimeout = 1500 * time.Millisecond
	injectionBudget       = 800
	maxInjectionChars     = 3200
)

type Service struct {
	paths       config.Paths
	store       *store.Store
	cfg         config.Config
	sender      Sender
	mu          sync.Mutex
	scans, boot int64
	// scanCycles counts completed scan passes, not published manifests. A
	// caller that has to know a scan finished cannot use scans: an unchanged
	// workspace publishes nothing and leaves that counter still.
	scanCycles atomic.Int64
	watch      *watcher.Watcher
	// transcripts maps a session to its vendor transcript path. Only paths are
	// held; content is read on demand and never copied (ADR-036).
	transcriptMu sync.Mutex
	transcripts  map[string]string
	// codexThreadLister is replaceable only in tests. Production starts the
	// existing private, read-only app-server child when hook-derived local
	// session state cannot identify a thread.
	codexThreadLister func(context.Context, string, int) ([]codexappserver.Thread, error)
	// codexReadRefreshFailed records, per session workstream, whether the last
	// inferred-read refresh actually answered. Coverage reads it so a device
	// with a Codex it cannot talk to stops claiming it can infer that session's
	// reads. Held in memory only: it describes this process's live experience of
	// the local app-server, and a restart correctly forgets it.
	codexReadHealthMu      sync.Mutex
	codexReadRefreshFailed map[string]bool
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
		// A workspace root can disappear while the service is stopped: the
		// member deleted, moved, or renamed the repository. That must degrade
		// only that workspace. Failing the boot would stop observation for
		// every other Project on this device.
		if _, statErr := os.Stat(w.Root); statErr != nil {
			slog.Warn("workspace root unavailable; skipping observation for it", "workspace", w.ID, "error", statErr)
			continue
		}
		if e = watch.Add(w.Root); e != nil {
			slog.Warn("watch workspace root failed; skipping observation for it", "workspace", w.ID, "error", e)
			continue
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
	defer s.scanCycles.Add(1)
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
			// Contract extraction runs where changed paths are already known.
			// It publishes only files whose exported surface moved (ADR-048).
			s.publishContractFingerprints(ctx, w, m.Entries)
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
		// A focus the member has forgotten is the failure mode of any mute, so
		// the count travels with the health that the menu bar already reads.
		focused, _ := s.store.ActiveFocus(ctx, time.Now())
		return daemon.Response{OK: true, Data: map[string]any{"status": "ok", "bootCount": s.boot, "workspaces": len(w), "pausedWorkspaces": paused, "focusedSessions": len(focused), "pending": len(p), "scans": s.scans, "scanCycles": s.scanCycles.Load(), "pid": os.Getpid()}}
	case "pause", "resume":
		// Pause is scoped to whatever the caller named. A member reading one
		// Project means that Project, and asking them to name every workspace
		// inside it - or to reach for a switch that stops sharing on
		// repositories they were not looking at - is not the same request.
		paused := q.Method == "pause"
		if q.WorkspaceID == "" && q.ProjectID != "" {
			changed, e := s.store.SetProjectPaused(ctx, q.ProjectID, paused)
			if e != nil {
				return daemon.Response{Error: e.Error()}
			}
			return daemon.Response{OK: true, Data: map[string]any{"workspaces": changed, "paused": paused}}
		}
		if e := s.store.SetPaused(ctx, q.WorkspaceID, paused); e != nil {
			return daemon.Response{Error: e.Error()}
		}
		return daemon.Response{OK: true, Data: map[string]any{"workspaces": 1, "paused": paused}}
	case "focus", "unfocus", "focus_state":
		return s.handleFocus(ctx, q)
	case "unfocus_all":
		cleared, e := s.store.ClearAllFocus(ctx)
		if e != nil {
			return daemon.Response{Error: e.Error()}
		}
		return daemon.Response{OK: true, Data: map[string]any{"cleared": cleared}}
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
		if q.WaitingOn != nil {
			payload["waitingOn"] = q.WaitingOn
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
	case "resolve_agent_session":
		return s.handleAgentSessionResolution(ctx, q)
	case "get_resolutions":
		return s.handleCollaboration(ctx, q)
	case "session_detail":
		return s.handleSessionDetail(q)
	case "agent_event":
		return s.handleAgentEvent(ctx, q)
	case "agent_injection":
		return s.handleAgentInjection(ctx, q)
	case "scan":
		s.scanAll(ctx)
		return daemon.Response{OK: true}
	case "add_development_workspace":
		workspace, err := s.addWorkspace(ctx, q, true)
		if err != nil {
			return daemon.Response{Error: err.Error()}
		}
		return daemon.Response{OK: true, Data: workspace}
	case "add_project_workspace":
		workspace, err := s.addWorkspace(ctx, q, false)
		if err != nil {
			return daemon.Response{Error: err.Error()}
		}
		return daemon.Response{OK: true, Data: workspace}
	default:
		return daemon.Response{Error: "unsupported method"}
	}
}

type agentInjectionResult struct {
	AdditionalContext string   `json:"additionalContext,omitempty"`
	ItemIDs           []string `json:"itemIds,omitempty"`
}

// handleAgentInjection is a fail-open, fetch-through IPC operation. The hook
// process never talks to the hosted service directly, and no failure here can
// block or alter the vendor's turn.
// handleFocus reads and writes the local, never-transmitted request of one
// agent session not to be interrupted.
//
// Focus is the inbound half of a pair. `pause` stops this device publishing;
// focus stops the Project reaching one agent's turns. They are deliberately
// not the same control and deliberately not symmetric: a member who wants
// quiet should absorb the risk of missing a correction, not hide their work
// from teammates who are relying on seeing it.
func (s *Service) handleFocus(ctx context.Context, q daemon.Request) daemon.Response {
	session := q.AgentWorkstreamID
	if !validContractID(session) {
		return daemon.Response{Error: "session id required"}
	}
	now := time.Now()
	switch q.Method {
	case "focus":
		until, e := s.store.SetFocus(ctx, session, now, time.Duration(q.FocusSeconds)*time.Second)
		if e != nil {
			return daemon.Response{Error: e.Error()}
		}
		return daemon.Response{OK: true, Data: focusState(session, until, true)}
	case "unfocus":
		if e := s.store.ClearFocus(ctx, session); e != nil {
			return daemon.Response{Error: e.Error()}
		}
		return daemon.Response{OK: true, Data: focusState(session, time.Time{}, false)}
	default:
		until, focused, e := s.store.FocusedUntil(ctx, session, now)
		if e != nil {
			return daemon.Response{Error: e.Error()}
		}
		return daemon.Response{OK: true, Data: focusState(session, until, focused)}
	}
}

func focusState(session string, until time.Time, focused bool) map[string]any {
	state := map[string]any{"sessionId": session, "focused": focused}
	if focused {
		state["until"] = until.UTC().Format(time.RFC3339)
	}
	return state
}

func (s *Service) handleAgentInjection(ctx context.Context, q daemon.Request) daemon.Response {
	result := agentInjectionResult{}
	if q.AgentEvent != "SessionStart" && q.AgentEvent != "UserPromptSubmit" {
		return daemon.Response{OK: true, Data: result}
	}
	if q.AgentVendor != "claude" && q.AgentVendor != "codex" && q.AgentVendor != "cursor" || !validContractID(q.AgentWorkstreamID) {
		return daemon.Response{OK: true, Data: result}
	}
	// A focused session is skipped before anything is fetched or claimed. The
	// order matters more than the saved work: claiming marks a correction
	// delivered, so claiming for a session that will not be shown it would
	// retire the correction unread. Nothing is consumed here, so every pending
	// item is still waiting when the focus lapses.
	if _, focused, e := s.store.FocusedUntil(ctx, q.AgentWorkstreamID, time.Now()); e == nil && focused {
		return daemon.Response{OK: true, Data: result}
	}
	if _, _, ok := workspaceForAnyRoot(s.cfg, q.AgentCWD, q.AgentCandidateRoots); !ok {
		return daemon.Response{OK: true, Data: result}
	}
	provider, ok := s.sender.(briefProvider)
	if !ok {
		return daemon.Response{OK: true, Data: result}
	}
	fetchContext, cancel := context.WithTimeout(ctx, injectionFetchTimeout)
	defer cancel()
	brief, err := provider.CreateBrief(fetchContext, q.AgentWorkstreamID, "refresh", "", injectionBudget)
	if err != nil || len(brief.Items) == 0 {
		return daemon.Response{OK: true, Data: result}
	}
	candidates := make([]store.InjectionItem, 0, len(brief.Items))
	for _, item := range brief.Items {
		candidates = append(candidates, store.InjectionItem{ID: item.ID, Revision: item.Revision})
	}
	undelivered, err := s.store.UndeliveredInjectionItems(fetchContext, q.AgentWorkstreamID, candidates)
	if err != nil || len(undelivered) == 0 {
		return daemon.Response{OK: true, Data: result}
	}
	undeliveredSet := make(map[store.InjectionItem]bool, len(undelivered))
	for _, item := range undelivered {
		undeliveredSet[item] = true
	}
	pendingItems := make([]hosted.BriefItem, 0, len(undelivered))
	for _, item := range brief.Items {
		if undeliveredSet[store.InjectionItem{ID: item.ID, Revision: item.Revision}] {
			pendingItems = append(pendingItems, item)
		}
	}
	selected := selectInjectionItems(pendingItems)
	selectedCandidates := make([]store.InjectionItem, 0, len(selected))
	for _, item := range selected {
		selectedCandidates = append(selectedCandidates, store.InjectionItem{ID: item.ID, Revision: item.Revision})
	}
	// Claiming marks these revisions delivered so concurrent hooks cannot
	// inject the same correction twice. That trade is only safe while the
	// caller can still receive the payload: a claim written for a hook that has
	// already given up would retire the correction without anyone reading it,
	// and a stale-contract warning that is silently retired is worse than one
	// delivered twice.
	if fetchContext.Err() != nil {
		return daemon.Response{OK: true, Data: result}
	}
	claimed, err := s.store.ClaimInjectionDeliveries(fetchContext, q.AgentWorkstreamID, selectedCandidates, time.Now())
	if err != nil || len(claimed) == 0 {
		return daemon.Response{OK: true, Data: result}
	}
	claimedRevisions := make(map[store.InjectionItem]bool, len(claimed))
	for _, item := range claimed {
		claimedRevisions[item] = true
	}
	pending := make([]hosted.BriefItem, 0, len(claimed))
	for _, item := range selected {
		if claimedRevisions[store.InjectionItem{ID: item.ID, Revision: item.Revision}] {
			pending = append(pending, item)
			result.ItemIDs = append(result.ItemIDs, item.ID)
		}
	}
	result.AdditionalContext = renderInjection(pending)
	if result.AdditionalContext == "" {
		return daemon.Response{OK: true, Data: agentInjectionResult{}}
	}
	return daemon.Response{OK: true, Data: result}
}

func renderInjection(items []hosted.BriefItem) string {
	if len(items) == 0 {
		return ""
	}
	var rendered strings.Builder
	rendered.WriteString("Coordination update:\n")
	for _, item := range items {
		rendered.WriteString(injectionLine(item))
	}
	return rendered.String()
}

func selectInjectionItems(items []hosted.BriefItem) []hosted.BriefItem {
	selected := make([]hosted.BriefItem, 0, len(items))
	used := len("Coordination update:\n")
	for _, item := range items {
		line := injectionLine(item)
		remaining := maxInjectionChars - used
		if remaining <= 0 {
			break
		}
		if len(line) > remaining {
			reference := item
			reference.Text = "Review coordination item " + item.ID + "."
			reference.RelevanceReason = "Required context was compacted to honor the injection budget."
			reference.AdvisoryAction = "review_recommended"
			line = injectionLine(reference)
			if len(line) > remaining {
				break
			}
			item = reference
		}
		selected = append(selected, item)
		used += len(line)
	}
	return selected
}

func injectionLine(item hosted.BriefItem) string {
	line := "- " + strings.Join(strings.Fields(item.Text), " ")
	if reason := strings.Join(strings.Fields(item.RelevanceReason), " "); reason != "" {
		line += " Reason: " + reason
	}
	if action := strings.Join(strings.Fields(item.AdvisoryAction), " "); action != "" {
		line += " Action: " + action + "."
	}
	return line + "\n"
}

// handleCollaboration serves the one remaining agent-facing collaboration read:
// how collisions affecting this workspace were resolved (ADR-037).
// rememberTranscript records where this session's transcript lives so the owner
// can read it later. Only the path is kept; content is never copied (ADR-036).
func (s *Service) rememberTranscript(workstreamID, path string) {
	s.transcriptMu.Lock()
	defer s.transcriptMu.Unlock()
	if s.transcripts == nil {
		s.transcripts = map[string]string{}
	}
	s.transcripts[workstreamID] = path
}

func (s *Service) transcriptPath(workstreamID string) string {
	s.transcriptMu.Lock()
	defer s.transcriptMu.Unlock()
	return s.transcripts[workstreamID]
}

// shareTranscript projects session messages for an enrolled Project member.
// The workspace pause gate is enforced synchronously by flush, and every
// candidate is classified before it can be enqueued (ADR-047).
func (s *Service) shareTranscript(ctx context.Context, workspace config.Workspace, event agentactivity.Event, transcript sessiontranscript.Session) int {
	if len(transcript.Messages) == 0 {
		return 0
	}
	shared := 0
	for index, candidate := range transcript.Messages {
		if candidate.Kind == sessiontranscript.KindTool {
			continue
		}
		message, classifyErr := agentactivity.ClassifyMessage(agentactivity.Message{Kind: candidate.Kind, Text: candidate.Text})
		if classifyErr != nil {
			continue
		}
		// A stable identity per message makes redelivery a hosted no-op, so a
		// restart or a repeated hook never duplicates shared content.
		digest := sha256.Sum256([]byte(event.WorkstreamID + "\x00" + strconv.Itoa(index) + "\x00" + message.Kind + "\x00" + message.Text))
		messagePayload := map[string]any{
			"messageId": fmt.Sprintf("msg_%x", digest[:16]), "workstreamId": event.WorkstreamID,
			"vendor": event.Vendor, "sessionAlias": event.SessionAlias,
			"kind": message.Kind, "text": message.Text,
		}
		if s.store.EnqueueEvent(ctx, workspace.ID, newID("evt_"), "hook", "agent.conversation_shared", messagePayload) != nil {
			continue
		}
		shared++
	}
	return shared
}

// handleSessionDetail returns the caller's own session content. Only sessions
// this device observed have a remembered transcript, so a member can never read
// someone else's session through this path, and nothing is uploaded (ADR-036).
func (s *Service) handleSessionDetail(q daemon.Request) daemon.Response {
	if !validContractID(q.AgentWorkstreamID) {
		return daemon.Response{Error: "invalid session id"}
	}
	path := s.transcriptPath(q.AgentWorkstreamID)
	if path == "" {
		return daemon.Response{OK: true, Data: map[string]any{"available": false, "messages": []any{}}}
	}
	transcript, err := sessiontranscript.Read(path, sessiontranscript.MaxMessages)
	if err != nil {
		return daemon.Response{OK: true, Data: map[string]any{"available": false, "messages": []any{}}}
	}
	return daemon.Response{OK: true, Data: map[string]any{
		"available": true, "title": transcript.Title, "branch": transcript.Branch, "messages": transcript.Messages,
	}}
}

func (s *Service) handleCollaboration(ctx context.Context, q daemon.Request) daemon.Response {
	workspace := config.Workspace{}
	for _, candidate := range s.cfg.Workspaces {
		if candidate.ID == q.WorkspaceID {
			workspace = candidate
			break
		}
	}
	if workspace.ID == "" {
		return daemon.Response{Error: "workspace not found"}
	}
	provider, ok := s.sender.(collaborationProvider)
	if !ok {
		return daemon.Response{Error: "hosted collaboration unavailable"}
	}
	snapshot, err := provider.Collaboration(ctx, workspace.ProjectID)
	if err != nil {
		return daemon.Response{Error: "hosted collaboration unavailable"}
	}
	return daemon.Response{OK: true, Data: map[string]any{"collaboration": snapshot}}
}

func (s *Service) handleAgentEvent(ctx context.Context, q daemon.Request) daemon.Response {
	// A vendor may report several workspace roots for one session — Cursor sends
	// `workspace_roots` as an array for a multi-root workspace — and only this
	// process knows which of them the member registered. The first registered
	// root wins, and it is what a session-scoped variable is later pinned to, so
	// every later hook in that session resolves to the same repository.
	cwd, workspace, ok := workspaceForAnyRoot(s.cfg, q.AgentCWD, q.AgentCandidateRoots)
	if !ok {
		return daemon.Response{Error: "agent session is not inside a registered repository"}
	}
	event, err := agentactivity.NormalizePaths(agentactivity.Event{
		Vendor: q.AgentVendor, CWD: cwd, WorkstreamID: q.AgentWorkstreamID,
		SessionAlias: q.AgentSessionAlias, Kind: q.AgentEvent, Status: q.AgentStatus,
		Action: q.AgentAction, Tool: q.AgentTool, AgentType: q.AgentType,
		SubagentAlias: q.AgentSubagentAlias, CandidatePaths: q.AgentPaths,
	}, workspace.Root)
	if err != nil {
		// Passive observation fails closed. The coding-agent operation is never
		// blocked or modified because an activity candidate was rejected.
		return daemon.Response{OK: true, Data: map[string]any{"accepted": false}}
	}
	payload := map[string]any{
		"workstreamId": event.WorkstreamID, "vendor": event.Vendor,
		"sessionAlias": event.SessionAlias, "kind": event.Kind,
		"status": event.Status, "action": event.Action,
	}
	if event.Tool != "" {
		payload["tool"] = event.Tool
	}
	if event.AgentType != "" {
		payload["agentType"] = event.AgentType
	}
	if event.SubagentAlias != "" {
		payload["subagentAlias"] = event.SubagentAlias
	}
	// State what Stickguy can actually see of this session's reads, so an empty
	// read set is never mistaken for a session that read nothing (ADR-052).
	payload["readCoverage"] = agentactivity.ReadCoverage(event.Vendor, s.codexInferredReadsUsable(event.WorkstreamID))
	// Only mutation paths become session work evidence. An inspection tool's
	// paths are the read set, published below, and counting them here made a
	// session that merely read a file collide with the session that wrote it
	// (ADR-048). The read set still drives stale-assumption detection.
	if len(event.CandidatePaths) > 0 && !agentactivity.ReadTool(event.Tool) {
		payload["paths"] = event.CandidatePaths
	}
	// The branch is read from the registered worktree rather than reported by the
	// agent, so it reflects the real checkout. Observation must never delay the
	// coding agent, so a slow or failed read simply omits the branch.
	branchContext, cancelBranch := context.WithTimeout(ctx, 2*time.Second)
	branch, branchErr := git.CurrentBranch(branchContext, git.Runner{}, workspace.Root)
	cancelBranch()
	if branchErr == nil && branch != "" {
		payload["branch"] = branch
	}
	// The transcript names this session far better than a vendor alias does, so
	// the tree can show what each chat is actually about.
	var transcript sessiontranscript.Session
	// Claude names its transcript in the hook payload; Codex does not, but names
	// every rollout after the session id, so it is discoverable locally.
	transcriptPath := q.AgentTranscriptPath
	if !sessiontranscript.TranscriptAvailable(event.Vendor) {
		// Cursor publishes no session record this device can parse, so no path is
		// attempted. See internal/sessiontranscript/cursor.go for what that costs
		// and what supplies the session title instead.
		transcriptPath = ""
	}
	if transcriptPath == "" && event.Vendor == "codex" {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			transcriptPath = sessiontranscript.LocateCodexRollout(home, q.AgentVendorSessionID)
		}
	}
	// A vendor that writes no transcript Stickguy can read may still have named
	// this session. Cursor's adapter derives that name from the submitted prompt
	// and runs it through ClassifyCoordinationTitle before it arrives here, so
	// this value is already classifier output (ADR-042). A transcript title,
	// where one exists, is the better name and overwrites it below.
	if q.AgentSessionTitle != "" {
		if title, titleErr := agentactivity.ClassifyCoordinationTitle(q.AgentSessionTitle); titleErr == nil {
			payload["sessionTitle"] = title
		}
	}
	if transcriptPath != "" {
		if parsed, readErr := sessiontranscript.Read(transcriptPath, sessiontranscript.MaxMessages); readErr == nil {
			transcript = parsed
			s.rememberTranscript(event.WorkstreamID, transcriptPath)
			if title, titleErr := agentactivity.ClassifyCoordinationTitle(transcript.Title); titleErr == nil {
				payload["sessionTitle"] = title
			}
			if _, present := payload["branch"]; !present && transcript.Branch != "" {
				payload["branch"] = transcript.Branch
			}
		}
	}
	if err := s.store.EnqueueEvent(ctx, workspace.ID, newID("evt_"), "hook", "agent.activity_reported", payload); err != nil {
		return daemon.Response{Error: err.Error()}
	}
	if err := s.store.RecordAgentObservation(ctx, workspace.ID, event.Vendor, time.Now()); err != nil {
		return daemon.Response{Error: err.Error()}
	}
	if event.Vendor == "codex" {
		canonicalCWD, canonical := canonicalDirectory(event.CWD)
		if !canonical {
			return daemon.Response{OK: true, Data: map[string]any{"accepted": false}}
		}
		if err := s.store.RecordAgentSession(ctx, store.AgentSession{
			WorkspaceID: workspace.ID, Vendor: event.Vendor,
			WorkstreamID: event.WorkstreamID, CWD: canonicalCWD,
			Status: event.Status, ObservedAt: time.Now(),
		}); err != nil {
			return daemon.Response{Error: err.Error()}
		}
	}
	// A read set is fed by inspection tools only; an edit is a write, and the
	// manifest pipeline already reports it.
	if agentactivity.ReadTool(event.Tool) && len(event.CandidatePaths) > 0 {
		s.publishReadSet(ctx, workspace, event.WorkstreamID, event.CandidatePaths, store.ReadFidelityObserved, readPathsPerEvent)
	}
	// Codex names no file it reads, so its read set is recovered from its own
	// command classification at a turn boundary rather than from tool events
	// (ADR-052). This runs after the turn, never during it.
	if event.Vendor == "codex" && (event.Kind == "Stop" || event.Kind == "SessionEnd") {
		s.publishCodexInferredReads(ctx, workspace, q.AgentVendorSessionID, event.WorkstreamID)
	}
	shared := s.shareTranscript(ctx, workspace, event, transcript)
	// The resolved root is returned so an adapter whose later hooks report no
	// working directory can pin this one for the rest of the session.
	return daemon.Response{OK: true, Data: map[string]any{"accepted": true, "sharedMessages": shared, "workspaceRoot": workspace.Root}}
}

// workspaceForAnyRoot resolves the first candidate root that lies inside a
// registered repository, returning the candidate itself alongside the workspace
// so relative paths in the event still resolve against the directory the vendor
// actually reported.
func workspaceForAnyRoot(cfg config.Config, primary string, candidates []string) (string, config.Workspace, bool) {
	for _, candidate := range append([]string{primary}, candidates...) {
		if candidate == "" {
			continue
		}
		if workspace, ok := workspaceForCWD(cfg, candidate); ok {
			return candidate, workspace, true
		}
	}
	return "", config.Workspace{}, false
}

func workspaceForCWD(cfg config.Config, cwd string) (config.Workspace, bool) {
	abs, ok := canonicalDirectory(cwd)
	if !ok {
		return config.Workspace{}, false
	}
	var selected config.Workspace
	for _, candidate := range cfg.Workspaces {
		relative, relErr := filepath.Rel(candidate.Root, abs)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if selected.Root == "" || len(candidate.Root) > len(selected.Root) {
			selected = candidate
		}
	}
	return selected, selected.Root != ""
}

func canonicalDirectory(directory string) (string, bool) {
	if directory == "" {
		return "", false
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", false
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		absolute = resolved
	}
	return filepath.Clean(absolute), true
}

func (s *Service) addWorkspace(ctx context.Context, q daemon.Request, requireExistingProjectMember bool) (config.Workspace, error) {
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
	if requireExistingProjectMember && !projectMemberFound {
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
	// An MCP client that could identify its own agent session gets that
	// session's workstream. Findings are routed to the per-session workstream
	// the activity hooks create, and a brief is filtered by the workstream it
	// is requested for, so lifecycle calls made against the workspace
	// workstream could never surface a finding routed to the calling session.
	// The same split made an agent's own intent land on a second identity and
	// then collide with itself. A vendor that exposes no session identity still
	// falls back to the workspace workstream.
	if q.AgentWorkstreamID != "" && validContractID(q.AgentWorkstreamID) {
		workstreamID = q.AgentWorkstreamID
	}
	result := lifecycleResult{}
	trigger := ""
	var readSetPaths []string
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
			// The paths an MCP client reports consuming at begin_work join the
			// session's read set alongside hook-observed inspections. They are
			// published after the intent so the workstream exists first.
			readSetPaths = q.AnticipatedPaths
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
	if len(readSetPaths) > 0 {
		if workspace, found := workspaceByID(s.cfg, q.WorkspaceID); found {
			// These are the paths an MCP client said it expects to consume, not
			// files anything watched it open, so they enter the read set as the
			// agent's own claim (ADR-052).
			s.publishReadSet(ctx, workspace, workstreamID, readSetPaths, store.ReadFidelitySelfDeclared, readPathsPerEvent)
		}
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
	if q.WaitingOn != nil {
		payload["waitingOn"] = q.WaitingOn
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
		item := map[string]any{"state": value.State, "checkKind": value.CheckKind, "label": value.Label, "summary": value.Summary, "source": "mcp"}
		if value.ObservedAt != "" {
			item["observedAt"] = value.ObservedAt
		}
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

func workspaceByID(cfg config.Config, workspaceID string) (config.Workspace, bool) {
	for _, workspace := range cfg.Workspaces {
		if workspace.ID == workspaceID {
			return workspace, true
		}
	}
	return config.Workspace{}, false
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
	if len(q.Components) > 32 || len(q.Contracts) > 32 || len(q.WaitingOn) > 8 || len(q.AnticipatedPaths) > 100 || len(q.PlanItemIDs) > 32 {
		return errors.New("intent list exceeds contract limit")
	}
	for _, values := range []struct {
		name  string
		items []string
		max   int
	}{{"component", q.Components, 160}, {"contract", q.Contracts, 160}, {"waiting_on claim", q.WaitingOn, 160}, {"anticipated path", q.AnticipatedPaths, 512}, {"plan item id", q.PlanItemIDs, 128}} {
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
