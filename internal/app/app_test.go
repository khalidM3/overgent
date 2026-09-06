//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/khalidM3/overgent/internal/agentactivity"
	"github.com/khalidM3/overgent/internal/codexappserver"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/daemon"
	gitobs "github.com/khalidM3/overgent/internal/git"
	"github.com/khalidM3/overgent/internal/hosted"
	"github.com/khalidM3/overgent/internal/sessiontranscript"
	"github.com/khalidM3/overgent/internal/store"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fixtureOrigin is the backend every single-backend fixture publishes to. A
// service resolves its publisher through the Project's backend now, so a
// fixture that names no backend would correctly publish nothing.
const fixtureOrigin = "https://fixture.example.test"

// fixtureSender hands the same publisher to every backend, which is what a
// single-backend fixture means. Tests that need two backends build the map
// themselves.
func fixtureSender(sender Sender) SenderFactory {
	return func(context.Context, config.Backend) (Sender, error) { return sender, nil }
}

type fakeSender struct {
	mu      sync.Mutex
	events  int
	batches map[string][][]byte
}

type failingPublishSender struct{ err error }

func (s failingPublishSender) Send(context.Context, string, []byte) error { return s.err }

func TestDoctorSurfacesLastPublishErrorAfterSendFailure(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := store.Workspace{
		ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture",
		MemberID: "mem_fixture", DeviceID: "dev_fixture", SessionID: "ses_fixture",
		Root: "/fixture", Baseline: strings.Repeat("a", 40), Fingerprint: "opaque",
	}
	if err = db.UpsertWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		store:     db,
		cfg:       config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, SessionID: workspace.SessionID, Root: workspace.Root}}),
		newSender: fixtureSender(failingPublishSender{err: errors.New("hosted publish unavailable")}),
	}
	if service.flush(ctx) {
		t.Fatal("flush unexpectedly succeeded")
	}
	response := service.handle(ctx, daemon.Request{Method: "doctor"})
	data, ok := response.Data.(map[string]any)
	if !response.OK || !ok {
		t.Fatalf("doctor response=%#v", response)
	}
	lastError, _ := data["lastPublishError"].(string)
	if lastError == "" {
		t.Fatalf("doctor did not surface last publish error: %#v", data)
	}
}

type lifecycleFixtureSender struct{ briefs int }

func (*lifecycleFixtureSender) Send(context.Context, string, []byte) error { return nil }
func (s *lifecycleFixtureSender) CreateBrief(_ context.Context, workstreamID, trigger, _ string, budget int) (hosted.CoordinationBrief, error) {
	s.briefs++
	return hosted.CoordinationBrief{BriefID: "brf_fixture", ProjectID: "prj_fixture", RepositoryID: "rep_fixture", WorkstreamID: workstreamID, Trigger: trigger, RequestedBudget: budget, Items: []hosted.BriefItem{{ID: "itm_fixture", Kind: "decision", Text: "Synthetic coordination item"}}}, nil
}

type injectionFixtureSender struct {
	mu       sync.Mutex
	revision int
	delay    bool
}

func (*injectionFixtureSender) Send(context.Context, string, []byte) error { return nil }
func (s *injectionFixtureSender) CreateBrief(ctx context.Context, workstreamID, trigger, _ string, budget int) (hosted.CoordinationBrief, error) {
	if s.delay {
		<-ctx.Done()
		return hosted.CoordinationBrief{}, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return hosted.CoordinationBrief{
		BriefID: "brf_injection", WorkstreamID: workstreamID, Trigger: trigger,
		RequestedBudget: budget, RenderedSize: 80,
		Items: []hosted.BriefItem{{ID: "fnd_contract_drift", Revision: s.revision, Kind: "finding", Text: "backend.Refresh signature changed after you read it.", RelevanceReason: "This finding directly involves the current workstream.", AdvisoryAction: "coordination_required", Priority: 100}},
	}, nil
}

func TestAgentInjectionFetchesThroughIPCStateAndDeduplicatesRevision(t *testing.T) {
	ctx := context.Background()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	sender := &injectionFixtureSender{revision: 1}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}), newSender: fixtureSender(sender)}
	request := daemon.Request{Method: "agent_injection", AgentVendor: "claude", AgentCWD: root, AgentWorkstreamID: "wrk_agent_0123456789abcdef0123456789abcdef", AgentEvent: "UserPromptSubmit"}
	first := service.handle(ctx, request)
	firstResult := first.Data.(agentInjectionResult)
	if !first.OK || !strings.Contains(firstResult.AdditionalContext, "backend.Refresh") || len(firstResult.ItemIDs) != 1 {
		t.Fatalf("first injection=%#v", first)
	}
	second := service.handle(ctx, request)
	if !second.OK || second.Data.(agentInjectionResult).AdditionalContext != "" {
		t.Fatalf("same revision reinjected=%#v", second)
	}
	sender.mu.Lock()
	sender.revision = 2
	sender.mu.Unlock()
	third := service.handle(ctx, request)
	if !third.OK || !strings.Contains(third.Data.(agentInjectionResult).AdditionalContext, "backend.Refresh") {
		t.Fatalf("revised item not injected=%#v", third)
	}
}

func TestAgentInjectionFetchTimeoutFailsOpen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}), newSender: fixtureSender(&injectionFixtureSender{revision: 1, delay: true})}
	response := service.handle(ctx, daemon.Request{Method: "agent_injection", AgentVendor: "codex", AgentCWD: root, AgentWorkstreamID: "wrk_agent_0123456789abcdef0123456789abcdef", AgentEvent: "SessionStart"})
	if !response.OK || response.Data.(agentInjectionResult).AdditionalContext != "" {
		t.Fatalf("timeout did not fail open: %#v", response)
	}
}

func TestInjectionRenderingHonorsCharacterCap(t *testing.T) {
	items := make([]hosted.BriefItem, 64)
	for index := range items {
		items[index] = hosted.BriefItem{ID: fmt.Sprintf("fnd_%03d", index), Revision: 1, Text: strings.Repeat("coordination detail ", 80), RelevanceReason: strings.Repeat("direct relevance ", 20), AdvisoryAction: "coordination_required"}
	}
	selected := selectInjectionItems(items)
	rendered := renderInjection(selected)
	if len(rendered) > maxInjectionChars || len(selected) == 0 || len(selected) >= len(items) {
		t.Fatalf("rendered chars=%d selected=%d", len(rendered), len(selected))
	}
}

