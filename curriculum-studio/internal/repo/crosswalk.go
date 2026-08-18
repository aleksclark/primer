package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// CrosswalkRepo persists curriculum_studio.standard_crosswalks.
type CrosswalkRepo struct {
	Q Querier
}

// NewCrosswalkRepo binds to q.
func NewCrosswalkRepo(q Querier) *CrosswalkRepo {
	return &CrosswalkRepo{Q: q}
}

// Create inserts a crosswalk whose endpoints are both visible to workspaceID.
func (r *CrosswalkRepo) Create(ctx context.Context, workspaceID uuid.UUID, in *domain.StandardCrosswalk) (*domain.StandardCrosswalk, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if in == nil {
		return nil, fmt.Errorf("crosswalk is required")
	}
	if in.FromStandardID == uuid.Nil || in.ToStandardID == uuid.Nil {
		return nil, fmt.Errorf("from_standard_id and to_standard_id are required")
	}
	rel := in.Relationship
	if rel == "" {
		rel = domain.CrosswalkRelated
	}
	const q = `
INSERT INTO curriculum_studio.standard_crosswalks (from_standard_id, to_standard_id, relationship, notes)
SELECT from_s.id, to_s.id, $3, $4
FROM curriculum_studio.catalog_standards from_s
JOIN curriculum_studio.standard_frameworks from_f ON from_f.id = from_s.framework_id
JOIN curriculum_studio.catalog_standards to_s ON to_s.id = $2
JOIN curriculum_studio.standard_frameworks to_f ON to_f.id = to_s.framework_id
WHERE from_s.id = $1
  AND (from_f.workspace_id IS NULL OR from_f.workspace_id = $5)
  AND (to_f.workspace_id IS NULL OR to_f.workspace_id = $5)
RETURNING id, from_standard_id, to_standard_id, relationship, notes, created_at`
	out, err := scanCrosswalk(r.Q.QueryRow(ctx, q, in.FromStandardID, in.ToStandardID, rel, in.Notes, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns a crosswalk visible to workspaceID.
func (r *CrosswalkRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.StandardCrosswalk, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT c.id, c.from_standard_id, c.to_standard_id, c.relationship, c.notes, c.created_at
FROM curriculum_studio.standard_crosswalks c
JOIN curriculum_studio.catalog_standards from_s ON from_s.id = c.from_standard_id
JOIN curriculum_studio.standard_frameworks from_f ON from_f.id = from_s.framework_id
JOIN curriculum_studio.catalog_standards to_s ON to_s.id = c.to_standard_id
JOIN curriculum_studio.standard_frameworks to_f ON to_f.id = to_s.framework_id
WHERE c.id = $1
  AND (from_f.workspace_id IS NULL OR from_f.workspace_id = $2)
  AND (to_f.workspace_id IS NULL OR to_f.workspace_id = $2)`
	out, err := scanCrosswalk(r.Q.QueryRow(ctx, q, id, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// ListFrom returns crosswalks originating at a standard visible to workspaceID.
func (r *CrosswalkRepo) ListFrom(ctx context.Context, workspaceID, fromStandardID uuid.UUID) ([]domain.StandardCrosswalk, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || fromStandardID == uuid.Nil {
		return []domain.StandardCrosswalk{}, nil
	}
	const q = `
SELECT c.id, c.from_standard_id, c.to_standard_id, c.relationship, c.notes, c.created_at
FROM curriculum_studio.standard_crosswalks c
JOIN curriculum_studio.catalog_standards from_s ON from_s.id = c.from_standard_id
JOIN curriculum_studio.standard_frameworks from_f ON from_f.id = from_s.framework_id
JOIN curriculum_studio.catalog_standards to_s ON to_s.id = c.to_standard_id
JOIN curriculum_studio.standard_frameworks to_f ON to_f.id = to_s.framework_id
WHERE c.from_standard_id = $1
  AND (from_f.workspace_id IS NULL OR from_f.workspace_id = $2)
  AND (to_f.workspace_id IS NULL OR to_f.workspace_id = $2)
ORDER BY c.to_standard_id`
	rows, err := r.Q.Query(ctx, q, fromStandardID, workspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.StandardCrosswalk
	for rows.Next() {
		cw, err := scanCrosswalk(rows)
		if err != nil {
			return nil, MapError(err)
		}
		out = append(out, *cw)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.StandardCrosswalk{}
	}
	return out, nil
}

func scanCrosswalk(row pgx.Row) (*domain.StandardCrosswalk, error) {
	var c domain.StandardCrosswalk
	if err := row.Scan(&c.ID, &c.FromStandardID, &c.ToStandardID, &c.Relationship, &c.Notes, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}
