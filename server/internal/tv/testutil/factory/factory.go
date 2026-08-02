// Package factory provides FactoryBot-style test data builders for the TV
// schema. Each factory inserts a row through the repo layer with sensible
// defaults, accepting override maps for per-test customization. Associations
// are created automatically unless the relevant foreign key is provided.
package factory

import (
	"fmt"
	"testing"
	"time"

	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/testutil/build"
	"github.com/aleksclark/primer/server/internal/tv/auth"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// Override is a column->value map merged over factory defaults.
type Override = build.Override

var (
	n        = build.N
	merge    = build.Merge
	ensureFK = build.EnsureFK
)

// create inserts a row through the shared factory helper.
func create[T any](t *testing.T, q baserepo.Querier, res *baserepo.Resource[T], values map[string]any) *T {
	t.Helper()
	return build.Create(t, q, res, values)
}

// Device creates an unpaired device holding a fresh pairing code.
func Device(t *testing.T, q baserepo.Querier, overrides ...Override) *domain.Device {
	t.Helper()
	i := n()
	code, err := auth.NewPairingCode()
	if err != nil {
		t.Fatalf("factory pairing code: %v", err)
	}
	return create(t, q, tvrepo.Devices, merge(map[string]any{
		"name":               fmt.Sprintf("Device %d", i),
		"kind":               domain.DeviceTablet,
		"pairing_code":       code,
		"pairing_expires_at": time.Now().UTC().Add(15 * time.Minute),
	}, overrides))
}

// PairedDevice creates a device that has already exchanged its code for a
// token. The plaintext token is returned so tests can authenticate as it.
func PairedDevice(t *testing.T, q baserepo.Querier, overrides ...Override) (*domain.Device, string) {
	t.Helper()
	token, hash, err := auth.NewToken()
	if err != nil {
		t.Fatalf("factory device token: %v", err)
	}
	now := time.Now().UTC()
	merged := merge(map[string]any{
		"token_hash":   hash,
		"pairing_code": "",
		"paired_at":    now,
		"last_seen_at": now,
	}, overrides)
	device := Device(t, q, merged)
	return device, token
}

// MediaItem creates a media item.
func MediaItem(t *testing.T, q baserepo.Querier, overrides ...Override) *domain.MediaItem {
	t.Helper()
	i := n()
	return create(t, q, tvrepo.MediaItems, merge(map[string]any{
		"jellyfin_item_id": fmt.Sprintf("jf-%d", i),
		"title":            fmt.Sprintf("Media Item %d", i),
		"sort_title":       fmt.Sprintf("Media Item %d", i),
		"class":            domain.ClassEntertainment,
		"runtime_seconds":  3600,
		"container":        "mkv",
		"video_codec":      "h264",
		"audio_codec":      "aac",
		"direct_play_ok":   true,
		"image_tag":        fmt.Sprintf("tag-%d", i),
	}, overrides))
}

// AvailabilityWindow opens a window that is active now, creating a media item
// unless media_item_id is provided.
func AvailabilityWindow(t *testing.T, q baserepo.Querier, overrides ...Override) *domain.AvailabilityWindow {
	t.Helper()
	now := time.Now().UTC()
	merged := merge(map[string]any{
		"starts_at": now.Add(-time.Hour),
		"ends_at":   now.Add(24 * time.Hour),
	}, overrides)
	ensureFK(merged, "media_item_id", func() string { return MediaItem(t, q).ID })
	return create(t, q, tvrepo.AvailabilityWindows, merged)
}

// PlayLedgerEntry consumes an item's play, creating the item and window unless
// their IDs are provided.
func PlayLedgerEntry(t *testing.T, q baserepo.Querier, overrides ...Override) *domain.PlayLedgerEntry {
	t.Helper()
	merged := merge(map[string]any{
		"consumed_at": time.Now().UTC(),
	}, overrides)
	if _, ok := merged["availability_window_id"]; !ok {
		window := AvailabilityWindow(t, q, keepMediaItem(merged))
		merged["availability_window_id"] = window.ID
		merged["media_item_id"] = window.MediaItemID
	}
	ensureFK(merged, "media_item_id", func() string { return MediaItem(t, q).ID })
	return create(t, q, tvrepo.PlayLedger, merged)
}

// ScheduleEntry places an item in the grid, creating the item unless
// media_item_id is provided.
func ScheduleEntry(t *testing.T, q baserepo.Querier, overrides ...Override) *domain.ScheduleEntry {
	t.Helper()
	merged := merge(map[string]any{
		"airs_at":          time.Now().UTC().Add(time.Duration(n()) * time.Minute).Truncate(time.Second),
		"join_in_progress": true,
		"block":            "morning",
	}, overrides)
	ensureFK(merged, "media_item_id", func() string { return MediaItem(t, q).ID })
	return create(t, q, tvrepo.ScheduleEntries, merged)
}

// PlayGrant issues a redeemable grant, creating the device, item, and window
// unless their IDs are provided.
func PlayGrant(t *testing.T, q baserepo.Querier, overrides ...Override) *domain.PlayGrant {
	t.Helper()
	now := time.Now().UTC()
	merged := merge(map[string]any{
		"mode":                 domain.ModeOnDemand,
		"stream_url":           "http://jellyfin.test/Videos/jf-1/stream?static=true",
		"start_offset_seconds": 0,
		"issued_at":            now,
		"expires_at":           now.Add(5 * time.Minute),
	}, overrides)
	if _, ok := merged["availability_window_id"]; !ok {
		window := AvailabilityWindow(t, q, keepMediaItem(merged))
		merged["availability_window_id"] = window.ID
		merged["media_item_id"] = window.MediaItemID
	}
	ensureFK(merged, "media_item_id", func() string { return MediaItem(t, q).ID })
	ensureFK(merged, "device_id", func() string {
		device, _ := PairedDevice(t, q)
		return device.ID
	})
	return create(t, q, tvrepo.PlayGrants, merged)
}

// PlaybackSession records a session, creating a grant unless grant_id is
// provided.
func PlaybackSession(t *testing.T, q baserepo.Querier, overrides ...Override) *domain.PlaybackSession {
	t.Helper()
	merged := merge(map[string]any{
		"started_at":           time.Now().UTC(),
		"watched_seconds":      0,
		"max_position_seconds": 0,
		"completed":            false,
	}, overrides)
	if _, ok := merged["grant_id"]; !ok {
		grant := PlayGrant(t, q, keepMediaItem(merged))
		merged["grant_id"] = grant.ID
		merged["media_item_id"] = grant.MediaItemID
		merged["device_id"] = grant.DeviceID
	}
	ensureFK(merged, "media_item_id", func() string { return MediaItem(t, q).ID })
	ensureFK(merged, "device_id", func() string {
		device, _ := PairedDevice(t, q)
		return device.ID
	})
	return create(t, q, tvrepo.PlaybackSessions, merged)
}

// PrimerReport records an export, creating a session unless
// playback_session_id is provided.
func PrimerReport(t *testing.T, q baserepo.Querier, overrides ...Override) *domain.PrimerReport {
	t.Helper()
	merged := merge(map[string]any{
		"reported_at": time.Now().UTC(),
		"primer_ref":  fmt.Sprintf("primer-%d", n()),
	}, overrides)
	ensureFK(merged, "playback_session_id", func() string { return PlaybackSession(t, q).ID })
	return create(t, q, tvrepo.PrimerReports, merged)
}

// keepMediaItem forwards a caller-specified media item to a nested factory so
// the association stays coherent when only the item was pinned.
func keepMediaItem(merged map[string]any) Override {
	if id, ok := merged["media_item_id"]; ok {
		return Override{"media_item_id": id}
	}
	return Override{}
}