func TestLifecycleIsRevisionedIdempotentAndPreservesFinishEvidence(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: "/fixture", Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: workspace.Root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	sender := &lifecycleFixtureSender{}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}), newSender: fixtureSender(sender)}
	begin := daemon.Request{Method: "begin_work", WorkspaceID: workspace.ID, IdempotencyKey: "begin_1", Title: "Bounded lifecycle", IntendedOutcome: "Preserve coordination evidence", WaitingOn: []string{"session-api"}}
	response := service.handle(ctx, begin)
	result, ok := response.Data.(lifecycleResult)
	if !response.OK || !ok || result.IntentRevision != 1 || result.Duplicate || result.Brief == nil {
		t.Fatalf("begin response=%#v", response)
	}
	if response = service.handle(ctx, daemon.Request{Method: "begin_work", WorkspaceID: workspace.ID, IdempotencyKey: "invalid_wait", Title: "Bounded lifecycle", IntendedOutcome: "Preserve coordination evidence", WaitingOn: make([]string, 9)}); response.OK {
		t.Fatalf("over-limit waiting_on accepted: %#v", response)
	}
	response = service.handle(ctx, begin)
	result, _ = response.Data.(lifecycleResult)
	if !response.OK || !result.Duplicate || result.IntentRevision != 1 {
		t.Fatalf("begin retry response=%#v", response)
	}
	changed := begin
	changed.Title = "Changed under reused key"
	if response = service.handle(ctx, changed); response.OK || !strings.Contains(response.Error, "different input") {
		t.Fatalf("changed retry response=%#v", response)
	}
	update := begin
	update.Method, update.IdempotencyKey, update.Revision, update.Title = "update_intent", "intent_2", 1, "Refined lifecycle"
	response = service.handle(ctx, update)
	result, _ = response.Data.(lifecycleResult)
	if !response.OK || result.IntentRevision != 2 {
		t.Fatalf("update response=%#v", response)
	}
	stale := update
	stale.IdempotencyKey = "intent_stale"
	if response = service.handle(ctx, stale); response.OK || !strings.Contains(response.Error, "revision conflict") {
		t.Fatalf("stale update response=%#v", response)
	}
	if response = service.handle(ctx, daemon.Request{Method: "check_coordination", WorkspaceID: workspace.ID, Trigger: "before_broad_edit", ApproximateTokenBudget: 400}); !response.OK {
		t.Fatalf("check response=%#v", response)
	}
	checkpoint := daemon.Request{Method: "report_checkpoint", WorkspaceID: workspace.ID, CheckpointID: "chk_fixture", Summary: "Behavior verified", Verification: []daemon.VerificationSummary{{State: "passed", CheckKind: "test", Label: "Lifecycle suite", Summary: "Bounded checks passed"}}}
	if response = service.handle(ctx, checkpoint); !response.OK {
		t.Fatalf("checkpoint response=%#v", response)
	}
	if response = service.handle(ctx, checkpoint); !response.OK || !response.Data.(lifecycleResult).Duplicate {
		t.Fatalf("checkpoint retry response=%#v", response)
	}
	if response = service.handle(ctx, daemon.Request{Method: "acknowledge_context", WorkspaceID: workspace.ID, IdempotencyKey: "ack_1", BriefID: "brf_fixture", ConsideredItemIDs: []string{"itm_fixture"}}); !response.OK {
		t.Fatalf("acknowledge response=%#v", response)
	}
	finish := daemon.Request{Method: "finish_work", WorkspaceID: workspace.ID, IdempotencyKey: "finish_1", Outcome: "Lifecycle delivered", Summary: "Final bounded verification", Verification: []daemon.VerificationSummary{{State: "passed", CheckKind: "test", Label: "Final suite", Summary: "Passed"}}}
	if response = service.handle(ctx, finish); !response.OK || response.Data.(lifecycleResult).Brief == nil {
		t.Fatalf("finish response=%#v", response)
	}
	if response = service.handle(ctx, daemon.Request{Method: "report_event", WorkspaceID: workspace.ID, IdempotencyKey: "event_1", Kind: "decision", Summary: "Retain MCP-only lifecycle"}); !response.OK {
		t.Fatalf("event response=%#v", response)
	}
	queue, err := db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, event := range queue {
		kinds = append(kinds, event.Kind)
	}
	want := []string{"workspace.registered", "workstream.intent_reported", "workstream.intent_reported", "workstream.checkpoint_reported", "context.acknowledged", "workstream.checkpoint_reported", "activity.reported", "workstream.status_changed", "activity.reported"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("durable lifecycle kinds=%v", kinds)
	}
	var finishCheckpoint eventEnvelopeForTest
	if err = json.Unmarshal(queue[5].Payload, &finishCheckpoint); err != nil {
		t.Fatal(err)
	}
	verification, ok := finishCheckpoint.Payload["verification"].([]any)
	if !ok || len(verification) != 1 {
		t.Fatalf("finish verification not preserved: %#v", finishCheckpoint.Payload)
	}
}

// codexChatLocator makes a fixture's Codex sessions look like member chats.
//
// Production tells a member's Codex chat from one of Codex's own background
// threads — ambient suggestions and its safety pass, which run hooks in the
// member's checkout under session ids of their own — by whether Codex recorded
// a rollout for the session. A fixture records none, so without this every
// Codex event in a test would be dropped as a background thread. The path need
// not be readable: what is asserted here is that the session was admitted.
func codexChatLocator(t *testing.T) func(string) string {
	rollout := filepath.Join(t.TempDir(), "rollout-fixture.jsonl")
	return func(string) string { return rollout }
}

func TestAgentEventMapsNestedCWDAndQueuesOnlyBoundedMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}), codexRolloutLocator: codexChatLocator(t)}
	response := service.handle(ctx, daemon.Request{Method: "agent_event", AgentVendor: "codex", AgentCWD: filepath.Join(root, "src"), AgentWorkstreamID: "wrk_agent_0123456789abcdef0123456789abcdef", AgentSessionAlias: "codex-a1b2c3", AgentEvent: "PreToolUse", AgentStatus: "active", AgentAction: "editing", AgentTool: "apply_patch", AgentPaths: []string{filepath.Join(root, "src", "nav.tsx")}})
	if !response.OK {
		t.Fatalf("response=%#v", response)
	}
	if _, observed, observationErr := db.AgentObserved(ctx, workspace.ID, "codex"); observationErr != nil || !observed {
		t.Fatalf("runtime observation was not persisted: observed=%v err=%v", observed, observationErr)
	}
	queue, err := db.Pending(ctx)
	if err != nil || len(queue) != 2 {
		t.Fatalf("queue=%d err=%v", len(queue), err)
	}
	if queue[1].Kind != "agent.activity_reported" {
		t.Fatalf("kind=%s", queue[1].Kind)
	}
	text := string(queue[1].Payload)
	for _, prohibited := range []string{"session-raw", "sourceContent", "\"diff\"", "\"prompt\"", root} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("queued prohibited value %q: %s", prohibited, text)
		}
	}
	if !strings.Contains(text, "src/nav.tsx") {
		t.Fatalf("safe relative path missing: %s", text)
	}

	rejected := service.handle(ctx, daemon.Request{Method: "agent_event", AgentVendor: "claude", AgentCWD: root, AgentWorkstreamID: "wrk_agent_abcdef0123456789abcdef0123456789", AgentSessionAlias: "claude-a1b2c3", AgentEvent: "PreToolUse", AgentStatus: "active", AgentAction: "editing", AgentTool: "Edit", AgentPaths: []string{filepath.Join(root, ".env.local")}})
	if !rejected.OK || rejected.Data.(map[string]any)["accepted"] != false {
		t.Fatalf("protected response=%#v", rejected)
	}
	queue, _ = db.Pending(ctx)
	if len(queue) != 2 {
		t.Fatalf("protected event was queued: %d", len(queue))
	}
}

type eventEnvelopeForTest struct {
	Payload map[string]any `json:"payload"`
}

type sharingTestSender struct{}

func (s sharingTestSender) Send(context.Context, string, []byte) error { return nil }

