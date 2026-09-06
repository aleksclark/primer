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

// CurriculumRepo persists workspace-scoped curriculum identities.
type CurriculumRepo struct{ Q Querier }

func NewCurriculumRepo(q Querier) *CurriculumRepo { return &CurriculumRepo{Q: q} }

func (r *CurriculumRepo) Create(ctx context.Context, in *domain.Curriculum) (*domain.Curriculum, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || in.WorkspaceID == uuid.Nil || strings.TrimSpace(in.Slug) == "" || strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("workspace, slug, and title are required")
	}
	approach, status := in.Approach, in.Status
	if approach == "" {
		approach = "custom"
	}
	if status == "" {
		status = "draft"
	}
	metadata, err := objectJSON(in.Metadata, "metadata")
	if err != nil {
		return nil, err
	}
	const q = `
INSERT INTO curriculum_studio.curricula (workspace_id, slug, title, description, approach, grade_band, status, metadata)
SELECT w.id, $2, $3, $4, $5, $6, $7, $8
FROM curriculum_studio.workspaces w WHERE w.id=$1
RETURNING id, workspace_id, slug, title, description, approach, grade_band, status,
 current_draft_revision_id, published_revision_id, metadata, created_at, updated_at`
	out, err := scanCurriculum(r.Q.QueryRow(ctx, q, in.WorkspaceID, strings.TrimSpace(in.Slug), strings.TrimSpace(in.Title), in.Description, approach, in.GradeBand, status, metadata))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *CurriculumRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Curriculum, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `SELECT c.id,c.workspace_id,c.slug,c.title,c.description,c.approach,c.grade_band,c.status,c.current_draft_revision_id,c.published_revision_id,c.metadata,c.created_at,c.updated_at FROM curriculum_studio.curricula c WHERE c.id=$1 AND c.workspace_id=$2`
	out, err := scanCurriculum(r.Q.QueryRow(ctx, q, id, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}
func (r *CurriculumRepo) List(ctx context.Context, workspaceID uuid.UUID) ([]domain.Curriculum, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []domain.Curriculum{}, nil
	}
	rows, err := r.Q.Query(ctx, `SELECT c.id,c.workspace_id,c.slug,c.title,c.description,c.approach,c.grade_band,c.status,c.current_draft_revision_id,c.published_revision_id,c.metadata,c.created_at,c.updated_at FROM curriculum_studio.curricula c WHERE c.workspace_id=$1 ORDER BY c.slug`, workspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.Curriculum
	for rows.Next() {
		v, e := scanCurriculum(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.Curriculum{}
	}
	return out, nil
}

// UpdateCore updates only a draft curriculum. Published revisions remain governed by DB immutability triggers.
func (r *CurriculumRepo) UpdateCore(ctx context.Context, workspaceID, id uuid.UUID, in *domain.Curriculum) (*domain.Curriculum, error) {
	if in == nil {
		return nil, fmt.Errorf("curriculum is required")
	}
	metadata, err := objectJSON(in.Metadata, "metadata")
	if err != nil {
		return nil, err
	}
	const q = `UPDATE curriculum_studio.curricula SET slug=$3,title=$4,description=$5,approach=$6,grade_band=$7,metadata=$8,updated_at=now() WHERE id=$1 AND workspace_id=$2 RETURNING id,workspace_id,slug,title,description,approach,grade_band,status,current_draft_revision_id,published_revision_id,metadata,created_at,updated_at`
	v, err := scanCurriculum(r.Q.QueryRow(ctx, q, id, workspaceID, in.Slug, in.Title, in.Description, in.Approach, in.GradeBand, metadata))
	if err != nil {
		return nil, MapError(err)
	}
	return v, nil
}

// PlanRevisionRepo persists revisions and owns the transactional publication CAS.
type PlanRevisionRepo struct{ Q Querier }

func NewPlanRevisionRepo(q Querier) *PlanRevisionRepo { return &PlanRevisionRepo{Q: q} }
func (r *PlanRevisionRepo) Create(ctx context.Context, workspaceID uuid.UUID, in *domain.PlanRevision) (*domain.PlanRevision, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || workspaceID == uuid.Nil || in.CurriculumID == uuid.Nil || in.Revision < 1 || strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("workspace, curriculum, revision, and title are required")
	}
	brief, err := objectJSON(in.Brief, "brief")
	if err != nil {
		return nil, err
	}
	var out *domain.PlanRevision
	err = WithTx(ctx, r.Q, func(qr Querier) error {
		const q = `INSERT INTO curriculum_studio.plan_revisions (curriculum_id,revision,title,brief) SELECT c.id,$2,$3,$4 FROM curriculum_studio.curricula c WHERE c.id=$1 AND c.workspace_id=$5 RETURNING id,curriculum_id,revision,title,brief,status,published_at,published_by_subject_ref,created_at,updated_at`
		var e error
		out, e = scanPlanRevision(qr.QueryRow(ctx, q, in.CurriculumID, in.Revision, in.Title, brief, workspaceID))
		if e != nil {
			return e
		}
		_, e = qr.Exec(ctx, `UPDATE curriculum_studio.curricula SET current_draft_revision_id=$2,updated_at=now() WHERE id=$1 AND workspace_id=$3 AND status='draft'`, in.CurriculumID, out.ID, workspaceID)
		return e
	})
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}
func (r *PlanRevisionRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.PlanRevision, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	v, e := scanPlanRevision(r.Q.QueryRow(ctx, revisionSelect+` WHERE r.id=$1 AND c.workspace_id=$2`, id, workspaceID))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}
func (r *PlanRevisionRepo) List(ctx context.Context, workspaceID, curriculumID uuid.UUID) ([]domain.PlanRevision, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || curriculumID == uuid.Nil {
		return []domain.PlanRevision{}, nil
	}
	rows, e := r.Q.Query(ctx, revisionSelect+` WHERE r.curriculum_id=$1 AND c.workspace_id=$2 ORDER BY r.revision`, curriculumID, workspaceID)
	if e != nil {
		return nil, MapError(e)
	}
	defer rows.Close()
	var out []domain.PlanRevision
	for rows.Next() {
		v, e := scanPlanRevision(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.PlanRevision{}
	}
	return out, nil
}
func (r *PlanRevisionRepo) UpdateCore(ctx context.Context, workspaceID, id uuid.UUID, in *domain.PlanRevision) (*domain.PlanRevision, error) {
	if in == nil {
		return nil, fmt.Errorf("revision is required")
	}
	brief, e := objectJSON(in.Brief, "brief")
	if e != nil {
		return nil, e
	}
	const q = `UPDATE curriculum_studio.plan_revisions r SET title=$3,brief=$4,updated_at=now() FROM curriculum_studio.curricula c WHERE r.id=$1 AND c.id=r.curriculum_id AND c.workspace_id=$2 RETURNING r.id,r.curriculum_id,r.revision,r.title,r.brief,r.status,r.published_at,r.published_by_subject_ref,r.created_at,r.updated_at`
	out, e := scanPlanRevision(r.Q.QueryRow(ctx, q, id, workspaceID, in.Title, brief))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}

// PublishRevision atomically locks the curriculum, supersedes its prior
// published revision, publishes id, and updates the curriculum pointers.
func (r *PlanRevisionRepo) PublishRevision(ctx context.Context, id uuid.UUID, subjectRef string) error {
	if r == nil || r.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	if id == uuid.Nil || strings.TrimSpace(subjectRef) == "" {
		return fmt.Errorf("revision and subject_ref are required")
	}
	err := WithTx(ctx, r.Q, func(q Querier) error { return publishRevision(ctx, q, id, subjectRef) })
	return MapError(err)
}

// Publish is the workspace-scoped variant used by callers with an authorization boundary.
func (r *PlanRevisionRepo) Publish(ctx context.Context, workspaceID, id uuid.UUID, subjectRef string) error {
	if workspaceID == uuid.Nil {
		return fmt.Errorf("%w", ErrNotFound)
	}
	return r.publishScoped(ctx, workspaceID, id, subjectRef)
}
func (r *PlanRevisionRepo) publishScoped(ctx context.Context, workspaceID, id uuid.UUID, subjectRef string) error {
	return MapError(WithTx(ctx, r.Q, func(q Querier) error {
		var owner uuid.UUID
		if e := q.QueryRow(ctx, `SELECT c.workspace_id FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$1`, id).Scan(&owner); e != nil {
			return e
		}
		if owner != workspaceID {
			return pgx.ErrNoRows
		}
		return publishRevision(ctx, q, id, subjectRef)
	}))
}
func publishRevision(ctx context.Context, q Querier, id uuid.UUID, subjectRef string) error {
	var policyWS uuid.UUID
	if err := q.QueryRow(ctx, `SELECT c.workspace_id FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$1`, id).Scan(&policyWS); err != nil {
		return err
	}
	policy, err := lockPolicy(ctx, q, policyWS, false)
	if err != nil {
		return err
	}
	var curriculumID, workspaceID uuid.UUID
	var status string
	if e := q.QueryRow(ctx, `SELECT r.curriculum_id,c.workspace_id,r.status FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$1 FOR UPDATE OF c,r`, id).Scan(&curriculumID, &workspaceID, &status); e != nil {
		return e
	}
	if status != "draft" {
		return fmt.Errorf("%w: revision status %s", ErrInvalidTransition, status)
	}
	if policy.RequireApprovalForPublish {
		if err := requireCurrentApproval(ctx, q, workspaceID, id); err != nil {
			return err
		}
	}
	var oldID *uuid.UUID
	if e := q.QueryRow(ctx, `SELECT published_revision_id FROM curriculum_studio.curricula WHERE id=$1 FOR UPDATE`, curriculumID).Scan(&oldID); e != nil {
		return e
	}
	if oldID != nil && *oldID != uuid.Nil {
		if _, e := q.Exec(ctx, `UPDATE curriculum_studio.plan_revisions SET status='superseded',updated_at=now() WHERE id=$1 AND status='published'`, *oldID); e != nil {
			return e
		}
	}
	if _, e := q.Exec(ctx, `UPDATE curriculum_studio.plan_revisions SET status='published',published_at=now(),published_by_subject_ref=$2,updated_at=now() WHERE id=$1 AND status='draft'`, id, subjectRef); e != nil {
		return e
	}
	if _, e := q.Exec(ctx, `UPDATE curriculum_studio.curricula SET status='active',published_revision_id=$2,current_draft_revision_id=NULL,updated_at=now() WHERE id=$1`, curriculumID, id); e != nil {
		return e
	}
	payload := json.RawMessage(fmt.Sprintf(`{"revision_id":%q}`, id.String()))
	if _, e := q.Exec(ctx, `INSERT INTO curriculum_studio.outbox_events(workspace_id,event_type,aggregate_kind,aggregate_id,payload) VALUES($1,'plan_revision.published','plan_revision',$2,$3)`, workspaceID, id, payload); e != nil {
		return e
	}
	_, e := q.Exec(ctx, `INSERT INTO curriculum_studio.audit_events(workspace_id,actor_subject_ref,action,entity_kind,entity_id,before,after) VALUES($1,$2,'plan_revision.published','plan_revision',$3,'{}'::jsonb,$4)`, workspaceID, subjectRef, id, payload)
	return e
}
func (r *PlanRevisionRepo) Supersede(ctx context.Context, workspaceID, id uuid.UUID) error {
	if r == nil || r.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	return MapError(WithTx(ctx, r.Q, func(q Querier) error {
		var status string
		e := q.QueryRow(ctx, `SELECT r.status FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$1 AND c.workspace_id=$2 FOR UPDATE`, id, workspaceID).Scan(&status)
		if e != nil {
			return e
		}
		if status != "published" {
			return fmt.Errorf("%w: revision status %s", ErrInvalidTransition, status)
		}
		_, e = q.Exec(ctx, `UPDATE curriculum_studio.plan_revisions SET status='superseded',updated_at=now() WHERE id=$1 AND status='published'`, id)
		return e
	}))
}

const revisionSelect = `SELECT r.id,r.curriculum_id,r.revision,r.title,r.brief,r.status,r.published_at,r.published_by_subject_ref,r.created_at,r.updated_at FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id`

// PlanGraphRepo is the revision-scoped write/read facade for the plan graph.
type PlanGraphRepo struct{ Q Querier }

func NewPlanGraphRepo(q Querier) *PlanGraphRepo { return &PlanGraphRepo{Q: q} }
func (r *PlanGraphRepo) ensureRevision(ctx context.Context, workspaceID, revisionID uuid.UUID) error {
	if r == nil || r.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || revisionID == uuid.Nil {
		return fmt.Errorf("%w", ErrNotFound)
	}
	var one int
	if e := r.Q.QueryRow(ctx, `SELECT 1 FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$1 AND c.workspace_id=$2`, revisionID, workspaceID).Scan(&one); e != nil {
		return MapError(e)
	}
	return nil
}
func (r *PlanGraphRepo) CreateObjective(ctx context.Context, ws uuid.UUID, in *domain.Objective) (*domain.Objective, error) {
	if e := r.ensureRevision(ctx, ws, inRevision(in)); e != nil {
		return nil, e
	}
	const q = `INSERT INTO curriculum_studio.objectives(plan_revision_id,code,title,description,position) VALUES($1,$2,$3,$4,$5) RETURNING id,plan_revision_id,code,title,description,position,created_at,updated_at`
	v, e := scanObjective(r.Q.QueryRow(ctx, q, in.PlanRevisionID, in.Code, in.Title, in.Description, in.Position))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}
func inRevision(in *domain.Objective) uuid.UUID {
	if in == nil {
		return uuid.Nil
	}
	return in.PlanRevisionID
}
func (r *PlanGraphRepo) CreateOutcome(ctx context.Context, ws uuid.UUID, in *domain.Outcome) (*domain.Outcome, error) {
	if in == nil {
		return nil, fmt.Errorf("outcome is required")
	}
	if e := r.ensureRevision(ctx, ws, in.PlanRevisionID); e != nil {
		return nil, e
	}
	if in.ObjectiveID != nil {
		var one int
		if e := r.Q.QueryRow(ctx, `SELECT 1 FROM curriculum_studio.objectives WHERE id=$1 AND plan_revision_id=$2`, *in.ObjectiveID, in.PlanRevisionID).Scan(&one); e != nil {
			return nil, MapError(e)
		}
	}
	const q = `INSERT INTO curriculum_studio.outcomes(plan_revision_id,objective_id,code,title,description,mastery_criteria,position) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,plan_revision_id,objective_id,code,title,description,mastery_criteria,position,created_at,updated_at`
	v, e := scanOutcome(r.Q.QueryRow(ctx, q, in.PlanRevisionID, in.ObjectiveID, in.Code, in.Title, in.Description, in.MasteryCriteria, in.Position))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}

// CreateMapping is the concise alias for CreateOutcomeStandardMapping.
func (r *PlanGraphRepo) CreateMapping(ctx context.Context, ws uuid.UUID, in *domain.OutcomeStandardMapping) (*domain.OutcomeStandardMapping, error) {
	return r.CreateOutcomeStandardMapping(ctx, ws, in)
}

func (r *PlanGraphRepo) CreateOutcomeStandardMapping(ctx context.Context, ws uuid.UUID, in *domain.OutcomeStandardMapping) (*domain.OutcomeStandardMapping, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil {
		return nil, fmt.Errorf("mapping is required")
	}
	const q = `INSERT INTO curriculum_studio.outcome_standard_mappings(outcome_id,standard_id,alignment,notes) SELECT o.id,s.id,$3,$4 FROM curriculum_studio.outcomes o JOIN curriculum_studio.plan_revisions r ON r.id=o.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id JOIN curriculum_studio.catalog_standards s ON s.id=$2 JOIN curriculum_studio.standard_frameworks f ON f.id=s.framework_id WHERE o.id=$1 AND c.workspace_id=$5 AND (f.workspace_id IS NULL OR f.workspace_id=$5) RETURNING id,outcome_id,standard_id,alignment,notes,created_at`
	v, e := scanMappingInsert(r.Q.QueryRow(ctx, q, in.OutcomeID, in.StandardID, in.Alignment, in.Notes, ws))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}

// CreatePrerequisite is the concise alias used by graph callers.
func (r *PlanGraphRepo) CreatePrerequisite(ctx context.Context, ws uuid.UUID, in *domain.OutcomePrerequisite) (*domain.OutcomePrerequisite, error) {
	return r.CreateOutcomePrerequisite(ctx, ws, in)
}

func (r *PlanGraphRepo) CreateOutcomePrerequisite(ctx context.Context, ws uuid.UUID, in *domain.OutcomePrerequisite) (*domain.OutcomePrerequisite, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil {
		return nil, fmt.Errorf("prerequisite is required")
	}
	const q = `INSERT INTO curriculum_studio.outcome_prerequisites(plan_revision_id,outcome_id,prerequisite_id,requirement) SELECT r.id,o.id,p.id,$4 FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id JOIN curriculum_studio.outcomes o ON o.id=$2 AND o.plan_revision_id=r.id JOIN curriculum_studio.outcomes p ON p.id=$3 AND p.plan_revision_id=r.id WHERE r.id=$1 AND c.workspace_id=$5 RETURNING id,plan_revision_id,outcome_id,prerequisite_id,requirement`
	v, e := scanOutcomePrereq(r.Q.QueryRow(ctx, q, in.PlanRevisionID, in.OutcomeID, in.PrerequisiteID, in.Requirement, ws))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}

// CreateArc is the concise alias for CreateLearningArc.
func (r *PlanGraphRepo) CreateArc(ctx context.Context, ws uuid.UUID, in *domain.LearningArc) (*domain.LearningArc, error) {
	return r.CreateLearningArc(ctx, ws, in)
}

func (r *PlanGraphRepo) CreateLearningArc(ctx context.Context, ws uuid.UUID, in *domain.LearningArc) (*domain.LearningArc, error) {
	if in == nil {
		return nil, fmt.Errorf("learning arc is required")
	}
	if e := r.ensureRevision(ctx, ws, in.PlanRevisionID); e != nil {
		return nil, e
	}
	v, e := scanArc(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.learning_arcs(plan_revision_id,code,title,description,position) VALUES($1,$2,$3,$4,$5) RETURNING id,plan_revision_id,code,title,description,position,created_at,updated_at`, in.PlanRevisionID, in.Code, in.Title, in.Description, in.Position))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}
func (r *PlanGraphRepo) CreateUnit(ctx context.Context, ws uuid.UUID, in *domain.Unit) (*domain.Unit, error) {
	if in == nil {
		return nil, fmt.Errorf("unit is required")
	}
	if e := r.ensureRevision(ctx, ws, in.PlanRevisionID); e != nil {
		return nil, e
	}
	if in.LearningArcID != nil {
		var one int
		if e := r.Q.QueryRow(ctx, `SELECT 1 FROM curriculum_studio.learning_arcs WHERE id=$1 AND plan_revision_id=$2`, *in.LearningArcID, in.PlanRevisionID).Scan(&one); e != nil {
			return nil, MapError(e)
		}
	}
	bp, e := objectJSON(in.Blueprint, "blueprint")
	if e != nil {
		return nil, e
	}
	questions := in.EssentialQuestions
	if questions == nil {
		questions = []string{}
	}
	v, e := scanUnit(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.units(plan_revision_id,learning_arc_id,code,title,essential_questions,estimated_minutes,position,blueprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,plan_revision_id,learning_arc_id,code,title,essential_questions,estimated_minutes,position,blueprint,created_at,updated_at`, in.PlanRevisionID, in.LearningArcID, in.Code, in.Title, questions, in.EstimatedMinutes, in.Position, bp))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}
func (r *PlanGraphRepo) CreateProject(ctx context.Context, ws uuid.UUID, in *domain.Project) (*domain.Project, error) {
	if in == nil {
		return nil, fmt.Errorf("project is required")
	}
	if e := r.ensureRevision(ctx, ws, in.PlanRevisionID); e != nil {
		return nil, e
	}
	if in.UnitID != nil {
		var one int
		if e := r.Q.QueryRow(ctx, `SELECT 1 FROM curriculum_studio.units WHERE id=$1 AND plan_revision_id=$2`, *in.UnitID, in.PlanRevisionID).Scan(&one); e != nil {
			return nil, MapError(e)
		}
	}
	ph, e := projectPhasesJSON(in.Phases)
	if e != nil {
		return nil, e
	}
	v, e := scanProject(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.projects(plan_revision_id,unit_id,code,title,description,phases,estimated_minutes,position) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,plan_revision_id,unit_id,code,title,description,phases,estimated_minutes,position,created_at,updated_at`, in.PlanRevisionID, in.UnitID, in.Code, in.Title, in.Description, ph, in.EstimatedMinutes, in.Position))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}
func (r *PlanGraphRepo) CreateUnitOutcome(ctx context.Context, ws uuid.UUID, in *domain.UnitOutcome) (*domain.UnitOutcome, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil {
		return nil, fmt.Errorf("unit outcome is required")
	}
	var one int
	if e := r.Q.QueryRow(ctx, `SELECT 1 FROM curriculum_studio.units u JOIN curriculum_studio.plan_revisions r ON r.id=u.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id JOIN curriculum_studio.outcomes o ON o.id=$2 AND o.plan_revision_id=r.id WHERE u.id=$1 AND c.workspace_id=$3`, in.UnitID, in.OutcomeID, ws).Scan(&one); e != nil {
		return nil, MapError(e)
	}
	v := *in
	e := r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.unit_outcomes(unit_id,outcome_id,role) VALUES($1,$2,$3) RETURNING unit_id,outcome_id,role`, in.UnitID, in.OutcomeID, in.Role).Scan(&v.UnitID, &v.OutcomeID, &v.Role)
	if e != nil {
		return nil, MapError(e)
	}
	return &v, nil
}
func (r *PlanGraphRepo) CreateProjectOutcome(ctx context.Context, ws uuid.UUID, in *domain.ProjectOutcome) (*domain.ProjectOutcome, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil {
		return nil, fmt.Errorf("project outcome is required")
	}
	var one int
	if e := r.Q.QueryRow(ctx, `SELECT 1 FROM curriculum_studio.projects p JOIN curriculum_studio.plan_revisions r ON r.id=p.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id JOIN curriculum_studio.outcomes o ON o.id=$2 AND o.plan_revision_id=r.id WHERE p.id=$1 AND c.workspace_id=$3`, in.ProjectID, in.OutcomeID, ws).Scan(&one); e != nil {
		return nil, MapError(e)
	}
	v := *in
	e := r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.project_outcomes(project_id,outcome_id,role) VALUES($1,$2,$3) RETURNING project_id,outcome_id,role`, in.ProjectID, in.OutcomeID, in.Role).Scan(&v.ProjectID, &v.OutcomeID, &v.Role)
	if e != nil {
		return nil, MapError(e)
	}
	return &v, nil
}
func (r *PlanGraphRepo) CreateEvidenceRequirement(ctx context.Context, ws uuid.UUID, in *domain.EvidenceRequirement) (*domain.EvidenceRequirement, error) {
	if in == nil {
		return nil, fmt.Errorf("evidence requirement is required")
	}
	if e := r.ensureRevision(ctx, ws, in.PlanRevisionID); e != nil {
		return nil, e
	}
	c, e := objectJSON(in.Criteria, "criteria")
	if e != nil {
		return nil, e
	}
	v, e := scanEvidence(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.evidence_requirements(plan_revision_id,outcome_id,kind,description,criteria) SELECT $1,o.id,$3,$4,$5 FROM curriculum_studio.outcomes o JOIN curriculum_studio.plan_revisions r ON r.id=o.plan_revision_id JOIN curriculum_studio.curricula w ON w.id=r.curriculum_id WHERE o.id=$2 AND r.id=$1 AND w.workspace_id=$6 RETURNING id,plan_revision_id,outcome_id,kind,description,criteria,created_at`, in.PlanRevisionID, in.OutcomeID, in.Kind, in.Description, c, ws))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}
