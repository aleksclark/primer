package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// MaterializedItemRepo persists generated items and their lifecycle.
type MaterializedItemRepo struct{ Q Querier }

func NewMaterializedItemRepo(q Querier) *MaterializedItemRepo { return &MaterializedItemRepo{Q: q} }

func (r *MaterializedItemRepo) Create(ctx context.Context, workspaceID uuid.UUID, in *domain.MaterializedItem) (*domain.MaterializedItem, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || workspaceID == uuid.Nil || in.RunID == uuid.Nil || in.PlanRevisionID == uuid.Nil || strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("workspace, run, revision, and title are required")
	}
	body, e := materializationObject(in.Body, "body")
	if e != nil {
		return nil, e
	}
	prov, e := materializationObject(in.Provenance, "provenance")
	if e != nil {
		return nil, e
	}
	status := in.Status
	if status == "" {
		status = domain.ItemStatusDraft
	}
	kind := in.Kind
	if kind == "" {
		kind = domain.ItemKindLesson
	}
	const q = `INSERT INTO curriculum_studio.materialized_items(run_id,plan_revision_id,unit_id,project_id,outcome_id,kind,title,body,status,supersedes_item_id,provenance)
SELECT r.id,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11 FROM curriculum_studio.materialization_runs r
JOIN curriculum_studio.curricula c ON c.workspace_id=r.workspace_id AND c.id=(SELECT curriculum_id FROM curriculum_studio.plan_revisions WHERE id=r.plan_revision_id)
WHERE r.id=$1 AND r.plan_revision_id=$2 AND r.workspace_id=$12
RETURNING id,run_id,plan_revision_id,unit_id,project_id,outcome_id,kind,title,body,status,locked,locked_at,locked_by_subject_ref,supersedes_item_id,provenance,created_at,updated_at`
	out, e := scanMaterializedItem(r.Q.QueryRow(ctx, q, in.RunID, in.PlanRevisionID, in.UnitID, in.ProjectID, in.OutcomeID, kind, in.Title, body, status, in.SupersedesItemID, prov, workspaceID))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}

// CreateForStage creates an item exactly once for the (run, stage, kind, title)
// provenance tuple. The matching partial unique index makes resume safe even if
// a process dies after writing items and before completing the stage.
func (r *MaterializedItemRepo) CreateForStage(ctx context.Context, workspaceID, stageID uuid.UUID, in *domain.MaterializedItem) (*domain.MaterializedItem, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || workspaceID == uuid.Nil || stageID == uuid.Nil || in.RunID == uuid.Nil || in.PlanRevisionID == uuid.Nil || strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("workspace, stage, run, revision, and title are required")
	}
	body, err := materializationObject(in.Body, "body")
	if err != nil {
		return nil, err
	}
	provenance, err := materializationObject(in.Provenance, "provenance")
	if err != nil {
		return nil, err
	}
	var provenanceObject map[string]any
	if err := json.Unmarshal(provenance, &provenanceObject); err != nil || provenanceObject["stage_id"] != stageID.String() {
		return nil, fmt.Errorf("provenance stage_id must match stage")
	}
	status, kind := in.Status, in.Kind
	if status == "" {
		status = domain.ItemStatusDraft
	}
	if kind == "" {
		kind = domain.ItemKindLesson
	}
	const insert = `INSERT INTO curriculum_studio.materialized_items(run_id,plan_revision_id,unit_id,project_id,outcome_id,kind,title,body,status,supersedes_item_id,provenance)
SELECT r.id,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11 FROM curriculum_studio.materialization_runs r
JOIN curriculum_studio.curricula c ON c.workspace_id=r.workspace_id AND c.id=(SELECT curriculum_id FROM curriculum_studio.plan_revisions WHERE id=r.plan_revision_id)
WHERE r.id=$1 AND r.plan_revision_id=$2 AND r.workspace_id=$12
ON CONFLICT (run_id, (provenance->>'stage_id'), kind, title) WHERE provenance ? 'stage_id' DO NOTHING
RETURNING id,run_id,plan_revision_id,unit_id,project_id,outcome_id,kind,title,body,status,locked,locked_at,locked_by_subject_ref,supersedes_item_id,provenance,created_at,updated_at`
	out, err := scanMaterializedItem(r.Q.QueryRow(ctx, insert, in.RunID, in.PlanRevisionID, in.UnitID, in.ProjectID, in.OutcomeID, kind, in.Title, body, status, in.SupersedesItemID, provenance, workspaceID))
	if err == nil {
		return out, nil
	}
	if !isNoRows(err) {
		return nil, MapError(err)
	}
	out, err = scanMaterializedItem(r.Q.QueryRow(ctx, materializedItemSelect+` WHERE i.run_id=$1 AND m.workspace_id=$2 AND i.kind=$3 AND i.title=$4 AND i.provenance->>'stage_id'=$5`, in.RunID, workspaceID, kind, in.Title, stageID.String()))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *MaterializedItemRepo) Get(ctx context.Context, workspaceID, itemID uuid.UUID) (*domain.MaterializedItem, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || itemID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	out, e := scanMaterializedItem(r.Q.QueryRow(ctx, materializedItemSelect+` WHERE i.id=$1 AND m.workspace_id=$2`, itemID, workspaceID))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}
