package api_test

import (
	"fmt"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/api"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// channelBase is the fixed instant the channel tests pin the server clock to:
// a Tuesday morning, 09:00 Central (14:00 UTC, CDT).
var channelBase = time.Date(2031, 4, 8, 14, 0, 0, 0, time.UTC)

// channel builds an API whose clock is frozen at channelBase.
func channel(t *testing.T, opts ...tvtestutil.Options) (humatest.TestAPI, baserepo.Querier) {
	t.Helper()
	var o tvtestutil.Options
	if len(opts) > 0 {
		o = opts[0]
	}
	o.Now = func() time.Time { return channelBase }
	api, q, _ := tvtestutil.API(t, o)
	return api, q
}

// air places an item of the given runtime at an offset from channelBase.
func air(t *testing.T, q baserepo.Querier, offset time.Duration, runtimeSeconds int, overrides ...factory.Override) (*domain.ScheduleEntry, *domain.MediaItem) {
	t.Helper()
	item := factory.MediaItem(t, q, factory.Override{"runtime_seconds": runtimeSeconds})
	merged := factory.Override{"media_item_id": item.ID, "airs_at": channelBase.Add(offset)}
	for _, o := range overrides {
		maps.Copy(merged, o)
	}
	return factory.ScheduleEntry(t, q, merged), item
}

func TestNowReportsTheAiringProgrammeAndOffset(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	entry, item := air(t, q, -10*time.Minute, 1800)
	next, _ := air(t, q, 30*time.Minute, 1800)

	resp := h.Get("/now", authHeader(token))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.NowResponse](t, resp.Body.Bytes())

	require.NotNil(t, body.OnAir)
	assert.Equal(t, entry.ID, body.OnAir.ScheduleEntryID)
	assert.Equal(t, item.Title, body.OnAir.Title)
	assert.Equal(t, 600, body.OffsetSeconds, "the offset comes from the server clock")
	assert.Equal(t, 600, body.StartOffsetSeconds)
	assert.Equal(t, channelBase, body.ServerTime.UTC())
	assert.Equal(t, fmt.Sprintf("/images/%s/Primary", item.ID), body.OnAir.ImageURL)
	assert.Equal(t, channelBase.Add(20*time.Minute), body.OnAir.EndsAt.UTC())

	require.NotNil(t, body.Next)
	assert.Equal(t, next.ID, body.Next.ScheduleEntryID)
	assert.Equal(t, 1800, body.NextStartsInSeconds)
}

func TestNowReportsAGapExplicitly(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	air(t, q, -2*time.Hour, 600)
	upcoming, _ := air(t, q, time.Hour, 600)

	body := decode[api.NowResponse](t, h.Get("/now", authHeader(token)).Body.Bytes())
	assert.Nil(t, body.OnAir, "a gap is an absent programme, not a stale one")
	assert.Zero(t, body.OffsetSeconds)
	require.NotNil(t, body.Next)
	assert.Equal(t, upcoming.ID, body.Next.ScheduleEntryID)
	assert.Equal(t, 3600, body.NextStartsInSeconds)
}

func TestNowOnAnEmptyGridHasNeitherProgramme(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	body := decode[api.NowResponse](t, h.Get("/now", authHeader(token)).Body.Bytes())
	assert.Nil(t, body.OnAir)
	assert.Nil(t, body.Next)
	assert.Equal(t, channelBase, body.ServerTime.UTC())
}

func TestNowHonoursJoinInProgress(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	air(t, q, -10*time.Minute, 1800, factory.Override{"join_in_progress": false})

	body := decode[api.NowResponse](t, h.Get("/now", authHeader(token)).Body.Bytes())
	require.NotNil(t, body.OnAir)
	assert.Equal(t, 600, body.OffsetSeconds, "the broadcast position is still reported")
	assert.Equal(t, 0, body.StartOffsetSeconds, "but a late joiner starts from the top")
	assert.False(t, body.OnAir.JoinInProgress)
}

func TestScheduleBucketsDaysInTheChannelTimezone(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	// 23:00 Central on 8 April is 04:00 UTC on 9 April. Bucketed in UTC it
	// would land on the wrong day; bucketed in the channel zone it is still
	// Tuesday evening.
	lateEvening, _ := air(t, q, 9*time.Hour, 1800)

	resp := h.Get("/schedule?day=2031-04-08", authHeader(token))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.ScheduleResponse](t, resp.Body.Bytes())

	assert.Equal(t, "2031-04-08", body.Day)
	assert.Equal(t, "America/Chicago", body.Timezone)
	assert.Equal(t, time.Date(2031, 4, 8, 5, 0, 0, 0, time.UTC), body.DayStartsAt.UTC(),
		"midnight Central on 8 April is 05:00 UTC during CDT")
	assert.Equal(t, time.Date(2031, 4, 9, 5, 0, 0, 0, time.UTC), body.DayEndsAt.UTC())

	ids := make([]string, len(body.Programmes))
	for i, p := range body.Programmes {
		ids[i] = p.ScheduleEntryID
	}
	assert.Contains(t, ids, lateEvening.ID)
}

func TestScheduleDefaultsToTodayAndOrdersByAirTime(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	later, _ := air(t, q, 3*time.Hour, 1800)
	earlier, _ := air(t, q, time.Hour, 1800)
	// Tomorrow, so it must not appear on today's grid.
	air(t, q, 26*time.Hour, 1800)

	body := decode[api.ScheduleResponse](t, h.Get("/schedule", authHeader(token)).Body.Bytes())
	assert.Equal(t, "2031-04-08", body.Day)
	require.Len(t, body.Programmes, 2)
	assert.Equal(t, earlier.ID, body.Programmes[0].ScheduleEntryID)
	assert.Equal(t, later.ID, body.Programmes[1].ScheduleEntryID)
}

