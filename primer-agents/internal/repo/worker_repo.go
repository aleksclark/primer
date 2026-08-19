package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/agents/internal/domain"
)

// ClaimNextQueued selects one queued run whose lease has expired (or was never
// set) and atomically transitions it to running within the same transaction,
// using FOR UPDATE SKIP LOCKED so concurrent workers never double-claim.
//
// The caller must pass a pgx.Tx (not a pool). On return the row is locked and
// updated inside that transaction; the caller commits to confirm the claim.
// Returns ErrNotFound when the queue is empty or all eligible rows are locked
// by other workers.
func (r *RunRepo) ClaimNextQueued(ctx context.Context, tx pgx.Tx, leaseDuration time.Duration) (*domain.Run, error) {
	const selectSQL = `
		SELECT id, state_version
		FROM agents.runs
		WHERE status = 'queued'
		  AND (lease_expires_at IS NULL OR lease_expires_at < now())
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED`

	var id string
	var ver int64
	if err := tx.QueryRow(ctx, selectSQL).Scan(&id, &ver); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("claim next queued select: %w", err)
	}

	leaseExpiry := time.Now().Add(leaseDuration)
	const updateSQL = `
		UPDATE agents.runs SET
		    status          = 'running',
		    state_version   = state_version + 1,
		    attempt_count   = attempt_count + 1,
		    started_at      = COALESCE(started_at, now()),
		    lease_expires_at = $3
		WHERE id = $1
		  AND state_version = $2
		  AND status = 'queued'
		RETURNING ` + runCols

	rows, err := tx.Query(ctx, updateSQL, id, ver, leaseExpiry)
	if err != nil {
		return nil, fmt.Errorf("claim next queued update: %w", err)
	}
	run, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Run])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrStaleVersion
		}
		return nil, fmt.Errorf("scan claimed run: %w", err)
	}
	return &run, nil
}

// MarkProviderStarted sets provider_started_at = now() for a running run,
// recording that the provider invocation has begun. Once set, process death
// requires terminalising the run as interrupted rather than re-queuing.
func (r *RunRepo) MarkProviderStarted(ctx context.Context, q Querier, runID, namespace string, stateVersion int64) error {
	const sql = `
		UPDATE agents.runs
		SET provider_started_at = now()
		WHERE id = $1 AND owner_namespace = $2 AND state_version = $3
		  AND status = 'running' AND provider_started_at IS NULL`
	tag, err := q.Exec(ctx, sql, runID, namespace, stateVersion)
	if err != nil {
		return fmt.Errorf("mark provider started: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrStaleVersion
	}
	return nil
}

// ExtendLease pushes lease_expires_at forward. Returns ErrNotFound if the run
// is no longer owned by this worker/version.
func (r *RunRepo) ExtendLease(ctx context.Context, q Querier, runID, namespace string, stateVersion int64, leaseDuration time.Duration) error {
	newExpiry := time.Now().Add(leaseDuration)
	const sql = `
		UPDATE agents.runs
		SET lease_expires_at = $4
		WHERE id = $1 AND owner_namespace = $2 AND state_version = $3
		  AND status IN ('running','cancel_requested')`
	tag, err := q.Exec(ctx, sql, runID, namespace, stateVersion, newExpiry)
	if err != nil {
		return fmt.Errorf("extend lease: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReconcileInterrupted terminalises running rows whose provider has started and
// whose lease has expired. Returns updated run IDs.
func (r *RunRepo) ReconcileInterrupted(ctx context.Context, q Querier) ([]string, error) {
	const sql = `
		UPDATE agents.runs
		SET status        = 'interrupted',
		    state_version = state_version + 1,
		    ended_at      = COALESCE(ended_at, now()),
		    result_class  = 'interrupted',
		    error_class   = 'lease_expired'
		WHERE status IN ('running','cancel_requested')
		  AND provider_started_at IS NOT NULL
		  AND lease_expires_at IS NOT NULL
		  AND lease_expires_at < now()
		RETURNING id`
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("reconcile interrupted: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ReconcileNeverStartedInterrupted returns still-running rows whose provider
// was never started and whose lease expired to the queued state for safe retry.
func (r *RunRepo) ReconcileNeverStartedInterrupted(ctx context.Context, q Querier) ([]string, error) {
	const sql = `
		UPDATE agents.runs
		SET status          = 'queued',
		    state_version   = state_version + 1,
		    lease_expires_at = NULL,
		    started_at      = NULL
		WHERE status = 'running'
		  AND provider_started_at IS NULL
		  AND lease_expires_at IS NOT NULL
		  AND lease_expires_at < now()
		RETURNING id`
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("reconcile never-started: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ReconcileStaleCancels terminalises cancel_requested rows with no running
// lease (worker died before acknowledging) or that were never claimed. These
// must become canceled so the run lifecycle is complete.
func (r *RunRepo) ReconcileStaleCancels(ctx context.Context, q Querier) ([]string, error) {
	const sql = `
		UPDATE agents.runs
		SET status        = 'canceled',
		    state_version = state_version + 1,
		    ended_at      = COALESCE(ended_at, now()),
		    result_class  = 'canceled'
		WHERE status = 'cancel_requested'
		  AND provider_started_at IS NULL
		  AND (lease_expires_at IS NULL OR lease_expires_at < now())
		RETURNING id`
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("reconcile stale cancels: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
