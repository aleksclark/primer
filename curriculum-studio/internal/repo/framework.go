package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// FrameworkRepo persists curriculum_studio.standard_frameworks.
// Global frameworks (workspace_id NULL) are unique on code; workspace
// frameworks are unique per (workspace_id, code). Global frameworks are
// readable by every workspace; workspace-owned frameworks are not.
type FrameworkRepo struct {
	Q Querier
}

// NewFrameworkRepo binds to q.
func NewFrameworkRepo(q Querier) *FrameworkRepo {
	return &FrameworkRepo{Q: q}
}

// Create inserts a global or workspace-owned framework.
func (r *FrameworkRepo) Create(ctx context.Context, in *domain.StandardFramework) (*domain.StandardFramework, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil {
		return nil, fmt.Errorf("framework is required")
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" || name == "" {
		return nil, fmt.Errorf("framework code and name are required")
	}
	var workspaceID any
	if in.WorkspaceID != nil {
		if *in.WorkspaceID == uuid.Nil {
			return nil, fmt.Errorf("workspace_id is required")
		}
		workspaceID = *in.WorkspaceID
	}
	const q = `
INSERT INTO curriculum_studio.standard_frameworks (workspace_id, code, name, jurisdiction, version)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, workspace_id, code, name, jurisdiction, version, created_at, updated_at`
	out, err := scanFramework(r.Q.QueryRow(ctx, q, workspaceID, code, name, in.Jurisdiction, in.Version))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns a framework visible to workspaceID. Global frameworks are
// visible to every requested workspace; workspace-owned frameworks are not.
func (r *FrameworkRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.StandardFramework, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, workspace_id, code, name, jurisdiction, version, created_at, updated_at
FROM curriculum_studio.standard_frameworks
WHERE id = $1 AND (workspace_id IS NULL OR workspace_id = $2)`
	out, err := scanFramework(r.Q.QueryRow(ctx, q, id, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// GetByCode returns the framework with code in an exact workspace scope. A nil
// workspaceID selects a global framework; a non-nil ID selects a workspace-owned
// framework. This is used by imports to make the source/title key idempotent.
func (r *FrameworkRepo) GetByCode(ctx context.Context, workspaceID *uuid.UUID, code string) (*domain.StandardFramework, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("framework code is required")
	}
	const q = `
SELECT id, workspace_id, code, name, jurisdiction, version, created_at, updated_at
FROM curriculum_studio.standard_frameworks
WHERE workspace_id IS NOT DISTINCT FROM $1 AND code = $2`
	var scope any
	if workspaceID != nil {
		if *workspaceID == uuid.Nil {
			return nil, fmt.Errorf("workspace_id is required")
		}
		scope = *workspaceID
	}
	out, err := scanFramework(r.Q.QueryRow(ctx, q, scope, code))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// ListWorkspace returns frameworks owned by workspaceID (not globals).
func (r *FrameworkRepo) ListWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.StandardFramework, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []domain.StandardFramework{}, nil
	}
	const q = `
SELECT id, workspace_id, code, name, jurisdiction, version, created_at, updated_at
FROM curriculum_studio.standard_frameworks
WHERE workspace_id = $1
ORDER BY code ASC`
	return listFrameworks(ctx, r.Q, q, workspaceID)
}

// ListVisiblePage returns a server-paginated view of global and workspace-owned
// frameworks visible to workspaceID.
func (r *FrameworkRepo) ListVisiblePage(ctx context.Context, workspaceID uuid.UUID, limit, offset int) ([]domain.StandardFramework, int, error) {
	if r == nil || r.Q == nil {
		return nil, 0, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []domain.StandardFramework{}, 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	const q = `
SELECT id, workspace_id, code, name, jurisdiction, version, created_at, updated_at,
       COUNT(*) OVER() AS total_count
FROM curriculum_studio.standard_frameworks
WHERE workspace_id IS NULL OR workspace_id = $1
ORDER BY workspace_id NULLS FIRST, code ASC
LIMIT $2 OFFSET $3`
	rows, err := r.Q.Query(ctx, q, workspaceID, limit, offset)
	if err != nil {
		return nil, 0, MapError(err)
	}
	defer rows.Close()
	out := []domain.StandardFramework{}
	total := 0
	for rows.Next() {
		var fw domain.StandardFramework
		if err := rows.Scan(&fw.ID, &fw.WorkspaceID, &fw.Code, &fw.Name, &fw.Jurisdiction, &fw.Version, &fw.CreatedAt, &fw.UpdatedAt, &total); err != nil {
			return nil, 0, MapError(err)
		}
		out = append(out, fw)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, MapError(err)
	}
	return out, total, nil
}

// ListVisible returns global frameworks plus those owned by workspaceID.
func (r *FrameworkRepo) ListVisible(ctx context.Context, workspaceID uuid.UUID) ([]domain.StandardFramework, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []domain.StandardFramework{}, nil
	}
	const q = `
SELECT id, workspace_id, code, name, jurisdiction, version, created_at, updated_at
FROM curriculum_studio.standard_frameworks
WHERE workspace_id IS NULL OR workspace_id = $1
ORDER BY workspace_id NULLS FIRST, code ASC`
	return listFrameworks(ctx, r.Q, q, workspaceID)
}

func listFrameworks(ctx context.Context, q Querier, sql string, args ...any) ([]domain.StandardFramework, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.StandardFramework
	for rows.Next() {
		fw, err := scanFramework(rows)
		if err != nil {
			return nil, MapError(err)
		}
		out = append(out, *fw)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.StandardFramework{}
	}
	return out, nil
}

func scanFramework(row pgx.Row) (*domain.StandardFramework, error) {
	var fw domain.StandardFramework
	if err := row.Scan(&fw.ID, &fw.WorkspaceID, &fw.Code, &fw.Name, &fw.Jurisdiction, &fw.Version, &fw.CreatedAt, &fw.UpdatedAt); err != nil {
		return nil, err
	}
	return &fw, nil
}
