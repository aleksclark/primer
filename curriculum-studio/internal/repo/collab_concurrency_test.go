package repo_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// These tests replace the BAD-success expectations of the reviewer repros in
// /tmp/s17_{approval_publish,approval_revocation,library_publish}_race_test.go.
// Each worker starts a real pool-backed transaction on its own connection.
// pg_blocking_pids is the barrier: elapsed time/sleeps are never proof of a wait.
func collabRaceContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func raceTx(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, int) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	var pid int
	require.NoError(t, tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	return tx, pid
}
func raceWorker(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fn func(repo.Querier) error) (int, <-chan error) {
	t.Helper()
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	pid := int(conn.Conn().PgConn().PID())
	done := make(chan error, 1)
	go func() { defer conn.Release(); done <- fn(conn) }()
	return pid, done
}
func waitForCollabLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, waiter, holder int, results ...<-chan error) {
	t.Helper()
	for {
		var blocked bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT $2::int=ANY(pg_blocking_pids($1::int))`, waiter, holder).Scan(&blocked), "worker %d did not wait for holder %d", waiter, holder)
		if blocked {
			return
		}
		for _, done := range results {
			select {
			case err := <-done:
				t.Fatalf("worker %d completed instead of waiting for %d: %v", waiter, holder, err)
			default:
			}
		}
	}
}
func raceResult(t *testing.T, ctx context.Context, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatal("concurrent operation did not finish", ctx.Err())
		return ctx.Err()
	}
}

// Pool-backed publication leaves committed outbox rows. Remove only this
// fixture's events after assertions so unrelated global metrics tests stay
// isolated; graph/decision durability is asserted before cleanup.
func cleanupCollabOutbox(t *testing.T, pool *pgxpool.Pool, ws uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := pool.Exec(ctx, `DELETE FROM curriculum_studio.outbox_events WHERE workspace_id=$1`, ws)
		require.NoError(t, err)
	})
}

type approvalRaceFixture struct {
	ws, revision, outcome, membership uuid.UUID
	subject, fingerprint              string
}

func seedApprovalRace(t *testing.T, ctx context.Context, pool *pgxpool.Pool) approvalRaceFixture {
	t.Helper()
	ws, _, rev := planFixture(t, pool)
	cleanupCollabOutbox(t, pool, ws.ID)
	subject := domain.HumanSubjectRef(uuid.New())
	membership := factory.SeedMembership(t, pool, ws.ID, subject, domain.MembershipRoleReviewer)
	o, err := repo.NewPlanGraphRepo(pool).CreateOutcome(ctx, ws.ID, &domain.Outcome{PlanRevisionID: rev.ID, Code: "scale", Title: "Scale drawings"})
	require.NoError(t, err)
	a := repo.NewApprovalRepo(pool)
	fp, err := a.Fingerprint(ctx, ws.ID, rev.ID)
	require.NoError(t, err)
	_, err = a.Decide(ctx, ws.ID, rev.ID, subject, domain.ApprovalApproved, fp)
	require.NoError(t, err)
	return approvalRaceFixture{ws.ID, rev.ID, o.ID, membership.ID, subject, fp}
}
func (f approvalRaceFixture) decide(ctx context.Context, q repo.Querier) error {
	_, err := repo.NewApprovalRepo(q).Decide(ctx, f.ws, f.revision, f.subject, domain.ApprovalRejected, f.fingerprint)
	return err
}
func (f approvalRaceFixture) stored(t *testing.T, ctx context.Context, q repo.Querier, status string) {
	t.Helper()
	a, err := repo.NewApprovalRepo(q).GetByRevision(ctx, f.ws, f.revision)
	require.NoError(t, err)
	require.Equal(t, status, a.Status)
	require.Equal(t, f.fingerprint, a.ContentFingerprint)
}
func holdApprovalRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f approvalRaceFixture) (pgx.Tx, int) {
	t.Helper()
	tx, pid := raceTx(t, ctx, pool)
	var id uuid.UUID
	require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM curriculum_studio.plan_approvals WHERE plan_revision_id=$1 FOR UPDATE`, f.revision).Scan(&id))
	return tx, pid
}

