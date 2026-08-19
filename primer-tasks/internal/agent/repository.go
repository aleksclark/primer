package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the durable boundary used by the runtime and worker. The
// implementation is intentionally SQL-backed; test doubles must implement
// this interface rather than replacing production state with a map.
type Repository interface {
	CreateConversation(context.Context, Conversation) error
	GetConversation(context.Context, string, string) (Conversation, error)
	AppendUserMessage(context.Context, Message) (Message, bool, error)
	AppendMessage(context.Context, Message) error
	CreateRun(context.Context, Run) error
	GetRun(context.Context, string, string) (Run, error)
	TransitionRun(context.Context, string, string, RunStatus, int, Usage) error
	RequestCancel(context.Context, string, string) error
	LeaseRun(context.Context, string, string, time.Duration) (Run, bool, error)
	ReconcileExpiredLeases(context.Context, time.Time) error
	AppendEvent(context.Context, RunEvent) error
	ReplayEvents(context.Context, string, string, int64, int) ([]RunEvent, error)
	PutPreview(context.Context, ConfirmationPreview) error
	ConsumePreview(context.Context, ConfirmationPreview, time.Time) error
}

type RunEvent struct {
	RunID, TenantID string
	Sequence        int64
	EventType       string
	Payload         []byte
	CreatedAt       time.Time
}

// PostgresRepository uses transactions/conditional updates for all ownership
// changes. Schema installation is intentionally left to the Tasks migration
// owner; see schema.sql for the contract this repository expects.
type PostgresRepository struct{ DB *pgxpool.Pool }

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository { return &PostgresRepository{DB: db} }

