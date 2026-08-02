package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	baserepo "github.com/aleksclark/primer/server/internal/repo"
	basetestutil "github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/tv/auth"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// querier returns a savepoint-wrapped transaction for a test.
func querier(t *testing.T) baserepo.Querier {
	t.Helper()
	return basetestutil.NewSavepointQuerier(tvtestutil.Tx(t))
}

func TestColumnsDerivedFromDBTags(t *testing.T) {
	t.Parallel()

	// The repo layer derives its column list from the domain struct, so the
	// two cannot drift apart.
	assert.Contains(t, tvrepo.MediaItems.Config().Columns, "jellyfin_item_id")
	assert.Contains(t, tvrepo.Devices.Config().Columns, "token_hash")
	assert.Contains(t, tvrepo.PlayGrants.Config().Columns, "expires_at")
	assert.NotContains(t, tvrepo.MediaItems.Config().Columns, "-")
}

func TestCatalogResolution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	open := factory.MediaItem(t, q)
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": open.ID})

	closed := factory.MediaItem(t, q)
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": closed.ID,
		"starts_at":     now.Add(-3 * time.Hour),
		"ends_at":       now.Add(-time.Hour),
	})

	entries, err := tvrepo.Catalog(ctx, q, now)
	require.NoError(t, err)

	ids := map[string]bool{}
	for _, e := range entries {
		ids[e.MediaItem.ID] = true
		assert.NotEmpty(t, e.AvailabilityWindowID, "the window that authorizes the play is returned")
		assert.True(t, e.WindowEndsAt.After(now))
	}
	assert.True(t, ids[open.ID])
	assert.False(t, ids[closed.ID])
}

func TestCatalogDeduplicatesOverlappingWindows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	item := factory.MediaItem(t, q)
	soonest := factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": item.ID,
		"starts_at":     now.Add(-time.Hour),
		"ends_at":       now.Add(2 * time.Hour),
	})
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": item.ID,
		"starts_at":     now.Add(-time.Hour),
		"ends_at":       now.Add(48 * time.Hour),
	})

	entries, err := tvrepo.Catalog(ctx, q, now)
	require.NoError(t, err)

	var seen int
	for _, e := range entries {
		if e.MediaItem.ID == item.ID {
			seen++
			assert.Equal(t, soonest.ID, e.AvailabilityWindowID,
				"the window closing first is consumed first")
		}
	}
	assert.Equal(t, 1, seen, "an item appears once even with overlapping windows")
}

func TestCatalogScreensOnDirectPlay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	item := factory.MediaItem(t, q, factory.Override{"direct_play_ok": false})
	factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	entries, err := tvrepo.Catalog(ctx, q, now)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, item.ID, e.MediaItem.ID,
			"the catalog withholds items the client cannot decode")
	}

	// The single-item lookup still finds it, so the grant handler can tell
	// "unavailable" apart from "incompatible" and report which it is.
	entry, err := tvrepo.CatalogEntryFor(ctx, q, item.ID, now)
	require.NoError(t, err)
	assert.False(t, entry.DirectPlayOK)
}

func TestCatalogEntryForRespectsTheLedger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	item := factory.MediaItem(t, q)
	window := factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	entry, err := tvrepo.CatalogEntryFor(ctx, q, item.ID, now)
	require.NoError(t, err)
	assert.Equal(t, window.ID, entry.AvailabilityWindowID)

	_, err = tvrepo.ConsumePlay(ctx, q, item.ID, window.ID, nil, now)
	require.NoError(t, err)

	_, err = tvrepo.CatalogEntryFor(ctx, q, item.ID, now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound, "a consumed play is no longer offerable")

	_, err = tvrepo.CatalogEntryFor(ctx, q, "00000000-0000-0000-0000-000000000000", now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound)
}

func TestConsumePlayIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	item := factory.MediaItem(t, q)
	window := factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})

	first, err := tvrepo.ConsumePlay(ctx, q, item.ID, window.ID, nil, now)
	require.NoError(t, err)
	assert.True(t, first, "the first call writes the ledger row")

	second, err := tvrepo.ConsumePlay(ctx, q, item.ID, window.ID, nil, now)
	require.NoError(t, err)
	assert.False(t, second, "a repeat call is a no-op rather than an error")
}

func TestPairingCodeLookupAndClaim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	dev := factory.Device(t, q)

	found, err := tvrepo.DeviceByPairingCode(ctx, q, dev.PairingCode, now)
	require.NoError(t, err)
	assert.Equal(t, dev.ID, found.ID)

	_, hash, err := auth.NewToken()
	require.NoError(t, err)

	claimed, err := tvrepo.ClaimPairingCode(ctx, q, dev.PairingCode, hash, now)
	require.NoError(t, err)
	assert.Equal(t, hash, claimed.TokenHash)
	assert.Empty(t, claimed.PairingCode)
	assert.True(t, claimed.Paired())

	// The code is spent.
	_, err = tvrepo.ClaimPairingCode(ctx, q, dev.PairingCode, hash, now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound, "codes are single-use")

	_, err = tvrepo.DeviceByPairingCode(ctx, q, dev.PairingCode, now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound)
}