func TestP17ApprovalPublishRace(t *testing.T) {
	for _, first := range []string{"publish", "decision"} {
		t.Run(first+"-first", func(t *testing.T) {
			pool := testutil.DB(t)
			ctx := collabRaceContext(t)
			f := seedApprovalRace(t, ctx, pool)
			publish := func(q repo.Querier) error {
				return repo.NewPlanRevisionRepo(q).Publish(ctx, f.ws, f.revision, f.subject)
			}
			if first == "publish" {
				tx, pid := raceTx(t, ctx, pool)
				require.NoError(t, publish(tx)) // still uncommitted
				worker, done := raceWorker(t, ctx, pool, func(q repo.Querier) error { return f.decide(ctx, q) })
				waitForCollabLock(t, ctx, pool, worker, pid)
				require.NoError(t, tx.Commit(ctx))
				require.ErrorIs(t, raceResult(t, ctx, done), repo.ErrImmutable)
				f.stored(t, ctx, pool, domain.ApprovalApproved)
			} else {
				gate, gatePID := holdApprovalRow(t, ctx, pool, f)
				decider, decision := raceWorker(t, ctx, pool, func(q repo.Querier) error { return f.decide(ctx, q) })
				waitForCollabLock(t, ctx, pool, decider, gatePID)
				publisher, published := raceWorker(t, ctx, pool, publish)
				waitForCollabLock(t, ctx, pool, publisher, decider)
				require.NoError(t, gate.Commit(ctx))
				require.NoError(t, raceResult(t, ctx, decision))
				require.NoError(t, raceResult(t, ctx, published))
				f.stored(t, ctx, pool, domain.ApprovalRejected)
			}
			rev, err := repo.NewPlanRevisionRepo(pool).Get(ctx, f.ws, f.revision)
			require.NoError(t, err)
			require.Equal(t, "published", rev.Status)
			// Subsequent decisions must never turn a published plan into reviewed draft.
			require.ErrorIs(t, f.decide(ctx, pool), repo.ErrImmutable)
		})
	}
}

func TestP17ApprovalRevocationRace(t *testing.T) {
	for _, first := range []string{"revoke", "decision"} {
		t.Run(first+"-first", func(t *testing.T) {
			pool := testutil.DB(t)
			ctx := collabRaceContext(t)
			f := seedApprovalRace(t, ctx, pool)
			revoke := func(q repo.Querier) error {
				_, err := repo.NewMembershipRepo(q).UpdateStatus(ctx, f.ws, f.membership, "revoked")
				return err
			}
			if first == "revoke" {
				tx, pid := raceTx(t, ctx, pool)
				require.NoError(t, revoke(tx))
				worker, done := raceWorker(t, ctx, pool, func(q repo.Querier) error { return f.decide(ctx, q) })
				waitForCollabLock(t, ctx, pool, worker, pid)
				require.NoError(t, tx.Commit(ctx))
				require.ErrorIs(t, raceResult(t, ctx, done), repo.ErrConflict)
				f.stored(t, ctx, pool, domain.ApprovalApproved)
			} else {
				gate, pid := holdApprovalRow(t, ctx, pool, f)
				decider, decision := raceWorker(t, ctx, pool, func(q repo.Querier) error { return f.decide(ctx, q) })
				waitForCollabLock(t, ctx, pool, decider, pid)
				revoker, revoked := raceWorker(t, ctx, pool, revoke)
				waitForCollabLock(t, ctx, pool, revoker, decider)
				require.NoError(t, gate.Commit(ctx))
				require.NoError(t, raceResult(t, ctx, decision))
				require.NoError(t, raceResult(t, ctx, revoked))
				f.stored(t, ctx, pool, domain.ApprovalRejected)
			}
			current, err := repo.NewApprovalRepo(pool).Current(ctx, f.ws, f.revision)
			require.NoError(t, err)
			require.Nil(t, current, "a revoked reviewer cannot leave an effective decision")
			require.ErrorIs(t, f.decide(ctx, pool), repo.ErrConflict)
		})
	}
}

