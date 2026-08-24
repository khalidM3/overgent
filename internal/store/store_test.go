package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestRestartManifestQueueCursor(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.db")
	ctx := context.Background()
	s, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.UpsertWorkspace(ctx, Workspace{ID: "w", ProjectID: "p", WorkstreamID: "s", Root: "/fixture", Baseline: "abc"}); e != nil {
		t.Fatal(e)
	}
	entries := []map[string]string{{"path": "a", "status": "added"}}
	rev, e := s.PublishManifest(ctx, "w", "hash", entries, "evt")
	if e != nil || rev != 1 {
		t.Fatal(rev, e)
	}
	s.Close()
	s, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r, h, b, e := s.ActiveManifest(ctx, "w")
	if e != nil || r != 1 || h != "hash" {
		t.Fatal(r, h, e)
	}
	var got []map[string]string
	if json.Unmarshal(b, &got) != nil || len(got) != 1 {
		t.Fatal(string(b))
	}
	q, _ := s.Pending(ctx)
	if len(q) != 3 || q[0].Kind != "workspace.manifest_started" || q[1].Kind != "workspace.manifest_chunk" || q[2].ID != "evt" || q[2].Kind != "workspace.manifest_completed" {
		t.Fatal(q)
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
	if c, e := s.Cursor(ctx, "w"); e != nil || c != 3 {
		t.Fatal(c, e)
	}
	if n, e := s.CleanupAcknowledged(ctx, time.Now().Add(time.Second)); e != nil || n != 3 {
		t.Fatal(n, e)
	}
}
