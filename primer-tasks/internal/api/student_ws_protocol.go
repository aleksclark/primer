package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type studentDialogueHub struct {
	mu          sync.Mutex
	subscribers map[*studentSubscriber]struct{}
}

type studentSubscriber struct {
	tenant, student, occurrence, attempt string
	queue                                chan wireStudentEvent
	done                                 chan struct{}
	cancel                               context.CancelFunc
	once                                 sync.Once
}

type wireStudentEvent struct {
	Type             string    `json:"kind"`
	ProtocolVersion  int       `json:"protocol"`
	AttemptID        string    `json:"attemptId,omitempty"`
	OccurrenceID     string    `json:"occurrenceId,omitempty"`
	Sequence         int64     `json:"sequence"`
	Cursor           int64     `json:"cursor"`
	TenantID         string    `json:"-"`
	ConnectionID     string    `json:"connectionId,omitempty"`
	HeartbeatSeconds int       `json:"heartbeatSeconds,omitempty"`
	MessageID        string    `json:"messageId,omitempty"`
	ClientMessageID  string    `json:"clientMessageId,omitempty"`
	Text             string    `json:"text,omitempty"`
	Role             string    `json:"role,omitempty"`
	Phase            string    `json:"phase,omitempty"`
	Status           string    `json:"status,omitempty"`
	Code             string    `json:"code,omitempty"`
	Message          string    `json:"message,omitempty"`
	Retryable        bool      `json:"retryable,omitempty"`
	AcceptedCount    int       `json:"acceptedCount,omitempty"`
	RequiredCount    int       `json:"requiredCount,omitempty"`
	QuestionKey      string    `json:"questionKey,omitempty"`
	Time             time.Time `json:"time"`
}

type studentAttemptBinding struct {
	TenantID      string
	OccurrenceID  string
	AttemptID     string
	RequirementID string
	Status        string
	Kind          string
	Accepted      int
	Required      int
	NextSequence  int64
	Config        []byte
}

var studentHub = newStudentDialogueHub()

func newStudentDialogueHub() *studentDialogueHub {
	return &studentDialogueHub{subscribers: make(map[*studentSubscriber]struct{})}
}

func (s *Server) studentDialogueHub() *studentDialogueHub {
	return studentHub
}

func (h *studentDialogueHub) add(sub *studentSubscriber) {
	h.mu.Lock()
	h.subscribers[sub] = struct{}{}
	h.mu.Unlock()
}

func (h *studentDialogueHub) remove(sub *studentSubscriber) {
	h.mu.Lock()
	delete(h.subscribers, sub)
	h.mu.Unlock()
	sub.close()
}

func (sub *studentSubscriber) close() {
	sub.once.Do(func() {
		close(sub.done)
		if sub.cancel != nil {
			sub.cancel()
		}
	})
}

func (sub *studentSubscriber) enqueue(event wireStudentEvent) bool {
	select {
	case <-sub.done:
		return false
	default:
	}
	select {
	case sub.queue <- event:
		return true
	case <-sub.done:
		return false
	default:
		return false
	}
}

func (h *studentDialogueHub) publish(event wireStudentEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subscribers {
		if sub.tenant != event.TenantID {
			continue
		}
		if sub.attempt != "" && sub.attempt != event.AttemptID {
			continue
		}
		if !sub.enqueue(event) {
			sub.close()
		}
	}
}

func (s *Server) sendStudentToSubscriber(sub *studentSubscriber, event wireStudentEvent) {
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if event.ProtocolVersion == 0 {
		event.ProtocolVersion = studentProtocolVersion
	}
	if !sub.enqueue(event) {
		s.studentDialogueHub().remove(sub)
	}
}

