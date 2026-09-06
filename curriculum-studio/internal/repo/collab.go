package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// CommentRepo persists workspace-scoped comments on plan nodes.
type CommentRepo struct{ Q Querier }

func NewCommentRepo(q Querier) *CommentRepo { return &CommentRepo{Q: q} }

func (r *CommentRepo) Create(ctx context.Context, in *domain.PlanComment) (*domain.PlanComment, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || in.WorkspaceID == uuid.Nil || in.PlanRevisionID == uuid.Nil {
		return nil, fmt.Errorf("workspace and revision are required")
	}
	if strings.TrimSpace(in.NodeID) == "" || strings.TrimSpace(in.Body) == "" || strings.TrimSpace(in.AuthorSubjectRef) == "" {
		return nil, fmt.Errorf("node, body, and author are required")
	}
	const q = `
INSERT INTO curriculum_studio.plan_comments
    (workspace_id, plan_revision_id, node_id, author_subject_ref, author_display_name, body)
SELECT $1, r.id, $3, $4, $5, $6
FROM curriculum_studio.plan_revisions r
JOIN curriculum_studio.curricula c ON c.id = r.curriculum_id
WHERE r.id = $2 AND c.workspace_id = $1
RETURNING id, workspace_id, plan_revision_id, node_id, author_subject_ref, author_display_name, body, created_at`
	out, err := scanComment(r.Q.QueryRow(ctx, q, in.WorkspaceID, in.PlanRevisionID, strings.TrimSpace(in.NodeID), in.AuthorSubjectRef, in.AuthorDisplayName, strings.TrimSpace(in.Body)))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *CommentRepo) List(ctx context.Context, workspaceID, revisionID uuid.UUID, nodeID string) ([]domain.PlanComment, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || revisionID == uuid.Nil {
		return []domain.PlanComment{}, nil
	}
	const q = `
SELECT c.id, c.workspace_id, c.plan_revision_id, c.node_id, c.author_subject_ref, c.author_display_name, c.body, c.created_at
FROM curriculum_studio.plan_comments c
JOIN curriculum_studio.plan_revisions r ON r.id = c.plan_revision_id
JOIN curriculum_studio.curricula cur ON cur.id = r.curriculum_id
WHERE c.workspace_id = $1 AND c.plan_revision_id = $2 AND cur.workspace_id = $1
  AND ($3 = '' OR c.node_id = $3)
ORDER BY c.created_at, c.id`
	rows, err := r.Q.Query(ctx, q, workspaceID, revisionID, strings.TrimSpace(nodeID))
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	out := []domain.PlanComment{}
	for rows.Next() {
		v, e := scanComment(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

type commentScanner interface{ Scan(...any) error }

func scanComment(s commentScanner) (*domain.PlanComment, error) {
	v := new(domain.PlanComment)
	err := s.Scan(&v.ID, &v.WorkspaceID, &v.PlanRevisionID, &v.NodeID, &v.AuthorSubjectRef, &v.AuthorDisplayName, &v.Body, &v.CreatedAt)
	return v, err
}

// ApprovalRepo records reviewer decisions on draft revisions.
type ApprovalRepo struct{ Q Querier }

func NewApprovalRepo(q Querier) *ApprovalRepo { return &ApprovalRepo{Q: q} }

func (r *ApprovalRepo) Create(ctx context.Context, in *domain.PlanApproval) (*domain.PlanApproval, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || in.WorkspaceID == uuid.Nil || in.PlanRevisionID == uuid.Nil || strings.TrimSpace(in.ReviewerSubjectRef) == "" {
		return nil, fmt.Errorf("workspace, revision, and reviewer are required")
	}
	status := in.Status
	if status == "" {
		status = domain.ApprovalApproved
	}
	return r.Decide(ctx, in.WorkspaceID, in.PlanRevisionID, in.ReviewerSubjectRef, status, in.ContentFingerprint)
}

func (r *ApprovalRepo) GetByRevision(ctx context.Context, workspaceID, revisionID uuid.UUID) (*domain.PlanApproval, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	const q = `
SELECT a.id, a.workspace_id, a.plan_revision_id, a.reviewer_subject_ref, a.reviewer_display_name, a.status, a.content_fingerprint, a.created_at
FROM curriculum_studio.plan_approvals a
JOIN curriculum_studio.plan_revisions r ON r.id = a.plan_revision_id
JOIN curriculum_studio.curricula c ON c.id = r.curriculum_id
WHERE a.plan_revision_id = $2 AND a.workspace_id = $1 AND c.workspace_id = $1`
	out, err := scanApproval(r.Q.QueryRow(ctx, q, workspaceID, revisionID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

type approvalScanner interface{ Scan(...any) error }

func scanApproval(s approvalScanner) (*domain.PlanApproval, error) {
	v := new(domain.PlanApproval)
	err := s.Scan(&v.ID, &v.WorkspaceID, &v.PlanRevisionID, &v.ReviewerSubjectRef, &v.ReviewerDisplayName, &v.Status, &v.ContentFingerprint, &v.CreatedAt)
	return v, err
}

// ShareRepo persists workspace-to-workspace read-only curriculum grants.
type ShareRepo struct{ Q Querier }

func NewShareRepo(q Querier) *ShareRepo { return &ShareRepo{Q: q} }

func (r *ShareRepo) Create(ctx context.Context, in *domain.CurriculumShare) (*domain.CurriculumShare, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || in.CurriculumID == uuid.Nil || in.SourceWorkspaceID == uuid.Nil || in.TargetWorkspaceID == uuid.Nil {
		return nil, fmt.Errorf("curriculum and workspaces are required")
	}
	if in.SourceWorkspaceID == in.TargetWorkspaceID {
		return nil, fmt.Errorf("%w: cannot share to the same workspace", ErrCheckViolation)
	}
	perm := in.Permission
	if perm == "" {
		perm = domain.SharePermissionRead
	}
	const q = `
INSERT INTO curriculum_studio.curriculum_shares
    (curriculum_id, source_workspace_id, target_workspace_id, permission, created_by_subject_ref)
SELECT c.id, c.workspace_id, $3, $4, $5
FROM curriculum_studio.curricula c
WHERE c.id = $1 AND c.workspace_id = $2
ON CONFLICT (curriculum_id, target_workspace_id) DO UPDATE SET permission=EXCLUDED.permission
RETURNING id, curriculum_id, source_workspace_id, target_workspace_id, permission, created_by_subject_ref, created_at`
	out, err := scanShare(r.Q.QueryRow(ctx, q, in.CurriculumID, in.SourceWorkspaceID, in.TargetWorkspaceID, perm, in.CreatedBySubjectRef))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *ShareRepo) Get(ctx context.Context, targetWorkspaceID, curriculumID uuid.UUID) (*domain.CurriculumShare, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	const q = `
SELECT id, curriculum_id, source_workspace_id, target_workspace_id, permission, created_by_subject_ref, created_at
FROM curriculum_studio.curriculum_shares
WHERE curriculum_id = $1 AND target_workspace_id = $2`
	out, err := scanShare(r.Q.QueryRow(ctx, q, curriculumID, targetWorkspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *ShareRepo) ListForTarget(ctx context.Context, targetWorkspaceID uuid.UUID) ([]domain.CurriculumShare, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if targetWorkspaceID == uuid.Nil {
		return []domain.CurriculumShare{}, nil
	}
	rows, err := r.Q.Query(ctx, `
SELECT id, curriculum_id, source_workspace_id, target_workspace_id, permission, created_by_subject_ref, created_at
FROM curriculum_studio.curriculum_shares
WHERE target_workspace_id = $1
ORDER BY created_at, id`, targetWorkspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	out := []domain.CurriculumShare{}
	for rows.Next() {
		v, e := scanShare(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

type shareScanner interface{ Scan(...any) error }

func scanShare(s shareScanner) (*domain.CurriculumShare, error) {
	v := new(domain.CurriculumShare)
	err := s.Scan(&v.ID, &v.CurriculumID, &v.SourceWorkspaceID, &v.TargetWorkspaceID, &v.Permission, &v.CreatedBySubjectRef, &v.CreatedAt)
	return v, err
}

// UnitLibraryRepo stores reusable unit blueprints.
type UnitLibraryRepo struct{ Q Querier }

func NewUnitLibraryRepo(q Querier) *UnitLibraryRepo { return &UnitLibraryRepo{Q: q} }

func (r *UnitLibraryRepo) Create(ctx context.Context, in *domain.UnitLibraryEntry) (*domain.UnitLibraryEntry, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || in.WorkspaceID == uuid.Nil || strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("workspace and name are required")
	}
	blueprint, err := objectJSON(in.Blueprint, "blueprint")
	if err != nil {
		return nil, err
	}
	const q = `
INSERT INTO curriculum_studio.unit_library_entries (workspace_id, name, blueprint)
VALUES ($1, $2, $3)
RETURNING id, workspace_id, name, blueprint, created_at, updated_at`
	out, err := scanUnitLibrary(r.Q.QueryRow(ctx, q, in.WorkspaceID, strings.TrimSpace(in.Name), blueprint))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *UnitLibraryRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.UnitLibraryEntry, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	out, err := scanUnitLibrary(r.Q.QueryRow(ctx, `
SELECT id, workspace_id, name, blueprint, created_at, updated_at
FROM curriculum_studio.unit_library_entries
WHERE id = $1 AND workspace_id = $2`, id, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

type unitLibraryScanner interface{ Scan(...any) error }

func scanUnitLibrary(s unitLibraryScanner) (*domain.UnitLibraryEntry, error) {
	v := new(domain.UnitLibraryEntry)
	err := s.Scan(&v.ID, &v.WorkspaceID, &v.Name, &v.Blueprint, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

// TemplateRepo reads plan templates (global plus workspace-owned).
type TemplateRepo struct{ Q Querier }

func NewTemplateRepo(q Querier) *TemplateRepo { return &TemplateRepo{Q: q} }

func (r *TemplateRepo) GetByCode(ctx context.Context, workspaceID uuid.UUID, code string) (*domain.PlanTemplate, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	out, err := scanTemplate(r.Q.QueryRow(ctx, `
SELECT id, workspace_id, code, name, brief_type, seed, created_at
FROM curriculum_studio.plan_templates
WHERE code = $1 AND (workspace_id IS NULL OR workspace_id = $2)
ORDER BY workspace_id NULLS LAST
LIMIT 1`, code, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

type templateScanner interface{ Scan(...any) error }

func scanTemplate(s templateScanner) (*domain.PlanTemplate, error) {
	v := new(domain.PlanTemplate)
	err := s.Scan(&v.ID, &v.WorkspaceID, &v.Code, &v.Name, &v.BriefType, &v.Seed, &v.CreatedAt)
	return v, err
}

// DecodeTemplateSeed unmarshals a stored template seed object.
func DecodeTemplateSeed(raw json.RawMessage) (domain.TemplateSeed, error) {
	var seed domain.TemplateSeed
	if len(raw) == 0 {
		return seed, nil
	}
	if err := json.Unmarshal(raw, &seed); err != nil {
		return seed, err
	}
	return seed, nil
}
