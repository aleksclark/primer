// Package repo contains the SQL persistence boundary for verification.
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"primer-tasks/internal/domain"
	"primer-tasks/internal/verification"
)

type DialogueAttempt struct {
	TenantID, AttemptID, OccurrenceID, RequirementID, PolicyVersion string
	ConfigSnapshot                                                  map[string]any
	AcceptedCount, TurnCount                                        int
	NextSequence                                                    int64
	CreatedAt, UpdatedAt                                            time.Time
}

type DialogueRepository struct{ DB *pgxpool.Pool }

func NewDialogueRepository(db *pgxpool.Pool) *DialogueRepository { return &DialogueRepository{DB: db} }

func (r *DialogueRepository) CreateDialogueAttempt(ctx context.Context, a DialogueAttempt) error {
	if r == nil || r.DB == nil {
		return errors.New("dialogue repository database is required")
	}
	if a.TenantID == "" || a.AttemptID == "" || a.OccurrenceID == "" || a.RequirementID == "" || a.PolicyVersion == "" || a.NextSequence < 1 {
		return errors.New("invalid dialogue attempt")
	}
	if _, err := domain.ParseDialogueConfig(a.ConfigSnapshot); err != nil {
		return err
	}
	config, err := json.Marshal(a.ConfigSnapshot)
	if err != nil {
		return err
	}
	_, err = r.DB.Exec(ctx, `INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,policy_version,config_snapshot,accepted_count,turn_count,next_sequence,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,0,0,1,COALESCE($7,now()),COALESCE($7,now()))`, a.TenantID, a.AttemptID, a.OccurrenceID, a.RequirementID, a.PolicyVersion, config, a.CreatedAt)
	return err
}

