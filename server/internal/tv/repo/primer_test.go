package repo_test

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// finishedViewing records a closed-out session of an item in the given class.
func finishedViewing(t *testing.T, q baserepo.Querier, class string, overrides ...factory.Override) *domain.PlaybackSession {
	t.Helper()
	item := factory.MediaItem(t, q, factory.Override{
		"class":          class,
		"subject_tags":   []string{"science"},
		"standard_codes": []string{"TN.SCI.6.PS.2"},
	})
	merged := factory.Override{
		"media_item_id":        item.ID,
		"ended_at":             time.Now().UTC(),
		"watched_seconds":      1500,
		"max_position_seconds": 1500,
		"completed":            true,
	}
	for _, o := range overrides {
		maps.Copy(merged, o)
	}
	return factory.PlaybackSession(t, q, merged)
}

// sessionIDs collects the session identifiers from a reportable batch.
func sessionIDs(sessions []tvrepo.ReportableSession) map[string]tvrepo.ReportableSession {
	out := make(map[string]tvrepo.ReportableSession, len(sessions))
	for _, s := range sessions {
		out[s.SessionID] = s
	}
	return out
}

func TestUnreportedSessionsSelectsInstructionalViewingOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	educational := finishedViewing(t, q, domain.ClassEducational)
	mixed := finishedViewing(t, q, domain.ClassMixed)
	entertainment := finishedViewing(t, q, domain.ClassEntertainment)
	unfinished := finishedViewing(t, q, domain.ClassEducational, factory.Override{"ended_at": nil})
	untouched := finishedViewing(t, q, domain.ClassEducational, factory.Override{"watched_seconds": 0})

	sessions, err := tvrepo.UnreportedSessions(ctx, q, 100)
	require.NoError(t, err)
	byID := sessionIDs(sessions)

	assert.Contains(t, byID, educational.ID)
	assert.Contains(t, byID, mixed.ID)
	assert.NotContains(t, byID, entertainment.ID,
		"pure entertainment is not instructional time and must never reach the LMS")
	assert.NotContains(t, byID, unfinished.ID, "a session still in flight is not reportable")
	assert.NotContains(t, byID, untouched.ID, "a redeemed grant with no watch time books nothing")

	entry := byID[educational.ID]
	assert.Equal(t, domain.ClassEducational, entry.Class)
	assert.Equal(t, []string{"science"}, entry.SubjectTags, "subject tags flow through from the media item")
	assert.Equal(t, []string{"TN.SCI.6.PS.2"}, entry.StandardCodes)
	assert.Equal(t, 1500, entry.WatchedSeconds)
	assert.NotEmpty(t, entry.Title)
}

func TestUnreportedSessionsExcludesAlreadyExported(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)

	reported := finishedViewing(t, q, domain.ClassEducational)
	pending := finishedViewing(t, q, domain.ClassEducational)
	factory.PrimerReport(t, q, factory.Override{"playback_session_id": reported.ID})

	sessions, err := tvrepo.UnreportedSessions(ctx, q, 100)
	require.NoError(t, err)
	byID := sessionIDs(sessions)
	assert.NotContains(t, byID, reported.ID)
	assert.Contains(t, byID, pending.ID)
}

func TestUnreportedSessionsOrdersOldestFirstAndHonoursTheLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	base := time.Now().UTC().Add(-24 * time.Hour)

	oldest := finishedViewing(t, q, domain.ClassEducational, factory.Override{"ended_at": base})
	middle := finishedViewing(t, q, domain.ClassEducational, factory.Override{"ended_at": base.Add(time.Hour)})
	newest := finishedViewing(t, q, domain.ClassEducational, factory.Override{"ended_at": base.Add(2 * time.Hour)})

	first, err := tvrepo.UnreportedSessions(ctx, q, 2)
	require.NoError(t, err)
	require.Len(t, first, 2, "the batch size bounds one pass")
	assert.Equal(t, oldest.ID, first[0].SessionID)
	assert.Equal(t, middle.ID, first[1].SessionID)

	all, err := tvrepo.UnreportedSessions(ctx, q, 100)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, newest.ID, all[2].SessionID)
}

func TestRecordPrimerReportIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := querier(t)
	now := time.Now().UTC()

	session := finishedViewing(t, q, domain.ClassEducational)

	created, err := tvrepo.RecordPrimerReport(ctx, q, session.ID, "log-1", now)
	require.NoError(t, err)
	assert.True(t, created)

	again, err := tvrepo.RecordPrimerReport(ctx, q, session.ID, "log-2", now.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, again, "a retry does not create a second ledger row")

	page, err := tvrepo.PrimerReports.List(ctx, q, baserepo.ListParams{
		Filters: map[string]any{"playback_session_id": session.ID},
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "log-1", page.Items[0].PrimerRef, "the first successful export wins")
}

func TestRecordPrimerReportRejectsAnUnknownSession(t *testing.T) {
	t.Parallel()
	q := querier(t)

	_, err := tvrepo.RecordPrimerReport(context.Background(), q,
		"00000000-0000-0000-0000-000000000000", "log-1", time.Now().UTC())
	assert.Error(t, err, "the ledger cannot point at a session that does not exist")
}
