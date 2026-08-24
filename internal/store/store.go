package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }
type Workspace struct {
	ID, ProjectID, WorkstreamID, Root, Baseline string
	Paused                                      bool
	Revision                                    int64
}
type QueueEvent struct {
	ID, WorkspaceID, Kind string
	Sequence              int64
	Payload               []byte
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY);
INSERT OR IGNORE INTO schema_migrations(version) VALUES(1);
CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS workspaces(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,workstream_id TEXT NOT NULL,root TEXT NOT NULL UNIQUE,baseline TEXT NOT NULL,paused INTEGER NOT NULL DEFAULT 0,revision INTEGER NOT NULL DEFAULT 0,sequence INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS manifests(workspace_id TEXT NOT NULL,revision INTEGER NOT NULL,content_hash TEXT NOT NULL,path_count INTEGER NOT NULL,entries_json BLOB NOT NULL,active INTEGER NOT NULL,PRIMARY KEY(workspace_id,revision));
CREATE UNIQUE INDEX IF NOT EXISTS active_manifest ON manifests(workspace_id) WHERE active=1;
CREATE TABLE IF NOT EXISTS event_queue(id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL,sequence INTEGER NOT NULL,kind TEXT NOT NULL,payload BLOB NOT NULL,created_at INTEGER NOT NULL,acked_at INTEGER);
CREATE UNIQUE INDEX IF NOT EXISTS event_sequence ON event_queue(workspace_id,sequence);
CREATE TABLE IF NOT EXISTS cursors(workspace_id TEXT PRIMARY KEY,last_acked_sequence INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS service_state(id INTEGER PRIMARY KEY CHECK(id=1),boot_count INTEGER NOT NULL DEFAULT 0);
INSERT OR IGNORE INTO service_state(id,boot_count) VALUES(1,0);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(p, 0o600); err != nil && !os.IsNotExist(err) {
			db.Close()
			return nil, fmt.Errorf("secure sqlite file %s: %w", p, err)
		}
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Boot(ctx context.Context) (int64, error) {
	_, e := s.db.ExecContext(ctx, `UPDATE service_state SET boot_count=boot_count+1 WHERE id=1`)
	if e != nil {
		return 0, e
	}
	var n int64
	e = s.db.QueryRowContext(ctx, `SELECT boot_count FROM service_state WHERE id=1`).Scan(&n)
	return n, e
}
func (s *Store) UpsertWorkspace(ctx context.Context, w Workspace) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO projects(id) VALUES(?) ON CONFLICT DO NOTHING`, w.ProjectID)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO workspaces(id,project_id,workstream_id,root,baseline,paused,revision) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET project_id=excluded.project_id,workstream_id=excluded.workstream_id,root=excluded.root,baseline=excluded.baseline`, w.ID, w.ProjectID, w.WorkstreamID, w.Root, w.Baseline, w.Paused, w.Revision)
	return e
}
func (s *Store) Workspaces(ctx context.Context) ([]Workspace, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,project_id,workstream_id,root,baseline,paused,revision FROM workspaces ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		if e := rows.Scan(&w.ID, &w.ProjectID, &w.WorkstreamID, &w.Root, &w.Baseline, &w.Paused, &w.Revision); e != nil {
			return nil, e
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (s *Store) SetPaused(ctx context.Context, id string, p bool) error {
	r, e := s.db.ExecContext(ctx, `UPDATE workspaces SET paused=? WHERE id=?`, p, id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return fmt.Errorf("workspace not found")
	}
	return nil
}
func (s *Store) PublishManifest(ctx context.Context, workspaceID, hash string, entries any, eventID string) (int64, error) {
	payload, e := json.Marshal(entries)
	if e != nil {
		return 0, e
	}
	var all []json.RawMessage
	if e := json.Unmarshal(payload, &all); e != nil {
		return 0, fmt.Errorf("encode manifest entries: %w", e)
	}
	count := len(all)
	const chunkSize = 100
	chunkCount := (count + chunkSize - 1) / chunkSize
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	var rev, previousSequence int64
	if e = tx.QueryRowContext(ctx, `SELECT revision+1,sequence FROM workspaces WHERE id=?`, workspaceID).Scan(&rev, &previousSequence); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE manifests SET active=0 WHERE workspace_id=?`, workspaceID); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO manifests(workspace_id,revision,content_hash,path_count,entries_json,active) VALUES(?,?,?,?,?,1)`, workspaceID, rev, hash, count, payload); e != nil {
		return 0, e
	}
	type queued struct {
		id, kind string
		payload  any
	}
	events := []queued{{eventID + "_started", "workspace.manifest_started", map[string]any{"workspaceId": workspaceID, "revision": rev, "chunkCount": chunkCount, "pathCount": count}}}
	for i := 0; i < chunkCount; i++ {
		end := min((i+1)*chunkSize, count)
		events = append(events, queued{fmt.Sprintf("%s_chunk_%04d", eventID, i), "workspace.manifest_chunk", map[string]any{"workspaceId": workspaceID, "revision": rev, "chunkIndex": i, "entries": all[i*chunkSize : end]}})
	}
	events = append(events, queued{eventID, "workspace.manifest_completed", map[string]any{"workspaceId": workspaceID, "revision": rev, "contentHash": hash, "pathCount": count}})
	for i, event := range events {
		body, marshalErr := json.Marshal(event.payload)
		if marshalErr != nil {
			return 0, marshalErr
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO event_queue(id,workspace_id,sequence,kind,payload,created_at) VALUES(?,?,?,?,?,?)`, event.id, workspaceID, previousSequence+int64(i)+1, event.kind, body, time.Now().Unix()); e != nil {
			return 0, e
		}
	}
	lastSequence := previousSequence + int64(len(events))
	if _, e = tx.ExecContext(ctx, `UPDATE workspaces SET revision=?,sequence=? WHERE id=?`, rev, lastSequence, workspaceID); e != nil {
		return 0, e
	}
	return rev, tx.Commit()
}
func (s *Store) Pending(ctx context.Context) ([]QueueEvent, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,workspace_id,sequence,kind,payload FROM event_queue WHERE acked_at IS NULL ORDER BY workspace_id,sequence`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []QueueEvent
	for rows.Next() {
		var v QueueEvent
		if e := rows.Scan(&v.ID, &v.WorkspaceID, &v.Sequence, &v.Kind, &v.Payload); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Ack(ctx context.Context, id string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var w string
	var seq int64
	if e = tx.QueryRowContext(ctx, `SELECT workspace_id,sequence FROM event_queue WHERE id=?`, id).Scan(&w, &seq); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE event_queue SET acked_at=? WHERE id=?`, time.Now().Unix(), id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO cursors(workspace_id,last_acked_sequence) VALUES(?,?) ON CONFLICT(workspace_id) DO UPDATE SET last_acked_sequence=max(last_acked_sequence,excluded.last_acked_sequence)`, w, seq); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) CleanupAcknowledged(ctx context.Context, before time.Time) (int64, error) {
	r, e := s.db.ExecContext(ctx, `DELETE FROM event_queue WHERE acked_at IS NOT NULL AND acked_at<?`, before.Unix())
	if e != nil {
		return 0, e
	}
	return r.RowsAffected()
}
func (s *Store) Cursor(ctx context.Context, workspaceID string) (int64, error) {
	var n int64
	e := s.db.QueryRowContext(ctx, `SELECT last_acked_sequence FROM cursors WHERE workspace_id=?`, workspaceID).Scan(&n)
	return n, e
}
func (s *Store) ActiveManifest(ctx context.Context, id string) (int64, string, []byte, error) {
	var r int64
	var h string
	var b []byte
	e := s.db.QueryRowContext(ctx, `SELECT revision,content_hash,entries_json FROM manifests WHERE workspace_id=? AND active=1`, id).Scan(&r, &h, &b)
	return r, h, b, e
}
