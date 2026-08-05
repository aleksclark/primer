package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/domain"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

func TestDeviceRegistrationIssuesPairingCode(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t)

	resp := h.Post("/devices", objMap{"name": "Living Room TV", "kind": "tv_box"})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	dev := decode[domain.Device](t, resp.Body.Bytes())

	assert.Equal(t, "Living Room TV", dev.Name)
	assert.Equal(t, domain.DeviceTVBox, dev.Kind)
	assert.NotEmpty(t, dev.PairingCode, "a code is minted server-side")
	require.NotNil(t, dev.PairingExpiresAt)
	assert.True(t, dev.PairingExpiresAt.After(time.Now().UTC()), "the code has not expired yet")
	assert.Nil(t, dev.PairedAt)
	assert.False(t, dev.Paired())

	// The default kind is a tablet.
	resp = h.Post("/devices", objMap{"name": "Tablet"})
	require.Equal(t, http.StatusCreated, resp.Code)
	assert.Equal(t, domain.DeviceTablet, decode[domain.Device](t, resp.Body.Bytes()).Kind)
}

func TestDeviceCRUD(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev := factory.Device(t, q, factory.Override{"name": "Kitchen"})

	resp := h.Get("/devices/" + dev.ID)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "Kitchen", decode[domain.Device](t, resp.Body.Bytes()).Name)

	resp = h.Patch("/devices/"+dev.ID, objMap{"name": "Kitchen Tablet"})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	assert.Equal(t, "Kitchen Tablet", decode[domain.Device](t, resp.Body.Bytes()).Name)

	resp = h.Get("/devices?q=Kitchen")
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, 1, decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes()).TotalCount)

	require.Equal(t, http.StatusNoContent, h.Delete("/devices/"+dev.ID).Code)
	assert.Equal(t, http.StatusNotFound, h.Get("/devices/"+dev.ID).Code)
}

