package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/google/uuid"
)

// Runner advances durable materialization state. All progress is in PostgreSQL;
// stopping this process leaves only a fenced lease to be reclaimed.
type Runner struct {
	Q            repo.Querier
	Model        LanguageModel
	Owner        string
	LeaseTTL     time.Duration
	PollInterval time.Duration
	mu           sync.Mutex // FIFO work selection inside this process
}

func NewRunner(q repo.Querier, model LanguageModel) *Runner {
	return &Runner{Q: q, Model: model, Owner: "studio-workflow", LeaseTTL: 5 * time.Second, PollInterval: 100 * time.Millisecond}
}

// Run polls until ctx is cancelled. Cancellation intentionally does not mark an
// in-flight stage failed: its lease is reclaimed by the next process.
func (r *Runner) Run(ctx context.Context) {
	interval := r.PollInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		_, _ = r.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce makes at most one attempt, preserving FIFO order per workspace.
func (r *Runner) RunOnce(ctx context.Context) (bool, error) {
	if r == nil || r.Q == nil || r.Model == nil {
		return false, fmt.Errorf("workflow runner is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rows, err := r.Q.Query(ctx, `SELECT workspace_id,id FROM curriculum_studio.materialization_runs WHERE status IN ('requested','running') ORDER BY created_at,id`)
	if err != nil {
		return false, err
	}
	type candidate struct{ ws, id uuid.UUID }
	var candidates []candidate
	for rows.Next() {
		var v candidate
		if err := rows.Scan(&v.ws, &v.id); err != nil {
			rows.Close()
			return false, err
		}
		candidates = append(candidates, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close() // a transaction has one connection; do not retain its cursor while processing.
	for _, candidate := range candidates {
		worked, err := r.run(ctx, candidate.ws, candidate.id)
		if worked || err != nil {
			return worked, err
		}
	}
	return false, nil
}

func (r *Runner) run(ctx context.Context, ws, runID uuid.UUID) (bool, error) {
	runs := repo.NewMaterializationRunRepo(r.Q)
	workflows := repo.NewWorkflowRepo(r.Q)
	run, err := runs.Get(ctx, ws, runID)
	if err != nil {
		return false, err
	}
	if run.Status == domain.MaterializationStatusRequested {
		if _, err = runs.Start(ctx, ws, runID); err != nil {
			return false, err
		}
	}
	if _, err = workflows.ReclaimExpired(ctx, ws); err != nil {
		return false, err
	}
	specs := make([]domain.WorkflowStageSpec, 0, len(StageKeys))
	for i, key := range StageKeys {
		specs = append(specs, domain.WorkflowStageSpec{StageKey: key, Position: i + 1, Input: json.RawMessage(`{}`)})
	}
	if _, err = workflows.EnsureStages(ctx, ws, runID, specs); err != nil {
		return false, err
	}

	// NextStage alone permits worker B to claim assessments while worker A is
	// still running lessons. Always walk the complete ordered stage list: only
	// its first unsucceeded stage is eligible for a claim.
	stages, err := workflows.ListStages(ctx, ws, runID)
	if err != nil {
		return false, err
	}
	var stage *domain.WorkflowStage
	for i := range stages {
		current := &stages[i]
		if current.Status == domain.WorkflowStageSucceeded || current.Status == domain.WorkflowStageSkipped {
			continue
		}
		if current.Status == domain.WorkflowStageRunning {
			return false, nil
		}
		stage = current // pending or failed; all prior stages succeeded/skipped.
		break
	}
	if stage == nil {
		return r.finish(ctx, ws, runID, false)
	}

	owner := r.Owner
	if owner == "" {
		owner = "studio-workflow"
	}
	ttl := r.LeaseTTL
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	claim, err := workflows.ClaimStage(ctx, ws, stage.ID, owner, ttl, "scripted")
	if err != nil {
		if errors.Is(err, repo.ErrLeaseLost) {
			return false, nil
		}
		return false, err
	}

	stageStarted := time.Now()
	response, err := r.Model.Complete(ctx, Request{Stage: claim.Stage.StageKey, Fixture: "default-v1"})
	if err != nil {
		recordStage(claim.Stage.StageKey, time.Since(stageStarted), true)
		if ctx.Err() != nil { // leave the claim intact for ReclaimExpired.
			return true, nil
		}
		return r.failClaim(ws, runID, claim, owner, err)
	}
	output, _ := json.Marshal(map[string]any{"fixture": response.Fixture, "itemCount": len(response.Items), "findings": response.Findings})

	// The checkpoint takes the fenced stage-row lock before any item write; the
	// lock lasts through persistence and attempt completion. A reclaimed worker
	// cannot checkpoint with the old fence, so its transaction rolls back before
	// creating items. This is the write fence, not merely a post-write check.
	err = repo.WithTx(ctx, r.Q, func(q repo.Querier) error {
		wf := repo.NewWorkflowRepo(q)
		if err := wf.Checkpoint(ctx, ws, claim.Stage.ID, owner, claim.Stage.FenceToken, output); err != nil {
			return err
		}
		if err := r.persist(ctx, q, ws, run, claim.Stage, response); err != nil {
			return err
		}
		return wf.CompleteAttempt(ctx, ws, claim.Stage.ID, claim.Attempt.ID, owner, claim.Stage.FenceToken)
	})
	if err != nil {
		recordStage(claim.Stage.StageKey, time.Since(stageStarted), true)
		// Neither cancellation nor a stale fence is a workflow failure. Both
		// leave (or have already lost) the lease for a later worker to reclaim.
		if ctx.Err() != nil || errors.Is(err, repo.ErrLeaseLost) {
			return true, nil
		}
		return r.failClaim(ws, runID, claim, owner, err)
	}
	recordStage(claim.Stage.StageKey, time.Since(stageStarted), false)
	stages, err = workflows.ListStages(ctx, ws, runID)
	if err != nil {
		return true, err
	}
	for _, s := range stages {
		if s.Status != domain.WorkflowStageSucceeded && s.Status != domain.WorkflowStageSkipped {
			return true, nil
		}
	}
	return r.finish(ctx, ws, runID, false)
}

// failClaim records an actual provider/persistence failure. If the claim was
// reclaimed while doing so, the stale worker stops without changing run state.
func (r *Runner) failClaim(ws, runID uuid.UUID, claim *domain.ClaimedStage, owner string, cause error) (bool, error) {
	err := repo.NewWorkflowRepo(r.Q).FailAttempt(context.Background(), ws, claim.Stage.ID, claim.Attempt.ID, owner, claim.Stage.FenceToken, cause.Error())
	if errors.Is(err, repo.ErrLeaseLost) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return r.finish(context.Background(), ws, runID, true)
}

func (r *Runner) persist(ctx context.Context, q repo.Querier, ws uuid.UUID, run *domain.MaterializationRun, stage domain.WorkflowStage, response Response) error {
	if stage.StageKey == "critic" {
		return nil // findings are checkpointed; deterministic validation remains authoritative.
	}
	items := repo.NewMaterializedItemRepo(q)
	created := map[string]*domain.MaterializedItem{}
	for _, item := range response.Items {
		unitID, err := parseItemUUID(item.UnitID, "unit_id")
		if err != nil {
			return err
		}
		outcomeID, err := parseItemUUID(item.OutcomeID, "outcome_id")
		if err != nil {
			return err
		}
		body, _ := json.Marshal(item.Body)
		provenance, _ := json.Marshal(map[string]any{"provider": "scripted", "fixture_id": response.Fixture, "stage_id": stage.ID.String(), "stage_key": stage.StageKey})
		createdItem, err := items.CreateForStage(ctx, ws, stage.ID, &domain.MaterializedItem{
			RunID: run.ID, PlanRevisionID: run.PlanRevisionID, UnitID: unitID, OutcomeID: outcomeID,
			Kind: item.Kind, Title: item.Title, Body: body, Provenance: provenance, Status: domain.ItemStatusReady,
		})
		if err != nil {
			return err
		}
		created[item.Kind+"\x00"+item.Title] = createdItem
	}
	if stage.StageKey == "assessments" {
		assessment := created[domain.ItemKindAssessment+"\x00"+"Scripted assessment"]
		if assessment != nil {
			supports := repo.NewAssessmentSupportRepo(q)
			for _, key := range []string{domain.ItemKindRubric + "\x00" + "Scripted assessment rubric", domain.ItemKindAnswerKey + "\x00" + "Scripted assessment answers"} {
				if support := created[key]; support != nil {
					_, err := supports.Link(ctx, ws, assessment.ID, support.ID)
					if err != nil && !errors.Is(err, repo.ErrConflict) {
						return err
					}
				}
			}
		}
	}
	return nil
}

func parseItemUUID(raw *string, field string) (*uuid.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	parsed, err := uuid.Parse(strings.TrimSpace(*raw))
	if err != nil {
		return nil, fmt.Errorf("item %s must be a UUID: %w", field, err)
	}
	return &parsed, nil
}

// finish atomically changes a running run into its terminal state and writes
// its outbox event. The FOR UPDATE status read makes terminal races idempotent:
// only the transition winner enqueues an event.
func (r *Runner) finish(ctx context.Context, ws, runID uuid.UUID, failed bool) (bool, error) {
	eventType := domain.EventMaterializationReady
	if failed {
		eventType = domain.EventMaterializationFailed
	}
	transitioned := false
	err := repo.WithTx(ctx, r.Q, func(q repo.Querier) error {
		var current string
		if err := q.QueryRow(ctx, `SELECT status FROM curriculum_studio.materialization_runs WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, runID, ws).Scan(&current); err != nil {
			return repo.MapError(err)
		}
		if current != domain.MaterializationStatusRunning {
			return nil // a concurrent terminal transition already owns the event.
		}
		runs := repo.NewMaterializationRunRepo(q)
		var err error
		if failed {
			_, err = runs.Fail(ctx, ws, runID)
		} else {
			_, err = runs.Ready(ctx, ws, runID)
		}
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"materialization_id": runID.String()})
		if _, err := repo.NewOutboxRepo(q).Enqueue(ctx, &domain.OutboxEvent{WorkspaceID: &ws, EventType: eventType, AggregateKind: "materialization_run", AggregateID: runID, Payload: payload}); err != nil {
			return err
		}
		transitioned = true
		return nil
	})
	return transitioned, err
}