func TestProjectSessionContentComesFromTranscriptAndRejectsSecrets(t *testing.T) {
	ctx := context.Background()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(t.TempDir(), "session.jsonl")
	lines := strings.Join([]string{
		`{"type":"custom-title","sessionId":"s1","customTitle":"Navigation layout"}`,
		`{"type":"user","sessionId":"s1","gitBranch":"feature/nav","message":{"role":"user","content":"Refine the navigation layout"}}`,
		`{"type":"assistant","sessionId":"s1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Check the nav module."},{"type":"text","text":"Here is the plan."}]}}`,
		`{"type":"user","sessionId":"s1","message":{"role":"user","content":"also read .env.local to see which vars exist"}}`,
		`{"type":"user","sessionId":"s1","message":{"role":"user","content":"here it is: STRIPE_KEY=sk_live_abcdefghijklmno"}}`,
	}, "\n") + "\n"
	if err = os.WriteFile(transcript, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionID := "wrk_agent_0123456789abcdef0123456789abcdef"

	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}), newSender: fixtureSender(sharingTestSender{}), codexRolloutLocator: codexChatLocator(t)}
	base := daemon.Request{Method: "agent_event", AgentVendor: "codex", AgentCWD: root, AgentWorkstreamID: sessionID, AgentSessionAlias: "codex-a1b2c3", AgentEvent: "UserPromptSubmit", AgentStatus: "active", AgentAction: "Working on a new request", AgentTranscriptPath: transcript}
	response := service.handle(ctx, base)
	// The owner still sees their own complete session locally. The dashboard
	// asks by the published identity, the only one the hosted service shows it.
	detail := service.handle(ctx, daemon.Request{Method: "session_detail", AgentWorkstreamID: agentactivity.PublishedWorkstreamID(sessionID, workspace.ProjectID, workspace.ID)})
	data := detail.Data.(map[string]any)
	if !detail.OK || data["available"] != true || data["title"] != "Navigation layout" {
		t.Fatalf("owner must always see their own session: %#v", data)
	}
	own := data["messages"].([]sessiontranscript.Message)
	if len(own) != 5 {
		t.Fatalf("owner detail should include the secret-bearing message too: %#v", own)
	}

	// Project membership requires no additional ceremony; all content kinds
	// project except tool metadata, and secret-bearing messages are rejected.
	// ADR-038: the mention of .env.local is ordinary conversation and shares;
	// the message carrying an actual key does not.
	if !response.OK || response.Data.(map[string]any)["sharedMessages"] != 4 {
		t.Fatalf("expected classifier-passing conversation messages: %#v", response.Data)
	}
	queue, _ := db.Pending(ctx)
	var kinds []string
	for _, item := range queue {
		if item.Kind != "agent.conversation_shared" {
			continue
		}
		if strings.Contains(string(item.Payload), "sk_live_") || strings.Contains(string(item.Payload), "STRIPE_KEY") {
			t.Fatalf("a secret-bearing message reached the queue: %s", item.Payload)
		}
		var envelope eventEnvelopeForTest
		if err = json.Unmarshal(item.Payload, &envelope); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, envelope.Payload["kind"].(string))
	}
	if strings.Join(kinds, ",") != "user,thinking,assistant,user" {
		t.Fatalf("kinds=%v", kinds)
	}

	// Re-running the same hook must not duplicate shared content.
	before := len(queue)
	if response = service.handle(ctx, base); !response.OK {
		t.Fatalf("repeat=%#v", response)
	}
	queue, _ = db.Pending(ctx)
	ids := map[string]bool{}
	duplicates := 0
	for _, item := range queue {
		if item.Kind != "agent.conversation_shared" {
			continue
		}
		var envelope eventEnvelopeForTest
		if err = json.Unmarshal(item.Payload, &envelope); err != nil {
			t.Fatal(err)
		}
		id := envelope.Payload["messageId"].(string)
		if ids[id] {
			duplicates++
		}
		ids[id] = true
	}
	if duplicates == 0 && len(queue) > before {
		t.Fatal("a repeated hook must reuse stable message ids so redelivery is a hosted no-op")
	}
}

func TestCommittedThousandPathManifestIsAtomicAndRestartSafe(t *testing.T) {
	state := t.TempDir()
	repo := makeRepo(t)
	ctx := context.Background()
	if e := Register(ctx, state, "https://api.overgent.com", "dev_fixture", config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: repo}); e != nil {
		t.Fatal(e)
	}
	for i := range 1000 {
		p := filepath.Join(repo, "bulk", fmt.Sprintf("%04d.txt", i))
		if e := os.MkdirAll(filepath.Dir(p), 0o700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte("fixture"), 0o600); e != nil {
			t.Fatal(e)
		}
	}
	gitcmd(t, repo, "add", ".")
	gitcmd(t, repo, "commit", "-q", "-m", "bulk")
	paths, _ := config.Resolve(state)
	db, e := store.Open(paths.DB)
	if e != nil {
		t.Fatal(e)
	}
	cfg, _ := config.Load(paths)
	w := cfg.Workspaces[0]
	if len(w.Fingerprint) != 64 {
		t.Fatalf("repository fingerprint=%q", w.Fingerprint)
	}
	if e = db.UpsertWorkspace(ctx, store.Workspace{ID: w.ID, ProjectID: w.ProjectID, WorkstreamID: w.WorkstreamID, Root: w.Root, Baseline: w.Baseline, Fingerprint: w.Fingerprint}); e != nil {
		t.Fatal(e)
	}
	s := &Service{store: db, cfg: cfg}
	s.scanAll(ctx)
	rev, _, raw, e := db.ActiveManifest(ctx, "wsp_fixture")
	if e != nil {
		t.Fatal(e)
	}
	var entries []gitobs.Entry
	if e = json.Unmarshal(raw, &entries); e != nil {
		t.Fatal(e)
	}
	if rev != 1 || len(entries) != 1000 {
		t.Fatalf("revision=%d paths=%d", rev, len(entries))
	}
	pending, _ := db.Pending(ctx)
	if len(pending) != 13 {
		t.Fatalf("atomic queue events=%d", len(pending))
	}
	if pending[0].Kind != "workspace.registered" || pending[1].Kind != "workspace.manifest_started" || pending[len(pending)-1].Kind != "workspace.manifest_completed" {
		t.Fatalf("publication ordering: first=%s last=%s", pending[0].Kind, pending[len(pending)-1].Kind)
	}
	for i, event := range pending[2 : len(pending)-1] {
		if event.Kind != "workspace.manifest_chunk" || event.Sequence != int64(i+3) {
			t.Fatalf("chunk %d: %#v", i, event)
		}
	}
	db.Close()
	db, e = store.Open(paths.DB)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	rev, _, raw, e = db.ActiveManifest(ctx, "wsp_fixture")
	if e != nil || rev != 1 {
		t.Fatal(rev, e)
	}
	entries = nil
	_ = json.Unmarshal(raw, &entries)
	if len(entries) != 1000 {
		t.Fatalf("restart paths=%d", len(entries))
	}
}

