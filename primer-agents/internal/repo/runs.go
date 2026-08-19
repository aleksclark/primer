package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/agents/internal/domain"
)

const runCols = `
id, session_id, owner_namespace, idempotency_key, idempotency_hash,
profile, input_hash, input_preview, status, state_version, next_event_seq,
cancel_requested_at, cancel_reason_class, attempt_count, lease_expires_at,
provider_started_at, result_class, error_class,
created_at, started_at, ended_at`

// Runs is the canonical run repository instance.
var Runs = &RunRepo{}

// RunRepo provides persistence operations for agents.runs.
type RunRepo struct{}

// CreateRunCmd carries the inputs for creating a new run idempotently.
type CreateRunCmd struct {
	OwnerNamespace string
	IdempotencyKey string
	// IdempotencyHash is SHA-256 hex of (namespace|key|profile|input_hash),
	// computed by the caller before the database round-trip.
	IdempotencyHash string
	Profile         string
	InputHash       *string
	InputPreview    *string
	SessionID       *string
}

// Create inserts a new run or returns an existing one for the same
// (owner_namespace, idempotency_key).
//
//   - Same key + same IdempotencyHash → idempotent: returns the existing run.
//   - Same key + different IdempotencyHash → ErrConflict.
//   - Different namespace + same key → creates a new run (no collision).
func (r *RunRepo) Create(ctx context.Context, q Querier, cmd CreateRunCmd) (*domain.Run, error) {
	const insertSQL = `
		INSERT INTO agents.runs
		    (session_id, owner_namespace, idempotency_key, idempotency_hash,
		     profile, input_hash, input_preview)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (owner_namespace, idempotency_key) DO NOTHING
		RETURNING ` + runCols

	rows, err := q.Query(ctx, insertSQL,
		cmd.SessionID, cmd.OwnerNamespace, cmd.IdempotencyKey, cmd.IdempotencyHash,
		cmd.Profile, cmd.InputHash, cmd.InputPreview)
	if err != nil {
		return nil, fmt.Errorf("insert run: %w", err)
	}
	run, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Run])
	if err == nil {
		return &run, nil // fresh insert
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("scan new run: %w", err)
	}

	// ON CONFLICT hit — fetch existing to check hash.
	existing, err := r.Get(ctx, q, "", cmd.OwnerNamespace, cmd.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing.IdempotencyHash != cmd.IdempotencyHash {
		return nil, ErrConflict
	}
	return existing, nil // idempotent
}

// Get returns a run by id and owner namespace. When id is empty, idempotencyKey
// is used instead (internal idempotency lookup path only).
func (r *RunRepo) Get(ctx context.Context, q Querier, id, namespace, idempotencyKey string) (*domain.Run, error) {
	var (
		sqlStr string
		args   []any
	)
	if id != "" {
		sqlStr = `SELECT ` + runCols + ` FROM agents.runs WHERE id = $1 AND owner_namespace = $2`
		args = []any{id, namespace}
	} else {
		sqlStr = `SELECT ` + runCols + ` FROM agents.runs WHERE owner_namespace = $1 AND idempotency_key = $2`
		args = []any{namespace, idempotencyKey}
	}

	rows, err := q.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	run, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Run])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan run: %w", err)
	}
	return &run, nil
}

// TransitionCmd carries inputs for a compare-and-swap run status update.
type TransitionCmd struct {
	RunID          string
	OwnerNamespace string
	// ExpectedStatus is the status the run must currently have for the CAS to succeed.
	ExpectedStatus domain.RunStatus
	// ExpectedVersion is the state_version the run must currently have.
	ExpectedVersion int64
	NewStatus       domain.RunStatus
	// Terminal fields — only populated when NewStatus is terminal.
	ResultClass *string
	ErrorClass  *string
	// Lease fields — only populated when NewStatus is 'running'.
	LeaseExpiresAt    *time.Time
	ProviderStartedAt *time.Time
}

