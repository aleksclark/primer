package repo_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/repo"
)

func TestEventAppendAndList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	ns := "ns-ev-basic"

	run, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-ev-basic",
		IdempotencyHash: "e001" + makeHex(60), Profile: "tutor",
	})
	require.NoError(t, err)

	// Append 3 events in-transaction.
	for i := range 3 {
		kind := fmt.Sprintf("test.event.%d", i)
		err := repo.WithTx(ctx, p, func(q repo.Querier) error {
			_, err := repo.Events.Append(ctx, q, repo.AppendEventCmd{
				RunID: run.ID, OwnerNamespace: ns, Kind: kind,
			})
			return err
		})
		require.NoError(t, err)
	}

	evs, err := repo.Events.List(ctx, p, run.ID, ns, 0, 100)
	require.NoError(t, err)
	require.Len(t, evs, 3)
	// Sequences must be strictly increasing starting at 1.
	for i, ev := range evs {
		assert.Equal(t, int64(i+1), ev.Sequence, "event %d sequence", i)
	}
}

func TestEventSequenceStrictlyMonotonic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	ns := "ns-ev-mono"

	run, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-ev-mono",
		IdempotencyHash: "e002" + makeHex(60), Profile: "tutor",
	})
	require.NoError(t, err)

	const n = 20
	for range n {
		_ = repo.WithTx(ctx, p, func(q repo.Querier) error {
			_, err := repo.Events.Append(ctx, q, repo.AppendEventCmd{
				RunID: run.ID, OwnerNamespace: ns, Kind: "ping",
			})
			return err
		})
	}

	evs, err := repo.Events.List(ctx, p, run.ID, ns, 0, 200)
	require.NoError(t, err)
	require.Len(t, evs, n)
	for i := 1; i < len(evs); i++ {
		assert.Greater(t, evs[i].Sequence, evs[i-1].Sequence,
			"event sequences must be strictly increasing")
	}
}

func TestEventConcurrentAppendNoGaps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	ns := "ns-ev-concurrent"

	run, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-ev-conc",
		IdempotencyHash: "e003" + makeHex(60), Profile: "tutor",
	})
	require.NoError(t, err)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_ = repo.WithTx(ctx, p, func(q repo.Querier) error {
				_, err := repo.Events.Append(ctx, q, repo.AppendEventCmd{
					RunID: run.ID, OwnerNamespace: ns, Kind: "concurrent",
				})
				return err
			})
		}()
	}
	wg.Wait()

	evs, err := repo.Events.List(ctx, p, run.ID, ns, 0, 200)
	require.NoError(t, err)
	require.Len(t, evs, goroutines)

	seqSet := make(map[int64]struct{}, goroutines)
	for _, ev := range evs {
		_, dup := seqSet[ev.Sequence]
		assert.False(t, dup, "duplicate sequence %d", ev.Sequence)
		seqSet[ev.Sequence] = struct{}{}
	}
}

func TestEventCursorPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	ns := "ns-ev-cursor"

	run, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-ev-cursor",
		IdempotencyHash: "e004" + makeHex(60), Profile: "tutor",
	})
	require.NoError(t, err)

	for range 10 {
		_ = repo.WithTx(ctx, p, func(q repo.Querier) error {
			_, err := repo.Events.Append(ctx, q, repo.AppendEventCmd{
				RunID: run.ID, OwnerNamespace: ns, Kind: "page",
			})
			return err
		})
	}

	// Page 1: afterSeq=0, limit=4 → sequences 1..4
	page1, err := repo.Events.List(ctx, p, run.ID, ns, 0, 4)
	require.NoError(t, err)
	require.Len(t, page1, 4)
	assert.Equal(t, int64(1), page1[0].Sequence)

	// Page 2: afterSeq = last of page1 → next 4
	page2, err := repo.Events.List(ctx, p, run.ID, ns, page1[len(page1)-1].Sequence, 4)
	require.NoError(t, err)
	require.Len(t, page2, 4)
	assert.Equal(t, page1[len(page1)-1].Sequence+1, page2[0].Sequence)

	// No duplicates across pages.
	seen := make(map[int64]struct{})
	for _, ev := range append(page1, page2...) {
		_, dup := seen[ev.Sequence]
		assert.False(t, dup, "duplicate sequence %d across pages", ev.Sequence)
		seen[ev.Sequence] = struct{}{}
	}
}

func TestEventOwnershipEnforced(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)
	ns := "ns-ev-own"

	run, err := repo.Runs.Create(ctx, p, repo.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-ev-own",
		IdempotencyHash: "e005" + makeHex(60), Profile: "tutor",
	})
	require.NoError(t, err)

	// Wrong namespace cannot append.
	err = repo.WithTx(ctx, p, func(q repo.Querier) error {
		_, err := repo.Events.Append(ctx, q, repo.AppendEventCmd{
			RunID: run.ID, OwnerNamespace: "ns-ev-other", Kind: "ping",
		})
		return err
	})
	require.ErrorIs(t, err, repo.ErrNotFound)

	// Wrong namespace cannot list.
	evs, err := repo.Events.List(ctx, p, run.ID, "ns-ev-other", 0, 100)
	require.NoError(t, err)
	assert.Empty(t, evs, "wrong namespace must not see events")
}