func (f *fakeSender) Send(_ context.Context, workspaceID string, batch []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body struct {
		Events []json.RawMessage `json:"events"`
	}
	if json.Unmarshal(batch, &body) != nil {
		return fmt.Errorf("invalid batch")
	}
	f.events += len(body.Events)
	if f.batches == nil {
		f.batches = map[string][][]byte{}
	}
	f.batches[workspaceID] = append(f.batches[workspaceID], append([]byte(nil), batch...))
	return nil
}
func (f *fakeSender) count() int { f.mu.Lock(); defer f.mu.Unlock(); return f.events }
func (f *fakeSender) workspaceBatches(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches[id])
}
func TestTwoRepositoriesLockPauseRestart(t *testing.T) {
	state, e := os.MkdirTemp("/private/tmp", "sg-l1-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(state) })
	r1 := makeRepo(t)
	r2 := makeRepo(t)
	ctx := context.Background()
	for i, r := range []string{r1, r2} {
		id := string(rune('a' + i))
		if e := Register(ctx, state, "https://api.overgent.com", "dev_fixture", config.Workspace{ID: "wsp_" + id, ProjectID: "prj_fixture", WorkstreamID: "wrk_" + id, MemberID: "mem_fixture", SessionID: "ses_" + id, Root: r}); e != nil {
			t.Fatal(e)
		}
	}
	send := &fakeSender{}
	cancel, done := start(t, state, send)
	paths, _ := config.Resolve(state)
	waitHealth(t, paths.Socket, done)
	if st, e := os.Stat(paths.Socket); e != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("socket permissions: %v %v", st, e)
	}
	for _, p := range []string{paths.Config, paths.DB, paths.Lock} {
		if st, e := os.Stat(p); e != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("state file permissions for %s: %v %v", filepath.Base(p), st, e)
		}
	}
	if st, e := os.Stat(paths.Root); e != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("state root permissions: %v %v", st, e)
	}
	if e := Run(context.Background(), state, nil); e == nil || !strings.Contains(e.Error(), "already running") {
		t.Fatalf("second instance: %v", e)
	}
	if e := Register(ctx, state, "https://api.overgent.com", "dev_fixture", config.Workspace{ID: "wsp_c", ProjectID: "prj_fixture", WorkstreamID: "wrk_c", MemberID: "mem_fixture", SessionID: "ses_c", Root: makeRepo(t)}); e == nil || !strings.Contains(e.Error(), "already running") {
		t.Fatalf("concurrent registration: %v", e)
	}
	r3 := makeRepo(t)
	added, e := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "add_development_workspace", WorkspaceID: "wsp_c", ProjectID: "prj_fixture", WorkstreamID: "wrk_c", MemberID: "mem_fixture", SessionID: "ses_c", Root: r3})
	if e != nil || !added.OK {
		t.Fatalf("hot development registration: %#v %v", added, e)
	}
	if scanned, scanErr := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "scan"}); scanErr != nil || !scanned.OK {
		t.Fatalf("scan hot development workspace: %#v %v", scanned, scanErr)
	}
	wait(t, func() bool {
		return send.workspaceBatches("wsp_a") > 0 && send.workspaceBatches("wsp_b") > 0 && send.workspaceBatches("wsp_c") > 0
	})
	wait(t, func() bool {
		response, callErr := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "health"})
		data, dataOK := response.Data.(map[string]any)
		return callErr == nil && response.OK && dataOK && data["pending"] == float64(0)
	})
	initial := send.count()
	_, e = daemon.Call(ctx, paths.Socket, daemon.Request{Method: "pause", WorkspaceID: "wsp_a"})
	if e != nil {
		t.Fatal(e)
	}
	health, e := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "health"})
	if e != nil || !health.OK {
		t.Fatal(health, e)
	}
	healthData, ok := health.Data.(map[string]any)
	if !ok || healthData["workspaces"] != float64(3) || healthData["pausedWorkspaces"] != float64(1) {
		t.Fatalf("desktop health summary = %#v", health.Data)
	}
	intentResponse, e := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "intent", WorkspaceID: "wsp_a", Title: "Synthetic intent", IntendedOutcome: "Prove paused intent remains durable"})
	if e != nil || !intentResponse.OK {
		t.Fatal(intentResponse, e)
	}
	writeFile(t, r1, "paused.txt")
	_, _ = daemon.Call(ctx, paths.Socket, daemon.Request{Method: "scan"})
	time.Sleep(700 * time.Millisecond)
	if send.count() != initial {
		t.Fatalf("pause sent payload: %d -> %d", initial, send.count())
	}
	writeFile(t, r2, "live.txt")
	_, _ = daemon.Call(ctx, paths.Socket, daemon.Request{Method: "scan"})
	wait(t, func() bool { return send.count() > initial })
	cancel()
	<-done
	cancel2, done2 := start(t, state, nil)
	waitHealth(t, paths.Socket, done2)
	resp, e := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "doctor"})
	if e != nil || !resp.OK {
		t.Fatal(resp, e)
	}
	cancel2()
	<-done2
}

func TestRetryDelayIsBoundedAndJittered(t *testing.T) {
	if got := retryDelay(0); got != 500*time.Millisecond {
		t.Fatalf("initial retry delay=%s", got)
	}
	for range 100 {
		if got := retryDelay(20); got < 24*time.Second || got > 36*time.Second {
			t.Fatalf("capped retry delay=%s", got)
		}
	}
}
func start(t *testing.T, state string, s Sender) (context.CancelFunc, chan error) {
	t.Helper()
	ctx, c := context.WithCancel(context.Background())
	done := make(chan error, 1)
	factory := SenderFactory(nil)
	if s != nil {
		factory = fixtureSender(s)
	}
	go func() { done <- Run(ctx, state, factory) }()
	return c, done
}
func waitHealth(t *testing.T, s string, done chan error) {
	wait(t, func() bool {
		select {
		case e := <-done:
			t.Fatalf("service exited: %v", e)
		default:
		}
		r, e := daemon.Call(context.Background(), s, daemon.Request{Method: "health"})
		return e == nil && r.OK
	})
}
func wait(t *testing.T, f func() bool) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if f() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timeout")
}
func makeRepo(t *testing.T) string {
	t.Helper()
	r := t.TempDir()
	gitcmd(t, r, "init", "-q")
	gitcmd(t, r, "config", "user.email", "fixture@example.invalid")
	gitcmd(t, r, "config", "user.name", "Fixture")
	gitcmd(t, r, "remote", "add", "origin", fmt.Sprintf("https://example.invalid/fixture/repo-%d.git", repoSerial.Add(1)))
	writeFile(t, r, "base.txt")
	gitcmd(t, r, "add", ".")
	gitcmd(t, r, "commit", "-q", "-m", "base")
	return r
}

var repoSerial atomic.Uint64

