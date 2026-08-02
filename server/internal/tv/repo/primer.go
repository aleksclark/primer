package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
)

// ReportableSession is a finished viewing that Primer has not been told about
// yet, carrying the media item's tags so the export needs no second query.
type ReportableSession struct {
	SessionID      string    `db:"session_id"`
	MediaItemID    string    `db:"media_item_id"`
	Title          string    `db:"title"`
	Class          string    `db:"class"`
	SubjectTags    []string  `db:"subject_tags"`
	StandardCodes  []string  `db:"standard_codes"`
	WatchedSeconds int       `db:"watched_seconds"`
	StartedAt      time.Time `db:"started_at"`
	EndedAt        time.Time `db:"ended_at"`
}

// reportable is the predicate for "this viewing is instructional time the LMS
// has not counted yet".
//
// Entertainment is excluded here rather than filtered downstream so those
// sessions never become candidates at all: a reporter that skipped them after
// selecting them would rescan the same growing backlog on every cycle, and the
// LMS refuses them anyway.
//
// A session qualifies once it has been closed out (ended_at is set by the
// device's completion call) and actually accrued watch time, so a grant
// redeemed and abandoned at the first frame does not book an entry.
var reportable = fmt.Sprintf(`
FROM playback_sessions s
JOIN media_items m ON m.id = s.media_item_id
WHERE s.ended_at IS NOT NULL
  AND s.watched_seconds > 0
  AND m.class IN ('%s', '%s')
  AND NOT EXISTS (
      SELECT 1 FROM primer_reports r WHERE r.playback_session_id = s.id
  )`, domain.ClassEducational, domain.ClassMixed)

// UnreportedSessions returns the oldest finished educational viewings that
// have not been exported, up to limit.
func UnreportedSessions(ctx context.Context, q repo.Querier, limit int) ([]ReportableSession, error) {
	sqlStr := `
SELECT s.id AS session_id, s.media_item_id, m.title, m.class, m.subject_tags,
       m.standard_codes, s.watched_seconds, s.started_at, s.ended_at` +
		reportable + `
ORDER BY s.ended_at ASC
LIMIT $1`

	rows, err := q.Query(ctx, sqlStr, limit)
	if err != nil {
		return nil, fmt.Errorf("query unreported sessions: %w", err)
	}
	sessions, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[ReportableSession])
	if err != nil {
		return nil, fmt.Errorf("scan unreported sessions: %w", err)
	}
	return sessions, nil
}

// RecordPrimerReport writes the export ledger row for a session. It is
// idempotent: the table's unique playback_session_id means a repeat call is a
// no-op, so a retry after a partial failure cannot book the hours twice. It
// reports whether this call created the row.
func RecordPrimerReport(ctx context.Context, q repo.Querier, sessionID, primerRef string, at time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `
INSERT INTO primer_reports (playback_session_id, reported_at, primer_ref)
VALUES ($1, $2, $3)
ON CONFLICT (playback_session_id) DO NOTHING`, sessionID, at, primerRef)
	if err != nil {
		return false, fmt.Errorf("record primer report: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
