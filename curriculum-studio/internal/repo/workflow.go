package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// WorkflowRepo persists resumable stages and DB-fenced worker attempts.
type WorkflowRepo struct{ Q Querier }

func NewWorkflowRepo(q Querier) *WorkflowRepo { return &WorkflowRepo{Q: q} }

// EnsureStages creates missing stage rows without replacing an existing
// checkpoint. The whole stage set is inserted in one transaction.
func (r *WorkflowRepo) EnsureStages(ctx context.Context, workspaceID, runID uuid.UUID, specs []domain.WorkflowStageSpec) ([]domain.WorkflowStage, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || runID == uuid.Nil {
		return nil, fmt.Errorf("workspace and run are required")
	}
	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec.StageKey) == "" || spec.Position < 1 || seen[spec.StageKey] {
			return nil, fmt.Errorf("invalid or duplicate workflow stage")
		}
		seen[spec.StageKey] = true
		if _, e := workflowObject(spec.Input, "stage input"); e != nil {
			return nil, e
		}
	}
	err := WithTx(ctx, r.Q, func(q Querier) error {
		var one int
		if e := q.QueryRow(ctx, `SELECT 1 FROM curriculum_studio.materialization_runs WHERE id=$1 AND workspace_id=$2`, runID, workspaceID).Scan(&one); e != nil {
			return e
		}
		for _, spec := range specs {
			input, _ := workflowObject(spec.Input, "stage input")
			if _, e := q.Exec(ctx, `INSERT INTO curriculum_studio.workflow_stages(run_id,stage_key,position,input) VALUES($1,$2,$3,$4) ON CONFLICT (run_id,stage_key) DO NOTHING`, runID, spec.StageKey, spec.Position, input); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return nil, MapError(err)
	}
	return r.ListStages(ctx, workspaceID, runID)
}

func (r *WorkflowRepo) ListStages(ctx context.Context, workspaceID, runID uuid.UUID) ([]domain.WorkflowStage, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || runID == uuid.Nil {
		return []domain.WorkflowStage{}, nil
	}
	rows, e := r.Q.Query(ctx, workflowStageFrom+` WHERE s.run_id=$1 AND m.workspace_id=$2 ORDER BY s.position`, runID, workspaceID)
	if e != nil {
		return nil, MapError(e)
	}
	defer rows.Close()
	var out []domain.WorkflowStage
	for rows.Next() {
		v, e := scanWorkflowStage(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.WorkflowStage{}
	}
	return out, nil
}

// NextStage returns the first failed/pending stage without mutating it.
func (r *WorkflowRepo) NextStage(ctx context.Context, workspaceID, runID uuid.UUID) (*domain.WorkflowStage, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || runID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	v, e := scanWorkflowStage(r.Q.QueryRow(ctx, workflowStageFrom+` WHERE s.run_id=$1 AND m.workspace_id=$2 AND s.status IN ('failed','pending') ORDER BY s.position LIMIT 1`, runID, workspaceID))
	if e != nil {
		return nil, MapError(e)
	}
	return v, nil
}

// ClaimStage acquires/reacquires a stage and opens the next attempt while the
// stage row lock is held. A new fence token is emitted for every claim.
func (r *WorkflowRepo) ClaimStage(ctx context.Context, workspaceID, stageID uuid.UUID, owner string, leaseTTL time.Duration, agent string) (*domain.ClaimedStage, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || stageID == uuid.Nil || strings.TrimSpace(owner) == "" || leaseTTL <= 0 {
		return nil, fmt.Errorf("workspace, stage, owner, and positive lease are required")
	}
	var claimed *domain.ClaimedStage
	err := WithTx(ctx, r.Q, func(q Querier) error {
		const claim = workflowStageSelect + ` FROM curriculum_studio.workflow_stages s JOIN curriculum_studio.materialization_runs m ON m.id=s.run_id WHERE s.id=$1 AND m.workspace_id=$2 AND s.status IN ('pending','failed','running') AND (s.lease_expires_at IS NULL OR s.lease_expires_at<=now()) FOR UPDATE`
		// Lock before changing so attempt numbers are serialized per stage.
		var current *domain.WorkflowStage
		var e error
		current, e = scanWorkflowStage(q.QueryRow(ctx, claim, stageID, workspaceID))
		if e != nil {
			return e
		}
		const update = `UPDATE curriculum_studio.workflow_stages s SET status='running',lease_owner=$2,lease_expires_at=now()+($3 * interval '1 second'),fence_token=fence_token+1,started_at=COALESCE(started_at,now()),updated_at=now() WHERE s.id=$1 RETURNING ` + workflowStageColumns
		_, e = scanWorkflowStage(q.QueryRow(ctx, update, stageID, owner, leaseTTL.Seconds()))
		if e != nil {
			return e
		}
		var attempt *domain.WorkflowAttempt
		const insertAttempt = `INSERT INTO curriculum_studio.workflow_attempts(stage_id,attempt_number,agent) VALUES($1,(SELECT COALESCE(MAX(attempt_number),0)+1 FROM curriculum_studio.workflow_attempts WHERE stage_id=$1),$2) RETURNING id,stage_id,attempt_number,status,agent,error,started_at,finished_at`
		attempt, e = scanWorkflowAttempt(q.QueryRow(ctx, insertAttempt, stageID, agent))
		if e != nil {
			return e
		}
		_ = current
		// Re-read the changed row to return the fence token and lease.
		stage, e := scanWorkflowStage(q.QueryRow(ctx, workflowStageFrom+` WHERE s.id=$1 AND m.workspace_id=$2`, stageID, workspaceID))
		if e != nil {
			return e
		}
		claimed = &domain.ClaimedStage{Stage: *stage, Attempt: *attempt}
		return nil
	})
	if err != nil {
		if isLeaseNoRows(err) {
			return nil, fmt.Errorf("%w", ErrLeaseLost)
		}
		return nil, MapError(err)
	}
	return claimed, nil
}

// Checkpoint durably stores output only while the owner holds the current
// fence and an unexpired lease. A stale worker receives ErrLeaseLost.
func (r *WorkflowRepo) Checkpoint(ctx context.Context, workspaceID, stageID uuid.UUID, owner string, fenceToken int64, output json.RawMessage) error {
	if r == nil || r.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || stageID == uuid.Nil || strings.TrimSpace(owner) == "" {
		return fmt.Errorf("workspace, stage, and owner are required")
	}
	out, e := workflowObject(output, "stage output")
	if e != nil {
		return e
	}
	var updated uuid.UUID
	e = r.Q.QueryRow(ctx, `UPDATE curriculum_studio.workflow_stages s SET output=$5,updated_at=now() FROM curriculum_studio.materialization_runs m WHERE s.id=$1 AND s.run_id=m.id AND m.workspace_id=$2 AND s.lease_owner=$3 AND s.fence_token=$4 AND s.status='running' AND s.lease_expires_at>now() RETURNING s.id`, stageID, workspaceID, owner, fenceToken, out).Scan(&updated)
	if e != nil {
		if isLeaseNoRows(e) {
			return fmt.Errorf("%w", ErrLeaseLost)
		}
		return MapError(e)
	}
	return nil
}

func (r *WorkflowRepo) CompleteAttempt(ctx context.Context, workspaceID, stageID, attemptID uuid.UUID, owner string, fenceToken int64) error {
	return r.finishAttempt(ctx, workspaceID, stageID, attemptID, owner, fenceToken, domain.WorkflowAttemptSucceeded, "")
}
func (r *WorkflowRepo) FailAttempt(ctx context.Context, workspaceID, stageID, attemptID uuid.UUID, owner string, fenceToken int64, message string) error {
	return r.finishAttempt(ctx, workspaceID, stageID, attemptID, owner, fenceToken, domain.WorkflowAttemptFailed, message)
}
func (r *WorkflowRepo) finishAttempt(ctx context.Context, workspaceID, stageID, attemptID uuid.UUID, owner string, fenceToken int64, status, message string) error {
	if r == nil || r.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	err := WithTx(ctx, r.Q, func(q Querier) error {
		var updated uuid.UUID
		if e := q.QueryRow(ctx, `UPDATE curriculum_studio.workflow_attempts a SET status=$6,error=$7,finished_at=now() FROM curriculum_studio.workflow_stages s JOIN curriculum_studio.materialization_runs m ON m.id=s.run_id WHERE a.id=$1 AND a.stage_id=$2 AND s.id=a.stage_id AND m.workspace_id=$3 AND s.lease_owner=$4 AND s.fence_token=$5 AND s.status='running' AND s.lease_expires_at>now() RETURNING a.id`, attemptID, stageID, workspaceID, owner, fenceToken, status, message).Scan(&updated); e != nil {
			if isLeaseNoRows(e) {
				return fmt.Errorf("%w", ErrLeaseLost)
			}
			return e
		}
		if _, e := q.Exec(ctx, `UPDATE curriculum_studio.workflow_stages SET status=$3,lease_owner='',lease_expires_at=NULL,updated_at=now(),completed_at=CASE WHEN $3 IN ('succeeded','failed') THEN now() ELSE completed_at END WHERE id=$1 AND fence_token=$2`, stageID, fenceToken, map[string]string{domain.WorkflowAttemptSucceeded: domain.WorkflowStageSucceeded, domain.WorkflowAttemptFailed: domain.WorkflowStageFailed}[status]); e != nil {
			return e
		}
		return nil
	})
	return MapError(err)
}

// ReclaimExpired fences off timed-out workers and makes their stage retryable.
func (r *WorkflowRepo) ReclaimExpired(ctx context.Context, workspaceID uuid.UUID) (int64, error) {
	if r == nil || r.Q == nil {
		return 0, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return 0, nil
	}
	var count int64
	err := WithTx(ctx, r.Q, func(q Querier) error {
		if _, e := q.Exec(ctx, `UPDATE curriculum_studio.workflow_attempts a SET status='failed',error=CASE WHEN error='' THEN 'lease expired' ELSE error END,finished_at=now() FROM curriculum_studio.workflow_stages s JOIN curriculum_studio.materialization_runs m ON m.id=s.run_id WHERE a.stage_id=s.id AND a.status='running' AND s.status='running' AND s.lease_expires_at<=now() AND m.workspace_id=$1`, workspaceID); e != nil {
			return e
		}
		tag, e := q.Exec(ctx, `UPDATE curriculum_studio.workflow_stages s SET status='failed',lease_owner='',lease_expires_at=NULL,updated_at=now() FROM curriculum_studio.materialization_runs m WHERE s.run_id=m.id AND m.workspace_id=$1 AND s.status='running' AND s.lease_expires_at<=now()`, workspaceID)
		if e == nil {
			count = tag.RowsAffected()
		}
		return e
	})
	return count, MapError(err)
}

func (r *WorkflowRepo) ListAttempts(ctx context.Context, workspaceID, stageID uuid.UUID) ([]domain.WorkflowAttempt, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	rows, e := r.Q.Query(ctx, `SELECT a.id,a.stage_id,a.attempt_number,a.status,a.agent,a.error,a.started_at,a.finished_at FROM curriculum_studio.workflow_attempts a JOIN curriculum_studio.workflow_stages s ON s.id=a.stage_id JOIN curriculum_studio.materialization_runs m ON m.id=s.run_id WHERE a.stage_id=$1 AND m.workspace_id=$2 ORDER BY a.attempt_number`, stageID, workspaceID)
	if e != nil {
		return nil, MapError(e)
	}
	defer rows.Close()
	var out []domain.WorkflowAttempt
	for rows.Next() {
		v, e := scanWorkflowAttempt(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.WorkflowAttempt{}
	}
	return out, nil
}

const workflowStageColumns = `s.id,s.run_id,s.stage_key,s.position,s.status,s.input,s.output,s.lease_owner,s.lease_expires_at,s.fence_token,s.started_at,s.completed_at,s.created_at,s.updated_at`
const workflowStageSelect = `SELECT ` + workflowStageColumns
const workflowStageFrom = workflowStageSelect + ` FROM curriculum_studio.workflow_stages s JOIN curriculum_studio.materialization_runs m ON m.id=s.run_id`

func scanWorkflowStage(s interface{ Scan(...any) error }) (*domain.WorkflowStage, error) {
	v := new(domain.WorkflowStage)
	e := s.Scan(&v.ID, &v.RunID, &v.StageKey, &v.Position, &v.Status, &v.Input, &v.Output, &v.LeaseOwner, &v.LeaseExpiresAt, &v.FenceToken, &v.StartedAt, &v.CompletedAt, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanWorkflowAttempt(s interface{ Scan(...any) error }) (*domain.WorkflowAttempt, error) {
	v := new(domain.WorkflowAttempt)
	e := s.Scan(&v.ID, &v.StageID, &v.AttemptNumber, &v.Status, &v.Agent, &v.Error, &v.StartedAt, &v.FinishedAt)
	return v, e
}
func workflowObject(raw json.RawMessage, name string) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var value any
	if e := json.Unmarshal(raw, &value); e != nil {
		return nil, fmt.Errorf("%s must be JSON: %w", name, e)
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	return raw, nil
}
func isLeaseNoRows(e error) bool {
	return e != nil && (e == pgx.ErrNoRows || strings.Contains(strings.ToLower(e.Error()), "no rows"))
}
