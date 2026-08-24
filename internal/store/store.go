package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }
type Workspace struct {
	ID, ProjectID, WorkstreamID, MemberID, DeviceID, SessionID string
	Root, Baseline, Fingerprint                                string
	Paused                                                     bool
	Revision                                                   int64
}
type QueueEvent struct {
	ID, WorkspaceID, Kind string
	Sequence              int64
	Payload               []byte
}
type ManifestPublication struct {
	WorkspaceID, ManifestID       string
	Baseline, Head, Hash, EventID string
	Entries                       any
}
type eventEnvelope struct {
	SchemaVersion int       `json:"schemaVersion"`
	EventID       string    `json:"eventId"`
	ProjectID     string    `json:"projectId"`
	MemberID      string    `json:"memberId"`
	DeviceID      string    `json:"deviceId"`
	WorkspaceID   string    `json:"workspaceId"`
	SessionID     string    `json:"sessionId"`
	Sequence      int64     `json:"sequence"`
	ObservedAt    time.Time `json:"observedAt"`
	SentAt        time.Time `json:"sentAt"`
	Source        string    `json:"source"`
	Type          string    `json:"type"`
	Payload       any       `json:"payload"`
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
CREATE TABLE IF NOT EXISTS workspaces(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,workstream_id TEXT NOT NULL,member_id TEXT NOT NULL,device_id TEXT NOT NULL,session_id TEXT NOT NULL,root TEXT NOT NULL UNIQUE,baseline TEXT NOT NULL,repository_fingerprint TEXT NOT NULL,registration_enqueued INTEGER NOT NULL DEFAULT 0,paused INTEGER NOT NULL DEFAULT 0,revision INTEGER NOT NULL DEFAULT 0,sequence INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS workstreams(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,workspace_id TEXT NOT NULL,baseline TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active');
CREATE TABLE IF NOT EXISTS manifests(workspace_id TEXT NOT NULL,revision INTEGER NOT NULL,manifest_id TEXT NOT NULL,workstream_id TEXT NOT NULL,baseline_ref TEXT NOT NULL,head_ref TEXT NOT NULL,content_hash TEXT NOT NULL,path_count INTEGER NOT NULL,entries_json BLOB NOT NULL,active INTEGER NOT NULL,PRIMARY KEY(workspace_id,revision));
CREATE UNIQUE INDEX IF NOT EXISTS active_manifest ON manifests(workspace_id) WHERE active=1;
CREATE TABLE IF NOT EXISTS event_queue(id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL,sequence INTEGER NOT NULL,kind TEXT NOT NULL,payload BLOB NOT NULL,created_at INTEGER NOT NULL,acked_at INTEGER);
CREATE UNIQUE INDEX IF NOT EXISTS event_sequence ON event_queue(workspace_id,sequence);
CREATE TABLE IF NOT EXISTS cursors(workspace_id TEXT PRIMARY KEY,last_acked_sequence INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS service_state(id INTEGER PRIMARY KEY CHECK(id=1),boot_count INTEGER NOT NULL DEFAULT 0);
INSERT OR IGNORE INTO service_state(id,boot_count) VALUES(1,0);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}
	hasFingerprint, err := hasWorkspaceColumn(db, "repository_fingerprint")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("inspect sqlite migration: %w", err)
	}
	if !hasFingerprint {
		if _, err = db.Exec(`ALTER TABLE workspaces ADD COLUMN repository_fingerprint TEXT NOT NULL DEFAULT ''`); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate repository fingerprint: %w", err)
		}
	}
	workspaceIdentityMigrations := []struct {
		column, statement string
	}{
		{"member_id", `ALTER TABLE workspaces ADD COLUMN member_id TEXT NOT NULL DEFAULT ''`},
		{"device_id", `ALTER TABLE workspaces ADD COLUMN device_id TEXT NOT NULL DEFAULT ''`},
		{"session_id", `ALTER TABLE workspaces ADD COLUMN session_id TEXT NOT NULL DEFAULT ''`},
		{"registration_enqueued", `ALTER TABLE workspaces ADD COLUMN registration_enqueued INTEGER NOT NULL DEFAULT 0`},
	}
	for _, migration := range workspaceIdentityMigrations {
		has, inspectErr := hasWorkspaceColumn(db, migration.column)
		if inspectErr != nil {
			db.Close()
			return nil, fmt.Errorf("inspect workspace identity migration: %w", inspectErr)
		}
		if !has {
			if _, err = db.Exec(migration.statement); err != nil {
				db.Close()
				return nil, fmt.Errorf("migrate workspace identity: %w", err)
			}
		}
	}
	manifestMigrations := []struct {
		column, statement string
	}{
		{"manifest_id", `ALTER TABLE manifests ADD COLUMN manifest_id TEXT NOT NULL DEFAULT ''`},
		{"workstream_id", `ALTER TABLE manifests ADD COLUMN workstream_id TEXT NOT NULL DEFAULT ''`},
		{"baseline_ref", `ALTER TABLE manifests ADD COLUMN baseline_ref TEXT NOT NULL DEFAULT ''`},
		{"head_ref", `ALTER TABLE manifests ADD COLUMN head_ref TEXT NOT NULL DEFAULT ''`},
	}
	for _, migration := range manifestMigrations {
		has, inspectErr := hasManifestColumn(db, migration.column)
		if inspectErr != nil {
			db.Close()
			return nil, fmt.Errorf("inspect manifest migration: %w", inspectErr)
		}
		if !has {
			if _, err = db.Exec(migration.statement); err != nil {
				db.Close()
				return nil, fmt.Errorf("migrate manifest metadata: %w", err)
			}
		}
	}
	if _, err = db.Exec(`INSERT OR IGNORE INTO schema_migrations(version) VALUES(2),(3)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("record sqlite migration: %w", err)
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(p, 0o600); err != nil && !os.IsNotExist(err) {
			db.Close()
			return nil, fmt.Errorf("secure sqlite file %s: %w", p, err)
		}
	}
	return s, nil
}

func hasWorkspaceColumn(db *sql.DB, column string) (bool, error) {
	return hasColumn(db, `PRAGMA table_info(workspaces)`, column)
}

func hasManifestColumn(db *sql.DB, column string) (bool, error) {
	return hasColumn(db, `PRAGMA table_info(manifests)`, column)
}

func hasColumn(db *sql.DB, query, column string) (bool, error) {
	rows, err := db.Query(query)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
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
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO projects(id) VALUES(?) ON CONFLICT DO NOTHING`, w.ProjectID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO workspaces(id,project_id,workstream_id,member_id,device_id,session_id,root,baseline,repository_fingerprint,paused,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET project_id=excluded.project_id,workstream_id=excluded.workstream_id,member_id=excluded.member_id,device_id=excluded.device_id,session_id=excluded.session_id,root=excluded.root,baseline=excluded.baseline,repository_fingerprint=excluded.repository_fingerprint`, w.ID, w.ProjectID, w.WorkstreamID, w.MemberID, w.DeviceID, w.SessionID, w.Root, w.Baseline, w.Fingerprint, w.Paused, w.Revision); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO workstreams(id,project_id,workspace_id,baseline,status) VALUES(?,?,?,?,'active') ON CONFLICT(id) DO UPDATE SET project_id=excluded.project_id,workspace_id=excluded.workspace_id,baseline=excluded.baseline`, w.WorkstreamID, w.ProjectID, w.ID, w.Baseline); e != nil {
		return e
	}
	var registrationEnqueued bool
	var sequence int64
	if e = tx.QueryRowContext(ctx, `SELECT registration_enqueued,sequence FROM workspaces WHERE id=?`, w.ID).Scan(&registrationEnqueued, &sequence); e != nil {
		return e
	}
	if !registrationEnqueued {
		eventID := registrationEventID(w.ProjectID, w.DeviceID, w.ID)
		queuedAt := time.Now().UTC()
		body, marshalErr := json.Marshal(eventEnvelope{SchemaVersion: 1, EventID: eventID, ProjectID: w.ProjectID, MemberID: w.MemberID, DeviceID: w.DeviceID, WorkspaceID: w.ID, SessionID: w.SessionID, Sequence: sequence + 1, ObservedAt: queuedAt, SentAt: queuedAt, Source: "manual", Type: "workspace.registered", Payload: map[string]any{"repoFingerprint": w.Fingerprint, "label": w.ID, "capabilities": map[string]any{"gitManifest": "layered/v1"}}})
		if marshalErr != nil {
			return marshalErr
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO event_queue(id,workspace_id,sequence,kind,payload,created_at) VALUES(?,?,?,?,?,?)`, eventID, w.ID, sequence+1, "workspace.registered", body, queuedAt.Unix()); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE workspaces SET registration_enqueued=1,sequence=? WHERE id=?`, sequence+1, w.ID); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func registrationEventID(projectID, deviceID, workspaceID string) string {
	sum := sha256.Sum256([]byte("stickguy.workspace-registration.v1\x00" + projectID + "\x00" + deviceID + "\x00" + workspaceID))
	return fmt.Sprintf("evt_registration_%x", sum[:16])
}
func (s *Store) Workspaces(ctx context.Context) ([]Workspace, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,project_id,workstream_id,member_id,device_id,session_id,root,baseline,repository_fingerprint,paused,revision FROM workspaces ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		if e := rows.Scan(&w.ID, &w.ProjectID, &w.WorkstreamID, &w.MemberID, &w.DeviceID, &w.SessionID, &w.Root, &w.Baseline, &w.Fingerprint, &w.Paused, &w.Revision); e != nil {
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

func (s *Store) EnqueueEvent(ctx context.Context, workspaceID, eventID, source, kind string, payload any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sequence int64
	var projectID, memberID, deviceID, sessionID string
	if err = tx.QueryRowContext(ctx, `SELECT sequence,project_id,member_id,device_id,session_id FROM workspaces WHERE id=?`, workspaceID).Scan(&sequence, &projectID, &memberID, &deviceID, &sessionID); err != nil {
		return err
	}
	queuedAt := time.Now().UTC()
	body, err := json.Marshal(eventEnvelope{SchemaVersion: 1, EventID: eventID, ProjectID: projectID, MemberID: memberID, DeviceID: deviceID, WorkspaceID: workspaceID, SessionID: sessionID, Sequence: sequence + 1, ObservedAt: queuedAt, SentAt: queuedAt, Source: source, Type: kind, Payload: payload})
	if err != nil {
		return fmt.Errorf("encode queued event: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO event_queue(id,workspace_id,sequence,kind,payload,created_at) VALUES(?,?,?,?,?,?)`, eventID, workspaceID, sequence+1, kind, body, queuedAt.Unix()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE workspaces SET sequence=? WHERE id=?`, sequence+1, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) PublishManifest(ctx context.Context, publication ManifestPublication) (int64, error) {
	payload, e := json.Marshal(publication.Entries)
	if e != nil {
		return 0, e
	}
	var all []json.RawMessage
	if e := json.Unmarshal(payload, &all); e != nil {
		return 0, fmt.Errorf("encode manifest entries: %w", e)
	}
	count := len(all)
	if count > 100000 {
		return 0, fmt.Errorf("manifest exceeds 100000-path contract limit")
	}
	const chunkSize = 100
	chunkCount := (count + chunkSize - 1) / chunkSize
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	var rev, previousSequence int64
	var projectID, memberID, deviceID, sessionID, workstreamID string
	if e = tx.QueryRowContext(ctx, `SELECT revision+1,sequence,project_id,member_id,device_id,session_id,workstream_id FROM workspaces WHERE id=?`, publication.WorkspaceID).Scan(&rev, &previousSequence, &projectID, &memberID, &deviceID, &sessionID, &workstreamID); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE manifests SET active=0 WHERE workspace_id=?`, publication.WorkspaceID); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO manifests(workspace_id,revision,manifest_id,workstream_id,baseline_ref,head_ref,content_hash,path_count,entries_json,active) VALUES(?,?,?,?,?,?,?,?,?,1)`, publication.WorkspaceID, rev, publication.ManifestID, workstreamID, publication.Baseline, publication.Head, publication.Hash, count, payload); e != nil {
		return 0, e
	}
	type queued struct {
		id, kind string
		payload  any
	}
	events := []queued{{publication.EventID + "_started", "workspace.manifest_started", map[string]any{"manifestId": publication.ManifestID, "workstreamId": workstreamID, "revision": rev, "baselineRef": publication.Baseline, "headRef": publication.Head, "chunkCount": chunkCount}}}
	for i := 0; i < chunkCount; i++ {
		end := min((i+1)*chunkSize, count)
		events = append(events, queued{fmt.Sprintf("%s_chunk_%04d", publication.EventID, i), "workspace.manifest_chunk", map[string]any{"manifestId": publication.ManifestID, "chunkIndex": i, "entries": all[i*chunkSize : end]}})
	}
	events = append(events, queued{publication.EventID, "workspace.manifest_completed", map[string]any{"manifestId": publication.ManifestID, "revision": rev, "contentHash": publication.Hash}})
	queuedAt := time.Now().UTC()
	for i, event := range events {
		sequence := previousSequence + int64(i) + 1
		envelope := eventEnvelope{SchemaVersion: 1, EventID: event.id, ProjectID: projectID, MemberID: memberID, DeviceID: deviceID, WorkspaceID: publication.WorkspaceID, SessionID: sessionID, Sequence: sequence, ObservedAt: queuedAt, SentAt: queuedAt, Source: "git", Type: event.kind, Payload: event.payload}
		body, marshalErr := json.Marshal(envelope)
		if marshalErr != nil {
			return 0, marshalErr
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO event_queue(id,workspace_id,sequence,kind,payload,created_at) VALUES(?,?,?,?,?,?)`, event.id, publication.WorkspaceID, sequence, event.kind, body, queuedAt.Unix()); e != nil {
			return 0, e
		}
	}
	lastSequence := previousSequence + int64(len(events))
	if _, e = tx.ExecContext(ctx, `UPDATE workspaces SET revision=?,sequence=? WHERE id=?`, rev, lastSequence, publication.WorkspaceID); e != nil {
		return 0, e
	}
	return rev, tx.Commit()
}

func Batch(events []QueueEvent) ([]byte, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("empty event batch")
	}
	if len(events) > 100 {
		return nil, fmt.Errorf("event batch exceeds 100-event contract limit")
	}
	workspaceID := events[0].WorkspaceID
	envelopes := make([]json.RawMessage, len(events))
	for i, event := range events {
		if event.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("event batch crosses workspace boundary")
		}
		if !json.Valid(event.Payload) {
			return nil, fmt.Errorf("invalid queued event envelope")
		}
		envelopes[i] = append(json.RawMessage(nil), event.Payload...)
	}
	return json.Marshal(struct {
		Events []json.RawMessage `json:"events"`
	}{Events: envelopes})
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
