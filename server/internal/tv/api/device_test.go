package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/domain"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// decode unmarshals a JSON response body into T, failing the test on error.
func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(body, &out), "unmarshal response: %s", string(body))
	return out
}

type objMap = map[string]any

// authHeader formats a device token as a bearer credential.
func authHeader(token string) string { return "Authorization: Bearer " + token }

func TestHealth(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t)

	resp := h.Get("/health")
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "ok", decode[objMap](t, resp.Body.Bytes())["status"])
}

func TestPairingFlow(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev := factory.Device(t, q)

	resp := h.Post("/devices/pair", objMap{"code": dev.PairingCode})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	paired := decode[struct {
		Token  string        `json:"token"`
		Device domain.Device `json:"device"`
	}](t, resp.Body.Bytes())

	require.NotEmpty(t, paired.Token)
	assert.Equal(t, dev.ID, paired.Device.ID)
	assert.NotNil(t, paired.Device.PairedAt)
	assert.Empty(t, paired.Device.PairingCode, "the code is cleared once claimed")
	assert.NotContains(t, resp.Body.String(), "tokenHash", "the stored hash is never serialized")

	// The token now authenticates device requests.
	catalog := h.Get("/catalog", authHeader(paired.Token))
	assert.Equal(t, http.StatusOK, catalog.Code, catalog.Body.String())

	// The same code cannot be redeemed twice.
	replay := h.Post("/devices/pair", objMap{"code": dev.PairingCode})
	assert.Equal(t, http.StatusForbidden, replay.Code, "pairing codes are single-use")
}

func TestPairingRejectsBadCodes(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)

	unknown := h.Post("/devices/pair", objMap{"code": "ZZZZZZ"})
	assert.Equal(t, http.StatusForbidden, unknown.Code)

	expired := factory.Device(t, q, factory.Override{
		"pairing_expires_at": time.Now().UTC().Add(-time.Minute),
	})
	resp := h.Post("/devices/pair", objMap{"code": expired.PairingCode})
	assert.Equal(t, http.StatusForbidden, resp.Code, "expired codes are refused")

	revoked := factory.Device(t, q, factory.Override{"revoked_at": time.Now().UTC()})
	resp = h.Post("/devices/pair", objMap{"code": revoked.PairingCode})
	assert.Equal(t, http.StatusForbidden, resp.Code, "revoked devices cannot pair")
}

func TestDeviceEndpointsRequireAuth(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	item := factory.MediaItem(t, q)
	grant := factory.PlayGrant(t, q)

	// Each case is a request builder so GET and POST bodies stay well-formed.
	cases := map[string]func(args ...any) *httptest.ResponseRecorder{
		"GET /catalog": func(args ...any) *httptest.ResponseRecorder {
			return h.Get("/catalog", args...)
		},
		"POST /media/{id}/grant": func(args ...any) *httptest.ResponseRecorder {
			return h.Post("/media/"+item.ID+"/grant", append(args, objMap{})...)
		},
		"POST /grants/{id}/heartbeat": func(args ...any) *httptest.ResponseRecorder {
			return h.Post("/grants/"+grant.ID+"/heartbeat", append(args, objMap{"positionSeconds": 1})...)
		},
		"POST /grants/{id}/complete": func(args ...any) *httptest.ResponseRecorder {
			return h.Post("/grants/"+grant.ID+"/complete", append(args, objMap{"positionSeconds": 1})...)
		},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, http.StatusUnauthorized, request().Code,
				"no credentials must be rejected")
			assert.Equal(t, http.StatusUnauthorized, request(authHeader("not-a-real-token")).Code,
				"an unknown token must be rejected")
			assert.Equal(t, http.StatusUnauthorized, request("Authorization: Basic abc").Code,
				"a non-bearer scheme must be rejected")
		})
	}
}

func TestRevokedDeviceLosesAccess(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev, token := factory.PairedDevice(t, q)

	require.Equal(t, http.StatusOK, h.Get("/catalog", authHeader(token)).Code)

	resp := h.Patch("/devices/"+dev.ID, objMap{"revokedAt": time.Now().UTC().Format(time.RFC3339)})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	assert.Equal(t, http.StatusUnauthorized, h.Get("/catalog", authHeader(token)).Code,
		"revoking a device invalidates its token")
}

