package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// MaxResourceMetadataBytes is the largest JSON metadata object a resource may store.
// File bytes belong in the object store, referenced by an approved artifact_ref.
const MaxResourceMetadataBytes = 16 * 1024

// ResourceRepo persists curriculum_studio.resources metadata (no file bytes).
type ResourceRepo struct {
	Q Querier
}

// NewResourceRepo binds to q.
func NewResourceRepo(q Querier) *ResourceRepo {
	return &ResourceRepo{Q: q}
}

// Create inserts resource metadata. A workspace-owned resource carries its
// workspace scope in WorkspaceID; the DB trigger additionally proves that the
// workspace belongs to TenantID. A nil WorkspaceID is tenant-global metadata.
func (r *ResourceRepo) Create(ctx context.Context, in *domain.Resource) (*domain.Resource, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil {
		return nil, fmt.Errorf("resource is required")
	}
	if in.TenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant_id is required")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, fmt.Errorf("resource title is required")
	}
	kind := in.Kind
	if kind == "" {
		kind = domain.ResourceKindOther
	}
	if err := validateResourcePolicy(in); err != nil {
		return nil, err
	}
	meta := in.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	if in.WorkspaceID != nil {
		if *in.WorkspaceID == uuid.Nil {
			return nil, fmt.Errorf("workspace_id is required")
		}
		var one int
		if err := r.Q.QueryRow(ctx, `
SELECT 1 FROM curriculum_studio.workspaces
WHERE id = $1 AND tenant_id = $2`, *in.WorkspaceID, in.TenantID).Scan(&one); err != nil {
			return nil, MapError(err)
		}
	}
	var workspaceID any
	if in.WorkspaceID != nil {
		workspaceID = *in.WorkspaceID
	}
	const q = `
INSERT INTO curriculum_studio.resources
    (tenant_id, workspace_id, kind, title, authors, isbn, url, artifact_ref, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
RETURNING id, tenant_id, workspace_id, kind, title, authors, isbn, url, artifact_ref, metadata, created_at, updated_at`
	out, err := scanResource(r.Q.QueryRow(ctx, q,
		in.TenantID, workspaceID, kind, title, in.Authors, in.ISBN, in.URL, in.ArtifactRef, []byte(meta)))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns a resource visible to workspaceID under tenantID. Tenant-global
