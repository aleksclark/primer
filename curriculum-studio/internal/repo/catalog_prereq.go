package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// CatalogPrereqRepo persists curriculum_studio.catalog_standard_prerequisites.
// Acyclicity is enforced by the DB trigger in 00004, not only app validation.
type CatalogPrereqRepo struct {
	Q Querier
}

// NewCatalogPrereqRepo binds to q.
func NewCatalogPrereqRepo(q Querier) *CatalogPrereqRepo {
	return &CatalogPrereqRepo{Q: q}
}

// Create inserts a directed prerequisite edge. Cycles map to ErrPrerequisiteCycle.
func (r *CatalogPrereqRepo) Create(ctx context.Context, in domain.CatalogPrerequisite) (*domain.CatalogPrerequisite, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in.StandardID == uuid.Nil || in.PrerequisiteID == uuid.Nil {
		return nil, fmt.Errorf("standard_id and prerequisite_id are required")
	}
	if in.StandardID == in.PrerequisiteID {
		return nil, fmt.Errorf("%w: self edge", ErrCheckViolation)
	}
	// Serialize concurrent reverse-edge inserts by locking both endpoints
	// in a stable order. The 00004 trigger is the cycle authority; this
	// lock prevents two in-flight opposite edges from both passing the
	// walk against an empty snapshot.
	lo, hi := in.StandardID, in.PrerequisiteID
	if lo.String() > hi.String() {
		lo, hi = hi, lo
	}
	if _, err := r.Q.Exec(ctx, `
SELECT id FROM curriculum_studio.catalog_standards
WHERE id IN ($1, $2)
ORDER BY id
FOR UPDATE`, lo, hi); err != nil {
		return nil, MapError(err)
	}
	const q = `
INSERT INTO curriculum_studio.catalog_standard_prerequisites (standard_id, prerequisite_id)
VALUES ($1, $2)
RETURNING standard_id, prerequisite_id`
	var out domain.CatalogPrerequisite
	if err := r.Q.QueryRow(ctx, q, in.StandardID, in.PrerequisiteID).Scan(&out.StandardID, &out.PrerequisiteID); err != nil {
		return nil, MapError(err)
	}
	return &out, nil
}

// ListForStandard returns prerequisite edges whose standard_id is id.
func (r *CatalogPrereqRepo) ListForStandard(ctx context.Context, standardID uuid.UUID) ([]domain.CatalogPrerequisite, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if standardID == uuid.Nil {
		return []domain.CatalogPrerequisite{}, nil
	}
	const q = `
SELECT standard_id, prerequisite_id
FROM curriculum_studio.catalog_standard_prerequisites
WHERE standard_id = $1
ORDER BY prerequisite_id`
	rows, err := r.Q.Query(ctx, q, standardID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.CatalogPrerequisite
	for rows.Next() {
		var e domain.CatalogPrerequisite
		if err := rows.Scan(&e.StandardID, &e.PrerequisiteID); err != nil {
			return nil, MapError(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.CatalogPrerequisite{}
	}
	return out, nil
}