// catalogBody mirrors the catalog response for assertions.
type catalogBody struct {
	Items []struct {
		MediaItem            domain.MediaItem `json:"mediaItem"`
		AvailabilityWindowID string           `json:"availabilityWindowId"`
		WindowEndsAt         time.Time        `json:"windowEndsAt"`
		ImageURL             string           `json:"imageUrl"`
	} `json:"items"`
	ServerTime time.Time `json:"serverTime"`
}

func TestCatalogOnlyListsActiveWindows(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)
	now := time.Now().UTC()

	active := factory.MediaItem(t, q)
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": active.ID})

	past := factory.MediaItem(t, q)
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": past.ID,
		"starts_at":     now.Add(-48 * time.Hour),
		"ends_at":       now.Add(-24 * time.Hour),
	})

	future := factory.MediaItem(t, q)
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": future.ID,
		"starts_at":     now.Add(24 * time.Hour),
		"ends_at":       now.Add(48 * time.Hour),
	})

	// An item with no window at all.
	factory.MediaItem(t, q)

	resp := h.Get("/catalog", authHeader(token))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[catalogBody](t, resp.Body.Bytes())

	ids := map[string]bool{}
	for _, item := range body.Items {
		ids[item.MediaItem.ID] = true
	}
	assert.True(t, ids[active.ID], "an open window is offerable")
	assert.False(t, ids[past.ID], "an expired window is not offerable")
	assert.False(t, ids[future.ID], "a future window is not offerable")
	assert.NotZero(t, body.ServerTime, "clients trust the server clock")

	for _, item := range body.Items {
		if item.MediaItem.ID == active.ID {
			assert.Equal(t, "/images/"+active.ID+"/Primary", item.ImageURL)
		}
	}
}

func TestCatalogHidesConsumedItems(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q)
	window := factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	require.True(t, catalogContains(t, h, token, item.ID), "available before being consumed")

	factory.PlayLedgerEntry(t, q, factory.Override{
		"media_item_id":          item.ID,
		"availability_window_id": window.ID,
	})

	assert.False(t, catalogContains(t, h, token, item.ID), "a consumed play leaves the catalog")
}

func TestCatalogIgnoresOrphanedItems(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{"orphaned_at": time.Now().UTC()})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	assert.False(t, catalogContains(t, h, token, item.ID),
		"items whose Jellyfin source vanished are not offered")
}

func TestGrantIssuanceAndWatchOnceLockout(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake(jellyfin.Item{ID: "jf-watch-once", Name: "Cartoons"})
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Jellyfin: fake})
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{
		"jellyfin_item_id": "jf-watch-once",
		"class":            domain.ClassEntertainment,
		"runtime_seconds":  1000,
	})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	// Request a grant.
	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	grant := decode[struct {
		GrantID            string    `json:"grantId"`
		StreamURL          string    `json:"streamUrl"`
		StartOffsetSeconds int       `json:"startOffsetSeconds"`
		Mode               string    `json:"mode"`
		ExpiresAt          time.Time `json:"expiresAt"`
		ServerTime         time.Time `json:"serverTime"`
	}](t, resp.Body.Bytes())

	require.NotEmpty(t, grant.GrantID)
	assert.Contains(t, grant.StreamURL, "jf-watch-once")
	assert.Contains(t, grant.StreamURL, "static=true", "grants must be direct-play")
	assert.Equal(t, domain.ModeOnDemand, grant.Mode)
	assert.Equal(t, 0, grant.StartOffsetSeconds)
	assert.True(t, grant.ExpiresAt.After(grant.ServerTime), "grants are short-lived but not already expired")

	// Heartbeat partway through.
	beat := h.Post("/grants/"+grant.GrantID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 300})
	require.Equal(t, http.StatusOK, beat.Code, beat.Body.String())
	session := decode[sessionBody](t, beat.Body.Bytes())
	assert.Equal(t, 300, session.Session.MaxPositionSeconds)
	assert.False(t, session.PlayConsumed, "a partial watch does not burn the play")

	// A rewind must not lower the recorded maximum.
	rewind := h.Post("/grants/"+grant.GrantID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 100})
	require.Equal(t, http.StatusOK, rewind.Code)
	assert.Equal(t, 300, decode[sessionBody](t, rewind.Body.Bytes()).Session.MaxPositionSeconds,
		"positions only move forward")

	// Complete the session; this consumes the entertainment play.
	done := h.Post("/grants/"+grant.GrantID+"/complete", authHeader(token), objMap{"positionSeconds": 1000})
	require.Equal(t, http.StatusOK, done.Code, done.Body.String())
	finished := decode[sessionBody](t, done.Body.Bytes())
	assert.True(t, finished.Session.Completed)
	assert.NotNil(t, finished.Session.EndedAt)
	assert.True(t, finished.PlayConsumed, "completion burns the entertainment play")

	// The item is now locked out.
	assert.False(t, catalogContains(t, h, token, item.ID), "watched entertainment leaves the catalog")

	again := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, again.Code, "a second grant is refused after the play is consumed")
}

