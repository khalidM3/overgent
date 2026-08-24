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
