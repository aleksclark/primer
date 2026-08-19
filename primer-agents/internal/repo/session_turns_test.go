package repo_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
)

// ── BDD: AppendTurn creates a turn and run atomically ─────────────────────────

func TestAppendTurnCreatesRunAndTurn(t *testing.T) {
	ctx := context.Background()
	p := pool(t)

	// Create a session first.
	ns := "ns-turn-create-" + uuid.NewString()[:8]
	sess, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: ns,
		Profile:        "admin",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), sess.Revision)

	result, err := repo.SessionTurns.AppendTurn(ctx, p, repo.Runs, repo.Sessions, repo.AppendTurnCmd{
		SessionID:        sess.ID,
		OwnerNamespace:   ns,
		IdempotencyKey:   "turn-1",
		Profile:          "admin",
		ExpectedRevision: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Turn)
	require.NotNil(t, result.Run)
	require.NotNil(t, result.Session)

	assert.Equal(t, int64(2), result.Session.Revision, "session revision must advance")
	assert.Equal(t, int64(2), result.Turn.TurnSequence, "turn_sequence == new revision")
	assert.Equal(t, sess.ID, result.Turn.SessionID)
	assert.NotNil(t, result.Turn.RunID)
	assert.Equal(t, *result.Turn.RunID, result.Run.ID)
	assert.Equal(t, ns, result.Run.OwnerNamespace)
	assert.Equal(t, domain.RunStatusQueued, result.Run.Status)
}

// ── BDD: AppendTurn is idempotent on same idempotency key ─────────────────────

func TestAppendTurnIdempotent(t *testing.T) {
	ctx := context.Background()
	p := pool(t)

	ns := "ns-turn-idem-" + uuid.NewString()[:8]
	sess, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: ns,
		Profile:        "admin",
	})
	require.NoError(t, err)

	cmd := repo.AppendTurnCmd{
		SessionID:        sess.ID,
		OwnerNamespace:   ns,
		IdempotencyKey:   "turn-idem",
		Profile:          "admin",
		ExpectedRevision: 1,
	}

	r1, err := repo.SessionTurns.AppendTurn(ctx, p, repo.Runs, repo.Sessions, cmd)
	require.NoError(t, err)

	// Second call with same idempotency key → same turn/run returned.
	r2, err := repo.SessionTurns.AppendTurn(ctx, p, repo.Runs, repo.Sessions, cmd)
	require.NoError(t, err)

	assert.Equal(t, r1.Turn.ID, r2.Turn.ID, "idempotent turn must return same turn ID")
	assert.Equal(t, r1.Run.ID, r2.Run.ID, "idempotent turn must return same run ID")
}

// ── BDD: stale expected revision returns ErrStaleVersion ─────────────────────

func TestAppendTurnRevisionConflict(t *testing.T) {
	ctx := context.Background()
	p := pool(t)

	ns := "ns-turn-conflict-" + uuid.NewString()[:8]
	sess, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: ns,
		Profile:        "admin",
	})
	require.NoError(t, err)

	// First turn succeeds at revision 1.
	_, err = repo.SessionTurns.AppendTurn(ctx, p, repo.Runs, repo.Sessions, repo.AppendTurnCmd{
		SessionID:        sess.ID,
		OwnerNamespace:   ns,
		IdempotencyKey:   "turn-1",
		Profile:          "admin",
		ExpectedRevision: 1,
	})
	require.NoError(t, err)

	// Second turn with stale revision 1 (should be 2 now) → conflict.
	_, err = repo.SessionTurns.AppendTurn(ctx, p, repo.Runs, repo.Sessions, repo.AppendTurnCmd{
		SessionID:        sess.ID,
		OwnerNamespace:   ns,
		IdempotencyKey:   "turn-2-stale",
		Profile:          "admin",
		ExpectedRevision: 1, // stale — session is now at revision 2
	})
	require.ErrorIs(t, err, repo.ErrStaleVersion,
		"stale expected revision must return ErrStaleVersion")
}

// ── BDD: concurrent turns at same revision — exactly one wins ────────────────

func TestAppendTurnConcurrentRevisionConflict(t *testing.T) {
	ctx := context.Background()
	p := pool(t)

	ns := "ns-turn-race-" + uuid.NewString()[:8]
	sess, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: ns,
		Profile:        "admin",
	})
	require.NoError(t, err)

	const goroutines = 8
	var wins, conflicts int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			_, err := repo.SessionTurns.AppendTurn(ctx, p, repo.Runs, repo.Sessions, repo.AppendTurnCmd{
				SessionID:        sess.ID,
				OwnerNamespace:   ns,
				IdempotencyKey:   fmt.Sprintf("turn-race-%d", i),
				Profile:          "admin",
				ExpectedRevision: 1,
			})
			mu.Lock()
			if err == nil {
				wins++
			} else {
				conflicts++
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int64(1), wins, "exactly one turn must win the revision CAS")
	assert.Equal(t, int64(goroutines-1), conflicts, "all others must receive a conflict")
}

// ── BDD: cross-namespace denied ───────────────────────────────────────────────

func TestAppendTurnCrossNamespaceDenied(t *testing.T) {
	ctx := context.Background()
	p := pool(t)

	ownerNS := "ns-turn-own-" + uuid.NewString()[:8]
	otherNS := "ns-turn-other-" + uuid.NewString()[:8]

	sess, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: ownerNS,
		Profile:        "admin",
	})
	require.NoError(t, err)

	_, err = repo.SessionTurns.AppendTurn(ctx, p, repo.Runs, repo.Sessions, repo.AppendTurnCmd{
		SessionID:        sess.ID,
		OwnerNamespace:   otherNS, // wrong namespace
		IdempotencyKey:   "turn-idor",
		Profile:          "admin",
		ExpectedRevision: 1,
	})
	require.ErrorIs(t, err, repo.ErrNotFound,
		"wrong namespace must receive ErrNotFound, not create a turn")
}

// ── BDD: ListTurns returns turns in order ────────────────────────────────────

func TestListTurnsOrder(t *testing.T) {
	ctx := context.Background()
	p := pool(t)

	ns := "ns-turn-list-" + uuid.NewString()[:8]
	sess, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: ns,
		Profile:        "admin",
	})
	require.NoError(t, err)

	// Append 3 turns sequentially.
	for i := range 3 {
		r, err := repo.SessionTurns.AppendTurn(ctx, p, repo.Runs, repo.Sessions, repo.AppendTurnCmd{
			SessionID:        sess.ID,
			OwnerNamespace:   ns,
			IdempotencyKey:   fmt.Sprintf("turn-%d", i),
			Profile:          "admin",
			ExpectedRevision: int64(i + 1),
		})
		require.NoError(t, err)
		assert.Equal(t, int64(i+2), r.Turn.TurnSequence)
	}

	turns, err := repo.SessionTurns.ListTurns(ctx, p, sess.ID, ns, 10)
	require.NoError(t, err)
	require.Len(t, turns, 3)

	// Must be in ascending turn_sequence order.
	for i := 1; i < len(turns); i++ {
		assert.Greater(t, turns[i].TurnSequence, turns[i-1].TurnSequence,
			"turns must be listed in ascending sequence order")
	}
}
