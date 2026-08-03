package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
)

// CatalogEntry is a media item currently offerable to a device, together with
// the availability window that makes it offerable.
type CatalogEntry struct {
	domain.MediaItem
	AvailabilityWindowID string    `json:"availabilityWindowId" db:"availability_window_id" format:"uuid"`
	WindowEndsAt         time.Time `json:"windowEndsAt" db:"window_ends_at"`
}

// mediaColumns qualifies the media item column list for joined queries.
func mediaColumns() string {
	cols := MediaItems.Config().Columns
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = "m." + c
	}
	return strings.Join(out, ", ")
}

// offerable is the shared predicate for "this item may be played right now at
// $1": its Jellyfin source still exists, an availability window is open, and
// the window's play has not been consumed. Watch-once is enforced by the
// ledger's unique (media_item_id, availability_window_id) key, so the absence
// of a ledger row is exactly "still available".
const offerable = `
FROM media_items m
JOIN availability_windows w ON w.media_item_id = m.id
WHERE m.orphaned_at IS NULL
  AND $1 >= w.starts_at
  AND $1 < w.ends_at
  AND NOT EXISTS (
      SELECT 1 FROM play_ledger l
      WHERE l.media_item_id = m.id AND l.availability_window_id = w.id
  )`

// windowColumns are the joined window fields every catalog row carries.
const windowColumns = `w.id AS availability_window_id, w.ends_at AS window_ends_at`

// Catalog returns the on-demand items a device may play at the given instant.
// Items that cannot direct-play are withheld: transcoding is disabled, so
// offering them would only produce a player that fails at the first frame.
func Catalog(ctx context.Context, q repo.Querier, at time.Time) ([]CatalogEntry, error) {
	sqlStr := fmt.Sprintf(`
SELECT DISTINCT ON (m.id) %s, %s%s
  AND m.direct_play_ok
ORDER BY m.id, w.ends_at ASC`, mediaColumns(), windowColumns, offerable)

	rows, err := q.Query(ctx, sqlStr, at)
	if err != nil {
		return nil, fmt.Errorf("query catalog: %w", err)
	}
	entries, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[CatalogEntry])
	if err != nil {
		return nil, fmt.Errorf("scan catalog: %w", err)
	}
	return entries, nil
}

// CatalogEntryFor returns the catalog entry for one media item, or
// repo.ErrNotFound when the item is unavailable or already consumed. Unlike
// Catalog it does not screen on direct play, so the grant handler can tell an
// unavailable item apart from an incompatible one.
func CatalogEntryFor(ctx context.Context, q repo.Querier, mediaItemID string, at time.Time) (*CatalogEntry, error) {
	sqlStr := fmt.Sprintf(`
SELECT %s, %s%s
  AND m.id = $2
ORDER BY w.ends_at ASC
LIMIT 1`, mediaColumns(), windowColumns, offerable)

	rows, err := q.Query(ctx, sqlStr, at, mediaItemID)
	if err != nil {
		return nil, fmt.Errorf("query catalog entry: %w", err)
	}
	entry, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[CatalogEntry])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, fmt.Errorf("scan catalog entry: %w", err)
	}
	return &entry, nil
}

// deviceColumns is the device column list for hand-written queries.
func deviceColumns() string { return strings.Join(Devices.Config().Columns, ", ") }

// DeviceByPairingCode finds an unpaired device offering the given code. An
// expired or already-claimed code does not match.
func DeviceByPairingCode(ctx context.Context, q repo.Querier, code string, at time.Time) (*domain.Device, error) {
	// Codes are generated uppercase; compare case-insensitively so a D-pad
	// that slips into lowercase still pairs.
	sqlStr := fmt.Sprintf(`
SELECT %s FROM devices
WHERE upper(pairing_code) = upper($1)
  AND pairing_code <> ''
  AND revoked_at IS NULL
  AND (pairing_expires_at IS NULL OR pairing_expires_at > $2)
LIMIT 1`, deviceColumns())
	return oneDevice(ctx, q, sqlStr, code, at)
}

// DeviceByTokenHash finds the paired, unrevoked device holding the given token
// hash.
func DeviceByTokenHash(ctx context.Context, q repo.Querier, hash string) (*domain.Device, error) {
	sqlStr := fmt.Sprintf(`
SELECT %s FROM devices
WHERE token_hash = $1 AND token_hash <> '' AND revoked_at IS NULL
LIMIT 1`, deviceColumns())
	return oneDevice(ctx, q, sqlStr, hash)
}

