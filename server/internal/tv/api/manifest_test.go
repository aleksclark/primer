package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

func TestContentManifestSyncAndAttempt(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		Now:                     func() time.Time { return now },
		ManifestFailMaxAttempts: 2,
		ManifestFailMaxDays:     0,
	})

	resp := h.Post("/content-manifest/sync", objMap{
		"items": []objMap{
			{
				"slug":  "matrix",
				"title": "The Matrix",
				"kind":  domain.ManifestKindMovie,
				"class": domain.ClassEntertainment,
				"year":  1999,
				"tmdbId": 603,
			},
			{
				"slug":  "bernstein-ypc",
				"title": "Bernstein YPC",
				"kind":  domain.ManifestKindManual,
				"class": domain.ClassEducational,
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	sync := decode[struct {
		Created int `json:"created"`
		Updated int `json:"updated"`
		Total   int `json:"total"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 2, sync.Created)
	assert.Equal(t, 2, sync.Total)

	// Second sync updates desired state only.
	resp = h.Post("/content-manifest/sync", objMap{
		"items": []objMap{
			{
				"slug":  "matrix",
				"title": "The Matrix (4K)",
				"kind":  domain.ManifestKindMovie,
				"class": domain.ClassEntertainment,
				"tmdbId": 603,
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	sync = decode[struct {
		Created int `json:"created"`
		Updated int `json:"updated"`
		Total   int `json:"total"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 0, sync.Created)
	assert.Equal(t, 1, sync.Updated)

	resp = h.Post("/content-manifest-entries/matrix/attempt", objMap{"error": "no release"})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	entry := decode[domain.ContentManifestEntry](t, resp.Body.Bytes())
	assert.Equal(t, 1, entry.AttemptCount)
	assert.Equal(t, domain.ManifestStatusMissing, entry.Status)
	assert.Equal(t, "no release", entry.LastError)

	resp = h.Post("/content-manifest-entries/matrix/attempt", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	entry = decode[domain.ContentManifestEntry](t, resp.Body.Bytes())
	assert.Equal(t, 2, entry.AttemptCount)
	assert.Equal(t, domain.ManifestStatusFailed, entry.Status)

	resp = h.Post("/content-manifest-entries/matrix/present", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	entry = decode[domain.ContentManifestEntry](t, resp.Body.Bytes())
	assert.Equal(t, domain.ManifestStatusPresent, entry.Status)
	require.NotNil(t, entry.PresentAt)

	resp = h.Get("/content-manifest-entries?filter=status:manual")
	require.Equal(t, http.StatusOK, resp.Code)
	page := decode[struct {
		TotalCount int `json:"totalCount"`
		Items      []domain.ContentManifestEntry `json:"items"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 1, page.TotalCount)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "bernstein-ypc", page.Items[0].Slug)
}

func TestContentManifestCRUD(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	entry := factory.ContentManifestEntry(t, q, factory.Override{
		"slug": "apollo-13", "title": "Apollo 13",
	})

	resp := h.Get("/content-manifest-entries/" + entry.ID)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "Apollo 13", decode[domain.ContentManifestEntry](t, resp.Body.Bytes()).Title)

	resp = h.Patch("/content-manifest-entries/"+entry.ID, objMap{
		"notes":  "buy blu-ray",
		"status": domain.ManifestStatusFailed,
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	got := decode[domain.ContentManifestEntry](t, resp.Body.Bytes())
	assert.Equal(t, "buy blu-ray", got.Notes)
	assert.Equal(t, domain.ManifestStatusFailed, got.Status)
}
