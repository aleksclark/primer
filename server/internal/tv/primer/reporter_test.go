package primer_test

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	baserepo "github.com/aleksclark/primer/server/internal/repo"
	basetestutil "github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	"github.com/aleksclark/primer/server/internal/tv/primer"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// chicago is the household zone the reporter buckets viewing days in.
var chicago = mustLoad("America/Chicago")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// querier returns a savepoint-wrapped TV transaction for a test.
func querier(t *testing.T) baserepo.Querier {
	t.Helper()
	return basetestutil.NewSavepointQuerier(tvtestutil.Tx(t))
}

// viewing records a closed-out session of an item in the given class.
func viewing(t *testing.T, q baserepo.Querier, class string, overrides ...factory.Override) *domain.PlaybackSession {
	t.Helper()
	item := factory.MediaItem(t, q, factory.Override{
		"class":          class,
		"title":          "Bill Nye: Inertia",
		"subject_tags":   []string{"science", "physics"},
		"standard_codes": []string{"TN.SCI.6.PS.2"},
	})
	merged := factory.Override{
		"media_item_id":        item.ID,
		"started_at":           time.Date(2031, 4, 16, 1, 30, 0, 0, time.UTC),
		"ended_at":             time.Date(2031, 4, 16, 2, 0, 0, 0, time.UTC),
		"watched_seconds":      1500,
		"max_position_seconds": 1500,
		"completed":            true,
	}
	for _, o := range overrides {
		maps.Copy(merged, o)
	}
	return factory.PlaybackSession(t, q, merged)
}

// ledger lists the export ledger rows for a session.
func ledger(t *testing.T, q baserepo.Querier, sessionID string) []domain.PrimerReport {
	t.Helper()
	page, err := tvrepo.PrimerReports.List(context.Background(), q, baserepo.ListParams{
		Filters: map[string]any{"playback_session_id": sessionID},
	})
	require.NoError(t, err)
	return page.Items
}

func TestReporterExportsInstructionalViewing(t *testing.T) {
	t.Parallel()
	q := querier(t)
	fake := primer.NewFake()
	reporter := &primer.Reporter{Ingest: fake, Location: chicago}

	session := viewing(t, q, domain.ClassEducational)

	summary, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, primer.Summary{Scanned: 1, Reported: 1}, summary)

	logs := fake.Accepted()
	require.Len(t, logs, 1)
	assert.Equal(t, primer.SourceTV, logs[0].Source)
	assert.Equal(t, session.ID, logs[0].SourceRef, "the session ID is the idempotency key")
	assert.Equal(t, "Bill Nye: Inertia", logs[0].MediaTitle)
	assert.Equal(t, domain.ClassEducational, logs[0].Class)
	assert.Equal(t, []string{"science", "physics"}, logs[0].SubjectTags)
	assert.Equal(t, []string{"TN.SCI.6.PS.2"}, logs[0].StandardCodes)
	assert.Equal(t, 1500, logs[0].WatchedSeconds)
	assert.Equal(t, "2031-04-15", logs[0].OccurredOn,
		"a viewing at 8:30pm Central is that evening's, not the next UTC day's")

	rows := ledger(t, q, session.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, "log-1", rows[0].PrimerRef, "the LMS's ID is kept so the export can be traced")
}

func TestReporterLeavesEntertainmentAlone(t *testing.T) {
	t.Parallel()
	q := querier(t)
	fake := primer.NewFake()
	reporter := &primer.Reporter{Ingest: fake, Location: chicago}

	session := viewing(t, q, domain.ClassEntertainment)

	summary, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	assert.Zero(t, summary.Scanned, "entertainment never becomes a candidate")
	assert.Empty(t, fake.Accepted())
	assert.Empty(t, ledger(t, q, session.ID),
		"an unreportable session is not marked reported either, it is simply not eligible")
}

func TestReporterDoesNotDoubleCount(t *testing.T) {
	t.Parallel()
	q := querier(t)
	fake := primer.NewFake()
	reporter := &primer.Reporter{Ingest: fake, Location: chicago}

	session := viewing(t, q, domain.ClassMixed)

	first, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, 1, first.Reported)

	second, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, primer.Summary{}, second, "a second pass finds nothing left to report")
	assert.Equal(t, 1, fake.Calls, "the LMS is not asked twice about the same viewing")
	assert.Len(t, ledger(t, q, session.ID), 1)
}

func TestReporterRetriesAfterTheLMSIsUnreachable(t *testing.T) {
	t.Parallel()
	q := querier(t)
	fake := primer.NewFake()
	fake.Err = errors.New("connection refused")
	reporter := &primer.Reporter{Ingest: fake, Location: chicago}

	session := viewing(t, q, domain.ClassEducational)

	down, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err, "an LMS that is down must not wedge the reporting loop")
	assert.Equal(t, primer.Summary{Scanned: 1, Failed: 1}, down)
	assert.Empty(t, ledger(t, q, session.ID), "nothing is marked reported that was not")

	fake.Err = nil
	recovered, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, primer.Summary{Scanned: 1, Reported: 1}, recovered,
		"the viewing survived the outage and is exported once the LMS returns")
	assert.Len(t, ledger(t, q, session.ID), 1)
}

