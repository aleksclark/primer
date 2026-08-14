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

// IntegrationIdentityRepo persists curriculum_studio.integration_identities.
// Snapshots are opaque JSON; this package never opens LMS/Identity DSNs.
// Callers must not log full snapshot payloads at info level.
// Inputs are bound/sanitized (domain.SanitizeIntegrationIdentity) before SQL.
type IntegrationIdentityRepo struct {
	Q Querier
}

// NewIntegrationIdentityRepo binds to q.
func NewIntegrationIdentityRepo(q Querier) *IntegrationIdentityRepo {
	return &IntegrationIdentityRepo{Q: q}
}

// Upsert inserts or updates by unique (workspace_id, system, external_kind, external_ref).
// On conflict, snapshot/display_label are replaced and last_seen_at/updated_at bump.
// Email is never required. external_ref is stored as opaque text only.
// Snapshot bodies are never logged by this method.
func (r *IntegrationIdentityRepo) Upsert(ctx context.Context, in *domain.IntegrationIdentity) (*domain.IntegrationIdentity, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil {
		return nil, fmt.Errorf("integration identity is required")
	}
	if in.WorkspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace_id is required")
	}
	system := strings.TrimSpace(in.System)
	kind := strings.TrimSpace(in.ExternalKind)
	if system == "" || kind == "" {
		return nil, fmt.Errorf("system, external_kind, and external_ref are required")
	}
	// Copy so caller-visible rejection leaves their pointer untouched on failure.
	local := *in
	local.System = system
	local.ExternalKind = kind
	if err := domain.SanitizeIntegrationIdentity(&local); err != nil {
		return nil, err
	}
	if local.ExternalRef == "" {
		return nil, fmt.Errorf("system, external_kind, and external_ref are required")
	}
	const q = `
INSERT INTO curriculum_studio.integration_identities
    (workspace_id, system, external_kind, external_ref, display_label, snapshot, last_seen_at)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, now())
ON CONFLICT (workspace_id, system, external_kind, external_ref)
DO UPDATE SET
    display_label = EXCLUDED.display_label,
    snapshot = EXCLUDED.snapshot,
    last_seen_at = now(),
    updated_at = now()
RETURNING id, workspace_id, system, external_kind, external_ref, display_label, snapshot, last_seen_at, created_at, updated_at`
	out, err := scanIntegrationIdentity(r.Q.QueryRow(ctx, q, local.WorkspaceID, local.System, local.ExternalKind, local.ExternalRef, local.DisplayLabel, []byte(local.Snapshot)))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns an integration identity by id within workspace scope.
func (r *IntegrationIdentityRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.IntegrationIdentity, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, workspace_id, system, external_kind, external_ref, display_label, snapshot, last_seen_at, created_at, updated_at
FROM curriculum_studio.integration_identities
WHERE id = $1 AND workspace_id = $2`
	out, err := scanIntegrationIdentity(r.Q.QueryRow(ctx, q, id, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// GetByExternalRef looks up by natural key within workspace.
func (r *IntegrationIdentityRepo) GetByExternalRef(ctx context.Context, workspaceID uuid.UUID, system, externalKind, externalRef string) (*domain.IntegrationIdentity, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, workspace_id, system, external_kind, external_ref, display_label, snapshot, last_seen_at, created_at, updated_at
FROM curriculum_studio.integration_identities
WHERE workspace_id = $1 AND system = $2 AND external_kind = $3 AND external_ref = $4`
	out, err := scanIntegrationIdentity(r.Q.QueryRow(ctx, q, workspaceID, system, externalKind, externalRef))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// List returns integration identities for a workspace.
func (r *IntegrationIdentityRepo) List(ctx context.Context, workspaceID uuid.UUID) ([]domain.IntegrationIdentity, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []domain.IntegrationIdentity{}, nil
	}
	const q = `
SELECT id, workspace_id, system, external_kind, external_ref, display_label, snapshot, last_seen_at, created_at, updated_at
FROM curriculum_studio.integration_identities
WHERE workspace_id = $1
ORDER BY system ASC, external_kind ASC, external_ref ASC`
	rows, err := r.Q.Query(ctx, q, workspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.IntegrationIdentity
	for rows.Next() {
		var m domain.IntegrationIdentity
		var snap []byte
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.System, &m.ExternalKind, &m.ExternalRef, &m.DisplayLabel, &snap, &m.LastSeenAt, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, MapError(err)
		}
		m.Snapshot = json.RawMessage(snap)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.IntegrationIdentity{}
	}
	return out, nil
}

func scanIntegrationIdentity(row pgx.Row) (*domain.IntegrationIdentity, error) {
	var m domain.IntegrationIdentity
	var snap []byte
	if err := row.Scan(&m.ID, &m.WorkspaceID, &m.System, &m.ExternalKind, &m.ExternalRef, &m.DisplayLabel, &snap, &m.LastSeenAt, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	m.Snapshot = json.RawMessage(snap)
	return &m, nil
}
