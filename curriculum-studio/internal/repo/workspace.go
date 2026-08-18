package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// WorkspaceListOptions controls server-side filtering, sorting and pagination
// for ListForSubject. All fields are optional; safe defaults are applied.
type WorkspaceListOptions struct {
	// Q is a free-text search applied to the workspace name (case-insensitive
	// prefix/substring). Empty means no name filter.
	Q string
	// Sort is the column name to sort by. Allowed values: name, status,
	// created_at. Default: name.
	Sort string
	// Dir is the sort direction: asc or desc (case-insensitive). Default: asc.
	Dir string
	// Limit is the maximum rows to return (1–100). Default: 50.
	Limit int
	// Offset is the number of rows to skip. Default: 0.
	Offset int
}

// ListResult holds the paged workspace rows and the total count before paging.
type ListResult struct {
	Items []domain.Workspace
	Total int
}

// ListForSubject returns workspaces visible to subjectRef via active memberships,
// with server-side filtering/sorting/pagination. Filtering is applied in SQL;
// no row is loaded into memory before the WHERE clause is evaluated.
func (r *WorkspaceRepo) ListForSubject(ctx context.Context, subjectRef string, opts WorkspaceListOptions) (ListResult, error) {
	if r == nil || r.Q == nil {
		return ListResult{}, fmt.Errorf("%w", ErrClosed)
	}
	canon, err := domain.CanonicalizeSubjectRef(subjectRef)
	if err != nil {
		return ListResult{}, err
	}

	// Whitelist sort column.
	sortCol := "name"
	switch opts.Sort {
	case "status", "created_at":
		sortCol = opts.Sort
	}
	// Whitelist direction.
	dir := "ASC"
	if strings.EqualFold(strings.TrimSpace(opts.Dir), "desc") {
		dir = "DESC"
	}
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	// Build args and optional WHERE fragment for free-text search.
	// $1 is always the canonical subject_ref; q (if set) appends $2.
	args := []any{canon.String()}
	qClause := ""
	if q := strings.TrimSpace(opts.Q); q != "" {
		args = append(args, "%"+escapeLike(q)+"%")
		qClause = fmt.Sprintf("AND w.name ILIKE $%d", len(args))
	}
	// LIMIT and OFFSET are appended last.
	args = append(args, limit, offset)
	limitPos := len(args) - 1
	offsetPos := len(args)

	//nolint:gosec // sort column and direction are whitelist-validated above, not user input.
	sql := fmt.Sprintf(`
SELECT w.id, w.tenant_id, w.slug, w.name, w.kind, w.status, w.created_at, w.updated_at,
       COUNT(*) OVER() AS total_count
FROM curriculum_studio.workspaces w
INNER JOIN curriculum_studio.workspace_memberships m
    ON m.workspace_id = w.id
    AND m.subject_ref = $1
    AND m.status = 'active'
WHERE TRUE %s
ORDER BY w.%s %s
LIMIT $%d OFFSET $%d`,
		qClause, sortCol, dir, limitPos, offsetPos)

	rows, err := r.Q.Query(ctx, sql, args...)
	if err != nil {
		return ListResult{}, MapError(err)
	}
	defer rows.Close()

	var result ListResult
	for rows.Next() {
		var w domain.Workspace
		var total int
		if err := rows.Scan(
			&w.ID, &w.TenantID, &w.Slug, &w.Name, &w.Kind, &w.Status,
			&w.CreatedAt, &w.UpdatedAt,
			&total,
		); err != nil {
			return ListResult{}, MapError(err)
		}
		result.Items = append(result.Items, w)
		result.Total = total
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, MapError(err)
	}
	if result.Items == nil {
		result.Items = []domain.Workspace{}
	}
	return result, nil
}

// escapeLike escapes LIKE metacharacters so that q cannot use wildcards.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// Update sets name and status for the tenant-scoped workspace. Empty string
// values are ignored (the existing column value is kept).
func (r *WorkspaceRepo) Update(ctx context.Context, tenantID, workspaceID uuid.UUID, name, status string) (*domain.Workspace, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil || workspaceID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
UPDATE curriculum_studio.workspaces
SET
    name       = CASE WHEN $3 <> '' THEN $3 ELSE name END,
    status     = CASE WHEN $4 <> '' THEN $4 ELSE status END,
    updated_at = now()
WHERE id = $1 AND tenant_id = $2
RETURNING id, tenant_id, slug, name, kind, status, created_at, updated_at`
	out, err := scanWorkspace(r.Q.QueryRow(ctx, q, workspaceID, tenantID, name, status))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

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
