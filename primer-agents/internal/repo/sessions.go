package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/agents/internal/domain"
)

const sessionCols = `
id, owner_namespace, caller_context, profile, status, revision,
state_ref, created_at, updated_at, expires_at`

// CreateSessionCmd carries the inputs for creating a new session.
type CreateSessionCmd struct {
	OwnerNamespace string
	Profile        string
	CallerContext  *string
	ExpiresAt      *time.Time
}

// Sessions is the canonical sessions repository instance.
var Sessions = &SessionRepo{}

// SessionRepo provides persistence operations for agents.sessions.
type SessionRepo struct{}

// Create inserts a new session and returns it.
func (r *SessionRepo) Create(ctx context.Context, q Querier, cmd CreateSessionCmd) (*domain.Session, error) {
	const sql = `
		INSERT INTO agents.sessions
		    (owner_namespace, profile, caller_context, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + sessionCols

	rows, err := q.Query(ctx, sql,
		cmd.OwnerNamespace, cmd.Profile, cmd.CallerContext, cmd.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	s, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Session])
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	return &s, nil
}

// Get returns the session with the given id owned by namespace. Returns
// ErrNotFound when absent or owned by a different namespace.
func (r *SessionRepo) Get(ctx context.Context, q Querier, id, namespace string) (*domain.Session, error) {
	const sql = `SELECT ` + sessionCols + `
		FROM agents.sessions WHERE id = $1 AND owner_namespace = $2`

	rows, err := q.Query(ctx, sql, id, namespace)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	s, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Session])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan session: %w", err)
	}
	return &s, nil
}

// Close transitions a session from open to closed using an optimistic revision
// check. Returns ErrNotFound, ErrStaleVersion, or ErrInvalidTransition.
func (r *SessionRepo) Close(ctx context.Context, q Querier, id, namespace string, revision int64) (*domain.Session, error) {
	const sql = `
		UPDATE agents.sessions
		SET status = 'closed', revision = revision + 1, updated_at = now()
		WHERE id = $1 AND owner_namespace = $2 AND status = 'open' AND revision = $3
		RETURNING ` + sessionCols

	rows, err := q.Query(ctx, sql, id, namespace, revision)
	if err != nil {
		return nil, fmt.Errorf("close session: %w", err)
	}
	s, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Session])
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("scan closed session: %w", err)
		}
		// Diagnose: not found, stale, or already closed.
		existing, getErr := r.Get(ctx, q, id, namespace)
		if getErr != nil {
			return nil, getErr // ErrNotFound or other
		}
		if existing.Revision != revision {
			return nil, ErrStaleVersion
		}
		return nil, ErrInvalidTransition // already closed
	}
	return &s, nil
}