// sessionBody mirrors the playback session response.
type sessionBody struct {
	Session      domain.PlaybackSession `json:"session"`
	PlayConsumed bool                   `json:"playConsumed"`
	ServerTime   time.Time              `json:"serverTime"`
}

func TestCompletionThresholdConsumesPlay(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{
		"class":           domain.ClassEntertainment,
		"runtime_seconds": 1000,
	})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	grantID := decode[objMap](t, resp.Body.Bytes())["grantId"].(string)

	// Stopping past 80% without signalling completion still burns the play.
	done := h.Post("/grants/"+grantID+"/complete", authHeader(token), objMap{
		"positionSeconds": 850,
		"completed":       false,
	})
	require.Equal(t, http.StatusOK, done.Code, done.Body.String())
	body := decode[sessionBody](t, done.Body.Bytes())
	assert.False(t, body.Session.Completed)
	assert.True(t, body.PlayConsumed, "past the threshold counts as watched")
}

func TestStoppingEarlyKeepsThePlay(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{
		"class":           domain.ClassEntertainment,
		"runtime_seconds": 1000,
	})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code)
	grantID := decode[objMap](t, resp.Body.Bytes())["grantId"].(string)

	done := h.Post("/grants/"+grantID+"/complete", authHeader(token), objMap{
		"positionSeconds": 100,
		"completed":       false,
	})
	require.Equal(t, http.StatusOK, done.Code, done.Body.String())
	assert.False(t, decode[sessionBody](t, done.Body.Bytes()).PlayConsumed,
		"abandoning early leaves the play available")
	assert.True(t, catalogContains(t, h, token, item.ID))
}

func TestEducationalItemsAreReplayable(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{
		"class":           domain.ClassEducational,
		"runtime_seconds": 600,
	})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code)
	grantID := decode[objMap](t, resp.Body.Bytes())["grantId"].(string)

	done := h.Post("/grants/"+grantID+"/complete", authHeader(token), objMap{"positionSeconds": 600})
	require.Equal(t, http.StatusOK, done.Code, done.Body.String())
	assert.False(t, decode[sessionBody](t, done.Body.Bytes()).PlayConsumed,
		"educational content is not rationed")

	assert.True(t, catalogContains(t, h, token, item.ID), "it stays available")
	second := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	assert.Equal(t, http.StatusCreated, second.Code, "it can be watched again")
}

func TestGrantRefusedWhenUnavailable(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)
	now := time.Now().UTC()

	// No window at all.
	noWindow := factory.MediaItem(t, q)
	resp := h.Post("/media/"+noWindow.ID+"/grant", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, resp.Code, "an item with no window cannot be played")

	// Expired window.
	expired := factory.MediaItem(t, q)
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": expired.ID,
		"starts_at":     now.Add(-48 * time.Hour),
		"ends_at":       now.Add(-time.Hour),
	})
	resp = h.Post("/media/"+expired.ID+"/grant", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, resp.Code, "an expired window cannot be played")

	// Unknown media item.
	resp = h.Post("/media/00000000-0000-0000-0000-000000000000/grant", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, resp.Code)
}

func TestGrantRefusedForIncompatibleMedia(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{"direct_play_ok": false})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	assert.Equal(t, http.StatusForbidden, resp.Code,
		"transcoding is disabled, so incompatible files must fail loudly")
}

func TestExpiredGrantCannotBeUsed(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev, token := factory.PairedDevice(t, q)

	grant := factory.PlayGrant(t, q, factory.Override{
		"device_id":  dev.ID,
		"issued_at":  time.Now().UTC().Add(-time.Hour),
		"expires_at": time.Now().UTC().Add(-30 * time.Minute),
	})

	resp := h.Post("/grants/"+grant.ID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 10})
	assert.Equal(t, http.StatusForbidden, resp.Code, "an expired grant cannot start playback")

	resp = h.Post("/grants/"+grant.ID+"/complete", authHeader(token), objMap{"positionSeconds": 10})
	assert.Equal(t, http.StatusForbidden, resp.Code)
}