func (r *PlanGraphRepo) CreateSchedulingConstraint(ctx context.Context, ws uuid.UUID, in *domain.SchedulingConstraint) (*domain.SchedulingConstraint, error) {
	if in == nil {
		return nil, fmt.Errorf("scheduling constraint is required")
	}
	if e := r.ensureRevision(ctx, ws, in.PlanRevisionID); e != nil {
		return nil, e
	}
	p, e := objectJSON(in.Payload, "payload")
	if e != nil {
		return nil, e
	}
	v, e := scanConstraint(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.scheduling_constraints(plan_revision_id,kind,payload) VALUES($1,$2,$3) RETURNING id,plan_revision_id,kind,payload,created_at`, in.PlanRevisionID, in.Kind, p))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}
func (r *PlanGraphRepo) CreatePlanResource(ctx context.Context, ws uuid.UUID, in *domain.PlanResource) (*domain.PlanResource, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil {
		return nil, fmt.Errorf("plan resource is required")
	}
	v, e := scanPlanResourceInsert(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.plan_resources(plan_revision_id,resource_id,unit_id,project_id,role,notes) SELECT $1,res.id,$3,$4,$5,$6 FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id JOIN curriculum_studio.resources res ON res.id=$2 AND (res.workspace_id IS NULL OR res.workspace_id=$7) WHERE r.id=$1 AND c.workspace_id=$7 RETURNING id,plan_revision_id,resource_id,unit_id,project_id,role,notes`, in.PlanRevisionID, in.ResourceID, in.UnitID, in.ProjectID, in.Role, in.Notes, ws))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}

