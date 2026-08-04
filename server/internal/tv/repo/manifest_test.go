package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

func TestUpsertManifestDesiredPreservesAttempts(t *testing.T) {
	t.Parallel()
	q := tvtestutil.Tx(t)
	ctx := context.Background()

	created, err := tvrepo.UpsertManifestDesired(ctx, q, map[string]any{
		"slug":  "living-planet",
		"title": "The Living Planet",
		"kind":  domain.ManifestKindSeries,
		"class": domain.ClassEducational,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.ManifestStatusMissing, created.Status)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	attempted, err := tvrepo.RecordManifestAttempt(ctx, q, "living-planet", "still downloading", tvrepo.ManifestFailPolicy{
		MaxAttempts: 10, MaxDays: 14, Now: now,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, attempted.AttemptCount)

	updated, err := tvrepo.UpsertManifestDesired(ctx, q, map[string]any{
		"slug":    "living-planet",
		"title":   "The Living Planet (remaster)",
		"kind":    domain.ManifestKindSeries,
		"class":   domain.ClassEducational,
		"tvdb_id": 79165,
	})
	require.NoError(t, err)
	assert.Equal(t, "The Living Planet (remaster)", updated.Title)
	assert.Equal(t, 79165, updated.TVDBID)
	assert.Equal(t, 1, updated.AttemptCount, "desired-state sync must not reset attempts")
	assert.Equal(t, domain.ManifestStatusMissing, updated.Status)
}

func TestRecordManifestAttemptFailsOnMaxAttempts(t *testing.T) {
	t.Parallel()
	q := tvtestutil.Tx(t)
	ctx := context.Background()
	factory.ContentManifestEntry(t, q, factory.Override{
		"slug": "scarce-title", "title": "Scarce",
	})

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	var last *domain.ContentManifestEntry
	for i := range 3 {
		entry, err := tvrepo.RecordManifestAttempt(ctx, q, "scarce-title", "no indexer hit", tvrepo.ManifestFailPolicy{
			MaxAttempts: 3, MaxDays: 0, Now: now.Add(time.Duration(i) * time.Hour),
		})
		require.NoError(t, err)
		last = entry
	}
	require.NotNil(t, last)
	assert.Equal(t, 3, last.AttemptCount)
	assert.Equal(t, domain.ManifestStatusFailed, last.Status)
	require.NotNil(t, last.FailedAt)
}

func TestRecordManifestAttemptFailsOnMaxDays(t *testing.T) {
	t.Parallel()
	q := tvtestutil.Tx(t)
	ctx := context.Background()
	factory.ContentManifestEntry(t, q, factory.Override{
		"slug": "slow-title", "title": "Slow",
	})

	first := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	_, err := tvrepo.RecordManifestAttempt(ctx, q, "slow-title", "", tvrepo.ManifestFailPolicy{
		MaxAttempts: 0, MaxDays: 7, Now: first,
	})
	require.NoError(t, err)

	later := first.Add(7 * 24 * time.Hour)
	entry, err := tvrepo.RecordManifestAttempt(ctx, q, "slow-title", "still missing", tvrepo.ManifestFailPolicy{
		MaxAttempts: 0, MaxDays: 7, Now: later,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.ManifestStatusFailed, entry.Status)
	assert.Equal(t, 2, entry.AttemptCount)
}

func TestMarkManifestPresent(t *testing.T) {
	t.Parallel()
	q := tvtestutil.Tx(t)
	ctx := context.Background()
	factory.ContentManifestEntry(t, q, factory.Override{
		"slug": "matrix", "title": "The Matrix", "attempt_count": 4,
	})

	at := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	entry, err := tvrepo.MarkManifestPresent(ctx, q, "matrix", at)
	require.NoError(t, err)
	assert.Equal(t, domain.ManifestStatusPresent, entry.Status)
	require.NotNil(t, entry.PresentAt)
	assert.True(t, entry.PresentAt.Equal(at))
	assert.Empty(t, entry.LastError)

	// Idempotent: second mark keeps original present_at.
	later := at.Add(time.Hour)
	again, err := tvrepo.MarkManifestPresent(ctx, q, "matrix", later)
	require.NoError(t, err)
	require.NotNil(t, again.PresentAt)
	assert.True(t, again.PresentAt.Equal(at))
}

func TestRecordAttemptNoopsOnPresent(t *testing.T) {
	t.Parallel()
	q := tvtestutil.Tx(t)
	ctx := context.Background()
	now := time.Now().UTC()
	factory.ContentManifestEntry(t, q, factory.Override{
		"slug": "done", "status": domain.ManifestStatusPresent, "present_at": now, "attempt_count": 2,
	})

	entry, err := tvrepo.RecordManifestAttempt(ctx, q, "done", "should ignore", tvrepo.ManifestFailPolicy{
		MaxAttempts: 1, MaxDays: 1, Now: now,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, entry.AttemptCount)
	assert.Equal(t, domain.ManifestStatusPresent, entry.Status)
}

func TestContentManifestBySlugNotFound(t *testing.T) {
	t.Parallel()
	q := tvtestutil.Tx(t)
	_, err := tvrepo.ContentManifestBySlug(context.Background(), q, "nope")
	assert.ErrorIs(t, err, baserepo.ErrNotFound)
}

func TestManualKindStartsManual(t *testing.T) {
	t.Parallel()
	q := tvtestutil.Tx(t)
	entry, err := tvrepo.UpsertManifestDesired(context.Background(), q, map[string]any{
		"slug":  "bernstein",
		"title": "Bernstein",
		"kind":  domain.ManifestKindManual,
		"class": domain.ClassEducational,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.ManifestStatusManual, entry.Status)
}
