package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/api"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// rfc3339 renders an instant the way a client would send it.
func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func TestCreateScheduleEntryDerivesTheBlockFromTheLocalHour(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	item := factory.MediaItem(t, q, factory.Override{"runtime_seconds": 1800})

	// 14:00 UTC is 09:00 Central: morning.
	resp := h.Post("/schedule-entries", objMap{
		"mediaItemId": item.ID,
		"airsAt":      rfc3339(channelBase),
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	entry := decode[domain.ScheduleEntry](t, resp.Body.Bytes())
	assert.Equal(t, domain.BlockMorning, entry.Block)
	assert.True(t, entry.JoinInProgress, "a linear channel joins in progress by default")

	// 01:00 UTC the next day is 20:00 Central: evening.
	evening := h.Post("/schedule-entries", objMap{
		"mediaItemId": item.ID,
		"airsAt":      rfc3339(channelBase.Add(11 * time.Hour)),
	})
	require.Equal(t, http.StatusCreated, evening.Code, evening.Body.String())
	assert.Equal(t, domain.BlockEvening, decode[domain.ScheduleEntry](t, evening.Body.Bytes()).Block)
}

func TestCreateScheduleEntryRefusesAnOverlapAndNamesIt(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, blocker := air(t, q, 0, 3600)
	candidate := factory.MediaItem(t, q, factory.Override{"runtime_seconds": 600})

	resp := h.Post("/schedule-entries", objMap{
		"mediaItemId": candidate.ID,
		"airsAt":      rfc3339(channelBase.Add(30 * time.Minute)),
	})
	require.Equal(t, http.StatusConflict, resp.Code, resp.Body.String())
	assert.Contains(t, resp.Body.String(), blocker.Title,
		"the parent is told which airing is in the way")
}

func TestCreateScheduleEntryRefusesAnItemOfUnknownLength(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	item := factory.MediaItem(t, q, factory.Override{"runtime_seconds": 0})

	resp := h.Post("/schedule-entries", objMap{
		"mediaItemId": item.ID,
		"airsAt":      rfc3339(channelBase),
	})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code,
		"a slot with no end cannot be placed in a linear grid")
	assert.Contains(t, resp.Body.String(), "runtime")
}

func TestCreateScheduleEntryRejectsAnUnknownMediaItem(t *testing.T) {
	t.Parallel()
	h, _ := channel(t)

	resp := h.Post("/schedule-entries", objMap{
		"mediaItemId": "00000000-0000-0000-0000-000000000000",
		"airsAt":      rfc3339(channelBase),
	})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
}

func TestUpdateScheduleEntryMovesAndRefusesCollisions(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	entry, _ := air(t, q, 0, 1800)
	air(t, q, 3*time.Hour, 1800)

	moved := h.Patch("/schedule-entries/"+entry.ID, objMap{
		"airsAt": rfc3339(channelBase.Add(time.Hour)),
	})
	require.Equal(t, http.StatusOK, moved.Code, moved.Body.String())
	assert.Equal(t, channelBase.Add(time.Hour),
		decode[domain.ScheduleEntry](t, moved.Body.Bytes()).AirsAt.UTC())

	// Only the block changes: the entry must not collide with its own slot.
	relabelled := h.Patch("/schedule-entries/"+entry.ID, objMap{"block": "evening"})
	require.Equal(t, http.StatusOK, relabelled.Code, relabelled.Body.String())
	assert.Equal(t, domain.BlockEvening, decode[domain.ScheduleEntry](t, relabelled.Body.Bytes()).Block)

	clash := h.Patch("/schedule-entries/"+entry.ID, objMap{
		"airsAt": rfc3339(channelBase.Add(3*time.Hour + 10*time.Minute)),
	})
	assert.Equal(t, http.StatusConflict, clash.Code)
}

func TestUpdateScheduleEntryReportsAMissingEntry(t *testing.T) {
	t.Parallel()
	h, _ := channel(t)

	resp := h.Patch("/schedule-entries/00000000-0000-0000-0000-000000000000", objMap{"block": "evening"})
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestScheduleGridSpansCalendarDays(t *testing.T) {
	t.Parallel()
	h, q := channel(t)

	today, _ := air(t, q, time.Hour, 1800)
	tomorrow, _ := air(t, q, 26*time.Hour, 1800)
	nextWeek, _ := air(t, q, 8*24*time.Hour, 1800)

	resp := h.Get("/schedule-grid?from=2031-04-08&days=7")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.GridResponse](t, resp.Body.Bytes())

	assert.Equal(t, 7, body.Days)
	assert.Equal(t, "America/Chicago", body.Timezone)
	assert.Equal(t, body.StartsAt.AddDate(0, 0, 7).UTC(), body.EndsAt.UTC())

	ids := map[string]bool{}
	for _, p := range body.Programmes {
		ids[p.ScheduleEntryID] = true
	}
	assert.True(t, ids[today.ID])
	assert.True(t, ids[tomorrow.ID])
	assert.False(t, ids[nextWeek.ID], "the span stops at seven days")
}

func TestScheduleGridRejectsAMalformedStart(t *testing.T) {
	t.Parallel()
	h, _ := channel(t)
	assert.Equal(t, http.StatusBadRequest, h.Get("/schedule-grid?from=whenever").Code)
}

func TestCopyWeekReAirsTheWeeksProgramming(t *testing.T) {
	t.Parallel()
	h, q := channel(t)

	first, firstItem := air(t, q, time.Hour, 1800)
	second, _ := air(t, q, 3*24*time.Hour, 3600)

	resp := h.Post("/schedule-entries/copy-week", objMap{
		"fromWeekStart": "2031-04-08",
		"toWeekStart":   "2031-04-15",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.CopyWeekResponse](t, resp.Body.Bytes())
	assert.Equal(t, 2, body.Copied)
	assert.Empty(t, body.Skipped)
	assert.Zero(t, body.Deleted)

	copied, err := tvrepo.AiringsBetween(t.Context(), q,
		channelBase.AddDate(0, 0, 7), channelBase.AddDate(0, 0, 14))
	require.NoError(t, err)
	require.Len(t, copied, 2)
	assert.Equal(t, firstItem.ID, copied[0].MediaItemID)
	assert.Equal(t, channelBase.Add(time.Hour).AddDate(0, 0, 7), copied[0].AirsAt.UTC(),
		"an airing keeps its weekday and time of day")
	assert.NotEqual(t, first.ID, copied[0].ID, "the copy is a new entry")
	assert.NotEqual(t, second.ID, copied[1].ID)
}

func TestCopyWeekReportsWhatItCouldNotPlace(t *testing.T) {
	t.Parallel()
	h, q := channel(t)

	_, sourceItem := air(t, q, time.Hour, 1800)
	// Something already sits in the destination slot.
	blocker, _ := air(t, q, 7*24*time.Hour+time.Hour+10*time.Minute, 1800)

	resp := h.Post("/schedule-entries/copy-week", objMap{
		"fromWeekStart": "2031-04-08",
		"toWeekStart":   "2031-04-15",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.CopyWeekResponse](t, resp.Body.Bytes())

	assert.Zero(t, body.Copied)
	require.Len(t, body.Skipped, 1)
	assert.Equal(t, sourceItem.ID, body.Skipped[0].MediaItemID)
	assert.Equal(t, sourceItem.Title, body.Skipped[0].Title)
	assert.Contains(t, body.Skipped[0].Reason, "overlaps")

	// The obstruction is left alone: a copy is not a destructive operation.
	still, err := tvrepo.ScheduleEntries.Get(t.Context(), q, blocker.ID)
	require.NoError(t, err)
	assert.NotNil(t, still)
}

func TestCopyWeekWithReplaceClearsTheDestinationFirst(t *testing.T) {
	t.Parallel()
	h, q := channel(t)

	air(t, q, time.Hour, 1800)
	stale, _ := air(t, q, 7*24*time.Hour+time.Hour+10*time.Minute, 1800)

	resp := h.Post("/schedule-entries/copy-week", objMap{
		"fromWeekStart": "2031-04-08",
		"toWeekStart":   "2031-04-15",
		"replace":       true,
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.CopyWeekResponse](t, resp.Body.Bytes())
	assert.Equal(t, 1, body.Copied)
	assert.Equal(t, 1, body.Deleted)
	assert.Empty(t, body.Skipped)

	_, err := tvrepo.ScheduleEntries.Get(t.Context(), q, stale.ID)
	assert.Error(t, err, "the replaced entry is gone")
}

func TestCopyWeekSkipsAProgrammeThatMerelyRunsIntoTheWeek(t *testing.T) {
	t.Parallel()
	h, q := channel(t)

	// Starts at 22:00 the night before the week opens and runs past midnight
	// into its first morning.
	air(t, q, -11*time.Hour, 8*3600)
	inside, _ := air(t, q, 2*24*time.Hour, 1800)

	resp := h.Post("/schedule-entries/copy-week", objMap{
		"fromWeekStart": "2031-04-08",
		"toWeekStart":   "2031-04-15",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	assert.Equal(t, 1, decode[api.CopyWeekResponse](t, resp.Body.Bytes()).Copied,
		"only airings that start inside the source week are re-aired")

	copied, err := tvrepo.AiringsBetween(t.Context(), q,
		channelBase.AddDate(0, 0, 7), channelBase.AddDate(0, 0, 14))
	require.NoError(t, err)
	require.Len(t, copied, 1)
	assert.Equal(t, inside.MediaItemID, copied[0].MediaItemID)
}

func TestCopyWeekRefusesToCopyAWeekOntoItself(t *testing.T) {
	t.Parallel()
	h, _ := channel(t)

	resp := h.Post("/schedule-entries/copy-week", objMap{
		"fromWeekStart": "2031-04-08",
		"toWeekStart":   "2031-04-08",
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCopyWeekRejectsAMalformedWeek(t *testing.T) {
	t.Parallel()
	h, _ := channel(t)

	resp := h.Post("/schedule-entries/copy-week", objMap{
		"fromWeekStart": "week 14",
		"toWeekStart":   "2031-04-15",
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestScheduleAdminRequiresTheAdminKey(t *testing.T) {
	t.Parallel()
	h, q := channel(t, tvtestutil.Options{AdminKey: "s3cret"})
	item := factory.MediaItem(t, q, factory.Override{"runtime_seconds": 1800})

	assert.Equal(t, http.StatusUnauthorized, h.Get("/schedule-grid").Code)
	assert.Equal(t, http.StatusUnauthorized, h.Post("/schedule-entries", objMap{
		"mediaItemId": item.ID,
		"airsAt":      rfc3339(channelBase),
	}).Code)
	assert.Equal(t, http.StatusUnauthorized, h.Post("/schedule-entries/copy-week", objMap{
		"fromWeekStart": "2031-04-08",
		"toWeekStart":   "2031-04-15",
	}).Code)

	ok := h.Post("/schedule-entries", "X-Admin-Key: s3cret", objMap{
		"mediaItemId": item.ID,
		"airsAt":      rfc3339(channelBase),
	})
	assert.Equal(t, http.StatusCreated, ok.Code, ok.Body.String())
}
