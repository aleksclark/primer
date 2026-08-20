package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/fingerprint"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func workflowRun(t *testing.T, q repo.Querier) (*domain.Workspace, *domain.MaterializationRun) {
	t.Helper()
	ctx := context.Background()
	ws := factory.Workspace(t, q)
	cur, err := repo.NewCurriculumRepo(q).Create(ctx, &domain.Curriculum{WorkspaceID: ws.ID, Slug: "workflow-" + uuid.NewString()[:8], Title: "Workflow"})
	require.NoError(t, err)
	rev, err := repo.NewPlanRevisionRepo(q).Create(ctx, ws.ID, &domain.PlanRevision{CurriculumID: cur.ID, Revision: 1, Title: "Published workflow"})
	require.NoError(t, err)
	require.NoError(t, repo.NewPlanRevisionRepo(q).Publish(ctx, ws.ID, rev.ID, "identity:"+uuid.NewString()))
	snapshot := json.RawMessage(`{"workflow":true}`)
	fp, err := fingerprint.Hash(snapshot)
	require.NoError(t, err)
	run, err := repo.NewMaterializationRunRepo(q).Create(ctx, &domain.MaterializationRun{WorkspaceID: ws.ID, PlanRevisionID: rev.ID, InputSnapshot: snapshot, InputFingerprint: fp})
	require.NoError(t, err)
	return ws, run
}

func TestP12E1HappyScriptedRun(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := workflowRun(t, tx)
	runner := NewRunner(tx, &Scripted{Fixture: "default-v1"})
	for i := 0; i < len(StageKeys); i++ {
		worked, err := runner.RunOnce(ctx)
		require.NoError(t, err)
		require.True(t, worked)
	}
	got, err := repo.NewMaterializationRunRepo(tx).Get(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.MaterializationStatusReady, got.Status)
	items, err := repo.NewMaterializedItemRepo(tx).ListByRun(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(items), 4)
	kinds := map[string]bool{}
	for _, item := range items {
		kinds[item.Kind] = true
		var provenance map[string]any
		require.NoError(t, json.Unmarshal(item.Provenance, &provenance))
		require.Equal(t, "scripted", provenance["provider"])
		require.Equal(t, "default-v1", provenance["fixture_id"])
	}
	require.True(t, kinds[domain.ItemKindLesson])
	require.True(t, kinds[domain.ItemKindAssessment])
	stages, err := repo.NewWorkflowRepo(tx).ListStages(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.Len(t, stages, len(StageKeys))
	for _, stage := range stages {
		require.Equal(t, domain.WorkflowStageSucceeded, stage.Status)
		attempts, e := repo.NewWorkflowRepo(tx).ListAttempts(ctx, ws.ID, stage.ID)
		require.NoError(t, e)
		require.Len(t, attempts, 1)
	}
	var events int
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.outbox_events WHERE aggregate_id=$1 AND event_type='materialization.ready'`, run.ID).Scan(&events))
	require.Equal(t, 1, events)
}

// KillResume proves a cancelled process leaves the claimed attempt fenced;
// after lease reclamation the new runner resumes that stage without deleting or
// duplicating side effects from succeeded stages.
func TestP12E2KillResume(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	ws, run := workflowRun(t, pool)

	// Lessons succeed before the process is interrupted while assessments is
	// claimed. A resume must not execute that succeeded lessons stage again.
	seed := NewRunner(pool, &Scripted{})
	seed.LeaseTTL = 20 * time.Millisecond
	worked, err := seed.RunOnce(ctx)
	require.NoError(t, err)
	require.True(t, worked)

	pause := make(chan struct{})
	started := make(chan struct{}, 1)
	first := NewRunner(pool, &Scripted{Pause: pause, Started: started})
	first.LeaseTTL = 20 * time.Millisecond
	callCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := first.RunOnce(callCtx); done <- err }()
	<-started // assessments is claimed and Complete is blocked.
	// A concurrent worker observes the running first-unsucceeded stage; it must
	// not leapfrog to critic while assessments still owns its lease.
	worked, err = NewRunner(pool, &Scripted{}).RunOnce(ctx)
	require.NoError(t, err)
	require.False(t, worked)
	cancel()
	require.NoError(t, <-done)
	stages, err := repo.NewWorkflowRepo(pool).ListStages(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.WorkflowStageSucceeded, stages[0].Status)
	require.Equal(t, domain.WorkflowStageRunning, stages[1].Status)
	require.EqualValues(t, 1, stages[1].FenceToken)
	time.Sleep(30 * time.Millisecond)
	close(pause)

	resumed := NewRunner(pool, &Scripted{})
	resumed.LeaseTTL = 20 * time.Millisecond
	for i := 0; i < 2; i++ { // reclaim assessments, then run critic
		worked, err := resumed.RunOnce(ctx)
		require.NoError(t, err)
		require.True(t, worked)
	}
	got, err := repo.NewMaterializationRunRepo(pool).Get(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.MaterializationStatusReady, got.Status)
	stages, err = repo.NewWorkflowRepo(pool).ListStages(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, stages[0].FenceToken, "succeeded lessons was not re-executed")
	require.EqualValues(t, 2, stages[1].FenceToken, "reclaimed assessment has a new fence")
	for i, expected := range []int{1, 2, 1} {
		attempts, err := repo.NewWorkflowRepo(pool).ListAttempts(ctx, ws.ID, stages[i].ID)
		require.NoError(t, err)
		require.Len(t, attempts, expected)
	}
	items, err := repo.NewMaterializedItemRepo(pool).ListByRunFiltered(ctx, ws.ID, run.ID, domain.ItemKindLesson, "")
	require.NoError(t, err)
	require.Len(t, items, 1, "resume does not duplicate succeeded-stage lessons")
}

func TestP12E3ScriptedNoNetwork(t *testing.T) {
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	var dials atomic.Int64
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		dials.Add(1)
		return nil, errors.New("network use is forbidden")
	})
	m := &Scripted{}
	_, err := m.Complete(context.Background(), Request{Stage: "lessons"})
	require.NoError(t, err)
	require.Zero(t, dials.Load())
}

func TestP12E4FailureRetry(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := workflowRun(t, tx)
	runner := NewRunner(tx, &failStageOnce{stage: "assessments"})

	// Lessons are durable before assessments fails. Retry must pick up only the
	// failed assessments stage, never re-run completed lessons.
	for i := 0; i < 2; i++ {
		worked, err := runner.RunOnce(ctx)
		require.NoError(t, err)
		require.True(t, worked)
	}
	failed, err := repo.NewMaterializationRunRepo(tx).Get(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.MaterializationStatusFailed, failed.Status)
	var failedEvents int
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.outbox_events WHERE aggregate_id=$1 AND event_type='materialization.failed'`, run.ID).Scan(&failedEvents))
	require.Equal(t, 1, failedEvents)
	_, err = repo.NewMaterializationRunRepo(tx).Start(ctx, ws.ID, run.ID) // same transition the author retry endpoint uses
	require.NoError(t, err)
	for i := 0; i < 2; i++ { // retry assessments, then critic
		worked, err := runner.RunOnce(ctx)
		require.NoError(t, err)
		require.True(t, worked)
	}
	ready, err := repo.NewMaterializationRunRepo(tx).Get(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.MaterializationStatusReady, ready.Status)
	stages, err := repo.NewWorkflowRepo(tx).ListStages(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	for i, expected := range []int{1, 2, 1} {
		attempts, err := repo.NewWorkflowRepo(tx).ListAttempts(ctx, ws.ID, stages[i].ID)
		require.NoError(t, err)
		require.Len(t, attempts, expected)
	}
}

