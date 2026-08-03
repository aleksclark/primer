package radarr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/radarr"
)

func TestNewRequiresBaseURL(t *testing.T) {
	t.Parallel()
	_, err := radarr.New(radarr.Options{})
	assert.ErrorIs(t, err, radarr.ErrNotConfigured)
}

func TestLookupAddTag(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/movie/lookup", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "secret", r.Header.Get("X-Api-Key"))
		assert.Equal(t, "Matrix", r.URL.Query().Get("term"))
		_ = json.NewEncoder(w).Encode([]radarr.Movie{{
			Title: "The Matrix", Year: 1999, TmdbID: 603, TitleSlug: "the-matrix",
		}})
	})
	mux.HandleFunc("/api/v3/tag", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]radarr.Tag{})
		case http.MethodPost:
			var body radarr.Tag
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			body.ID = 9
			_ = json.NewEncoder(w).Encode(body)
		}
	})
	mux.HandleFunc("/api/v3/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]radarr.Movie{})
			return
		}
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, float64(603), body["tmdbId"])
		assert.Equal(t, true, body["addOptions"].(map[string]any)["searchForMovie"])
		_ = json.NewEncoder(w).Encode(radarr.Movie{ID: 1, Title: "The Matrix", TmdbID: 603, HasFile: false})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := radarr.New(radarr.Options{BaseURL: srv.URL, APIKey: "secret", HTTPClient: srv.Client()})
	require.NoError(t, err)

	hits, err := client.Lookup(context.Background(), "Matrix")
	require.NoError(t, err)
	require.Len(t, hits, 1)

	tagID, err := client.EnsureTag(context.Background(), "primer")
	require.NoError(t, err)
	assert.Equal(t, 9, tagID)

	added, err := client.Add(context.Background(), hits[0], 4, "/movies", []int{tagID})
	require.NoError(t, err)
	assert.Equal(t, 1, added.ID)
}

func TestByTMDBAndFilterYear(t *testing.T) {
	t.Parallel()
	movies := []radarr.Movie{
		{Title: "A", Year: 1999, TmdbID: 1},
		{Title: "B", Year: 2000, TmdbID: 2},
	}
	assert.Equal(t, 1, radarr.ByTMDB(movies, 1).TmdbID)
	assert.Nil(t, radarr.ByTMDB(movies, 99))
	assert.Len(t, radarr.FilterYear(movies, 1999), 1)
	assert.Len(t, radarr.FilterYear(movies, 0), 2)
}

func TestFake(t *testing.T) {
	t.Parallel()
	f := radarr.NewFake()
	f.LookupResults = []radarr.Movie{{Title: "The Matrix", Year: 1999, TmdbID: 603}}
	hits, err := f.Lookup(context.Background(), "Matrix")
	require.NoError(t, err)
	require.Len(t, hits, 1)

	tag, err := f.EnsureTag(context.Background(), "primer")
	require.NoError(t, err)
	_, err = f.Add(context.Background(), hits[0], 1, "/m", []int{tag})
	require.NoError(t, err)
	lib, err := f.List(context.Background())
	require.NoError(t, err)
	require.Len(t, lib, 1)

	// Existing tag short-circuit + duplicate add error.
	tag2, err := f.EnsureTag(context.Background(), "primer")
	require.NoError(t, err)
	assert.Equal(t, tag, tag2)
	_, err = f.Add(context.Background(), hits[0], 1, "/m", nil)
	assert.Error(t, err)
	_, err = f.Add(context.Background(), radarr.Movie{Title: "x"}, 1, "/m", nil)
	assert.Error(t, err)
}

func TestListAndAddValidation(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]radarr.Movie{{ID: 3, Title: "Held", TmdbID: 1, HasFile: true}})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`nope`))
	})
	mux.HandleFunc("/api/v3/tag", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]radarr.Tag{{ID: 2, Label: "primer"}})
			return
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client, err := radarr.New(radarr.Options{BaseURL: srv.URL + "/", APIKey: "k", HTTPClient: srv.Client()})
	require.NoError(t, err)

	lib, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, lib, 1)
	assert.True(t, lib[0].HasFile)

	tag, err := client.EnsureTag(context.Background(), "primer")
	require.NoError(t, err)
	assert.Equal(t, 2, tag)

	_, err = client.Add(context.Background(), radarr.Movie{Title: "x"}, 1, "/m", nil)
	assert.Error(t, err, "tmdb required")
	_, err = client.Add(context.Background(), radarr.Movie{Title: "x", TmdbID: 1}, 0, "/m", nil)
	assert.Error(t, err, "quality required")
	_, err = client.Add(context.Background(), radarr.Movie{Title: "x", TmdbID: 1}, 1, "", nil)
	assert.Error(t, err, "root required")
	_, err = client.Add(context.Background(), radarr.Movie{Title: "x", TmdbID: 1}, 1, "/m", nil)
	assert.Error(t, err, "server refused")
}
