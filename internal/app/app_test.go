//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	gitobs "github.com/stickguy/stickguy/internal/git"
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/stickguy/stickguy/internal/store"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSender struct {
	mu      sync.Mutex
	events  int
	batches map[string][][]byte
}

type lifecycleFixtureSender struct{ briefs int }

func (*lifecycleFixtureSender) Send(context.Context, string, []byte) error { return nil }
func (s *lifecycleFixtureSender) CreateBrief(_ context.Context, workstreamID, trigger, _ string, budget int) (hosted.CoordinationBrief, error) {
	s.briefs++
	return hosted.CoordinationBrief{BriefID: "brf_fixture", ProjectID: "prj_fixture", RepositoryID: "rep_fixture", WorkstreamID: workstreamID, Trigger: trigger, RequestedBudget: budget, Items: []hosted.BriefItem{{ID: "itm_fixture", Kind: "decision", Text: "Synthetic coordination item"}}}, nil
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
	service := &Service{store: db, cfg: config.Config{Version: 1, DeviceID: "dev_fixture", Workspaces: []config.Workspace{workspace}}, sender: sender}
	begin := daemon.Request{Method: "begin_work", WorkspaceID: workspace.ID, IdempotencyKey: "begin_1", Title: "Bounded lifecycle", IntendedOutcome: "Preserve coordination evidence"}
	response := service.handle(ctx, begin)
	result, ok := response.Data.(lifecycleResult)
	if !response.OK || !ok || result.IntentRevision != 1 || result.Duplicate || result.Brief == nil {
		t.Fatalf("begin response=%#v", response)
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

type eventEnvelopeForTest struct {
	Payload map[string]any `json:"payload"`
}

func TestCommittedThousandPathManifestIsAtomicAndRestartSafe(t *testing.T) {
	state := t.TempDir()
	repo := makeRepo(t)
	ctx := context.Background()
	if e := Register(ctx, state, "https://api.stickguy.dev", "dev_fixture", config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: repo}); e != nil {
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
		if e := Register(ctx, state, "https://api.stickguy.dev", "dev_fixture", config.Workspace{ID: "wsp_" + id, ProjectID: "prj_fixture", WorkstreamID: "wrk_" + id, MemberID: "mem_fixture", SessionID: "ses_" + id, Root: r}); e != nil {
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
	if e := Register(ctx, state, "https://api.stickguy.dev", "dev_fixture", config.Workspace{ID: "wsp_c", ProjectID: "prj_fixture", WorkstreamID: "wrk_c", MemberID: "mem_fixture", SessionID: "ses_c", Root: makeRepo(t)}); e == nil || !strings.Contains(e.Error(), "already running") {
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
	go func() { done <- Run(ctx, state, s) }()
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
			if e := Register(context.Background(), state, "https://api.stickguy.dev", "dev_valid", w); e == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
	if e := Register(context.Background(), state, "https://api.stickguy.dev", "BAD", valid); e == nil {
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