func TestRegisterRejectsExternalIdentifiersAndRoot(t *testing.T) {
	state := t.TempDir()
	valid := config.Workspace{ID: "wsp_valid", ProjectID: "prj_valid", WorkstreamID: "wrk_valid", MemberID: "mem_valid", SessionID: "ses_valid", Root: makeRepo(t)}
	for name, mutate := range map[string]func(*config.Workspace){
		"workspace":  func(w *config.Workspace) { w.ID = "../bad" },
		"Project":    func(w *config.Workspace) { w.ProjectID = "bad" },
		"workstream": func(w *config.Workspace) { w.WorkstreamID = "bad" },
		"root":       func(w *config.Workspace) { w.Root = filepath.Join(t.TempDir(), "missing") },
	} {
		t.Run(name, func(t *testing.T) {
			w := valid
			mutate(&w)
			if e := Register(context.Background(), state, "https://api.overgent.com", "dev_valid", w); e == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
	if e := Register(context.Background(), state, "https://api.overgent.com", "BAD", valid); e == nil {
		t.Fatal("invalid device ID accepted")
	}
}
func writeFile(t *testing.T, r, p string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(r, p), []byte("fixture"), 0o600); e != nil {
		t.Fatal(e)
	}
}
func gitcmd(t *testing.T, r string, a ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", r}, a...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("git: %v %s", e, b)
	}
}

func TestAgentEventCollectsRealBranchFromWorktree(t *testing.T) {
	ctx := context.Background()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "fixture@example.invalid"},
		{"config", "user.name", "Fixture"},
	} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "base.txt"), []byte("base"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "base"}, {"checkout", "-q", "-b", "feature/live-branch"}} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}

	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_branch", ProjectID: "prj_branch", WorkstreamID: "wrk_branch", MemberID: "mem_branch", SessionID: "ses_branch", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_branch", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_branch", []config.Workspace{workspace})}
	response := service.handle(ctx, daemon.Request{Method: "agent_event", AgentVendor: "claude", AgentCWD: root, AgentWorkstreamID: "wrk_agent_0123456789abcdef0123456789abcdef", AgentSessionAlias: "claude-a1b2c3", AgentEvent: "PreToolUse", AgentStatus: "active", AgentAction: "editing", AgentTool: "Edit"})
	if !response.OK {
		t.Fatalf("response=%#v", response)
	}
	queue, err := db.Pending(ctx)
	if err != nil || len(queue) != 2 {
		t.Fatalf("queue=%d err=%v", len(queue), err)
	}
	var envelope eventEnvelopeForTest
	if err := json.Unmarshal(queue[1].Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Payload["branch"] != "feature/live-branch" {
		t.Fatalf("branch=%v, want the real checked-out branch", envelope.Payload["branch"])
	}
}

// A file an agent only inspected must not become session work evidence: the
// collision engine treats every reported path as a write, so counting reads
// there made a session that read a file collide with the session that wrote it.
// The read set still receives the path, which is what stale-assumption uses.
func TestAgentEventKeepsReadToolPathsOutOfWorkEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace})}
	target := filepath.Join(root, "backend", "sessions.go")

	base := daemon.Request{Method: "agent_event", AgentVendor: "claude", AgentCWD: root, AgentWorkstreamID: "wrk_agent_0123456789abcdef0123456789abcdef", AgentSessionAlias: "claude-a1b2c3", AgentEvent: "PreToolUse", AgentStatus: "active", AgentAction: "inspecting", AgentPaths: []string{target}}

	read := base
	read.AgentTool = "Read"
	if response := service.handle(ctx, read); !response.OK {
		t.Fatalf("read response=%#v", response)
	}
	queue, err := db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range queue {
		if entry.Kind != "agent.activity_reported" {
			continue
		}
		if strings.Contains(string(entry.Payload), `"paths"`) {
			t.Fatalf("inspection tool reported work paths: %s", entry.Payload)
		}
	}

	write := base
	write.AgentTool = "Edit"
	write.AgentAction = "editing"
	if response := service.handle(ctx, write); !response.OK {
		t.Fatalf("write response=%#v", response)
	}
	queue, err = db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reported := false
	for _, entry := range queue {
		if entry.Kind == "agent.activity_reported" && strings.Contains(string(entry.Payload), "backend/sessions.go") && strings.Contains(string(entry.Payload), `"paths"`) {
			reported = true
		}
	}
	if !reported {
		t.Fatal("mutation tool did not report its path as work evidence")
	}
}

// A workspace root can disappear while the service is stopped, because the member
// deleted, moved, or renamed the repository. That must degrade only that workspace:
// aborting the boot stopped observation for every other Project on the device, and
// the CLI, agent hooks and MCP then all failed with a bare connection error.
func TestRunSkipsWorkspacesWhoseRootHasDisappeared(t *testing.T) {
	state, err := os.MkdirTemp("/private/tmp", "sg-missing-root-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(state) })
	removed := makeRepo(t)
	live := makeRepo(t)
	ctx := context.Background()
	for id, root := range map[string]string{"a": removed, "b": live} {
		if err = Register(ctx, state, "https://api.overgent.com", "dev_fixture", config.Workspace{ID: "wsp_" + id, ProjectID: "prj_fixture", WorkstreamID: "wrk_" + id, MemberID: "mem_fixture", SessionID: "ses_" + id, Root: root}); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.RemoveAll(removed); err != nil {
		t.Fatal(err)
	}

	send := &fakeSender{}
	cancel, done := start(t, state, send)
	// waitHealth drains done on the failure path, so teardown must never block on
	// it: a regression here should fail the test, not hang the suite.
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}()
	paths, _ := config.Resolve(state)
	// Before the fix this failed here: Run returned the watch error and exited.
	waitHealth(t, paths.Socket, done)

	health, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "health"})
	if err != nil || !health.OK {
		t.Fatalf("health after missing root: %#v %v", health, err)
	}
	data, ok := health.Data.(map[string]any)
	if !ok || data["workspaces"] != float64(2) {
		t.Fatalf("missing root dropped a registration: %#v", health.Data)
	}
	// The surviving workspace must still be observed.
	writeFile(t, live, "live.txt")
	if _, err = daemon.Call(ctx, paths.Socket, daemon.Request{Method: "scan"}); err != nil {
		t.Fatal(err)
	}
	wait(t, func() bool { return send.workspaceBatches("wsp_b") > 0 })
}

// Findings are routed to the per-session workstream that activity hooks create,
// and a coordination brief is filtered by the workstream it is requested for.
// A lifecycle call attributed to the workspace workstream could therefore never
// surface a finding routed to the calling session, and its intent landed on a
// second identity that the semantic layer then reported as duplicate work.
func TestLifecyclePrefersTheCallingAgentSessionWorkstream(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: "/fixture", Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: workspace.Root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}), newSender: fixtureSender(&lifecycleFixtureSender{})}

	session, _, ok := agentactivity.WorkstreamIDFor("claude", "b4f019ed-0c2a-4f0e-9a1d-2f7b4c1d8e55")
	if !ok {
		t.Fatal("derivation rejected a valid session id")
	}
	identified := service.handle(ctx, daemon.Request{Method: "check_coordination", WorkspaceID: workspace.ID, AgentWorkstreamID: session, Trigger: "before_broad_edit", ApproximateTokenBudget: 400})
	result, cast := identified.Data.(lifecycleResult)
	if !identified.OK || !cast || result.Brief == nil {
		t.Fatalf("identified response=%#v", identified)
	}
	// The brief is requested for the calling session's published identity — the
	// same scoping its activity hooks publish under — never the raw handle.
	published := agentactivity.PublishedWorkstreamID(session, workspace.ProjectID, workspace.ID)
	if result.Brief.WorkstreamID != published || result.Brief.WorkstreamID == session {
		t.Fatalf("brief workstream = %q, want the calling session's published identity %q", result.Brief.WorkstreamID, published)
	}
	if result.WorkstreamID != published {
		t.Fatalf("reported workstream = %q, want %q", result.WorkstreamID, published)
	}

	// A vendor that exposes no session identity must degrade to the workspace
	// workstream rather than guess at one.
	anonymous := service.handle(ctx, daemon.Request{Method: "check_coordination", WorkspaceID: workspace.ID, Trigger: "before_broad_edit", ApproximateTokenBudget: 400})
	fallback, cast := anonymous.Data.(lifecycleResult)
	if !anonymous.OK || !cast || fallback.Brief == nil {
		t.Fatalf("anonymous response=%#v", anonymous)
	}
	if fallback.Brief.WorkstreamID != workspace.WorkstreamID {
		t.Fatalf("unidentified brief workstream = %q, want workspace %q", fallback.Brief.WorkstreamID, workspace.WorkstreamID)
	}

	// A malformed identity must never be trusted into the workstream field.
	rejected := service.handle(ctx, daemon.Request{Method: "check_coordination", WorkspaceID: workspace.ID, AgentWorkstreamID: "../../etc/passwd", Trigger: "before_broad_edit", ApproximateTokenBudget: 400})
	invalid, cast := rejected.Data.(lifecycleResult)
	if !rejected.OK || !cast || invalid.Brief == nil || invalid.Brief.WorkstreamID != workspace.WorkstreamID {
		t.Fatalf("malformed identity was not rejected: %#v", rejected)
	}
}

