package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// MaxResourceMetadataBytes is the largest JSON metadata object a resource may store.
// File bytes belong in the object store, referenced by artifact_ref.
const MaxResourceMetadataBytes = 16 * 1024

// ResourceRepo persists curriculum_studio.resources metadata (no file bytes).
type ResourceRepo struct {
	Q Querier
}

// NewResourceRepo binds to q.
func NewResourceRepo(q Querier) *ResourceRepo {
	return &ResourceRepo{Q: q}
}

// Create inserts resource metadata. Giant / encoded-byte payloads are refused.
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
	if err := rejectResourceBytes(in); err != nil {
		return nil, err
	}
	meta := in.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	if len(meta) > MaxResourceMetadataBytes {
		return nil, fmt.Errorf("%w", ErrPayloadTooLarge)
	}
	var workspaceID any
	if in.WorkspaceID != nil {
		if *in.WorkspaceID == uuid.Nil {
			return nil, fmt.Errorf("workspace_id is required")
		}
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

// Get returns a resource scoped to tenantID (IDOR defense).
func (r *ResourceRepo) Get(ctx context.Context, tenantID, id uuid.UUID) (*domain.Resource, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if tenantID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, tenant_id, workspace_id, kind, title, authors, isbn, url, artifact_ref, metadata, created_at, updated_at
FROM curriculum_studio.resources
WHERE id = $1 AND tenant_id = $2`
	out, err := scanResource(r.Q.QueryRow(ctx, q, id, tenantID))
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
SELECT id, tenant_id, workspace_id, kind, title, authors, isbn, url, artifact_ref, metadata, created_at, updated_at
FROM curriculum_studio.resources
WHERE tenant_id = $1 AND workspace_id = $2
ORDER BY title ASC`
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

func rejectResourceBytes(in *domain.Resource) error {
	if looksLikeEncodedBytes(in.ArtifactRef) || looksLikeEncodedBytes(in.Title) || looksLikeEncodedBytes(in.URL) {
		return fmt.Errorf("%w", ErrPayloadTooLarge)
	}
	if len(in.Metadata) == 0 {
		return nil
	}
	if len(in.Metadata) > MaxResourceMetadataBytes {
		return fmt.Errorf("%w", ErrPayloadTooLarge)
	}
	if !utf8.Valid(in.Metadata) {
		return fmt.Errorf("%w", ErrPayloadTooLarge)
	}
	low := strings.ToLower(string(in.Metadata))
	for _, needle := range []string{"content_b64", "file_bytes", "base64,", `"body"`, `"bytes"`, `"blob"`} {
		if strings.Contains(low, needle) {
			return fmt.Errorf("%w", ErrPayloadTooLarge)
		}
	}
	return nil
}

func looksLikeEncodedBytes(s string) bool {
	if len(s) > 4096 {
		return true
	}
	low := strings.ToLower(s)
	return strings.Contains(low, "base64,") || strings.Contains(low, "data:")
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
