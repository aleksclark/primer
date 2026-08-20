package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	stage, err := workflows.NextStage(ctx, ws, runID)
	if errors.Is(err, repo.ErrNotFound) {
		stages, listErr := workflows.ListStages(ctx, ws, runID)
		if listErr != nil {
			return false, listErr
		}
		for _, current := range stages {
			if current.Status == domain.WorkflowStageRunning {
				return false, nil
			}
			if current.Status != domain.WorkflowStageSucceeded && current.Status != domain.WorkflowStageSkipped {
				return false, nil
			}
		}
		return r.finish(ctx, ws, runID, false)
	}
	if err != nil {
		return false, err
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
		// A process shutdown must leave the lease for safe recovery, rather than
		// converting an interrupted request into an unrecoverable failure.
		if ctx.Err() != nil {
			return true, nil
		}
		_ = workflows.FailAttempt(context.Background(), ws, claim.Stage.ID, claim.Attempt.ID, owner, claim.Stage.FenceToken, err.Error())
		_, _ = r.finish(context.Background(), ws, runID, true)
		return true, nil
	}
	if err = r.persist(ctx, ws, run, claim.Stage, response); err != nil {
		recordStage(claim.Stage.StageKey, time.Since(stageStarted), true)
		_ = workflows.FailAttempt(context.Background(), ws, claim.Stage.ID, claim.Attempt.ID, owner, claim.Stage.FenceToken, err.Error())
		_, _ = r.finish(context.Background(), ws, runID, true)
		return true, nil
	}
	output, _ := json.Marshal(map[string]any{"fixture": response.Fixture, "itemCount": len(response.Items), "findings": response.Findings})
	if err = workflows.Checkpoint(ctx, ws, claim.Stage.ID, owner, claim.Stage.FenceToken, output); err != nil {
		return true, err
	}
	if err = workflows.CompleteAttempt(ctx, ws, claim.Stage.ID, claim.Attempt.ID, owner, claim.Stage.FenceToken); err != nil {
		recordStage(claim.Stage.StageKey, time.Since(stageStarted), true)
		return true, err
	}
	recordStage(claim.Stage.StageKey, time.Since(stageStarted), false)
	stages, err := workflows.ListStages(ctx, ws, runID)
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

func (r *Runner) persist(ctx context.Context, ws uuid.UUID, run *domain.MaterializationRun, stage domain.WorkflowStage, response Response) error {
	if stage.StageKey == "critic" {
		return nil
	} // findings are checkpointed; deterministic validation remains authoritative.
	items := repo.NewMaterializedItemRepo(r.Q)
	existing, err := items.ListByRun(ctx, ws, run.ID)
	if err != nil {
		return err
	}
	byTitle := map[string]domain.MaterializedItem{}
	for _, item := range existing {
		var p map[string]any
		_ = json.Unmarshal(item.Provenance, &p)
		if p["stage_id"] == stage.ID.String() {
			byTitle[item.Kind+"\x00"+item.Title] = item
		}
	}
	created := map[string]*domain.MaterializedItem{}
	for _, item := range response.Items {
		key := item.Kind + "\x00" + item.Title
		if old, ok := byTitle[key]; ok {
			copy := old
			created[key] = &copy
			continue
		}
		body, _ := json.Marshal(item.Body)
		prov, _ := json.Marshal(map[string]any{"provider": "scripted", "fixture_id": response.Fixture, "stage_id": stage.ID.String(), "stage_key": stage.StageKey})
		v, err := items.Create(ctx, ws, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Kind: item.Kind, Title: item.Title, Body: body, Provenance: prov, Status: domain.ItemStatusReady})
		if err != nil {
			return err
		}
		created[key] = v
	}
	if stage.StageKey == "assessments" {
		assessment := created[domain.ItemKindAssessment+"\x00"+"Scripted assessment"]
		if assessment != nil {
			supports := repo.NewAssessmentSupportRepo(r.Q)
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

func (r *Runner) finish(ctx context.Context, ws, runID uuid.UUID, failed bool) (bool, error) {
	runs := repo.NewMaterializationRunRepo(r.Q)
	var err error
	if failed {
		_, err = runs.Fail(ctx, ws, runID)
	} else {
		_, err = runs.Ready(ctx, ws, runID)
	}
	if err != nil {
		return false, err
	}
	typ := domain.EventMaterializationReady
	if failed {
		typ = domain.EventMaterializationFailed
	}
	payload, _ := json.Marshal(map[string]any{"materialization_id": runID.String()})
	_, err = repo.NewOutboxRepo(r.Q).Enqueue(ctx, &domain.OutboxEvent{WorkspaceID: &ws, EventType: typ, AggregateKind: "materialization_run", AggregateID: runID, Payload: payload})
	return true, err
}
