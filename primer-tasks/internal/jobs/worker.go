// Package jobs provides the PostgreSQL-leased run worker. A websocket request
// only enqueues work; the worker context owns execution and survives socket
// disconnects.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"primer-tasks/internal/agent"
)

type Status string

const (
	Queued  Status = "queued"
	Running Status = "running"
	Done    Status = "done"
	Failed  Status = "failed"
)

type Job struct {
	ID, TenantID, RunID string
	Status              Status
	Attempts            int
	MaxAttempts         int
	AvailableAt         time.Time
	LeaseOwner          string
	LeaseUntil          *time.Time
}

type Repository interface {
	Enqueue(context.Context, Job) error
	Claim(context.Context, string, time.Duration, int) (Job, bool, error)
	Renew(context.Context, string, string, time.Duration) error
	Complete(context.Context, string, string) error
	Fail(context.Context, string, string, error) error
	RequeueExpired(context.Context, time.Time) error
}

type PostgresRepository struct{ DB *pgxpool.Pool }

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository { return &PostgresRepository{DB: db} }
func (r *PostgresRepository) Enqueue(ctx context.Context, j Job) error {
	if j.MaxAttempts < 1 || j.MaxAttempts > 10 {
		return errors.New("invalid max attempts")
	}
	_, err := r.DB.Exec(ctx, `INSERT INTO agent_jobs(id,tenant_id,run_id,kind,status,attempts,max_attempts,available_at) VALUES($1,$2,$3,'agent_run','queued',0,$4,COALESCE($5,now()))`, j.ID, j.TenantID, j.RunID, j.MaxAttempts, j.AvailableAt)
	return err
}
func (r *PostgresRepository) Claim(ctx context.Context, owner string, lease time.Duration, maxTenants int) (j Job, ok bool, err error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback(ctx)
	// Serialize the count-and-claim decision globally. Without this admission
	// lock two workers can both observe a free tenant slot and exceed the cap.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('primer.tasks.agent_jobs.tenant_admission',0))`); err != nil {
		return Job{}, false, err
	}
	err = tx.QueryRow(ctx, `WITH candidate AS (SELECT j.id FROM agent_jobs j WHERE j.status='queued' AND j.available_at<=now() AND (j.lease_until IS NULL OR j.lease_until<now()) AND j.attempts<j.max_attempts AND ($3<=0 OR EXISTS (SELECT 1 FROM agent_jobs active WHERE active.status='running' AND active.tenant_id=j.tenant_id) OR (SELECT count(DISTINCT tenant_id) FROM agent_jobs active WHERE active.status='running')<$3) ORDER BY j.available_at,j.id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE agent_jobs a SET status='running',lease_owner=$1,lease_until=now()+$2::interval,attempts=attempts+1,updated_at=now() FROM candidate WHERE a.id=candidate.id RETURNING a.id,a.tenant_id,a.run_id,a.status,a.attempts,a.max_attempts,a.available_at,a.lease_owner,a.lease_until`, owner, fmt.Sprintf("%f seconds", lease.Seconds()), maxTenants).Scan(&j.ID, &j.TenantID, &j.RunID, &j.Status, &j.Attempts, &j.MaxAttempts, &j.AvailableAt, &j.LeaseOwner, &j.LeaseUntil)
	if err == pgx.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}