// ClaimPairingCode exchanges a pairing code for a token hash, clearing the
// code so it cannot be reused. The conditional UPDATE makes the exchange
// atomic: two devices racing on one code, only one wins.
func ClaimPairingCode(ctx context.Context, q repo.Querier, code, tokenHash string, at time.Time) (*domain.Device, error) {
	sqlStr := fmt.Sprintf(`
UPDATE devices SET
    token_hash = $2,
    pairing_code = '',
    pairing_expires_at = NULL,
    paired_at = $3,
    last_seen_at = $3,
    updated_at = now()
WHERE upper(pairing_code) = upper($1)
  AND pairing_code <> ''
  AND revoked_at IS NULL
  AND (pairing_expires_at IS NULL OR pairing_expires_at > $3)
RETURNING %s`, deviceColumns())
	return oneDevice(ctx, q, sqlStr, code, tokenHash, at)
}

// TouchDevice records that a device just made a request.
func TouchDevice(ctx context.Context, q repo.Querier, deviceID string, at time.Time) error {
	if _, err := q.Exec(ctx,
		`UPDATE devices SET last_seen_at = $2, updated_at = now() WHERE id = $1`,
		deviceID, at); err != nil {
		return fmt.Errorf("touch device: %w", err)
	}
	return nil
}

func oneDevice(ctx context.Context, q repo.Querier, sqlStr string, args ...any) (*domain.Device, error) {
	rows, err := q.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query device: %w", err)
	}
	device, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Device])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, fmt.Errorf("scan device: %w", err)
	}
	return &device, nil
}

// RedeemGrant consumes a grant exactly once. It returns repo.ErrNotFound when
// the grant does not exist, is already consumed, has expired, or belongs to
// another device; the conditional UPDATE is what makes grants single-use even
// under concurrent requests.
func RedeemGrant(ctx context.Context, q repo.Querier, grantID, deviceID string, at time.Time) (*domain.PlayGrant, error) {
	sqlStr := fmt.Sprintf(`
UPDATE play_grants SET consumed_at = $3, updated_at = now()
WHERE id = $1
  AND device_id = $2
  AND consumed_at IS NULL
  AND expires_at > $3
RETURNING %s`, strings.Join(PlayGrants.Config().Columns, ", "))

	rows, err := q.Query(ctx, sqlStr, grantID, deviceID, at)
	if err != nil {
		return nil, fmt.Errorf("redeem grant: %w", err)
	}
	grant, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.PlayGrant])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, fmt.Errorf("scan grant: %w", err)
	}
	return &grant, nil
}

// GrantForDevice fetches a grant scoped to its owning device.
func GrantForDevice(ctx context.Context, q repo.Querier, grantID, deviceID string) (*domain.PlayGrant, error) {
	sqlStr := fmt.Sprintf(`SELECT %s FROM play_grants WHERE id = $1 AND device_id = $2`,
		strings.Join(PlayGrants.Config().Columns, ", "))
	rows, err := q.Query(ctx, sqlStr, grantID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("query grant: %w", err)
	}
	grant, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.PlayGrant])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, fmt.Errorf("scan grant: %w", err)
	}
	return &grant, nil
}

// SessionForGrant returns the playback session belonging to a grant.
func SessionForGrant(ctx context.Context, q repo.Querier, grantID string) (*domain.PlaybackSession, error) {
	sqlStr := fmt.Sprintf(`SELECT %s FROM playback_sessions WHERE grant_id = $1`,
		strings.Join(PlaybackSessions.Config().Columns, ", "))
	rows, err := q.Query(ctx, sqlStr, grantID)
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}
	session, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.PlaybackSession])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, fmt.Errorf("scan session: %w", err)
	}
	return &session, nil
}

// MaxPositionForDeviceMedia is the furthest playhead position this device has
// reached on an item across every prior session. Zero means never watched.
// Used to seed on-demand resume offsets and the seek ceiling.
func MaxPositionForDeviceMedia(ctx context.Context, q repo.Querier, deviceID, mediaItemID string) (int, error) {
	var maxPos int
	err := q.QueryRow(ctx, `
SELECT COALESCE(MAX(max_position_seconds), 0)::int
FROM playback_sessions
WHERE device_id = $1 AND media_item_id = $2`, deviceID, mediaItemID).Scan(&maxPos)
	if err != nil {
		return 0, fmt.Errorf("max position for device media: %w", err)
	}
	return maxPos, nil
}