// Transition applies a CAS status update and returns the updated run.
// Returns ErrNotFound, ErrInvalidTransition, or ErrStaleVersion on failure.
func (r *RunRepo) Transition(ctx context.Context, q Querier, cmd TransitionCmd) (*domain.Run, error) {
	if !domain.CanTransition(cmd.ExpectedStatus, cmd.NewStatus) {
		return nil, ErrInvalidTransition
	}

	var setTimestamp string
	switch {
	case cmd.NewStatus == domain.RunStatusRunning:
		setTimestamp = `, started_at = now(), attempt_count = attempt_count + 1`
	case cmd.NewStatus.IsTerminal():
		setTimestamp = `, ended_at = now()`
	}

	sql := `
		UPDATE agents.runs SET
		    status        = $3,
		    state_version = state_version + 1,
		    result_class  = COALESCE($4, result_class),
		    error_class   = COALESCE($5, error_class),
		    lease_expires_at    = COALESCE($6, lease_expires_at),
		    provider_started_at = COALESCE($7, provider_started_at)` +
		setTimestamp + `
		WHERE id = $1 AND owner_namespace = $2
		  AND status        = $8
		  AND state_version = $9
		RETURNING ` + runCols

	rows, err := q.Query(ctx, sql,
		cmd.RunID, cmd.OwnerNamespace, string(cmd.NewStatus),
		cmd.ResultClass, cmd.ErrorClass,
		cmd.LeaseExpiresAt, cmd.ProviderStartedAt,
		string(cmd.ExpectedStatus), cmd.ExpectedVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("transition run: %w", err)
	}
	run, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Run])
	if err == nil {
		return &run, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("scan transitioned run: %w", err)
	}

	// Diagnose the miss.
	existing, getErr := r.Get(ctx, q, cmd.RunID, cmd.OwnerNamespace, "")
	if getErr != nil {
		return nil, getErr
	}
	if existing.StateVersion != cmd.ExpectedVersion {
		return nil, ErrStaleVersion
	}
	if existing.Status != cmd.ExpectedStatus {
		return nil, ErrInvalidTransition
	}
	return nil, ErrInvalidTransition
}

// RequestCancelCmd carries inputs for requesting cancellation of a run.
type RequestCancelCmd struct {
	RunID          string
	OwnerNamespace string
	ReasonClass    *string
}

// RequestCancel transitions a queued or running run to cancel_requested.
// Idempotent: if already cancel_requested it returns the existing run.
func (r *RunRepo) RequestCancel(ctx context.Context, q Querier, cmd RequestCancelCmd) (*domain.Run, error) {
	const sql = `
		UPDATE agents.runs SET
		    status              = 'cancel_requested',
		    state_version       = state_version + 1,
		    cancel_requested_at = COALESCE(cancel_requested_at, now()),
		    cancel_reason_class = COALESCE($3, cancel_reason_class)
		WHERE id = $1 AND owner_namespace = $2
		  AND status IN ('queued','running','cancel_requested')
		RETURNING ` + runCols

	rows, err := q.Query(ctx, sql, cmd.RunID, cmd.OwnerNamespace, cmd.ReasonClass)
	if err != nil {
		return nil, fmt.Errorf("request cancel: %w", err)
	}
	run, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Run])
	if err == nil {
		return &run, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("scan cancel-requested run: %w", err)
	}

	// Either not found or already terminal.
	existing, getErr := r.Get(ctx, q, cmd.RunID, cmd.OwnerNamespace, "")
	if getErr != nil {
		return nil, getErr
	}
	if existing.Status.IsTerminal() {
		return nil, ErrInvalidTransition
	}
	return nil, ErrNotFound
}

// ListByOwner returns up to limit runs owned by namespace, ordered by
// created_at DESC, paginating from afterCreatedAt/afterID.
func (r *RunRepo) ListByOwner(ctx context.Context, q Querier, namespace string, limit int) ([]*domain.Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const sql = `SELECT ` + runCols + `
		FROM agents.runs WHERE owner_namespace = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2`
	rows, err := q.Query(ctx, sql, namespace, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.Run])
	if err != nil {
		return nil, fmt.Errorf("scan runs: %w", err)
	}
	out := make([]*domain.Run, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out, nil
}