func TestP17ApprovalContentMutationRace(t *testing.T) {
	for _, first := range []string{"content", "decision"} {
		t.Run(first+"-first", func(t *testing.T) {
			pool := testutil.DB(t)
			ctx := collabRaceContext(t)
			f := seedApprovalRace(t, ctx, pool)
			edit := func(q repo.Querier) error {
				_, err := q.Exec(ctx, `UPDATE curriculum_studio.outcomes SET title='Changed during review' WHERE id=$1`, f.outcome)
				return err
			}
			if first == "content" {
				tx, pid := raceTx(t, ctx, pool)
				require.NoError(t, edit(tx))
				worker, done := raceWorker(t, ctx, pool, func(q repo.Querier) error { return f.decide(ctx, q) })
				waitForCollabLock(t, ctx, pool, worker, pid)
				require.NoError(t, tx.Commit(ctx))
				require.ErrorIs(t, raceResult(t, ctx, done), repo.ErrConflict)
				f.stored(t, ctx, pool, domain.ApprovalApproved)
			} else {
				gate, pid := holdApprovalRow(t, ctx, pool, f)
				decider, decision := raceWorker(t, ctx, pool, func(q repo.Querier) error { return f.decide(ctx, q) })
				waitForCollabLock(t, ctx, pool, decider, pid)
				editor, edited := raceWorker(t, ctx, pool, edit)
				waitForCollabLock(t, ctx, pool, editor, decider)
				require.NoError(t, gate.Commit(ctx))
				require.NoError(t, raceResult(t, ctx, decision))
				require.NoError(t, raceResult(t, ctx, edited))
				f.stored(t, ctx, pool, domain.ApprovalRejected)
			}
			current, err := repo.NewApprovalRepo(pool).Current(ctx, f.ws, f.revision)
			require.NoError(t, err)
			require.Nil(t, current)
			fp, err := repo.NewApprovalRepo(pool).Fingerprint(ctx, f.ws, f.revision)
			require.NoError(t, err)
			require.NotEqual(t, f.fingerprint, fp)
		})
	}
}

func TestP17LibraryPublishRace(t *testing.T) {
	for _, first := range []string{"publish", "copy"} {
		t.Run(first+"-first", func(t *testing.T) {
			pool := testutil.DB(t)
			ctx := collabRaceContext(t)
			ws, _, rev := planFixture(t, pool)
			cleanupCollabOutbox(t, pool, ws.ID)
			standard := factory.CatalogStandard(t, pool)
			raw, err := json.Marshal(domain.UnitLibrarySnapshot{Unit: domain.Unit{Code: "unit", Title: "Library unit", Blueprint: json.RawMessage(`{}`)}, Outcomes: []domain.LibraryOutcome{{Outcome: domain.Outcome{ID: uuid.New(), Code: "outcome", Title: "Library outcome"}, Role: "target", Standards: []domain.OutcomeStandardMapping{{StandardID: standard.ID, Alignment: "addresses"}}, Evidence: []domain.EvidenceRequirement{{Kind: "formal", Description: "Show your work", Criteria: json.RawMessage(`{}`)}}}}})
			require.NoError(t, err)
			library := repo.NewUnitLibraryRepo(pool)
			entry, err := library.Create(ctx, &domain.UnitLibraryEntry{WorkspaceID: ws.ID, Name: "Reusable", Blueprint: raw})
			require.NoError(t, err)
			copy := func(q repo.Querier) error {
				_, err := repo.NewUnitLibraryRepo(q).CopyIntoRevision(ctx, ws.ID, rev.ID, entry.ID)
				return err
			}
			publish := func(q repo.Querier) error {
				return repo.NewPlanRevisionRepo(q).Publish(ctx, ws.ID, rev.ID, domain.HumanSubjectRef(uuid.New()))
			}
			if first == "publish" {
				tx, pid := raceTx(t, ctx, pool)
				require.NoError(t, publish(tx))
				worker, done := raceWorker(t, ctx, pool, copy)
				waitForCollabLock(t, ctx, pool, worker, pid)
				require.NoError(t, tx.Commit(ctx))
				require.ErrorIs(t, raceResult(t, ctx, done), repo.ErrImmutable)
			} else {
				// Copy owns its revision lock before the standard FK wait. Publication
				// must wait for the entire copied graph, not commit a partial snapshot.
				gate, pid := raceTx(t, ctx, pool)
				var id uuid.UUID
				require.NoError(t, gate.QueryRow(ctx, `SELECT id FROM curriculum_studio.catalog_standards WHERE id=$1 FOR UPDATE`, standard.ID).Scan(&id))
				copier, copied := raceWorker(t, ctx, pool, copy)
				waitForCollabLock(t, ctx, pool, copier, pid, copied)
				publisher, published := raceWorker(t, ctx, pool, publish)
				waitForCollabLock(t, ctx, pool, publisher, copier)
				require.NoError(t, gate.Commit(ctx))
				require.NoError(t, raceResult(t, ctx, copied))
				require.NoError(t, raceResult(t, ctx, published))
			}
			graph, err := repo.NewPlanGraphRepo(pool).Load(ctx, ws.ID, rev.ID)
			require.NoError(t, err)
			require.Equal(t, "published", graph.Revision.Status)
			expected := 0
			if first == "copy" {
				expected = 1
			}
			require.Len(t, graph.Units, expected)
			require.Len(t, graph.Outcomes, expected)
			require.Len(t, graph.UnitOutcomes, expected)
			require.Len(t, graph.OutcomeStandardMappings, expected)
			require.Len(t, graph.EvidenceRequirements, expected)
			require.ErrorIs(t, copy(pool), repo.ErrImmutable)
		})
	}
}