func TestGrantsAreScopedToTheirDevice(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	owner, _ := factory.PairedDevice(t, q)
	_, otherToken := factory.PairedDevice(t, q)

	grant := factory.PlayGrant(t, q, factory.Override{"device_id": owner.ID})

	resp := h.Post("/grants/"+grant.ID+"/heartbeat", authHeader(otherToken), objMap{"positionSeconds": 10})
	assert.Equal(t, http.StatusNotFound, resp.Code, "a device cannot use another device's grant")
}

func TestHeartbeatOnUnknownGrant(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	resp := h.Post("/grants/00000000-0000-0000-0000-000000000000/heartbeat",
		authHeader(token), objMap{"positionSeconds": 10})
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestHeartbeatDefaultsWatchedToPosition(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev, token := factory.PairedDevice(t, q)
	grant := factory.PlayGrant(t, q, factory.Override{"device_id": dev.ID})

	resp := h.Post("/grants/"+grant.ID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 42})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	session := decode[sessionBody](t, resp.Body.Bytes()).Session
	assert.Equal(t, 42, session.WatchedSeconds, "watched defaults to the reported position")
	assert.Equal(t, 42, session.MaxPositionSeconds)

	// An explicit watched value is respected.
	resp = h.Post("/grants/"+grant.ID+"/heartbeat", authHeader(token), objMap{
		"positionSeconds": 60,
		"watchedSeconds":  55,
	})
	require.Equal(t, http.StatusOK, resp.Code)
	session = decode[sessionBody](t, resp.Body.Bytes()).Session
	assert.Equal(t, 55, session.WatchedSeconds)
	assert.Equal(t, 60, session.MaxPositionSeconds)
}

func TestCompletingTwiceConsumesOnlyOnce(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{
		"class":           domain.ClassEntertainment,
		"runtime_seconds": 100,
	})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code)
	grantID := decode[objMap](t, resp.Body.Bytes())["grantId"].(string)

	first := h.Post("/grants/"+grantID+"/complete", authHeader(token), objMap{"positionSeconds": 100})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.True(t, decode[sessionBody](t, first.Body.Bytes()).PlayConsumed)

	// A retried completion must not double-charge the ledger.
	second := h.Post("/grants/"+grantID+"/complete", authHeader(token), objMap{"positionSeconds": 100})
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.False(t, decode[sessionBody](t, second.Body.Bytes()).PlayConsumed,
		"the ledger is idempotent per (item, window)")
}

func TestSecondDeviceCannotReplayAConsumedItem(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, firstToken := factory.PairedDevice(t, q)
	_, secondToken := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{
		"class":           domain.ClassEntertainment,
		"runtime_seconds": 100,
	})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(firstToken), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code)
	grantID := decode[objMap](t, resp.Body.Bytes())["grantId"].(string)
	require.Equal(t, http.StatusOK,
		h.Post("/grants/"+grantID+"/complete", authHeader(firstToken), objMap{"positionSeconds": 100}).Code)

	// The window, not the device, is what gets consumed.
	assert.False(t, catalogContains(t, h, secondToken, item.ID))
	blocked := h.Post("/media/"+item.ID+"/grant", authHeader(secondToken), objMap{})
	assert.Equal(t, http.StatusForbidden, blocked.Code,
		"one play per availability window, regardless of device")
}

func TestNewWindowRestoresAvailability(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)
	now := time.Now().UTC()

	item := factory.MediaItem(t, q, factory.Override{
		"class":           domain.ClassEntertainment,
		"runtime_seconds": 100,
	})
	old := factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": item.ID,
		"starts_at":     now.Add(-2 * time.Hour),
		"ends_at":       now.Add(time.Hour),
	})
	factory.PlayLedgerEntry(t, q, factory.Override{
		"media_item_id":          item.ID,
		"availability_window_id": old.ID,
	})
	require.False(t, catalogContains(t, h, token, item.ID))

	// Rotating the item back in with a fresh window makes it playable again.
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": item.ID,
		"starts_at":     now.Add(-time.Minute),
		"ends_at":       now.Add(48 * time.Hour),
	})
	assert.True(t, catalogContains(t, h, token, item.ID),
		"a new availability window grants a new play")
}

