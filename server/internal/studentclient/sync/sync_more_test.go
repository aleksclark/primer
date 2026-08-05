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

func TestSyncOnceFlushesArtifactsAndReportsStatus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var gotArtifact bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{Items: []studentapi.WorkItem{{
			Assignment: domain.StudentAssignment{ID: "asg", State: "available", UpdatedAt: time.Now().UTC()},
			Activity:   domain.LearningActivity{Slug: "s"},
			Revision:   domain.LearningActivityRevision{ID: "r", Content: map[string]any{}},
		}}})
	})
	mux.HandleFunc("POST /student/sessions/srv-art/artifacts", func(w http.ResponseWriter, r *http.Request) {
		gotArtifact = true
		_ = json.NewEncoder(w).Encode(domain.LearningSessionArtifact{ID: "art-srv"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "a.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv-art", AssignmentID: "asg",
		LastAckedSequence: -1, NextSequence: 0, State: "started",
	}))
	require.NoError(t, store.SavePendingArtifact(ctx, csid, "srv-art", contracts.ArtifactMeta{
		SchemaVersion: contracts.SchemaVersion,
		ArtifactID:    uuid.NewString(),
		Filename:      "out.txt",
		MediaType:     "text/plain",
		ByteSize:      3,
		SHA256:        "abc",
		CreatedAt:     time.Now().UTC(),
	}))

	// Client starts without token so ensureToken loads from store.
	cl := studentapi.New(srv.URL, "")
	loop := sync.New(cl, store)
	assert.Equal(t, sync.StatusIdle, loop.Status())

	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.True(t, gotArtifact)
	assert.Equal(t, 1, res.ArtifactsSent)
	assert.Equal(t, 1, res.WorkItems)
	assert.Equal(t, sync.StatusOnline, res.Status)
	assert.Equal(t, sync.StatusOnline, loop.Status())
	assert.Equal(t, 1, loop.LastResult().WorkItems)
}

func TestSyncOnceNetworkErrorWithPendingIsAwaiting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	store, err := cache.Open(filepath.Join(t.TempDir(), "p.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv", AssignmentID: "a",
		LastAckedSequence: -1, NextSequence: 0, State: "started",
	}))
	_, err = store.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: 0,
	})
	require.NoError(t, err)

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.Error(t, res.Err)
	assert.Equal(t, sync.StatusAwaiting, res.Status)
	assert.GreaterOrEqual(t, res.PendingEvents, 1)
}

func TestSyncRunStopsOnRevoked(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		loop.Run(ctx, 50*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
		// returned because revoked
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on revoked")
	}
	assert.Equal(t, sync.StatusRevoked, loop.Status())
}

func TestSyncOnceMissingToken(t *testing.T) {
	t.Parallel()
	store, err := cache.Open(filepath.Join(t.TempDir(), "e.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	loop := sync.New(studentapi.New("http://127.0.0.1:1", ""), store)
	res := loop.SyncOnce(context.Background())
	require.Error(t, res.Err)
	assert.Equal(t, sync.StatusOffline, res.Status)
	assert.Contains(t, res.Err.Error(), "no device token")
}