// Load returns a revision-scoped graph snapshot with bounded, table-oriented queries.
func (r *PlanGraphRepo) Load(ctx context.Context, ws, revisionID uuid.UUID) (*domain.PlanGraph, error) {
	if e := r.ensureRevision(ctx, ws, revisionID); e != nil {
		return nil, e
	}
	rev, e := NewPlanRevisionRepo(r.Q).Get(ctx, ws, revisionID)
	if e != nil {
		return nil, e
	}
	g := &domain.PlanGraph{Revision: *rev}
	g.Objectives, e = listObjectives(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.Outcomes, e = listOutcomes(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.OutcomeStandardMappings, e = listMappings(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.OutcomePrerequisites, e = listOutcomePrereqs(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.LearningArcs, e = listArcs(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.Units, e = listUnits(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.Projects, e = listProjects(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.UnitOutcomes, e = listUnitOutcomes(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.ProjectOutcomes, e = listProjectOutcomes(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.EvidenceRequirements, e = listEvidence(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.SchedulingConstraints, e = listConstraints(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	g.PlanResources, e = listPlanResources(ctx, r.Q, revisionID)
	if e != nil {
		return nil, e
	}
	return g, nil
}

func objectJSON(raw json.RawMessage, name string) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var v any
	if e := json.Unmarshal(raw, &v); e != nil {
		return nil, fmt.Errorf("%s must be JSON: %w", name, e)
	}
	if _, ok := v.(map[string]any); !ok {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	return raw, nil
}
func arrayJSON(raw json.RawMessage, name string) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = []byte(`[]`)
	}
	var v any
	if e := json.Unmarshal(raw, &v); e != nil {
		return nil, fmt.Errorf("%s must be JSON: %w", name, e)
	}
	if _, ok := v.([]any); !ok {
		return nil, fmt.Errorf("%s must be a JSON array", name)
	}
	return raw, nil
}

func projectPhasesJSON(raw json.RawMessage) (json.RawMessage, error) {
	phases, err := domain.ParseProjectPhases(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCheckViolation, err.Error())
	}
	encoded, err := json.Marshal(phases)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

type scanner interface{ Scan(...any) error }

func scanCurriculum(s scanner) (*domain.Curriculum, error) {
	v := new(domain.Curriculum)
	e := s.Scan(&v.ID, &v.WorkspaceID, &v.Slug, &v.Title, &v.Description, &v.Approach, &v.GradeBand, &v.Status, &v.CurrentDraftRevisionID, &v.PublishedRevisionID, &v.Metadata, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanPlanRevision(s scanner) (*domain.PlanRevision, error) {
	v := new(domain.PlanRevision)
	e := s.Scan(&v.ID, &v.CurriculumID, &v.Revision, &v.Title, &v.Brief, &v.Status, &v.PublishedAt, &v.PublishedBySubjectRef, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanObjective(s scanner) (*domain.Objective, error) {
	v := new(domain.Objective)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.Code, &v.Title, &v.Description, &v.Position, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanOutcome(s scanner) (*domain.Outcome, error) {
	v := new(domain.Outcome)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.ObjectiveID, &v.Code, &v.Title, &v.Description, &v.MasteryCriteria, &v.Position, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanMappingInsert(s scanner) (*domain.OutcomeStandardMapping, error) {
	v := new(domain.OutcomeStandardMapping)
	e := s.Scan(&v.ID, &v.OutcomeID, &v.StandardID, &v.Alignment, &v.Notes, &v.CreatedAt)
	return v, e
}
func scanMapping(s scanner) (*domain.OutcomeStandardMapping, error) {
	v := new(domain.OutcomeStandardMapping)
	e := s.Scan(&v.ID, &v.OutcomeID, &v.StandardID, &v.Alignment, &v.Notes, &v.CreatedAt, &v.StandardCode)
	return v, e
}
func scanOutcomePrereq(s scanner) (*domain.OutcomePrerequisite, error) {
	v := new(domain.OutcomePrerequisite)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.OutcomeID, &v.PrerequisiteID, &v.Requirement)
	return v, e
}
func scanArc(s scanner) (*domain.LearningArc, error) {
	v := new(domain.LearningArc)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.Code, &v.Title, &v.Description, &v.Position, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanUnit(s scanner) (*domain.Unit, error) {
	v := new(domain.Unit)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.LearningArcID, &v.Code, &v.Title, &v.EssentialQuestions, &v.EstimatedMinutes, &v.Position, &v.Blueprint, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanProject(s scanner) (*domain.Project, error) {
	v := new(domain.Project)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.UnitID, &v.Code, &v.Title, &v.Description, &v.Phases, &v.EstimatedMinutes, &v.Position, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanEvidence(s scanner) (*domain.EvidenceRequirement, error) {
	v := new(domain.EvidenceRequirement)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.OutcomeID, &v.Kind, &v.Description, &v.Criteria, &v.CreatedAt)
	return v, e
}
func scanConstraint(s scanner) (*domain.SchedulingConstraint, error) {
	v := new(domain.SchedulingConstraint)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.Kind, &v.Payload, &v.CreatedAt)
	return v, e
}
func scanPlanResourceInsert(s scanner) (*domain.PlanResource, error) {
	v := new(domain.PlanResource)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.ResourceID, &v.UnitID, &v.ProjectID, &v.Role, &v.Notes)
	return v, e
}
func scanPlanResource(s scanner) (*domain.PlanResource, error) {
	v := new(domain.PlanResource)
	e := s.Scan(&v.ID, &v.PlanRevisionID, &v.ResourceID, &v.UnitID, &v.ProjectID, &v.Role, &v.Notes, &v.ResourceKind, &v.ResourceTitle)
	return v, e
}

func listRows[T any](ctx context.Context, q Querier, sql string, args []any, scan func(scanner) (*T, error)) ([]T, error) {
	rows, e := q.Query(ctx, sql, args...)
	if e != nil {
		return nil, MapError(e)
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = make([]T, 0)
	}
	return out, nil
}
func listObjectives(c context.Context, q Querier, id uuid.UUID) ([]domain.Objective, error) {
	return listRows(c, q, `SELECT id,plan_revision_id,code,title,description,position,created_at,updated_at FROM curriculum_studio.objectives WHERE plan_revision_id=$1 ORDER BY position,code`, []any{id}, scanObjective)
}
func listOutcomes(c context.Context, q Querier, id uuid.UUID) ([]domain.Outcome, error) {
	return listRows(c, q, `SELECT id,plan_revision_id,objective_id,code,title,description,mastery_criteria,position,created_at,updated_at FROM curriculum_studio.outcomes WHERE plan_revision_id=$1 ORDER BY position,code`, []any{id}, scanOutcome)
}
func listMappings(c context.Context, q Querier, id uuid.UUID) ([]domain.OutcomeStandardMapping, error) {
	return listRows(c, q, `SELECT m.id,m.outcome_id,m.standard_id,m.alignment,m.notes,m.created_at,s.code FROM curriculum_studio.outcome_standard_mappings m JOIN curriculum_studio.outcomes o ON o.id=m.outcome_id JOIN curriculum_studio.catalog_standards s ON s.id=m.standard_id WHERE o.plan_revision_id=$1 ORDER BY m.id`, []any{id}, scanMapping)
}
func listOutcomePrereqs(c context.Context, q Querier, id uuid.UUID) ([]domain.OutcomePrerequisite, error) {
	return listRows(c, q, `SELECT id,plan_revision_id,outcome_id,prerequisite_id,requirement FROM curriculum_studio.outcome_prerequisites WHERE plan_revision_id=$1 ORDER BY id`, []any{id}, scanOutcomePrereq)
}
func listArcs(c context.Context, q Querier, id uuid.UUID) ([]domain.LearningArc, error) {
	return listRows(c, q, `SELECT id,plan_revision_id,code,title,description,position,created_at,updated_at FROM curriculum_studio.learning_arcs WHERE plan_revision_id=$1 ORDER BY position,code`, []any{id}, scanArc)
}
func listUnits(c context.Context, q Querier, id uuid.UUID) ([]domain.Unit, error) {
	return listRows(c, q, `SELECT id,plan_revision_id,learning_arc_id,code,title,essential_questions,estimated_minutes,position,blueprint,created_at,updated_at FROM curriculum_studio.units WHERE plan_revision_id=$1 ORDER BY position,code`, []any{id}, scanUnit)
}
func listProjects(c context.Context, q Querier, id uuid.UUID) ([]domain.Project, error) {
	return listRows(c, q, `SELECT id,plan_revision_id,unit_id,code,title,description,phases,estimated_minutes,position,created_at,updated_at FROM curriculum_studio.projects WHERE plan_revision_id=$1 ORDER BY position,code`, []any{id}, scanProject)
}
func listUnitOutcomes(c context.Context, q Querier, id uuid.UUID) ([]domain.UnitOutcome, error) {
	return listRows(c, q, `SELECT uo.unit_id,uo.outcome_id,uo.role FROM curriculum_studio.unit_outcomes uo JOIN curriculum_studio.units u ON u.id=uo.unit_id WHERE u.plan_revision_id=$1 ORDER BY uo.unit_id,uo.outcome_id`, []any{id}, func(s scanner) (*domain.UnitOutcome, error) {
		v := new(domain.UnitOutcome)
		e := s.Scan(&v.UnitID, &v.OutcomeID, &v.Role)
		return v, e
	})
}
func listProjectOutcomes(c context.Context, q Querier, id uuid.UUID) ([]domain.ProjectOutcome, error) {
	return listRows(c, q, `SELECT po.project_id,po.outcome_id,po.role FROM curriculum_studio.project_outcomes po JOIN curriculum_studio.projects p ON p.id=po.project_id WHERE p.plan_revision_id=$1 ORDER BY po.project_id,po.outcome_id`, []any{id}, func(s scanner) (*domain.ProjectOutcome, error) {
		v := new(domain.ProjectOutcome)
		e := s.Scan(&v.ProjectID, &v.OutcomeID, &v.Role)
		return v, e
	})
}
func listEvidence(c context.Context, q Querier, id uuid.UUID) ([]domain.EvidenceRequirement, error) {
	return listRows(c, q, `SELECT id,plan_revision_id,outcome_id,kind,description,criteria,created_at FROM curriculum_studio.evidence_requirements WHERE plan_revision_id=$1 ORDER BY created_at,id`, []any{id}, scanEvidence)
}
func listConstraints(c context.Context, q Querier, id uuid.UUID) ([]domain.SchedulingConstraint, error) {
	return listRows(c, q, `SELECT id,plan_revision_id,kind,payload,created_at FROM curriculum_studio.scheduling_constraints WHERE plan_revision_id=$1 ORDER BY created_at,id`, []any{id}, scanConstraint)
}
func listPlanResources(c context.Context, q Querier, id uuid.UUID) ([]domain.PlanResource, error) {
	return listRows(c, q, `SELECT pr.id,pr.plan_revision_id,pr.resource_id,pr.unit_id,pr.project_id,pr.role,pr.notes,COALESCE(res.kind,''),COALESCE(res.title,'') FROM curriculum_studio.plan_resources pr LEFT JOIN curriculum_studio.resources res ON res.id=pr.resource_id WHERE pr.plan_revision_id=$1 ORDER BY pr.id`, []any{id}, scanPlanResource)
}
