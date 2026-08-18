package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// WorkspaceRepo persists curriculum_studio.workspaces with tenant scope on reads.
type WorkspaceRepo struct {
	Q Querier
}

// NewWorkspaceRepo binds to q.
func NewWorkspaceRepo(q Querier) *WorkspaceRepo {
	return &WorkspaceRepo{Q: q}
}

// Create inserts a workspace under tenant. Kind/status default when empty.
func (r *WorkspaceRepo) Create(ctx context.Context, w *domain.Workspace) (*domain.Workspace, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if w == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	if w.TenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant_id is required")
	}
	slug := strings.TrimSpace(w.Slug)
	name := strings.TrimSpace(w.Name)
	if slug == "" || name == "" {
		return nil, fmt.Errorf("workspace slug and name are required")
	}
	kind := w.Kind
	if kind == "" {
		kind = domain.WorkspaceKindTeacher
	}
	status := w.Status
	if status == "" {
		status = domain.WorkspaceStatusActive
	}
	const q = `
INSERT INTO curriculum_studio.workspaces (tenant_id, slug, name, kind, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, slug, name, kind, status, created_at, updated_at`
	out, err := scanWorkspace(r.Q.QueryRow(ctx, q, w.TenantID, slug, name, kind, status))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// GetByID returns a workspace by primary key. Callers that already resolved
// membership must still not leak rows across workspaces.
func (r *WorkspaceRepo) GetByID(ctx context.Context, workspaceID uuid.UUID) (*domain.Workspace, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, tenant_id, slug, name, kind, status, created_at, updated_at
FROM curriculum_studio.workspaces
WHERE id = $1`
	out, err := scanWorkspace(r.Q.QueryRow(ctx, q, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns a workspace by id only when it belongs to tenantID (IDOR defense).
func (r *WorkspaceRepo) Get(ctx context.Context, tenantID, workspaceID uuid.UUID) (*domain.Workspace, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil || workspaceID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, tenant_id, slug, name, kind, status, created_at, updated_at
FROM curriculum_studio.workspaces
WHERE id = $1 AND tenant_id = $2`
	out, err := scanWorkspace(r.Q.QueryRow(ctx, q, workspaceID, tenantID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// GetBySlug returns a workspace by (tenant_id, slug).
func (r *WorkspaceRepo) GetBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (*domain.Workspace, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, tenant_id, slug, name, kind, status, created_at, updated_at
FROM curriculum_studio.workspaces
WHERE tenant_id = $1 AND slug = $2`
	out, err := scanWorkspace(r.Q.QueryRow(ctx, q, tenantID, slug))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// List returns workspaces for a tenant ordered by slug.
func (r *WorkspaceRepo) List(ctx context.Context, tenantID uuid.UUID) ([]domain.Workspace, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil {
		return []domain.Workspace{}, nil
	}
	const q = `
SELECT id, tenant_id, slug, name, kind, status, created_at, updated_at
FROM curriculum_studio.workspaces
WHERE tenant_id = $1
ORDER BY slug ASC`
	rows, err := r.Q.Query(ctx, q, tenantID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.Workspace
	for rows.Next() {
		var w domain.Workspace
		if err := rows.Scan(&w.ID, &w.TenantID, &w.Slug, &w.Name, &w.Kind, &w.Status, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, MapError(err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.Workspace{}
	}
	return out, nil
}

// UpdateStatus sets workspace status when the row belongs to tenantID.
func (r *WorkspaceRepo) UpdateStatus(ctx context.Context, tenantID, workspaceID uuid.UUID, status string) (*domain.Workspace, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if status == "" {
		return nil, fmt.Errorf("status is required")
	}
	const q = `
UPDATE curriculum_studio.workspaces
SET status = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2
RETURNING id, tenant_id, slug, name, kind, status, created_at, updated_at`
	out, err := scanWorkspace(r.Q.QueryRow(ctx, q, workspaceID, tenantID, status))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func scanWorkspace(row pgx.Row) (*domain.Workspace, error) {
	var w domain.Workspace
	if err := row.Scan(&w.ID, &w.TenantID, &w.Slug, &w.Name, &w.Kind, &w.Status, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, err
	}
	return &w, nil
}
