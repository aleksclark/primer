package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

func TestExpireOpenWindowsClosesFutureAndCurrentOffers(t *testing.T) {
	t.Parallel()
	q := querier(t)
	ctx := context.Background()
	now := time.Now().UTC()

	item := factory.MediaItem(t, q)
	current := factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": item.ID,
		"starts_at":     now.Add(-time.Hour),
		"ends_at":       now.Add(48 * time.Hour),
	})
	// A window that has not opened yet still has to be closable, which the
	// ends_at > starts_at constraint makes non-obvious.
	future := factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": factory.MediaItem(t, q).ID,
		"starts_at":     now.Add(24 * time.Hour),
		"ends_at":       now.Add(72 * time.Hour),
	})
	past := factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": factory.MediaItem(t, q).ID,
		"starts_at":     now.AddDate(0, 0, -10),
		"ends_at":       now.AddDate(0, 0, -3),
	})

	closed, err := tvrepo.ExpireOpenWindows(ctx, q, now)
	require.NoError(t, err)
	assert.Equal(t, 2, closed, "the already-closed window is left alone")

	after, err := tvrepo.AvailabilityWindows.Get(ctx, q, current.ID)
	require.NoError(t, err)
	assert.False(t, after.EndsAt.After(now))

	stillFuture, err := tvrepo.AvailabilityWindows.Get(ctx, q, future.ID)
	require.NoError(t, err)
	assert.False(t, stillFuture.EndsAt.After(now))
	assert.False(t, stillFuture.StartsAt.After(stillFuture.EndsAt), "the span stays valid")

	untouched, err := tvrepo.AvailabilityWindows.Get(ctx, q, past.ID)
	require.NoError(t, err)
	assert.WithinDuration(t, past.EndsAt, untouched.EndsAt, time.Second)
}

func TestSuggestRotationPrefersTheLongestUnoffered(t *testing.T) {
	t.Parallel()
	q := querier(t)
	ctx := context.Background()
	now := time.Now().UTC()

	recent := factory.MediaItem(t, q, factory.Override{"title": "Shown recently"})
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": recent.ID,
		"starts_at":     now.AddDate(0, 0, -9),
		"ends_at":       now.AddDate(0, 0, -2),
	})
	old := factory.MediaItem(t, q, factory.Override{"title": "Shown long ago"})
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": old.ID,
		"starts_at":     now.AddDate(0, 0, -60),
		"ends_at":       now.AddDate(0, 0, -50),
	})

	got, err := tvrepo.SuggestRotation(ctx, q, now, 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(got), 2)

	var oldIdx, recentIdx = -1, -1
	for i, c := range got {
		switch c.ID {
		case old.ID:
			oldIdx = i
		case recent.ID:
			recentIdx = i
		}
	}
	require.NotEqual(t, -1, oldIdx)
	require.NotEqual(t, -1, recentIdx)
	assert.Less(t, oldIdx, recentIdx, "the longest-unseen title is offered back first")
}

func TestSuggestRotationSkipsItemsWithNoRuntime(t *testing.T) {
	t.Parallel()
	q := querier(t)

	unknown := factory.MediaItem(t, q, factory.Override{"runtime_seconds": 0})

	got, err := tvrepo.SuggestRotation(context.Background(), q, time.Now().UTC(), 20)
	require.NoError(t, err)
	for _, c := range got {
		assert.NotEqual(t, unknown.ID, c.ID, "an item of unknown length cannot be scheduled or offered")
	}
}

func TestOpenWindowsForIsIdempotentAndBounded(t *testing.T) {
	t.Parallel()
	q := querier(t)
	ctx := context.Background()
	now := time.Now().UTC()

	item := factory.MediaItem(t, q)

	opened, err := tvrepo.OpenWindowsFor(ctx, q, []string{item.ID}, now, now.AddDate(0, 0, 7))
	require.NoError(t, err)
	require.Len(t, opened, 1)

	again, err := tvrepo.OpenWindowsFor(ctx, q, []string{item.ID}, now, now.AddDate(0, 0, 7))
	require.NoError(t, err)
	assert.Empty(t, again, "an item already on offer is not offered twice")

	none, err := tvrepo.OpenWindowsFor(ctx, q, nil, now, now.AddDate(0, 0, 7))
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestOpenWindowsForSkipsOrphanedItems(t *testing.T) {
	t.Parallel()
	q := querier(t)
	now := time.Now().UTC()

	gone := factory.MediaItem(t, q, factory.Override{"orphaned_at": now})

	opened, err := tvrepo.OpenWindowsFor(context.Background(), q, []string{gone.ID}, now, now.AddDate(0, 0, 7))
	require.NoError(t, err)
	assert.Empty(t, opened, "Jellyfin no longer has the file, so it cannot be offered")
}

func TestMetricsQueriesOnAnIdleChannel(t *testing.T) {
	t.Parallel()
	q := querier(t)
	ctx := context.Background()
	now := time.Now().UTC()
	from, to := now.AddDate(0, 0, -7), now

	byClass, err := tvrepo.WatchTimeByClassBetween(ctx, q, from, to)
	require.NoError(t, err)
	assert.Empty(t, byClass)

	bySubject, err := tvrepo.WatchTimeBySubjectBetween(ctx, q, from, to)
	require.NoError(t, err)
	assert.Empty(t, bySubject)

	byDay, err := tvrepo.WatchTimeByDayBetween(ctx, q, from, to, "America/Chicago")
	require.NoError(t, err)
	assert.Empty(t, byDay)

	completion, err := tvrepo.CompletionStatsBetween(ctx, q, from, to)
	require.NoError(t, err)
	assert.Zero(t, completion.Sessions)

	usage, err := tvrepo.EntertainmentUsageBetween(ctx, q, from, to)
	require.NoError(t, err)
	assert.Zero(t, usage.PlaysUsed)
}

func TestWatchTimeByDayBucketsInTheChannelZone(t *testing.T) {
	t.Parallel()
	q := querier(t)
	ctx := context.Background()

	// 02:00 UTC is still the previous evening in Chicago. Bucketing in UTC
	// would file this viewing under the wrong school day.
	watchedAt := time.Date(2025, 3, 5, 2, 0, 0, 0, time.UTC)
	item := factory.MediaItem(t, q)
	factory.PlaybackSession(t, q, factory.Override{
		"media_item_id":   item.ID,
		"started_at":      watchedAt,
		"watched_seconds": 600,
	})

	from := watchedAt.AddDate(0, 0, -2)
	to := watchedAt.AddDate(0, 0, 2)

	chicago, err := tvrepo.WatchTimeByDayBetween(ctx, q, from, to, "America/Chicago")
	require.NoError(t, err)
	require.Len(t, chicago, 1)
	assert.Equal(t, "2025-03-04", chicago[0].Day.Format("2006-01-02"))

	utc, err := tvrepo.WatchTimeByDayBetween(ctx, q, from, to, "UTC")
	require.NoError(t, err)
	require.Len(t, utc, 1)
	assert.Equal(t, "2025-03-05", utc[0].Day.Format("2006-01-02"))
}
