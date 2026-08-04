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

func TestCacheRestartSafe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")

	s, err := cache.Open(path)
	require.NoError(t, err)

	require.NoError(t, s.SetDeviceToken(ctx, "tok-abc"))
	require.NoError(t, s.SetDeviceIdentity(ctx, "dev-1", "stu-1", "ws"))

	items := []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{
			ID: "asg-1", State: "available", UpdatedAt: time.Now().UTC(),
		},
		Activity: domain.LearningActivity{ID: "act-1", Slug: "basic-navigation", Title: "Basic Navigation"},
		Revision: domain.LearningActivityRevision{
			ID: "rev-1", ContentSHA256: "deadbeef",
			Content: map[string]any{"objective": "nav"},
		},
	}}
	require.NoError(t, s.SaveWork(ctx, items))

	clientSID := uuid.NewString()
	require.NoError(t, s.SaveSession(ctx, cache.Session{
		ClientSessionID:    clientSID,
		AssignmentID:       "asg-1",
		ActivityRevisionID: "rev-1",
		State:              "started",
		NextSequence:       0,
		LastAckedSequence:  -1,
	}))

	evID := uuid.NewString()
	ev, err := s.EnqueueEvent(ctx, clientSID, contracts.SessionEvent{
		EventID:  evID,
		Type:     contracts.EventSessionStarted,
		Sequence: -1,
		Payload:  map[string]any{"k": "v"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), ev.Sequence)

	ev2, err := s.EnqueueEvent(ctx, clientSID, contracts.SessionEvent{
		EventID:  uuid.NewString(),
		Type:     contracts.EventTaskViewed,
		Sequence: -1,
		Payload:  map[string]any{"taskId": "orient"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), ev2.Sequence)

	require.NoError(t, s.BindServerSession(ctx, clientSID, "srv-sess-1"))

	req := contracts.CompletionRequest{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  uuid.NewString(),
		RequestDigest: "digest-1",
		ClientTime:    time.Now().UTC(),
		Observations: []contracts.Observation{{
			SchemaVersion: contracts.ObservationSchemaVersion,
			CheckID:       "welcome-exists",
			Kind:          contracts.CheckFileExists,
			Passed:        true,
			ObservedAt:    time.Now().UTC(),
		}},
	}
	require.NoError(t, s.SaveCompletionIntent(ctx, clientSID, "srv-sess-1", req))

	pending, err := s.GetPendingSync(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, pending.EventCount)
	require.Equal(t, 1, pending.CompleteCount)
	// Server session id should have been stamped on outbox.
	assert.Equal(t, "srv-sess-1", pending.Events[0].ServerSessionID)

	require.NoError(t, s.Close())

	// Reopen and verify durability.
	s2, err := cache.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s2.Close() })

	tok, err := s2.DeviceToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "tok-abc", tok)

	work, err := s2.ListWork(ctx)
	require.NoError(t, err)
	require.Len(t, work, 1)
	assert.Equal(t, "basic-navigation", work[0].Activity.Slug)

	sess, err := s2.GetSession(ctx, clientSID)
	require.NoError(t, err)
	assert.Equal(t, "srv-sess-1", sess.ServerSessionID)
	assert.Equal(t, int64(2), sess.NextSequence)

	pending2, err := s2.GetPendingSync(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, pending2.EventCount)
	require.Equal(t, 1, pending2.CompleteCount)

	require.NoError(t, s2.AckEvents(ctx, clientSID, 1))
	pending3, err := s2.GetPendingSync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, pending3.EventCount)

	sess2, err := s2.GetSession(ctx, clientSID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), sess2.LastAckedSequence)

	res := contracts.CompletionResult{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  req.CompletionID,
		Accepted:      true,
		RequestDigest: req.RequestDigest,
	}
	require.NoError(t, s2.MarkCompletionAcked(ctx, req.CompletionID, res))
	pending4, err := s2.GetPendingSync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, pending4.CompleteCount)

	got, err := s2.GetCompletion(ctx, req.CompletionID)
	require.NoError(t, err)
	require.True(t, got.Acked)
	require.NotNil(t, got.Response)
	assert.True(t, got.Response.Accepted)
}

func TestEnqueueWithoutSessionFails(t *testing.T) {
	t.Parallel()
	s, err := cache.Open(filepath.Join(t.TempDir(), "x.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	_, err = s.EnqueueEvent(context.Background(), "missing", contracts.SessionEvent{
		EventID: uuid.NewString(), Type: "x", Sequence: -1,
	})
	require.ErrorIs(t, err, cache.ErrNotFound)
}
