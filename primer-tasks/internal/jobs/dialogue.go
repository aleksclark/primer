package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DialogueJob struct {
	ID, TenantID, AttemptID, MessageID string
	Status                             string
	Attempts                           int
	MaxAttempts                        int
	AvailableAt                        time.Time
	LeaseOwner                         string
	LeaseUntil                         *time.Time
}

type DialogueEvent struct {
	TenantID, AttemptID string
	Sequence            int64
	Kind                string
	Payload             []byte
	CreatedAt           time.Time
}

// EnqueueDialogue is separate from the Phase 3 parent-agent queue. A message
// has at most one dialogue verification job, so provider retries cannot create
// another evaluation path.
func (r *PostgresRepository) EnqueueDialogue(ctx context.Context, j DialogueJob) error {
	if r == nil || r.DB == nil || j.TenantID == "" || j.AttemptID == "" || j.MessageID == "" || j.MaxAttempts < 1 || j.MaxAttempts > 10 {
		return errors.New("invalid dialogue job")
	}
	if j.ID == "" {
		j.ID = uuid.NewString()
	}
	_, err := r.DB.Exec(ctx, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,message_id,status,attempts,max_attempts,available_at) VALUES($1,$2,$3,$4,'queued',0,$5,COALESCE($6,now())) ON CONFLICT(tenant_id,message_id) DO NOTHING`, j.ID, j.TenantID, j.AttemptID, j.MessageID, j.MaxAttempts, j.AvailableAt)
	return err
}

func (r *PostgresRepository) ClaimDialogue(ctx context.Context, owner string, lease time.Duration) (j DialogueJob, ok bool, err error) {
	if r == nil || r.DB == nil || owner == "" || lease <= 0 {
		return j, false, errors.New("invalid dialogue lease")
	}
	err = r.DB.QueryRow(ctx, `WITH candidate AS (SELECT id FROM verification_jobs WHERE status='queued' AND available_at<=now() AND (lease_until IS NULL OR lease_until<now()) AND attempts<max_attempts ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE verification_jobs v SET status='running',lease_owner=$1,lease_until=now()+$2::interval,attempts=attempts+1,updated_at=now() FROM candidate WHERE v.id=candidate.id RETURNING v.id,v.tenant_id,v.attempt_id,v.message_id,v.status,v.attempts,v.max_attempts,v.available_at,COALESCE(v.lease_owner,''),v.lease_until`, owner, fmt.Sprintf("%f seconds", lease.Seconds())).Scan(&j.ID, &j.TenantID, &j.AttemptID, &j.MessageID, &j.Status, &j.Attempts, &j.MaxAttempts, &j.AvailableAt, &j.LeaseOwner, &j.LeaseUntil)
	if err == pgx.ErrNoRows {
		return DialogueJob{}, false, nil
	}
	return j, err == nil, err
}

func (r *PostgresRepository) CompleteDialogue(ctx context.Context, id, owner string) error {
	if r == nil || r.DB == nil || id == "" || owner == "" {
		return errors.New("invalid dialogue completion")
	}
	_, err := r.DB.Exec(ctx, `UPDATE verification_jobs SET status='succeeded',lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2`, id, owner)
	return err
}

func (r *PostgresRepository) FailDialogue(ctx context.Context, id, owner string, cause error) error {
	if r == nil || r.DB == nil || id == "" || owner == "" {
		return errors.New("invalid dialogue failure")
	}
	code := "dialogue_job_failed"
	if errors.Is(cause, context.Canceled) {
		code = "canceled"
	}
	_, err := r.DB.Exec(ctx, `UPDATE verification_jobs SET status=CASE WHEN attempts>=max_attempts THEN 'failed' ELSE 'queued' END,available_at=now()+interval '1 second',lease_owner=NULL,lease_until=NULL,last_error=$3,updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2`, id, owner, code)
	return err
}

func (r *PostgresRepository) RequeueExpiredDialogue(ctx context.Context, now time.Time) error {
	if r == nil || r.DB == nil || now.IsZero() {
		return errors.New("invalid dialogue requeue")
	}
	_, err := r.DB.Exec(ctx, `UPDATE verification_jobs SET status=CASE WHEN attempts>=max_attempts THEN 'failed' ELSE 'queued' END,lease_owner=NULL,lease_until=NULL,updated_at=$1 WHERE status='running' AND lease_until<$1`, now)
	return err
}

func (r *PostgresRepository) AppendDialogueEvent(ctx context.Context, e DialogueEvent) error {
	if e.Sequence < 1 || e.TenantID == "" || e.AttemptID == "" || e.Kind == "" {
		return errors.New("invalid dialogue event")
	}
	_, err := r.DB.Exec(ctx, `INSERT INTO verification_events(tenant_id,attempt_id,sequence,kind,payload,created_at) VALUES($1,$2,$3,$4,$5,COALESCE($6,now())) ON CONFLICT(tenant_id,attempt_id,sequence) DO NOTHING`, e.TenantID, e.AttemptID, e.Sequence, e.Kind, e.Payload, e.CreatedAt)
	return err
}

func (r *PostgresRepository) ReplayDialogueEvents(ctx context.Context, tenant, attempt string, after int64, limit int) ([]DialogueEvent, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("invalid replay limit")
	}
	rows, err := r.DB.Query(ctx, `SELECT tenant_id,attempt_id,sequence,kind,payload,created_at FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2 AND sequence>$3 ORDER BY sequence LIMIT $4`, tenant, attempt, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DialogueEvent
	for rows.Next() {
		var e DialogueEvent
		if err := rows.Scan(&e.TenantID, &e.AttemptID, &e.Sequence, &e.Kind, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
