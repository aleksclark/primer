package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// CatalogStandardRepo persists curriculum_studio.catalog_standards.
type CatalogStandardRepo struct {
	Q Querier
}

// NewCatalogStandardRepo binds to q.
func NewCatalogStandardRepo(q Querier) *CatalogStandardRepo {
	return &CatalogStandardRepo{Q: q}
}

// Create inserts a catalog standard. Unique (framework_id, code) is enforced by Postgres.
func (r *CatalogStandardRepo) Create(ctx context.Context, in *domain.CatalogStandard) (*domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil {
		return nil, fmt.Errorf("catalog standard is required")
	}
	if in.FrameworkID == uuid.Nil {
		return nil, fmt.Errorf("framework_id is required")
	}
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return nil, fmt.Errorf("standard code is required")
	}
	var parent any
	if in.ParentID != nil {
		if *in.ParentID == uuid.Nil {
			return nil, fmt.Errorf("parent_id is required")
		}
		parent = *in.ParentID
	}
	meta := in.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	const q = `
INSERT INTO curriculum_studio.catalog_standards
    (framework_id, parent_id, code, subject_code, grade_band, domain, cluster, description, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
RETURNING id, framework_id, parent_id, code, subject_code, grade_band, domain, cluster, description, metadata, created_at, updated_at`
	out, err := scanCatalogStandard(r.Q.QueryRow(ctx, q,
		in.FrameworkID, parent, code, in.SubjectCode, in.GradeBand, in.Domain, in.Cluster, in.Description, []byte(meta)))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns a catalog standard by id.
func (r *CatalogStandardRepo) Get(ctx context.Context, id uuid.UUID) (*domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, framework_id, parent_id, code, subject_code, grade_band, domain, cluster, description, metadata, created_at, updated_at
FROM curriculum_studio.catalog_standards
WHERE id = $1`
	out, err := scanCatalogStandard(r.Q.QueryRow(ctx, q, id))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// ListByFramework returns all standards in a framework ordered by code.
func (r *CatalogStandardRepo) ListByFramework(ctx context.Context, frameworkID uuid.UUID) ([]domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if frameworkID == uuid.Nil {
		return []domain.CatalogStandard{}, nil
	}
	const q = `
SELECT id, framework_id, parent_id, code, subject_code, grade_band, domain, cluster, description, metadata, created_at, updated_at
FROM curriculum_studio.catalog_standards
WHERE framework_id = $1
ORDER BY code ASC`
	return listCatalogStandards(ctx, r.Q, q, frameworkID)
}

// ListChildren returns direct children of parentID ordered by code.
func (r *CatalogStandardRepo) ListChildren(ctx context.Context, parentID uuid.UUID) ([]domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if parentID == uuid.Nil {
		return []domain.CatalogStandard{}, nil
	}
	const q = `
SELECT id, framework_id, parent_id, code, subject_code, grade_band, domain, cluster, description, metadata, created_at, updated_at
FROM curriculum_studio.catalog_standards
WHERE parent_id = $1
ORDER BY code ASC`
	return listCatalogStandards(ctx, r.Q, q, parentID)
}

// Import inserts a small standards set. Callers should wrap this in WithTx.
func (r *CatalogStandardRepo) Import(ctx context.Context, frameworkID uuid.UUID, items []domain.CatalogStandard) ([]domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if frameworkID == uuid.Nil {
		return nil, fmt.Errorf("framework_id is required")
	}
	out := make([]domain.CatalogStandard, 0, len(items))
	for i := range items {
		in := items[i]
		in.FrameworkID = frameworkID
		created, err := r.Create(ctx, &in)
		if err != nil {
			return nil, err
		}
		out = append(out, *created)
	}
	return out, nil
}

func listCatalogStandards(ctx context.Context, q Querier, sql string, args ...any) ([]domain.CatalogStandard, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.CatalogStandard
	for rows.Next() {
		s, err := scanCatalogStandard(rows)
		if err != nil {
			return nil, MapError(err)
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.CatalogStandard{}
	}
	return out, nil
}

func scanCatalogStandard(row pgx.Row) (*domain.CatalogStandard, error) {
	var s domain.CatalogStandard
	var meta []byte
	if err := row.Scan(&s.ID, &s.FrameworkID, &s.ParentID, &s.Code, &s.SubjectCode, &s.GradeBand, &s.Domain, &s.Cluster, &s.Description, &meta, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.Metadata = json.RawMessage(meta)
	return &s, nil
}
