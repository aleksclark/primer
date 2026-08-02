package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
)

// ExpireOpenWindows closes every window still open at the given instant,
// returning how many were closed.
//
// A window that has not started yet is pulled back to just before the cut-off
// as well as closed: the table requires ends_at > starts_at, so collapsing a
// future window onto the cut-off alone would violate the constraint. The
// second it keeps is meaningless except as a record that it never really ran.
func ExpireOpenWindows(ctx context.Context, q repo.Querier, at time.Time) (int, error) {
	tag, err := q.Exec(ctx, `
UPDATE availability_windows
   SET starts_at = LEAST(starts_at, $1 - interval '1 second'),
       ends_at = $1,
       updated_at = now()
 WHERE ends_at > $1`, at)
	if err != nil {
		return 0, fmt.Errorf("expire windows: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// StaleWindowCandidate is an offered item whose window has closed without the
// student ever spending it, which is what makes it worth re-offering.
type StaleWindowCandidate struct {
	domain.MediaItem
	LastWindowEndedAt time.Time `db:"last_window_ended_at" json:"lastWindowEndedAt"`
}

// SuggestRotation proposes items to put back into the on-demand catalog.
//
// The bar is deliberately low-tech: an item is a candidate when it is not
// orphaned, can direct-play, has no window open at the cut-off, and has never
// had a play spent against it. Anything already watched is excluded because
// re-offering it would either be a repeat of entertainment the ration has
// already paid for, or busywork.
func SuggestRotation(ctx context.Context, q repo.Querier, at time.Time, limit int) ([]StaleWindowCandidate, error) {
	if limit <= 0 {
		limit = 20
	}
	sqlStr := fmt.Sprintf(`
SELECT %s, COALESCE(MAX(w.ends_at), 'epoch'::timestamptz) AS last_window_ended_at
FROM media_items m
LEFT JOIN availability_windows w ON w.media_item_id = m.id
WHERE m.orphaned_at IS NULL
  AND m.direct_play_ok
  AND m.runtime_seconds > 0
  AND NOT EXISTS (
      SELECT 1 FROM availability_windows o
      WHERE o.media_item_id = m.id AND o.ends_at > $1
  )
  AND NOT EXISTS (
      SELECT 1 FROM play_ledger l WHERE l.media_item_id = m.id
  )
GROUP BY m.id
ORDER BY last_window_ended_at ASC, m.title ASC
LIMIT %d`, qualify(MediaItems.Config().Columns, "m."), limit)

	rows, err := q.Query(ctx, sqlStr, at)
	if err != nil {
		return nil, fmt.Errorf("query rotation suggestions: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[StaleWindowCandidate])
	if err != nil {
		return nil, fmt.Errorf("scan rotation suggestions: %w", err)
	}
	return out, nil
}

// OpenWindowsFor opens a window over the given items, skipping any that
// already have one open so a repeated rotation is harmless.
func OpenWindowsFor(ctx context.Context, q repo.Querier, mediaItemIDs []string, startsAt, endsAt time.Time) ([]domain.AvailabilityWindow, error) {
	if len(mediaItemIDs) == 0 {
		return nil, nil
	}
	sqlStr := fmt.Sprintf(`
INSERT INTO availability_windows (media_item_id, starts_at, ends_at)
SELECT m.id, $2, $3
FROM media_items m
WHERE m.id = ANY($1::uuid[])
  AND m.orphaned_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM availability_windows o
      WHERE o.media_item_id = m.id AND o.ends_at > $2
  )
RETURNING %s`, strings.Join(AvailabilityWindows.Config().Columns, ", "))

	rows, err := q.Query(ctx, sqlStr, mediaItemIDs, startsAt, endsAt)
	if err != nil {
		return nil, fmt.Errorf("open windows: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.AvailabilityWindow])
	if err != nil {
		return nil, fmt.Errorf("scan opened windows: %w", err)
	}
	return out, nil
}