type failStageOnce struct {
	stage  string
	failed bool
}

func (m *failStageOnce) Complete(_ context.Context, req Request) (Response, error) {
	if req.Stage == m.stage && !m.failed {
		m.failed = true
		return Response{}, errors.New("scripted stage failure")
	}
	return loadFixture("default-v1", req.Stage)
}

func TestP12E5AssessmentSupportRequired(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := workflowRun(t, tx)
	_, err := repo.NewMaterializationRunRepo(tx).Start(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	sp := testutil.NewSavepointQuerier(tx)
	items := repo.NewMaterializedItemRepo(sp)
	assessment, err := items.Create(ctx, ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Kind: domain.ItemKindAssessment, Title: "Assessment", Body: json.RawMessage(`{}`), Provenance: json.RawMessage(`{}`)})
	require.NoError(t, err)
	_, err = items.Publish(ctx, ws.ID, assessment.ID)
	require.ErrorIs(t, err, repo.ErrAssessmentSupportRequired)
	rubric, err := items.Create(ctx, ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Kind: domain.ItemKindRubric, Title: "Rubric", Body: json.RawMessage(`{}`), Provenance: json.RawMessage(`{}`), Status: domain.ItemStatusReady})
	require.NoError(t, err)
	_, err = repo.NewAssessmentSupportRepo(sp).Link(ctx, ws.ID, assessment.ID, rubric.ID)
	require.NoError(t, err)
	published, err := items.Publish(ctx, ws.ID, assessment.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ItemStatusPublished, published.Status)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
