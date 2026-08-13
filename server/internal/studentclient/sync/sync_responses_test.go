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

func TestSyncOnceFlushesResponses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var hit atomic.Bool
	subID := uuid.NewString()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{Items: nil})
	})
	mux.HandleFunc("POST /student/sessions/srv-resp/responses", func(w http.ResponseWriter, r *http.Request) {
		hit.Store(true)
		var body contracts.ResponseSubmission
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(contracts.ResponseSubmissionResult{
			SchemaVersion:  contracts.ResponseSchemaVersion,
			SubmissionID:   body.SubmissionID,
			ResponseID:     uuid.NewString(),
			Status:         domain.ResponseSubmitted,
			ReviewRequired: true,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "resp.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))

	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv-resp", AssignmentID: "asg",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	// Empty server id on intent — resolved via GetSession.
	require.NoError(t, store.SaveResponseIntent(ctx, csid, "", contracts.ResponseSubmission{
		SchemaVersion: contracts.ResponseSchemaVersion,
		SubmissionID:  subID,
		TaskID:        "reflect",
		Body:          "I learned about ls and pwd.",
		ClientTime:    time.Now().UTC(),
	}))

	// Unbound pending response is skipped (no server id on session).
	csid2 := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid2, ServerSessionID: "", AssignmentID: "asg2",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	require.NoError(t, store.SaveResponseIntent(ctx, csid2, "", contracts.ResponseSubmission{
		SubmissionID: uuid.NewString(), TaskID: "t", Body: "pending unbound",
	}))

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.True(t, hit.Load())

	// Bound response should be acked; unbound remains pending.
	pending, err := store.ListPendingResponses(ctx)
	require.NoError(t, err)
	for _, p := range pending {
		assert.NotEqual(t, subID, p.SubmissionID)
	}
	assert.GreaterOrEqual(t, len(pending), 1)
}

func TestSyncOnceFlushResponseError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
	})
	mux.HandleFunc("POST /student/sessions/srv-re/responses", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "re.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv-re", AssignmentID: "a",
		State: "started", LastAckedSequence: -1, NextSequence: 0,
	}))
	require.NoError(t, store.SaveResponseIntent(ctx, csid, "srv-re", contracts.ResponseSubmission{
		SubmissionID: uuid.NewString(), TaskID: "t", Body: "x",
	}))

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.Error(t, res.Err)
}
