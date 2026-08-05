package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// ErrConflict indicates an idempotency conflict (same key, different payload).
var ErrConflict = errors.New("conflict")

// StartOrResumeSession creates a session for clientSessionID or returns the existing one.
// capabilities lists runner capability flags (e.g. structured_command_evidence).
// When omitted, structured command evidence is treated as unsupported.
func StartOrResumeSession(ctx context.Context, q Querier, device *domain.StudentDevice, clientSessionID, assignmentID string, now time.Time, capabilities ...string) (*domain.LearningSession, error) {
	caps := map[string]bool{}
	for _, c := range capabilities {
		if c != "" {
			caps[c] = true
		}
	}
	var session *domain.LearningSession
	err := WithTx(ctx, q, func(tx Querier) error {
		if existing, err := sessionByClientID(ctx, tx, device.ID, clientSessionID); err == nil {
			session = existing
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}

		asg, err := StudentAssignments.Get(ctx, tx, assignmentID)
		if err != nil {
			return err
		}
		if asg.StudentID != device.StudentID {
			return ErrNotFound
		}
		if asg.State == domain.AssignmentCancelled {
			return ErrBadRequest{Msg: "assignment is cancelled"}
		}
		if asg.State == domain.AssignmentCompleted {
			return ErrBadRequest{Msg: "assignment is already completed"}
		}

		rev, err := GetRevision(ctx, tx, asg.ActivityRevisionID)
		if err != nil {
			return err
		}
		if err := RejectIncompatibleRevision(rev.Content, caps); err != nil {
			return err
		}

		// Resume open session on another device for the same assignment.
		if open, err := openSessionForAssignment(ctx, tx, assignmentID); err == nil {
			session = open
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}

		sess, err := LearningSessions.Create(ctx, tx, map[string]any{
			"assignment_id":        assignmentID,
			"student_id":           device.StudentID,
			"device_id":            device.ID,
			"client_session_id":    clientSessionID,
			"activity_revision_id": asg.ActivityRevisionID,
			"state":                domain.SessionStarted,
			"started_at":           now,
		})
		if err != nil {
			// Race on unique (device, client_session_id) or open assignment.
			if existing, e2 := sessionByClientID(ctx, tx, device.ID, clientSessionID); e2 == nil {
				session = existing
				return nil
			}
			if open, e2 := openSessionForAssignment(ctx, tx, assignmentID); e2 == nil {
				session = open
				return nil
			}
			return err
		}
		if asg.State == domain.AssignmentAvailable {
			if _, err := StudentAssignments.Update(ctx, tx, asg.ID, map[string]any{
				"state": domain.AssignmentInProgress,
			}); err != nil {
				return err
			}
		}
		session = sess
		return nil
	})
	return session, err
}

// RejectIncompatibleRevision returns ErrBadRequest when every required completion
// path needs structured command evidence the runner lacks.
func RejectIncompatibleRevision(contentMap map[string]any, capabilities map[string]bool) error {
	raw, err := json.Marshal(contentMap)
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}
	var content contracts.ActivityContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return fmt.Errorf("decode content: %w", err)
	}
	if err := contracts.RejectIncompatibleRevision(content, capabilities); err != nil {
		return ErrBadRequest{Msg: err.Error()}
	}
	return nil
}

// RevisionRequiresStructuredCommand reports whether required completion cannot
// succeed without structured command evidence (no filesystem-only path).
func RevisionRequiresStructuredCommand(content contracts.ActivityContent) bool {
	return contracts.RevisionRequiresStructuredCommand(content)
}

func sessionByClientID(ctx context.Context, q Querier, deviceID, clientSessionID string) (*domain.LearningSession, error) {
	const sqlStr = `
SELECT id, assignment_id, student_id, device_id, client_session_id, activity_revision_id,
       state, started_at, last_event_at, completed_at, duration_seconds, summary, created_at, updated_at
FROM learning_sessions
WHERE device_id = $1 AND client_session_id = $2`
	rows, err := q.Query(ctx, sqlStr, deviceID, clientSessionID)
	if err != nil {
		return nil, fmt.Errorf("query session by client id: %w", err)
	}
	s, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSession])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan session: %w", err)
	}
	return &s, nil
}

