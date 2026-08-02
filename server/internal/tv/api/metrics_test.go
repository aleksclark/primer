package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/api"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

func TestMetricsSummarizesViewingByClassSubjectAndDay(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)

	doc := factory.MediaItem(t, q, factory.Override{
		"class":        domain.ClassEducational,
		"subject_tags": []string{"science", "history"},
	})
	film := factory.MediaItem(t, q, factory.Override{
		"class":        domain.ClassEntertainment,
		"subject_tags": []string{"history"},
	})

	now := time.Now().UTC()
	factory.PlaybackSession(t, q, factory.Override{
		"media_item_id":   doc.ID,
		"started_at":      now.Add(-2 * time.Hour),
		"watched_seconds": 1500,
		"completed":       true,
	})
	factory.PlaybackSession(t, q, factory.Override{
		"media_item_id":   film.ID,
		"started_at":      now.Add(-time.Hour),
		"watched_seconds": 600,
		"completed":       false,
	})

	resp := h.Get("/metrics?days=7")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.MetricsResponse](t, resp.Body.Bytes())

	byClass := map[string]int{}
	for _, row := range body.ByClass {
		byClass[row.Class] = row.WatchedSeconds
	}
	assert.Equal(t, 1500, byClass[domain.ClassEducational])
	assert.Equal(t, 600, byClass[domain.ClassEntertainment])

	bySubject := map[string]int{}
	for _, row := range body.BySubject {
		bySubject[row.Subject] = row.WatchedSeconds
	}
	assert.Equal(t, 1500, bySubject["science"])
	assert.Equal(t, 2100, bySubject["history"], "an item counts under every subject it carries")

	assert.Equal(t, 2, body.Completion.Sessions)
	assert.Equal(t, 1, body.Completion.Completed)
	assert.Equal(t, 2100, body.Completion.WatchedSeconds)

	require.NotEmpty(t, body.ByDay)
	var instructional int
	for _, day := range body.ByDay {
		instructional += day.InstructionalWatchedSeconds
	}
	assert.Equal(t, 1500, instructional, "entertainment is not instructional time")
}

func TestMetricsExcludesViewingOutsideTheWindow(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)

	item := factory.MediaItem(t, q, factory.Override{"class": domain.ClassEducational})
	factory.PlaybackSession(t, q, factory.Override{
		"media_item_id":   item.ID,
		"started_at":      time.Now().UTC().AddDate(0, 0, -30),
		"watched_seconds": 900,
	})

	body := decode[api.MetricsResponse](t, h.Get("/metrics?days=7").Body.Bytes())
	assert.Zero(t, body.Completion.Sessions, "a viewing from last month is outside a one-week window")

	wide := decode[api.MetricsResponse](t, h.Get("/metrics?days=60").Body.Bytes())
	assert.Equal(t, 1, wide.Completion.Sessions)
}

func TestMetricsCountsTheEntertainmentRation(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)

	now := time.Now().UTC()
	film := factory.MediaItem(t, q, factory.Override{"class": domain.ClassEntertainment})
	window := factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": film.ID,
		"starts_at":     now.Add(-time.Hour),
		"ends_at":       now.Add(24 * time.Hour),
	})
	factory.MediaItem(t, q, factory.Override{"class": domain.ClassEducational})

	body := decode[api.MetricsResponse](t, h.Get("/metrics?days=7").Body.Bytes())
	assert.Equal(t, 1, body.Entertainment.WindowsOffered)
	assert.Zero(t, body.Entertainment.PlaysUsed)

	factory.PlayLedgerEntry(t, q, factory.Override{
		"media_item_id":          film.ID,
		"availability_window_id": window.ID,
		"consumed_at":            now,
	})

	spent := decode[api.MetricsResponse](t, h.Get("/metrics?days=7").Body.Bytes())
	assert.Equal(t, 1, spent.Entertainment.PlaysUsed)
}

func TestMetricsOnAnIdleChannelReturnsEmptyLists(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t)

	resp := h.Get("/metrics")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.MetricsResponse](t, resp.Body.Bytes())

	assert.NotNil(t, body.ByClass, "an idle channel returns [] rather than null")
	assert.NotNil(t, body.BySubject)
	assert.NotNil(t, body.ByDay)
	assert.Empty(t, body.ByClass)
	assert.Zero(t, body.Completion.Sessions)
	assert.Equal(t, body.To.Sub(body.From), 14*24*time.Hour, "the default window is a fortnight")
}

func TestMetricsRequiresTheAdminKey(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{AdminKey: "secret"})

	assert.Equal(t, http.StatusUnauthorized, h.Get("/metrics").Code)
	assert.Equal(t, http.StatusOK, h.Get("/metrics", "X-Admin-Key: secret").Code)
}