// RecordHeartbeat advances a session's watch counters, creating the session on
// the first heartbeat. Positions only ever move forward so a client that
// rewinds or reconnects cannot lower its recorded progress.
func RecordHeartbeat(ctx context.Context, q repo.Querier, grant *domain.PlayGrant, positionSeconds, watchedSeconds int, at time.Time) (*domain.PlaybackSession, error) {
	sqlStr := fmt.Sprintf(`
INSERT INTO playback_sessions
    (grant_id, media_item_id, device_id, started_at, watched_seconds, max_position_seconds)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (grant_id) DO UPDATE SET
    watched_seconds = GREATEST(playback_sessions.watched_seconds, EXCLUDED.watched_seconds),
    max_position_seconds = GREATEST(playback_sessions.max_position_seconds, EXCLUDED.max_position_seconds),
    updated_at = now()
RETURNING %s`, strings.Join(PlaybackSessions.Config().Columns, ", "))

	rows, err := q.Query(ctx, sqlStr, grant.ID, grant.MediaItemID, grant.DeviceID, at, watchedSeconds, positionSeconds)
	if err != nil {
		return nil, fmt.Errorf("record heartbeat: %w", err)
	}
	session, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.PlaybackSession])
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	return &session, nil
}

// CompleteSession closes a session out, marking it completed when the client
// says so or when it already passed the completion threshold.
func CompleteSession(ctx context.Context, q repo.Querier, grant *domain.PlayGrant, positionSeconds, watchedSeconds int, completed bool, at time.Time) (*domain.PlaybackSession, error) {
	sqlStr := fmt.Sprintf(`
INSERT INTO playback_sessions
    (grant_id, media_item_id, device_id, started_at, ended_at, watched_seconds, max_position_seconds, completed)
VALUES ($1, $2, $3, $4, $4, $5, $6, $7)
ON CONFLICT (grant_id) DO UPDATE SET
    ended_at = $4,
    watched_seconds = GREATEST(playback_sessions.watched_seconds, EXCLUDED.watched_seconds),
    max_position_seconds = GREATEST(playback_sessions.max_position_seconds, EXCLUDED.max_position_seconds),
    completed = playback_sessions.completed OR EXCLUDED.completed,
    updated_at = now()
RETURNING %s`, strings.Join(PlaybackSessions.Config().Columns, ", "))

	rows, err := q.Query(ctx, sqlStr, grant.ID, grant.MediaItemID, grant.DeviceID, at, watchedSeconds, positionSeconds, completed)
	if err != nil {
		return nil, fmt.Errorf("complete session: %w", err)
	}
	session, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.PlaybackSession])
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	return &session, nil
}

// ConsumePlay writes the watch-once ledger row for a (media item, window). It
// is idempotent: a repeat call for the same pair is a no-op, so retries and
// duplicate completions cannot double-charge. It reports whether this call
// created the row.
func ConsumePlay(ctx context.Context, q repo.Querier, mediaItemID, windowID string, deviceID *string, at time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `
INSERT INTO play_ledger (media_item_id, device_id, availability_window_id, consumed_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (media_item_id, availability_window_id) DO NOTHING`,
		mediaItemID, deviceID, windowID, at)
	if err != nil {
		return false, fmt.Errorf("consume play: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// MediaItemByJellyfinID finds the media item mirroring a Jellyfin item.
func MediaItemByJellyfinID(ctx context.Context, q repo.Querier, jellyfinID string) (*domain.MediaItem, error) {
	sqlStr := fmt.Sprintf(`SELECT %s FROM media_items WHERE jellyfin_item_id = $1`,
		strings.Join(MediaItems.Config().Columns, ", "))
	rows, err := q.Query(ctx, sqlStr, jellyfinID)
	if err != nil {
		return nil, fmt.Errorf("query media item: %w", err)
	}
	item, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.MediaItem])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, fmt.Errorf("scan media item: %w", err)
	}
	return &item, nil
}

// AllJellyfinIDs lists the Jellyfin IDs of every imported media item, used by
// the sync job to spot library churn.
func AllJellyfinIDs(ctx context.Context, q repo.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT jellyfin_item_id FROM media_items ORDER BY jellyfin_item_id`)
	if err != nil {
		return nil, fmt.Errorf("query jellyfin ids: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("scan jellyfin ids: %w", err)
	}
	return ids, nil
}