func TestMediaItemCRUD(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t)

	resp := h.Post("/media-items", objMap{
		"jellyfinItemId": "jf-crud-1",
		"title":          "Apollo 13",
		"class":          domain.ClassMixed,
		"runtimeSeconds": 8160,
		"subjectTags":    []string{"science", "history"},
		"standardCodes":  []string{"TN.SCI.8.PS2.1"},
		"container":      "mkv",
		"videoCodec":     "hevc",
		"audioCodec":     "eac3",
		"qualityNotes":   "10-bit HEVC; verify on the box",
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	item := decode[domain.MediaItem](t, resp.Body.Bytes())

	assert.Equal(t, "Apollo 13", item.Title)
	assert.Equal(t, domain.ClassMixed, item.Class)
	assert.Equal(t, []string{"science", "history"}, item.SubjectTags)
	assert.Equal(t, []string{"TN.SCI.8.PS2.1"}, item.StandardCodes)
	assert.True(t, item.DirectPlayOK, "items default to direct-play capable")

	// Reclassification.
	resp = h.Patch("/media-items/"+item.ID, objMap{
		"class":        domain.ClassEducational,
		"directPlayOk": false,
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	updated := decode[domain.MediaItem](t, resp.Body.Bytes())
	assert.Equal(t, domain.ClassEducational, updated.Class)
	assert.False(t, updated.DirectPlayOK)
	assert.Equal(t, "Apollo 13", updated.Title, "unspecified fields are unchanged")

	// The Jellyfin ID is unique.
	dup := h.Post("/media-items", objMap{
		"jellyfinItemId": "jf-crud-1",
		"title":          "Duplicate",
		"class":          domain.ClassEntertainment,
	})
	assert.Equal(t, http.StatusConflict, dup.Code, "one media item per Jellyfin item")

	require.Equal(t, http.StatusNoContent, h.Delete("/media-items/"+item.ID).Code)
}

func TestMediaItemValidation(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t)

	resp := h.Post("/media-items", objMap{
		"jellyfinItemId": "jf-bad",
		"title":          "Bad Class",
		"class":          "not-a-class",
	})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	resp = h.Post("/media-items", objMap{"title": "No Jellyfin ID", "class": domain.ClassMixed})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
}

func TestAvailabilityWindowCRUD(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	item := factory.MediaItem(t, q)
	now := time.Now().UTC().Truncate(time.Second)

	resp := h.Post("/availability-windows", objMap{
		"mediaItemId": item.ID,
		"startsAt":    now.Format(time.RFC3339),
		"endsAt":      now.Add(24 * time.Hour).Format(time.RFC3339),
		"note":        "force-laws week",
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	window := decode[domain.AvailabilityWindow](t, resp.Body.Bytes())
	assert.Equal(t, item.ID, window.MediaItemID)
	assert.Equal(t, "force-laws week", window.Note)

	// Expiring a window early is how rotation works.
	resp = h.Patch("/availability-windows/"+window.ID, objMap{
		"endsAt": now.Add(-time.Hour).Format(time.RFC3339),
	})
	require.Equal(t, http.StatusUnprocessableEntity, resp.Code,
		"a window cannot end before it starts")

	resp = h.Patch("/availability-windows/"+window.ID, objMap{
		"endsAt": now.Add(time.Hour).Format(time.RFC3339),
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	list := h.Get("/availability-windows?filter=media_item_id:" + item.ID)
	require.Equal(t, http.StatusOK, list.Code)
	assert.Equal(t, 1, decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, list.Body.Bytes()).TotalCount)

	require.Equal(t, http.StatusNoContent, h.Delete("/availability-windows/"+window.ID).Code)
}

func TestAvailabilityWindowRequiresRealMediaItem(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t)
	now := time.Now().UTC()

	resp := h.Post("/availability-windows", objMap{
		"mediaItemId": "00000000-0000-0000-0000-000000000000",
		"endsAt":      now.Add(time.Hour).Format(time.RFC3339),
	})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code, "the foreign key is enforced")
}

func TestScheduleEntryCRUD(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	item := factory.MediaItem(t, q)
	airsAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)

	resp := h.Post("/schedule-entries", objMap{
		"mediaItemId": item.ID,
		"airsAt":      airsAt.Format(time.RFC3339),
		"block":       "afternoon",
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	entry := decode[domain.ScheduleEntry](t, resp.Body.Bytes())
	assert.Equal(t, "afternoon", entry.Block)
	assert.True(t, entry.JoinInProgress, "join-in-progress is the default for a linear channel")

	// The same item cannot air twice at the same instant.
	dup := h.Post("/schedule-entries", objMap{
		"mediaItemId": item.ID,
		"airsAt":      airsAt.Format(time.RFC3339),
	})
	assert.Equal(t, http.StatusConflict, dup.Code)

	resp = h.Patch("/schedule-entries/"+entry.ID, objMap{"block": "evening"})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	assert.Equal(t, "evening", decode[domain.ScheduleEntry](t, resp.Body.Bytes()).Block)

	require.Equal(t, http.StatusNoContent, h.Delete("/schedule-entries/"+entry.ID).Code)
}

func TestJellyfinBrowseFlagsImportedItems(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake(
		jellyfin.Item{ID: "jf-imported", Name: "Already Here", Runtime: time.Hour, Container: "mkv"},
		jellyfin.Item{ID: "jf-new", Name: "Brand New", Runtime: 30 * time.Minute},
	)
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
	factory.MediaItem(t, q, factory.Override{"jellyfin_item_id": "jf-imported"})

	resp := h.Get("/jellyfin/browse")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[struct {
		Items []struct {
			JellyfinItemID string `json:"jellyfinItemId"`
			Title          string `json:"title"`
			RuntimeSeconds int    `json:"runtimeSeconds"`
			Imported       bool   `json:"imported"`
		} `json:"items"`
	}](t, resp.Body.Bytes())

	require.Len(t, body.Items, 2)
	byID := map[string]bool{}
	for _, item := range body.Items {
		byID[item.JellyfinItemID] = item.Imported
	}
	assert.True(t, byID["jf-imported"], "already-imported items are flagged")
	assert.False(t, byID["jf-new"], "unimported items are offered for import")

	// Search is forwarded to Jellyfin.
	resp = h.Get("/jellyfin/browse?q=Brand&limit=10")
	require.Equal(t, http.StatusOK, resp.Code)
	filtered := decode[struct {
		Items []objMap `json:"items"`
	}](t, resp.Body.Bytes())
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, "Brand New", filtered.Items[0]["title"])
}

func TestJellyfinBrowseSurfacesUpstreamFailure(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake()
	fake.Err = assert.AnError
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})

	resp := h.Get("/jellyfin/browse")
	assert.Equal(t, http.StatusBadGateway, resp.Code, "a broken media source is a gateway error")
}

func TestJellyfinSyncUpdatesMetadata(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake(jellyfin.Item{
		ID:         "jf-sync",
		Name:       "Renamed Title",
		SortName:   "Renamed Title",
		Overview:   "A fresh synopsis.",
		Runtime:    45 * time.Minute,
		Container:  "mp4",
		VideoCodec: "hevc",
		AudioCodec: "eac3",
		ImageTag:   "new-tag",
	})
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
	item := factory.MediaItem(t, q, factory.Override{
		"jellyfin_item_id": "jf-sync",
		"title":            "Stale Title",
		"runtime_seconds":  10,
		"image_tag":        "old-tag",
	})

	resp := h.Post("/jellyfin/sync", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	summary := decode[struct {
		Checked  int      `json:"checked"`
		Updated  int      `json:"updated"`
		Orphaned []string `json:"orphaned"`
	}](t, resp.Body.Bytes())

	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, 1, summary.Updated)
	assert.Empty(t, summary.Orphaned)

	refreshed := h.Get("/media-items/" + item.ID)
	require.Equal(t, http.StatusOK, refreshed.Code)
	got := decode[domain.MediaItem](t, refreshed.Body.Bytes())
	assert.Equal(t, "Renamed Title", got.Title)
	assert.Equal(t, "A fresh synopsis.", got.Overview)
	assert.Equal(t, 2700, got.RuntimeSeconds)
	assert.Equal(t, "mp4", got.Container)
	assert.Equal(t, "hevc", got.VideoCodec)
	assert.Equal(t, "new-tag", got.ImageTag)
	assert.Nil(t, got.OrphanedAt)
}

func TestJellyfinSyncIsIdempotent(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake(jellyfin.Item{
		ID: "jf-stable", Name: "Stable", SortName: "Stable",
		Runtime: time.Hour, Container: "mkv", VideoCodec: "h264",
		AudioCodec: "aac", ImageTag: "tag-1", Overview: "Same.",
	})
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
	factory.MediaItem(t, q, factory.Override{
		"jellyfin_item_id": "jf-stable",
		"title":            "Stable",
		"sort_title":       "Stable",
		"overview":         "Same.",
		"runtime_seconds":  3600,
		"container":        "mkv",
		"video_codec":      "h264",
		"audio_codec":      "aac",
		"image_tag":        "tag-1",
	})

	resp := h.Post("/jellyfin/sync", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	assert.Equal(t, 0, decode[struct {
		Updated int `json:"updated"`
	}](t, resp.Body.Bytes()).Updated, "unchanged metadata triggers no writes")
}

func TestJellyfinSyncRepairsStaleDirectPlayOK(t *testing.T) {
	t.Parallel()
	// Codecs already match Jellyfin; only the stored policy flag is stale from
	// an older allowlist that blocked EAC3 before the Media3 FFmpeg fallback.
	// The auto-generated quality note is the safe signal that this was policy,
	// not a curator withhold.
	fake := jellyfin.NewFake(jellyfin.Item{
		ID: "jf-eac3", Name: "Atmos Night", SortName: "Atmos Night",
		Runtime: time.Hour, Container: "mkv", VideoCodec: "h264",
		AudioCodec: "eac3", ImageTag: "tag-1", Overview: "Same.",
	})
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
	item := factory.MediaItem(t, q, factory.Override{
		"jellyfin_item_id": "jf-eac3",
		"title":            "Atmos Night",
		"sort_title":       "Atmos Night",
		"overview":         "Same.",
		"runtime_seconds":  3600,
		"container":        "mkv",
		"video_codec":      "h264",
		"audio_codec":      "eac3",
		"image_tag":        "tag-1",
		"direct_play_ok":   false,
		"quality_notes":    "audio codec eac3",
	})

	resp := h.Post("/jellyfin/sync", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	summary := decode[struct {
		Checked int `json:"checked"`
		Updated int `json:"updated"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, 1, summary.Updated)

	refreshed := h.Get("/media-items/" + item.ID)
	require.Equal(t, http.StatusOK, refreshed.Code)
	got := decode[domain.MediaItem](t, refreshed.Body.Bytes())
	assert.True(t, got.DirectPlayOK, "stale auto codec notes authorize false→true repair")
	assert.Empty(t, got.QualityNotes, "stale auto codec notes are cleared")
	assert.Equal(t, "eac3", got.AudioCodec)
}

func TestJellyfinSyncPreservesCuratorDirectPlayWithhold(t *testing.T) {
	t.Parallel()
	// Allowlisted codecs + curator-unchecked direct_play_ok must not be flipped
	// back on by sync, whether the curator left a manual note or none at all.
	cases := []struct {
		name  string
		jfID  string
		notes string
	}{
		{name: "manual note", jfID: "jf-withhold-manual", notes: "curator: hold for movie night"},
		{name: "blank note", jfID: "jf-withhold-blank", notes: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := jellyfin.NewFake(jellyfin.Item{
				ID: tc.jfID, Name: "Withheld", SortName: "Withheld",
				Runtime: time.Hour, Container: "mkv", VideoCodec: "h264",
				AudioCodec: "eac3", ImageTag: "tag-1", Overview: "Same.",
			})
			h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
			item := factory.MediaItem(t, q, factory.Override{
				"jellyfin_item_id": tc.jfID,
				"title":            "Withheld",
				"sort_title":       "Withheld",
				"overview":         "Same.",
				"runtime_seconds":  3600,
				"container":        "mkv",
				"video_codec":      "h264",
				"audio_codec":      "eac3",
				"image_tag":        "tag-1",
				"direct_play_ok":   false,
				"quality_notes":    tc.notes,
			})

			resp := h.Post("/jellyfin/sync", objMap{})
			require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
			summary := decode[struct {
				Checked int `json:"checked"`
				Updated int `json:"updated"`
			}](t, resp.Body.Bytes())
			assert.Equal(t, 1, summary.Checked)
			assert.Equal(t, 0, summary.Updated, "curator withhold must not trigger a write")

			refreshed := h.Get("/media-items/" + item.ID)
			require.Equal(t, http.StatusOK, refreshed.Code)
			got := decode[domain.MediaItem](t, refreshed.Body.Bytes())
			assert.False(t, got.DirectPlayOK, "curator withhold survives jellyfin sync")
			assert.Equal(t, tc.notes, got.QualityNotes)
		})
	}
}

func TestJellyfinSyncBlocksUnsupportedDirectPlay(t *testing.T) {
	t.Parallel()
	// Unsupported codecs always force true→false even when a curator had OK set.
	fake := jellyfin.NewFake(jellyfin.Item{
		ID: "jf-truehd", Name: "TrueHD Title", SortName: "TrueHD Title",
		Runtime: time.Hour, Container: "mkv", VideoCodec: "h264",
		AudioCodec: "truehd", ImageTag: "tag-1", Overview: "Same.",
	})
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
	item := factory.MediaItem(t, q, factory.Override{
		"jellyfin_item_id": "jf-truehd",
		"title":            "TrueHD Title",
		"sort_title":       "TrueHD Title",
		"overview":         "Same.",
		"runtime_seconds":  3600,
		"container":        "mkv",
		"video_codec":      "h264",
		"audio_codec":      "aac",
		"image_tag":        "tag-1",
		"direct_play_ok":   true,
		"quality_notes":    "",
	})

	resp := h.Post("/jellyfin/sync", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	summary := decode[struct {
		Checked int `json:"checked"`
		Updated int `json:"updated"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, 1, summary.Updated)

	refreshed := h.Get("/media-items/" + item.ID)
	require.Equal(t, http.StatusOK, refreshed.Code)
	got := decode[domain.MediaItem](t, refreshed.Body.Bytes())
	assert.False(t, got.DirectPlayOK, "unsupported codec always blocks direct play")
	assert.Equal(t, "truehd", got.AudioCodec)
	assert.Equal(t, "audio codec truehd", got.QualityNotes)
}

func TestJellyfinSyncPreservesBlankWithholdAcrossTrueHDThenEAC3(t *testing.T) {
	t.Parallel()
	// Multi-sync regression: curator blank withhold must not be laundered into
	// a repairable auto note by an intermediate unsupported-codec sync.
	//   1) blank notes + false
	//   2) truehd sync writes "audio codec truehd"
	//   3) eac3 sync must leave direct_play_ok false
	fake := jellyfin.NewFake(jellyfin.Item{
		ID: "jf-multi-sync", Name: "Withheld Multi", SortName: "Withheld Multi",
		Runtime: time.Hour, Container: "mkv", VideoCodec: "h264",
		AudioCodec: "truehd", ImageTag: "tag-1", Overview: "Same.",
	})
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
	item := factory.MediaItem(t, q, factory.Override{
		"jellyfin_item_id": "jf-multi-sync",
		"title":            "Withheld Multi",
		"sort_title":       "Withheld Multi",
		"overview":         "Same.",
		"runtime_seconds":  3600,
		"container":        "mkv",
		"video_codec":      "h264",
		"audio_codec":      "aac",
		"image_tag":        "tag-1",
		"direct_play_ok":   false,
		"quality_notes":    "",
	})

	resp1 := h.Post("/jellyfin/sync", objMap{})
	require.Equal(t, http.StatusOK, resp1.Code, resp1.Body.String())
	afterTrueHD := decode[domain.MediaItem](t, h.Get("/media-items/"+item.ID).Body.Bytes())
	require.False(t, afterTrueHD.DirectPlayOK)
	require.Equal(t, "truehd", afterTrueHD.AudioCodec)
	require.Equal(t, "audio codec truehd", afterTrueHD.QualityNotes)

	// Upstream audio flips to allowlisted eac3 on a later sync.
	fake.Items[0].AudioCodec = "eac3"
	resp2 := h.Post("/jellyfin/sync", objMap{})
	require.Equal(t, http.StatusOK, resp2.Code, resp2.Body.String())

	afterEAC3 := decode[domain.MediaItem](t, h.Get("/media-items/"+item.ID).Body.Bytes())
	assert.Equal(t, "eac3", afterEAC3.AudioCodec)
	assert.False(t, afterEAC3.DirectPlayOK,
		"blank curator withhold must survive truehd auto-note then eac3 sync")
}

func TestJellyfinSyncCoversTheWholeLibrary(t *testing.T) {
	t.Parallel()
	// More items than a single page holds: stopping after one page would leave
	// the rest stale and their orphans undetected.
	const count = 150
	items := make([]jellyfin.Item, 0, count)
	for i := range count {
		items = append(items, jellyfin.Item{
			ID: fmt.Sprintf("jf-page-%03d", i), Name: "Fresh Title",
		})
	}
	fake := jellyfin.NewFake(items...)
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
	for i := range count {
		factory.MediaItem(t, q, factory.Override{
			"jellyfin_item_id": fmt.Sprintf("jf-page-%03d", i),
			"title":            "Stale Title",
		})
	}

	resp := h.Post("/jellyfin/sync", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	summary := decode[struct {
		Checked  int      `json:"checked"`
		Updated  int      `json:"updated"`
		Orphaned []string `json:"orphaned"`
	}](t, resp.Body.Bytes())

	assert.Equal(t, count, summary.Checked, "every imported item is examined")
	assert.Equal(t, count, summary.Updated, "every stale title is refreshed")
	assert.Empty(t, summary.Orphaned)
}

func TestJellyfinSyncFlagsOrphans(t *testing.T) {
	t.Parallel()
	// The fake library is empty, so every imported item looks orphaned.
	h, q, _ := tvtestutil.API(t)
	item := factory.MediaItem(t, q, factory.Override{"jellyfin_item_id": "jf-vanished"})

	resp := h.Post("/jellyfin/sync", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	summary := decode[struct {
		Checked  int      `json:"checked"`
		Orphaned []string `json:"orphaned"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, []string{item.ID}, summary.Orphaned)

	refreshed := h.Get("/media-items/" + item.ID)
	require.Equal(t, http.StatusOK, refreshed.Code)
	assert.NotNil(t, decode[domain.MediaItem](t, refreshed.Body.Bytes()).OrphanedAt,
		"the row survives so the admin UI can surface the breakage")

	// Re-syncing keeps it flagged without re-reporting a fresh orphan time.
	again := h.Post("/jellyfin/sync", objMap{})
	require.Equal(t, http.StatusOK, again.Code)
	assert.Len(t, decode[struct {
		Orphaned []string `json:"orphaned"`
	}](t, again.Body.Bytes()).Orphaned, 1)
}

func TestImageProxyServesArtwork(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake(jellyfin.Item{ID: "jf-art", Name: "Art"})
	fake.ImageData = []byte("cover-bytes")
	fake.ImageContentType = "image/png"
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
	item := factory.MediaItem(t, q, factory.Override{"jellyfin_item_id": "jf-art"})

	resp := h.Get("/images/" + item.ID + "/Primary")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	assert.Equal(t, "cover-bytes", resp.Body.String(), "artwork is proxied, not redirected")
	assert.Equal(t, "image/png", resp.Header().Get("Content-Type"))
	assert.Contains(t, resp.Header().Get("Cache-Control"), "max-age")
}

func TestImageProxyErrors(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake(jellyfin.Item{ID: "jf-known"})
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})

	// Unknown media item.
	resp := h.Get("/images/00000000-0000-0000-0000-000000000000/Primary")
	assert.Equal(t, http.StatusNotFound, resp.Code)

	// Known media item whose Jellyfin artwork is missing.
	missing := factory.MediaItem(t, q, factory.Override{"jellyfin_item_id": "jf-no-art"})
	resp = h.Get("/images/" + missing.ID + "/Primary")
	assert.Equal(t, http.StatusNotFound, resp.Code)

	// An unsupported image type is rejected by schema validation.
	item := factory.MediaItem(t, q, factory.Override{"jellyfin_item_id": "jf-known"})
	resp = h.Get("/images/" + item.ID + "/Bogus")
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
}

func TestListValidation(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t)

	assert.Equal(t, http.StatusBadRequest, h.Get("/media-items?sort=title;drop").Code,
		"sort columns are whitelisted")
	assert.Equal(t, http.StatusBadRequest, h.Get("/media-items?filter=nope:1").Code,
		"filter columns are whitelisted")
	assert.Equal(t, http.StatusBadRequest, h.Get("/media-items?filter=malformed").Code)
}

func TestMediaItemListSearchAndFilter(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)

	factory.MediaItem(t, q, factory.Override{
		"title": "Zebra Documentary", "class": domain.ClassEducational,
	})
	factory.MediaItem(t, q, factory.Override{
		"title": "Zebra Cartoons", "class": domain.ClassEntertainment,
	})

	resp := h.Get("/media-items?q=Zebra&sort=title&dir=asc")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	page := decode[struct {
		Items      []domain.MediaItem `json:"items"`
		TotalCount int                `json:"totalCount"`
	}](t, resp.Body.Bytes())
	require.Equal(t, 2, page.TotalCount)
	assert.Equal(t, "Zebra Cartoons", page.Items[0].Title, "ascending title order")

	resp = h.Get("/media-items?q=Zebra&filter=class:" + domain.ClassEducational)
	require.Equal(t, http.StatusOK, resp.Code)
	filtered := decode[struct {
		Items []domain.MediaItem `json:"items"`
	}](t, resp.Body.Bytes())
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, "Zebra Documentary", filtered.Items[0].Title)
}

func TestAdminEndpointsRequireTheAdminKey(t *testing.T) {
	t.Parallel()
	// The admin surface hands out pairing codes, so with a key configured it
	// must refuse anonymous callers outright.
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{AdminKey: "s3cret"})
	dev := factory.Device(t, q)
	item := factory.MediaItem(t, q)

	anonymous := map[string]func(args ...any) *httptest.ResponseRecorder{
		"GET /devices":      func(a ...any) *httptest.ResponseRecorder { return h.Get("/devices", a...) },
		"GET /devices/{id}": func(a ...any) *httptest.ResponseRecorder { return h.Get("/devices/"+dev.ID, a...) },
		"GET /media-items":  func(a ...any) *httptest.ResponseRecorder { return h.Get("/media-items", a...) },
		"GET /jellyfin/browse": func(a ...any) *httptest.ResponseRecorder {
			return h.Get("/jellyfin/browse", a...)
		},
		"POST /devices": func(a ...any) *httptest.ResponseRecorder {
			return h.Post("/devices", append(a, objMap{"name": "New"})...)
		},
		"DELETE /media-items/{id}": func(a ...any) *httptest.ResponseRecorder {
			return h.Delete("/media-items/"+item.ID, a...)
		},
	}
	for name, request := range anonymous {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, http.StatusUnauthorized, request().Code,
				"no credentials must be rejected")
			assert.Equal(t, http.StatusUnauthorized, request("X-Admin-Key: wrong").Code,
				"a wrong key must be rejected")
		})
	}

	// The configured key gets in, and a bearer token is accepted too.
	assert.Equal(t, http.StatusOK, h.Get("/devices", "X-Admin-Key: s3cret").Code)
	assert.Equal(t, http.StatusOK, h.Get("/devices", "Authorization: Bearer s3cret").Code)
}

func TestAdminKeyDoesNotBlockDeviceEndpoints(t *testing.T) {
	t.Parallel()
	// Device credentials and admin credentials are separate: a paired device
	// must not need the admin key to reach the catalog.
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{AdminKey: "s3cret"})
	_, token := factory.PairedDevice(t, q)

	assert.Equal(t, http.StatusOK, h.Get("/catalog", authHeader(token)).Code)
	assert.Equal(t, http.StatusUnauthorized, h.Get("/catalog", "X-Admin-Key: s3cret").Code,
		"the admin key is not a device credential")
}

func TestAdminIsOpenWhenNoKeyIsConfigured(t *testing.T) {
	t.Parallel()
	// Spec generation and a bare local checkout must keep working.
	h, _, _ := tvtestutil.API(t)
	assert.Equal(t, http.StatusOK, h.Get("/devices").Code)
}