func TestScheduleRejectsAMalformedDay(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	resp := h.Get("/schedule?day=next-tuesday", authHeader(token))
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestChannelEndpointsRequireADeviceToken(t *testing.T) {
	t.Parallel()
	h, _ := channel(t)

	for _, path := range []string{"/now", "/schedule"} {
		t.Run(path, func(t *testing.T) {
			assert.Equal(t, http.StatusUnauthorized, h.Get(path).Code)
			assert.Equal(t, http.StatusUnauthorized, h.Get(path, authHeader("nope")).Code)
		})
	}
}

func TestProgrammedGrantCarriesTheBroadcastOffset(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)
	entry, item := air(t, q, -7*time.Minute, 3600)

	resp := h.Post("/media/"+item.ID+"/grant?mode=programmed", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	grant := decode[api.GrantResponse](t, resp.Body.Bytes())

	assert.Equal(t, domain.ModeProgrammed, grant.Mode)
	assert.Equal(t, 420, grant.StartOffsetSeconds)
	assert.NotEmpty(t, grant.StreamURL)

	stored, err := tvrepo.PlayGrants.Get(t.Context(), q, grant.GrantID)
	require.NoError(t, err)
	require.NotNil(t, stored.ScheduleEntryID)
	assert.Equal(t, entry.ID, *stored.ScheduleEntryID)
	assert.Nil(t, stored.AvailabilityWindowID,
		"a programmed grant is authorized by the grid, not by a rotation window")
}

func TestProgrammedGrantStartsFromTheTopWhenJoiningLateIsForbidden(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)
	_, item := air(t, q, -7*time.Minute, 3600, factory.Override{"join_in_progress": false})

	resp := h.Post("/media/"+item.ID+"/grant?mode=programmed", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	assert.Zero(t, decode[api.GrantResponse](t, resp.Body.Bytes()).StartOffsetSeconds)
}

func TestProgrammedGrantRefusesAnythingNotAiringNow(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	_, finished := air(t, q, -3*time.Hour, 1800)
	_, upcoming := air(t, q, time.Hour, 1800)
	_, onAir := air(t, q, -5*time.Minute, 1800)

	// A programme that has already ended: missed slot, missed.
	past := h.Post("/media/"+finished.ID+"/grant?mode=programmed", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, past.Code, "the channel offers no catch-up")
	assert.Contains(t, past.Body.String(), "not airing now")

	// A programme that has not started.
	future := h.Post("/media/"+upcoming.ID+"/grant?mode=programmed", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, future.Code)

	// An item that is not in the grid at all.
	stranger := factory.MediaItem(t, q)
	unscheduled := h.Post("/media/"+stranger.ID+"/grant?mode=programmed", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, unscheduled.Code)

	// The one actually airing is granted.
	ok := h.Post("/media/"+onAir.ID+"/grant?mode=programmed", authHeader(token), objMap{})
	assert.Equal(t, http.StatusCreated, ok.Code, ok.Body.String())
}

func TestProgrammedGrantRefusedInAGap(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)
	_, item := air(t, q, 2*time.Hour, 1800)

	resp := h.Post("/media/"+item.ID+"/grant?mode=programmed", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, resp.Code)
	assert.Contains(t, resp.Body.String(), "nothing is airing")
}

func TestProgrammedGrantRefusesItemsThatCannotDirectPlay(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)
	_, item := air(t, q, -time.Minute, 1800, factory.Override{})
	_, err := tvrepo.MediaItems.Update(t.Context(), q, item.ID, map[string]any{"direct_play_ok": false})
	require.NoError(t, err)

	resp := h.Post("/media/"+item.ID+"/grant?mode=programmed", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, resp.Code)
	assert.Contains(t, resp.Body.String(), "direct-play")
}

func TestProgrammedViewingRecordsAPlaybackSession(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)
	_, item := air(t, q, -time.Minute, 1800)

	grant := decode[api.GrantResponse](t, h.Post("/media/"+item.ID+"/grant?mode=programmed",
		authHeader(token), objMap{}).Body.Bytes())

	beat := h.Post("/grants/"+grant.GrantID+"/heartbeat", authHeader(token),
		objMap{"positionSeconds": 120, "watchedSeconds": 60})
	require.Equal(t, http.StatusOK, beat.Code, beat.Body.String())

	done := h.Post("/grants/"+grant.GrantID+"/complete", authHeader(token),
		objMap{"positionSeconds": 1800, "watchedSeconds": 1740, "completed": true})
	require.Equal(t, http.StatusOK, done.Code, done.Body.String())
	session := decode[api.SessionResponse](t, done.Body.Bytes())

	assert.True(t, session.Session.Completed)
	assert.Equal(t, 1740, session.Session.WatchedSeconds)
	assert.False(t, session.PlayConsumed,
		"the watch-once ledger is a rotation concept; the channel does not ration")

	// The session is the record phase 5 reports instructional hours from.
	stored, err := tvrepo.SessionForGrant(t.Context(), q, grant.GrantID)
	require.NoError(t, err)
	assert.Equal(t, item.ID, stored.MediaItemID)
	assert.Equal(t, 1800, stored.MaxPositionSeconds)
}

func TestOnDemandGrantIsStillTheDefaultMode(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q)
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": item.ID,
		"starts_at":     channelBase.Add(-time.Hour),
		"ends_at":       channelBase.Add(time.Hour),
	})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	grant := decode[api.GrantResponse](t, resp.Body.Bytes())
	assert.Equal(t, domain.ModeOnDemand, grant.Mode)
	assert.Zero(t, grant.StartOffsetSeconds)
}
