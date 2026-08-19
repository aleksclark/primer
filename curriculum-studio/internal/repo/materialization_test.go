package repo_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/fingerprint"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func TestP7E1LearnerAndClassProfiles(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, _ := planFixture(t, tx)
	r := repo.NewLearnerProfileRepo(tx)
	learner, err := r.Create(ctx, &domain.LearnerProfile{WorkspaceID: ws.ID, Kind: domain.ProfileKindLearner, Label: "Ada", Profile: json.RawMessage(`{"grade":6}`)})
	require.NoError(t, err)
	class, err := r.Create(ctx, &domain.LearnerProfile{WorkspaceID: ws.ID, Kind: domain.ProfileKindClass, Label: "Math", Profile: json.RawMessage(`{"size":1}`)})
	require.NoError(t, err)
	require.NotEqual(t, learner.ID, class.ID)
	list, err := r.ListByWorkspace(ctx, ws.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	foreign, err := r.Get(ctx, uuid.New(), learner.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
	require.Nil(t, foreign)
}

func TestP7E2RunStoresCompleteSnapshotAndTransitions(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, rev := planFixture(t, tx)
	snapshot := json.RawMessage(`{"learner":{"grade":6},"revision":"draft"}`)
	fp, err := fingerprint.Hash(snapshot)
	require.NoError(t, err)
	r := repo.NewMaterializationRunRepo(tx)
	run, err := r.Create(ctx, &domain.MaterializationRun{WorkspaceID: ws.ID, PlanRevisionID: rev.ID, InputSnapshot: snapshot, InputFingerprint: fp, RequestedBySubjectRef: "identity:" + uuid.NewString()})
	require.NoError(t, err)
	require.Equal(t, domain.MaterializationStatusRequested, run.Status)
	run, err = r.Start(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.MaterializationStatusRunning, run.Status)
	run, err = r.Ready(ctx, ws.ID, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.MaterializationStatusReady, run.Status)
	require.JSONEq(t, string(snapshot), string(run.InputSnapshot))
	_, err = r.Fail(ctx, ws.ID, run.ID)
	require.ErrorIs(t, err, repo.ErrInvalidTransition)
}

func TestListByRevisionFiltersWorkspace(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, _, rev := planFixture(t, tx)
	snapshot := json.RawMessage(`{"revision":true}`)
	fp, err := fingerprint.Hash(snapshot)
	require.NoError(t, err)
	r := repo.NewMaterializationRunRepo(tx)
	run, err := r.Create(ctx, &domain.MaterializationRun{WorkspaceID: ws.ID, PlanRevisionID: rev.ID, InputSnapshot: snapshot, InputFingerprint: fp})
	require.NoError(t, err)
	got, err := r.ListByRevision(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, run.ID, got[0].ID)
	empty, err := r.ListByRevision(ctx, uuid.New(), rev.ID)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestP7E3ConcurrentIdempotentRuns(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	ws, _, rev := planFixture(t, pool)
	snapshot := json.RawMessage(`{"inputs":{"grade":6,"subject":"math"}}`)
	fp, err := fingerprint.Hash(snapshot)
	require.NoError(t, err)
	var wg sync.WaitGroup
	results := make(chan *domain.MaterializationRun, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run, err := repo.NewMaterializationRunRepo(pool).CreateRunIdempotent(ctx, &domain.MaterializationRun{WorkspaceID: ws.ID, PlanRevisionID: rev.ID, InputSnapshot: snapshot, InputFingerprint: fp})
			results <- run
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var id uuid.UUID
	for err := range errs {
		require.NoError(t, err)
	}
	for run := range results {
		require.NotNil(t, run)
		if id == uuid.Nil {
			id = run.ID
		} else {
			require.Equal(t, id, run.ID)
		}
	}
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.materialization_runs WHERE plan_revision_id=$1 AND input_fingerprint=$2`, rev.ID, fp).Scan(&count))
	require.Equal(t, 1, count)
}
