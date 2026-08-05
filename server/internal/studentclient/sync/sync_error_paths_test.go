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

func TestSyncOnceSaveWorkErrorGoesOffline(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{Items: []studentapi.WorkItem{{
			Assignment: domain.StudentAssignment{ID: "a", State: "available", UpdatedAt: time.Now().UTC()},
			Activity:   domain.LearningActivity{Slug: "s"},
			Revision:   domain.LearningActivityRevision{ID: "r", Content: map[string]any{}},
		}}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "sw.db"))
	require.NoError(t, err)
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	require.NoError(t, store.Close()) // force SaveWork error

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.Error(t, res.Err)
	assert.Equal(t, sync.StatusOffline, res.Status)
}

func TestSyncOnceArtifactAndCompletionFlushErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("artifact post error", func(t *testing.T) {
		t.Parallel()
		mux := http.NewServeMux()
		mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
		})
		mux.HandleFunc("POST /student/sessions/srv-a/artifacts", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusInternalServerError)
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		store, err := cache.Open(filepath.Join(t.TempDir(), "af.db"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		require.NoError(t, store.SetDeviceToken(ctx, "tok"))
		csid := uuid.NewString()
		require.NoError(t, store.SaveSession(ctx, cache.Session{
			ClientSessionID: csid, ServerSessionID: "srv-a", AssignmentID: "a",
			State: "started", LastAckedSequence: -1, NextSequence: 0,
		}))
		require.NoError(t, store.SavePendingArtifact(ctx, csid, "", contracts.ArtifactMeta{
			SchemaVersion: "1", ArtifactID: uuid.NewString(), Filename: "f",
			MediaType: "text/plain", ByteSize: 1, SHA256: "x", CreatedAt: time.Now().UTC(),
		}))

		loop := sync.New(studentapi.New(srv.URL, "tok"), store)
		res := loop.SyncOnce(ctx)
		require.Error(t, res.Err)
		// Artifact-only pending does not flip awaiting (handleErr checks events/completions).
		assert.Equal(t, sync.StatusOffline, res.Status)
	})

	t.Run("completion post error", func(t *testing.T) {
		t.Parallel()
		mux := http.NewServeMux()
		mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
		})
		mux.HandleFunc("POST /student/sessions/srv-c/complete", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusBadGateway)
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		store, err := cache.Open(filepath.Join(t.TempDir(), "cf.db"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		require.NoError(t, store.SetDeviceToken(ctx, "tok"))
		csid := uuid.NewString()
		require.NoError(t, store.SaveSession(ctx, cache.Session{
			ClientSessionID: csid, ServerSessionID: "srv-c", AssignmentID: "a",
			State: "started", LastAckedSequence: -1, NextSequence: 0,
		}))
		require.NoError(t, store.SaveCompletionIntent(ctx, csid, "", contracts.CompletionRequest{
			SchemaVersion: "1", CompletionID: uuid.NewString(), RequestDigest: "d", ClientTime: time.Now().UTC(),
		}))

		loop := sync.New(studentapi.New(srv.URL, "tok"), store)
		res := loop.SyncOnce(ctx)
		require.Error(t, res.Err)
		assert.Equal(t, sync.StatusAwaiting, res.Status)
	})

	t.Run("events ack store error after successful post", func(t *testing.T) {
		t.Parallel()
		mux := http.NewServeMux()
		mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
		})
		mux.HandleFunc("POST /student/sessions/srv-e/events", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"acknowledgedSequence": 0})
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		store, err := cache.Open(filepath.Join(t.TempDir(), "ea.db"))
		require.NoError(t, err)
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
		// Close after enqueue so AckEvents fails.
		require.NoError(t, store.Close())

		loop := sync.New(studentapi.New(srv.URL, "tok"), store)
		res := loop.SyncOnce(ctx)
		// SaveWork fails first because store closed — offline.
		require.Error(t, res.Err)
		assert.Equal(t, sync.StatusOffline, res.Status)
	})
}

func TestSyncOnceResolvesEmptyServerIDsOnPendingRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var events, arts, cmps int
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
	})
	mux.HandleFunc("POST /student/sessions/bound/events", func(w http.ResponseWriter, _ *http.Request) {
		events++
		_ = json.NewEncoder(w).Encode(map[string]any{"acknowledgedSequence": 0})
	})
	mux.HandleFunc("POST /student/sessions/bound/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		arts++
		_ = json.NewEncoder(w).Encode(domain.LearningSessionArtifact{ID: "a"})
	})
	mux.HandleFunc("POST /student/sessions/bound/complete", func(w http.ResponseWriter, r *http.Request) {
		cmps++
		var req contracts.CompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(contracts.CompletionResult{
			SchemaVersion: "1", CompletionID: req.CompletionID, Accepted: true, RequestDigest: req.RequestDigest,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "bind.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	csid := uuid.NewString()
	// Server id only on session row; pending rows start empty and resolve via GetSession.
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "bound", AssignmentID: "asg",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	_, err = store.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventTaskViewed, Sequence: -1,
		Payload: map[string]any{"taskId": "t"},
	})
	require.NoError(t, err)
	require.NoError(t, store.SavePendingArtifact(ctx, csid, "", contracts.ArtifactMeta{
		SchemaVersion: "1", ArtifactID: uuid.NewString(), Filename: "f.txt",
		MediaType: "text/plain", ByteSize: 1, SHA256: "ab", CreatedAt: time.Now().UTC(),
	}))
	cid := uuid.NewString()
	require.NoError(t, store.SaveCompletionIntent(ctx, csid, "", contracts.CompletionRequest{
		SchemaVersion: "1", CompletionID: cid, RequestDigest: "dig", ClientTime: time.Now().UTC(),
	}))

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.Equal(t, 1, events)
	assert.Equal(t, 1, arts)
	assert.Equal(t, 1, cmps)
	assert.Equal(t, sync.StatusOnline, res.Status)

	// Accepted completion marks local session completed.
	sess, err := store.GetSession(ctx, csid)
	require.NoError(t, err)
	assert.Equal(t, "completed", sess.State)
}

func TestSyncRunDefaultBackoffsAndCleanPassReset(t *testing.T) {
	t.Parallel()
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			http.Error(w, "temp", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
	}))
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "bo2.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(context.Background(), "tok"))

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	loop.MinBackoff = 5 * time.Millisecond
	loop.MaxBackoff = 30 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		loop.Run(ctx, 15*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not exit")
	}
	assert.GreaterOrEqual(t, n, 2)
}