func (s *Server) bindStudentAttempt(ctx context.Context, identity studentIdentity, occurrenceID, attemptID string) (studentAttemptBinding, error) {
	var binding studentAttemptBinding
	if occurrenceID == "" && attemptID == "" {
		return binding, errors.New("attempt required")
	}
	query := `
SELECT a.tenant_id, a.occurrence_id::text, a.id::text, a.requirement_id::text, a.status, r.kind,
       COALESCE(d.accepted_count, 0),
       COALESCE((d.config_snapshot->>'requiredAccepted')::int, (r.config->>'requiredAccepted')::int, 3),
       COALESCE(d.next_sequence, 1),
       COALESCE(d.config_snapshot, r.config)
  FROM verification_attempts a
  JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=a.occurrence_id
  JOIN verification_requirements r ON r.tenant_id=a.tenant_id AND r.id=a.requirement_id
  LEFT JOIN dialogue_attempts d ON d.tenant_id=a.tenant_id AND d.attempt_id=a.id
 WHERE o.student_id=$1 AND a.tenant_id=$2`
	args := []any{identity.StudentID, identity.TenantID}
	switch {
	case attemptID != "" && occurrenceID != "":
		query += ` AND a.id=$3 AND a.occurrence_id=$4`
		args = append(args, attemptID, occurrenceID)
	case attemptID != "":
		query += ` AND a.id=$3`
		args = append(args, attemptID)
	default:
		query += ` AND a.occurrence_id=$3 ORDER BY a.number DESC LIMIT 1`
		args = append(args, occurrenceID)
	}
	err := s.DB.QueryRow(ctx, query, args...).Scan(
		&binding.TenantID, &binding.OccurrenceID, &binding.AttemptID, &binding.RequirementID, &binding.Status, &binding.Kind,
		&binding.Accepted, &binding.Required, &binding.NextSequence, &binding.Config,
	)
	if err != nil {
		return binding, err
	}
	if binding.Kind != "agent_dialogue" {
		return binding, errors.New("not a dialogue attempt")
	}
	return binding, nil
}

func (s *Server) studentSubscribe(ctx context.Context, identity studentIdentity, sub *studentSubscriber, cmd studentCommand) {
	binding, err := s.bindStudentAttempt(ctx, identity, cmd.OccurrenceID, cmd.AttemptID)
	if err != nil {
		s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, Code: "not_found", Message: "This task is unavailable.", Retryable: false})
		return
	}
	sub.attempt = binding.AttemptID
	sub.occurrence = binding.OccurrenceID
	sub.tenant = binding.TenantID
	sub.student = identity.StudentID.String()
	s.sendStudentToSubscriber(sub, wireStudentEvent{
		Type:          "state",
		AttemptID:     binding.AttemptID,
		OccurrenceID:  binding.OccurrenceID,
		Status:        binding.Status,
		AcceptedCount: binding.Accepted,
		RequiredCount: binding.Required,
		Cursor:        cmd.Cursor,
		Sequence:      cmd.Cursor,
	})
	rows, err := s.DB.Query(ctx, `SELECT sequence, kind, payload FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2 AND sequence>$3 ORDER BY sequence LIMIT 200`, binding.TenantID, binding.AttemptID, cmd.Cursor)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var sequence int64
		var kind string
		var payload []byte
		if err := rows.Scan(&sequence, &kind, &payload); err != nil {
			return
		}
		event := wireStudentEvent{Type: kind, ProtocolVersion: studentProtocolVersion, AttemptID: binding.AttemptID, OccurrenceID: binding.OccurrenceID, Sequence: sequence, Cursor: sequence, TenantID: binding.TenantID}
		if json.Unmarshal(payload, &event) != nil {
			continue
		}
		event.Sequence, event.Cursor, event.TenantID = sequence, sequence, binding.TenantID
		if !sub.enqueue(event) {
			s.studentDialogueHub().remove(sub)
			return
		}
	}
}

