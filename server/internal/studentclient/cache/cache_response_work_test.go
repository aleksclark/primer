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

func TestResponseDraftAndIntentLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := cache.Open(filepath.Join(t.TempDir(), "resp.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	csid := uuid.NewString()
	require.NoError(t, s.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, AssignmentID: "asg-r", State: "started",
		LastAckedSequence: -1, NextSequence: 0,
	}))

	// Missing draft returns empty string, not error.
	body, err := s.GetResponseDraft(ctx, csid, "task-a")
	require.NoError(t, err)
	assert.Equal(t, "", body)

	require.NoError(t, s.SaveResponseDraft(ctx, csid, "task-a", "first draft"))
	body, err = s.GetResponseDraft(ctx, csid, "task-a")
	require.NoError(t, err)
	assert.Equal(t, "first draft", body)

	// Upsert overwrites body for same session+task.
	require.NoError(t, s.SaveResponseDraft(ctx, csid, "task-a", "revised answer"))
	body, err = s.GetResponseDraft(ctx, csid, "task-a")
	require.NoError(t, err)
	assert.Equal(t, "revised answer", body)

	// Other task stays independent.
	require.NoError(t, s.SaveResponseDraft(ctx, csid, "task-b", "other"))
	body, err = s.GetResponseDraft(ctx, csid, "task-b")
	require.NoError(t, err)
	assert.Equal(t, "other", body)

	subA := uuid.NewString()
	subB := uuid.NewString()
	reqA := contracts.ResponseSubmission{
		SchemaVersion: "1", SubmissionID: subA, TaskID: "task-a",
		Body: "revised answer", ClientTime: time.Now().UTC(), RequestDigest: "dig-a",
	}
	reqB := contracts.ResponseSubmission{
		SchemaVersion: "1", SubmissionID: subB, TaskID: "task-b",
		Body: "other", ClientTime: time.Now().UTC(), RequestDigest: "dig-b",
	}

	// Empty server session id is allowed; later bind can stamp.
	require.NoError(t, s.SaveResponseIntent(ctx, csid, "", reqA))
	require.NoError(t, s.SaveResponseIntent(ctx, csid, "srv-1", reqB))

	// Re-save with non-empty server id upgrades the blank stamp.
	require.NoError(t, s.SaveResponseIntent(ctx, csid, "srv-1", reqA))

	pending, err := s.ListPendingResponses(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	bySub := map[string]cache.ResponseIntent{}
	for _, p := range pending {
		bySub[p.SubmissionID] = p
		assert.False(t, p.Acked)
		assert.Nil(t, p.Response)
	}
	assert.Equal(t, "srv-1", bySub[subA].ServerSessionID)
	assert.Equal(t, "task-a", bySub[subA].TaskID)
	assert.Equal(t, "revised answer", bySub[subA].Request.Body)

	// Pending tasks count as submitted for local progress.
	ackedTasks, err := s.ListAckedResponseTaskIDs(ctx, csid)
	require.NoError(t, err)
	assert.True(t, ackedTasks["task-a"])
	assert.True(t, ackedTasks["task-b"])
	assert.False(t, ackedTasks["task-missing"])

	require.NoError(t, s.MarkResponseAcked(ctx, subA, contracts.ResponseSubmissionResult{
		SchemaVersion: "1", SubmissionID: subA, ResponseID: "resp-1",
		Status: "accepted", ReviewRequired: true, EvidenceIDs: []string{"ev-1"},
	}))

	pending, err = s.ListPendingResponses(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, subB, pending[0].SubmissionID)

	// Acked task still present in progress map.
	ackedTasks, err = s.ListAckedResponseTaskIDs(ctx, csid)
	require.NoError(t, err)
	assert.True(t, ackedTasks["task-a"])
	assert.True(t, ackedTasks["task-b"])

	// Closed store returns errors without panic.
	require.NoError(t, s.Close())
	require.Error(t, s.SaveResponseDraft(ctx, csid, "task-a", "x"))
	_, err = s.GetResponseDraft(ctx, csid, "task-a")
	require.Error(t, err)
	require.Error(t, s.SaveResponseIntent(ctx, csid, "srv", reqA))
	require.Error(t, s.MarkResponseAcked(ctx, subA, contracts.ResponseSubmissionResult{SubmissionID: subA}))
	_, err = s.ListPendingResponses(ctx)
	require.Error(t, err)
	_, err = s.ListAckedResponseTaskIDs(ctx, csid)
	require.Error(t, err)
}

