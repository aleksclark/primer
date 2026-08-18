package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// ExportRepo persists export-job metadata and object-store references only.
type ExportRepo struct{ Q Querier }

func NewExportRepo(q Querier) *ExportRepo { return &ExportRepo{Q: q} }

func (r *ExportRepo) Create(ctx context.Context, in *domain.Export) (*domain.Export, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || in.WorkspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace is required")
	}
	format := in.Format
	if format == "" {
		return nil, fmt.Errorf("format is required")
	}
	status := in.Status
	if status == "" {
		status = domain.ExportStatusRequested
	}
	const q = `INSERT INTO curriculum_studio.exports(workspace_id,run_id,plan_revision_id,format,status,requested_by_subject_ref) SELECT $1,$2,$3,$4,$5,$6 WHERE ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM curriculum_studio.materialization_runs m WHERE m.id=$2 AND m.workspace_id=$1)) AND ($3::uuid IS NULL OR EXISTS (SELECT 1 FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$3 AND c.workspace_id=$1)) RETURNING id,workspace_id,run_id,plan_revision_id,format,status,artifact_ref,checksum,requested_by_subject_ref,created_at,completed_at`
	out, e := scanExport(r.Q.QueryRow(ctx, q, in.WorkspaceID, in.RunID, in.PlanRevisionID, format, status, in.RequestedBySubjectRef))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}
func (r *ExportRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Export, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	out, e := scanExport(r.Q.QueryRow(ctx, `SELECT id,workspace_id,run_id,plan_revision_id,format,status,artifact_ref,checksum,requested_by_subject_ref,created_at,completed_at FROM curriculum_studio.exports WHERE id=$1 AND workspace_id=$2`, id, workspaceID))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}
func (r *ExportRepo) ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.Export, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []domain.Export{}, nil
	}
	rows, e := r.Q.Query(ctx, `SELECT id,workspace_id,run_id,plan_revision_id,format,status,artifact_ref,checksum,requested_by_subject_ref,created_at,completed_at FROM curriculum_studio.exports WHERE workspace_id=$1 ORDER BY created_at,id`, workspaceID)
	if e != nil {
		return nil, MapError(e)
	}
	defer rows.Close()
	var out []domain.Export
	for rows.Next() {
		v, e := scanExport(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.Export{}
	}
	return out, nil
}
func (r *ExportRepo) Complete(ctx context.Context, workspaceID, id uuid.UUID, artifactRef, checksum string) (*domain.Export, error) {
	if strings.TrimSpace(artifactRef) == "" || strings.TrimSpace(checksum) == "" {
		return nil, fmt.Errorf("artifact_ref and checksum are required")
	}
	return r.finish(ctx, workspaceID, id, domain.ExportStatusReady, artifactRef, checksum)
}
func (r *ExportRepo) Fail(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Export, error) {
	return r.finish(ctx, workspaceID, id, domain.ExportStatusFailed, "", "")
}
func (r *ExportRepo) finish(ctx context.Context, workspaceID, id uuid.UUID, status, artifactRef, checksum string) (*domain.Export, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	out, e := scanExport(r.Q.QueryRow(ctx, `UPDATE curriculum_studio.exports SET status=$3,artifact_ref=$4,checksum=$5,completed_at=now() WHERE id=$1 AND workspace_id=$2 AND status='requested' RETURNING id,workspace_id,run_id,plan_revision_id,format,status,artifact_ref,checksum,requested_by_subject_ref,created_at,completed_at`, id, workspaceID, status, artifactRef, checksum))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}

func scanExport(s interface{ Scan(...any) error }) (*domain.Export, error) {
	v := new(domain.Export)
	e := s.Scan(&v.ID, &v.WorkspaceID, &v.RunID, &v.PlanRevisionID, &v.Format, &v.Status, &v.ArtifactRef, &v.Checksum, &v.RequestedBySubjectRef, &v.CreatedAt, &v.CompletedAt)
	return v, e
}