func TestClaimPairingCodeRejectsExpiredAndRevoked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()
	_, hash, err := auth.NewToken()
	require.NoError(t, err)

	expired := factory.Device(t, q, factory.Override{"pairing_expires_at": now.Add(-time.Minute)})
	_, err = tvrepo.ClaimPairingCode(ctx, q, expired.PairingCode, hash, now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound)

	revoked := factory.Device(t, q, factory.Override{"revoked_at": now})
	_, err = tvrepo.ClaimPairingCode(ctx, q, revoked.PairingCode, hash, now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound)
}

func TestDeviceByTokenHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	dev, token := factory.PairedDevice(t, q)

	found, err := tvrepo.DeviceByTokenHash(ctx, q, auth.HashToken(token))
	require.NoError(t, err)
	assert.Equal(t, dev.ID, found.ID)

	_, err = tvrepo.DeviceByTokenHash(ctx, q, auth.HashToken("wrong"))
	assert.ErrorIs(t, err, baserepo.ErrNotFound)

	// An empty hash must never match an unpaired device.
	factory.Device(t, q)
	_, err = tvrepo.DeviceByTokenHash(ctx, q, "")
	assert.ErrorIs(t, err, baserepo.ErrNotFound)
}

func TestDeviceByTokenHashIgnoresRevoked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	_, token := factory.PairedDevice(t, q, factory.Override{"revoked_at": time.Now().UTC()})

	_, err := tvrepo.DeviceByTokenHash(ctx, q, auth.HashToken(token))
	assert.ErrorIs(t, err, baserepo.ErrNotFound, "revoked devices lose access")
}

func TestTouchDevice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	dev, _ := factory.PairedDevice(t, q, factory.Override{"last_seen_at": nil})
	require.Nil(t, dev.LastSeenAt)

	seenAt := time.Now().UTC()
	require.NoError(t, tvrepo.TouchDevice(ctx, q, dev.ID, seenAt))

	refreshed, err := tvrepo.Devices.Get(ctx, q, dev.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.LastSeenAt)
	assert.WithinDuration(t, seenAt, *refreshed.LastSeenAt, time.Second)

	// Touching an unknown device is not an error; there is nothing to record.
	assert.NoError(t, tvrepo.TouchDevice(ctx, q, "00000000-0000-0000-0000-000000000000", seenAt))
}

func TestRedeemGrantIsSingleUse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	dev, _ := factory.PairedDevice(t, q)
	grant := factory.PlayGrant(t, q, factory.Override{"device_id": dev.ID})

	redeemed, err := tvrepo.RedeemGrant(ctx, q, grant.ID, dev.ID, now)
	require.NoError(t, err)
	require.NotNil(t, redeemed.ConsumedAt)
	assert.False(t, redeemed.Redeemable(now), "a redeemed grant is spent")

	_, err = tvrepo.RedeemGrant(ctx, q, grant.ID, dev.ID, now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound, "grants cannot be redeemed twice")
}

func TestRedeemGrantRejectsExpiredAndForeignGrants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	dev, _ := factory.PairedDevice(t, q)
	other, _ := factory.PairedDevice(t, q)

	expired := factory.PlayGrant(t, q, factory.Override{
		"device_id":  dev.ID,
		"expires_at": now.Add(-time.Second),
	})
	_, err := tvrepo.RedeemGrant(ctx, q, expired.ID, dev.ID, now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound, "an expired grant cannot be redeemed")

	mine := factory.PlayGrant(t, q, factory.Override{"device_id": dev.ID})
	_, err = tvrepo.RedeemGrant(ctx, q, mine.ID, other.ID, now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound, "grants are scoped to their device")

	// The boundary is exclusive.
	boundary := factory.PlayGrant(t, q, factory.Override{
		"device_id":  dev.ID,
		"expires_at": now,
	})
	_, err = tvrepo.RedeemGrant(ctx, q, boundary.ID, dev.ID, now)
	assert.ErrorIs(t, err, baserepo.ErrNotFound)
}

func TestGrantForDevice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	dev, _ := factory.PairedDevice(t, q)
	other, _ := factory.PairedDevice(t, q)
	grant := factory.PlayGrant(t, q, factory.Override{"device_id": dev.ID})

	found, err := tvrepo.GrantForDevice(ctx, q, grant.ID, dev.ID)
	require.NoError(t, err)
	assert.Equal(t, grant.ID, found.ID)

	_, err = tvrepo.GrantForDevice(ctx, q, grant.ID, other.ID)
	assert.ErrorIs(t, err, baserepo.ErrNotFound)
}

func TestHeartbeatCreatesThenAdvancesSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	dev, _ := factory.PairedDevice(t, q)
	grant := factory.PlayGrant(t, q, factory.Override{"device_id": dev.ID})

	created, err := tvrepo.RecordHeartbeat(ctx, q, grant, 100, 100, now)
	require.NoError(t, err)
	assert.Equal(t, grant.ID, created.GrantID)
	assert.Equal(t, 100, created.MaxPositionSeconds)

	advanced, err := tvrepo.RecordHeartbeat(ctx, q, grant, 250, 240, now)
	require.NoError(t, err)
	assert.Equal(t, created.ID, advanced.ID, "the same session is reused per grant")
	assert.Equal(t, 250, advanced.MaxPositionSeconds)
	assert.Equal(t, 240, advanced.WatchedSeconds)

	// Rewinding must not lower the recorded maxima.
	rewound, err := tvrepo.RecordHeartbeat(ctx, q, grant, 10, 5, now)
	require.NoError(t, err)
	assert.Equal(t, 250, rewound.MaxPositionSeconds)
	assert.Equal(t, 240, rewound.WatchedSeconds)

	found, err := tvrepo.SessionForGrant(ctx, q, grant.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

func TestCompleteSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	dev, _ := factory.PairedDevice(t, q)
	grant := factory.PlayGrant(t, q, factory.Override{"device_id": dev.ID})

	// Completing without any prior heartbeat still records a session.
	session, err := tvrepo.CompleteSession(ctx, q, grant, 900, 880, true, now)
	require.NoError(t, err)
	assert.True(t, session.Completed)
	require.NotNil(t, session.EndedAt)
	assert.Equal(t, 900, session.MaxPositionSeconds)

	// A later non-completing call cannot un-complete the session.
	reopened, err := tvrepo.CompleteSession(ctx, q, grant, 100, 100, false, now)
	require.NoError(t, err)
	assert.True(t, reopened.Completed, "completion is sticky")
	assert.Equal(t, 900, reopened.MaxPositionSeconds)
}

func TestSessionForGrantNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	grant := factory.PlayGrant(t, q)
	_, err := tvrepo.SessionForGrant(ctx, q, grant.ID)
	assert.ErrorIs(t, err, baserepo.ErrNotFound, "no session exists until playback starts")
}

func TestWatchOnceLedgerUniqueness(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	item := factory.MediaItem(t, q)
	window := factory.AvailabilityWindow(t, q, factory.Override{"media_item_id": item.ID})
	factory.PlayLedgerEntry(t, q, factory.Override{
		"media_item_id":          item.ID,
		"availability_window_id": window.ID,
	})

	// The database, not just application code, enforces watch-once.
	_, err := tvrepo.PlayLedger.Create(ctx, q, map[string]any{
		"media_item_id":          item.ID,
		"availability_window_id": window.ID,
	})
	assert.Error(t, err, "one ledger row per (item, window)")
}

func TestPrimerReportIdempotency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	session := factory.PlaybackSession(t, q)
	factory.PrimerReport(t, q, factory.Override{"playback_session_id": session.ID})

	// The unique key is what stops double-reporting instructional hours.
	_, err := tvrepo.PrimerReports.Create(ctx, q, map[string]any{
		"playback_session_id": session.ID,
	})
	assert.Error(t, err, "a session can only be reported once")
}

func TestMediaItemByJellyfinID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	item := factory.MediaItem(t, q, factory.Override{"jellyfin_item_id": "jf-lookup"})

	found, err := tvrepo.MediaItemByJellyfinID(ctx, q, "jf-lookup")
	require.NoError(t, err)
	assert.Equal(t, item.ID, found.ID)

	_, err = tvrepo.MediaItemByJellyfinID(ctx, q, "jf-absent")
	assert.ErrorIs(t, err, baserepo.ErrNotFound)
}

func TestAllJellyfinIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	factory.MediaItem(t, q, factory.Override{"jellyfin_item_id": "jf-aaa"})
	factory.MediaItem(t, q, factory.Override{"jellyfin_item_id": "jf-bbb"})

	ids, err := tvrepo.AllJellyfinIDs(ctx, q)
	require.NoError(t, err)
	assert.Contains(t, ids, "jf-aaa")
	assert.Contains(t, ids, "jf-bbb")
}

func TestScheduleEntriesArePersistedForLaterPhases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	airsAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	entry := factory.ScheduleEntry(t, q, factory.Override{
		"airs_at": airsAt,
		"block":   "midday",
	})

	found, err := tvrepo.ScheduleEntries.Get(ctx, q, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, "midday", found.Block)
	assert.WithinDuration(t, airsAt, found.AirsAt, time.Second)
}

func TestMediaItemDefaults(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	item, err := tvrepo.MediaItems.Create(ctx, q, map[string]any{
		"jellyfin_item_id": "jf-defaults",
		"title":            "Minimal",
	})
	require.NoError(t, err)

	assert.Equal(t, domain.ClassEntertainment, item.Class, "unclassified content is rationed by default")
	assert.True(t, item.DirectPlayOK)
	assert.Equal(t, 0, item.RuntimeSeconds)
	assert.Empty(t, item.SubjectTags)
	assert.Empty(t, item.StandardCodes)
	assert.Nil(t, item.OrphanedAt)
}
