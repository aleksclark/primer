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
SELECT id, tenant_id, workspace_id, kind, title, authors, isbn, url, artifact_ref, metadata, created_at, updated_at
FROM curriculum_studio.resources
WHERE id = $1 AND tenant_id = $2 AND (workspace_id IS NULL OR workspace_id = $3)`
	out, err := scanResource(r.Q.QueryRow(ctx, q, id, tenantID, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
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