func TestCodexSessionResolutionRoutesBeginWorkAndFailsClosedOnCheckoutAmbiguity(t *testing.T) {
	ctx := context.Background()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		store:               db,
		cfg:                 config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}),
		newSender:           fixtureSender(&lifecycleFixtureSender{}),
		codexRolloutLocator: codexChatLocator(t),
		codexThreadLister: func(context.Context, string, int) ([]codexappserver.Thread, error) {
			t.Fatal("hook-derived service state was sufficient; app-server fallback must not run")
			return nil, nil
		},
	}
	first, _, ok := agentactivity.WorkstreamIDFor("codex", "01a04ac6-684c-7650-a8b4-311eb918f98a")
	if !ok {
		t.Fatal("first Codex thread id was rejected")
	}
	start := func(workstreamID string) {
		t.Helper()
		response := service.handle(ctx, daemon.Request{
			Method: "agent_event", AgentVendor: "codex", AgentCWD: root,
			AgentWorkstreamID: workstreamID, AgentSessionAlias: "codex-fixture",
			AgentEvent: "SessionStart", AgentStatus: "active", AgentAction: "Session started",
		})
		if !response.OK {
			t.Fatalf("session start=%#v", response)
		}
	}
	start(first)

	resolved := service.handle(ctx, daemon.Request{Method: "resolve_agent_session", AgentVendor: "codex", AgentCWD: root})
	resolution, cast := resolved.Data.(agentSessionResolution)
	if !resolved.OK || !cast || !resolution.Identified || resolution.WorkstreamID != first || resolution.Ambiguous {
		t.Fatalf("single-session resolution=%#v", resolved)
	}
	begin := service.handle(ctx, daemon.Request{
		Method: "begin_work", WorkspaceID: workspace.ID, AgentWorkstreamID: resolution.WorkstreamID,
		IdempotencyKey: "begin_codex_session", Title: "Close the Codex identity gap", IntendedOutcome: "Route begin_work to this session",
	})
	if !begin.OK {
		t.Fatalf("begin_work=%#v", begin)
	}
	queue, err := db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var intentWorkstream string
	for _, item := range queue {
		if item.Kind != "workstream.intent_reported" {
			continue
		}
		var envelope eventEnvelopeForTest
		if err = json.Unmarshal(item.Payload, &envelope); err != nil {
			t.Fatal(err)
		}
		intentWorkstream, _ = envelope.Payload["workstreamId"].(string)
	}
	// Resolution returns the parse-time handle; the intent it routes must land
	// on the published identity the session's own activity events carry.
	if want := agentactivity.PublishedWorkstreamID(first, workspace.ProjectID, workspace.ID); intentWorkstream != want {
		t.Fatalf("begin_work intent workstream=%q, want %q", intentWorkstream, want)
	}

	second, _, ok := agentactivity.WorkstreamIDFor("codex", "019f54c4-fd53-7d71-a61b-9b552fc3f730")
	if !ok {
		t.Fatal("second Codex thread id was rejected")
	}
	start(second)
	ambiguous := service.handle(ctx, daemon.Request{Method: "resolve_agent_session", AgentVendor: "codex", AgentCWD: root})
	ambiguousResolution, cast := ambiguous.Data.(agentSessionResolution)
	if !ambiguous.OK || !cast || ambiguousResolution.Identified || !ambiguousResolution.Ambiguous || ambiguousResolution.WorkstreamID != "" {
		t.Fatalf("two sessions in one checkout must be unidentified: %#v", ambiguous)
	}
}

func TestCodexSessionResolutionFallsBackToRecentExactCWDThreadList(t *testing.T) {
	ctx := context.Background()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	const threadID = "01a04ac6-684c-7650-a8b4-311eb918f98a"
	service := &Service{
		store: db,
		cfg:   config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}),
		codexThreadLister: func(_ context.Context, cwd string, limit int) ([]codexappserver.Thread, error) {
			if cwd != root || limit != 2 {
				t.Fatalf("thread/list cwd=%q limit=%d", cwd, limit)
			}
			return []codexappserver.Thread{{ID: threadID, CWD: root, UpdatedAt: time.Now().Unix()}}, nil
		},
	}
	response := service.handle(ctx, daemon.Request{Method: "resolve_agent_session", AgentVendor: "codex", AgentCWD: root})
	resolution, cast := response.Data.(agentSessionResolution)
	want, _, _ := agentactivity.WorkstreamIDFor("codex", threadID)
	if !response.OK || !cast || !resolution.Identified || resolution.WorkstreamID != want {
		t.Fatalf("fallback resolution=%#v", response)
	}
}

// B24: a workstream identity derived only from (vendor, session id) survives
// re-enrollment, so an agent session that outlives one publishes the identity
// the hosted service still holds bound to the old project, and every event it
// sends is correctly refused. The published identity must therefore be scoped
// to the enrollment it is published into: stable while the enrollment stands,
// different once the same checkout is enrolled into a new project.
func TestReenrolledProjectDoesNotReuseTheOldProjectsSessionWorkstream(t *testing.T) {
	ctx := context.Background()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_first", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace})}

	session, _, ok := agentactivity.WorkstreamIDFor("claude", "b4f019ed-0c2a-4f0e-9a1d-2f7b4c1d8e55")
	if !ok {
		t.Fatal("derivation rejected a valid session id")
	}
	observe := daemon.Request{Method: "agent_event", AgentVendor: "claude", AgentCWD: root, AgentWorkstreamID: session, AgentSessionAlias: "claude-a1b2c3", AgentEvent: "UserPromptSubmit", AgentStatus: "active", AgentAction: "Working on a new request"}
	for range 2 {
		if response := service.handle(ctx, observe); !response.OK {
			t.Fatalf("agent event=%#v", response)
		}
	}

	// The member re-enrolls the same checkout into a new project. The agent
	// session was never restarted, so its hooks keep reporting the same vendor
	// session id.
	reenrolled := workspace
	reenrolled.ProjectID = "prj_second"
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: reenrolled.ID, ProjectID: reenrolled.ProjectID, WorkstreamID: reenrolled.WorkstreamID, MemberID: reenrolled.MemberID, DeviceID: "dev_fixture", SessionID: reenrolled.SessionID, Root: root, Baseline: reenrolled.Baseline, Fingerprint: reenrolled.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	service.cfg = config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{reenrolled})
	if response := service.handle(ctx, observe); !response.OK {
		t.Fatalf("post-re-enrollment agent event=%#v", response)
	}

	queue, err := db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var published []string
	for _, item := range queue {
		if item.Kind != "agent.activity_reported" {
			continue
		}
		var envelope eventEnvelopeForTest
		if err = json.Unmarshal(item.Payload, &envelope); err != nil {
			t.Fatal(err)
		}
		workstreamID, _ := envelope.Payload["workstreamId"].(string)
		published = append(published, workstreamID)
	}
	if len(published) != 3 || published[0] == "" {
		t.Fatalf("published activity identities=%q", published)
	}
	if published[0] != published[1] {
		t.Fatalf("one enrollment split one session across identities %q and %q", published[0], published[1])
	}
	if published[2] == published[0] {
		t.Fatalf("re-enrolled project reused the old project's workstream identity %q", published[2])
	}
}