func openSessionForAssignment(ctx context.Context, q Querier, assignmentID string) (*domain.LearningSession, error) {
	const sqlStr = `
SELECT id, assignment_id, student_id, device_id, client_session_id, activity_revision_id,
       state, started_at, last_event_at, completed_at, duration_seconds, summary, created_at, updated_at
FROM learning_sessions
WHERE assignment_id = $1 AND state IN ('started', 'paused')
LIMIT 1`
	rows, err := q.Query(ctx, sqlStr, assignmentID)
	if err != nil {
		return nil, fmt.Errorf("query open session: %w", err)
	}
	s, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSession])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan open session: %w", err)
	}
	return &s, nil
}

// GetSession loads a session by id.
func GetSession(ctx context.Context, q Querier, id string) (*domain.LearningSession, error) {
	return LearningSessions.Get(ctx, q, id)
}

// IngestSessionEvents inserts an idempotent batch of events.
// Returns the highest contiguous acknowledged sequence from 0.
func IngestSessionEvents(ctx context.Context, q Querier, sessionID string, events []contracts.SessionEvent, now time.Time) (acked int64, err error) {
	err = WithTx(ctx, q, func(tx Querier) error {
		for _, ev := range events {
			if ev.EventID == "" {
				return ErrBadRequest{Msg: "eventId is required"}
			}
			payload := ev.Payload
			if payload == nil {
				payload = map[string]any{}
			}
			const ins = `
INSERT INTO learning_session_events
    (session_id, event_id, sequence, event_type, client_time, received_at, schema_version, payload)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (event_id) DO NOTHING`
			schema := ev.SchemaVersion
			if schema == "" {
				schema = contracts.EventSchemaVersion
			}
			if _, err := tx.Exec(ctx, ins,
				sessionID, ev.EventID, ev.Sequence, ev.Type, ev.ClientTime, now, schema, payload,
			); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					// sequence conflict with different event_id
					existing, e2 := eventBySequence(ctx, tx, sessionID, ev.Sequence)
					if e2 == nil && existing.EventID != ev.EventID {
						return fmt.Errorf("%w: sequence %d already held by another event", ErrConflict, ev.Sequence)
					}
					// same event_id already handled by ON CONFLICT
					continue
				}
				return fmt.Errorf("insert event: %w", err)
			}
		}
		if _, err := tx.Exec(ctx,
			`UPDATE learning_sessions SET last_event_at = $2, updated_at = now() WHERE id = $1`,
			sessionID, now); err != nil {
			return fmt.Errorf("touch session events: %w", err)
		}
		var maxSeq *int64
		if err := tx.QueryRow(ctx,
			`SELECT MAX(sequence) FROM learning_session_events WHERE session_id = $1`,
			sessionID).Scan(&maxSeq); err != nil {
			return fmt.Errorf("max sequence: %w", err)
		}
		if maxSeq == nil {
			acked = -1
			return nil
		}
		// Contiguous from 0.
		acked = contiguousAck(ctx, tx, sessionID, *maxSeq)
		return nil
	})
	return acked, err
}

func eventBySequence(ctx context.Context, q Querier, sessionID string, seq int64) (*domain.LearningSessionEvent, error) {
	const sqlStr = `
SELECT id, session_id, event_id, sequence, event_type, client_time, received_at, schema_version, payload
FROM learning_session_events WHERE session_id = $1 AND sequence = $2`
	rows, err := q.Query(ctx, sqlStr, sessionID, seq)
	if err != nil {
		return nil, err
	}
	ev, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSessionEvent])
	if err != nil {
		return nil, err
	}
	return &ev, nil
}

