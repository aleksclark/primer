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
		DirectPlayOK:   true,
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
		assert.NotContains(t, diff, "direct_play_ok", "hevc+eac3 remains direct-play OK")
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

	t.Run("stale EAC3 direct_play_ok false becomes true", func(t *testing.T) {
		t.Parallel()
		stale := current
		stale.AudioCodec = "eac3"
		stale.DirectPlayOK = false
		stale.QualityNotes = "audio codec eac3"

		// Codecs unchanged upstream — policy still repairs the flag.
		same := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "eac3", ImageTag: "tag-1",
		}
		diff := metadataDiff(stale, same)
		require.Equal(t, true, diff["direct_play_ok"])
		require.Equal(t, "", diff["quality_notes"])
		assert.NotContains(t, diff, "audio_codec")
	})

	t.Run("stale DTS direct_play_ok false becomes true", func(t *testing.T) {
		t.Parallel()
		stale := current
		stale.VideoCodec = "hevc"
		stale.AudioCodec = "dts"
		stale.DirectPlayOK = false
		stale.QualityNotes = "audio codec dts"

		same := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "hevc", AudioCodec: "dts", ImageTag: "tag-1",
		}
		diff := metadataDiff(stale, same)
		assert.Equal(t, true, diff["direct_play_ok"])
		assert.Equal(t, "", diff["quality_notes"])
	})

	t.Run("unsupported codec flips direct_play_ok true to false", func(t *testing.T) {
		t.Parallel()
		// Remote reports a newly-unsupported audio track.
		changed := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "truehd", ImageTag: "tag-1",
		}
		diff := metadataDiff(current, changed)
		assert.Equal(t, "truehd", diff["audio_codec"])
		assert.Equal(t, false, diff["direct_play_ok"])
		assert.Equal(t, "audio codec truehd", diff["quality_notes"])
	})

	t.Run("stale auto-note cleaned without flag change", func(t *testing.T) {
		t.Parallel()
		stale := current
		stale.AudioCodec = "eac3"
		stale.DirectPlayOK = true
		stale.QualityNotes = "audio codec eac3"

		same := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "eac3", ImageTag: "tag-1",
		}
		diff := metadataDiff(stale, same)
		assert.NotContains(t, diff, "direct_play_ok")
		assert.Equal(t, "", diff["quality_notes"])
	})

	t.Run("manual quality notes withhold direct play", func(t *testing.T) {
		t.Parallel()
		// Curator unchecked direct-play and left a manual note — sync must not
		// clobber either the flag or the note just because codecs are allowlisted.
		withheld := current
		withheld.AudioCodec = "eac3"
		withheld.DirectPlayOK = false
		withheld.QualityNotes = "curator: family movie night pick"

		same := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "eac3", ImageTag: "tag-1",
		}
		diff := metadataDiff(withheld, same)
		assert.NotContains(t, diff, "direct_play_ok", "manual withhold must survive sync")
		assert.NotContains(t, diff, "quality_notes", "manual notes must not be wiped")
	})

	t.Run("blank quality notes withhold direct play", func(t *testing.T) {
		t.Parallel()
		// Curator unchecked without a note (or never annotated). Only
		// recognizably auto-generated codec notes may repair false→true.
		withheld := current
		withheld.AudioCodec = "eac3"
		withheld.DirectPlayOK = false
		withheld.QualityNotes = ""

		same := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "eac3", ImageTag: "tag-1",
		}
		diff := metadataDiff(withheld, same)
		assert.NotContains(t, diff, "direct_play_ok", "blank note is not safe stale-policy evidence")
		assert.NotContains(t, diff, "quality_notes")
	})

	t.Run("unsupported auto-note does not authorize false to true", func(t *testing.T) {
		t.Parallel()
		// An auto note naming a still-unsupported codec (truehd) is not stale
		// allowlist evidence — even when the current codecs are allowlisted.
		withheld := current
		withheld.AudioCodec = "eac3"
		withheld.DirectPlayOK = false
		withheld.QualityNotes = "audio codec truehd"

		same := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "eac3", ImageTag: "tag-1",
		}
		diff := metadataDiff(withheld, same)
		assert.NotContains(t, diff, "direct_play_ok", "truehd auto note must not authorize repair")
		// Notes still reconcile to empty because codecs are now OK and the note is auto-shaped.
		assert.Equal(t, "", diff["quality_notes"])
	})

	t.Run("mixed auto-note with unsupported codec does not authorize repair", func(t *testing.T) {
		t.Parallel()
		withheld := current
		withheld.VideoCodec = "h264"
		withheld.AudioCodec = "eac3"
		withheld.DirectPlayOK = false
		withheld.QualityNotes = "video codec av1; audio codec eac3"

		same := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "eac3", ImageTag: "tag-1",
		}
		diff := metadataDiff(withheld, same)
		assert.NotContains(t, diff, "direct_play_ok", "partially unsupported auto note must not authorize repair")
	})

	t.Run("multi-sync blank withhold survives truehd then eac3", func(t *testing.T) {
		t.Parallel()
		// Regression: blank curator withhold → truehd sync writes auto note →
		// later eac3 sync must NOT treat that auto note as stale-allowlist
		// evidence and flip direct_play_ok back to true.
		item := current
		item.DirectPlayOK = false
		item.QualityNotes = ""
		item.AudioCodec = "aac"

		truehdRemote := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "truehd", ImageTag: "tag-1",
		}
		afterTrueHD := metadataDiff(item, truehdRemote)
		// Already false — no flag write, but codec + auto note are recorded.
		assert.NotContains(t, afterTrueHD, "direct_play_ok")
		require.Equal(t, "truehd", afterTrueHD["audio_codec"])
		require.Equal(t, "audio codec truehd", afterTrueHD["quality_notes"])

		// Apply first-sync writes as the DB would.
		item.DirectPlayOK = false
		item.AudioCodec = "truehd"
		item.QualityNotes = "audio codec truehd"

		eac3Remote := &jellyfin.Item{
			Name: "Old", SortName: "Old", Overview: "Old synopsis",
			Runtime: 100 * time.Second, Container: "mkv",
			VideoCodec: "h264", AudioCodec: "eac3", ImageTag: "tag-1",
		}
		afterEAC3 := metadataDiff(item, eac3Remote)
		assert.Equal(t, "eac3", afterEAC3["audio_codec"])
		assert.NotContains(t, afterEAC3, "direct_play_ok",
			"curator blank withhold must survive truehd→eac3 multi-sync")
		// Auto note may clear once codecs are OK; the flag must stay false.
		if notes, ok := afterEAC3["quality_notes"]; ok {
			assert.Equal(t, "", notes)
		}
	})
}

func TestIsUniqueViolation(t *testing.T) {
	t.Parallel()
	assert.False(t, isUniqueViolation(nil))
	assert.False(t, isUniqueViolation(assert.AnError))
	assert.True(t, isUniqueViolation(&pgconn.PgError{Code: "23505"}))
	assert.False(t, isUniqueViolation(&pgconn.PgError{Code: "23503"}), "a foreign-key failure is not retryable")
}