func (r *MaterializedItemRepo) ListByRun(ctx context.Context, workspaceID, runID uuid.UUID) ([]domain.MaterializedItem, error) {
	return r.ListByRunFiltered(ctx, workspaceID, runID, "", "")
}

func (r *MaterializedItemRepo) ListByRunFiltered(ctx context.Context, workspaceID, runID uuid.UUID, kind, status string) ([]domain.MaterializedItem, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || runID == uuid.Nil {
		return []domain.MaterializedItem{}, nil
	}
	rows, e := r.Q.Query(ctx, materializedItemSelect+` WHERE i.run_id=$1 AND m.workspace_id=$2 AND ($3='' OR i.kind=$3) AND ($4='' OR i.status=$4) ORDER BY i.created_at,i.id`, runID, workspaceID, strings.TrimSpace(kind), strings.TrimSpace(status))
	if e != nil {
		return nil, MapError(e)
	}
	defer rows.Close()
	var out []domain.MaterializedItem
	for rows.Next() {
		v, e := scanMaterializedItem(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.MaterializedItem{}
	}
	return out, nil
}

func (r *MaterializedItemRepo) Lock(ctx context.Context, workspaceID, itemID uuid.UUID, subjectRef string) (*domain.MaterializedItem, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if strings.TrimSpace(subjectRef) == "" {
		return nil, fmt.Errorf("subject_ref is required")
	}
	out, e := scanMaterializedItem(r.Q.QueryRow(ctx, materializedItemSelect+` WHERE i.id=$1 AND m.workspace_id=$2`, itemID, workspaceID))
	if e != nil {
		return nil, MapError(e)
	}
	if out.Locked {
		return nil, fmt.Errorf("%w", ErrLocked)
	}
	out, e = scanMaterializedItem(r.Q.QueryRow(ctx, `UPDATE curriculum_studio.materialized_items i SET locked=true,locked_at=now(),locked_by_subject_ref=$3,updated_at=now() FROM curriculum_studio.materialization_runs m WHERE i.id=$1 AND i.run_id=m.id AND m.workspace_id=$2 RETURNING i.id,i.run_id,i.plan_revision_id,i.unit_id,i.project_id,i.outcome_id,i.kind,i.title,i.body,i.status,i.locked,i.locked_at,i.locked_by_subject_ref,i.supersedes_item_id,i.provenance,i.created_at,i.updated_at`, itemID, workspaceID, subjectRef))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}
func (r *MaterializedItemRepo) Unlock(ctx context.Context, workspaceID, itemID uuid.UUID) (*domain.MaterializedItem, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	out, e := scanMaterializedItem(r.Q.QueryRow(ctx, `UPDATE curriculum_studio.materialized_items i SET locked=false,locked_at=NULL,locked_by_subject_ref='',updated_at=now() FROM curriculum_studio.materialization_runs m WHERE i.id=$1 AND i.run_id=m.id AND m.workspace_id=$2 AND i.locked=true RETURNING i.id,i.run_id,i.plan_revision_id,i.unit_id,i.project_id,i.outcome_id,i.kind,i.title,i.body,i.status,i.locked,i.locked_at,i.locked_by_subject_ref,i.supersedes_item_id,i.provenance,i.created_at,i.updated_at`, itemID, workspaceID))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}
func (r *MaterializedItemRepo) ApplyEdit(ctx context.Context, workspaceID, itemID uuid.UUID, edit *domain.MaterializedItemEdit) (*domain.MaterializedItemEdit, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if edit == nil {
		return nil, fmt.Errorf("edit is required")
	}
	patch, e := materializationObject(edit.Patch, "patch")
	if e != nil {
		return nil, e
	}
	out := *edit
	const q = `INSERT INTO curriculum_studio.materialized_item_edits(item_id,editor_subject_ref,patch) SELECT i.id,$3,$4 FROM curriculum_studio.materialized_items i JOIN curriculum_studio.materialization_runs m ON m.id=i.run_id WHERE i.id=$1 AND m.workspace_id=$2 RETURNING id,item_id,editor_subject_ref,patch,created_at`
	e = r.Q.QueryRow(ctx, q, itemID, workspaceID, edit.EditorSubjectRef, patch).Scan(&out.ID, &out.ItemID, &out.EditorSubjectRef, &out.Patch, &out.CreatedAt)
	if e != nil {
		return nil, MapError(e)
	}
	return &out, nil
}
func (r *MaterializedItemRepo) ListEdits(ctx context.Context, workspaceID, itemID uuid.UUID) ([]domain.MaterializedItemEdit, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	rows, e := r.Q.Query(ctx, `SELECT e.id,e.item_id,e.editor_subject_ref,e.patch,e.created_at FROM curriculum_studio.materialized_item_edits e JOIN curriculum_studio.materialized_items i ON i.id=e.item_id JOIN curriculum_studio.materialization_runs m ON m.id=i.run_id WHERE e.item_id=$1 AND m.workspace_id=$2 ORDER BY e.created_at,e.id`, itemID, workspaceID)
	if e != nil {
		return nil, MapError(e)
	}
	defer rows.Close()
	var out []domain.MaterializedItemEdit
	for rows.Next() {
		var v domain.MaterializedItemEdit
		if e := rows.Scan(&v.ID, &v.ItemID, &v.EditorSubjectRef, &v.Patch, &v.CreatedAt); e != nil {
			return nil, MapError(e)
		}
		out = append(out, v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.MaterializedItemEdit{}
	}
	return out, nil
}

func (r *MaterializedItemRepo) UpdateContent(ctx context.Context, workspaceID, itemID uuid.UUID, title string, body json.RawMessage, editor string) (*domain.MaterializedItem, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	body, err := materializationObject(body, "body")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	out, e := scanMaterializedItem(r.Q.QueryRow(ctx, `UPDATE curriculum_studio.materialized_items i SET title=$3,body=$4,updated_at=now() FROM curriculum_studio.materialization_runs m WHERE i.id=$1 AND i.run_id=m.id AND m.workspace_id=$2 RETURNING i.id,i.run_id,i.plan_revision_id,i.unit_id,i.project_id,i.outcome_id,i.kind,i.title,i.body,i.status,i.locked,i.locked_at,i.locked_by_subject_ref,i.supersedes_item_id,i.provenance,i.created_at,i.updated_at`, itemID, workspaceID, strings.TrimSpace(title), body))
	if e != nil {
		return nil, MapError(e)
	}
	if strings.TrimSpace(editor) != "" {
		if _, e = r.ApplyEdit(ctx, workspaceID, itemID, &domain.MaterializedItemEdit{EditorSubjectRef: editor, Patch: json.RawMessage(`{"title":true}`)}); e != nil {
			return nil, e
		}
	}
	return out, nil
}

func (r *MaterializedItemRepo) Publish(ctx context.Context, workspaceID, itemID uuid.UUID) (*domain.MaterializedItem, error) {
	return r.updateStatus(ctx, workspaceID, itemID, domain.ItemStatusPublished)
}
func (r *MaterializedItemRepo) updateStatus(ctx context.Context, workspaceID, itemID uuid.UUID, status string) (*domain.MaterializedItem, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	out, e := scanMaterializedItem(r.Q.QueryRow(ctx, `UPDATE curriculum_studio.materialized_items i SET status=$3,updated_at=now() FROM curriculum_studio.materialization_runs m WHERE i.id=$1 AND i.run_id=m.id AND m.workspace_id=$2 RETURNING i.id,i.run_id,i.plan_revision_id,i.unit_id,i.project_id,i.outcome_id,i.kind,i.title,i.body,i.status,i.locked,i.locked_at,i.locked_by_subject_ref,i.supersedes_item_id,i.provenance,i.created_at,i.updated_at`, itemID, workspaceID, status))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}

// Supersede creates replacement and marks the prior item superseded in one UoW.
func (r *MaterializedItemRepo) Supersede(ctx context.Context, workspaceID, priorID uuid.UUID, replacement *domain.MaterializedItem) (*domain.MaterializedItem, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if replacement == nil {
		return nil, fmt.Errorf("replacement is required")
	}
	replacement.SupersedesItemID = &priorID
	var out *domain.MaterializedItem
	err := WithTx(ctx, r.Q, func(q Querier) error {
		repo := NewMaterializedItemRepo(q)
		var e error
		out, e = repo.Create(ctx, workspaceID, replacement)
		if e != nil {
			return e
		}
		_, e = q.Exec(ctx, `UPDATE curriculum_studio.materialized_items i SET status='superseded',updated_at=now() FROM curriculum_studio.materialization_runs m WHERE i.id=$1 AND i.run_id=m.id AND m.workspace_id=$2`, priorID, workspaceID)
		return e
	})
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// AssessmentSupportRepo persists rubric/answer-key links checked by DB triggers.
type AssessmentSupportRepo struct{ Q Querier }

func NewAssessmentSupportRepo(q Querier) *AssessmentSupportRepo { return &AssessmentSupportRepo{Q: q} }
func (r *AssessmentSupportRepo) Link(ctx context.Context, workspaceID, assessmentID, supportID uuid.UUID) (*domain.AssessmentSupport, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	var out domain.AssessmentSupport
	e := r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.assessment_supports(assessment_item_id,support_item_id) SELECT a.id,s.id FROM curriculum_studio.materialized_items a JOIN curriculum_studio.materialization_runs am ON am.id=a.run_id JOIN curriculum_studio.materialized_items s ON s.id=$3 JOIN curriculum_studio.materialization_runs sm ON sm.id=s.run_id WHERE a.id=$1 AND s.id=$3 AND am.workspace_id=$2 AND sm.workspace_id=$2 RETURNING assessment_item_id,support_item_id`, assessmentID, workspaceID, supportID).Scan(&out.AssessmentItemID, &out.SupportItemID)
	if e != nil {
		return nil, MapError(e)
	}
	return &out, nil
}
func (r *AssessmentSupportRepo) List(ctx context.Context, workspaceID, assessmentID uuid.UUID) ([]domain.AssessmentSupport, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	rows, e := r.Q.Query(ctx, `SELECT s.assessment_item_id,s.support_item_id FROM curriculum_studio.assessment_supports s JOIN curriculum_studio.materialized_items a ON a.id=s.assessment_item_id JOIN curriculum_studio.materialization_runs m ON m.id=a.run_id WHERE s.assessment_item_id=$1 AND m.workspace_id=$2 ORDER BY s.support_item_id`, assessmentID, workspaceID)
	if e != nil {
		return nil, MapError(e)
	}
	defer rows.Close()
	var out []domain.AssessmentSupport
	for rows.Next() {
		var v domain.AssessmentSupport
		if e := rows.Scan(&v.AssessmentItemID, &v.SupportItemID); e != nil {
			return nil, MapError(e)
		}
		out = append(out, v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.AssessmentSupport{}
	}
	return out, nil
}

const materializedItemColumns = `i.id,i.run_id,i.plan_revision_id,i.unit_id,i.project_id,i.outcome_id,i.kind,i.title,i.body,i.status,i.locked,i.locked_at,i.locked_by_subject_ref,i.supersedes_item_id,i.provenance,i.created_at,i.updated_at`
const materializedItemSelect = `SELECT ` + materializedItemColumns + ` FROM curriculum_studio.materialized_items i JOIN curriculum_studio.materialization_runs m ON m.id=i.run_id`

func scanMaterializedItem(s interface{ Scan(...any) error }) (*domain.MaterializedItem, error) {
	v := new(domain.MaterializedItem)
	e := s.Scan(&v.ID, &v.RunID, &v.PlanRevisionID, &v.UnitID, &v.ProjectID, &v.OutcomeID, &v.Kind, &v.Title, &v.Body, &v.Status, &v.Locked, &v.LockedAt, &v.LockedBySubjectRef, &v.SupersedesItemID, &v.Provenance, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
