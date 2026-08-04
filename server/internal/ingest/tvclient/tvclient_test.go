package tvclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/tvclient"
)

func TestNewRequiresBaseURL(t *testing.T) {
	t.Parallel()
	_, err := tvclient.New(tvclient.Options{})
	assert.ErrorIs(t, err, tvclient.ErrNotConfigured)
}

func TestCRUDAndSync(t *testing.T) {
	t.Parallel()
	var items []tvclient.MediaItem
	mux := http.NewServeMux()
	var manifest []tvclient.ManifestEntry
	mux.HandleFunc("/media-items", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "adminkey", r.Header.Get(tvclient.AdminKeyHeader))
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": items, "totalCount": len(items),
			})
		case http.MethodPost:
			var in tvclient.MediaItemCreate
			require.NoError(t, json.NewDecoder(r.Body).Decode(&in))
			item := tvclient.MediaItem{
				ID: "mi-1", JellyfinItemID: in.JellyfinItemID,
				Title: in.Title, Class: in.Class,
				SubjectTags: in.SubjectTags, StandardCodes: in.StandardCodes,
			}
			items = append(items, item)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(item)
		}
	})
	mux.HandleFunc("/media-items/mi-1", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		var in tvclient.MediaItemUpdate
		require.NoError(t, json.NewDecoder(r.Body).Decode(&in))
		items[0].Class = *in.Class
		_ = json.NewEncoder(w).Encode(items[0])
	})
	mux.HandleFunc("/jellyfin/sync", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		_ = json.NewEncoder(w).Encode(tvclient.SyncResult{Checked: 1, Updated: 0})
	})
	mux.HandleFunc("/content-manifest/sync", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		var body struct {
			Items []tvclient.ManifestDesired `json:"items"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		for _, in := range body.Items {
			manifest = append(manifest, tvclient.ManifestEntry{
				ID: "me-1", Slug: in.Slug, Title: in.Title, Status: "missing",
			})
		}
		_ = json.NewEncoder(w).Encode(tvclient.ManifestSyncResult{
			Created: len(body.Items), Total: len(body.Items),
		})
	})
	mux.HandleFunc("/content-manifest-entries", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": manifest, "totalCount": len(manifest),
		})
	})
	mux.HandleFunc("/content-manifest-entries/matrix/attempt", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		_ = json.NewEncoder(w).Encode(tvclient.ManifestEntry{
			Slug: "matrix", Status: "missing", AttemptCount: 1,
		})
	})
	mux.HandleFunc("/content-manifest-entries/matrix/present", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		_ = json.NewEncoder(w).Encode(tvclient.ManifestEntry{
			Slug: "matrix", Status: "present", AttemptCount: 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := tvclient.New(tvclient.Options{
		BaseURL: srv.URL, AdminKey: "adminkey", HTTPClient: srv.Client(),
	})
	require.NoError(t, err)

	created, err := client.CreateMediaItem(context.Background(), tvclient.MediaItemCreate{
		JellyfinItemID: "jf-1", Title: "Apollo 13", Class: "mixed",
		SubjectTags: []string{"science"},
	})
	require.NoError(t, err)
	assert.Equal(t, "mi-1", created.ID)

	class := "educational"
	updated, err := client.UpdateMediaItem(context.Background(), "mi-1", tvclient.MediaItemUpdate{Class: &class})
	require.NoError(t, err)
	assert.Equal(t, "educational", updated.Class)

	listed, err := client.ListMediaItems(context.Background())
	require.NoError(t, err)
	require.Len(t, listed, 1)

	sync, err := client.SyncJellyfin(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, sync.Checked)

	ms, err := client.SyncManifest(context.Background(), []tvclient.ManifestDesired{{
		Slug: "matrix", Title: "The Matrix", Kind: "movie", Class: "entertainment",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, ms.Created)

	entries, err := client.ListManifestEntries(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 1)

	attempted, err := client.RecordManifestAttempt(context.Background(), "matrix", "waiting")
	require.NoError(t, err)
	assert.Equal(t, 1, attempted.AttemptCount)

	present, err := client.MarkManifestPresent(context.Background(), "matrix")
	require.NoError(t, err)
	assert.Equal(t, "present", present.Status)
}

func TestClassificationChanged(t *testing.T) {
	t.Parallel()
	existing := tvclient.MediaItem{
		Class: "mixed", SubjectTags: []string{"a"}, StandardCodes: []string{"c1"},
	}
	assert.False(t, tvclient.ClassificationChanged(existing, "mixed", []string{"a"}, []string{"c1"}))
	assert.True(t, tvclient.ClassificationChanged(existing, "educational", []string{"a"}, []string{"c1"}))
	assert.True(t, tvclient.ClassificationChanged(existing, "mixed", []string{"b"}, []string{"c1"}))
}

func TestFake(t *testing.T) {
	t.Parallel()
	f := tvclient.NewFake()
	_, err := f.CreateMediaItem(context.Background(), tvclient.MediaItemCreate{
		JellyfinItemID: "jf", Title: "T", Class: "mixed",
	})
	require.NoError(t, err)
	items, err := f.ListMediaItems(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	class := "educational"
	title := "T2"
	overview := "o"
	tags := []string{"a"}
	codes := []string{"c"}
	_, err = f.UpdateMediaItem(context.Background(), items[0].ID, tvclient.MediaItemUpdate{
		Class: &class, Title: &title, Overview: &overview, SubjectTags: &tags, StandardCodes: &codes,
	})
	require.NoError(t, err)
	_, err = f.SyncJellyfin(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, f.SyncCalls)

	_, err = f.CreateMediaItem(context.Background(), tvclient.MediaItemCreate{
		JellyfinItemID: "jf", Title: "dup", Class: "mixed",
	})
	assert.Error(t, err)
	_, err = f.UpdateMediaItem(context.Background(), "missing", tvclient.MediaItemUpdate{})
	assert.Error(t, err)

	idx := tvclient.ByJellyfinID(items)
	assert.Contains(t, idx, "jf")
}
