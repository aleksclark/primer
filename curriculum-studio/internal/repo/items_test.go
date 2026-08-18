package repo_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func itemFixture(t *testing.T, q repo.Querier) (*domain.Workspace, *domain.MaterializationRun) {
	t.Helper()
	ws, run := workflowFixture(t, q)
	return ws, run
}

func TestP9E1CreateItemWithProvenance(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := itemFixture(t, tx)
	r := repo.NewMaterializedItemRepo(tx)
	item, err := r.Create(ctx, ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Kind: domain.ItemKindLesson, Title: "Fractions", Body: json.RawMessage(`{"question":"1/2"}`), Provenance: json.RawMessage(`{"stage_key":"draft"}`)})
	require.NoError(t, err)
	require.Equal(t, run.ID, item.RunID)
	require.JSONEq(t, `{"stage_key":"draft"}`, string(item.Provenance))
	got, err := r.Get(ctx, ws.ID, item.ID)
	require.NoError(t, err)
	require.Equal(t, item.ID, got.ID)
}

func TestP9E2LockedItemOverwriteAndEditRejected(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := itemFixture(t, tx)
	r := repo.NewMaterializedItemRepo(tx)
	item, err := r.Create(ctx, ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Title: "Locked"})
	require.NoError(t, err)
	locked, err := r.Lock(ctx, ws.ID, item.ID, "identity:"+uuid.NewString())
	require.NoError(t, err)
	require.True(t, locked.Locked)
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewMaterializedItemRepo(sp).ApplyEdit(ctx, ws.ID, item.ID, &domain.MaterializedItemEdit{EditorSubjectRef: "identity:" + uuid.NewString(), Patch: json.RawMessage(`{"title":"bad"}`)})
	require.ErrorIs(t, err, repo.ErrLocked)
	_, err = sp.Exec(ctx, `UPDATE curriculum_studio.materialized_items SET title='raw overwrite' WHERE id=$1`, item.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrLocked)
}

func TestP9E3UnlockThenEdit(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := itemFixture(t, tx)
	r := repo.NewMaterializedItemRepo(tx)
	item, err := r.Create(ctx, ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Title: "Editable"})
	require.NoError(t, err)
	_, err = r.Lock(ctx, ws.ID, item.ID, "identity:"+uuid.NewString())
	require.NoError(t, err)
	_, err = r.Unlock(ctx, ws.ID, item.ID)
	require.NoError(t, err)
	_, err = r.ApplyEdit(ctx, ws.ID, item.ID, &domain.MaterializedItemEdit{EditorSubjectRef: "identity:" + uuid.NewString(), Patch: json.RawMessage(`{"title":"edited"}`)})
	require.NoError(t, err)
	edits, err := r.ListEdits(ctx, ws.ID, item.ID)
	require.NoError(t, err)
	require.Len(t, edits, 1)
}

func TestP9E5SupersessionChain(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := itemFixture(t, tx)
	r := repo.NewMaterializedItemRepo(tx)
	old, err := r.Create(ctx, ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Title: "Old"})
	require.NoError(t, err)
	newItem, err := r.Supersede(ctx, ws.ID, old.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Title: "New"})
	require.NoError(t, err)
	require.Equal(t, old.ID, *newItem.SupersedesItemID)
	old, err = r.Get(ctx, ws.ID, old.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ItemStatusSuperseded, old.Status)
}

func TestP9E6AssessmentRequiresSupport(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	ws, run := itemFixture(t, tx)
	r := repo.NewMaterializedItemRepo(tx)
	assessment, err := r.Create(ctx, ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Kind: domain.ItemKindAssessment, Title: "Quiz"})
	require.NoError(t, err)
	sp := testutil.NewSavepointQuerier(tx)
	_, err = repo.NewMaterializedItemRepo(sp).Publish(ctx, ws.ID, assessment.ID)
	require.ErrorIs(t, err, repo.ErrAssessmentSupportRequired)
	rubric, err := r.Create(ctx, ws.ID, &domain.MaterializedItem{RunID: run.ID, PlanRevisionID: run.PlanRevisionID, Kind: domain.ItemKindRubric, Title: "Rubric"})
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `UPDATE curriculum_studio.materialized_items SET status='ready' WHERE id=$1`, rubric.ID)
	require.NoError(t, err)
	_, err = repo.NewAssessmentSupportRepo(tx).Link(ctx, ws.ID, assessment.ID, rubric.ID)
	require.NoError(t, err)
	published, err := r.Publish(ctx, ws.ID, assessment.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ItemStatusPublished, published.Status)
}