// catalogContains reports whether the device's catalog offers the given item.
func catalogContains(t *testing.T, h humatest.TestAPI, token, mediaItemID string) bool {
	t.Helper()
	resp := h.Get("/catalog", authHeader(token))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	for _, item := range decode[catalogBody](t, resp.Body.Bytes()).Items {
		if item.MediaItem.ID == mediaItemID {
			return true
		}
	}
	return false
}

func TestPairingCodeRotationInvalidatesOldCode(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev := factory.Device(t, q)
	oldCode := dev.PairingCode

	resp := h.Post("/devices/"+dev.ID+"/pairing-code", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	rotated := decode[domain.Device](t, resp.Body.Bytes())
	require.NotEmpty(t, rotated.PairingCode)
	assert.NotEqual(t, oldCode, rotated.PairingCode)

	assert.Equal(t, http.StatusForbidden, h.Post("/devices/pair", objMap{"code": oldCode}).Code,
		"the superseded code no longer works")
	assert.Equal(t, http.StatusCreated, h.Post("/devices/pair", objMap{"code": rotated.PairingCode}).Code)
}

func TestRotatingCodeForcesRepairing(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev, token := factory.PairedDevice(t, q)
	require.Equal(t, http.StatusOK, h.Get("/catalog", authHeader(token)).Code)

	resp := h.Post("/devices/"+dev.ID+"/pairing-code", objMap{})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	assert.Equal(t, http.StatusUnauthorized, h.Get("/catalog", authHeader(token)).Code,
		"issuing a new code revokes the old token")
}

func TestDeviceLastSeenIsRecorded(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev, token := factory.PairedDevice(t, q, factory.Override{"last_seen_at": nil})

	require.Equal(t, http.StatusOK, h.Get("/catalog", authHeader(token)).Code)

	resp := h.Get("/devices/" + dev.ID)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.NotNil(t, decode[domain.Device](t, resp.Body.Bytes()).LastSeenAt,
		"device activity is tracked for the admin UI")
}

func TestGrantExpiryIsConfigurable(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{GrantTTL: 30 * time.Second})
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q)
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	grant := decode[struct {
		ExpiresAt  time.Time `json:"expiresAt"`
		ServerTime time.Time `json:"serverTime"`
	}](t, resp.Body.Bytes())

	assert.WithinDuration(t, grant.ServerTime.Add(30*time.Second), grant.ExpiresAt, time.Second)
}

func TestConcurrentPairingClaimsOneWinner(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev := factory.Device(t, q)

	first := h.Post("/devices/pair", objMap{"code": dev.PairingCode})
	second := h.Post("/devices/pair", objMap{"code": dev.PairingCode})

	codes := []int{first.Code, second.Code}
	assert.Contains(t, codes, http.StatusCreated)
	assert.Contains(t, codes, http.StatusForbidden, "only one claim can succeed")
}

func TestGrantExpiryBoundary(t *testing.T) {
	t.Parallel()
	// A clock frozen just past the grant's expiry proves the boundary is
	// exclusive without waiting in real time.
	frozen := time.Now().UTC()
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Now: func() time.Time { return frozen }})
	dev, token := factory.PairedDevice(t, q)

	grant := factory.PlayGrant(t, q, factory.Override{
		"device_id":  dev.ID,
		"issued_at":  frozen.Add(-time.Minute),
		"expires_at": frozen,
	})

	resp := h.Post("/grants/"+grant.ID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 5})
	assert.Equal(t, http.StatusForbidden, resp.Code, "expiry is exclusive")
}

func TestPlaybackOutlivesTheGrantTTL(t *testing.T) {
	t.Parallel()
	// A 90-minute film runs far longer than the 5-minute grant TTL, so expiry
	// must only gate starting playback, never a session already underway.
	clock := time.Now().UTC()
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{
		Now:      func() time.Time { return clock },
		GrantTTL: 5 * time.Minute,
	})
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{
		"class":           domain.ClassEntertainment,
		"runtime_seconds": 5400,
	})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	grantID := decode[objMap](t, resp.Body.Bytes())["grantId"].(string)

	// Playback starts inside the TTL, which redeems the grant.
	clock = clock.Add(30 * time.Second)
	require.Equal(t, http.StatusOK,
		h.Post("/grants/"+grantID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 30}).Code)

	// Ten minutes in the grant has expired, but the film is still running.
	clock = clock.Add(10 * time.Minute)
	late := h.Post("/grants/"+grantID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 630})
	require.Equal(t, http.StatusOK, late.Code, late.Body.String())
	assert.Equal(t, 630, decode[sessionBody](t, late.Body.Bytes()).Session.MaxPositionSeconds)

	// And the closing report is still accepted 90 minutes in.
	clock = clock.Add(80 * time.Minute)
	done := h.Post("/grants/"+grantID+"/complete", authHeader(token), objMap{"positionSeconds": 5400})
	require.Equal(t, http.StatusOK, done.Code, done.Body.String())
	assert.True(t, decode[sessionBody](t, done.Body.Bytes()).PlayConsumed,
		"the play is charged at the end of a long film")
}

