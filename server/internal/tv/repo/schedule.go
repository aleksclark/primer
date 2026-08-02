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

// ErrScheduleConflict reports that an entry would overlap an airing already in
// the grid. The programmed channel is a single linear stream, so two
// programmes cannot occupy the same instant.
var ErrScheduleConflict = errors.New("schedule conflict")

// Airing is a schedule entry resolved against its media item: the grid row
// plus the runtime that decides when it comes off air.
//
// An entry's span is [airs_at, airs_at + runtime). An item whose runtime is
// unknown (zero) therefore has an empty span: it never airs and never
// conflicts, which is the safe reading of "we do not know how long this is".
type Airing struct {
	domain.ScheduleEntry
	EndsAt         time.Time `db:"ends_at"`
	Title          string    `db:"title"`
	Overview       string    `db:"overview"`
	Class          string    `db:"class"`
	RuntimeSeconds int       `db:"runtime_seconds"`
	SubjectTags    []string  `db:"subject_tags"`
	DirectPlayOK   bool      `db:"direct_play_ok"`
	JellyfinItemID string    `db:"jellyfin_item_id"`
}

// OffsetSecondsAt is how far into the programme the given instant falls,
// clamped to the airing's own span.
func (a Airing) OffsetSecondsAt(at time.Time) int {
	if at.Before(a.AirsAt) {
		return 0
	}
	offset := int(at.Sub(a.AirsAt).Seconds())
	if offset > a.RuntimeSeconds {
		return a.RuntimeSeconds
	}
	return offset
}

// StartOffsetSecondsAt is the position a device tuning in at the given instant
// must start from. An entry that forbids joining in progress always starts at
// the beginning, which is what makes a short programme worth scheduling
// without the student missing its opening.
func (a Airing) StartOffsetSecondsAt(at time.Time) int {
	if !a.JoinInProgress {
		return 0
	}
	return a.OffsetSecondsAt(at)
}

// qualify prefixes a column list for a joined query.
func qualify(cols []string, prefix string) string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = prefix + c
	}
	return strings.Join(out, ", ")
}

// scheduleColumns qualifies the schedule entry columns for joined queries.
func scheduleColumns(prefix string) string {
	return qualify(ScheduleEntries.Config().Columns, prefix)
}

// airingSelect is the projection every airing query shares: the grid row, the
// media metadata an EPG needs, and the computed end of the slot.
const airingSelect = `m.title, m.overview, m.class, m.runtime_seconds, m.subject_tags,
       m.direct_play_ok, m.jellyfin_item_id,
       (s.airs_at + make_interval(secs => m.runtime_seconds)) AS ends_at
FROM schedule_entries s
JOIN media_items m ON m.id = s.media_item_id`

// overlapPredicate matches grid rows whose span intersects
// [$start, $start + candidate runtime). Half-open on both ends, so a
// programme starting exactly when the previous one ends is not a conflict.
const overlapPredicate = `
SELECT 1 FROM schedule_entries o
JOIN media_items om ON om.id = o.media_item_id
WHERE o.airs_at < %[1]s + make_interval(secs => c.runtime_seconds)
  AND o.airs_at + make_interval(secs => om.runtime_seconds) > %[1]s
  %[2]s`

// AiringAt returns the programme on air at the given instant, or
// repo.ErrNotFound when the channel is in a gap. Overlaps are rejected at
// write time, so at most one entry can match; the ordering only makes the
// result deterministic if a runtime change has since introduced one.
func AiringAt(ctx context.Context, q repo.Querier, at time.Time) (*Airing, error) {
	sqlStr := fmt.Sprintf(`
SELECT %s, %s
WHERE s.airs_at <= $1
  AND s.airs_at + make_interval(secs => m.runtime_seconds) > $1
ORDER BY s.airs_at DESC
LIMIT 1`, scheduleColumns("s."), airingSelect)
	return oneAiring(ctx, q, sqlStr, at)
}

// NextAiringAfter returns the next programme to start after the given instant,
// which is what the client shows while the channel is in a gap.
func NextAiringAfter(ctx context.Context, q repo.Querier, at time.Time) (*Airing, error) {
	sqlStr := fmt.Sprintf(`
SELECT %s, %s
WHERE s.airs_at > $1
ORDER BY s.airs_at ASC
LIMIT 1`, scheduleColumns("s."), airingSelect)
	return oneAiring(ctx, q, sqlStr, at)
}

// AiringsBetween returns every programme whose span intersects [from, to).
// Intersection rather than start time, so a film that runs past midnight still
// appears on the day it is playing.
func AiringsBetween(ctx context.Context, q repo.Querier, from, to time.Time) ([]Airing, error) {
	sqlStr := fmt.Sprintf(`
SELECT %s, %s
WHERE s.airs_at < $2
  AND s.airs_at + make_interval(secs => m.runtime_seconds) > $1
ORDER BY s.airs_at ASC`, scheduleColumns("s."), airingSelect)

	rows, err := q.Query(ctx, sqlStr, from, to)
	if err != nil {
		return nil, fmt.Errorf("query airings: %w", err)
	}
	airings, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[Airing])
	if err != nil {
		return nil, fmt.Errorf("scan airings: %w", err)
	}
	return airings, nil
}

