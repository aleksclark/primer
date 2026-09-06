package jobs

// Original P4 dialogue queue, adapted to generation-fenced renewable leases.
// This queue never records accepted evaluations or occurrence completion.
import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const DialogueLeaseDuration = 10 * time.Second

var ErrDialogueLeaseLost = errors.New("dialogue lease lost")

type DialogueJob struct {
	ID, TenantID, StudentID, SessionID, OccurrenceID, AttemptID, MessageID string
	Stage, LeaseOwner                                                      string
	LeaseGeneration                                                        int64
	Attempts, MaxAttempts                                                  int
	Deadline                                                               time.Time
}

func (r *PostgresRepository) ClaimDialogue(ctx context.Context, owner string) (j DialogueJob, ok bool, err error) {
	if r == nil || r.DB == nil || owner == "" {
		return j, false, ErrDialogueLeaseLost
	}
	// Recovery is a production queue action, not a test-only requeue call. A
	// takeover spends another finite attempt and preserves the durable stage.
	_, err = r.DB.Exec(ctx, `UPDATE verification_jobs SET status=CASE WHEN attempts>=max_attempts OR deadline<=clock_timestamp() THEN 'failed' ELSE 'queued' END,lease_owner=NULL,lease_until=NULL,last_error='lease_lost',updated_at=clock_timestamp() WHERE status='running' AND lease_until<=clock_timestamp()`)
	if err != nil {
		return j, false, err
	}
	_, err = r.DB.Exec(ctx, `UPDATE verification_jobs SET status='failed',last_error='exhausted',updated_at=clock_timestamp() WHERE status='queued' AND (attempts>=max_attempts OR deadline<=clock_timestamp())`)
	if err != nil {
		return j, false, err
	}
	err = r.DB.QueryRow(ctx, `WITH candidate AS (SELECT id FROM verification_jobs WHERE status='queued' AND available_at<=clock_timestamp() AND deadline>clock_timestamp() AND attempts<max_attempts ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1), claimed AS (UPDATE verification_jobs j SET status='running',lease_owner=$1,lease_generation=lease_generation+1,lease_until=clock_timestamp()+interval '10 seconds',attempts=attempts+1,updated_at=clock_timestamp() FROM candidate c WHERE j.id=c.id RETURNING j.*) SELECT j.id,j.tenant_id,d.student_id,j.session_id,d.occurrence_id,j.attempt_id,COALESCE(j.message_id::text,''),j.stage,j.lease_owner,j.lease_generation,j.attempts,j.max_attempts,j.deadline FROM claimed j JOIN dialogue_attempts d ON d.tenant_id=j.tenant_id AND d.attempt_id=j.attempt_id`, owner).Scan(&j.ID, &j.TenantID, &j.StudentID, &j.SessionID, &j.OccurrenceID, &j.AttemptID, &j.MessageID, &j.Stage, &j.LeaseOwner, &j.LeaseGeneration, &j.Attempts, &j.MaxAttempts, &j.Deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return DialogueJob{}, false, nil
	}
	return j, err == nil, err
}
func (r *PostgresRepository) lockDialogueJob(ctx context.Context, j DialogueJob) (pgx.Tx, error) {
	if r == nil || r.DB == nil || j.ID == "" || j.TenantID == "" || j.LeaseOwner == "" || j.LeaseGeneration < 1 {
		return nil, ErrDialogueLeaseLost
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var id string
	if err = tx.QueryRow(ctx, `SELECT id FROM verification_jobs WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, j.ID, j.TenantID).Scan(&id); err != nil {
		_ = tx.Rollback(ctx)
		return nil, ErrDialogueLeaseLost
	}
	return tx, nil
}
func (r *PostgresRepository) RenewDialogue(ctx context.Context, j DialogueJob) error {
	tx, err := r.lockDialogueJob(ctx, j)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Clock predicates run AFTER the row lock, not on a pre-wait snapshot.
	tag, err := tx.Exec(ctx, `UPDATE verification_jobs SET lease_until=clock_timestamp()+interval '10 seconds',updated_at=clock_timestamp() WHERE id=$1 AND tenant_id=$2 AND status='running' AND lease_owner=$3 AND lease_generation=$4 AND lease_until>clock_timestamp() AND deadline>clock_timestamp()`, j.ID, j.TenantID, j.LeaseOwner, j.LeaseGeneration)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDialogueLeaseLost
	}
	return tx.Commit(ctx)
}
func (r *PostgresRepository) FailDialogue(ctx context.Context, j DialogueJob, code string) error {
	if code != "provider_unavailable" && code != "lease_lost" && code != "revoked" && code != "exhausted" && code != "invalid_evaluation" {
		return errors.New("invalid dialogue failure code")
	}
	tx, err := r.lockDialogueJob(ctx, j)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// No automatic reset of attempts. Explicit retry may requeue the same saved
	// answer only while the original finite budget and deadline permit it.
	tag, err := tx.Exec(ctx, `UPDATE verification_jobs SET status='failed',lease_owner=NULL,lease_until=NULL,last_error=$5,updated_at=clock_timestamp() WHERE id=$1 AND tenant_id=$2 AND status='running' AND lease_owner=$3 AND lease_generation=$4 AND lease_until>clock_timestamp()`, j.ID, j.TenantID, j.LeaseOwner, j.LeaseGeneration, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDialogueLeaseLost
	}
	return tx.Commit(ctx)
}