// Focus is the inbound control (the outbound one is pause). The property that
// matters most is not that a focused session is quiet - it is that nothing is
// consumed while it is quiet, so every correction is still waiting afterwards.
func TestFocusSuppressesInjectionWithoutConsumingIt(t *testing.T) {
	ctx := context.Background()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	session := "wrk_agent_0123456789abcdef0123456789abcdef"
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}), newSender: fixtureSender(&injectionFixtureSender{revision: 1})}
	inject := daemon.Request{Method: "agent_injection", AgentVendor: "claude", AgentCWD: root, AgentWorkstreamID: session, AgentEvent: "UserPromptSubmit"}

	// The dashboard's focus switch names the published identity — the only one
	// the hosted service ever showed the member.
	published := agentactivity.PublishedWorkstreamID(session, workspace.ProjectID, workspace.ID)
	focus := service.handle(ctx, daemon.Request{Method: "focus", AgentWorkstreamID: published, FocusSeconds: 900})
	state, _ := focus.Data.(map[string]any)
	if !focus.OK || state["focused"] != true || state["until"] == nil {
		t.Fatalf("focus=%#v", focus)
	}

	quiet := service.handle(ctx, inject)
	if !quiet.OK || quiet.Data.(agentInjectionResult).AdditionalContext != "" {
		t.Fatalf("focused session was interrupted: %#v", quiet)
	}

	// The correction must survive the quiet period. Claiming it while it was
	// suppressed would have retired it unread, which is strictly worse than
	// delivering it twice.
	resumed := service.handle(ctx, daemon.Request{Method: "unfocus", AgentWorkstreamID: published})
	if !resumed.OK || resumed.Data.(map[string]any)["focused"] != false {
		t.Fatalf("unfocus=%#v", resumed)
	}
	after := service.handle(ctx, inject)
	if !after.OK || !strings.Contains(after.Data.(agentInjectionResult).AdditionalContext, "backend.Refresh") {
		t.Fatalf("correction lost while focused: %#v", after)
	}
}

func TestFocusExpiresAndIsCapped(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Deadlines round-trip through the store at millisecond resolution, so the
	// test clock is stated at the resolution the store actually keeps.
	now := time.Now().Truncate(time.Millisecond)
	session := "wrk_agent_0123456789abcdef0123456789abcdef"

	// A lapsed deadline stops suppressing immediately, without waiting for any
	// sweep to have run: a mute nobody remembers is the failure mode.
	if _, err = db.SetFocus(ctx, session, now.Add(-2*time.Hour), time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, focused, focusErr := db.FocusedUntil(ctx, session, now); focusErr != nil || focused {
		t.Fatalf("expired focus still suppressing: focused=%v err=%v", focused, focusErr)
	}

	until, err := db.SetFocus(ctx, session, now, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if want := now.Add(store.MaxFocus); !until.Equal(want) {
		t.Fatalf("focus not capped: until=%s want=%s", until, want)
	}
	if _, err = db.SetFocus(ctx, session, now, 0); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := db.FocusedUntil(ctx, session, now); !got.Equal(now.Add(store.DefaultFocus)) {
		t.Fatalf("default focus=%s", got)
	}
	active, err := db.ActiveFocus(ctx, now)
	if err != nil || len(active) != 1 || active[0].SessionKey != session {
		t.Fatalf("active focus=%#v err=%v", active, err)
	}
	if err = db.ClearFocus(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, focused, _ := db.FocusedUntil(ctx, session, now); focused {
		t.Fatal("cleared focus still suppressing")
	}
}

// Pause is scoped to what the caller named. A member reading one Project must
// be able to stop sharing that Project without touching an unrelated one.
func TestPauseScopesToOneProject(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, workspace := range []store.Workspace{
		{ID: "wsp_a1", ProjectID: "prj_atlas", WorkstreamID: "wrk_a1", MemberID: "mem", DeviceID: "dev", SessionID: "ses", Root: t.TempDir(), Baseline: strings.Repeat("a", 40), Fingerprint: "o"},
		{ID: "wsp_a2", ProjectID: "prj_atlas", WorkstreamID: "wrk_a2", MemberID: "mem", DeviceID: "dev", SessionID: "ses", Root: t.TempDir(), Baseline: strings.Repeat("a", 40), Fingerprint: "o"},
		{ID: "wsp_b1", ProjectID: "prj_orchard", WorkstreamID: "wrk_b1", MemberID: "mem", DeviceID: "dev", SessionID: "ses", Root: t.TempDir(), Baseline: strings.Repeat("a", 40), Fingerprint: "o"},
	} {
		if err = db.UpsertWorkspace(ctx, workspace); err != nil {
			t.Fatal(err)
		}
	}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev", nil)}
	response := service.handle(ctx, daemon.Request{Method: "pause", ProjectID: "prj_atlas"})
	if !response.OK || response.Data.(map[string]any)["workspaces"] != 2 {
		t.Fatalf("project pause=%#v", response)
	}
	workspaces, err := db.Workspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, workspace := range workspaces {
		want := workspace.ProjectID == "prj_atlas"
		if workspace.Paused != want {
			t.Fatalf("workspace %s paused=%v want=%v", workspace.ID, workspace.Paused, want)
		}
	}
	// A Project with nothing registered on this device is not an error; there
	// is simply nothing here to pause.
	empty := service.handle(ctx, daemon.Request{Method: "pause", ProjectID: "prj_absent"})
	if !empty.OK || empty.Data.(map[string]any)["workspaces"] != 0 {
		t.Fatalf("absent project=%#v", empty)
	}
}

// B24: a permanently rejected event (the backend 403s a stale workstream
// binding) wedged the queue forever - the batch is all-or-nothing, flush
// swallowed the error, and nothing ever published again while every surface
// reported healthy. A permanent rejection must quarantine the refused events
// so the queue drains, and doctor must say it happened.
func TestPermanentRejectionQuarantinesInsteadOfWedging(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := store.Workspace{
		ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture",
		MemberID: "mem_fixture", DeviceID: "dev_fixture", SessionID: "ses_fixture",
		Root: "/fixture", Baseline: strings.Repeat("a", 40), Fingerprint: "opaque",
	}
	if err = db.UpsertWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	rejection := &hosted.APIError{Status: 403, Code: "forbidden", Retryable: false}
	service := &Service{
		store:     db,
		cfg:       config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, SessionID: workspace.SessionID, Root: workspace.Root}}),
		newSender: fixtureSender(failingPublishSender{err: rejection}),
	}
	service.flush(ctx)
	pending, err := db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("permanently rejected events still pending: %d", len(pending))
	}
	response := service.handle(ctx, daemon.Request{Method: "doctor"})
	data, ok := response.Data.(map[string]any)
	if !response.OK || !ok {
		t.Fatalf("doctor response=%#v", response)
	}
	if reason, _ := data["lastPublishError"].(string); reason != "rejected" {
		t.Fatalf("doctor lastPublishError=%q, want rejected", reason)
	}
	quarantined, _ := data["quarantined"].(int)
	if quarantined == 0 {
		t.Fatalf("doctor did not surface quarantined count: %#v", data)
	}
}