func decodeConfig(raw []byte) (domain.DialogueConfig, error) {
	var c domain.DialogueConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

func (r *DialogueRepository) GetDialogueState(ctx context.Context, scope verification.DialogueContext) (state verification.DialogueState, err error) {
	if r == nil || r.DB == nil {
		return state, errors.New("dialogue repository database is required")
	}
	if err = scope.Validate(); err != nil {
		return
	}
	var raw []byte
	var status string
	err = r.DB.QueryRow(ctx, `SELECT d.config_snapshot,d.accepted_count,d.turn_count,a.status FROM dialogue_attempts d JOIN verification_attempts a ON a.tenant_id=d.tenant_id AND a.id=d.attempt_id JOIN task_occurrences o ON o.tenant_id=d.tenant_id AND o.id=d.occurrence_id WHERE d.tenant_id=$1 AND d.attempt_id=$2 AND d.occurrence_id=$3 AND d.requirement_id=$4 AND d.policy_version=$5 AND o.student_id=$6`, scope.TenantID, scope.AttemptID, scope.OccurrenceID, scope.RequirementID, scope.PolicyVersion, scope.StudentID).Scan(&raw, &state.AcceptedCount, &state.TurnCount, &status)
	if err == pgx.ErrNoRows {
		return state, verification.ErrDialogueContext
	}
	if err != nil {
		return
	}
	state.Context = scope
	state.Config, err = decodeConfig(raw)
	if err != nil {
		return state, err
	}
	state.Terminal = status != "open"
	state.Questions, err = r.questions(ctx, scope)
	if err != nil {
		return
	}
	state.Evaluations, err = r.evaluations(ctx, scope)
	if err != nil {
		return
	}
	state.Messages, err = r.messages(ctx, scope, 100)
	return
}

func (r *DialogueRepository) questions(ctx context.Context, s verification.DialogueContext) ([]verification.DialogueQuestion, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,attempt_id,question_key,ordinal,prompt,created_at FROM dialogue_questions WHERE tenant_id=$1 AND attempt_id=$2 ORDER BY ordinal LIMIT 100`, s.TenantID, s.AttemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []verification.DialogueQuestion
	for rows.Next() {
		var q verification.DialogueQuestion
		if err := rows.Scan(&q.ID, &q.AttemptID, &q.QuestionKey, &q.Ordinal, &q.Prompt, &q.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (r *DialogueRepository) evaluations(ctx context.Context, s verification.DialogueContext) ([]verification.DialogueEvaluation, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,attempt_id,question_id,message_id,accepted,rationale,provider,model,policy_version,usage,created_at FROM verification_evaluations WHERE tenant_id=$1 AND attempt_id=$2 ORDER BY created_at,id LIMIT 200`, s.TenantID, s.AttemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []verification.DialogueEvaluation
	for rows.Next() {
		var e verification.DialogueEvaluation
		var usage []byte
		if err := rows.Scan(&e.ID, &e.AttemptID, &e.QuestionID, &e.MessageID, &e.Accepted, &e.Rationale, &e.Provider, &e.Model, &e.PolicyVersion, &usage, &e.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(usage, &e.Usage)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *DialogueRepository) messages(ctx context.Context, s verification.DialogueContext, limit int) ([]verification.DialogueMessage, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,role,content,sequence,created_at FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2 ORDER BY sequence DESC LIMIT $3`, s.TenantID, s.AttemptID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reverse []verification.DialogueMessage
	for rows.Next() {
		var m verification.DialogueMessage
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.Sequence, &m.CreatedAt); err != nil {
			return nil, err
		}
		reverse = append(reverse, m)
	}
	out := make([]verification.DialogueMessage, len(reverse))
	for i := range reverse {
		out[len(reverse)-1-i] = reverse[i]
	}
	return out, rows.Err()
}

// AppendMessage allocates the ordered attempt sequence while holding the
// dialogue attempt lock. Client sequence values are never trusted.
func (r *DialogueRepository) AppendMessage(ctx context.Context, s verification.DialogueContext, id, role, content, clientID string) (verification.DialogueMessage, bool, error) {
	if r == nil || r.DB == nil {
		return verification.DialogueMessage{}, false, errors.New("dialogue repository database is required")
	}
	if err := s.Validate(); err != nil {
		return verification.DialogueMessage{}, false, err
	}
	// The occurrence/student join is the authorization check for every append,
	// not only for the websocket upgrade path.
	if _, err := r.GetDialogueState(ctx, s); err != nil {
		return verification.DialogueMessage{}, false, err
	}
	if role != "student" && role != "agent" && role != "system" {
		return verification.DialogueMessage{}, false, errors.New("invalid dialogue message role")
	}
	if strings.TrimSpace(clientID) == "" {
		return verification.DialogueMessage{}, false, errors.New("client message id is required")
	}
	if len([]rune(content)) == 0 || len([]rune(content)) > 12000 {
		return verification.DialogueMessage{}, false, errors.New("invalid dialogue message")
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return verification.DialogueMessage{}, false, err
	}
	defer tx.Rollback(ctx)
	var seq int64
	if err = tx.QueryRow(ctx, `SELECT next_sequence FROM dialogue_attempts WHERE tenant_id=$1 AND attempt_id=$2 FOR UPDATE`, s.TenantID, s.AttemptID).Scan(&seq); err != nil {
		return verification.DialogueMessage{}, false, err
	}
	var m verification.DialogueMessage
	err = tx.QueryRow(ctx, `INSERT INTO verification_messages(id,tenant_id,attempt_id,sequence,role,content,client_message_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant_id,attempt_id,client_message_id) DO NOTHING RETURNING id,role,content,sequence,created_at`, id, s.TenantID, s.AttemptID, seq, role, content, clientID).Scan(&m.ID, &m.Role, &m.Content, &m.Sequence, &m.CreatedAt)
	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx, `SELECT id,role,content,sequence,created_at FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2 AND client_message_id=$3`, s.TenantID, s.AttemptID, clientID).Scan(&m.ID, &m.Role, &m.Content, &m.Sequence, &m.CreatedAt)
		return m, false, err
	}
	if err != nil {
		return m, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dialogue_attempts SET next_sequence=next_sequence+1,updated_at=now() WHERE tenant_id=$1 AND attempt_id=$2`, s.TenantID, s.AttemptID); err != nil {
		return m, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return m, false, err
	}
	return m, true, nil
}

func (r *DialogueRepository) RecordQuestion(ctx context.Context, s verification.DialogueContext, q verification.DialogueQuestion) (verification.DialogueQuestion, bool, error) {
	if r == nil || r.DB == nil {
		return q, false, errors.New("dialogue repository database is required")
	}
	if err := s.Validate(); err != nil {
		return q, false, err
	}
	state, err := r.GetDialogueState(ctx, s)
	if err != nil {
		return q, false, err
	}
	state, err = verification.RecordQuestion(state, q)
	if err != nil {
		return q, false, err
	}
	_, err = r.DB.Exec(ctx, `INSERT INTO dialogue_questions(id,tenant_id,attempt_id,question_key,ordinal,prompt) VALUES($1,$2,$3,$4,$5,$6)`, q.ID, s.TenantID, s.AttemptID, q.QuestionKey, q.Ordinal, q.Prompt)
	if err == nil {
		_, err = r.DB.Exec(ctx, `UPDATE dialogue_attempts SET turn_count=$3,updated_at=now() WHERE tenant_id=$1 AND attempt_id=$2`, s.TenantID, s.AttemptID, state.TurnCount)
	}
	if err != nil {
		if !isUniqueViolation(err) {
			return q, false, err
		}
		var existing verification.DialogueQuestion
		err = r.DB.QueryRow(ctx, `SELECT id,attempt_id,question_key,ordinal,prompt,created_at FROM dialogue_questions WHERE tenant_id=$1 AND attempt_id=$2 AND question_key=$3`, s.TenantID, s.AttemptID, q.QuestionKey).Scan(&existing.ID, &existing.AttemptID, &existing.QuestionKey, &existing.Ordinal, &existing.Prompt, &existing.CreatedAt)
		return existing, false, err
	}
	return q, true, nil
}

func (r *DialogueRepository) RecordAnswerEvaluation(ctx context.Context, s verification.DialogueContext, key string, e verification.DialogueEvaluation) (verification.DialogueEvaluation, verification.DecisionReady, bool, error) {
	if r == nil || r.DB == nil {
		return e, verification.DecisionReady{}, false, errors.New("dialogue repository database is required")
	}
	state, err := r.GetDialogueState(ctx, s)
	if err != nil {
		return e, verification.DecisionReady{}, false, err
	}
	var q verification.DialogueQuestion
	for _, candidate := range state.Questions {
		if candidate.QuestionKey == key {
			q = candidate
			break
		}
	}
	if q.ID == "" {
		return e, verification.DecisionReady{}, false, verification.ErrDialogueEvaluation
	}
	e.QuestionID = q.ID
	if _, _, err = verification.RecordEvaluation(state, e); err != nil {
		return e, verification.DecisionReady{}, false, err
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return e, verification.DecisionReady{}, false, err
	}
	defer tx.Rollback(ctx)
	var status string
	var raw []byte
	var acceptedCount int
	if err = tx.QueryRow(ctx, `SELECT a.status,d.config_snapshot,d.accepted_count FROM dialogue_attempts d JOIN verification_attempts a ON a.tenant_id=d.tenant_id AND a.id=d.attempt_id WHERE d.tenant_id=$1 AND d.attempt_id=$2 FOR UPDATE`, s.TenantID, s.AttemptID).Scan(&status, &raw, &acceptedCount); err != nil {
		return e, verification.DecisionReady{}, false, err
	}
	if status != "open" {
		return e, verification.DecisionReady{}, false, verification.ErrDialogueTerminal
	}
	var role string
	if err = tx.QueryRow(ctx, `SELECT role FROM verification_messages WHERE tenant_id=$1 AND id=$2 AND attempt_id=$3`, s.TenantID, s.MessageID, s.AttemptID).Scan(&role); err != nil || role != "student" {
		return e, verification.DecisionReady{}, false, verification.ErrDialogueEvaluation
	}
	var existing verification.DialogueEvaluation
	var usage []byte
	err = tx.QueryRow(ctx, `INSERT INTO verification_evaluations(id,tenant_id,attempt_id,question_id,message_id,accepted,rationale,provider,model,policy_version,usage) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(tenant_id,attempt_id,question_id,message_id) DO NOTHING RETURNING id,accepted,rationale,provider,model,policy_version,usage,created_at`, e.ID, s.TenantID, s.AttemptID, q.ID, s.MessageID, e.Accepted, e.Rationale, e.Provider, e.Model, e.PolicyVersion, jsonBytes(e.Usage)).Scan(&existing.ID, &existing.Accepted, &existing.Rationale, &existing.Provider, &existing.Model, &existing.PolicyVersion, &usage, &existing.CreatedAt)
	inserted := err == nil
	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx, `SELECT id,accepted,rationale,provider,model,policy_version,usage,created_at FROM verification_evaluations WHERE tenant_id=$1 AND attempt_id=$2 AND question_id=$3 AND message_id=$4`, s.TenantID, s.AttemptID, q.ID, s.MessageID).Scan(&existing.ID, &existing.Accepted, &existing.Rationale, &existing.Provider, &existing.Model, &existing.PolicyVersion, &usage, &existing.CreatedAt)
		if err != nil {
			return e, verification.DecisionReady{}, false, err
		}
		_ = json.Unmarshal(usage, &existing.Usage)
		ready := verification.DecisionReady{Accepted: acceptedCount >= state.Config.RequiredQuestions, AcceptedCount: acceptedCount, RequiredCount: state.Config.RequiredQuestions}
		return existing, ready, false, tx.Commit(ctx)
	}
	if err != nil {
		return e, verification.DecisionReady{}, false, err
	}
	_ = json.Unmarshal(usage, &existing.Usage)
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(DISTINCT question_id) FROM verification_evaluations WHERE tenant_id=$1 AND attempt_id=$2 AND accepted`, s.TenantID, s.AttemptID).Scan(&count); err != nil {
		return e, verification.DecisionReady{}, false, err
	}
	ready := verification.DecisionReady{Accepted: count >= state.Config.RequiredQuestions, AcceptedCount: count, RequiredCount: state.Config.RequiredQuestions}
	if ready.Accepted {
		ready.Reason = "required distinct dialogue questions accepted"
		_, err = tx.Exec(ctx, `UPDATE dialogue_attempts SET accepted_count=$3,updated_at=now() WHERE tenant_id=$1 AND attempt_id=$2`, s.TenantID, s.AttemptID, count)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($3,$1,$2,true,$4,'verification_engine') ON CONFLICT(tenant_id,attempt_id) DO NOTHING`, s.TenantID, s.AttemptID, uuid.NewString(), ready.Reason)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE verification_attempts SET status='accepted' WHERE tenant_id=$1 AND id=$2 AND status='open'`, s.TenantID, s.AttemptID)
		}
	} else {
		_, err = tx.Exec(ctx, `UPDATE dialogue_attempts SET accepted_count=$3,updated_at=now() WHERE tenant_id=$1 AND attempt_id=$2`, s.TenantID, s.AttemptID, count)
	}
	if err != nil {
		return e, verification.DecisionReady{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return e, verification.DecisionReady{}, false, err
	}
	return existing, ready, inserted, nil
}

func jsonBytes(v map[string]any) []byte { b, _ := json.Marshal(v); return b }
func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint"))
}