func TestReporterCountsALMSReplayAsADuplicate(t *testing.T) {
	t.Parallel()
	q := querier(t)
	fake := primer.NewFake()
	reporter := &primer.Reporter{Ingest: fake, Location: chicago}

	session := viewing(t, q, domain.ClassEducational)

	// Stand in for a crash between the successful post and the ledger write:
	// the LMS already holds the log, we do not know it yet.
	_, err := fake.Ingest(context.Background(), primer.InstructionLog{
		Source: primer.SourceTV, SourceRef: session.ID,
	})
	require.NoError(t, err)

	summary, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, primer.Summary{Scanned: 1, Duplicate: 1}, summary,
		"the re-send is recognised as already counted, not booked a second time")
	assert.Len(t, ledger(t, q, session.ID), 1)
}

func TestReporterHonoursItsBatchSize(t *testing.T) {
	t.Parallel()
	q := querier(t)
	fake := primer.NewFake()
	reporter := &primer.Reporter{Ingest: fake, Location: chicago, BatchSize: 2}

	base := time.Date(2031, 4, 16, 2, 0, 0, 0, time.UTC)
	for i := range 5 {
		viewing(t, q, domain.ClassEducational, factory.Override{
			"ended_at": base.Add(time.Duration(i) * time.Minute),
		})
	}

	first, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, 2, first.Reported)

	second, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, 2, second.Reported)

	third, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, 1, third.Reported, "the backlog drains over successive passes")
}

// cancellingIngester cancels the pass while answering its first call, which is
// what a shutdown arriving mid-batch looks like.
type cancellingIngester struct {
	cancel context.CancelFunc
	calls  int
}

func (c *cancellingIngester) Ingest(context.Context, primer.InstructionLog) (*primer.IngestResult, error) {
	c.calls++
	c.cancel()
	return &primer.IngestResult{LogID: "log-1", Created: true}, nil
}

func TestReporterStopsOnCancellation(t *testing.T) {
	t.Parallel()
	q := querier(t)

	base := time.Date(2031, 4, 16, 2, 0, 0, 0, time.UTC)
	viewing(t, q, domain.ClassEducational, factory.Override{"ended_at": base})
	viewing(t, q, domain.ClassEducational, factory.Override{"ended_at": base.Add(time.Minute)})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ingester := &cancellingIngester{cancel: cancel}
	reporter := &primer.Reporter{Ingest: ingester, Location: chicago}

	summary, err := reporter.RunOnce(ctx, q)
	require.NoError(t, err)
	assert.Equal(t, 2, summary.Scanned)
	assert.Equal(t, 1, ingester.calls,
		"shutdown abandons the rest of the batch rather than pushing through it")
	assert.Equal(t, 1, summary.Failed,
		"the viewing whose ledger write was cut short stays queued for the next pass")
}

func TestReporterSurfacesAnUnreadableBacklog(t *testing.T) {
	t.Parallel()
	reporter := &primer.Reporter{Ingest: primer.NewFake()}

	// A closed transaction stands in for an unusable database: the pass has to
	// report the failure rather than silently claim it found nothing.
	tx := tvtestutil.Tx(t)
	require.NoError(t, tx.Rollback(context.Background()))

	_, err := reporter.RunOnce(context.Background(), tx)
	assert.Error(t, err)
}

func TestReporterDefaultsToUTCDaysAndTheStandardBatch(t *testing.T) {
	t.Parallel()
	q := querier(t)
	fake := primer.NewFake()
	reporter := &primer.Reporter{Ingest: fake}

	viewing(t, q, domain.ClassEducational)

	_, err := reporter.RunOnce(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, fake.Accepted(), 1)
	assert.Equal(t, "2031-04-16", fake.Accepted()[0].OccurredOn,
		"without a configured zone the day is the UTC one")
}

func TestReporterRunLoopReportsAndStops(t *testing.T) {
	t.Parallel()
	q := querier(t)
	fake := primer.NewFake()
	reporter := &primer.Reporter{Ingest: fake, Location: chicago, Now: func() time.Time { return time.Now().UTC() }}

	viewing(t, q, domain.ClassEducational)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		reporter.Run(ctx, q, 10*time.Millisecond)
	}()

	require.Eventually(t, func() bool { return len(fake.Accepted()) == 1 }, 5*time.Second, 5*time.Millisecond,
		"the loop reports immediately rather than waiting out its first interval")

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the reporting loop did not stop when its context was cancelled")
	}
}

func TestReporterRunLoopSurvivesAFailingPass(t *testing.T) {
	t.Parallel()
	reporter := &primer.Reporter{Ingest: primer.NewFake(), Location: chicago}

	// An unusable database makes every pass fail. The loop must keep its shape
	// and still stop cleanly, rather than exiting and taking reporting down
	// until the next restart.
	tx := tvtestutil.Tx(t)
	require.NoError(t, tx.Rollback(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		reporter.Run(ctx, tx, time.Millisecond)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the reporting loop did not stop after repeated failures")
	}
}
