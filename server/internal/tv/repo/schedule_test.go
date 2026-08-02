package repo_test

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// gridBase is a fixed instant the schedule tests anchor their grids to, well
// clear of the factory's own "now"-relative default airings.
var gridBase = time.Date(2031, 4, 8, 9, 0, 0, 0, time.UTC)

// schedule places an item of the given runtime at an offset from gridBase.
func schedule(t *testing.T, q baserepo.Querier, offset time.Duration, runtimeSeconds int, overrides ...factory.Override) (*domain.ScheduleEntry, *domain.MediaItem) {
	t.Helper()
	item := factory.MediaItem(t, q, factory.Override{"runtime_seconds": runtimeSeconds})
	merged := factory.Override{"media_item_id": item.ID, "airs_at": gridBase.Add(offset)}
	for _, o := range overrides {
		maps.Copy(merged, o)
	}
	return factory.ScheduleEntry(t, q, merged), item
}

func TestAiringAtResolvesTheCurrentProgramme(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	entry, item := schedule(t, q, 0, 1800)
	schedule(t, q, 40*time.Minute, 1800)

	airing, err := tvrepo.AiringAt(ctx, q, gridBase.Add(10*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, entry.ID, airing.ID)
	assert.Equal(t, item.Title, airing.Title)
	assert.Equal(t, 1800, airing.RuntimeSeconds)
	assert.Equal(t, gridBase.Add(30*time.Minute).UTC(), airing.EndsAt.UTC(),
		"the slot ends one runtime after it starts")
	assert.Equal(t, 600, airing.OffsetSecondsAt(gridBase.Add(10*time.Minute)))
}

func TestAiringAtIsHalfOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	entry, _ := schedule(t, q, 0, 1800)

	first, err := tvrepo.AiringAt(ctx, q, gridBase)
	require.NoError(t, err, "the programme is on air at its own start")
	assert.Equal(t, entry.ID, first.ID)

	last, err := tvrepo.AiringAt(ctx, q, gridBase.Add(1799*time.Second))
	require.NoError(t, err)
	assert.Equal(t, entry.ID, last.ID)

	_, err = tvrepo.AiringAt(ctx, q, gridBase.Add(1800*time.Second))
	assert.ErrorIs(t, err, baserepo.ErrNotFound, "the end of the slot is exclusive")
}

func TestAiringAtReportsAGapAsNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	schedule(t, q, 0, 600)
	schedule(t, q, time.Hour, 600)

	_, err := tvrepo.AiringAt(ctx, q, gridBase.Add(30*time.Minute))
	assert.ErrorIs(t, err, baserepo.ErrNotFound)
}

func TestAiringAtIgnoresItemsWithUnknownRuntime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	schedule(t, q, 0, 0)

	_, err := tvrepo.AiringAt(ctx, q, gridBase.Add(time.Minute))
	assert.ErrorIs(t, err, baserepo.ErrNotFound,
		"an item of unknown length has an empty slot and never airs")
}

func TestNextAiringAfterFindsTheUpcomingProgramme(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	schedule(t, q, 0, 600)
	soon, _ := schedule(t, q, time.Hour, 600)
	schedule(t, q, 3*time.Hour, 600)

	next, err := tvrepo.NextAiringAfter(ctx, q, gridBase.Add(10*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, soon.ID, next.ID)

	_, err = tvrepo.NextAiringAfter(ctx, q, gridBase.Add(24*time.Hour))
	assert.ErrorIs(t, err, baserepo.ErrNotFound, "an empty tail of the grid is a gap")
}

func TestAiringsBetweenIncludesProgrammesStraddlingTheBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	// Starts before the window and runs into it.
	straddling, _ := schedule(t, q, -30*time.Minute, 3600)
	inside, _ := schedule(t, q, 2*time.Hour, 1800)
	// Ends exactly at the window's start: outside, since spans are half-open.
	schedule(t, q, -2*time.Hour, 3600)
	// Starts exactly at the window's end: outside.
	schedule(t, q, 6*time.Hour, 1800)

	airings, err := tvrepo.AiringsBetween(ctx, q, gridBase, gridBase.Add(6*time.Hour))
	require.NoError(t, err)

	ids := make([]string, len(airings))
	for i, a := range airings {
		ids[i] = a.ID
	}
	assert.Equal(t, []string{straddling.ID, inside.ID}, ids,
		"a programme playing across the boundary belongs to the day it is playing")
}

