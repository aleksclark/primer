package cache_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func workItem(id, state string) studentapi.WorkItem {
	return studentapi.WorkItem{
		Assignment: domain.StudentAssignment{ID: id, State: state, UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "s-" + id},
		Revision:   domain.LearningActivityRevision{ID: "r-" + id, Content: map[string]any{"objective": "o"}},
	}
}

func TestCacheWorkSyncMultiPageAndCorruptSeen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := cache.Open(filepath.Join(t.TempDir(), "ws.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	// Seed two available assignments, then snapshot that only keeps one.
	require.NoError(t, s.SaveWork(ctx, []studentapi.WorkItem{
		workItem("keep-me", "available"),
		workItem("drop-me", "available"),
		workItem("done-me", "completed"),
		workItem("cancel-me", "cancelled"),
	}))

	require.NoError(t, s.BeginWorkPage(ctx, studentapi.WorkModeSnapshot))
	st, err := s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.True(t, st.InProgress)
	assert.Equal(t, studentapi.WorkModeSnapshot, st.PageMode)
	// Fresh snapshot resets seen IDs.
	assert.Empty(t, st.SnapshotSeenIDs)

	require.NoError(t, s.ApplyWorkPage(ctx, []studentapi.WorkItem{workItem("keep-me", "available")}, "cur-1", studentapi.WorkModeSnapshot))
	// Second page continues with page cursor already set — Begin again should NOT wipe seen.
	require.NoError(t, s.BeginWorkPage(ctx, studentapi.WorkModeSnapshot))
	st, err = s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "cur-1", st.PageCursor)
	require.Contains(t, st.SnapshotSeenIDs, "keep-me")

	// Incremental page does not accumulate snapshot seen IDs.
	require.NoError(t, s.ApplyWorkPage(ctx, []studentapi.WorkItem{workItem("inc-1", "available")}, "inc-cur", studentapi.WorkModeIncremental))

	// Snapshot commit soft-cancels missing non-terminal rows.
	require.NoError(t, s.BeginWorkPage(ctx, studentapi.WorkModeSnapshot))
	require.NoError(t, s.ApplyWorkPage(ctx, []studentapi.WorkItem{workItem("keep-me", "available")}, "final", studentapi.WorkModeSnapshot))
	require.NoError(t, s.CommitWorkSync(ctx, "cursor-final", studentapi.WorkModeSnapshot))

	st, err = s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.False(t, st.InProgress)
	assert.Equal(t, "cursor-final", st.Cursor)
	assert.Equal(t, studentapi.WorkModeSnapshot, st.Mode)
	assert.NotEmpty(t, st.LastFullSyncAt)
	assert.Empty(t, st.PageCursor)
	assert.Empty(t, st.SnapshotSeenIDs)

	drop, err := s.GetWork(ctx, "drop-me")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", drop.Assignment.State)

	done, err := s.GetWork(ctx, "done-me")
	require.NoError(t, err)
	assert.Equal(t, "completed", done.Assignment.State)

	// Corrupt seen JSON is tolerated on GetWorkSyncState? decode error surfaces.
	require.NoError(t, s.SetMeta(ctx, cache.MetaWorkPageSeenIDs, "{not-json"))
	_, err = s.GetWorkSyncState(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")

	// Reset clears durable cursor.
	require.NoError(t, s.SetMeta(ctx, cache.MetaWorkPageSeenIDs, "[]"))
	require.NoError(t, s.ResetWorkSyncCursor(ctx))
	st, err = s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.Empty(t, st.Cursor)
	assert.False(t, st.InProgress)
}

func TestCacheClosedWorkSyncAndResponseDrafts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := cache.Open(filepath.Join(t.TempDir(), "closed2.db"))
	require.NoError(t, err)
	require.NoError(t, s.Close())

	_, err = s.GetWorkSyncState(ctx)
	require.Error(t, err)
	require.Error(t, s.BeginWorkPage(ctx, studentapi.WorkModeSnapshot))
	require.Error(t, s.ApplyWorkPage(ctx, nil, "", studentapi.WorkModeSnapshot))
	require.Error(t, s.CommitWorkSync(ctx, "c", studentapi.WorkModeSnapshot))
	require.Error(t, s.ResetWorkSyncCursor(ctx))
	require.Error(t, s.SaveResponseDraft(ctx, "c", "t", "b"))
	_, err = s.GetResponseDraft(ctx, "c", "t")
	require.Error(t, err)
	require.Error(t, s.SaveResponseIntent(ctx, "c", "s", contracts.ResponseSubmission{
		SchemaVersion: "1", SubmissionID: uuid.NewString(), TaskID: "t", Body: "x",
		ClientTime: time.Now().UTC(), RequestDigest: "d",
	}))
	require.Error(t, s.MarkResponseAcked(ctx, "x", contracts.ResponseSubmissionResult{SubmissionID: "x"}))
	_, err = s.ListPendingResponses(ctx)
	require.Error(t, err)
	_, err = s.ListAckedResponseTaskIDs(ctx, "c")
	require.Error(t, err)
}

func TestCacheEnqueueExplicitSequenceAndIdentity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := cache.Open(filepath.Join(t.TempDir(), "enq.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	require.NoError(t, s.SetDeviceIdentity(ctx, "dev-1", "stu-1", "laptop"))
	assert.Equal(t, "dev-1", mustMeta(t, s, "device_id"))
	assert.Equal(t, "stu-1", mustMeta(t, s, "student_id"))
	assert.Equal(t, "laptop", mustMeta(t, s, "device_name"))

	csid := uuid.NewString()
	require.NoError(t, s.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, AssignmentID: "a", State: "started",
		LastAckedSequence: -1, NextSequence: 5,
	}))

	// Explicit sequence >= next advances past it.
	ev, err := s.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventTaskViewed, Sequence: 10,
		Payload: map[string]any{"taskId": "t"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(10), ev.Sequence)

	sess, err := s.GetSession(ctx, csid)
	require.NoError(t, err)
	assert.Equal(t, int64(11), sess.NextSequence)

	// Nil payload becomes {}.
	ev2, err := s.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: -1, Payload: nil,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(11), ev2.Sequence)
	assert.Equal(t, contracts.EventSchemaVersion, ev2.SchemaVersion)

	// Unmarshalable payload errors.
	_, err = s.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: "x", Sequence: -1,
		Payload: map[string]any{"ch": make(chan int)},
	})
	require.Error(t, err)

	// ListWork with UpdatedAt zero uses now path via ApplyWorkPage already covered;
	// GetWork missing remains ErrNotFound-ish.
	_, err = s.GetWork(ctx, "nope")
	require.Error(t, err)
}

func mustMeta(t *testing.T, s *cache.Store, key string) string {
	t.Helper()
	v, err := s.GetMeta(context.Background(), key)
	require.NoError(t, err)
	return v
}
