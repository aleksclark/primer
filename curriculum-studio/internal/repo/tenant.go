package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// TenantRepo persists curriculum_studio.tenants.
type TenantRepo struct {
	Q Querier
}

// NewTenantRepo binds to q.
func NewTenantRepo(q Querier) *TenantRepo {
	return &TenantRepo{Q: q}
}

// Create inserts a tenant. Status defaults to active when empty.
func (r *TenantRepo) Create(ctx context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if t == nil {
		return nil, fmt.Errorf("tenant is required")
	}
	slug := strings.TrimSpace(t.Slug)
	name := strings.TrimSpace(t.Name)
	if slug == "" || name == "" {
		return nil, fmt.Errorf("tenant slug and name are required")
	}
	status := t.Status
	if status == "" {
		status = domain.TenantStatusActive
	}
	const q = `
INSERT INTO curriculum_studio.tenants (slug, name, status)
VALUES ($1, $2, $3)
RETURNING id, slug, name, status, created_at, updated_at`
	row := r.Q.QueryRow(ctx, q, slug, name, status)
	out, err := scanTenant(row)
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns a tenant by id.
func (r *TenantRepo) Get(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	const q = `
SELECT id, slug, name, status, created_at, updated_at
FROM curriculum_studio.tenants
WHERE id = $1`
	out, err := scanTenant(r.Q.QueryRow(ctx, q, id))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// GetBySlug returns a tenant by unique slug.
func (r *TenantRepo) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	const q = `
SELECT id, slug, name, status, created_at, updated_at
FROM curriculum_studio.tenants
WHERE slug = $1`
	out, err := scanTenant(r.Q.QueryRow(ctx, q, slug))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// List returns tenants ordered by slug.
func (r *TenantRepo) List(ctx context.Context) ([]domain.Tenant, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	const q = `
SELECT id, slug, name, status, created_at, updated_at
FROM curriculum_studio.tenants
ORDER BY slug ASC`
	rows, err := r.Q.Query(ctx, q)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, MapError(err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.Tenant{}
	}
	return out, nil
}

// UpdateStatus sets tenant status and bumps updated_at.
func (r *TenantRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) (*domain.Tenant, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if status == "" {
		return nil, fmt.Errorf("status is required")
	}
	const q = `
UPDATE curriculum_studio.tenants
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING id, slug, name, status, created_at, updated_at`
	out, err := scanTenant(r.Q.QueryRow(ctx, q, id, status))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func scanTenant(row pgx.Row) (*domain.Tenant, error) {
	var t domain.Tenant
	if err := row.Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}
