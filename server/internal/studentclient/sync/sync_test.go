package sync_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
)

func TestSyncOncePullAndFlush(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var gotEvents int
	var gotComplete bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{
			Items: []studentapi.WorkItem{{
				Assignment: domain.StudentAssignment{ID: "asg-1", State: "available", UpdatedAt: time.Now().UTC()},
				Activity:   domain.LearningActivity{Slug: "basic-navigation"},
				Revision:   domain.LearningActivityRevision{ID: "rev-1", Content: map[string]any{}},
			}},
		})
	})
	mux.HandleFunc("POST /student/sessions/srv-1/events", func(w http.ResponseWriter, r *http.Request) {
		var body studentapi.EventsRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		gotEvents += len(body.Events)
		var maxSeq int64 = -1
		for _, e := range body.Events {
			if e.Sequence > maxSeq {
				maxSeq = e.Sequence
			}
		}
		_ = json.NewEncoder(w).Encode(studentapi.EventsAck{AcknowledgedSequence: maxSeq})
	})
	mux.HandleFunc("POST /student/sessions/srv-1/complete", func(w http.ResponseWriter, r *http.Request) {
		gotComplete = true
		var req contracts.CompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(contracts.CompletionResult{
			SchemaVersion: contracts.CompletionSchemaVersion,
			CompletionID:  req.CompletionID,
			Accepted:      true,
			RequestDigest: req.RequestDigest,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "s.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))

	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv-1", AssignmentID: "asg-1",
		LastAckedSequence: -1, NextSequence: 0, State: "started",
	}))
	_, err = store.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: -1,
	})
	require.NoError(t, err)
	cmpID := uuid.NewString()
	require.NoError(t, store.SaveCompletionIntent(ctx, csid, "srv-1", contracts.CompletionRequest{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  cmpID,
		RequestDigest: "d",
		ClientTime:    time.Now().UTC(),
	}))

	cl := studentapi.New(srv.URL, "tok")
	loop := sync.New(cl, store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.Equal(t, 1, res.WorkItems)
	assert.Equal(t, 1, res.EventsFlushed)
	assert.Equal(t, 1, res.CompletionsSent)
	assert.True(t, gotComplete)
	assert.Equal(t, 1, gotEvents)
	assert.Equal(t, sync.StatusOnline, res.Status)

	pending, err := store.GetPendingSync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, pending.EventCount)
	assert.Equal(t, 0, pending.CompleteCount)
}

func TestSyncOnceRevoked(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "revoked", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	store, err := cache.Open(filepath.Join(t.TempDir(), "r.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(context.Background(), "dead"))
	loop := sync.New(studentapi.New(srv.URL, "dead"), store)
	res := loop.SyncOnce(context.Background())
	require.Error(t, res.Err)
	assert.Equal(t, sync.StatusRevoked, res.Status)
	var uerr *studentapi.ErrUnauthorized
	require.ErrorAs(t, res.Err, &uerr)
}