func (r *PostgresRepository) CreateConversation(ctx context.Context, c Conversation) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO agent_conversations(id,tenant_id,actor_id,status,policy_version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,COALESCE($6,now()),COALESCE($6,now()))`, c.ID, c.TenantID, c.ActorID, c.Status, c.PolicyVersion, c.CreatedAt)
	return err
}
func (r *PostgresRepository) GetConversation(ctx context.Context, tenant, id string) (c Conversation, err error) {
	err = r.DB.QueryRow(ctx, `SELECT id,tenant_id,actor_id,status,policy_version,created_at,updated_at FROM agent_conversations WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&c.ID, &c.TenantID, &c.ActorID, &c.Status, &c.PolicyVersion, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		err = ErrNotFound
	}
	return
}
func (r *PostgresRepository) AppendUserMessage(ctx context.Context, m Message) (Message, bool, error) {
	var inserted bool
	err := r.DB.QueryRow(ctx, `INSERT INTO agent_messages(id,tenant_id,conversation_id,client_message_id,role,content,sequence,created_at) VALUES($1,$2,$3,$4,'user',$5,$6,COALESCE($7,now())) ON CONFLICT(tenant_id,conversation_id,client_message_id) DO NOTHING RETURNING true`, m.ID, m.TenantID, m.ConversationID, m.ClientMessageID, m.Content, m.Sequence, m.CreatedAt).Scan(&inserted)
	if err == pgx.ErrNoRows {
		err = r.DB.QueryRow(ctx, `SELECT id,tenant_id,conversation_id,client_message_id,role,content,sequence,created_at FROM agent_messages WHERE tenant_id=$1 AND conversation_id=$2 AND client_message_id=$3`, m.TenantID, m.ConversationID, m.ClientMessageID).Scan(&m.ID, &m.TenantID, &m.ConversationID, &m.ClientMessageID, &m.Role, &m.Content, &m.Sequence, &m.CreatedAt)
		return m, false, err
	}
	return m, inserted, err
}
func (r *PostgresRepository) AppendMessage(ctx context.Context, m Message) error {
	if m.Role == RoleUser {
		return fmt.Errorf("use AppendUserMessage for user messages")
	}
	_, err := r.DB.Exec(ctx, `INSERT INTO agent_messages(id,tenant_id,conversation_id,client_message_id,role,content,sequence,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,COALESCE($8,now()))`, m.ID, m.TenantID, m.ConversationID, m.ClientMessageID, m.Role, m.Content, m.Sequence, m.CreatedAt)
	return err
}
func (r *PostgresRepository) CreateRun(ctx context.Context, run Run) error {
	if err := ValidateRunLimits(run); err != nil {
		return err
	}
	_, err := r.DB.Exec(ctx, `INSERT INTO agent_runs(id,tenant_id,conversation_id,user_message_id,status,durable_step,attempt,max_steps,max_tokens,deadline,lease_owner,lease_until,cancel_requested,input_tokens,output_tokens,total_tokens,reasoning_tokens,provider,model,policy_version,prompt_digest,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULL,NULL,false,$11,$12,$13,$14,$15,$16,$17,$18,COALESCE($19,now()),COALESCE($19,now()))`, run.ID, run.TenantID, run.ConversationID, run.UserMessageID, run.Status, run.DurableStep, run.Attempt, run.MaxSteps, run.MaxTokens, run.Deadline, run.Usage.InputTokens, run.Usage.OutputTokens, run.Usage.TotalTokens, run.Usage.ReasoningTokens, run.Provenance.Provider, run.Provenance.Model, run.Provenance.PolicyVersion, run.Provenance.PromptDigest, run.CreatedAt)
	return err
}
func (r *PostgresRepository) GetRun(ctx context.Context, tenant, id string) (run Run, err error) {
	err = r.DB.QueryRow(ctx, `SELECT id,tenant_id,conversation_id,user_message_id,status,durable_step,attempt,max_steps,max_tokens,deadline,COALESCE(lease_owner,''),lease_until,cancel_requested,input_tokens,output_tokens,total_tokens,reasoning_tokens,provider,model,policy_version,prompt_digest,created_at,updated_at FROM agent_runs WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&run.ID, &run.TenantID, &run.ConversationID, &run.UserMessageID, &run.Status, &run.DurableStep, &run.Attempt, &run.MaxSteps, &run.MaxTokens, &run.Deadline, &run.LeaseOwner, &run.LeaseUntil, &run.CancelRequested, &run.Usage.InputTokens, &run.Usage.OutputTokens, &run.Usage.TotalTokens, &run.Usage.ReasoningTokens, &run.Provenance.Provider, &run.Provenance.Model, &run.Provenance.PolicyVersion, &run.Provenance.PromptDigest, &run.CreatedAt, &run.UpdatedAt)
	if err == pgx.ErrNoRows {
		err = ErrNotFound
	}
	return
}
func (r *PostgresRepository) TransitionRun(ctx context.Context, tenant, id string, to RunStatus, step int, u Usage) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var from RunStatus
	err = tx.QueryRow(ctx, `SELECT status FROM agent_runs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, id).Scan(&from)
	if err == pgx.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if from.Terminal() {
		return ErrAlreadyTerminal
	}
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: %s to %s", ErrInvalidTransition, from, to)
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_runs SET status=$3,durable_step=$4,input_tokens=$5,output_tokens=$6,total_tokens=$7,reasoning_tokens=$8,updated_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, id, to, step, u.InputTokens, u.OutputTokens, u.TotalTokens, u.ReasoningTokens); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r *PostgresRepository) RequestCancel(ctx context.Context, tenant, id string) error {
	_, err := r.DB.Exec(ctx, `UPDATE agent_runs SET cancel_requested=true,status=CASE WHEN status='queued' THEN 'cancel_requested' ELSE status END,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status IN ('queued','running')`, tenant, id)
	return err
}
func (r *PostgresRepository) LeaseRun(ctx context.Context, tenant, owner string, d time.Duration) (run Run, ok bool, err error) {
	err = r.DB.QueryRow(ctx, `WITH candidate AS (SELECT id FROM agent_runs WHERE tenant_id=$1 AND status IN ('queued','running','cancel_requested') AND (lease_until IS NULL OR lease_until<now()) AND deadline>now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE agent_runs a SET lease_owner=$2,lease_until=now()+$3::interval,attempt=attempt+1,updated_at=now() FROM candidate WHERE a.id=candidate.id RETURNING a.id,a.tenant_id,a.conversation_id,a.user_message_id,a.status,a.durable_step,a.attempt,a.max_steps,a.max_tokens,a.deadline,COALESCE(a.lease_owner,''),a.lease_until,a.cancel_requested,a.input_tokens,a.output_tokens,a.total_tokens,a.reasoning_tokens,a.provider,a.model,a.policy_version,a.prompt_digest,a.created_at,a.updated_at`, tenant, owner, fmt.Sprintf("%f seconds", d.Seconds())).Scan(&run.ID, &run.TenantID, &run.ConversationID, &run.UserMessageID, &run.Status, &run.DurableStep, &run.Attempt, &run.MaxSteps, &run.MaxTokens, &run.Deadline, &run.LeaseOwner, &run.LeaseUntil, &run.CancelRequested, &run.Usage.InputTokens, &run.Usage.OutputTokens, &run.Usage.TotalTokens, &run.Usage.ReasoningTokens, &run.Provenance.Provider, &run.Provenance.Model, &run.Provenance.PolicyVersion, &run.Provenance.PromptDigest, &run.CreatedAt, &run.UpdatedAt)
	if err == pgx.ErrNoRows {
		return Run{}, false, nil
	}
	return run, err == nil, err
}
func (r *PostgresRepository) ReconcileExpiredLeases(ctx context.Context, now time.Time) error {
	_, err := r.DB.Exec(ctx, `UPDATE agent_runs SET lease_owner=NULL,lease_until=NULL,status=CASE WHEN deadline<= $1 THEN 'failed' ELSE status END,updated_at=$1 WHERE lease_until IS NOT NULL AND lease_until<$1 AND status IN ('running','queued','cancel_requested')`, now)
	return err
}
func (r *PostgresRepository) AppendEvent(ctx context.Context, e RunEvent) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO agent_run_events(run_id,tenant_id,sequence,event_type,payload,created_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(run_id,sequence) DO NOTHING`, e.RunID, e.TenantID, e.Sequence, e.EventType, e.Payload, e.CreatedAt)
	return err
}
func (r *PostgresRepository) ReplayEvents(ctx context.Context, tenant, run string, after int64, limit int) (out []RunEvent, err error) {
	rows, err := r.DB.Query(ctx, `SELECT run_id,tenant_id,sequence,event_type,payload,created_at FROM agent_run_events WHERE tenant_id=$1 AND run_id=$2 AND sequence>$3 ORDER BY sequence LIMIT $4`, tenant, run, after, limit)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var e RunEvent
		if err = rows.Scan(&e.RunID, &e.TenantID, &e.Sequence, &e.EventType, &e.Payload, &e.CreatedAt); err != nil {
			return
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) PutPreview(ctx context.Context, p ConfirmationPreview) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO agent_confirmation_previews(id,tenant_id,actor_id,action,action_digest,payload,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, p.ID, p.TenantID, p.ActorID, p.Action, p.ActionDigest, p.Payload, p.ExpiresAt)
	return err
}
func (r *PostgresRepository) ConsumePreview(ctx context.Context, p ConfirmationPreview, now time.Time) error {
	var used time.Time
	err := r.DB.QueryRow(ctx, `UPDATE agent_confirmation_previews SET used_at=$6 WHERE id=$1 AND tenant_id=$2 AND actor_id=$3 AND action=$4 AND action_digest=$5 AND used_at IS NULL AND expires_at>$6 RETURNING used_at`, p.ID, p.TenantID, p.ActorID, p.Action, p.ActionDigest, now).Scan(&used)
	if err == pgx.ErrNoRows {
		return ErrPreviewExpired
	}
	return err
}

func NewID() string     { return uuid.NewString() }
func JSON(v any) []byte { b, _ := json.Marshal(v); return b }