func contiguousAck(ctx context.Context, q Querier, sessionID string, maxSeq int64) int64 {
	// Find highest n such that 0..n all exist.
	rows, err := q.Query(ctx,
		`SELECT sequence FROM learning_session_events WHERE session_id = $1 AND sequence >= 0 ORDER BY sequence`,
		sessionID)
	if err != nil {
		return -1
	}
	defer rows.Close()
	expect := int64(0)
	acked := int64(-1)
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			return acked
		}
		if seq != expect {
			return acked
		}
		acked = seq
		expect++
		if acked >= maxSeq {
			break
		}
	}
	return acked
}

// CountSessionEvents returns how many events a session has (for tests).
func CountSessionEvents(ctx context.Context, q Querier, sessionID string) (int, error) {
	var n int
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM learning_session_events WHERE session_id = $1`, sessionID).Scan(&n)
	return n, err
}

// UpsertSessionArtifact stores artifact metadata idempotently by artifact_id.
func UpsertSessionArtifact(ctx context.Context, q Querier, sessionID string, meta contracts.ArtifactMeta) (*domain.LearningSessionArtifact, error) {
	const sqlStr = `
INSERT INTO learning_session_artifacts
    (session_id, artifact_id, filename, media_type, byte_size, sha256)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (artifact_id) DO UPDATE SET filename = EXCLUDED.filename
RETURNING id, session_id, artifact_id, filename, media_type, byte_size, sha256, storage_path, created_at`
	rows, err := q.Query(ctx, sqlStr, sessionID, meta.ArtifactID, meta.Filename, meta.MediaType, meta.ByteSize, meta.SHA256)
	if err != nil {
		return nil, fmt.Errorf("upsert artifact: %w", err)
	}
	art, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSessionArtifact])
	if err != nil {
		return nil, fmt.Errorf("scan artifact: %w", err)
	}
	return &art, nil
}

// GetCompletionBySession returns the completion row if present.
func GetCompletionBySession(ctx context.Context, q Querier, sessionID string) (*domain.LearningSessionCompletion, error) {
	const sqlStr = `
SELECT id, session_id, completion_id, request_digest, response, created_at
FROM learning_session_completions WHERE session_id = $1`
	rows, err := q.Query(ctx, sqlStr, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query completion: %w", err)
	}
	c, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSessionCompletion])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan completion: %w", err)
	}
	return &c, nil
}

// GetCompletionByID returns a completion by client completion UUID.
func GetCompletionByID(ctx context.Context, q Querier, completionID string) (*domain.LearningSessionCompletion, error) {
	const sqlStr = `
SELECT id, session_id, completion_id, request_digest, response, created_at
FROM learning_session_completions WHERE completion_id = $1`
	rows, err := q.Query(ctx, sqlStr, completionID)
	if err != nil {
		return nil, fmt.Errorf("query completion by id: %w", err)
	}
	c, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSessionCompletion])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan completion: %w", err)
	}
	return &c, nil
}

// InsertCompletion stores an immutable completion result.
func InsertCompletion(ctx context.Context, q Querier, sessionID, completionID, digest string, response contracts.CompletionResult) (*domain.LearningSessionCompletion, error) {
	raw, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshal completion response: %w", err)
	}
	var respMap map[string]any
	if err := json.Unmarshal(raw, &respMap); err != nil {
		return nil, err
	}
	const sqlStr = `
INSERT INTO learning_session_completions (session_id, completion_id, request_digest, response)
VALUES ($1, $2, $3, $4)
RETURNING id, session_id, completion_id, request_digest, response, created_at`
	rows, err := q.Query(ctx, sqlStr, sessionID, completionID, digest, respMap)
	if err != nil {
		return nil, fmt.Errorf("insert completion: %w", err)
	}
	c, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSessionCompletion])
	if err != nil {
		return nil, fmt.Errorf("scan completion: %w", err)
	}
	return &c, nil
}

// CompletionResultFromRow decodes the stored response JSON.
func CompletionResultFromRow(c *domain.LearningSessionCompletion) (contracts.CompletionResult, error) {
	raw, err := json.Marshal(c.Response)
	if err != nil {
		return contracts.CompletionResult{}, err
	}
	var out contracts.CompletionResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return contracts.CompletionResult{}, err
	}
	return out, nil
}

// MarkSessionCompleted transitions session and assignment to completed.
func MarkSessionCompleted(ctx context.Context, q Querier, session *domain.LearningSession, summary string, now time.Time) error {
	dur := int(now.Sub(session.StartedAt).Seconds())
	if dur < 0 {
		dur = 0
	}
	if _, err := LearningSessions.Update(ctx, q, session.ID, map[string]any{
		"state":            domain.SessionCompleted,
		"completed_at":     now,
		"duration_seconds": dur,
		"summary":          summary,
	}); err != nil {
		return err
	}
	if _, err := StudentAssignments.Update(ctx, q, session.AssignmentID, map[string]any{
		"state": domain.AssignmentCompleted,
	}); err != nil {
		return err
	}
	return nil
}

// ServerEventSequenceBase is the floor for server-originated session events
// (tutor exchanges). Client sequences stay in the low range so contiguous
// client acks are unaffected; uniqueness is still enforced per session.
const ServerEventSequenceBase int64 = 1_000_000_000

// InsertServerSessionEvent appends a server-origin event with an auto-allocated
// high sequence number. Payload should already omit secrets and raw transcripts.
func InsertServerSessionEvent(ctx context.Context, q Querier, sessionID, eventType string, payload map[string]any, now time.Time) error {
	if payload == nil {
		payload = map[string]any{}
	}
	return WithTx(ctx, q, func(tx Querier) error {
		var maxSeq *int64
		if err := tx.QueryRow(ctx,
			`SELECT MAX(sequence) FROM learning_session_events WHERE session_id = $1 AND sequence >= $2`,
			sessionID, ServerEventSequenceBase).Scan(&maxSeq); err != nil {
			return fmt.Errorf("max server sequence: %w", err)
		}
		seq := ServerEventSequenceBase
		if maxSeq != nil {
			seq = *maxSeq + 1
		}
		eventID := uuid.NewString()
		const ins = `
INSERT INTO learning_session_events
    (session_id, event_id, sequence, event_type, client_time, received_at, schema_version, payload)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
		if _, err := tx.Exec(ctx, ins,
			sessionID, eventID, seq, eventType, now, now, contracts.EventSchemaVersion, payload,
		); err != nil {
			return fmt.Errorf("insert server event: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE learning_sessions SET last_event_at = $2, updated_at = now() WHERE id = $1`,
			sessionID, now); err != nil {
			return fmt.Errorf("touch session: %w", err)
		}
		return nil
	})
}

