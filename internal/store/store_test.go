package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	protocoltypes "github.com/stickguy/stickguy/protocol/generated/go"
)

func TestMigrationAddsRepositoryFingerprint(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.db")
	db, e := sql.Open("sqlite", p)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY);
INSERT INTO schema_migrations(version) VALUES(1);
CREATE TABLE workspaces(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,workstream_id TEXT NOT NULL,root TEXT NOT NULL UNIQUE,baseline TEXT NOT NULL,paused INTEGER NOT NULL DEFAULT 0,revision INTEGER NOT NULL DEFAULT 0,sequence INTEGER NOT NULL DEFAULT 0);`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	if e = s.UpsertWorkspace(ctx, Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", DeviceID: "dev_fixture", SessionID: "ses_fixture", Root: "/fixture", Baseline: "abc", Fingerprint: "opaque"}); e != nil {
		t.Fatal(e)
	}
	workspaces, e := s.Workspaces(ctx)
	if e != nil || len(workspaces) != 1 || workspaces[0].Fingerprint != "opaque" {
		t.Fatalf("migration result=%#v err=%v", workspaces, e)
	}
	var workstreams int
	if e := s.db.QueryRowContext(ctx, `SELECT count(*) FROM workstreams WHERE id='wrk_fixture'`).Scan(&workstreams); e != nil || workstreams != 1 {
		t.Fatalf("workstream migration count=%d err=%v", workstreams, e)
	}
}

func TestRegistrationEventIDIncludesDeviceScope(t *testing.T) {
	a := registrationEventID("prj_fixture", "dev_a", "wsp_default")
	b := registrationEventID("prj_fixture", "dev_b", "wsp_default")
	if a == b {
		t.Fatal("registration event IDs collide across devices")
	}
}

func TestAgentObservationPersistsRuntimeVerification(t *testing.T) {
	ctx := context.Background()
	state, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	observedAt := time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC)
	if err = state.RecordAgentObservation(ctx, "wsp_fixture", "codex", observedAt); err != nil {
		t.Fatal(err)
	}
	got, observed, err := state.AgentObserved(ctx, "wsp_fixture", "codex")
	if err != nil || !observed || !got.Equal(observedAt) {
		t.Fatalf("got=%v observed=%v err=%v", got, observed, err)
	}
	if _, observed, err = state.AgentObserved(ctx, "wsp_fixture", "claude"); err != nil || observed {
		t.Fatalf("unexpected Claude observation observed=%v err=%v", observed, err)
	}
	if err = state.ClearAgentObservation(ctx, "wsp_fixture", "codex"); err != nil {
		t.Fatal(err)
	}
	if _, observed, err = state.AgentObserved(ctx, "wsp_fixture", "codex"); err != nil || observed {
		t.Fatalf("cleared Codex observation remained observed=%v err=%v", observed, err)
	}
}

func TestInjectionDeliveriesDeduplicateExactRevisionAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	state, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	items := []InjectionItem{{ID: "fnd_fixture", Revision: 1}}
	claimed, err := state.ClaimInjectionDeliveries(ctx, "wrk_agent_fixture", items, time.Now())
	if err != nil || len(claimed) != 1 {
		t.Fatalf("first claim=%#v err=%v", claimed, err)
	}
	claimed, err = state.ClaimInjectionDeliveries(ctx, "wrk_agent_fixture", items, time.Now())
	if err != nil || len(claimed) != 0 {
		t.Fatalf("duplicate claim=%#v err=%v", claimed, err)
	}
	pending, err := state.UndeliveredInjectionItems(ctx, "wrk_agent_fixture", []InjectionItem{{ID: "fnd_fixture", Revision: 1}, {ID: "fnd_fixture", Revision: 2}})
	if err != nil || len(pending) != 1 || pending[0].Revision != 2 {
		t.Fatalf("pending revisions=%#v err=%v", pending, err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	claimed, err = state.ClaimInjectionDeliveries(ctx, "wrk_agent_fixture", []InjectionItem{{ID: "fnd_fixture", Revision: 2}}, time.Now())
	if err != nil || len(claimed) != 1 || claimed[0].Revision != 2 {
		t.Fatalf("revised claim=%#v err=%v", claimed, err)
	}
}

func TestLifecyclePublicationIsAtomicRevisionedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.UpsertWorkspace(ctx, Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", DeviceID: "dev_fixture", SessionID: "ses_fixture", Root: "/fixture", Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}); err != nil {
		t.Fatal(err)
	}
	publication := LifecyclePublication{
		WorkspaceID: "wsp_fixture", Method: "finish_work", IdempotencyKey: "finish_1", Source: "mcp",
		Kind: "workstream.checkpoint_reported", Payload: map[string]any{"checkpointId": "chk_finish_fixture", "workstreamId": "wrk_fixture", "summary": "verified", "verification": []map[string]any{{"state": "passed", "checkKind": "test", "label": "Lifecycle suite", "summary": "Passed", "source": "mcp", "observedAt": ""}}},
		Additional: []LifecycleEvent{
			{Kind: "activity.reported", Payload: map[string]any{"kind": "completion", "summary": "complete"}},
			{Kind: "workstream.status_changed", Payload: map[string]any{"workstreamId": "wrk_fixture", "status": "done"}},
		},
	}
	revision, duplicate, err := s.PublishLifecycle(ctx, publication)
	if err != nil || duplicate || revision != 0 {
		t.Fatalf("first publication revision=%d duplicate=%t err=%v", revision, duplicate, err)
	}
	queue, err := s.Pending(ctx)
	if err != nil || len(queue) != 4 {
		t.Fatalf("queue=%#v err=%v", queue, err)
	}
	if queue[1].Kind != "workstream.checkpoint_reported" || queue[2].Kind != "activity.reported" || queue[3].Kind != "workstream.status_changed" || queue[1].Sequence+1 != queue[2].Sequence || queue[2].Sequence+1 != queue[3].Sequence {
		t.Fatalf("lifecycle events are not an ordered atomic group: %#v", queue)
	}
	assertEnvelopeSchema(t, queue[1:])
	revision, duplicate, err = s.PublishLifecycle(ctx, publication)
	if err != nil || !duplicate || revision != 0 {
		t.Fatalf("retry revision=%d duplicate=%t err=%v", revision, duplicate, err)
	}
	publication.Additional[0].Payload = map[string]any{"kind": "completion", "summary": "changed"}
	if _, _, err = s.PublishLifecycle(ctx, publication); err == nil || !strings.Contains(err.Error(), "different input") {
		t.Fatalf("changed retry error=%v", err)
	}

	intent := LifecyclePublication{WorkspaceID: "wsp_fixture", Method: "begin_work", IdempotencyKey: "begin_1", Source: "mcp", Kind: "workstream.intent_reported", Payload: map[string]any{"workstreamId": "wrk_fixture", "title": "bounded", "intendedOutcome": "coordinate"}, IncrementIntentRevision: true}
	revision, duplicate, err = s.PublishLifecycle(ctx, intent)
	if err != nil || duplicate || revision != 1 {
		t.Fatalf("intent revision=%d duplicate=%t err=%v", revision, duplicate, err)
	}
	stale := int64(0)
	intent.Method, intent.IdempotencyKey, intent.ExpectedIntentRevision = "update_intent", "intent_2", &stale
	if revision, _, err = s.PublishLifecycle(ctx, intent); err == nil || revision != 1 || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("stale update revision=%d err=%v", revision, err)
	}
}

func TestRestartManifestQueueCursor(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.db")
	ctx := context.Background()
	s, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.UpsertWorkspace(ctx, Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", DeviceID: "dev_fixture", SessionID: "ses_fixture", Root: "/fixture", Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}); e != nil {
		t.Fatal(e)
	}
	entries := []map[string]any{{"path": "a", "states": map[string]any{"baseline": map[string]string{"status": "added"}}}}
	rev, e := s.PublishManifest(ctx, ManifestPublication{WorkspaceID: "wsp_fixture", ManifestID: "mft_fixture", Baseline: strings.Repeat("a", 40), Head: strings.Repeat("b", 40), Hash: strings.Repeat("c", 64), Entries: entries, EventID: "evt_fixture"})
	if e != nil || rev != 1 {
		t.Fatal(rev, e)
	}
	s.Close()
	s, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r, h, b, e := s.ActiveManifest(ctx, "wsp_fixture")
	if e != nil || r != 1 || h != strings.Repeat("c", 64) {
		t.Fatal(r, h, e)
	}
	var got []map[string]any
	if json.Unmarshal(b, &got) != nil || len(got) != 1 {
		t.Fatal(string(b))
	}
	q, _ := s.Pending(ctx)
	if len(q) != 4 || q[0].Kind != "workspace.registered" || q[1].Kind != "workspace.manifest_started" || q[2].Kind != "workspace.manifest_chunk" || q[3].ID != "evt_fixture" || q[3].Kind != "workspace.manifest_completed" {
		t.Fatal(q)
	}
	assertEnvelopeSchema(t, q)
	batchJSON, e := Batch(q)
	if e != nil {
		t.Fatal(e)
	}
	var batch protocoltypes.PublishEventBatchJSONBody
	if e = json.Unmarshal(batchJSON, &batch); e != nil || len(batch.Events) != 4 {
		t.Fatalf("generated batch decode: events=%d err=%v", len(batch.Events), e)
	}
	for i, event := range batch.Events {
		if event.ProjectId != "prj_fixture" || event.MemberId != "mem_fixture" || event.DeviceId != "dev_fixture" || event.WorkspaceId != "wsp_fixture" || event.SessionId != "ses_fixture" || event.Sequence != i+1 || event.SchemaVersion != float64(1) || event.ObservedAt.IsZero() || event.SentAt.IsZero() || !event.Source.Valid() || !event.Type.Valid() {
			t.Fatalf("invalid generated envelope %d: %#v", i, event)
		}
	}
	if len(batch.Events[0].Payload) != 3 || len(batch.Events[1].Payload) != 6 || len(batch.Events[2].Payload) != 3 || len(batch.Events[3].Payload) != 3 {
		t.Fatalf("unexpected manifest payload shapes: %#v", batch.Events)
	}
	if batch.Events[0].Payload["label"] != "wsp_fixture" || batch.Events[0].Payload["repoFingerprint"] != "opaque" {
		t.Fatalf("registration payload is not privacy-safe/stable: %#v", batch.Events[0].Payload)
	}
	for _, event := range q {
		if e = s.Ack(ctx, event.ID); e != nil {
			t.Fatal(e)
		}
	}
	q, _ = s.Pending(ctx)
	if len(q) != 0 {
		t.Fatal(q)
	}
	if c, e := s.Cursor(ctx, "wsp_fixture"); e != nil || c != 4 {
		t.Fatal(c, e)
	}
	if n, e := s.CleanupAcknowledged(ctx, time.Now().Add(time.Second)); e != nil || n != 4 {
		t.Fatal(n, e)
	}
	rev, e = s.PublishManifest(ctx, ManifestPublication{WorkspaceID: "wsp_fixture", ManifestID: "mft_empty", Baseline: strings.Repeat("a", 40), Head: strings.Repeat("b", 40), Hash: strings.Repeat("d", 64), Entries: []any{}, EventID: "evt_empty"})
	if e != nil || rev != 2 {
		t.Fatalf("zero-path revision=%d err=%v", rev, e)
	}
	q, e = s.Pending(ctx)
	if e != nil || len(q) != 2 || q[0].Kind != "workspace.manifest_started" || q[1].Kind != "workspace.manifest_completed" {
		t.Fatalf("zero-path queue=%#v err=%v", q, e)
	}
	assertEnvelopeSchema(t, q)
	var started eventEnvelope
	if e = json.Unmarshal(q[0].Payload, &started); e != nil {
		t.Fatal(e)
	}
	payloadMap, ok := started.Payload.(map[string]any)
	if !ok || payloadMap["chunkCount"] != float64(0) {
		t.Fatalf("zero-path started envelope=%#v", started)
	}
	_, _, b, e = s.ActiveManifest(ctx, "wsp_fixture")
	if e != nil || string(b) != "[]" {
		t.Fatalf("zero-path active manifest=%s err=%v", b, e)
	}
	if e = s.UpsertWorkspace(ctx, Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", DeviceID: "dev_fixture", SessionID: "ses_fixture", Root: "/fixture", Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}); e != nil {
		t.Fatal(e)
	}
	q, e = s.Pending(ctx)
	if e != nil || len(q) != 2 {
		t.Fatalf("ordinary restart duplicated registration: queue=%#v err=%v", q, e)
	}
}

func assertEnvelopeSchema(t *testing.T, events []QueueEvent) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for _, name := range []string{"change-manifest.schema.json", "verification.schema.json", "event-envelope.schema.json"} {
		b, e := os.ReadFile(filepath.Join("../../protocol/schemas", name))
		if e != nil {
			t.Fatal(e)
		}
		var document map[string]any
		if e = json.Unmarshal(b, &document); e != nil {
			t.Fatal(e)
		}
		url, ok := document["$id"].(string)
		if !ok {
			t.Fatalf("schema %s lacks $id", name)
		}
		if e = compiler.AddResource(url, document); e != nil {
			t.Fatal(e)
		}
	}
	const schemaURL = "https://schemas.stickguy.dev/v1/event-envelope.schema.json"
	schema, e := compiler.Compile(schemaURL)
	if e != nil {
		t.Fatal(e)
	}
	for _, event := range events {
		var envelope any
		if e = json.Unmarshal(event.Payload, &envelope); e != nil {
			t.Fatal(e)
		}
		if e = schema.Validate(envelope); e != nil {
			t.Fatalf("queued envelope %s violates schema: %v", event.ID, e)
		}
	}
}

// A path can be reported by sources of different strength: anticipated at
// begin_work, then actually observed. The read set must keep the strongest
// evidence, and must treat an upgrade as worth republishing because it can
// raise the confidence band of a finding already derived from that path
// (ADR-052).
func TestStrongerReadEvidenceWinsAndAWeakerReportCannotDowngradeIt(t *testing.T) {
	ctx := context.Background()
	state, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	const workspace, session, path = "wsp_a", "wrk_agent_a", "internal/session/rotate.go"
	const hash = "1122334455667788990011223344556677889900112233445566778899001122"
	entry := func(fidelity string) []ReadSetEntry {
		return []ReadSetEntry{{Path: path, FileContractHashAtRead: hash, ObservedAt: "2026-08-28T09:00:00Z", Fidelity: fidelity}}
	}

	changed, err := state.ChangedReadSet(ctx, workspace, session, entry(ReadFidelitySelfDeclared))
	if err != nil || len(changed) != 1 || changed[0].Fidelity != ReadFidelitySelfDeclared {
		t.Fatalf("first declaration: changed=%#v err=%v", changed, err)
	}

	// Observing the same unchanged path is an upgrade, not a duplicate.
	changed, err = state.ChangedReadSet(ctx, workspace, session, entry(ReadFidelityObserved))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0].Fidelity != ReadFidelityObserved {
		t.Fatalf("an upgrade to observed was not republished: changed=%#v", changed)
	}

	// A later weaker report must not erase the observation.
	changed, err = state.ChangedReadSet(ctx, workspace, session, entry(ReadFidelitySelfDeclared))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("a weaker later report republished: changed=%#v", changed)
	}
	var stored string
	if err = state.db.QueryRowContext(ctx, `SELECT fidelity FROM session_read_sets WHERE workspace_id=? AND session_workstream_id=? AND path=?`, workspace, session, path).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != ReadFidelityObserved {
		t.Fatalf("stored fidelity downgraded to %q", stored)
	}
}

// B19: the CLI, the service, and the tests all open the same database file. A
// second writer arriving during a held transaction got SQLITE_BUSY immediately,
// which surfaced as a random CI failure and, in production, as a lost
// observation rather than a delayed one.
func TestASecondWriterWaitsInsteadOfFailingBusy(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	// Hold a write transaction open on the first handle.
	tx, err := first.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO projects(id) VALUES('prj_holder')`); err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(250 * time.Millisecond)
		_ = tx.Commit()
		close(released)
	}()

	// The second writer must wait for the lock rather than be refused.
	start := time.Now()
	if _, err = second.db.ExecContext(ctx, `INSERT INTO projects(id) VALUES('prj_waiter')`); err != nil {
		t.Fatalf("second writer was refused instead of waiting: %v", err)
	}
	<-released
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("second writer did not actually contend for the lock (%s)", elapsed)
	}
	var count int
	if err = second.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("both writes should have landed, got %d", count)
	}
}
