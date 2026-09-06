package repo

import (
	"context"
	"fmt"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// lockDraftRevision must run in the transaction containing the eventual write.
// Publication owns the revision row too. Graph-write triggers use the same
// NO KEY UPDATE lock, which serializes content without conflicting with FK
// KEY SHARE locks. Do not acquire a curriculum lock after this lock (publishers
// may already own it). Approval order is revision -> membership -> approval.
func lockDraftRevision(ctx context.Context, q Querier, ws, revision uuid.UUID) (*domain.PlanRevision, error) {
	// WithTx also accepts lightweight Querier wrappers. Never silently degrade
	// these locking writes to autocommit, or claim fresh snapshots at RR/SSI.
	if _, ok := q.(pgx.Tx); !ok {
		return nil, fmt.Errorf("%w: collaboration requires a transaction", ErrConflict)
	}
	var isolation string
	if err := q.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil {
		return nil, MapError(err)
	}
	if isolation != "read committed" {
		return nil, fmt.Errorf("%w: collaboration requires READ COMMITTED", ErrConflict)
	}
	var id uuid.UUID
	if err := q.QueryRow(ctx, `SELECT r.id FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$1 AND c.workspace_id=$2 FOR NO KEY UPDATE OF r`, revision, ws).Scan(&id); err != nil {
		return nil, MapError(err)
	}
	// A distinct statement is essential: READ COMMITTED statement snapshots taken
	// before a lock wait do not refresh arbitrary predicates or SQL functions.
	rev, err := NewPlanRevisionRepo(q).Get(ctx, ws, revision)
	if err != nil {
		return nil, err
	}
	if rev.Status != "draft" {
		return nil, ErrImmutable
	}
	return rev, nil
}