func TestCreateScheduleEntryRejectsOverlaps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	_, first := schedule(t, q, 0, 3600)
	clashing := factory.MediaItem(t, q, factory.Override{"runtime_seconds": 600})

	_, err := tvrepo.CreateScheduleEntry(ctx, q, clashing.ID, gridBase.Add(30*time.Minute), true, domain.BlockMorning)
	assert.ErrorIs(t, err, tvrepo.ErrScheduleConflict, "a start inside another slot conflicts")

	// A candidate that starts before the existing airing but runs into it.
	long := factory.MediaItem(t, q, factory.Override{"runtime_seconds": 3600})
	_, err = tvrepo.CreateScheduleEntry(ctx, q, long.ID, gridBase.Add(-30*time.Minute), true, domain.BlockMorning)
	assert.ErrorIs(t, err, tvrepo.ErrScheduleConflict, "a candidate running into another slot conflicts")

	// Back to back is fine: the previous slot's end is exclusive.
	entry, err := tvrepo.CreateScheduleEntry(ctx, q, clashing.ID, gridBase.Add(time.Hour), true, domain.BlockMidday)
	require.NoError(t, err)
	assert.Equal(t, gridBase.Add(time.Hour).UTC(), entry.AirsAt.UTC())
	assert.Equal(t, domain.BlockMidday, entry.Block)
	assert.NotEqual(t, first.ID, entry.MediaItemID)
}

func TestCreateScheduleEntryAllowsARepeatAiringOfTheSameItem(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	_, item := schedule(t, q, 0, 1800)

	entry, err := tvrepo.CreateScheduleEntry(ctx, q, item.ID, gridBase.Add(4*time.Hour), false, domain.BlockAfternoon)
	require.NoError(t, err)
	assert.False(t, entry.JoinInProgress)
	assert.Equal(t, item.ID, entry.MediaItemID)
}

func TestUpdateScheduleEntryDoesNotConflictWithItself(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	entry, item := schedule(t, q, 0, 3600)

	moved, err := tvrepo.UpdateScheduleEntry(ctx, q, entry.ID, item.ID, gridBase.Add(10*time.Minute), true, domain.BlockMorning)
	require.NoError(t, err, "nudging an entry must not collide with its own old slot")
	assert.Equal(t, gridBase.Add(10*time.Minute).UTC(), moved.AirsAt.UTC())
}

func TestUpdateScheduleEntryRejectsAMoveOntoAnotherAiring(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	entry, item := schedule(t, q, 0, 1800)
	schedule(t, q, 2*time.Hour, 1800)

	_, err := tvrepo.UpdateScheduleEntry(ctx, q, entry.ID, item.ID, gridBase.Add(2*time.Hour+10*time.Minute), true, domain.BlockMidday)
	assert.ErrorIs(t, err, tvrepo.ErrScheduleConflict)

	// The entry is untouched by the refused move.
	unchanged, err := tvrepo.ScheduleEntries.Get(ctx, q, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, gridBase.UTC(), unchanged.AirsAt.UTC())
}

func TestConflictingAiringsNamesTheObstruction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	blocker, blockerItem := schedule(t, q, 0, 3600)
	candidate := factory.MediaItem(t, q, factory.Override{"runtime_seconds": 600})

	conflicts, err := tvrepo.ConflictingAirings(ctx, q, candidate.ID, gridBase.Add(30*time.Minute), "")
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	assert.Equal(t, blocker.ID, conflicts[0].ID)
	assert.Equal(t, blockerItem.Title, conflicts[0].Title)

	excluded, err := tvrepo.ConflictingAirings(ctx, q, candidate.ID, gridBase.Add(30*time.Minute), blocker.ID)
	require.NoError(t, err)
	assert.Empty(t, excluded, "the entry being edited is not its own conflict")
}

func TestStartOffsetHonoursJoinInProgress(t *testing.T) {
	t.Parallel()

	joinable := tvrepo.Airing{
		ScheduleEntry:  domain.ScheduleEntry{AirsAt: gridBase, JoinInProgress: true},
		RuntimeSeconds: 1800,
	}
	assert.Equal(t, 300, joinable.StartOffsetSecondsAt(gridBase.Add(5*time.Minute)))
	assert.Equal(t, 1800, joinable.StartOffsetSecondsAt(gridBase.Add(time.Hour)),
		"the offset is clamped to the programme's own length")
	assert.Equal(t, 0, joinable.StartOffsetSecondsAt(gridBase.Add(-time.Minute)),
		"an instant before the start is the start")

	fromTheTop := tvrepo.Airing{
		ScheduleEntry:  domain.ScheduleEntry{AirsAt: gridBase, JoinInProgress: false},
		RuntimeSeconds: 1800,
	}
	assert.Equal(t, 0, fromTheTop.StartOffsetSecondsAt(gridBase.Add(5*time.Minute)),
		"an entry that forbids joining late always starts from the beginning")
	assert.Equal(t, 300, fromTheTop.OffsetSecondsAt(gridBase.Add(5*time.Minute)),
		"the broadcast position is still reported")
}
