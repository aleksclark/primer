package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// CatalogPrereqRepo persists curriculum_studio.catalog_standard_prerequisites.
// Acyclicity is enforced by the serialized DB trigger in 00005, not only app
// validation, so raw concurrent SQL is covered too.
type CatalogPrereqRepo struct {
	Q Querier
}

// NewCatalogPrereqRepo binds to q.
func NewCatalogPrereqRepo(q Querier) *CatalogPrereqRepo {
	return &CatalogPrereqRepo{Q: q}
}

// Create inserts a directed prerequisite edge visible to workspaceID.
func (r *CatalogPrereqRepo) Create(ctx context.Context, workspaceID uuid.UUID, in domain.CatalogPrerequisite) (*domain.CatalogPrerequisite, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if in.StandardID == uuid.Nil || in.PrerequisiteID == uuid.Nil {
		return nil, fmt.Errorf("standard_id and prerequisite_id are required")
	}
	if in.StandardID == in.PrerequisiteID {
		return nil, fmt.Errorf("%w: self edge", ErrCheckViolation)
	}
	const q = `
INSERT INTO curriculum_studio.catalog_standard_prerequisites (standard_id, prerequisite_id)
SELECT s.id, p.id
FROM curriculum_studio.catalog_standards s
JOIN curriculum_studio.standard_frameworks sf ON sf.id = s.framework_id
JOIN curriculum_studio.catalog_standards p ON p.id = $2
JOIN curriculum_studio.standard_frameworks pf ON pf.id = p.framework_id
WHERE s.id = $1
  AND (sf.workspace_id IS NULL OR sf.workspace_id = $3)
  AND (pf.workspace_id IS NULL OR pf.workspace_id = $3)
RETURNING standard_id, prerequisite_id`
	var out domain.CatalogPrerequisite
	if err := r.Q.QueryRow(ctx, q, in.StandardID, in.PrerequisiteID, workspaceID).Scan(&out.StandardID, &out.PrerequisiteID); err != nil {
		return nil, MapError(err)
	}
	return &out, nil
}

// ListForStandard returns prerequisite edges for a standard visible to
// workspaceID. A foreign standard is indistinguishable from a missing one.
func (r *CatalogPrereqRepo) ListForStandard(ctx context.Context, workspaceID, standardID uuid.UUID) ([]domain.CatalogPrerequisite, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || standardID == uuid.Nil {
		return []domain.CatalogPrerequisite{}, nil
	}
	const q = `
SELECT p.standard_id, p.prerequisite_id
FROM curriculum_studio.catalog_standard_prerequisites p
JOIN curriculum_studio.catalog_standards s ON s.id = p.standard_id
JOIN curriculum_studio.standard_frameworks f ON f.id = s.framework_id
JOIN curriculum_studio.catalog_standards pre ON pre.id = p.prerequisite_id
JOIN curriculum_studio.standard_frameworks pre_f ON pre_f.id = pre.framework_id
WHERE p.standard_id = $1
  AND (f.workspace_id IS NULL OR f.workspace_id = $2)
  AND (pre_f.workspace_id IS NULL OR pre_f.workspace_id = $2)
ORDER BY p.prerequisite_id`
	rows, err := r.Q.Query(ctx, q, standardID, workspaceID)
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
