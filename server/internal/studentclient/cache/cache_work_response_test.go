package cache_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestWorkSyncStateAndResponseIntents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := cache.Open(filepath.Join(t.TempDir(), "ws.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	// Empty defaults.
	st, err := s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.Empty(t, st.Cursor)
	assert.False(t, st.InProgress)
	assert.Nil(t, st.SnapshotSeenIDs)

	require.NoError(t, s.BeginWorkPage(ctx, studentapi.WorkModeSnapshot))
	st, err = s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.True(t, st.InProgress)
	assert.Equal(t, studentapi.WorkModeSnapshot, st.PageMode)

	// Bad JSON in seen IDs.
	require.NoError(t, s.SetMeta(ctx, cache.MetaWorkPageSeenIDs, "{not-json"))
	_, err = s.GetWorkSyncState(ctx)
	require.Error(t, err)

	require.NoError(t, s.SetMeta(ctx, cache.MetaWorkPageSeenIDs, `["a1","a2"]`))
	require.NoError(t, s.SetMeta(ctx, cache.MetaWorkCursor, "cur-1"))
	require.NoError(t, s.SetMeta(ctx, cache.MetaWorkSyncMode, studentapi.WorkModeIncremental))
	require.NoError(t, s.SetMeta(ctx, cache.MetaWorkLastFullSync, time.Now().UTC().Format(time.RFC3339Nano)))
	require.NoError(t, s.SetMeta(ctx, cache.MetaWorkPageCursor, "page-2"))
	require.NoError(t, s.SetMeta(ctx, cache.MetaWorkInProgress, "1"))
	st, err = s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "cur-1", st.Cursor)
	assert.Equal(t, []string{"a1", "a2"}, st.SnapshotSeenIDs)
	assert.True(t, st.InProgress)

	// Response intents.
	csid := uuid.NewString()
	require.NoError(t, s.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv", AssignmentID: "a",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	sub := uuid.NewString()
	require.NoError(t, s.SaveResponseIntent(ctx, csid, "srv", contracts.ResponseSubmission{
		SchemaVersion: contracts.ResponseSchemaVersion,
		SubmissionID:  sub,
		TaskID:        "t1",
		Body:          "answer",
		ClientTime:    time.Now().UTC(),
	}))
	pending, err := s.ListPendingResponses(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, sub, pending[0].SubmissionID)

	require.NoError(t, s.MarkResponseAcked(ctx, sub, contracts.ResponseSubmissionResult{
		SchemaVersion: contracts.ResponseSchemaVersion,
		SubmissionID:  sub,
		ResponseID:    uuid.NewString(),
		Status:        "submitted",
	}))
	pending, err = s.ListPendingResponses(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
}