func (s *Server) studentMessage(ctx context.Context, identity studentIdentity, sub *studentSubscriber, cmd studentCommand) {
	text := strings.TrimSpace(cmd.Text)
	if text == "" || cmd.ClientMessageID == "" {
		s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, Code: "invalid_request", Message: "A message and idempotency key are required.", Retryable: false})
		return
	}
	binding, err := s.bindStudentAttempt(ctx, identity, cmd.OccurrenceID, cmd.AttemptID)
	if err != nil {
		s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, Code: "not_found", Message: "This task is unavailable.", Retryable: false})
		return
	}
	if binding.Status != "open" {
		s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, AttemptID: binding.AttemptID, OccurrenceID: binding.OccurrenceID, Code: "conflict", Message: "This attempt is no longer accepting answers.", Retryable: false})
		return
	}
	messageID, sequence, inserted, conflict, err := s.appendStudentMessage(ctx, binding, identity, cmd.ClientMessageID, text, cmd.ExpectedSequence)
	if err != nil {
		s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, AttemptID: binding.AttemptID, OccurrenceID: binding.OccurrenceID, Code: "internal", Message: "Unable to record the answer.", Retryable: true})
		return
	}
	if conflict {
		s.sendStudentToSubscriber(sub, wireStudentEvent{
			Type:            "error",
			ProtocolVersion: studentProtocolVersion,
			AttemptID:       binding.AttemptID,
			OccurrenceID:    binding.OccurrenceID,
			Code:            "conflict",
			Message:         "Another device is already answering this turn.",
			Retryable:       true,
			Cursor:          sequence,
			Sequence:        sequence,
		})
		return
	}
	ack := wireStudentEvent{
		Type:            "message_ack",
		ProtocolVersion: studentProtocolVersion,
		AttemptID:       binding.AttemptID,
		OccurrenceID:    binding.OccurrenceID,
		MessageID:       messageID,
		ClientMessageID: cmd.ClientMessageID,
		Text:            text,
		Role:            "student",
		Sequence:        sequence,
		Cursor:          sequence,
		AcceptedCount:   binding.Accepted,
		RequiredCount:   binding.Required,
		TenantID:        binding.TenantID,
	}
	if err := s.persistStudentEvent(ctx, binding, ack); err != nil {
		s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, AttemptID: binding.AttemptID, OccurrenceID: binding.OccurrenceID, Code: "internal", Message: "Unable to record the answer.", Retryable: true})
		return
	}
	if inserted {
		if err := s.enqueueDialogueJob(ctx, binding, messageID); err != nil {
			s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, AttemptID: binding.AttemptID, OccurrenceID: binding.OccurrenceID, Code: "internal", Message: "The answer was saved but could not start evaluation.", Retryable: true})
			return
		}
		progress := wireStudentEvent{
			Type:            "progress",
			ProtocolVersion: studentProtocolVersion,
			AttemptID:       binding.AttemptID,
			OccurrenceID:    binding.OccurrenceID,
			Phase:           "evaluating",
			Status:          "queued",
			Retryable:       false,
			AcceptedCount:   binding.Accepted,
			RequiredCount:   binding.Required,
			TenantID:        binding.TenantID,
		}
		_ = s.persistStudentEvent(ctx, binding, progress)
	}
}

func (s *Server) studentRetry(ctx context.Context, identity studentIdentity, sub *studentSubscriber, cmd studentCommand) {
	binding, err := s.bindStudentAttempt(ctx, identity, cmd.OccurrenceID, cmd.AttemptID)
	if err != nil {
		s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, Code: "not_found", Message: "This task is unavailable.", Retryable: false})
		return
	}
	var messageID *string
	err = s.DB.QueryRow(ctx, `SELECT message_id FROM verification_jobs WHERE tenant_id=$1 AND attempt_id=$2 AND status='failed' ORDER BY updated_at DESC LIMIT 1`, binding.TenantID, binding.AttemptID).Scan(&messageID)
	if err != nil {
		s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, AttemptID: binding.AttemptID, OccurrenceID: binding.OccurrenceID, Code: "conflict", Message: "There is no failed evaluation to retry.", Retryable: false})
		return
	}
	if err := s.enqueueDialogueJob(ctx, binding, stringPtr(messageID)); err != nil {
		s.sendStudentToSubscriber(sub, wireStudentEvent{Type: "error", ProtocolVersion: studentProtocolVersion, AttemptID: binding.AttemptID, OccurrenceID: binding.OccurrenceID, Code: "internal", Message: "Unable to retry evaluation.", Retryable: true})
		return
	}
	_ = s.persistStudentEvent(ctx, binding, wireStudentEvent{
		Type:          "progress",
		AttemptID:     binding.AttemptID,
		OccurrenceID:  binding.OccurrenceID,
		Phase:         "retry",
		Status:        "queued",
		Retryable:     true,
		AcceptedCount: binding.Accepted,
		RequiredCount: binding.Required,
		TenantID:      binding.TenantID,
	})
}

func stringPtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *Server) appendStudentMessage(ctx context.Context, binding studentAttemptBinding, identity studentIdentity, clientMessageID, text string, expectedSequence int64) (string, int64, bool, bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", 0, false, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, binding.TenantID+":"+binding.AttemptID); err != nil {
		return "", 0, false, false, err
	}
	var existingID string
	var existingSeq int64
	err = tx.QueryRow(ctx, `SELECT id::text, sequence FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2 AND client_message_id=$3`, binding.TenantID, binding.AttemptID, clientMessageID).Scan(&existingID, &existingSeq)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return "", 0, false, false, err
		}
		return existingID, existingSeq, false, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, false, err
	}
	var next int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(sequence),0)+1 FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2`, binding.TenantID, binding.AttemptID).Scan(&next); err != nil {
		return "", 0, false, false, err
	}
	if expectedSequenceConflict(expectedSequence, next) {
		if err = tx.Commit(ctx); err != nil {
			return "", 0, false, false, err
		}
		return "", next, false, true, nil
	}
	var openJobs int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM verification_jobs WHERE tenant_id=$1 AND attempt_id=$2 AND status IN ('queued','running')`, binding.TenantID, binding.AttemptID).Scan(&openJobs); err != nil {
		return "", 0, false, false, err
	}
	if openJobs > 0 {
		if err = tx.Commit(ctx); err != nil {
			return "", 0, false, false, err
		}
		return "", next, false, true, nil
	}
	messageID := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO verification_messages(id,tenant_id,attempt_id,sequence,role,content,client_message_id) VALUES($1,$2,$3,$4,'student',$5,$6)`, messageID, binding.TenantID, binding.AttemptID, next, text, clientMessageID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23505") {
			if err = tx.QueryRow(ctx, `SELECT id::text, sequence FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2 AND client_message_id=$3`, binding.TenantID, binding.AttemptID, clientMessageID).Scan(&existingID, &existingSeq); err == nil {
				if commitErr := tx.Commit(ctx); commitErr != nil {
					return "", 0, false, false, commitErr
				}
				return existingID, existingSeq, false, false, nil
			}
		}
		return "", 0, false, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,policy_version,config_snapshot,accepted_count,turn_count,next_sequence)
VALUES($1,$2,$3,$4,'dialogue.v1',$5,0,1,$6)
ON CONFLICT (tenant_id,attempt_id) DO UPDATE SET turn_count=dialogue_attempts.turn_count+1, next_sequence=EXCLUDED.next_sequence, updated_at=now()`,
		binding.TenantID, binding.AttemptID, binding.OccurrenceID, binding.RequirementID, binding.Config, next+1); err != nil {
		return "", 0, false, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", 0, false, false, err
	}
	return messageID, next, true, false, nil
}

func (s *Server) persistStudentEvent(ctx context.Context, binding studentAttemptBinding, event wireStudentEvent) error {
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	event.ProtocolVersion = studentProtocolVersion
	event.AttemptID = binding.AttemptID
	event.OccurrenceID = binding.OccurrenceID
	event.TenantID = binding.TenantID
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, binding.TenantID+":events:"+binding.AttemptID); err != nil {
		return err
	}
	var seq int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(sequence),0)+1 FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2`, binding.TenantID, binding.AttemptID).Scan(&seq); err != nil {
		return err
	}
	event.Sequence, event.Cursor = seq, seq
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO verification_events(tenant_id,attempt_id,sequence,kind,payload) VALUES($1,$2,$3,$4,$5)`, binding.TenantID, binding.AttemptID, seq, event.Type, payload); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.studentDialogueHub().publish(event)
	return nil
}

func (s *Server) enqueueDialogueJob(ctx context.Context, binding studentAttemptBinding, messageID string) error {
	var existing string
	err := s.DB.QueryRow(ctx, `SELECT id::text FROM verification_jobs WHERE tenant_id=$1 AND message_id=$2`, binding.TenantID, messageID).Scan(&existing)
	if err == nil {
		_, err = s.DB.Exec(ctx, `UPDATE verification_jobs SET status='queued', available_at=now(), last_error=NULL, updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='failed'`, binding.TenantID, existing)
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,message_id,status,max_attempts) VALUES($1,$2,$3,$4,'queued',3)`, uuid.New(), binding.TenantID, binding.AttemptID, messageID)
	return err
}
