package sonarr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/sonarr"
)

func TestNewRequiresBaseURL(t *testing.T) {
	t.Parallel()
	_, err := sonarr.New(sonarr.Options{})
	assert.ErrorIs(t, err, sonarr.ErrNotConfigured)
}

func TestLookupAddEpisodes(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/series/lookup", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "secret", r.Header.Get("X-Api-Key"))
		_ = json.NewEncoder(w).Encode([]sonarr.Series{{
			Title: "The Living Planet", Year: 1984, TvdbID: 79165, TitleSlug: "living-planet",
		}})
	})
	mux.HandleFunc("/api/v3/tag", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]sonarr.Tag{{ID: 1, Label: "primer"}})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/api/v3/series", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]sonarr.Series{})
			return
		}
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, float64(79165), body["tvdbId"])
		_ = json.NewEncoder(w).Encode(sonarr.Series{ID: 42, Title: "The Living Planet", TvdbID: 79165})
	})
	mux.HandleFunc("/api/v3/episode", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "42", r.URL.Query().Get("seriesId"))
		_ = json.NewEncoder(w).Encode([]sonarr.Episode{
			{ID: 1, SeriesID: 42, SeasonNumber: 1, EpisodeNumber: 1, Monitored: true},
			{ID: 2, SeriesID: 42, SeasonNumber: 1, EpisodeNumber: 7, Monitored: true},
		})
	})
	mux.HandleFunc("/api/v3/episode/2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(sonarr.Episode{ID: 2, SeasonNumber: 1, EpisodeNumber: 7, Monitored: true})
			return
		}
		var body sonarr.Episode
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.False(t, body.Monitored)
		_ = json.NewEncoder(w).Encode(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := sonarr.New(sonarr.Options{BaseURL: srv.URL, APIKey: "secret", HTTPClient: srv.Client()})
	require.NoError(t, err)

	hits, err := client.Lookup(context.Background(), "Living Planet")
	require.NoError(t, err)
	require.Len(t, hits, 1)

	tagID, err := client.EnsureTag(context.Background(), "primer")
	require.NoError(t, err)
	assert.Equal(t, 1, tagID)

	added, err := client.Add(context.Background(), hits[0], 4, "/tv", []int{tagID})
	require.NoError(t, err)
	assert.Equal(t, 42, added.ID)

	eps, err := client.Episodes(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, eps, 2)
	assert.Equal(t, "S01E07", eps[1].Key())
	require.NoError(t, client.SetEpisodeMonitored(context.Background(), 2, false))
}

func TestByTVDBAndFilterYear(t *testing.T) {
	t.Parallel()
	series := []sonarr.Series{
		{Title: "A", Year: 1984, TvdbID: 1},
		{Title: "B", Year: 2000, TvdbID: 2},
	}
	assert.Equal(t, 1, sonarr.ByTVDB(series, 1).TvdbID)
	assert.Nil(t, sonarr.ByTVDB(series, 99))
	assert.Len(t, sonarr.FilterYear(series, 1984), 1)
}

func TestFake(t *testing.T) {
	t.Parallel()
	f := sonarr.NewFake()
	f.LookupResults = []sonarr.Series{{Title: "X", Year: 2000, TvdbID: 9}}
	hits, err := f.Lookup(context.Background(), "X")
	require.NoError(t, err)
	added, err := f.Add(context.Background(), hits[0], 1, "/tv", nil)
	require.NoError(t, err)
	f.EpisodeMap[added.ID] = []sonarr.Episode{{ID: 5, SeriesID: added.ID, SeasonNumber: 1, EpisodeNumber: 1, Monitored: true}}
	require.NoError(t, f.SetEpisodeMonitored(context.Background(), 5, false))
	assert.Contains(t, f.Unmonitor, 5)
}
