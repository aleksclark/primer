package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/api"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	"github.com/aleksclark/primer/server/internal/tv/primer"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// watched records a finished viewing of an item in the given class.
func watched(t *testing.T, q baserepo.Querier, class string) *domain.PlaybackSession {
	t.Helper()
	item := factory.MediaItem(t, q, factory.Override{
		"class":        class,
		"title":        "Cosmos: Standing Up in the Milky Way",
		"subject_tags": []string{"science"},
	})
	return factory.PlaybackSession(t, q, factory.Override{
		"media_item_id":        item.ID,
		"ended_at":             time.Now().UTC(),
		"watched_seconds":      1500,
		"max_position_seconds": 1500,
		"completed":            true,
	})
}

func TestPrimerReportRunExportsAndIsRepeatable(t *testing.T) {
	t.Parallel()
	fake := primer.NewFake()
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{
		Primer: &primer.Reporter{Ingest: fake},
	})

	session := watched(t, q, domain.ClassEducational)
	watched(t, q, domain.ClassEntertainment)

	resp := h.Post("/primer-reports/run", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	first := decode[api.PrimerRunResponse](t, resp.Body.Bytes())
	assert.Equal(t, 1, first.Scanned, "only the educational viewing is instructional time")
	assert.Equal(t, 1, first.Reported)
	assert.Zero(t, first.Failed)

	// The parent can see what was counted.
	list := decode[struct {
		Items      []domain.PrimerReport `json:"items"`
		TotalCount int                   `json:"totalCount"`
	}](t, h.Get("/primer-reports?filter=playback_session_id:"+session.ID).Body.Bytes())
	require.Equal(t, 1, list.TotalCount)
	assert.Equal(t, session.ID, list.Items[0].PlaybackSessionID)
	assert.Equal(t, "log-1", list.Items[0].PrimerRef)

	// Running again is harmless.
	repeat := decode[api.PrimerRunResponse](t, h.Post("/primer-reports/run", objMap{}).Body.Bytes())
	assert.Zero(t, repeat.Scanned)
	assert.Equal(t, 1, fake.Calls, "the LMS is asked once per viewing, however often the parent clicks")
}

func TestPrimerReportRunReportsAnUnreachableLMS(t *testing.T) {
	t.Parallel()
	fake := primer.NewFake()
	fake.Err = assert.AnError
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Primer: &primer.Reporter{Ingest: fake}})

	watched(t, q, domain.ClassEducational)

	summary := decode[api.PrimerRunResponse](t, h.Post("/primer-reports/run", objMap{}).Body.Bytes())
	assert.Equal(t, 1, summary.Scanned)
	assert.Equal(t, 1, summary.Failed)
	assert.Zero(t, summary.Reported,
		"a failed export is reported as such rather than silently swallowed")
}

func TestPrimerReportRunIsUnavailableWithoutAnLMS(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t)

	assert.Equal(t, http.StatusServiceUnavailable, h.Post("/primer-reports/run", objMap{}).Code,
		"a channel running without an LMS says so instead of pretending to report")
}

func TestPrimerReportsAreReadableAndDeletable(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	session := watched(t, q, domain.ClassEducational)
	report := factory.PrimerReport(t, q, factory.Override{"playback_session_id": session.ID})

	resp := h.Get("/primer-reports/" + report.ID)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, report.PlaybackSessionID, decode[domain.PrimerReport](t, resp.Body.Bytes()).PlaybackSessionID)

	// Deleting a ledger row is the "re-report this viewing" lever.
	require.Equal(t, http.StatusNoContent, h.Delete("/primer-reports/"+report.ID).Code)

	assert.Equal(t, http.StatusNotFound, h.Get("/primer-reports/"+report.ID).Code)

	// With the ledger row gone the viewing is a candidate again, which is the
	// point of allowing the delete.
	sessions, err := tvrepo.UnreportedSessions(context.Background(), q, 10)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, report.PlaybackSessionID, sessions[0].SessionID)
}

func TestPrimerReportsRequireTheAdminKey(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{
		AdminKey: "s3cret",
		Primer:   &primer.Reporter{Ingest: primer.NewFake()},
	})
	watched(t, q, domain.ClassEducational)

	assert.Equal(t, http.StatusUnauthorized, h.Get("/primer-reports").Code)
	assert.Equal(t, http.StatusUnauthorized, h.Post("/primer-reports/run", objMap{}).Code)
	assert.Equal(t, http.StatusOK, h.Get("/primer-reports", "X-Admin-Key: s3cret").Code)
	assert.Equal(t, http.StatusOK, h.Post("/primer-reports/run", "X-Admin-Key: s3cret", objMap{}).Code)
}
