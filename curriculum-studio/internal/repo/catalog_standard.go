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

// Create inserts a catalog standard visible to workspaceID. The framework may
// be global or owned by that workspace; a foreign workspace framework is not
// a valid insert scope.
func (r *CatalogStandardRepo) Create(ctx context.Context, workspaceID uuid.UUID, in *domain.CatalogStandard) (*domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace_id is required")
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
SELECT f.id, $2, $3, $4, $5, $6, $7, $8, $9::jsonb
FROM curriculum_studio.standard_frameworks f
WHERE f.id = $1 AND (f.workspace_id IS NULL OR f.workspace_id = $10)
RETURNING id, framework_id, parent_id, code, subject_code, grade_band, domain, cluster, description, metadata, created_at, updated_at`
	out, err := scanCatalogStandard(r.Q.QueryRow(ctx, q,
		in.FrameworkID, parent, code, in.SubjectCode, in.GradeBand, in.Domain, in.Cluster, in.Description, []byte(meta), workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns a standard visible to workspaceID. A standard in a global
// framework is readable by every requested workspace.
func (r *CatalogStandardRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT s.id, s.framework_id, s.parent_id, s.code, s.subject_code, s.grade_band, s.domain, s.cluster, s.description, s.metadata, s.created_at, s.updated_at
FROM curriculum_studio.catalog_standards s
JOIN curriculum_studio.standard_frameworks f ON f.id = s.framework_id
WHERE s.id = $1 AND (f.workspace_id IS NULL OR f.workspace_id = $2)`
	out, err := scanCatalogStandard(r.Q.QueryRow(ctx, q, id, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// StandardListOptions controls server-side filtering and pagination.
type StandardListOptions struct {
	Q        string
	Subject  string
	Grade    string
	ParentID *uuid.UUID
	Limit    int
	Offset   int
}

// ListPage returns visible standards with SQL-side filters and total count.
func (r *CatalogStandardRepo) ListPage(ctx context.Context, workspaceID, frameworkID uuid.UUID, opts StandardListOptions) ([]domain.CatalogStandard, int, error) {
	if r == nil || r.Q == nil {
		return nil, 0, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || frameworkID == uuid.Nil {
		return []domain.CatalogStandard{}, 0, nil
	}
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	args := []any{frameworkID, workspaceID}
	filters := []string{"s.framework_id = $1", "(f.workspace_id IS NULL OR f.workspace_id = $2)"}
	if q := strings.TrimSpace(opts.Q); q != "" {
		args = append(args, "%"+escapeCatalogLike(q)+"%")
		filters = append(filters, fmt.Sprintf("(s.code ILIKE $%d ESCAPE E'\\\\' OR s.description ILIKE $%d ESCAPE E'\\\\')", len(args), len(args)))
	}
	if subject := strings.TrimSpace(opts.Subject); subject != "" {
		args = append(args, subject)
		filters = append(filters, fmt.Sprintf("s.subject_code = $%d", len(args)))
	}
	if grade := strings.TrimSpace(opts.Grade); grade != "" {
		args = append(args, grade)
		filters = append(filters, fmt.Sprintf("s.grade_band = $%d", len(args)))
	}
	if opts.ParentID != nil {
		args = append(args, *opts.ParentID)
		filters = append(filters, fmt.Sprintf("s.parent_id = $%d", len(args)))
	}
	args = append(args, limit, offset)
	limitPos, offsetPos := len(args)-1, len(args)
	query := fmt.Sprintf(`
SELECT s.id, s.framework_id, s.parent_id, s.code, s.subject_code, s.grade_band, s.domain, s.cluster, s.description, s.metadata, s.created_at, s.updated_at,
       COUNT(*) OVER() AS total_count
FROM curriculum_studio.catalog_standards s
JOIN curriculum_studio.standard_frameworks f ON f.id = s.framework_id
WHERE %s
ORDER BY s.code ASC
LIMIT $%d OFFSET $%d`, strings.Join(filters, " AND "), limitPos, offsetPos)
	rows, err := r.Q.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, MapError(err)
	}
	defer rows.Close()
	out := []domain.CatalogStandard{}
	total := 0
	for rows.Next() {
		var standard domain.CatalogStandard
		if err := rows.Scan(&standard.ID, &standard.FrameworkID, &standard.ParentID, &standard.Code, &standard.SubjectCode, &standard.GradeBand, &standard.Domain, &standard.Cluster, &standard.Description, &standard.Metadata, &standard.CreatedAt, &standard.UpdatedAt, &total); err != nil {
			return nil, 0, MapError(err)
		}
		out = append(out, standard)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, MapError(err)
	}
	return out, total, nil
}

// ListByFramework returns all visible standards in a framework ordered by code.
func (r *CatalogStandardRepo) ListByFramework(ctx context.Context, workspaceID, frameworkID uuid.UUID) ([]domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || frameworkID == uuid.Nil {
		return []domain.CatalogStandard{}, nil
	}
	const q = `
SELECT s.id, s.framework_id, s.parent_id, s.code, s.subject_code, s.grade_band, s.domain, s.cluster, s.description, s.metadata, s.created_at, s.updated_at
FROM curriculum_studio.catalog_standards s
JOIN curriculum_studio.standard_frameworks f ON f.id = s.framework_id
WHERE s.framework_id = $1 AND (f.workspace_id IS NULL OR f.workspace_id = $2)
ORDER BY s.code ASC`
	return listCatalogStandards(ctx, r.Q, q, frameworkID, workspaceID)
}

// ListChildren returns direct children of a visible parent ordered by code.
func (r *CatalogStandardRepo) ListChildren(ctx context.Context, workspaceID, parentID uuid.UUID) ([]domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || parentID == uuid.Nil {
		return []domain.CatalogStandard{}, nil
	}
	const q = `
SELECT s.id, s.framework_id, s.parent_id, s.code, s.subject_code, s.grade_band, s.domain, s.cluster, s.description, s.metadata, s.created_at, s.updated_at
FROM curriculum_studio.catalog_standards s
JOIN curriculum_studio.standard_frameworks f ON f.id = s.framework_id
WHERE s.parent_id = $1 AND (f.workspace_id IS NULL OR f.workspace_id = $2)
ORDER BY s.code ASC`
	return listCatalogStandards(ctx, r.Q, q, parentID, workspaceID)
}

// Import inserts a small standards set in one framework and workspace scope.
// Callers should wrap this in WithTx.
func (r *CatalogStandardRepo) Import(ctx context.Context, workspaceID, frameworkID uuid.UUID, items []domain.CatalogStandard) ([]domain.CatalogStandard, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if frameworkID == uuid.Nil {
		return nil, fmt.Errorf("framework_id is required")
	}
	out := make([]domain.CatalogStandard, 0, len(items))
	for i := range items {
		in := items[i]
		in.FrameworkID = frameworkID
		created, err := r.Create(ctx, workspaceID, &in)
		if err != nil {
			return nil, err
		}
		out = append(out, *created)
	}
	return out, nil
}

func escapeCatalogLike(s string) string {
	s = strings.ReplaceAll(s, `\\`, `\\\\`)
	s = strings.ReplaceAll(s, `%`, `\\%`)
	s = strings.ReplaceAll(s, `_`, `\\_`)
	return s
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
