package repo_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/fingerprint"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func workflowFixture(t *testing.T, q repo.Querier) (*domain.Workspace, *domain.MaterializationRun) {
	t.Helper()
	ws, _, rev := planFixture(t, q)
	// This fingerprint is only a fixture value; use the repository's normal
	// helper path so the run is durable and scoped.
	snapshot := json.RawMessage(`{"workflow":"test"}`)
	fp := ""
	var err error
	fp, err = fingerprintForTest(snapshot)
	require.NoError(t, err)
	run, err := repo.NewMaterializationRunRepo(q).Create(context.Background(), &domain.MaterializationRun{WorkspaceID: ws.ID, PlanRevisionID: rev.ID, InputSnapshot: snapshot, InputFingerprint: fp})
	require.NoError(t, err)
	return ws, run
}

func fingerprintForTest(raw []byte) (string, error) {
	return fingerprint.Hash(raw)
}

func TestP8E1EnsureStagesAndCheckpoint(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := workflowFixture(t, tx)
	r := repo.NewWorkflowRepo(tx)
	stages, err := r.EnsureStages(ctx, ws.ID, run.ID, []domain.WorkflowStageSpec{{StageKey: "draft", Position: 1}, {StageKey: "review", Position: 2}})
	require.NoError(t, err)
	require.Len(t, stages, 2)
	claimed, err := r.ClaimStage(ctx, ws.ID, stages[0].ID, "worker-a", time.Minute, "agent-a")
	require.NoError(t, err)
	require.Equal(t, int64(1), claimed.Stage.FenceToken)
	require.Equal(t, 1, claimed.Attempt.AttemptNumber)
	require.NoError(t, r.Checkpoint(ctx, ws.ID, claimed.Stage.ID, "worker-a", claimed.Stage.FenceToken, json.RawMessage(`{"step":1}`)))
	got, err := r.ListStages(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.JSONEq(t, `{"step":1}`, string(got[0].Output))
}

func TestP8E2ResumeFromFailedStage(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := workflowFixture(t, tx)
	r := repo.NewWorkflowRepo(tx)
	stages, err := r.EnsureStages(ctx, ws.ID, run.ID, []domain.WorkflowStageSpec{{StageKey: "one", Position: 1}, {StageKey: "two", Position: 2}})
	require.NoError(t, err)
	claimed, err := r.ClaimStage(ctx, ws.ID, stages[0].ID, "worker", time.Minute, "agent")
	require.NoError(t, err)
	require.NoError(t, r.FailAttempt(ctx, ws.ID, claimed.Stage.ID, claimed.Attempt.ID, "worker", claimed.Stage.FenceToken, "failed"))
	next, err := r.NextStage(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.Equal(t, stages[0].ID, next.ID)
}

func TestP8E3RetryIncrementsAttempts(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := workflowFixture(t, tx)
	r := repo.NewWorkflowRepo(tx)
	stages, err := r.EnsureStages(ctx, ws.ID, run.ID, []domain.WorkflowStageSpec{{StageKey: "one", Position: 1}})
	require.NoError(t, err)
	first, err := r.ClaimStage(ctx, ws.ID, stages[0].ID, "worker", time.Minute, "agent")
	require.NoError(t, err)
	require.NoError(t, r.FailAttempt(ctx, ws.ID, first.Stage.ID, first.Attempt.ID, "worker", first.Stage.FenceToken, "retry"))
	second, err := r.ClaimStage(ctx, ws.ID, stages[0].ID, "worker", time.Minute, "agent")
	require.NoError(t, err)
	require.Equal(t, 2, second.Attempt.AttemptNumber)
	attempts, err := r.ListAttempts(ctx, ws.ID, stages[0].ID)
	require.NoError(t, err)
	require.Len(t, attempts, 2)
}

func TestP8E4ReclaimFencesStaleWorker(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	ws, run := workflowFixture(t, pool)
	r := repo.NewWorkflowRepo(pool)
	stages, err := r.EnsureStages(ctx, ws.ID, run.ID, []domain.WorkflowStageSpec{{StageKey: "one", Position: 1}})
	require.NoError(t, err)
	old, err := r.ClaimStage(ctx, ws.ID, stages[0].ID, "worker-a", 50*time.Millisecond, "agent-a")
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	n, err := r.ReclaimExpired(ctx, ws.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	fresh, err := r.ClaimStage(ctx, ws.ID, stages[0].ID, "worker-b", time.Minute, "agent-b")
	require.NoError(t, err)
	require.Greater(t, fresh.Stage.FenceToken, old.Stage.FenceToken)
	err = r.Checkpoint(ctx, ws.ID, old.Stage.ID, "worker-a", old.Stage.FenceToken, json.RawMessage(`{"stale":true}`))
	require.ErrorIs(t, err, repo.ErrLeaseLost)
	require.NoError(t, r.Checkpoint(ctx, ws.ID, fresh.Stage.ID, "worker-b", fresh.Stage.FenceToken, json.RawMessage(`{"owner":"b"}`)))
}

func TestP8E5ReclaimDoesNotActBeforeExpiry(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := workflowFixture(t, tx)
	r := repo.NewWorkflowRepo(tx)
	stages, err := r.EnsureStages(ctx, ws.ID, run.ID, []domain.WorkflowStageSpec{{StageKey: "one", Position: 1}})
	require.NoError(t, err)
	_, err = r.ClaimStage(ctx, ws.ID, stages[0].ID, "worker", time.Minute, "agent")
	require.NoError(t, err)
	n, err := r.ReclaimExpired(ctx, ws.ID)
	require.NoError(t, err)
	require.Zero(t, n)
}
