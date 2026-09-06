package repo_test

import (
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestP17ConcurrentPolicyPublication(t *testing.T) {
	for _, first := range []string{"policy", "publish"} {
		t.Run(first, func(t *testing.T) {
			q := testutil.DB(t)
			ctx := collabRaceContext(t)
			ws, _, rev := planFixture(t, q)
			cleanupCollabOutbox(t, q, ws.ID)
			owner := domain.HumanSubjectRef(uuid.New())
			factory.SeedMembership(t, q, ws.ID, owner, "owner")
			require.NoError(t, repo.NewPolicyRepo(q).Set(ctx, ws.ID, owner, domain.DefaultCollaborationPolicy()))
			strict := domain.CollaborationPolicy{RequireApprovalForPublish: true, SharingEnabled: true}
			publish := func(q repo.Querier) error { return repo.NewPlanRevisionRepo(q).Publish(ctx, ws.ID, rev.ID, owner) }
			if first == "policy" {
				gate, pid := raceTx(t, ctx, q)
				_, err := gate.Exec(ctx, `UPDATE curriculum_studio.workspace_policies SET policies='{"requireApprovalForPublish":true,"sharingEnabled":true}' WHERE workspace_id=$1`, ws.ID)
				require.NoError(t, err)
				worker, done := raceWorker(t, ctx, q, publish)
				waitForCollabLock(t, ctx, q, worker, pid, done)
				require.NoError(t, gate.Commit(ctx))
				require.ErrorIs(t, raceResult(t, ctx, done), repo.ErrApprovalRequired)
				got, err := repo.NewPlanRevisionRepo(q).Get(ctx, ws.ID, rev.ID)
				require.NoError(t, err)
				require.Equal(t, "draft", got.Status)
			} else {
				gate, pid := raceTx(t, ctx, q)
				var id uuid.UUID
				require.NoError(t, gate.QueryRow(ctx, `SELECT id FROM curriculum_studio.plan_revisions WHERE id=$1 FOR UPDATE`, rev.ID).Scan(&id))
				publisher, published := raceWorker(t, ctx, q, publish)
				waitForCollabLock(t, ctx, q, publisher, pid, published)
				setter, set := raceWorker(t, ctx, q, func(q repo.Querier) error { return repo.NewPolicyRepo(q).Set(ctx, ws.ID, owner, strict) })
				waitForCollabLock(t, ctx, q, setter, publisher, set)
				require.NoError(t, gate.Commit(ctx))
				require.NoError(t, raceResult(t, ctx, published))
				require.NoError(t, raceResult(t, ctx, set))
				got, err := repo.NewPlanRevisionRepo(q).Get(ctx, ws.ID, rev.ID)
				require.NoError(t, err)
				require.Equal(t, "published", got.Status)
			}
		})
	}
}
func TestP17PolicyRoleRevocationAndClosedSchema(t *testing.T) {
	q := testutil.DB(t)
	ctx := collabRaceContext(t)
	ws := factory.Workspace(t, q)
	owner := domain.HumanSubjectRef(uuid.New())
	m := factory.SeedMembership(t, q, ws.ID, owner, "owner")
	require.NoError(t, repo.NewPolicyRepo(q).Set(ctx, ws.ID, owner, domain.DefaultCollaborationPolicy()))
	gate, pid := raceTx(t, ctx, q)
	_, err := gate.Exec(ctx, `UPDATE curriculum_studio.workspace_memberships SET status='revoked' WHERE id=$1`, m.ID)
	require.NoError(t, err)
	worker, done := raceWorker(t, ctx, q, func(q repo.Querier) error {
		return repo.NewPolicyRepo(q).Set(ctx, ws.ID, owner, domain.CollaborationPolicy{SharingEnabled: false})
	})
	waitForCollabLock(t, ctx, q, worker, pid, done)
	require.NoError(t, gate.Commit(ctx))
	require.ErrorIs(t, raceResult(t, ctx, done), repo.ErrRoleDenied)
	p, err := repo.NewPolicyRepo(q).Get(ctx, ws.ID)
	require.NoError(t, err)
	require.True(t, p.SharingEnabled)
	_, err = q.Exec(ctx, `UPDATE curriculum_studio.workspace_policies SET policies='{"unknown":true}' WHERE workspace_id=$1`, ws.ID)
	require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
}
