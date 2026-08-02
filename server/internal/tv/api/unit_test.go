package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/domain"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
)

func TestNewServesHealthAndSpec(t *testing.T) {
	t.Parallel()
	_, handler := New(nil, Options{CORSOrigins: []string{"http://localhost:5174"}})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	spec, err := http.Get(srv.URL + "/openapi.json")
	require.NoError(t, err)
	defer spec.Body.Close()
	assert.Equal(t, http.StatusOK, spec.StatusCode)
}

func TestSpecDocumentsDeviceSecurityScheme(t *testing.T) {
	t.Parallel()
	api, _ := New(nil, Options{})

	schemes := api.OpenAPI().Components.SecuritySchemes
	require.Contains(t, schemes, deviceSecurityScheme)
	assert.Equal(t, "http", schemes[deviceSecurityScheme].Type)
	assert.Equal(t, "bearer", schemes[deviceSecurityScheme].Scheme)
	require.Contains(t, schemes, adminSecurityScheme)
	assert.Equal(t, "apiKey", schemes[adminSecurityScheme].Type)
	assert.Equal(t, adminKeyHeader, schemes[adminSecurityScheme].Name)

	// The two audiences carry different credentials: devices a bearer token,
	// admin callers an API key.
	catalog := api.OpenAPI().Paths["/catalog"].Get
	require.NotNil(t, catalog)
	assert.Equal(t, []map[string][]string{{deviceSecurityScheme: {}}}, catalog.Security,
		"the catalog requires a device token")

	mediaItems := api.OpenAPI().Paths["/media-items"].Get
	require.NotNil(t, mediaItems)
	assert.Equal(t, []map[string][]string{{adminSecurityScheme: {}}}, mediaItems.Security,
		"admin CRUD requires the admin key, not a device token")
}

func TestDefaultsApplied(t *testing.T) {
	t.Parallel()
	api, _ := New(nil, Options{})
	require.NotNil(t, api)

	// Constructing with no clock, TTLs, or media client must not panic and
	// must still register every route, so spec generation works offline.
	assert.NotEmpty(t, api.OpenAPI().Paths)
	assert.Contains(t, api.OpenAPI().Paths, "/devices/pair")
	assert.Contains(t, api.OpenAPI().Paths, "/jellyfin/sync")
}

func TestCORSMiddlewareApplied(t *testing.T) {
	t.Parallel()
	_, handler := New(nil, Options{CORSOrigins: []string{"http://localhost:5174"}})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/catalog", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://localhost:5174")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "http://localhost:5174", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestDeviceFromContextRequiresAuthentication(t *testing.T) {
	t.Parallel()

	_, err := device(t.Context())
	assert.Error(t, err, "handlers must not run without an authenticated device")

	_, ok := DeviceFromContext(t.Context())
	assert.False(t, ok)
}

func TestImageProxyPath(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/images/abc/Primary", imageProxyPath("abc", "Primary"))
	assert.Equal(t, "/images/abc/Backdrop", imageProxyPath("abc", "Backdrop"))
}

func TestMetadataDiff(t *testing.T) {
	t.Parallel()

	current := domain.MediaItem{
		Title:          "Old",
		SortTitle:      "Old",
		Overview:       "Old synopsis",
		RuntimeSeconds: 100,
		Container:      "mkv",
		VideoCodec:     "h264",
		AudioCodec:     "aac",
		ImageTag:       "tag-1",
	}

	t.Run("no changes yields no writes", func(t *testing.T) {
		t.Parallel()
		same := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "aac", ImageTag: "tag-1",
		}
		assert.Empty(t, metadataDiff(current, same))
	})

	t.Run("changed fields are collected", func(t *testing.T) {
		t.Parallel()
		changed := &jellyfin.Item{
			Name: "New", SortName: "New", Overview: "New synopsis",
			Runtime: 200 * time.Second, Container: "mp4",
			VideoCodec: "hevc", AudioCodec: "eac3", ImageTag: "tag-2",
		}
		diff := metadataDiff(current, changed)
		assert.Equal(t, "New", diff["title"])
		assert.Equal(t, "New", diff["sort_title"])
		assert.Equal(t, "New synopsis", diff["overview"])
		assert.Equal(t, 200, diff["runtime_seconds"])
		assert.Equal(t, "mp4", diff["container"])
		assert.Equal(t, "hevc", diff["video_codec"])
		assert.Equal(t, "eac3", diff["audio_codec"])
		assert.Equal(t, "tag-2", diff["image_tag"])
	})

	t.Run("empty upstream values never blank local data", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, metadataDiff(current, &jellyfin.Item{}),
			"a sparse Jellyfin response must not erase cached metadata")
	})

	t.Run("a recovered item is un-orphaned", func(t *testing.T) {
		t.Parallel()
		now := time.Now().UTC()
		orphaned := current
		orphaned.OrphanedAt = &now

		diff := metadataDiff(orphaned, &jellyfin.Item{Name: "Old"})
		require.Contains(t, diff, "orphaned_at")
		assert.Nil(t, diff["orphaned_at"], "reappearing in Jellyfin clears the orphan flag")
	})
}

func TestIsUniqueViolation(t *testing.T) {
	t.Parallel()
	assert.False(t, isUniqueViolation(nil))
	assert.False(t, isUniqueViolation(assert.AnError))
	assert.True(t, isUniqueViolation(&pgconn.PgError{Code: "23505"}))
	assert.False(t, isUniqueViolation(&pgconn.PgError{Code: "23503"}), "a foreign-key failure is not retryable")
}
