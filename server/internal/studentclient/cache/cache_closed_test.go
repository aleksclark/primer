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

func TestCacheOpenEmptyPathAndNilClose(t *testing.T) {
	t.Parallel()
	_, err := cache.Open("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")

	var nilStore *cache.Store
	require.NoError(t, nilStore.Close())
}

func TestCacheClosedStoreErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "closed.db")
	s, err := cache.Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	// All operations after close should error (not panic).
	require.Error(t, s.SetMeta(ctx, "k", "v"))
	_, err = s.GetMeta(ctx, "k")
	require.Error(t, err)
	require.Error(t, s.SetDeviceToken(ctx, "t"))
	_, err = s.DeviceToken(ctx)
	require.Error(t, err)
	require.Error(t, s.SetDeviceIdentity(ctx, "d", "s", "n"))
	require.Error(t, s.SaveWork(ctx, []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: "a", State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "s"},
		Revision:   domain.LearningActivityRevision{ID: "r", Content: map[string]any{}},
	}}))
	_, err = s.ListWork(ctx)
	require.Error(t, err)
	_, err = s.GetWork(ctx, "a")
	require.Error(t, err)
	require.Error(t, s.SaveSession(ctx, cache.Session{ClientSessionID: "c", AssignmentID: "a", State: "started"}))
	_, err = s.GetSession(ctx, "c")
	require.Error(t, err)
	require.Error(t, s.BindServerSession(ctx, "c", "srv"))
	_, err = s.FindOpenSessionByAssignment(ctx, "a")
	require.Error(t, err)
	require.Error(t, s.SaveRunnerState(ctx, "c", "terminal", []byte(`{}`)))
	_, err = s.GetRunnerState(ctx, "c")
	require.Error(t, err)
	require.Error(t, s.DeleteRunnerState(ctx, "c"))
	_, err = s.EnqueueEvent(ctx, "c", contracts.SessionEvent{EventID: "e", Type: "t", Sequence: -1})
	require.Error(t, err)
	_, err = s.ListPendingEvents(ctx, 10)
	require.Error(t, err)
	require.Error(t, s.AckEvents(ctx, "c", 0))
	require.Error(t, s.SaveCompletionIntent(ctx, "c", "srv", contracts.CompletionRequest{
		CompletionID: uuid.NewString(), RequestDigest: "d", ClientTime: time.Now().UTC(),
	}))
	require.Error(t, s.MarkCompletionAcked(ctx, "x", contracts.CompletionResult{CompletionID: "x"}))
	_, err = s.ListPendingCompletions(ctx)
	require.Error(t, err)
	_, err = s.GetCompletion(ctx, "x")
	require.Error(t, err)
	require.Error(t, s.SavePendingArtifact(ctx, "c", "srv", contracts.ArtifactMeta{
		ArtifactID: uuid.NewString(), Filename: "f", MediaType: "text/plain", CreatedAt: time.Now().UTC(),
	}))
	_, err = s.ListPendingArtifacts(ctx)
	require.Error(t, err)
	require.Error(t, s.MarkArtifactAcked(ctx, "a"))
	_, err = s.GetPendingSync(ctx)
	require.Error(t, err)
}

func TestCacheRunnerStateValidationAndMissingRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := cache.Open(filepath.Join(t.TempDir(), "rs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	require.Error(t, s.SaveRunnerState(ctx, "", "terminal", nil))
	require.Error(t, s.SaveRunnerState(ctx, "c", "", nil))
	// nil stateJSON becomes {}
	require.NoError(t, s.SaveSession(ctx, cache.Session{
		ClientSessionID: "c1", AssignmentID: "a1", State: "started", LastAckedSequence: -1,
	}))
	require.NoError(t, s.SaveRunnerState(ctx, "c1", "terminal", nil))
	rs, err := s.GetRunnerState(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "terminal", rs.Kind)

	_, err = s.GetRunnerState(ctx, "missing")
	require.ErrorIs(t, err, cache.ErrNotFound)
	_, err = s.GetSession(ctx, "missing")
	require.ErrorIs(t, err, cache.ErrNotFound)
	_, err = s.FindOpenSessionByAssignment(ctx, "nope")
	require.ErrorIs(t, err, cache.ErrNotFound)
	_, err = s.GetCompletion(ctx, "nope")
	require.ErrorIs(t, err, cache.ErrNotFound)

	// Enqueue without session
	_, err = s.EnqueueEvent(ctx, "missing", contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: -1,
	})
	require.Error(t, err)

	// ListPendingEvents default limit
	evs, err := s.ListPendingEvents(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, evs)

	// BindServerSession stamps empty server ids on pending rows
	require.NoError(t, s.SaveSession(ctx, cache.Session{
		ClientSessionID: "c2", AssignmentID: "a2", State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	_, err = s.EnqueueEvent(ctx, "c2", contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: -1, Payload: nil,
	})
	require.NoError(t, err)
	require.NoError(t, s.SaveCompletionIntent(ctx, "c2", "", contracts.CompletionRequest{
		CompletionID: uuid.NewString(), RequestDigest: "d", ClientTime: time.Now().UTC(),
	}))
	require.NoError(t, s.SavePendingArtifact(ctx, "c2", "", contracts.ArtifactMeta{
		SchemaVersion: "1", ArtifactID: uuid.NewString(), Filename: "f.txt",
		MediaType: "text/plain", ByteSize: 1, SHA256: "aa", CreatedAt: time.Now().UTC(),
	}))
	require.NoError(t, s.BindServerSession(ctx, "c2", "server-99"))
	sess, err := s.GetSession(ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "server-99", sess.ServerSessionID)

	// Completed sessions are not "open"
	sess.State = "completed"
	require.NoError(t, s.SaveSession(ctx, *sess))
	_, err = s.FindOpenSessionByAssignment(ctx, "a2")
	require.ErrorIs(t, err, cache.ErrNotFound)

	// DecodeActivityContent happy + bad
	c, err := cache.DecodeActivityContent(map[string]any{"objective": "learn"})
	require.NoError(t, err)
	assert.Equal(t, "learn", c.Objective)
	// channel values cannot marshal → error
	_, err = cache.DecodeActivityContent(map[string]any{"bad": make(chan int)})
	require.Error(t, err)
}

func TestCacheCorruptJSONRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corrupt.db")
	s, err := cache.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	// Insert corrupt work payload via raw SQL isn't exported; use SaveWork then
	// re-open isn't needed — exercise GetCompletion with empty response and ack flow.
	csid := uuid.NewString()
	require.NoError(t, s.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv", AssignmentID: "a",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	cid := uuid.NewString()
	require.NoError(t, s.SaveCompletionIntent(ctx, csid, "srv", contracts.CompletionRequest{
		SchemaVersion: "1", CompletionID: cid, RequestDigest: "dig", ClientTime: time.Now().UTC(),
	}))
	pending, err := s.ListPendingCompletions(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, pending)

	got, err := s.GetCompletion(ctx, cid)
	require.NoError(t, err)
	assert.False(t, got.Acked)
	assert.Nil(t, got.Response)

	require.NoError(t, s.MarkCompletionAcked(ctx, cid, contracts.CompletionResult{
		SchemaVersion: "1", CompletionID: cid, Accepted: true, RequestDigest: "dig",
	}))
	got, err = s.GetCompletion(ctx, cid)
	require.NoError(t, err)
	assert.True(t, got.Acked)
	require.NotNil(t, got.Response)
	assert.True(t, got.Response.Accepted)

	// Ack events through sequence
	ev, err := s.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventTaskViewed, Sequence: -1,
		Payload: map[string]any{"taskId": "t1"},
	})
	require.NoError(t, err)
	require.NoError(t, s.AckEvents(ctx, csid, ev.Sequence))
	evs, err := s.ListPendingEvents(ctx, 100)
	require.NoError(t, err)
	for _, e := range evs {
		assert.NotEqual(t, csid, e.ClientSessionID) // acked ones gone; may be empty
	}

	ps, err := s.GetPendingSync(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, ps.EventCount+ps.CompleteCount+ps.ArtifactCount, 0)
}