func TestUnredeemedGrantExpires(t *testing.T) {
	t.Parallel()
	// A grant the client never started must not be usable once it has expired.
	frozen := time.Now().UTC()
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{Now: func() time.Time { return frozen }})
	dev, token := factory.PairedDevice(t, q)

	grant := factory.PlayGrant(t, q, factory.Override{
		"device_id":   dev.ID,
		"issued_at":   frozen.Add(-time.Hour),
		"expires_at":  frozen.Add(-30 * time.Minute),
		"consumed_at": nil,
	})

	resp := h.Post("/grants/"+grant.ID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 10})
	assert.Equal(t, http.StatusForbidden, resp.Code,
		"playback cannot begin on a grant that expired unused")
}

func TestHeartbeatPastThresholdConsumesPlay(t *testing.T) {
	t.Parallel()
	// A student who watches to the end and then kills the app never sends a
	// completion, so the heartbeat has to charge the play on its own.
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{
		"class":           domain.ClassEntertainment,
		"runtime_seconds": 1000,
	})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	resp := h.Post("/media/"+item.ID+"/grant", authHeader(token), objMap{})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	grantID := decode[objMap](t, resp.Body.Bytes())["grantId"].(string)

	early := h.Post("/grants/"+grantID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 400})
	require.Equal(t, http.StatusOK, early.Code, early.Body.String())
	assert.False(t, decode[sessionBody](t, early.Body.Bytes()).PlayConsumed,
		"a partial watch leaves the play alone")

	past := h.Post("/grants/"+grantID+"/heartbeat", authHeader(token), objMap{"positionSeconds": 950})
	require.Equal(t, http.StatusOK, past.Code, past.Body.String())
	assert.True(t, decode[sessionBody](t, past.Body.Bytes()).PlayConsumed,
		"crossing the threshold burns the play without an explicit completion")

	assert.False(t, catalogContains(t, h, token, item.ID), "the item is locked out")
}

func TestCatalogWithholdsIncompatibleItems(t *testing.T) {
	t.Parallel()
	// Transcoding is disabled, so offering an item the box cannot decode would
	// only produce a player that dies on the first frame.
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	item := factory.MediaItem(t, q, factory.Override{"direct_play_ok": false})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	assert.False(t, catalogContains(t, h, token, item.ID),
		"items that cannot direct-play are not offered")
}

func TestValidationRejectsBadInput(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	dev, token := factory.PairedDevice(t, q)
	grant := factory.PlayGrant(t, q, factory.Override{"device_id": dev.ID})

	// Negative positions are rejected by schema validation.
	resp := h.Post("/grants/"+grant.ID+"/heartbeat", authHeader(token), objMap{"positionSeconds": -5})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	// A too-short pairing code is rejected before hitting the database.
	resp = h.Post("/devices/pair", objMap{"code": "ab"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	// A non-UUID media ID is rejected.
	resp = h.Post("/media/not-a-uuid/grant", authHeader(token), objMap{})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
}

func TestPlaybackSessionsAreReadOnly(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	session := factory.PlaybackSession(t, q, factory.Override{"watched_seconds": 120})

	resp := h.Get("/playback-sessions/" + session.ID)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, 120, decode[domain.PlaybackSession](t, resp.Body.Bytes()).WatchedSeconds)

	list := h.Get(fmt.Sprintf("/playback-sessions?filter=device_id:%s", session.DeviceID))
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	assert.Equal(t, 1, decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, list.Body.Bytes()).TotalCount)

	// Sessions are written by the device flow, never by the admin API.
	assert.Equal(t, http.StatusMethodNotAllowed, h.Post("/playback-sessions", objMap{}).Code)
	assert.Equal(t, http.StatusMethodNotAllowed, h.Delete("/playback-sessions/"+session.ID).Code)
}
