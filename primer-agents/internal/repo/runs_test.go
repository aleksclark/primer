package repo_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
)

// makeRun creates a run and returns it.
func makeRun(t *testing.T, ctx context.Context, namespace, key, hash string) *domain.Run {
	t.Helper()
	p := pool(t)
	run, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace:  namespace,
		IdempotencyKey:  key,
		IdempotencyHash: hash,
		Profile:         "tutor",
	})
	require.NoError(t, err)
	return run
}

func TestRunCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	run := makeRun(t, ctx, "ns-run-create", "key-1", "a"+makeHex(63))
	assert.NotEmpty(t, run.ID)
	assert.Equal(t, "queued", string(run.Status))
	assert.Equal(t, int64(1), run.StateVersion)
	assert.Equal(t, "tutor", run.Profile)
}

func TestRunCreateIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	hash := "b" + makeHex(63)

	r1 := makeRun(t, ctx, "ns-run-idem", "key-idem", hash)
	r2 := makeRun(t, ctx, "ns-run-idem", "key-idem", hash)

	assert.Equal(t, r1.ID, r2.ID, "idempotent create must return same ID")
}

func TestRunCreateIdempotentConcurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	hash := "c" + makeHex(63)
	const goroutines = 8

	ids := make([]string, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			r, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
				OwnerNamespace:  "ns-run-concurrent",
				IdempotencyKey:  "key-concurrent",
				IdempotencyHash: hash,
				Profile:         "tutor",
			})
			if err == nil {
				ids[i] = r.ID
			}
			errs[i] = err
		}(i)
	}
	wg.Wait()

	// All goroutines must succeed (idempotent) and return the same ID.
	var firstID string
	for i, err := range errs {
		require.NoError(t, err, "goroutine %d", i)
		if firstID == "" {
			firstID = ids[i]
		} else {
			assert.Equal(t, firstID, ids[i], "all concurrent creates must return same run ID")
		}
	}
}

func TestRunCreateConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	hash1 := "d" + makeHex(63)
	hash2 := "e" + makeHex(63)

	_, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace:  "ns-run-conflict",
		IdempotencyKey:  "key-conflict",
		IdempotencyHash: hash1,
		Profile:         "tutor",
	})
	require.NoError(t, err)

	_, err = repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace:  "ns-run-conflict",
		IdempotencyKey:  "key-conflict",
		IdempotencyHash: hash2, // different hash → conflict
		Profile:         "tutor",
	})
	require.ErrorIs(t, err, repo.ErrConflict)
}

func TestRunCreateNamespaceIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	hash := "f" + makeHex(63)

	r1, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace:  "ns-iso-A",
		IdempotencyKey:  "shared-key",
		IdempotencyHash: hash,
		Profile:         "tutor",
	})
	require.NoError(t, err)

	// Same key, different namespace → independent run.
	r2, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace:  "ns-iso-B",
		IdempotencyKey:  "shared-key",
		IdempotencyHash: hash,
		Profile:         "tutor",
	})
	require.NoError(t, err)
	assert.NotEqual(t, r1.ID, r2.ID, "different namespaces must produce independent runs")
}

func TestRunGetNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	_, err := repo.Runs.Get(ctx, p, "00000000-0000-0000-0000-000000000000", "ns-miss", "")
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestRunTransitionQueuedToRunning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	run := makeRun(t, ctx, "ns-trans-running", "key-tr", "g"+makeHex(63))

	updated, err := repo.Runs.Transition(ctx, pool(t), repo.TransitionCmd{
		RunID:           run.ID,
		OwnerNamespace:  "ns-trans-running",
		ExpectedStatus:  domain.RunStatusQueued,
		ExpectedVersion: run.StateVersion,
		NewStatus:       domain.RunStatusRunning,
	})
	require.NoError(t, err)
	assert.Equal(t, "running", string(updated.Status))
	assert.Equal(t, int64(2), updated.StateVersion)
	assert.NotNil(t, updated.StartedAt)
}

func TestRunTransitionTerminal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)

	cases := []struct {
		terminal domain.RunStatus
		result   string
	}{
		{domain.RunStatusSucceeded, "ok"},
		{domain.RunStatusFailed, "error"},
		{domain.RunStatusInterrupted, "interrupted"},
	}
	for _, tc := range cases {
		t.Run(string(tc.terminal), func(t *testing.T) {
			t.Parallel()
			ns := "ns-term-" + string(tc.terminal)
			run := makeRun(t, ctx, ns, "key-term", "h"+makeHex(63))

			// queued→running
			running, err := repo.Runs.Transition(ctx, p, repo.TransitionCmd{
				RunID: run.ID, OwnerNamespace: ns,
				ExpectedStatus: domain.RunStatusQueued, ExpectedVersion: run.StateVersion,
				NewStatus: domain.RunStatusRunning,
			})
			require.NoError(t, err)

			// running→terminal
			rc := tc.result
			term, err := repo.Runs.Transition(ctx, p, repo.TransitionCmd{
				RunID: run.ID, OwnerNamespace: ns,
				ExpectedStatus: domain.RunStatusRunning, ExpectedVersion: running.StateVersion,
				NewStatus:   tc.terminal,
				ResultClass: &rc,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.terminal, term.Status)
			assert.NotNil(t, term.EndedAt)
		})
	}
}