func TestCommitWorkSyncSnapshotCancelsMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := cache.Open(filepath.Join(t.TempDir(), "worksync.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	keep := studentapi.WorkItem{
		Assignment: domain.StudentAssignment{ID: "asg-keep", State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "keep-me"},
		Revision:   domain.LearningActivityRevision{ID: "r1", Content: map[string]any{"o": 1}},
	}
	gone := studentapi.WorkItem{
		Assignment: domain.StudentAssignment{ID: "asg-gone", State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "drop-me"},
		Revision:   domain.LearningActivityRevision{ID: "r2", Content: map[string]any{"o": 2}},
	}
	alreadyDone := studentapi.WorkItem{
		Assignment: domain.StudentAssignment{ID: "asg-done", State: "completed", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "done"},
		Revision:   domain.LearningActivityRevision{ID: "r3", Content: map[string]any{"o": 3}},
	}

	// Seed local cache with three items; snapshot will only report keep.
	require.NoError(t, s.SaveWork(ctx, []studentapi.WorkItem{keep, gone, alreadyDone}))
	require.NoError(t, s.BeginWorkPage(ctx, studentapi.WorkModeSnapshot))
	require.NoError(t, s.ApplyWorkPage(ctx, []studentapi.WorkItem{keep}, "page-1", studentapi.WorkModeSnapshot))

	st, err := s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.True(t, st.InProgress)
	assert.Equal(t, studentapi.WorkModeSnapshot, st.PageMode)
	assert.Contains(t, st.SnapshotSeenIDs, "asg-keep")

	require.NoError(t, s.CommitWorkSync(ctx, "cursor-final", studentapi.WorkModeSnapshot))

	st, err = s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.False(t, st.InProgress)
	assert.Equal(t, "cursor-final", st.Cursor)
	assert.Equal(t, studentapi.WorkModeSnapshot, st.Mode)
	assert.Empty(t, st.PageCursor)
	assert.NotEmpty(t, st.LastFullSyncAt)

	// Missing snapshot rows soft-cancel; completed rows stay completed.
	gotGone, err := s.GetWork(ctx, "asg-gone")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", gotGone.Assignment.State)

	gotKeep, err := s.GetWork(ctx, "asg-keep")
	require.NoError(t, err)
	assert.Equal(t, "available", gotKeep.Assignment.State)
	assert.Equal(t, "keep-me", gotKeep.Activity.Slug)

	gotDone, err := s.GetWork(ctx, "asg-done")
	require.NoError(t, err)
	assert.Equal(t, "completed", gotDone.Assignment.State)

	// Incremental commit advances cursor without cancelling.
	extra := studentapi.WorkItem{
		Assignment: domain.StudentAssignment{ID: "asg-inc", State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "inc"},
		Revision:   domain.LearningActivityRevision{ID: "r4", Content: map[string]any{}},
	}
	require.NoError(t, s.BeginWorkPage(ctx, studentapi.WorkModeIncremental))
	require.NoError(t, s.ApplyWorkPage(ctx, []studentapi.WorkItem{extra}, "inc-page", studentapi.WorkModeIncremental))
	require.NoError(t, s.CommitWorkSync(ctx, "cursor-inc", studentapi.WorkModeIncremental))

	st, err = s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "cursor-inc", st.Cursor)
	assert.Equal(t, studentapi.WorkModeIncremental, st.Mode)
	// Prior cancelled item remains cancelled under incremental commit.
	gotGone, err = s.GetWork(ctx, "asg-gone")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", gotGone.Assignment.State)

	require.NoError(t, s.ResetWorkSyncCursor(ctx))
	st, err = s.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "", st.Cursor)
	assert.False(t, st.InProgress)
}

func TestSavePendingArtifactServerSessionStamp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := cache.Open(filepath.Join(t.TempDir(), "art.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	csid := uuid.NewString()
	artID := uuid.NewString()
	meta := contracts.ArtifactMeta{
		SchemaVersion: "1", ArtifactID: artID, Filename: "note.txt",
		MediaType: "text/plain", ByteSize: 4, SHA256: "abcd",
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, s.SavePendingArtifact(ctx, csid, "", meta))
	// Re-save with empty server id must not wipe a later stamp — first stamp empty.
	pending, err := s.ListPendingArtifacts(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "", pending[0].ServerSessionID)

	// Non-empty server id stamps; empty re-save keeps it.
	meta.Filename = "note-v2.txt"
	require.NoError(t, s.SavePendingArtifact(ctx, csid, "srv-art", meta))
	require.NoError(t, s.SavePendingArtifact(ctx, csid, "", meta))
	pending, err = s.ListPendingArtifacts(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "srv-art", pending[0].ServerSessionID)
	assert.Equal(t, "note-v2.txt", pending[0].Meta.Filename)
	assert.False(t, pending[0].Acked)

	require.NoError(t, s.MarkArtifactAcked(ctx, artID))
	pending, err = s.ListPendingArtifacts(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
}
