package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/api"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

func TestProbeCopyWeekBackwards(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	// Source: an airing on the Tuesday of week 2031-04-15
	air(t, q, 7*24*time.Hour+time.Hour, 1800)

	resp := h.Post("/schedule-entries/copy-week", objMap{
		"fromWeekStart": "2031-04-15",
		"toWeekStart":   "2031-04-08",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.CopyWeekResponse](t, resp.Body.Bytes())
	t.Logf("copied=%d skipped=%v", body.Copied, body.Skipped)

	got, err := tvrepo.AiringsBetween(t.Context(), q, channelBase.AddDate(0, 0, -1), channelBase.AddDate(0, 0, 7))
	require.NoError(t, err)
	for _, a := range got {
		t.Logf("landed at %s (want %s)", a.AirsAt.UTC(), channelBase.Add(time.Hour).UTC())
	}
}

func TestProbeUpdateWithUnknownMediaItem(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	entry, _ := air(t, q, 0, 1800)

	resp := h.Patch("/schedule-entries/"+entry.ID, objMap{
		"mediaItemId": "00000000-0000-0000-0000-000000000000",
	})
	t.Logf("unknown media item on update -> %d %s", resp.Code, resp.Body.String())
}

func TestProbeUpdateOntoZeroRuntimeItem(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	entry, _ := air(t, q, 0, 1800)
	zero := factory.MediaItem(t, q, factory.Override{"runtime_seconds": 0})

	resp := h.Patch("/schedule-entries/"+entry.ID, objMap{"mediaItemId": zero.ID})
	t.Logf("zero-runtime item on update -> %d %s", resp.Code, resp.Body.String())
}

func TestProbeProgrammedGrantDoesNotBurnAWatchOncePlay(t *testing.T) {
	t.Parallel()
	h, q := channel(t)
	_, token := factory.PairedDevice(t, q)

	_, item := air(t, q, -time.Minute, 1800) // entertainment by factory default
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": item.ID,
		"starts_at":     channelBase.Add(-time.Hour),
		"ends_at":       channelBase.Add(24 * time.Hour),
	})

	grant := decode[api.GrantResponse](t, h.Post("/media/"+item.ID+"/grant?mode=programmed",
		authHeader(token), objMap{}).Body.Bytes())
	done := h.Post("/grants/"+grant.GrantID+"/complete", authHeader(token),
		objMap{"positionSeconds": 1800, "watchedSeconds": 1800, "completed": true})
	require.Equal(t, http.StatusOK, done.Code, done.Body.String())
	sess := decode[api.SessionResponse](t, done.Body.Bytes())
	t.Logf("playConsumed=%v", sess.PlayConsumed)

	cat := decode[api.CatalogResponse](t, h.Get("/catalog", authHeader(token)).Body.Bytes())
	t.Logf("catalog items after channel viewing = %d", len(cat.Items))
	assert.Len(t, cat.Items, 1, "the on-demand play must survive a channel viewing")
}