// A non-transactional Querier facade must not cause WithTx's fallback to
// release each lock in autocommit. Higher-isolation snapshots must not be
// mistaken for the fresh statement snapshots this protocol requires.
type collabAutocommitQuerier struct{ repo.Querier }

func TestP17CollabRequiresReadCommittedTransaction(t *testing.T) {
	pool := testutil.DB(t)
	ctx := collabRaceContext(t)
	f := seedApprovalRace(t, ctx, pool)
	check := func(q repo.Querier) {
		require.ErrorIs(t, f.decide(ctx, q), repo.ErrConflict)
		_, err := repo.NewUnitLibraryRepo(q).CopyIntoRevision(ctx, f.ws, f.revision, uuid.New())
		require.ErrorIs(t, err, repo.ErrConflict)
	}
	check(collabAutocommitQuerier{pool})
	for _, level := range []pgx.TxIsoLevel{pgx.RepeatableRead, pgx.Serializable} {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: level})
		require.NoError(t, err)
		check(tx)
		require.NoError(t, tx.Rollback(ctx))
	}
	f.stored(t, ctx, pool, domain.ApprovalApproved)
}

func TestP17GraphWriteWaitsForPublication(t *testing.T) {
	pool := testutil.DB(t)
	ctx := collabRaceContext(t)
	f := seedApprovalRace(t, ctx, pool)
	tx, pid := raceTx(t, ctx, pool)
	require.NoError(t, repo.NewPlanRevisionRepo(tx).Publish(ctx, f.ws, f.revision, f.subject))
	worker, done := raceWorker(t, ctx, pool, func(q repo.Querier) error {
		_, err := q.Exec(ctx, `UPDATE curriculum_studio.outcomes SET title='Too late' WHERE id=$1`, f.outcome)
		return repo.MapError(err)
	})
	waitForCollabLock(t, ctx, pool, worker, pid)
	require.NoError(t, tx.Commit(ctx))
	require.ErrorIs(t, raceResult(t, ctx, done), repo.ErrImmutable)
	graph, err := repo.NewPlanGraphRepo(pool).Load(ctx, f.ws, f.revision)
	require.NoError(t, err)
	require.Equal(t, "Scale drawings", graph.Outcomes[0].Title)
}