// ListTutorReplyHints returns prior tutor reply strings for a session (oldest first).
func ListTutorReplyHints(ctx context.Context, q Querier, sessionID string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 20
	}
	const sqlStr = `
SELECT payload
FROM learning_session_events
WHERE session_id = $1 AND event_type = $2
ORDER BY sequence ASC
LIMIT $3`
	rows, err := q.Query(ctx, sqlStr, sessionID, contracts.EventTutorMessage, limit)
	if err != nil {
		return nil, fmt.Errorf("list tutor events: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var payload map[string]any
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		if payload == nil {
			continue
		}
		if reply, ok := payload["reply"].(string); ok && reply != "" {
			out = append(out, reply)
		}
	}
	return out, rows.Err()
}

// CountTutorEvents returns how many tutor_message events a session has.
func CountTutorEvents(ctx context.Context, q Querier, sessionID string) (int, error) {
	var n int
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM learning_session_events WHERE session_id = $1 AND event_type = $2`,
		sessionID, contracts.EventTutorMessage).Scan(&n)
	return n, err
}

// CountTutorEventsForStudent counts tutor_message events across a student's sessions.
func CountTutorEventsForStudent(ctx context.Context, q Querier, studentID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
SELECT COUNT(*)
FROM learning_session_events e
JOIN learning_sessions s ON s.id = e.session_id
WHERE s.student_id = $1 AND e.event_type = $2`, studentID, contracts.EventTutorMessage).Scan(&n)
	return n, err
}
