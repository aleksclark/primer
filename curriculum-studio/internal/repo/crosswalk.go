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

// Create inserts a crosswalk edge.
func (r *CrosswalkRepo) Create(ctx context.Context, in *domain.StandardCrosswalk) (*domain.StandardCrosswalk, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
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
VALUES ($1, $2, $3, $4)
RETURNING id, from_standard_id, to_standard_id, relationship, notes, created_at`
	out, err := scanCrosswalk(r.Q.QueryRow(ctx, q, in.FromStandardID, in.ToStandardID, rel, in.Notes))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns a crosswalk by id.
func (r *CrosswalkRepo) Get(ctx context.Context, id uuid.UUID) (*domain.StandardCrosswalk, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, from_standard_id, to_standard_id, relationship, notes, created_at
FROM curriculum_studio.standard_crosswalks
WHERE id = $1`
	out, err := scanCrosswalk(r.Q.QueryRow(ctx, q, id))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// ListFrom returns crosswalks originating at fromStandardID.
func (r *CrosswalkRepo) ListFrom(ctx context.Context, fromStandardID uuid.UUID) ([]domain.StandardCrosswalk, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if fromStandardID == uuid.Nil {
		return []domain.StandardCrosswalk{}, nil
	}
	const q = `
SELECT id, from_standard_id, to_standard_id, relationship, notes, created_at
FROM curriculum_studio.standard_crosswalks
WHERE from_standard_id = $1
ORDER BY to_standard_id`
	rows, err := r.Q.Query(ctx, q, fromStandardID)
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