// resources are visible to every requested workspace; workspace-owned rows are
// never returned across workspace boundaries.
func (r *ResourceRepo) Get(ctx context.Context, tenantID, workspaceID, id uuid.UUID) (*domain.Resource, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil || workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT r.id, r.tenant_id, r.workspace_id, r.kind, r.title, r.authors, r.isbn, r.url, r.artifact_ref, r.metadata, r.created_at, r.updated_at
FROM curriculum_studio.resources r
JOIN curriculum_studio.workspaces requested_workspace
  ON requested_workspace.id = $3 AND requested_workspace.tenant_id = $2
WHERE r.id = $1 AND r.tenant_id = $2 AND (r.workspace_id IS NULL OR r.workspace_id = $3)`
	out, err := scanResource(r.Q.QueryRow(ctx, q, id, tenantID, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// ResourceListOptions controls server-side resource collection operations.
type ResourceListOptions struct {
	Q      string
	Kind   string
	Limit  int
	Offset int
}

// ListPageByWorkspace lists tenant-visible resources with SQL-side filtering
// and pagination. Workspace-owned and tenant-global metadata are both visible.
func (r *ResourceRepo) ListPageByWorkspace(ctx context.Context, tenantID, workspaceID uuid.UUID, opts ResourceListOptions) ([]domain.Resource, int, error) {
	if r == nil || r.Q == nil {
		return nil, 0, fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil || workspaceID == uuid.Nil {
		return []domain.Resource{}, 0, nil
	}
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	args := []any{tenantID, workspaceID}
	filters := []string{"r.tenant_id = $1", "(r.workspace_id IS NULL OR r.workspace_id = $2)"}
	if q := strings.TrimSpace(opts.Q); q != "" {
		args = append(args, "%"+escapeResourceLike(q)+"%")
		filters = append(filters, fmt.Sprintf("(r.title ILIKE $%d ESCAPE E'\\\\' OR r.authors ILIKE $%d ESCAPE E'\\\\')", len(args), len(args)))
	}
	if kind := strings.TrimSpace(opts.Kind); kind != "" {
		args = append(args, kind)
		filters = append(filters, fmt.Sprintf("r.kind = $%d", len(args)))
	}
	args = append(args, limit, offset)
	limitPos, offsetPos := len(args)-1, len(args)
	query := fmt.Sprintf(`
SELECT r.id, r.tenant_id, r.workspace_id, r.kind, r.title, r.authors, r.isbn, r.url, r.artifact_ref, r.metadata, r.created_at, r.updated_at,
       COUNT(*) OVER() AS total_count
FROM curriculum_studio.resources r
JOIN curriculum_studio.workspaces w ON w.id = $2 AND w.tenant_id = $1
WHERE %s
ORDER BY r.title ASC
LIMIT $%d OFFSET $%d`, strings.Join(filters, " AND "), limitPos, offsetPos)
	rows, err := r.Q.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, MapError(err)
	}
	defer rows.Close()
	out := []domain.Resource{}
	total := 0
	for rows.Next() {
		var resource domain.Resource
		if err := rows.Scan(&resource.ID, &resource.TenantID, &resource.WorkspaceID, &resource.Kind, &resource.Title, &resource.Authors, &resource.ISBN, &resource.URL, &resource.ArtifactRef, &resource.Metadata, &resource.CreatedAt, &resource.UpdatedAt, &total); err != nil {
			return nil, 0, MapError(err)
		}
		out = append(out, resource)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, MapError(err)
	}
	return out, total, nil
}

func escapeResourceLike(s string) string {
	s = strings.ReplaceAll(s, `\\`, `\\\\`)
	s = strings.ReplaceAll(s, `%`, `\\%`)
	s = strings.ReplaceAll(s, `_`, `\\_`)
	return s
}

// ListByWorkspace lists resources owned by a workspace under tenantID.
func (r *ResourceRepo) ListByWorkspace(ctx context.Context, tenantID, workspaceID uuid.UUID) ([]domain.Resource, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil || workspaceID == uuid.Nil {
		return []domain.Resource{}, nil
	}
	const q = `
SELECT r.id, r.tenant_id, r.workspace_id, r.kind, r.title, r.authors, r.isbn, r.url, r.artifact_ref, r.metadata, r.created_at, r.updated_at
FROM curriculum_studio.resources r
JOIN curriculum_studio.workspaces w ON w.id = r.workspace_id AND w.tenant_id = $1
WHERE r.tenant_id = $1 AND r.workspace_id = $2
ORDER BY r.title ASC`
	rows, err := r.Q.Query(ctx, q, tenantID, workspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.Resource
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, MapError(err)
		}
		out = append(out, *res)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.Resource{}
	}
	return out, nil
}

// validateResourcePolicy is an allowlist of metadata fields and reference
// formats. It deliberately has no denylist of suspicious spellings: unknown
// JSON keys and nested JSON values are rejected, so there is no alternate key
// or nesting path for file bytes/base64. The database trigger repeats this
// policy for direct SQL callers.
// Update replaces resource metadata within a tenant/workspace scope.
func (r *ResourceRepo) Update(ctx context.Context, tenantID, workspaceID, id uuid.UUID, in *domain.Resource) (*domain.Resource, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil || workspaceID == uuid.Nil || id == uuid.Nil || in == nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("resource title is required")
	}
	if err := validateResourcePolicy(in); err != nil {
		return nil, err
	}
	meta := in.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	const q = `
UPDATE curriculum_studio.resources
SET kind = $4, title = $5, authors = $6, isbn = $7, url = $8, artifact_ref = $9, metadata = $10::jsonb, updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND workspace_id = $3
RETURNING id, tenant_id, workspace_id, kind, title, authors, isbn, url, artifact_ref, metadata, created_at, updated_at`
	out, err := scanResource(r.Q.QueryRow(ctx, q, id, tenantID, workspaceID, in.Kind, strings.TrimSpace(in.Title), in.Authors, in.ISBN, in.URL, in.ArtifactRef, []byte(meta)))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Delete removes a workspace-owned resource. Database foreign keys surface
// plan-resource references as a conflict instead of silently orphaning them.
func (r *ResourceRepo) Delete(ctx context.Context, tenantID, workspaceID, id uuid.UUID) error {
	if r == nil || r.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil || workspaceID == uuid.Nil || id == uuid.Nil {
		return fmt.Errorf("%w", ErrNotFound)
	}
	_, err := r.Q.Exec(ctx, `DELETE FROM curriculum_studio.resources WHERE id = $1 AND tenant_id = $2 AND workspace_id = $3`, id, tenantID, workspaceID)
	return MapError(err)
}

func validateResourcePolicy(in *domain.Resource) error {
	if len(in.Metadata) > MaxResourceMetadataBytes {
		return fmt.Errorf("%w", ErrPayloadTooLarge)
	}
	if len(in.Metadata) > 0 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(in.Metadata, &fields); err != nil || fields == nil {
			return fmt.Errorf("%w", ErrCheckViolation)
		}
		for key, raw := range fields {
			switch key {
			case "description", "publisher", "language", "license", "note":
				var value string
				if json.Unmarshal(raw, &value) != nil || len(value) > 4096 {
					return fmt.Errorf("%w", ErrCheckViolation)
				}
			case "published_year":
				var value int
				if json.Unmarshal(raw, &value) != nil || value < 1000 || value > 3000 {
					return fmt.Errorf("%w", ErrCheckViolation)
				}
			case "tags":
				var tags []string
				if json.Unmarshal(raw, &tags) != nil || len(tags) > 64 {
					return fmt.Errorf("%w", ErrCheckViolation)
				}
				for _, tag := range tags {
					if len(tag) > 128 {
						return fmt.Errorf("%w", ErrCheckViolation)
					}
				}
			default:
				return fmt.Errorf("%w", ErrCheckViolation)
			}
		}
	}
	if in.ArtifactRef != "" && (len(in.ArtifactRef) > 512 || !validArtifactRef(in.ArtifactRef)) {
		return fmt.Errorf("%w", ErrCheckViolation)
	}
	if in.URL != "" {
		u, err := url.Parse(in.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("%w", ErrCheckViolation)
		}
	}
	return nil
}

func validArtifactRef(ref string) bool {
	if !strings.HasPrefix(ref, "obj:") && !strings.HasPrefix(ref, "urn:") {
		return false
	}
	for i, r := range ref[4:] {
		if i == 0 && !isArtifactStart(r) {
			return false
		}
		if !isArtifactPart(r) {
			return false
		}
	}
	return len(ref) > 4
}

func isArtifactStart(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

func isArtifactPart(r rune) bool {
	return isArtifactStart(r) || r == '.' || r == '_' || r == ':' || r == '/' || r == '-'
}

func scanResource(row pgx.Row) (*domain.Resource, error) {
	var r domain.Resource
	var meta []byte
	if err := row.Scan(&r.ID, &r.TenantID, &r.WorkspaceID, &r.Kind, &r.Title, &r.Authors, &r.ISBN, &r.URL, &r.ArtifactRef, &meta, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	r.Metadata = json.RawMessage(meta)
	return &r, nil
}
