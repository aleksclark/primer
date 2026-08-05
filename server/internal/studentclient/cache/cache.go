// Package cache provides a restart-safe SQLite store for the student client:
// device meta, work cache, session state, event outbox, completion intents,
// and pending artifacts.
package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) a SQLite database at path and applies migrations.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("cache path is required")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create cache dir: %w", err)
		}
	}
	// WAL + busy timeout for multi-process friendliness; pure Go driver.
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Store is the local durable state for the student engine.
type Store struct {
	db *sql.DB
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS device_meta (
  key   TEXT PRIMARY KEY NOT NULL,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS work_items (
  assignment_id TEXT PRIMARY KEY NOT NULL,
  payload_json  TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  client_session_id       TEXT PRIMARY KEY NOT NULL,
  server_session_id       TEXT NOT NULL DEFAULT '',
  assignment_id           TEXT NOT NULL,
  activity_revision_id    TEXT NOT NULL DEFAULT '',
  state                   TEXT NOT NULL DEFAULT 'local',
  last_acked_sequence     INTEGER NOT NULL DEFAULT -1,
  next_sequence           INTEGER NOT NULL DEFAULT 0,
  workspace_path          TEXT NOT NULL DEFAULT '',
  created_at              TEXT NOT NULL,
  updated_at              TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS event_outbox (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  client_session_id TEXT NOT NULL,
  server_session_id TEXT NOT NULL DEFAULT '',
  sequence          INTEGER NOT NULL,
  event_id          TEXT NOT NULL UNIQUE,
  event_type        TEXT NOT NULL,
  client_time       TEXT NOT NULL,
  payload_json      TEXT NOT NULL,
  acked             INTEGER NOT NULL DEFAULT 0,
  created_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_event_outbox_pending
  ON event_outbox(client_session_id, sequence) WHERE acked = 0;

CREATE TABLE IF NOT EXISTS completion_intents (
  completion_id     TEXT PRIMARY KEY NOT NULL,
  client_session_id TEXT NOT NULL,
  server_session_id TEXT NOT NULL DEFAULT '',
  request_json      TEXT NOT NULL,
  response_json     TEXT NOT NULL DEFAULT '',
  acked             INTEGER NOT NULL DEFAULT 0,
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_completion_pending
  ON completion_intents(acked) WHERE acked = 0;

CREATE TABLE IF NOT EXISTS artifacts_pending (
  artifact_id       TEXT PRIMARY KEY NOT NULL,
  client_session_id TEXT NOT NULL,
  server_session_id TEXT NOT NULL DEFAULT '',
  meta_json         TEXT NOT NULL,
  acked             INTEGER NOT NULL DEFAULT 0,
  created_at        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runner_state (
  client_session_id TEXT PRIMARY KEY NOT NULL,
  kind              TEXT NOT NULL,
  state_json        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_assignment_state
  ON sessions(assignment_id, state, updated_at);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate cache schema: %w", err)
	}
	return nil
}

// --- device meta -----------------------------------------------------------

// SetMeta stores a string value under key.
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO device_meta(key, value) VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetMeta returns a stored value or "" if missing.
func (s *Store) GetMeta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM device_meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetDeviceToken stores the bearer token (broker-only in production).
func (s *Store) SetDeviceToken(ctx context.Context, token string) error {
	return s.SetMeta(ctx, "device_token", token)
}

// DeviceToken returns the stored device token.
func (s *Store) DeviceToken(ctx context.Context) (string, error) {
	return s.GetMeta(ctx, "device_token")
}

// SetDeviceIdentity stores non-secret pairing identity.
func (s *Store) SetDeviceIdentity(ctx context.Context, deviceID, studentID, deviceName string) error {
	if err := s.SetMeta(ctx, "device_id", deviceID); err != nil {
		return err
	}
	if err := s.SetMeta(ctx, "student_id", studentID); err != nil {
		return err
	}
	return s.SetMeta(ctx, "device_name", deviceName)
}

// --- work cache ------------------------------------------------------------

// SaveWork upserts work queue items from the API.
func (s *Store) SaveWork(ctx context.Context, items []studentapi.WorkItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO work_items(assignment_id, payload_json, updated_at) VALUES(?, ?, ?)
ON CONFLICT(assignment_id) DO UPDATE SET
  payload_json = excluded.payload_json,
  updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, it := range items {
		raw, err := json.Marshal(it)
		if err != nil {
			return fmt.Errorf("marshal work item: %w", err)
		}
		updated := now
		if !it.Assignment.UpdatedAt.IsZero() {
			updated = it.Assignment.UpdatedAt.UTC().Format(time.RFC3339Nano)
		}
		if _, err := stmt.ExecContext(ctx, it.Assignment.ID, string(raw), updated); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListWork returns cached work items ordered by updated_at.
func (s *Store) ListWork(ctx context.Context) ([]studentapi.WorkItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM work_items ORDER BY updated_at ASC, assignment_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []studentapi.WorkItem
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var it studentapi.WorkItem
		if err := json.Unmarshal([]byte(raw), &it); err != nil {
			return nil, fmt.Errorf("decode work item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetWork returns one cached work item by assignment ID.
func (s *Store) GetWork(ctx context.Context, assignmentID string) (*studentapi.WorkItem, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM work_items WHERE assignment_id = ?`, assignmentID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var it studentapi.WorkItem
	if err := json.Unmarshal([]byte(raw), &it); err != nil {
		return nil, err
	}
	return &it, nil
}

// ErrNotFound is returned when a cached row is missing.
var ErrNotFound = errors.New("not found")

// --- sessions --------------------------------------------------------------

// Session is local session state mirrored for resume.
type Session struct {
	ClientSessionID    string
	ServerSessionID    string
	AssignmentID       string
	ActivityRevisionID string
	State              string
	LastAckedSequence  int64
	NextSequence       int64
	WorkspacePath      string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// SaveSession upserts local session state.
func (s *Store) SaveSession(ctx context.Context, sess Session) error {
	now := time.Now().UTC()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	sess.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions(
  client_session_id, server_session_id, assignment_id, activity_revision_id,
  state, last_acked_sequence, next_sequence, workspace_path, created_at, updated_at
) VALUES(?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(client_session_id) DO UPDATE SET
  server_session_id = excluded.server_session_id,
  assignment_id = excluded.assignment_id,
  activity_revision_id = excluded.activity_revision_id,
  state = excluded.state,
  last_acked_sequence = excluded.last_acked_sequence,
  next_sequence = excluded.next_sequence,
  workspace_path = excluded.workspace_path,
  updated_at = excluded.updated_at`,
		sess.ClientSessionID, sess.ServerSessionID, sess.AssignmentID, sess.ActivityRevisionID,
		sess.State, sess.LastAckedSequence, sess.NextSequence, sess.WorkspacePath,
		sess.CreatedAt.UTC().Format(time.RFC3339Nano),
		sess.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// GetSession loads a session by client ID.
func (s *Store) GetSession(ctx context.Context, clientSessionID string) (*Session, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT client_session_id, server_session_id, assignment_id, activity_revision_id,
       state, last_acked_sequence, next_sequence, workspace_path, created_at, updated_at
FROM sessions WHERE client_session_id = ?`, clientSessionID)
	var sess Session
	var created, updated string
	err := row.Scan(
		&sess.ClientSessionID, &sess.ServerSessionID, &sess.AssignmentID, &sess.ActivityRevisionID,
		&sess.State, &sess.LastAckedSequence, &sess.NextSequence, &sess.WorkspacePath,
		&created, &updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &sess, nil
}

// BindServerSession records the server session id after StartSession.
func (s *Store) BindServerSession(ctx context.Context, clientSessionID, serverSessionID string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sessions SET server_session_id = ?, updated_at = ? WHERE client_session_id = ?`,
		serverSessionID, time.Now().UTC().Format(time.RFC3339Nano), clientSessionID)
	if err != nil {
		return err
	}
	// Also stamp pending outbox/completion/artifact rows that lack a server id.
	_, _ = s.db.ExecContext(ctx, `UPDATE event_outbox SET server_session_id = ? WHERE client_session_id = ? AND server_session_id = ''`,
		serverSessionID, clientSessionID)
	_, _ = s.db.ExecContext(ctx, `UPDATE completion_intents SET server_session_id = ? WHERE client_session_id = ? AND server_session_id = ''`,
		serverSessionID, clientSessionID)
	_, _ = s.db.ExecContext(ctx, `UPDATE artifacts_pending SET server_session_id = ? WHERE client_session_id = ? AND server_session_id = ''`,
		serverSessionID, clientSessionID)
	return nil
}

// FindOpenSessionByAssignment returns the most recently updated non-completed
// session for an assignment, if any.
func (s *Store) FindOpenSessionByAssignment(ctx context.Context, assignmentID string) (*Session, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT client_session_id, server_session_id, assignment_id, activity_revision_id,
       state, last_acked_sequence, next_sequence, workspace_path, created_at, updated_at
FROM sessions
WHERE assignment_id = ?
  AND state NOT IN ('completed', 'cancelled')
ORDER BY updated_at DESC
LIMIT 1`, assignmentID)
	var sess Session
	var created, updated string
	err := row.Scan(
		&sess.ClientSessionID, &sess.ServerSessionID, &sess.AssignmentID, &sess.ActivityRevisionID,
		&sess.State, &sess.LastAckedSequence, &sess.NextSequence, &sess.WorkspacePath,
		&created, &updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &sess, nil
}

// --- runner state ----------------------------------------------------------

// RunnerState is durable activity-runner progress for one client session.
type RunnerState struct {
	ClientSessionID string
	Kind            string
	StateJSON       []byte
	UpdatedAt       time.Time
}

// SaveRunnerState upserts encoded runner progress.
func (s *Store) SaveRunnerState(ctx context.Context, clientSessionID, kind string, stateJSON []byte) error {
	if clientSessionID == "" {
		return fmt.Errorf("client session id required")
	}
	if kind == "" {
		return fmt.Errorf("runner kind required")
	}
	if stateJSON == nil {
		stateJSON = []byte("{}")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runner_state(client_session_id, kind, state_json, updated_at) VALUES(?,?,?,?)
ON CONFLICT(client_session_id) DO UPDATE SET
  kind = excluded.kind,
  state_json = excluded.state_json,
  updated_at = excluded.updated_at`,
		clientSessionID, kind, string(stateJSON), now)
	return err
}

// GetRunnerState loads durable runner progress for a session.
func (s *Store) GetRunnerState(ctx context.Context, clientSessionID string) (*RunnerState, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT client_session_id, kind, state_json, updated_at
FROM runner_state WHERE client_session_id = ?`, clientSessionID)
	var rs RunnerState
	var raw, updated string
	err := row.Scan(&rs.ClientSessionID, &rs.Kind, &raw, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rs.StateJSON = []byte(raw)
	rs.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &rs, nil
}

// DeleteRunnerState removes durable runner progress (after completion/cancel).
func (s *Store) DeleteRunnerState(ctx context.Context, clientSessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM runner_state WHERE client_session_id = ?`, clientSessionID)
	return err
}

// --- event outbox ----------------------------------------------------------

// EnqueueEvent appends an event to the durable outbox and advances next_sequence.
// Sequence is assigned from the session counter if ev.Sequence < 0.
func (s *Store) EnqueueEvent(ctx context.Context, clientSessionID string, ev contracts.SessionEvent) (contracts.SessionEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ev, err
	}
	defer func() { _ = tx.Rollback() }()

	var serverID string
	var nextSeq int64
	err = tx.QueryRowContext(ctx, `
SELECT server_session_id, next_sequence FROM sessions WHERE client_session_id = ?`, clientSessionID).
		Scan(&serverID, &nextSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return ev, fmt.Errorf("session %s: %w", clientSessionID, ErrNotFound)
	}
	if err != nil {
		return ev, err
	}
	if ev.Sequence < 0 {
		ev.Sequence = nextSeq
	}
	if ev.SchemaVersion == "" {
		ev.SchemaVersion = contracts.EventSchemaVersion
	}
	if ev.ClientTime.IsZero() {
		ev.ClientTime = time.Now().UTC()
	}
	payload, err := json.Marshal(ev.Payload)
	if err != nil {
		return ev, err
	}
	if payload == nil {
		payload = []byte("{}")
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO event_outbox(
  client_session_id, server_session_id, sequence, event_id, event_type, client_time, payload_json, acked, created_at
) VALUES(?,?,?,?,?,?,?,0,?)`,
		clientSessionID, serverID, ev.Sequence, ev.EventID, ev.Type,
		ev.ClientTime.UTC().Format(time.RFC3339Nano), string(payload),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return ev, fmt.Errorf("insert outbox: %w", err)
	}
	// Advance next sequence past this event.
	newNext := nextSeq
	if ev.Sequence+1 > newNext {
		newNext = ev.Sequence + 1
	}
	_, err = tx.ExecContext(ctx, `
UPDATE sessions SET next_sequence = ?, updated_at = ? WHERE client_session_id = ?`,
		newNext, time.Now().UTC().Format(time.RFC3339Nano), clientSessionID)
	if err != nil {
		return ev, err
	}
	if err := tx.Commit(); err != nil {
		return ev, err
	}
	return ev, nil
}

// OutboxEvent is a pending or acked outbox row.
type OutboxEvent struct {
	ID              int64
	ClientSessionID string
	ServerSessionID string
	Event           contracts.SessionEvent
	Acked           bool
}

// ListPendingEvents returns unacked outbox events ordered by sequence.
func (s *Store) ListPendingEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, client_session_id, server_session_id, sequence, event_id, event_type, client_time, payload_json
FROM event_outbox WHERE acked = 0
ORDER BY client_session_id ASC, sequence ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxEvent
	for rows.Next() {
		var o OutboxEvent
		var clientTime, payload string
		if err := rows.Scan(
			&o.ID, &o.ClientSessionID, &o.ServerSessionID,
			&o.Event.Sequence, &o.Event.EventID, &o.Event.Type, &clientTime, &payload,
		); err != nil {
			return nil, err
		}
		o.Event.SchemaVersion = contracts.EventSchemaVersion
		o.Event.ClientTime, _ = time.Parse(time.RFC3339Nano, clientTime)
		if payload != "" && payload != "null" {
			_ = json.Unmarshal([]byte(payload), &o.Event.Payload)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// AckEvents marks outbox rows acked through the given sequence for a session
// and updates sessions.last_acked_sequence.
func (s *Store) AckEvents(ctx context.Context, clientSessionID string, throughSequence int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
UPDATE event_outbox SET acked = 1
WHERE client_session_id = ? AND sequence <= ? AND acked = 0`, clientSessionID, throughSequence)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE sessions SET last_acked_sequence = CASE
  WHEN last_acked_sequence < ? THEN ?
  ELSE last_acked_sequence
END, updated_at = ? WHERE client_session_id = ?`,
		throughSequence, throughSequence,
		time.Now().UTC().Format(time.RFC3339Nano), clientSessionID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// --- completions -----------------------------------------------------------

// CompletionIntent is a durable completion request awaiting server ack.
type CompletionIntent struct {
	CompletionID    string
	ClientSessionID string
	ServerSessionID string
	Request         contracts.CompletionRequest
	Response        *contracts.CompletionResult
	Acked           bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SaveCompletionIntent stores (or refreshes) a completion intent before sync.
func (s *Store) SaveCompletionIntent(ctx context.Context, clientSessionID, serverSessionID string, req contracts.CompletionRequest) error {
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO completion_intents(
  completion_id, client_session_id, server_session_id, request_json, response_json, acked, created_at, updated_at
) VALUES(?,?,?,?, '', 0, ?, ?)
ON CONFLICT(completion_id) DO UPDATE SET
  request_json = excluded.request_json,
  server_session_id = CASE
    WHEN excluded.server_session_id != '' THEN excluded.server_session_id
    ELSE completion_intents.server_session_id
  END,
  updated_at = excluded.updated_at`,
		req.CompletionID, clientSessionID, serverSessionID, string(raw), now, now)
	return err
}

// MarkCompletionAcked records the server response and marks the intent synced.
func (s *Store) MarkCompletionAcked(ctx context.Context, completionID string, result contracts.CompletionResult) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE completion_intents
SET acked = 1, response_json = ?, updated_at = ?
WHERE completion_id = ?`, string(raw), time.Now().UTC().Format(time.RFC3339Nano), completionID)
	return err
}

// ListPendingCompletions returns unacked completion intents.
func (s *Store) ListPendingCompletions(ctx context.Context) ([]CompletionIntent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT completion_id, client_session_id, server_session_id, request_json, response_json, acked, created_at, updated_at
FROM completion_intents WHERE acked = 0 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CompletionIntent
	for rows.Next() {
		var c CompletionIntent
		var reqRaw, respRaw, created, updated string
		var acked int
		if err := rows.Scan(&c.CompletionID, &c.ClientSessionID, &c.ServerSessionID, &reqRaw, &respRaw, &acked, &created, &updated); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(reqRaw), &c.Request); err != nil {
			return nil, err
		}
		c.Acked = acked != 0
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		if strings.TrimSpace(respRaw) != "" {
			var res contracts.CompletionResult
			if err := json.Unmarshal([]byte(respRaw), &res); err == nil {
				c.Response = &res
			}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCompletion returns a completion intent by id.
func (s *Store) GetCompletion(ctx context.Context, completionID string) (*CompletionIntent, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT completion_id, client_session_id, server_session_id, request_json, response_json, acked, created_at, updated_at
FROM completion_intents WHERE completion_id = ?`, completionID)
	var c CompletionIntent
	var reqRaw, respRaw, created, updated string
	var acked int
	err := row.Scan(&c.CompletionID, &c.ClientSessionID, &c.ServerSessionID, &reqRaw, &respRaw, &acked, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(reqRaw), &c.Request); err != nil {
		return nil, err
	}
	c.Acked = acked != 0
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if strings.TrimSpace(respRaw) != "" {
		var res contracts.CompletionResult
		if err := json.Unmarshal([]byte(respRaw), &res); err == nil {
			c.Response = &res
		}
	}
	return &c, nil
}

// --- artifacts -------------------------------------------------------------

// SavePendingArtifact queues artifact metadata for upload.
func (s *Store) SavePendingArtifact(ctx context.Context, clientSessionID, serverSessionID string, meta contracts.ArtifactMeta) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO artifacts_pending(artifact_id, client_session_id, server_session_id, meta_json, acked, created_at)
VALUES(?,?,?,?,0,?)
ON CONFLICT(artifact_id) DO UPDATE SET
  meta_json = excluded.meta_json,
  server_session_id = CASE
    WHEN excluded.server_session_id != '' THEN excluded.server_session_id
    ELSE artifacts_pending.server_session_id
  END`,
		meta.ArtifactID, clientSessionID, serverSessionID, string(raw),
		time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// PendingArtifact is queued artifact metadata.
type PendingArtifact struct {
	ArtifactID      string
	ClientSessionID string
	ServerSessionID string
	Meta            contracts.ArtifactMeta
	Acked           bool
}

// ListPendingArtifacts returns unacked artifacts.
func (s *Store) ListPendingArtifacts(ctx context.Context) ([]PendingArtifact, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT artifact_id, client_session_id, server_session_id, meta_json
FROM artifacts_pending WHERE acked = 0 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingArtifact
	for rows.Next() {
		var a PendingArtifact
		var raw string
		if err := rows.Scan(&a.ArtifactID, &a.ClientSessionID, &a.ServerSessionID, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &a.Meta); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkArtifactAcked marks an artifact as uploaded.
func (s *Store) MarkArtifactAcked(ctx context.Context, artifactID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE artifacts_pending SET acked = 1 WHERE artifact_id = ?`, artifactID)
	return err
}

// --- pending sync summary --------------------------------------------------

// PendingSync is a snapshot of unsynced durable work.
type PendingSync struct {
	Events        []OutboxEvent
	Completions   []CompletionIntent
	Artifacts     []PendingArtifact
	EventCount    int
	CompleteCount int
	ArtifactCount int
}

// GetPendingSync returns all pending outbox/completion/artifact rows.
func (s *Store) GetPendingSync(ctx context.Context) (*PendingSync, error) {
	evs, err := s.ListPendingEvents(ctx, 1000)
	if err != nil {
		return nil, err
	}
	cmps, err := s.ListPendingCompletions(ctx)
	if err != nil {
		return nil, err
	}
	arts, err := s.ListPendingArtifacts(ctx)
	if err != nil {
		return nil, err
	}
	return &PendingSync{
		Events:        evs,
		Completions:   cmps,
		Artifacts:     arts,
		EventCount:    len(evs),
		CompleteCount: len(cmps),
		ArtifactCount: len(arts),
	}, nil
}

// DecodeActivityContent converts a revision content map into ActivityContent.
func DecodeActivityContent(m map[string]any) (contracts.ActivityContent, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return contracts.ActivityContent{}, err
	}
	var c contracts.ActivityContent
	if err := json.Unmarshal(raw, &c); err != nil {
		return contracts.ActivityContent{}, fmt.Errorf("decode activity content: %w", err)
	}
	return c, nil
}
