package sync_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
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

func TestSyncOnceSkipsUnboundServerSessions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{Items: nil})
	})
	// Should never be hit for unbound sessions.
	mux.HandleFunc("POST /student/sessions/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected call %s", r.URL.Path)
		http.Error(w, "no", 500)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "unbound.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))

	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "", AssignmentID: "a",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	_, err = store.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: -1,
	})
	require.NoError(t, err)
	require.NoError(t, store.SaveCompletionIntent(ctx, csid, "", contracts.CompletionRequest{
		CompletionID: uuid.NewString(), RequestDigest: "d", ClientTime: time.Now().UTC(),
	}))
	require.NoError(t, store.SavePendingArtifact(ctx, csid, "", contracts.ArtifactMeta{
		SchemaVersion: "1", ArtifactID: uuid.NewString(), Filename: "f",
		MediaType: "text/plain", ByteSize: 1, SHA256: "x", CreatedAt: time.Now().UTC(),
	}))

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	// Pending remains because unbound rows are skipped, not flushed.
	assert.Equal(t, sync.StatusAwaiting, res.Status)
	assert.GreaterOrEqual(t, res.PendingEvents+res.PendingCompletes, 1)
}

func TestSyncOnceFlushesEventsCompletionsAndBindsServerID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var eventsOK, completeOK atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{Items: []studentapi.WorkItem{{
			Assignment: domain.StudentAssignment{ID: "asg", State: "available", UpdatedAt: time.Now().UTC()},
			Activity:   domain.LearningActivity{Slug: "s"},
			Revision:   domain.LearningActivityRevision{ID: "r", Content: map[string]any{}},
		}}})
	})
	mux.HandleFunc("POST /student/sessions/srv-flush/events", func(w http.ResponseWriter, r *http.Request) {
		eventsOK.Store(true)
		_ = json.NewEncoder(w).Encode(map[string]any{"acknowledgedSequence": 0})
	})
	mux.HandleFunc("POST /student/sessions/srv-flush/complete", func(w http.ResponseWriter, r *http.Request) {
		completeOK.Store(true)
		_ = json.NewEncoder(w).Encode(contracts.CompletionResult{
			SchemaVersion: "1", CompletionID: "will-set", Accepted: true, RequestDigest: "d",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "flush.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))

	csid := uuid.NewString()
	// Session has server id; event outbox may start empty server id and resolve via GetSession.
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv-flush", AssignmentID: "asg",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	_, err = store.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: -1,
		Payload: map[string]any{"k": "v"},
	})
	require.NoError(t, err)
	cid := uuid.NewString()
	require.NoError(t, store.SaveCompletionIntent(ctx, csid, "", contracts.CompletionRequest{
		SchemaVersion: "1", CompletionID: cid, RequestDigest: "d", ClientTime: time.Now().UTC(),
	}))

	// Fix complete handler to echo completion id — use catch-all rewrite:
	// already encoded static; MarkCompletionAcked uses request id from store.
	// Re-register with dynamic response:
	// (handler above uses CompletionID will-set — MarkCompletionAcked still works with any result)

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.True(t, eventsOK.Load())
	assert.True(t, completeOK.Load())
	assert.GreaterOrEqual(t, res.EventsFlushed, 1)
	assert.GreaterOrEqual(t, res.CompletionsSent, 1)
}

func TestSyncRunBackoffThenCancel(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "temp", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "bo.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(context.Background(), "tok"))

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	loop.MinBackoff = 5 * time.Millisecond
	loop.MaxBackoff = 20 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		loop.Run(ctx, 0) // default interval
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after cancel")
	}
	assert.GreaterOrEqual(t, hits.Load(), int32(1))
	assert.Equal(t, sync.StatusOffline, loop.Status())
}

func TestSyncOnceFlushEventError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
	})
	mux.HandleFunc("POST /student/sessions/srv-e/events", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "fe.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv-e", AssignmentID: "a",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	_, err = store.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: -1,
	})
	require.NoError(t, err)

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.Error(t, res.Err)
	assert.Equal(t, sync.StatusAwaiting, res.Status)
}