func (r *PostgresRepository) Renew(ctx context.Context, id, owner string, lease time.Duration) error {
	var renewed bool
	if err := r.DB.QueryRow(ctx, `UPDATE agent_jobs SET lease_until=now()+$3::interval,updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2 RETURNING true`, id, owner, fmt.Sprintf("%f seconds", lease.Seconds())).Scan(&renewed); err != nil {
		return err
	}
	if !renewed {
		return errors.New("job lease is no longer owned")
	}
	return nil
}
func (r *PostgresRepository) Complete(ctx context.Context, id, owner string) error {
	_, err := r.DB.Exec(ctx, `UPDATE agent_jobs SET status='done',lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2`, id, owner)
	return err
}
func (r *PostgresRepository) Fail(ctx context.Context, id, owner string, cause error) error {
	// Provider/tool errors can contain prompt fragments or credentials. Store a
	// stable operational code, never the error string, in the durable job row.
	code := "job_failed"
	if cause != nil && errors.Is(cause, context.Canceled) {
		code = "canceled"
	}
	_, err := r.DB.Exec(ctx, `UPDATE agent_jobs SET status=CASE WHEN attempts>=max_attempts THEN 'failed' ELSE 'queued' END,available_at=now()+interval '1 second',lease_owner=NULL,lease_until=NULL,last_error=$3,updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2`, id, owner, code)
	return err
}
func (r *PostgresRepository) RequeueExpired(ctx context.Context, now time.Time) error {
	_, err := r.DB.Exec(ctx, `UPDATE agent_jobs SET status=CASE WHEN attempts>=max_attempts THEN 'failed' ELSE 'queued' END,lease_owner=NULL,lease_until=NULL,updated_at=$1 WHERE status='running' AND lease_until<$1`, now)
	return err
}

type Handler func(context.Context, Job) error
type Worker struct {
	Jobs                 Repository
	Runs                 agent.Repository
	Owner                string
	Lease                time.Duration
	Poll                 time.Duration
	MaxConcurrentTenants int
	Handle               Handler
}

func NewWorker(jobs Repository, runs agent.Repository, handle Handler) *Worker {
	return &Worker{Jobs: jobs, Runs: runs, Owner: uuid.NewString(), Lease: 30 * time.Second, Poll: 250 * time.Millisecond, MaxConcurrentTenants: 4, Handle: handle}
}
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Poll)
	defer ticker.Stop()
	_ = w.Reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Reconciliation is periodic, not only startup-time: a process
			// restart that happens before the previous lease expires must still
			// make the job claimable once that lease expires.
			_ = w.Reconcile(ctx)
			w.step(ctx)
		}
	}
}
func (w *Worker) Reconcile(ctx context.Context) error {
	now := time.Now()
	if err := w.Jobs.RequeueExpired(ctx, now); err != nil {
		return err
	}
	if w.Runs != nil {
		return w.Runs.ReconcileExpiredLeases(ctx, now)
	}
	return nil
}
func (w *Worker) step(ctx context.Context) {
	job, ok, err := w.Jobs.Claim(ctx, w.Owner, w.Lease, w.MaxConcurrentTenants)
	if err != nil || !ok {
		return
	}
	if w.Runs != nil {
		_ = w.Runs.TransitionRun(ctx, job.TenantID, job.RunID, agent.RunRunning, 0, agent.Usage{})
	}
	if w.Handle == nil {
		_ = w.Jobs.Fail(ctx, job.ID, w.Owner, errors.New("no job handler"))
		return
	}
	handleCtx, cancel := context.WithCancel(ctx)
	leaseLost := make(chan struct{})
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(w.Lease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-handleCtx.Done():
				return
			case <-ticker.C:
				if renewErr := w.Jobs.Renew(handleCtx, job.ID, w.Owner, w.Lease); renewErr != nil {
					close(leaseLost)
					cancel()
					return
				}
			}
		}
	}()
	err = w.Handle(handleCtx, job)
	cancel()
	<-renewDone
	select {
	case <-leaseLost:
		// Another worker may own the job now. Do not publish a competing
		// terminal state or report completion from this stale execution.
		return
	default:
	}
	if err == nil {
		_ = w.Jobs.Complete(ctx, job.ID, w.Owner)
		w.RunsTransition(ctx, job, agent.RunSucceeded)
	} else {
		_ = w.Jobs.Fail(ctx, job.ID, w.Owner, err)
		w.RunsTransition(ctx, job, agent.RunFailed)
	}
}
func (w *Worker) RunsTransition(ctx context.Context, j Job, status agent.RunStatus) {
	if w.Runs != nil {
		_ = w.Runs.TransitionRun(ctx, j.TenantID, j.RunID, status, 0, agent.Usage{})
	}
}