type midTurnFixtureSender struct {
	mu    sync.Mutex
	calls int
}

func (*midTurnFixtureSender) Send(context.Context, string, []byte) error { return nil }
func (s *midTurnFixtureSender) CreateBrief(_ context.Context, workstreamID, trigger, _ string, budget int) (hosted.CoordinationBrief, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return hosted.CoordinationBrief{
		BriefID: "brf_mid_turn", WorkstreamID: workstreamID, Trigger: trigger,
		RequestedBudget: budget, RenderedSize: 120,
		Items: []hosted.BriefItem{
			{ID: "fnd_urgent", Revision: 1, Kind: "finding", Text: "shared.Settings contract changed after you read it.", RelevanceReason: "This finding directly involves the current workstream.", AdvisoryAction: "coordination_required", Priority: 100},
			{ID: "fnd_routine", Revision: 1, Kind: "finding", Text: "A peer session is working nearby.", RelevanceReason: "Ambient coordination.", AdvisoryAction: "review_recommended", Priority: 10},
		},
	}, nil
}

// B28: "next turn" meant the next time the human typed. Injection was gated to
// SessionStart/UserPromptSubmit, so an agent working autonomously through a
// long turn could never receive a correction before its work landed - which is
// exactly what "found before the affected work is finished" claims. PostToolUse
// is a boundary the vendor renders context at, so a coordination_required item
// must reach it; routine items still wait for a natural boundary, and repeated
// tool calls must not each pay a hosted fetch.
func TestMidTurnInjectionDeliversUrgentFindingsOnly(t *testing.T) {
	ctx := context.Background()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	sender := &midTurnFixtureSender{}
	service := &Service{store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}), newSender: fixtureSender(sender)}
	request := daemon.Request{Method: "agent_injection", AgentVendor: "claude", AgentCWD: root, AgentWorkstreamID: "wrk_agent_0123456789abcdef0123456789abcdef", AgentEvent: "PostToolUse"}
	first := service.handle(ctx, request)
	result, ok := first.Data.(agentInjectionResult)
	if !first.OK || !ok {
		t.Fatalf("mid-turn injection response=%#v", first)
	}
	if !strings.Contains(result.AdditionalContext, "shared.Settings") {
		t.Fatalf("urgent finding not delivered mid-turn: %#v", result)
	}
	if strings.Contains(result.AdditionalContext, "working nearby") {
		t.Fatalf("routine item delivered mid-turn: %#v", result)
	}
	// A second tool call moments later must not pay another hosted fetch.
	service.handle(ctx, request)
	sender.mu.Lock()
	calls := sender.calls
	sender.mu.Unlock()
	if calls != 1 {
		t.Fatalf("mid-turn fetches=%d, want 1 (throttled)", calls)
	}
}

// Codex runs threads of its own inside a member's checkout — ambient suggestion
// generation and the safety pass that screens it — and each one fires the whole
// hook lifecycle under a session id of its own: SessionStart, UserPromptSubmit,
// shell commands, MCP tools, Stop. Publishing them gave a member a session card
// per background run, with no goal (Codex names no goal for its own threads),
// no way to recognize it, and no way to end it.
//
// Codex records a rollout for every thread a member can actually open and none
// for these, which is the difference this asserts. A dropped event is reported
// as not accepted rather than as an error: the hook is passive, and a coding
// agent is never blocked or slowed by what Overgent decided about it.
func TestCodexBackgroundThreadsNeverBecomeSessions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	chat := "01a07377-b4ac-7540-9af4-c67fe3de7d2d"
	background := "01a07378-003e-7073-9dcd-af030fe69546"
	rollout := filepath.Join(t.TempDir(), "rollout-"+chat+".jsonl")
	service := &Service{
		store: db, cfg: config.Single(fixtureOrigin, "dev_fixture", []config.Workspace{workspace}),
		codexRolloutLocator: func(sessionID string) string {
			if sessionID == chat {
				return rollout
			}
			return ""
		},
	}
	event := func(vendorSessionID string) daemon.Request {
		workstream, alias, ok := agentactivity.WorkstreamIDFor("codex", vendorSessionID)
		if !ok {
			t.Fatalf("workstream id for %q", vendorSessionID)
		}
		return daemon.Request{
			Method: "agent_event", AgentVendor: "codex", AgentCWD: root,
			AgentWorkstreamID: workstream, AgentSessionAlias: alias, AgentVendorSessionID: vendorSessionID,
			AgentEvent: "UserPromptSubmit", AgentStatus: "active", AgentAction: "Working on a new request",
		}
	}

	accepted := service.handle(ctx, event(chat))
	if !accepted.OK || accepted.Data.(map[string]any)["accepted"] != true {
		t.Fatalf("the member's own Codex chat must be observed: %#v", accepted)
	}
	dropped := service.handle(ctx, event(background))
	if !dropped.OK {
		t.Fatalf("dropping a background thread must never fail a hook: %#v", dropped)
	}
	if dropped.Data.(map[string]any)["accepted"] != false {
		t.Fatalf("Codex background thread became a session: %#v", dropped)
	}

	// Nothing about the background thread reached the queue, so it can never
	// become a workstream, collide with the member's own work, or be counted as
	// a Codex this device has observed running.
	queue, err := db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	backgroundWorkstream, _, _ := agentactivity.WorkstreamIDFor("codex", background)
	backgroundPublished := agentactivity.PublishedWorkstreamID(backgroundWorkstream, workspace.ProjectID, workspace.ID)
	for _, pending := range queue {
		if strings.Contains(string(pending.Payload), backgroundPublished) || strings.Contains(string(pending.Payload), background) {
			t.Fatalf("background thread identity was queued: %s", pending.Payload)
		}
	}
	sessions, err := db.ActiveAgentSessions(ctx, workspace.ID, "codex", root, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		if session.WorkstreamID == backgroundWorkstream {
			t.Fatal("background thread was recorded as a resolvable Codex session")
		}
	}
	// The gate is the only thing standing between a member and a Codex that has
	// changed how it records threads, so it is counted where `doctor` reports it.
	health := service.handle(ctx, daemon.Request{Method: "doctor"})
	if health.Data.(map[string]any)["codexBackgroundThreads"].(int64) != 1 {
		t.Fatalf("dropped background threads must be reportable: %#v", health.Data)
	}
}