// ConflictingAirings lists the grid rows a candidate airing would overlap.
// excludeID skips the entry being edited so moving it by a minute does not
// collide with itself.
func ConflictingAirings(ctx context.Context, q repo.Querier, mediaItemID string, airsAt time.Time, excludeID string) ([]Airing, error) {
	sqlStr := fmt.Sprintf(`
WITH c AS (SELECT runtime_seconds FROM media_items WHERE id = $1)
SELECT %s, %s
CROSS JOIN c
WHERE ($3 = '' OR s.id <> $3::uuid)
  AND s.airs_at < $2::timestamptz + make_interval(secs => c.runtime_seconds)
  AND s.airs_at + make_interval(secs => m.runtime_seconds) > $2::timestamptz
ORDER BY s.airs_at ASC`, scheduleColumns("s."), airingSelect)

	rows, err := q.Query(ctx, sqlStr, mediaItemID, airsAt, excludeID)
	if err != nil {
		return nil, fmt.Errorf("query schedule conflicts: %w", err)
	}
	airings, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[Airing])
	if err != nil {
		return nil, fmt.Errorf("scan schedule conflicts: %w", err)
	}
	return airings, nil
}

// CreateScheduleEntry inserts a grid row only if its span is clear, returning
// ErrScheduleConflict otherwise.
//
// The check lives in the INSERT's own WHERE clause rather than in a preceding
// SELECT so that the common case is decided by a single statement against a
// consistent snapshot. A database-level EXCLUDE constraint would be stronger
// still, but the end of a slot is media_items.runtime_seconds — a column in
// another table that the Jellyfin metadata sync rewrites — so an exclusion
// constraint would require a denormalised copy of the runtime kept in step by
// triggers. That is a large, drift-prone mechanism for a grid one parent edits,
// so the check stays here.
func CreateScheduleEntry(ctx context.Context, q repo.Querier, mediaItemID string, airsAt time.Time, joinInProgress bool, block string) (*domain.ScheduleEntry, error) {
	sqlStr := fmt.Sprintf(`
WITH c AS (SELECT id, runtime_seconds FROM media_items WHERE id = $1)
INSERT INTO schedule_entries (media_item_id, airs_at, join_in_progress, block)
SELECT c.id, $2, $3, $4 FROM c
WHERE NOT EXISTS (%s)
RETURNING %s`,
		fmt.Sprintf(overlapPredicate, "$2::timestamptz", ""),
		strings.Join(ScheduleEntries.Config().Columns, ", "))

	return oneScheduleEntry(ctx, q, sqlStr, mediaItemID, airsAt, joinInProgress, block)
}

// UpdateScheduleEntry rewrites a grid row's placement, refusing a move that
// would overlap another airing. All four mutable columns are written together
// because the conflict check has to see the entry's final position, not a
// half-applied one.
func UpdateScheduleEntry(ctx context.Context, q repo.Querier, id, mediaItemID string, airsAt time.Time, joinInProgress bool, block string) (*domain.ScheduleEntry, error) {
	sqlStr := fmt.Sprintf(`
WITH c AS (SELECT id, runtime_seconds FROM media_items WHERE id = $2)
UPDATE schedule_entries e
SET media_item_id = c.id,
    airs_at = $3,
    join_in_progress = $4,
    block = $5,
    updated_at = now()
FROM c
WHERE e.id = $1
  AND NOT EXISTS (%s)
RETURNING %s`,
		fmt.Sprintf(overlapPredicate, "$3::timestamptz", "AND o.id <> $1"),
		qualify(ScheduleEntries.Config().Columns, "e."))

	return oneScheduleEntry(ctx, q, sqlStr, id, mediaItemID, airsAt, joinInProgress, block)
}

// oneScheduleEntry runs a guarded write, reading "no row" as a conflict. The
// callers validate the media item first, so a missing candidate cannot be
// mistaken for one.
func oneScheduleEntry(ctx context.Context, q repo.Querier, sqlStr string, args ...any) (*domain.ScheduleEntry, error) {
	rows, err := q.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("write schedule entry: %w", err)
	}
	entry, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.ScheduleEntry])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrScheduleConflict
		}
		return nil, fmt.Errorf("scan schedule entry: %w", err)
	}
	return &entry, nil
}

func oneAiring(ctx context.Context, q repo.Querier, sqlStr string, args ...any) (*Airing, error) {
	rows, err := q.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query airing: %w", err)
	}
	airing, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[Airing])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, fmt.Errorf("scan airing: %w", err)
	}
	return &airing, nil
}
