package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// MembershipRepo persists curriculum_studio.workspace_memberships.
// All reads/writes are workspace-scoped to prevent IDOR.
// subject_ref is always canonicalized before SQL (see domain.CanonicalizeSubjectRef).
type MembershipRepo struct {
	Q Querier
}

// NewMembershipRepo binds to q.
func NewMembershipRepo(q Querier) *MembershipRepo {
	return &MembershipRepo{Q: q}
}

// Create inserts a membership projection. subject_ref is canonicalized at app layer.
func (r *MembershipRepo) Create(ctx context.Context, m *domain.WorkspaceMembership) (*domain.WorkspaceMembership, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if m == nil {
		return nil, fmt.Errorf("membership is required")
	}
	if m.WorkspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace_id is required")
	}
	kindHint := m.SubjectKind
	if kindHint == "" {
		kindHint = domain.SubjectKindHuman
	}
	canon, err := domain.CanonicalizeSubjectRef(m.SubjectRef)
	if err != nil {
		return nil, err
	}
	if kindHint != "" && kindHint != canon.Kind {
		return nil, fmt.Errorf("%w: kind %q does not match ref kind %q", domain.ErrInvalidSubject, kindHint, canon.Kind)
	}
	role := m.Role
	if role == "" {
		role = domain.MembershipRoleViewer
	}
	status := m.Status
	if status == "" {
		status = domain.MembershipStatusActive
	}
	display := m.DisplayName
	const q = `
INSERT INTO curriculum_studio.workspace_memberships
    (workspace_id, subject_ref, subject_kind, display_name, role, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, workspace_id, subject_ref, subject_kind, display_name, role, status, created_at, updated_at`
	out, err := scanMembership(r.Q.QueryRow(ctx, q, m.WorkspaceID, canon.String(), canon.Kind, display, role, status))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// Get returns a membership by id only within workspaceID.
func (r *MembershipRepo) Get(ctx context.Context, workspaceID, membershipID uuid.UUID) (*domain.WorkspaceMembership, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || membershipID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `
SELECT id, workspace_id, subject_ref, subject_kind, display_name, role, status, created_at, updated_at
FROM curriculum_studio.workspace_memberships
WHERE id = $1 AND workspace_id = $2`
	out, err := scanMembership(r.Q.QueryRow(ctx, q, membershipID, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// GetActiveMembership returns the active membership for subject_ref in workspace.
// subject_ref is canonicalized so case variants resolve the same row.
func (r *MembershipRepo) GetActiveMembership(ctx context.Context, workspaceID uuid.UUID, subjectRef string) (*domain.WorkspaceMembership, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	canon, err := domain.CanonicalizeSubjectRef(subjectRef)
	if err != nil {
		return nil, err
	}
	const q = `
SELECT id, workspace_id, subject_ref, subject_kind, display_name, role, status, created_at, updated_at
FROM curriculum_studio.workspace_memberships
WHERE workspace_id = $1 AND subject_ref = $2 AND status = 'active'`
	out, err := scanMembership(r.Q.QueryRow(ctx, q, workspaceID, canon.String()))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// ListActiveBySubject returns active memberships for a subject across workspaces.
func (r *MembershipRepo) ListActiveBySubject(ctx context.Context, subjectRef string) ([]domain.WorkspaceMembership, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	canon, err := domain.CanonicalizeSubjectRef(subjectRef)
	if err != nil {
		return nil, err
	}
	const q = `
SELECT id, workspace_id, subject_ref, subject_kind, display_name, role, status, created_at, updated_at
FROM curriculum_studio.workspace_memberships
WHERE subject_ref = $1 AND status = 'active'
ORDER BY workspace_id ASC`
	rows, err := r.Q.Query(ctx, q, canon.String())
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.WorkspaceMembership
	for rows.Next() {
		var m domain.WorkspaceMembership
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.SubjectRef, &m.SubjectKind, &m.DisplayName, &m.Role, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, MapError(err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.WorkspaceMembership{}
	}
	return out, nil
}

// List returns memberships for a workspace ordered by subject_ref.
func (r *MembershipRepo) List(ctx context.Context, workspaceID uuid.UUID) ([]domain.WorkspaceMembership, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []domain.WorkspaceMembership{}, nil
	}
	const q = `
SELECT id, workspace_id, subject_ref, subject_kind, display_name, role, status, created_at, updated_at
FROM curriculum_studio.workspace_memberships
WHERE workspace_id = $1
ORDER BY subject_ref ASC`
	rows, err := r.Q.Query(ctx, q, workspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.WorkspaceMembership
	for rows.Next() {
		var m domain.WorkspaceMembership
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.SubjectRef, &m.SubjectKind, &m.DisplayName, &m.Role, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, MapError(err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.WorkspaceMembership{}
	}
	return out, nil
}

// UpdateStatus sets membership status within workspace scope.
func (r *MembershipRepo) UpdateStatus(ctx context.Context, workspaceID, membershipID uuid.UUID, status string) (*domain.WorkspaceMembership, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if status == "" {
		return nil, fmt.Errorf("status is required")
	}
	const q = `
UPDATE curriculum_studio.workspace_memberships
SET status = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING id, workspace_id, subject_ref, subject_kind, display_name, role, status, created_at, updated_at`
	out, err := scanMembership(r.Q.QueryRow(ctx, q, membershipID, workspaceID, status))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// UpdateRole sets membership role within workspace scope.
func (r *MembershipRepo) UpdateRole(ctx context.Context, workspaceID, membershipID uuid.UUID, role string) (*domain.WorkspaceMembership, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if role == "" {
		return nil, fmt.Errorf("role is required")
	}
	const q = `
UPDATE curriculum_studio.workspace_memberships
SET role = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING id, workspace_id, subject_ref, subject_kind, display_name, role, status, created_at, updated_at`
	out, err := scanMembership(r.Q.QueryRow(ctx, q, membershipID, workspaceID, role))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func scanMembership(row pgx.Row) (*domain.WorkspaceMembership, error) {
	var m domain.WorkspaceMembership
	if err := row.Scan(&m.ID, &m.WorkspaceID, &m.SubjectRef, &m.SubjectKind, &m.DisplayName, &m.Role, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}
