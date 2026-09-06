package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"primer-tasks/internal/verification"
)

const studentProtocolVersion = 1
const studentWriteTimeout = 2 * time.Second

type studentCommand struct {
	Protocol        int    `json:"protocol"`
	Kind            string `json:"kind"`
	OccurrenceID    string `json:"occurrenceId,omitempty"`
	AttemptID       string `json:"attemptId,omitempty"`
	QuestionID      string `json:"questionId,omitempty"`
	PolicyVersion   string `json:"policyVersion,omitempty"`
	SnapshotDigest  string `json:"snapshotDigest,omitempty"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
	Text            string `json:"text,omitempty"`
	ExpectedVersion int64  `json:"expectedVersion,omitempty"`
	Cursor          int64  `json:"cursor,omitempty"`
}
type wireStudentEvent = verification.DialogueEvent

type studentSubscriber struct {
	mu                  sync.Mutex
	occurrence, attempt string
	cursor, acked       int64
	lastAck             time.Time
	lastState           string
	queue               chan wireStudentEvent
}

func decodeStudentCommand(data []byte) (c studentCommand, err error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&c); err != nil {
		return c, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return c, errors.New("invalid command envelope")
	}
	if c.Protocol != studentProtocolVersion || c.Cursor < 0 {
		return c, errors.New("unsupported command envelope")
	}
	switch c.Kind {
	case "subscribe":
		if c.OccurrenceID == "" || c.AttemptID == "" {
			return c, errors.New("subscription binding required")
		}
	case "user_message":
		if c.OccurrenceID == "" || c.AttemptID == "" || c.QuestionID == "" || c.ExpectedVersion < 1 || c.ClientMessageID == "" || len(c.ClientMessageID) > 128 || c.Text == "" || len(c.Text) > 12000 || c.PolicyVersion == "" || len(c.SnapshotDigest) != 64 {
			return c, errors.New("answer binding required")
		}
	case "retry":
		if c.OccurrenceID == "" || c.AttemptID == "" || c.ExpectedVersion < 1 {
			return c, errors.New("retry binding required")
		}
	case "ack", "unsubscribe":
	default:
		return c, errors.New("unknown student command")
	}
	return c, nil
}
func studentError(err error) wireStudentEvent {
	e := wireStudentEvent{Protocol: 1, Kind: "error", Time: time.Now().UTC(), Code: "unavailable", Retryable: true}
	switch {
	case errors.Is(err, verification.ErrDialogueRevoked):
		e.Code, e.Retryable = "revoked", false
	case errors.Is(err, verification.ErrDialogueContext):
		e.Code, e.Retryable = "not_found", false
	case errors.Is(err, verification.ErrDialogueConflict):
		e.Code = "conflict"
	case errors.Is(err, verification.ErrDialogueQuestion), errors.Is(err, verification.ErrDialogueEvaluation):
		e.Code, e.Retryable = "invalid_request", false
	case errors.Is(err, verification.ErrDialogueTerminal), errors.Is(err, verification.ErrDialogueLimit):
		e.Code, e.Retryable = "exhausted", false
	}
	return e
}
func validateStudentEvent(e wireStudentEvent) error {
	if e.Protocol != 1 || e.Sequence < 0 || e.Cursor < 0 || e.Time.IsZero() {
		return errors.New("invalid student event")
	}
	switch e.Kind {
	case "hello", "state", "message_ack", "question", "progress", "answer_evaluation", "complete", "error", "override":
	default:
		return errors.New("unknown student event")
	}
	if len(e.Text) > 12000 || e.AcceptedCount < 0 || e.AcceptedCount > 3 {
		return errors.New("invalid student event bounds")
	}
	return nil
}

func (s *Server) writeStudentFrame(ctx context.Context, conn *websocket.Conn, a verification.StudentAuthority, event wireStudentEvent) error {
	if err := validateStudentEvent(event); err != nil {
		return err
	}
	frameCtx, cancel := context.WithTimeout(ctx, studentWriteTimeout+time.Second)
	defer cancel()
	tx, err := s.DB.Begin(frameCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(frameCtx)
	if _, err = tx.Exec(frameCtx, `SET TRANSACTION ISOLATION LEVEL READ COMMITTED`); err != nil {
		return err
	}
	if event.AttemptID != "" {
		if err = verification.LockDialogueAccess(frameCtx, tx, a, event.OccurrenceID, event.AttemptID, false); err != nil {
			return err
		}
	} else if err = verification.LockStudentAuthority(frameCtx, tx, a); err != nil {
		return err
	}
	// Exactly ONE bounded write holds revocation authority. No mutable progress
	// lock is held through the network, and no global hub broadcasts data.
	slow := time.AfterFunc(studentWriteTimeout, func() {
		_ = conn.Close(websocket.StatusTryAgainLater, "slow subscriber; reconnect using durable cursor")
	})
	defer slow.Stop()
	return wsjson.Write(frameCtx, conn, event)
}
func closeStudentDelivery(conn *websocket.Conn, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		_ = conn.Close(websocket.StatusTryAgainLater, "bounded student delivery timed out")
		return
	}
	_ = conn.Close(websocket.StatusPolicyViolation, "student dialogue authority unavailable")
}

func (s *Server) tailStudentSocket(ctx context.Context, conn *websocket.Conn, a verification.StudentAuthority, sub *studentSubscriber) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-sub.queue:
			sub.mu.Lock()
			if event.AttemptID == "" || (event.AttemptID == sub.attempt && event.OccurrenceID == sub.occurrence) {
				if err := s.writeStudentFrame(ctx, conn, a, event); err != nil {
					sub.mu.Unlock()
					closeStudentDelivery(conn, err)
					return
				}
			}
			sub.mu.Unlock()
		case <-ticker.C:
		}
		check, done := context.WithTimeout(ctx, 3*time.Second)
		tx, err := s.DB.Begin(check)
		if err == nil {
			err = verification.LockStudentAuthority(check, tx, a)
			_ = tx.Rollback(check)
		}
		done()
		if err != nil {
			closeStudentDelivery(conn, err)
			return
		}
		sub.mu.Lock()
		if sub.attempt == "" {
			sub.mu.Unlock()
			continue
		}
		check, done = context.WithTimeout(ctx, 3*time.Second)
		state, err := s.readStudentDialogueState(check, a, sub.occurrence, sub.attempt)
		done()
		if err != nil {
			sub.mu.Unlock()
			closeStudentDelivery(conn, err)
			return
		}
		// State is ephemeral and never advances the durable event cursor. This
		// reflects failure/terminal state after restart without inventing events.
		signature := state.Status + ":" + state.OccurrenceStatus + ":" + state.Phase + ":" + state.Code
		if signature != sub.lastState {
			if err = s.writeStudentFrame(ctx, conn, a, state); err != nil {
				sub.mu.Unlock()
				closeStudentDelivery(conn, err)
				return
			}
			sub.lastState = signature
		}
		capacity := int64(64) - (sub.cursor - sub.acked)
		if capacity <= 0 {
			expired := time.Since(sub.lastAck) > studentWriteTimeout
			sub.mu.Unlock()
			if expired {
				_ = conn.Close(websocket.StatusTryAgainLater, "student acknowledgement window exhausted")
				return
			}
			continue
		}
		check, done = context.WithTimeout(ctx, 3*time.Second)
		rows, err := s.DB.Query(check, `SELECT sequence,payload FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2 AND sequence>$3 ORDER BY sequence LIMIT $4`, a.TenantID, sub.attempt, sub.cursor, min(int64(32), capacity))
		var events []wireStudentEvent
		if err == nil {
			for rows.Next() {
				var seq int64
				var raw []byte
				var event wireStudentEvent
				if err = rows.Scan(&seq, &raw); err != nil {
					break
				}
				if err = json.Unmarshal(raw, &event); err != nil {
					break
				}
				if event.Sequence != seq || event.Cursor != seq || event.AttemptID != sub.attempt || event.OccurrenceID != sub.occurrence {
					err = errors.New("corrupt durable student event")
					break
				}
				events = append(events, event)
			}
			if err == nil {
				err = rows.Err()
			}
			rows.Close()
		}
		done()
		if err != nil {
			sub.mu.Unlock()
			closeStudentDelivery(conn, err)
			return
		}
		for _, event := range events {
			if err = s.writeStudentFrame(ctx, conn, a, event); err != nil {
				break
			}
			sub.cursor = event.Sequence
		}
		sub.mu.Unlock()
		if err != nil {
			closeStudentDelivery(conn, err)
			return
		}
	}
}
