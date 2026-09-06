package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/fingerprint"
)

// LearnerProfileRepo persists workspace-scoped learner and class snapshots.
type LearnerProfileRepo struct{ Q Querier }

func NewLearnerProfileRepo(q Querier) *LearnerProfileRepo { return &LearnerProfileRepo{Q: q} }

func (r *LearnerProfileRepo) Create(ctx context.Context, in *domain.LearnerProfile) (*domain.LearnerProfile, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || in.WorkspaceID == uuid.Nil || strings.TrimSpace(in.Label) == "" {
		return nil, fmt.Errorf("workspace and label are required")
	}
	profile, err := materializationObject(in.Profile, "profile")
	if err != nil {
		return nil, err
	}
	kind := in.Kind
	if kind == "" {
		kind = domain.ProfileKindLearner
	}
	const q = `INSERT INTO curriculum_studio.learner_profiles(workspace_id,kind,label,grade_band,profile,integration_identity_id)
SELECT w.id,$2,$3,$4,$5,i.id FROM curriculum_studio.workspaces w
LEFT JOIN curriculum_studio.integration_identities i ON i.id=$6 AND i.workspace_id=w.id
WHERE w.id=$1 AND ($6::uuid IS NULL OR i.id IS NOT NULL)
RETURNING id,workspace_id,kind,label,grade_band,profile,integration_identity_id,created_at,updated_at`
	out, err := scanLearnerProfile(r.Q.QueryRow(ctx, q, in.WorkspaceID, kind, strings.TrimSpace(in.Label), in.GradeBand, profile, in.IntegrationIdentityID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *LearnerProfileRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.LearnerProfile, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `SELECT id,workspace_id,kind,label,grade_band,profile,integration_identity_id,created_at,updated_at FROM curriculum_studio.learner_profiles WHERE id=$1 AND workspace_id=$2`
	out, err := scanLearnerProfile(r.Q.QueryRow(ctx, q, id, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *LearnerProfileRepo) ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.LearnerProfile, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []domain.LearnerProfile{}, nil
	}
	rows, err := r.Q.Query(ctx, `SELECT id,workspace_id,kind,label,grade_band,profile,integration_identity_id,created_at,updated_at FROM curriculum_studio.learner_profiles WHERE workspace_id=$1 ORDER BY label,id`, workspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.LearnerProfile
	for rows.Next() {
		v, e := scanLearnerProfile(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.LearnerProfile{}
	}
	return out, nil
}

// MaterializationRunRepo owns durable runs and fingerprint idempotency.
type MaterializationRunRepo struct{ Q Querier }

func NewMaterializationRunRepo(q Querier) *MaterializationRunRepo {
	return &MaterializationRunRepo{Q: q}
}

func (r *MaterializationRunRepo) Create(ctx context.Context, in *domain.MaterializationRun) (*domain.MaterializationRun, error) {
	return r.create(ctx, in, false)
}

// CreateRun is an explicit alias for Create.
func (r *MaterializationRunRepo) CreateRun(ctx context.Context, in *domain.MaterializationRun) (*domain.MaterializationRun, error) {
	return r.Create(ctx, in)
}

func (r *MaterializationRunRepo) create(ctx context.Context, in *domain.MaterializationRun, idempotent bool) (*domain.MaterializationRun, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || in.WorkspaceID == uuid.Nil || in.PlanRevisionID == uuid.Nil || strings.TrimSpace(in.InputFingerprint) == "" {
		return nil, fmt.Errorf("workspace, revision, and input fingerprint are required")
	}
	snapshot, err := materializationObject(in.InputSnapshot, "input_snapshot")
	if err != nil {
		return nil, err
	}
	if expected, e := fingerprint.Hash(snapshot); e != nil || expected != in.InputFingerprint {
		return nil, fmt.Errorf("%w: input fingerprint does not match input snapshot", ErrConflict)
	}
	if in.WindowStart != nil && in.WindowEnd != nil && in.WindowEnd.Before(*in.WindowStart) {
		return nil, fmt.Errorf("window_end must not precede window_start")
	}
	if !idempotent {
		const q = `INSERT INTO curriculum_studio.materialization_runs(workspace_id,plan_revision_id,learner_profile_id,window_start,window_end,status,input_snapshot,input_fingerprint,requested_by_subject_ref)
SELECT c.workspace_id,$2,$3,$4,$5,$6,$7,$8,$9 FROM curriculum_studio.curricula c JOIN curriculum_studio.plan_revisions r ON r.curriculum_id=c.id
WHERE c.workspace_id=$1 AND r.id=$2 AND ($3::uuid IS NULL OR EXISTS (SELECT 1 FROM curriculum_studio.learner_profiles p WHERE p.id=$3 AND p.workspace_id=$1))
RETURNING id,workspace_id,plan_revision_id,learner_profile_id,window_start,window_end,status,input_snapshot,input_fingerprint,requested_by_subject_ref,started_at,completed_at,created_at,updated_at`
		out, err := scanMaterializationRun(r.Q.QueryRow(ctx, q, in.WorkspaceID, in.PlanRevisionID, in.LearnerProfileID, in.WindowStart, in.WindowEnd, runStatus(in.Status), snapshot, in.InputFingerprint, in.RequestedBySubjectRef))
		if err != nil {
			return nil, MapError(err)
		}
		return out, nil
	}
	var out *domain.MaterializationRun
	err = WithTx(ctx, r.Q, func(q Querier) error {
		const qInsert = `INSERT INTO curriculum_studio.materialization_runs(workspace_id,plan_revision_id,learner_profile_id,window_start,window_end,status,input_snapshot,input_fingerprint,requested_by_subject_ref)
SELECT c.workspace_id,$2,$3,$4,$5,$6,$7,$8,$9 FROM curriculum_studio.curricula c JOIN curriculum_studio.plan_revisions r ON r.curriculum_id=c.id
WHERE c.workspace_id=$1 AND r.id=$2 AND ($3::uuid IS NULL OR EXISTS (SELECT 1 FROM curriculum_studio.learner_profiles p WHERE p.id=$3 AND p.workspace_id=$1)
) ON CONFLICT (plan_revision_id,input_fingerprint) DO NOTHING
RETURNING id,workspace_id,plan_revision_id,learner_profile_id,window_start,window_end,status,input_snapshot,input_fingerprint,requested_by_subject_ref,started_at,completed_at,created_at,updated_at`
		var e error
		out, e = scanMaterializationRun(q.QueryRow(ctx, qInsert, in.WorkspaceID, in.PlanRevisionID, in.LearnerProfileID, in.WindowStart, in.WindowEnd, runStatus(in.Status), snapshot, in.InputFingerprint, in.RequestedBySubjectRef))
		if e == nil {
			return nil
		}
		if !isNoRows(e) {
			return e
		}
		const qExistingSafe = `SELECT m.id,m.workspace_id,m.plan_revision_id,m.learner_profile_id,m.window_start,m.window_end,m.status,m.input_snapshot,m.input_fingerprint,m.requested_by_subject_ref,m.started_at,m.completed_at,m.created_at,m.updated_at
FROM curriculum_studio.materialization_runs m JOIN curriculum_studio.plan_revisions r ON r.id=m.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id
WHERE m.plan_revision_id=$1 AND m.input_fingerprint=$2 AND m.workspace_id=$3 FOR UPDATE`
		out, e = scanMaterializationRun(q.QueryRow(ctx, qExistingSafe, in.PlanRevisionID, in.InputFingerprint, in.WorkspaceID))
		return e
	})
	if err != nil {
		return nil, MapError(err)
	}
	canonicalExisting, err := fingerprint.Hash(out.InputSnapshot)
	if err != nil {
		return nil, err
	}
	if canonicalExisting != in.InputFingerprint {
		return nil, fmt.Errorf("%w: existing run has a different input snapshot", ErrConflict)
	}
	return out, nil
}

// CreateRunIdempotent returns the existing run for an equivalent fingerprint,
// including when two callers race at the unique index.
func (r *MaterializationRunRepo) CreateRunIdempotent(ctx context.Context, in *domain.MaterializationRun) (*domain.MaterializationRun, error) {
	return r.create(ctx, in, true)
}

func (r *MaterializationRunRepo) Get(ctx context.Context, workspaceID, id uuid.UUID) (*domain.MaterializationRun, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	out, err := scanMaterializationRun(r.Q.QueryRow(ctx, materializationSelect+` WHERE m.id=$1 AND m.workspace_id=$2`, id, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *MaterializationRunRepo) ListByRevision(ctx context.Context, workspaceID, revisionID uuid.UUID) ([]domain.MaterializationRun, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || revisionID == uuid.Nil {
		return []domain.MaterializationRun{}, nil
	}
	rows, err := r.Q.Query(ctx, materializationSelect+` WHERE m.workspace_id=$1 AND m.plan_revision_id=$2 ORDER BY m.created_at,m.id`, workspaceID, revisionID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.MaterializationRun
	for rows.Next() {
		v, e := scanMaterializationRun(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.MaterializationRun{}
	}
	return out, nil
}

func (r *MaterializationRunRepo) ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.MaterializationRun, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []domain.MaterializationRun{}, nil
	}
	rows, err := r.Q.Query(ctx, materializationSelect+` WHERE m.workspace_id=$1 ORDER BY m.created_at,m.id`, workspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.MaterializationRun
	for rows.Next() {
		v, e := scanMaterializationRun(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.MaterializationRun{}
	}
	return out, nil
}

func (r *MaterializationRunRepo) Start(ctx context.Context, workspaceID, id uuid.UUID) (*domain.MaterializationRun, error) {
	return r.transition(ctx, workspaceID, id, domain.MaterializationStatusRunning)
}
func (r *MaterializationRunRepo) Ready(ctx context.Context, workspaceID, id uuid.UUID) (*domain.MaterializationRun, error) {
	return r.transition(ctx, workspaceID, id, domain.MaterializationStatusReady)
}
func (r *MaterializationRunRepo) Fail(ctx context.Context, workspaceID, id uuid.UUID) (*domain.MaterializationRun, error) {
	return r.transition(ctx, workspaceID, id, domain.MaterializationStatusFailed)
}
func (r *MaterializationRunRepo) Cancel(ctx context.Context, workspaceID, id uuid.UUID) (*domain.MaterializationRun, error) {
	return r.transition(ctx, workspaceID, id, domain.MaterializationStatusCancelled)
}

func (r *MaterializationRunRepo) transition(ctx context.Context, workspaceID, id uuid.UUID, next string) (*domain.MaterializationRun, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || id == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	var current string
	if err := r.Q.QueryRow(ctx, `SELECT status FROM curriculum_studio.materialization_runs WHERE id=$1 AND workspace_id=$2`, id, workspaceID).Scan(&current); err != nil {
		return nil, MapError(err)
	}
	allowed := false
	switch next {
	case domain.MaterializationStatusRunning:
		allowed = current == domain.MaterializationStatusRequested || current == domain.MaterializationStatusFailed
	case domain.MaterializationStatusReady, domain.MaterializationStatusFailed, domain.MaterializationStatusCancelled:
		allowed = current == domain.MaterializationStatusRunning
	}
	if !allowed {
		return nil, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current, next)
	}
	var q string
	if next == domain.MaterializationStatusRunning {
		q = `UPDATE curriculum_studio.materialization_runs m SET status=$3,started_at=COALESCE(started_at,now()),completed_at=NULL,updated_at=now() WHERE m.id=$1 AND m.workspace_id=$2 RETURNING m.id,m.workspace_id,m.plan_revision_id,m.learner_profile_id,m.window_start,m.window_end,m.status,m.input_snapshot,m.input_fingerprint,m.requested_by_subject_ref,m.started_at,m.completed_at,m.created_at,m.updated_at`
	} else {
		q = `UPDATE curriculum_studio.materialization_runs m SET status=$3,completed_at=now(),updated_at=now() WHERE m.id=$1 AND m.workspace_id=$2 RETURNING m.id,m.workspace_id,m.plan_revision_id,m.learner_profile_id,m.window_start,m.window_end,m.status,m.input_snapshot,m.input_fingerprint,m.requested_by_subject_ref,m.started_at,m.completed_at,m.created_at,m.updated_at`
	}
	out, err := scanMaterializationRun(r.Q.QueryRow(ctx, q, id, workspaceID, next))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

const materializationSelect = `SELECT m.id,m.workspace_id,m.plan_revision_id,m.learner_profile_id,m.window_start,m.window_end,m.status,m.input_snapshot,m.input_fingerprint,m.requested_by_subject_ref,m.started_at,m.completed_at,m.created_at,m.updated_at FROM curriculum_studio.materialization_runs m`

func runStatus(status string) string {
	if status == "" {
		return domain.MaterializationStatusRequested
	}
	return status
}
func materializationObject(raw json.RawMessage, name string) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be JSON: %w", name, err)
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	return raw, nil
}

type materializationScanner interface{ Scan(...any) error }

func scanLearnerProfile(s materializationScanner) (*domain.LearnerProfile, error) {
	v := new(domain.LearnerProfile)
	e := s.Scan(&v.ID, &v.WorkspaceID, &v.Kind, &v.Label, &v.GradeBand, &v.Profile, &v.IntegrationIdentityID, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanMaterializationRun(s materializationScanner) (*domain.MaterializationRun, error) {
	v := new(domain.MaterializationRun)
	e := s.Scan(&v.ID, &v.WorkspaceID, &v.PlanRevisionID, &v.LearnerProfileID, &v.WindowStart, &v.WindowEnd, &v.Status, &v.InputSnapshot, &v.InputFingerprint, &v.RequestedBySubjectRef, &v.StartedAt, &v.CompletedAt, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
