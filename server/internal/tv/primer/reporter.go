package primer

import (
	"context"
	"log/slog"
	"time"

	"github.com/aleksclark/primer/server/internal/repo"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// DefaultBatchSize bounds one reporting pass. A backlog is drained over
// several passes rather than in one long transaction-free burst.
const DefaultBatchSize = 100

// DefaultInterval is how often the reporter looks for new viewings. Reporting
// instructional hours is not time-critical; the LMS only ever renders them by
// day.
const DefaultInterval = 5 * time.Minute

// DayFormat is the calendar-day form the LMS ingest accepts.
const DayFormat = "2006-01-02"

// Summary counts what one reporting pass did.
type Summary struct {
	// Scanned is how many unreported viewings the pass picked up.
	Scanned int `json:"scanned"`
	// Reported is how many the LMS recorded as new instructional time.
	Reported int `json:"reported"`
	// Duplicate is how many the LMS already held under the same reference.
	Duplicate int `json:"duplicate"`
	// Failed is how many could not be reported and will be retried.
	Failed int `json:"failed"`
}

// Reporter pushes finished educational viewings to the LMS and records them in
// the local export ledger.
//
// It holds no database handle: the querier is passed per call so the same
// reporter serves the background loop (against the pool) and the admin
// "report now" action (against whatever the request is using).
type Reporter struct {
	// Ingest is the LMS ingest client.
	Ingest Ingester
	// Location is the zone a viewing's calendar day is computed in. It is the
	// channel timezone, because a Tennessee evening is one household day even
	// when the server keeps UTC.
	Location *time.Location
	// BatchSize bounds one pass; zero selects DefaultBatchSize.
	BatchSize int
	// Now overrides the clock, for tests.
	Now func() time.Time
}

// location returns the zone days are bucketed in, defaulting to UTC.
func (r *Reporter) location() *time.Location {
	if r.Location == nil {
		return time.UTC
	}
	return r.Location
}

// now returns the reporter's current time.
func (r *Reporter) now() time.Time {
	if r.Now == nil {
		return time.Now().UTC()
	}
	return r.Now().UTC()
}

// batchSize returns the configured pass size.
func (r *Reporter) batchSize() int {
	if r.BatchSize <= 0 {
		return DefaultBatchSize
	}
	return r.BatchSize
}

// RunOnce reports one batch of unreported viewings.
//
// A viewing the LMS refuses or cannot be reached for is counted as failed and
// left in place: nothing is dropped, and the next pass picks it up. Only a
// failure to read the backlog at all aborts the pass, because that means the
// database is unusable and retrying each row would just repeat the error.
func (r *Reporter) RunOnce(ctx context.Context, q repo.Querier) (Summary, error) {
	var sum Summary
	sessions, err := tvrepo.UnreportedSessions(ctx, q, r.batchSize())
	if err != nil {
		return sum, err
	}
	sum.Scanned = len(sessions)

	for _, session := range sessions {
		if ctx.Err() != nil {
			break
		}
		result, err := r.Ingest.Ingest(ctx, r.logFor(session))
		if err != nil {
			sum.Failed++
			slog.Warn("primer ingest failed; the viewing stays queued",
				"session", session.SessionID, "title", session.Title, "error", err)
			continue
		}
		// The ledger write comes second on purpose. Losing it means one extra
		// request next pass, which the LMS answers with created=false; writing
		// it first and then failing to post would lose the hours for good.
		if _, err := tvrepo.RecordPrimerReport(ctx, q, session.SessionID, result.LogID, r.now()); err != nil {
			sum.Failed++
			slog.Warn("recording the primer report failed; the viewing will be re-sent",
				"session", session.SessionID, "error", err)
			continue
		}
		if result.Created {
			sum.Reported++
		} else {
			sum.Duplicate++
		}
	}
	return sum, nil
}

// Run reports on a fixed interval until the context is cancelled. It reports
// once immediately so a restart drains whatever accumulated while the process
// was down.
func (r *Reporter) Run(ctx context.Context, q repo.Querier, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		sum, err := r.RunOnce(ctx, q)
		switch {
		case err != nil:
			slog.Warn("primer reporting pass failed", "error", err)
		case sum.Scanned > 0:
			slog.Info("reported instructional time to primer",
				"scanned", sum.Scanned, "reported", sum.Reported,
				"duplicate", sum.Duplicate, "failed", sum.Failed)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// logFor renders a viewing as the instruction log the LMS stores. The media
// item's subject tags and standard codes travel with it, which is how a
// documentary watched during force-laws week lands under the right standard.
// Both tag columns are NOT NULL arrays, so they arrive as empty slices rather
// than nil and the JSON body never carries a null where the LMS expects a list.
func (r *Reporter) logFor(session tvrepo.ReportableSession) InstructionLog {
	return InstructionLog{
		Source:         SourceTV,
		SourceRef:      session.SessionID,
		MediaTitle:     session.Title,
		Class:          session.Class,
		SubjectTags:    session.SubjectTags,
		StandardCodes:  session.StandardCodes,
		WatchedSeconds: session.WatchedSeconds,
		OccurredOn:     session.StartedAt.In(r.location()).Format(DayFormat),
	}
}
