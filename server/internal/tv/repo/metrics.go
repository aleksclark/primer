package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/repo"
)

// WatchTimeByClass is total viewing for one media class over a reporting window.
type WatchTimeByClass struct {
	Class          string `db:"class" json:"class"`
	Sessions       int    `db:"sessions" json:"sessions"`
	WatchedSeconds int    `db:"watched_seconds" json:"watchedSeconds"`
}

// WatchTimeBySubject is total viewing attributed to one subject tag. A single
// viewing counts once for every subject its item carries, so these rows sum to
// more than the total when items are tagged with several subjects.
type WatchTimeBySubject struct {
	Subject        string `db:"subject" json:"subject"`
	Sessions       int    `db:"sessions" json:"sessions"`
	WatchedSeconds int    `db:"watched_seconds" json:"watchedSeconds"`
}

// WatchTimeByDay is total viewing for one calendar day of the channel's
// timezone, split by whether it counted as instruction.
type WatchTimeByDay struct {
	Day                         time.Time `db:"day" json:"day" format:"date"`
	Sessions                    int       `db:"sessions" json:"sessions"`
	WatchedSeconds              int       `db:"watched_seconds" json:"watchedSeconds"`
	InstructionalWatchedSeconds int       `db:"instructional_watched_seconds" json:"instructionalWatchedSeconds"`
}

// CompletionStats describes how much of what was started got finished.
type CompletionStats struct {
	Sessions       int `db:"sessions" json:"sessions"`
	Completed      int `db:"completed" json:"completed"`
	WatchedSeconds int `db:"watched_seconds" json:"watchedSeconds"`
}

// EntertainmentUsage is the ration ledger: how many single viewings were spent
// against how many were offered in the window.
type EntertainmentUsage struct {
	WindowsOffered int `db:"windows_offered" json:"windowsOffered"`
	PlaysUsed      int `db:"plays_used" json:"playsUsed"`
}

// sessionsInWindow is the join every metric shares: a finished viewing with
// the item it played. Sessions are attributed by when they started, which is
// the instant the student sat down.
const sessionsInWindow = `
FROM playback_sessions s
JOIN play_grants g ON g.id = s.grant_id
JOIN media_items m ON m.id = g.media_item_id
WHERE s.started_at >= $1 AND s.started_at < $2`

// WatchTimeByClassBetween totals viewing per media class.
func WatchTimeByClassBetween(ctx context.Context, q repo.Querier, from, to time.Time) ([]WatchTimeByClass, error) {
	sqlStr := fmt.Sprintf(`
SELECT m.class, COUNT(*)::int AS sessions, COALESCE(SUM(s.watched_seconds), 0)::int AS watched_seconds
%s
GROUP BY m.class
ORDER BY watched_seconds DESC`, sessionsInWindow)
	return collect[WatchTimeByClass](ctx, q, "watch time by class", sqlStr, from, to)
}

// WatchTimeBySubjectBetween totals viewing per subject tag.
func WatchTimeBySubjectBetween(ctx context.Context, q repo.Querier, from, to time.Time) ([]WatchTimeBySubject, error) {
	sqlStr := `
SELECT subject, COUNT(*)::int AS sessions, COALESCE(SUM(s.watched_seconds), 0)::int AS watched_seconds
FROM playback_sessions s
JOIN play_grants g ON g.id = s.grant_id
JOIN media_items m ON m.id = g.media_item_id
CROSS JOIN LATERAL unnest(m.subject_tags) AS subject
WHERE s.started_at >= $1 AND s.started_at < $2
GROUP BY subject
ORDER BY watched_seconds DESC`
	return collect[WatchTimeBySubject](ctx, q, "watch time by subject", sqlStr, from, to)
}

// WatchTimeByDayBetween totals viewing per calendar day of the given zone.
// Only educational and mixed viewing counts as instructional, matching the
// rule the LMS enforces on ingest.
func WatchTimeByDayBetween(ctx context.Context, q repo.Querier, from, to time.Time, zone string) ([]WatchTimeByDay, error) {
	sqlStr := fmt.Sprintf(`
SELECT (s.started_at AT TIME ZONE $3)::date AS day,
       COUNT(*)::int AS sessions,
       COALESCE(SUM(s.watched_seconds), 0)::int AS watched_seconds,
       COALESCE(SUM(s.watched_seconds) FILTER (WHERE m.class IN ('educational', 'mixed')), 0)::int
         AS instructional_watched_seconds
%s
GROUP BY day
ORDER BY day`, sessionsInWindow)
	return collect[WatchTimeByDay](ctx, q, "watch time by day", sqlStr, from, to, zone)
}

// CompletionStatsBetween reports how many started viewings ran to the end.
func CompletionStatsBetween(ctx context.Context, q repo.Querier, from, to time.Time) (*CompletionStats, error) {
	sqlStr := fmt.Sprintf(`
SELECT COUNT(*)::int AS sessions,
       COUNT(*) FILTER (WHERE s.completed)::int AS completed,
       COALESCE(SUM(s.watched_seconds), 0)::int AS watched_seconds
%s`, sessionsInWindow)
	rows, err := q.Query(ctx, sqlStr, from, to)
	if err != nil {
		return nil, fmt.Errorf("query completion stats: %w", err)
	}
	stats, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[CompletionStats])
	if err != nil {
		return nil, fmt.Errorf("scan completion stats: %w", err)
	}
	return &stats, nil
}

// EntertainmentUsageBetween compares rationed viewings spent against offered.
// Windows are counted by when they open, so the figure answers "of what the
// parent put out this week, how much was used".
func EntertainmentUsageBetween(ctx context.Context, q repo.Querier, from, to time.Time) (*EntertainmentUsage, error) {
	sqlStr := `
SELECT
  (SELECT COUNT(*)::int
     FROM availability_windows w
     JOIN media_items m ON m.id = w.media_item_id
    WHERE m.class = 'entertainment'
      AND w.starts_at >= $1 AND w.starts_at < $2) AS windows_offered,
  (SELECT COUNT(*)::int
     FROM play_ledger l
     JOIN media_items m ON m.id = l.media_item_id
    WHERE m.class = 'entertainment'
      AND l.consumed_at >= $1 AND l.consumed_at < $2) AS plays_used`
	rows, err := q.Query(ctx, sqlStr, from, to)
	if err != nil {
		return nil, fmt.Errorf("query entertainment usage: %w", err)
	}
	usage, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[EntertainmentUsage])
	if err != nil {
		return nil, fmt.Errorf("scan entertainment usage: %w", err)
	}
	return &usage, nil
}

// collect runs a metric query and scans it into T.
func collect[T any](ctx context.Context, q repo.Querier, what, sqlStr string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", what, err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[T])
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", what, err)
	}
	return out, nil
}
