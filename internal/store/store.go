package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
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
type LifecyclePublication struct {
	WorkspaceID, Method, IdempotencyKey, Source, Kind string
	Payload                                           any
	Additional                                        []LifecycleEvent
	ExpectedIntentRevision                            *int64
	IncrementIntentRevision                           bool
}
type LifecycleEvent struct {
	Kind    string
	Payload any
}
type InjectionItem struct {
	ID       string
	Revision int
}
type AgentSession struct {
	WorkspaceID, Vendor, WorkstreamID, CWD, Status string
	ObservedAt                                     time.Time
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
	// busy_timeout is set through the DSN rather than a one-off PRAGMA because
	// database/sql may open a fresh connection at any time and a pragma applies
	// only to the connection that ran it.
	//
	// SetMaxOpenConns(1) serializes writers inside one process, but the CLI, the
	// service, and the tests all open the same file, and a second process
	// writing during a held transaction gets SQLITE_BUSY immediately with no
	// timeout. Waiting briefly is the correct behavior for a queue whose writes
	// are short: the alternative is a failed enqueue and a lost observation.
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: "_pragma=busy_timeout(5000)"}).String()
	db, err := sql.Open("sqlite", dsn)
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
CREATE TABLE IF NOT EXISTS idempotency_keys(workspace_id TEXT NOT NULL,method TEXT NOT NULL,key TEXT NOT NULL,request_hash TEXT NOT NULL,response_revision INTEGER NOT NULL DEFAULT 0,created_at INTEGER NOT NULL,PRIMARY KEY(workspace_id,method,key));
CREATE TABLE IF NOT EXISTS agent_observations(workspace_id TEXT NOT NULL,vendor TEXT NOT NULL,last_observed_at INTEGER NOT NULL,PRIMARY KEY(workspace_id,vendor));
CREATE TABLE IF NOT EXISTS agent_sessions(workspace_id TEXT NOT NULL,vendor TEXT NOT NULL,workstream_id TEXT NOT NULL,cwd TEXT NOT NULL,status TEXT NOT NULL,last_observed_at INTEGER NOT NULL,PRIMARY KEY(vendor,workstream_id));
CREATE INDEX IF NOT EXISTS active_agent_sessions_by_cwd ON agent_sessions(workspace_id,vendor,cwd,last_observed_at);
CREATE TABLE IF NOT EXISTS contract_fingerprints(workspace_id TEXT NOT NULL,path TEXT NOT NULL,file_contract_hash TEXT NOT NULL,observed_at INTEGER NOT NULL,PRIMARY KEY(workspace_id,path));
CREATE TABLE IF NOT EXISTS session_read_sets(workspace_id TEXT NOT NULL,session_workstream_id TEXT NOT NULL,path TEXT NOT NULL,file_contract_hash TEXT NOT NULL,observed_at TEXT NOT NULL,fidelity TEXT NOT NULL DEFAULT 'observed',PRIMARY KEY(workspace_id,session_workstream_id,path));
CREATE TABLE IF NOT EXISTS injection_deliveries(session_key TEXT NOT NULL,item_id TEXT NOT NULL,item_revision INTEGER NOT NULL,delivered_at INTEGER NOT NULL,PRIMARY KEY(session_key,item_id,item_revision));
CREATE TABLE IF NOT EXISTS session_focus(session_key TEXT PRIMARY KEY,until INTEGER NOT NULL,created_at INTEGER NOT NULL);
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
		{"intent_revision", `ALTER TABLE workspaces ADD COLUMN intent_revision INTEGER NOT NULL DEFAULT 0`},
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
	hasFidelity, err := hasColumn(db, `PRAGMA table_info(session_read_sets)`, "fidelity")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("inspect read set migration: %w", err)
	}
	if !hasFidelity {
		// Rows written before ADR-052 came from the hook inspection path, which
		// is the observed source, so that is the honest backfill.
		if _, err = db.Exec(`ALTER TABLE session_read_sets ADD COLUMN fidelity TEXT NOT NULL DEFAULT 'observed'`); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate read set fidelity: %w", err)
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
	if _, err = db.Exec(`INSERT OR IGNORE INTO schema_migrations(version) VALUES(2),(3),(4),(5),(6),(7)`); err != nil {
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

// ClaimInjectionDeliveries atomically returns and records only item revisions
// this local agent session has not received before. Claiming before the hook
// response prevents concurrent hook invocations from injecting one revision
// twice; a newer revision remains independently eligible.
func (s *Store) ClaimInjectionDeliveries(ctx context.Context, sessionKey string, items []InjectionItem, deliveredAt time.Time) ([]InjectionItem, error) {
	if sessionKey == "" {
		return nil, errors.New("injection delivery session key is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	claimed := make([]InjectionItem, 0, len(items))
	for _, item := range items {
		if item.ID == "" || item.Revision < 1 {
			return nil, errors.New("injection delivery item identity is invalid")
		}
		result, insertErr := tx.ExecContext(ctx, `INSERT OR IGNORE INTO injection_deliveries(session_key,item_id,item_revision,delivered_at) VALUES(?,?,?,?)`, sessionKey, item.ID, item.Revision, deliveredAt.UTC().UnixMilli())
		if insertErr != nil {
			return nil, insertErr
		}
		inserted, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return nil, rowsErr
		}
		if inserted == 1 {
			claimed = append(claimed, item)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

// UndeliveredInjectionItems returns item revisions absent from the local
// session delivery set. ClaimInjectionDeliveries remains the atomic arbiter if
// concurrent hook invocations race after this read.
func (s *Store) UndeliveredInjectionItems(ctx context.Context, sessionKey string, items []InjectionItem) ([]InjectionItem, error) {
	if sessionKey == "" {
		return nil, errors.New("injection delivery session key is required")
	}
	pending := make([]InjectionItem, 0, len(items))
	for _, item := range items {
		if item.ID == "" || item.Revision < 1 {
			return nil, errors.New("injection delivery item identity is invalid")
		}
		var present int
		err := s.db.QueryRowContext(ctx, `SELECT 1 FROM injection_deliveries WHERE session_key=? AND item_id=? AND item_revision=?`, sessionKey, item.ID, item.Revision).Scan(&present)
		if errors.Is(err, sql.ErrNoRows) {
			pending = append(pending, item)
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	return pending, nil
}

func (s *Store) RecordAgentObservation(ctx context.Context, workspaceID, vendor string, observedAt time.Time) error {
	if workspaceID == "" || vendor != "codex" && vendor != "claude" && vendor != "cursor" {
		return fmt.Errorf("agent observation workspace and supported vendor are required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_observations(workspace_id,vendor,last_observed_at) VALUES(?,?,?) ON CONFLICT(workspace_id,vendor) DO UPDATE SET last_observed_at=excluded.last_observed_at`, workspaceID, vendor, observedAt.UTC().UnixMilli())
	return err
}

func (s *Store) AgentObserved(ctx context.Context, workspaceID, vendor string) (time.Time, bool, error) {
	var milliseconds int64
	err := s.db.QueryRowContext(ctx, `SELECT last_observed_at FROM agent_observations WHERE workspace_id=? AND vendor=?`, workspaceID, vendor).Scan(&milliseconds)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return time.UnixMilli(milliseconds).UTC(), true, nil
}

func (s *Store) ClearAgentObservation(ctx context.Context, workspaceID, vendor string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_observations WHERE workspace_id=? AND vendor=?`, workspaceID, vendor)
	return err
}

// RecordAgentSession keeps the local lifecycle evidence needed to bind an MCP
// call to the same per-session workstream as its hooks. The vendor's raw
// session identifier is deliberately absent: the derived workstream identity
// is sufficient here and remains local.
func (s *Store) RecordAgentSession(ctx context.Context, session AgentSession) error {
	if session.WorkspaceID == "" || session.WorkstreamID == "" || session.CWD == "" || session.ObservedAt.IsZero() {
		return errors.New("agent session identity and observation time are required")
	}
	if session.Vendor != "codex" && session.Vendor != "claude" {
		return errors.New("agent session vendor is unsupported")
	}
	if !map[string]bool{"active": true, "idle": true, "waiting": true, "error": true, "done": true}[session.Status] {
		return errors.New("agent session status is unsupported")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_sessions(workspace_id,vendor,workstream_id,cwd,status,last_observed_at) VALUES(?,?,?,?,?,?) ON CONFLICT(vendor,workstream_id) DO UPDATE SET workspace_id=excluded.workspace_id,cwd=excluded.cwd,status=excluded.status,last_observed_at=excluded.last_observed_at`, session.WorkspaceID, session.Vendor, session.WorkstreamID, session.CWD, session.Status, session.ObservedAt.UTC().UnixMilli())
	return err
}

// ActiveAgentSessions returns only sessions with recent lifecycle evidence.
// The same thirty-minute window ends silent hosted agent sessions; using it
// here prevents an abandoned local session from becoming a permanent false
// ambiguity while still preferring unidentified over a stale guess.
func (s *Store) ActiveAgentSessions(ctx context.Context, workspaceID, vendor, cwd string, activeAfter time.Time) ([]AgentSession, error) {
	if workspaceID == "" || cwd == "" || activeAfter.IsZero() || vendor != "codex" && vendor != "claude" {
		return nil, errors.New("active agent session query is invalid")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT workspace_id,vendor,workstream_id,cwd,status,last_observed_at FROM agent_sessions WHERE workspace_id=? AND vendor=? AND cwd=? AND status<>'done' AND last_observed_at>=? ORDER BY workstream_id`, workspaceID, vendor, cwd, activeAfter.UTC().UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []AgentSession
	for rows.Next() {
		var session AgentSession
		var observedAt int64
		if err = rows.Scan(&session.WorkspaceID, &session.Vendor, &session.WorkstreamID, &session.CWD, &session.Status, &observedAt); err != nil {
			return nil, err
		}
		session.ObservedAt = time.UnixMilli(observedAt).UTC()
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

// ReadSetEntry is one observation of a fingerprintable path by one agent
// session, carrying the file contract hash current when the session read it.
type ReadSetEntry struct {
	Path                   string `json:"path"`
	FileContractHashAtRead string `json:"fileContractHashAtRead"`
	ObservedAt             string `json:"observedAt"`
	// Fidelity records how this observation was obtained (ADR-052). A read set
	// mixes sources of different strength, and a stale-assumption finding
	// raised from evidence that is not observed is not deterministic.
	Fidelity string `json:"fidelity"`
}

// Read-set fidelities in ascending order of strength (ADR-052).
const (
	ReadFidelitySelfDeclared   = "self_declared"
	ReadFidelityVendorInferred = "vendor_inferred"
	ReadFidelityObserved       = "observed"
)

// ReadFidelityRank orders the sources so the strongest evidence for a path
// wins. An unrecognized value ranks below every known source rather than
// displacing one.
func ReadFidelityRank(fidelity string) int {
	switch fidelity {
	case ReadFidelityObserved:
		return 3
	case ReadFidelityVendorInferred:
		return 2
	case ReadFidelitySelfDeclared:
		return 1
	}
	return 0
}

// ChangedFingerprints records the observed file contract hash of every path in
// observed and returns, sorted, only the paths whose hash is new or different.
// Publishing is therefore proportional to contract drift rather than to how
// often the manifest pipeline runs.
func (s *Store) ChangedFingerprints(ctx context.Context, workspaceID string, observed map[string]string, at time.Time) ([]string, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("contract fingerprint workspace is required")
	}
	if len(observed) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	paths := make([]string, 0, len(observed))
	for path := range observed {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	var changed []string
	for _, path := range paths {
		hash := observed[path]
		var stored string
		queryErr := tx.QueryRowContext(ctx, `SELECT file_contract_hash FROM contract_fingerprints WHERE workspace_id=? AND path=?`, workspaceID, path).Scan(&stored)
		if queryErr != nil && !errors.Is(queryErr, sql.ErrNoRows) {
			return nil, queryErr
		}
		if queryErr == nil && stored == hash {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO contract_fingerprints(workspace_id,path,file_contract_hash,observed_at) VALUES(?,?,?,?) ON CONFLICT(workspace_id,path) DO UPDATE SET file_contract_hash=excluded.file_contract_hash,observed_at=excluded.observed_at`, workspaceID, path, hash, at.UTC().UnixMilli()); err != nil {
			return nil, err
		}
		changed = append(changed, path)
	}
	return changed, tx.Commit()
}

// FingerprintHash returns the last recorded file contract hash for a path. It
// is the cache consulted when a read is observed for a file that cannot be read
// from disk at that moment.
func (s *Store) FingerprintHash(ctx context.Context, workspaceID, path string) (string, bool, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT file_contract_hash FROM contract_fingerprints WHERE workspace_id=? AND path=?`, workspaceID, path).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}

// ChangedReadSet keeps exactly one entry per (session, path): re-observing a
// path replaces its hash and time rather than appending. It returns only the
// entries worth publishing, which are the ones that are new or whose hash moved.
func (s *Store) ChangedReadSet(ctx context.Context, workspaceID, sessionWorkstreamID string, entries []ReadSetEntry) ([]ReadSetEntry, error) {
	if workspaceID == "" || sessionWorkstreamID == "" {
		return nil, fmt.Errorf("read set workspace and session workstream are required")
	}
	if len(entries) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var changed []ReadSetEntry
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.Path == "" || entry.FileContractHashAtRead == "" || seen[entry.Path] {
			continue
		}
		seen[entry.Path] = true
		var storedHash, storedFidelity string
		queryErr := tx.QueryRowContext(ctx, `SELECT file_contract_hash,fidelity FROM session_read_sets WHERE workspace_id=? AND session_workstream_id=? AND path=?`, workspaceID, sessionWorkstreamID, entry.Path).Scan(&storedHash, &storedFidelity)
		if queryErr != nil && !errors.Is(queryErr, sql.ErrNoRows) {
			return nil, queryErr
		}
		// A path can be seen more than once by sources of different strength:
		// anticipated at begin_work, then actually observed. Keep the strongest
		// evidence rather than letting a weaker later source erase it (ADR-052).
		fidelity := entry.Fidelity
		if queryErr == nil && ReadFidelityRank(storedFidelity) > ReadFidelityRank(fidelity) {
			fidelity = storedFidelity
		}
		entry.Fidelity = fidelity
		// An upgraded fidelity is news even when the hash is unchanged: it can
		// raise the confidence band of a finding already derived from this path.
		unchanged := queryErr == nil && storedHash == entry.FileContractHashAtRead && storedFidelity == fidelity
		if _, err = tx.ExecContext(ctx, `INSERT INTO session_read_sets(workspace_id,session_workstream_id,path,file_contract_hash,observed_at,fidelity) VALUES(?,?,?,?,?,?) ON CONFLICT(workspace_id,session_workstream_id,path) DO UPDATE SET file_contract_hash=excluded.file_contract_hash,observed_at=excluded.observed_at,fidelity=excluded.fidelity`, workspaceID, sessionWorkstreamID, entry.Path, entry.FileContractHashAtRead, entry.ObservedAt, fidelity); err != nil {
			return nil, err
		}
		if !unchanged {
			changed = append(changed, entry)
		}
	}
	return changed, tx.Commit()
}

func (s *Store) PublishLifecycle(ctx context.Context, publication LifecyclePublication) (revision int64, duplicate bool, err error) {
	if publication.WorkspaceID == "" || publication.Method == "" || publication.IdempotencyKey == "" || publication.Kind == "" {
		return 0, false, fmt.Errorf("lifecycle publication identifiers are required")
	}
	events := append([]LifecycleEvent{{Kind: publication.Kind, Payload: publication.Payload}}, publication.Additional...)
	for _, event := range events {
		if event.Kind == "" {
			return 0, false, fmt.Errorf("lifecycle event kind is required")
		}
	}
	request, err := json.Marshal(events)
	if err != nil {
		return 0, false, fmt.Errorf("encode lifecycle events: %w", err)
	}
	requestSum := sha256.Sum256(request)
	requestHash := fmt.Sprintf("%x", requestSum[:])
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var storedHash string
	if queryErr := tx.QueryRowContext(ctx, `SELECT request_hash,response_revision FROM idempotency_keys WHERE workspace_id=? AND method=? AND key=?`, publication.WorkspaceID, publication.Method, publication.IdempotencyKey).Scan(&storedHash, &revision); queryErr == nil {
		if storedHash != requestHash {
			return 0, false, fmt.Errorf("idempotency key reused with different input")
		}
		return revision, true, nil
	} else if queryErr != sql.ErrNoRows {
		return 0, false, queryErr
	}
	var sequence, currentIntentRevision int64
	var projectID, memberID, deviceID, sessionID string
	if err = tx.QueryRowContext(ctx, `SELECT sequence,intent_revision,project_id,member_id,device_id,session_id FROM workspaces WHERE id=?`, publication.WorkspaceID).Scan(&sequence, &currentIntentRevision, &projectID, &memberID, &deviceID, &sessionID); err != nil {
		return 0, false, err
	}
	revision = currentIntentRevision
	if publication.IncrementIntentRevision {
		if publication.ExpectedIntentRevision != nil && *publication.ExpectedIntentRevision != currentIntentRevision {
			return currentIntentRevision, false, fmt.Errorf("intent revision conflict: current revision is %d", currentIntentRevision)
		}
		revision++
	}
	queuedAt := time.Now().UTC()
	for i, event := range events {
		eventSum := sha256.Sum256([]byte(fmt.Sprintf("stickguy.lifecycle-event.v1\x00%s\x00%s\x00%s\x00%d", publication.WorkspaceID, publication.Method, publication.IdempotencyKey, i)))
		eventID := fmt.Sprintf("evt_%x", eventSum[:16])
		encodedPayload, encodeErr := json.Marshal(event.Payload)
		if encodeErr != nil {
			return 0, false, encodeErr
		}
		var payload any
		if encodeErr = json.Unmarshal(encodedPayload, &payload); encodeErr != nil {
			return 0, false, encodeErr
		}
		payload = fillObservedAt(payload, queuedAt.Format(time.RFC3339Nano))
		body, encodeErr := json.Marshal(eventEnvelope{SchemaVersion: 1, EventID: eventID, ProjectID: projectID, MemberID: memberID, DeviceID: deviceID, WorkspaceID: publication.WorkspaceID, SessionID: sessionID, Sequence: sequence + int64(i) + 1, ObservedAt: queuedAt, SentAt: queuedAt, Source: publication.Source, Type: event.Kind, Payload: payload})
		if encodeErr != nil {
			return 0, false, encodeErr
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO event_queue(id,workspace_id,sequence,kind,payload,created_at) VALUES(?,?,?,?,?,?)`, eventID, publication.WorkspaceID, sequence+int64(i)+1, event.Kind, body, queuedAt.Unix()); err != nil {
			return 0, false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE workspaces SET sequence=?,intent_revision=? WHERE id=?`, sequence+int64(len(events)), revision, publication.WorkspaceID); err != nil {
		return 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO idempotency_keys(workspace_id,method,key,request_hash,response_revision,created_at) VALUES(?,?,?,?,?,?)`, publication.WorkspaceID, publication.Method, publication.IdempotencyKey, requestHash, revision, queuedAt.Unix()); err != nil {
		return 0, false, err
	}
	return revision, false, tx.Commit()
}

func fillObservedAt(value any, timestamp string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if key == "observedAt" && item == "" {
				typed[key] = timestamp
			} else {
				typed[key] = fillObservedAt(item, timestamp)
			}
		}
	case []any:
		for i, item := range typed {
			typed[i] = fillObservedAt(item, timestamp)
		}
	}
	return value
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

// SetProjectPaused pauses or resumes every workspace registered to one Project
// and reports how many it changed.
//
// Pause was reachable only per workspace or, from the menu bar, for every
// workspace on the machine. Neither matches how anyone reads the product: a
// member looking at one Project wants to stop sharing that Project, not their
// work on an unrelated repository. A Project with no registered workspace on
// this device is not an error - there is simply nothing here to pause - so the
// count is the answer rather than a failure.
func (s *Store) SetProjectPaused(ctx context.Context, projectID string, p bool) (int, error) {
	if projectID == "" {
		return 0, fmt.Errorf("project id required")
	}
	r, e := s.db.ExecContext(ctx, `UPDATE workspaces SET paused=? WHERE project_id=?`, p, projectID)
	if e != nil {
		return 0, e
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}

// Focus is an agent session that has asked not to be interrupted, and the
// moment that request lapses.
type Focus struct {
	SessionKey string
	Until      time.Time
}

/*
Focus suppresses coordination *into* one agent session until a deadline.

It is deliberately the opposite direction from pause. Pausing stops this
device's activity reaching the Project, which makes the member invisible and
therefore makes their teammates less safe: nobody can avoid work they cannot
see. Focus stops the Project reaching one agent's turns and changes nothing
about what this device publishes, so the member who wants quiet carries their
own risk instead of transferring it to everyone else.

It is local state and never crosses the wire. A teammate does not need to know,
because nothing about what they can see has changed.

Every focus expires. A mute that outlives the reason for it is worse than no
mute at all in a tool whose value is being told things, so a deadline is
required rather than optional and the caller cannot set one beyond MaxFocus.
*/
const (
	DefaultFocus = time.Hour
	MaxFocus     = 8 * time.Hour
)

func (s *Store) SetFocus(ctx context.Context, sessionKey string, now time.Time, duration time.Duration) (time.Time, error) {
	if sessionKey == "" {
		return time.Time{}, fmt.Errorf("session id required")
	}
	if duration <= 0 {
		duration = DefaultFocus
	}
	if duration > MaxFocus {
		duration = MaxFocus
	}
	until := now.Add(duration)
	if _, e := s.db.ExecContext(ctx, `INSERT INTO session_focus(session_key,until,created_at) VALUES(?,?,?)
		ON CONFLICT(session_key) DO UPDATE SET until=excluded.until`, sessionKey, until.UnixMilli(), now.UnixMilli()); e != nil {
		return time.Time{}, e
	}
	return until, nil
}

func (s *Store) ClearFocus(ctx context.Context, sessionKey string) error {
	_, e := s.db.ExecContext(ctx, `DELETE FROM session_focus WHERE session_key=?`, sessionKey)
	return e
}

// ClearAllFocus lets every quiet session start hearing again. It exists so a
// focus the member has forgotten is recoverable from wherever they notice it,
// without first having to remember which session it was on.
func (s *Store) ClearAllFocus(ctx context.Context) (int, error) {
	r, e := s.db.ExecContext(ctx, `DELETE FROM session_focus`)
	if e != nil {
		return 0, e
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}

// FocusedUntil reports whether one session is currently focused. An expired row
// answers false without needing a sweep to have run first, so a deadline that
// has passed can never keep suppressing corrections.
func (s *Store) FocusedUntil(ctx context.Context, sessionKey string, now time.Time) (time.Time, bool, error) {
	if sessionKey == "" {
		return time.Time{}, false, nil
	}
	var until int64
	e := s.db.QueryRowContext(ctx, `SELECT until FROM session_focus WHERE session_key=?`, sessionKey).Scan(&until)
	if errors.Is(e, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if e != nil {
		return time.Time{}, false, e
	}
	deadline := time.UnixMilli(until)
	if !deadline.After(now) {
		return time.Time{}, false, nil
	}
	return deadline, true, nil
}

// ActiveFocus lists the sessions still focused, dropping rows that have lapsed.
func (s *Store) ActiveFocus(ctx context.Context, now time.Time) ([]Focus, error) {
	if _, e := s.db.ExecContext(ctx, `DELETE FROM session_focus WHERE until<=?`, now.UnixMilli()); e != nil {
		return nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT session_key,until FROM session_focus ORDER BY session_key`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Focus
	for rows.Next() {
		var focus Focus
		var until int64
		if e = rows.Scan(&focus.SessionKey, &until); e != nil {
			return nil, e
		}
		focus.Until = time.UnixMilli(until)
		out = append(out, focus)
	}
	return out, rows.Err()
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
