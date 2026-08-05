package sync_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
)

func TestSyncOnceGetPendingSyncErrorStaysOnline(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "gp.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))

	// Empty client token → ensureToken loads from store.
	loop := sync.New(studentapi.New(srv.URL, ""), store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.Equal(t, sync.StatusOnline, res.Status)
}

func TestSyncOnceEnsureTokenFromClosedStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := cache.Open(filepath.Join(t.TempDir(), "cl.db"))
	require.NoError(t, err)
	require.NoError(t, store.Close())

	loop := sync.New(studentapi.New("http://127.0.0.1:1", ""), store)
	res := loop.SyncOnce(ctx)
	require.Error(t, res.Err)
	assert.Equal(t, sync.StatusOffline, res.Status)
}

func TestSyncFlushMultiBatchEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var eventPosts int
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
	})
	mux.HandleFunc("POST /student/sessions/srv-batch/events", func(w http.ResponseWriter, r *http.Request) {
		eventPosts++
		_ = json.NewEncoder(w).Encode(map[string]any{"acknowledgedSequence": 10})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "ms.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))

	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv-batch", AssignmentID: "a",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	for i := 0; i < 2; i++ {
		_, err = store.EnqueueEvent(ctx, csid, contracts.SessionEvent{
			EventID: uuid.NewString(), Type: contracts.EventTaskViewed, Sequence: -1,
			Payload: map[string]any{"i": i},
		})
		require.NoError(t, err)
	}
	// Second client batch with same server id.
	csid2 := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid2, ServerSessionID: "srv-batch", AssignmentID: "b",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	_, err = store.EnqueueEvent(ctx, csid2, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: -1,
	})
	require.NoError(t, err)

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.Equal(t, 2, eventPosts)
	assert.GreaterOrEqual(t, res.EventsFlushed, 3)
}