func TestRunTransitionRejectsInvalid(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	run := makeRun(t, ctx, "ns-bad-trans", "key-bad", "i"+makeHex(63))

	// queued→succeeded is not in the state machine.
	_, err := repo.Runs.Transition(ctx, pool(t), repo.TransitionCmd{
		RunID: run.ID, OwnerNamespace: "ns-bad-trans",
		ExpectedStatus: domain.RunStatusQueued, ExpectedVersion: run.StateVersion,
		NewStatus: domain.RunStatusSucceeded,
	})
	require.ErrorIs(t, err, repo.ErrInvalidTransition)
}

func TestRunTransitionRejectsTerminalOverwrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	ns := "ns-term-overwrite"
	run := makeRun(t, ctx, ns, "key-ov", "j"+makeHex(63))

	// queued→running
	running, err := repo.Runs.Transition(ctx, p, repo.TransitionCmd{
		RunID: run.ID, OwnerNamespace: ns,
		ExpectedStatus: domain.RunStatusQueued, ExpectedVersion: run.StateVersion,
		NewStatus: domain.RunStatusRunning,
	})
	require.NoError(t, err)

	// running→succeeded
	term, err := repo.Runs.Transition(ctx, p, repo.TransitionCmd{
		RunID: run.ID, OwnerNamespace: ns,
		ExpectedStatus: domain.RunStatusRunning, ExpectedVersion: running.StateVersion,
		NewStatus: domain.RunStatusSucceeded,
	})
	require.NoError(t, err)

	// Attempt to overwrite terminal with another terminal → must fail.
	_, err = repo.Runs.Transition(ctx, p, repo.TransitionCmd{
		RunID: run.ID, OwnerNamespace: ns,
		ExpectedStatus: domain.RunStatusSucceeded, ExpectedVersion: term.StateVersion,
		NewStatus: domain.RunStatusFailed,
	})
	require.ErrorIs(t, err, repo.ErrInvalidTransition,
		"terminal state must not be overwritable")
}

func TestRunTransitionStaleVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	run := makeRun(t, ctx, "ns-stale-ver", "key-sv", "k"+makeHex(63))

	_, err := repo.Runs.Transition(ctx, pool(t), repo.TransitionCmd{
		RunID: run.ID, OwnerNamespace: "ns-stale-ver",
		ExpectedStatus:  domain.RunStatusQueued,
		ExpectedVersion: run.StateVersion + 99,
		NewStatus:       domain.RunStatusRunning,
	})
	require.ErrorIs(t, err, repo.ErrStaleVersion)
}

func TestRunRequestCancel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	run := makeRun(t, ctx, "ns-cancel", "key-cancel", "l"+makeHex(63))

	rc := "user_request"
	updated, err := repo.Runs.RequestCancel(ctx, pool(t), repo.RequestCancelCmd{
		RunID: run.ID, OwnerNamespace: "ns-cancel", ReasonClass: &rc,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusCancelRequested, updated.Status)
	assert.NotNil(t, updated.CancelRequestedAt)
	assert.Equal(t, &rc, updated.CancelReasonClass)
}

func TestRunRequestCancelIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	run := makeRun(t, ctx, "ns-cancel-idem", "key-ci", "m"+makeHex(63))

	r1, err := repo.Runs.RequestCancel(ctx, p, repo.RequestCancelCmd{
		RunID: run.ID, OwnerNamespace: "ns-cancel-idem",
	})
	require.NoError(t, err)

	r2, err := repo.Runs.RequestCancel(ctx, p, repo.RequestCancelCmd{
		RunID: run.ID, OwnerNamespace: "ns-cancel-idem",
	})
	require.NoError(t, err)
	assert.Equal(t, r1.ID, r2.ID)
	assert.Equal(t, domain.RunStatusCancelRequested, r2.Status)
}

func TestRunRequestCancelOnTerminalRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	ns := "ns-cancel-term"
	run := makeRun(t, ctx, ns, "key-ct", "n"+makeHex(63))

	// queued→running→succeeded
	running, err := repo.Runs.Transition(ctx, p, repo.TransitionCmd{
		RunID: run.ID, OwnerNamespace: ns,
		ExpectedStatus: domain.RunStatusQueued, ExpectedVersion: run.StateVersion,
		NewStatus: domain.RunStatusRunning,
	})
	require.NoError(t, err)
	_, err = repo.Runs.Transition(ctx, p, repo.TransitionCmd{
		RunID: run.ID, OwnerNamespace: ns,
		ExpectedStatus: domain.RunStatusRunning, ExpectedVersion: running.StateVersion,
		NewStatus: domain.RunStatusSucceeded,
	})
	require.NoError(t, err)

	_, err = repo.Runs.RequestCancel(ctx, p, repo.RequestCancelCmd{
		RunID: run.ID, OwnerNamespace: ns,
	})
	require.ErrorIs(t, err, repo.ErrInvalidTransition,
		"cancel on a terminal run must be rejected")
}

func TestRunListByOwner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	ns := "ns-list-owner"

	for i := range 3 {
		_, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
			OwnerNamespace:  ns,
			IdempotencyKey:  fmt.Sprintf("key-list-%d", i),
			IdempotencyHash: makeHex64(i),
			Profile:         "tutor",
		})
		require.NoError(t, err)
	}

	runs, err := repo.Runs.ListByOwner(ctx, p, ns, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(runs), 3)
	for _, r := range runs {
		assert.Equal(t, ns, r.OwnerNamespace)
	}
}

// makeHex returns a hex string of length n filled with 'a'.
func makeHex(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

// makeHex64 returns a deterministic 64-char lowercase hex string based on i.
func makeHex64(i int) string {
	return fmt.Sprintf("%064x", i+100)
}
