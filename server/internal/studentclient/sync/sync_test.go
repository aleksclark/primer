package sync_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
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

func TestSyncWorkPaginationOver100(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const total = 150
	const page = 100

	var requests []string // recorded after values
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after")
		requests = append(requests, after)
		start := 0
		if after != "" {
			// cursor is "c|asg-N" where N is 1-based index of last returned item
			if _, id, ok := strings.Cut(after, "|"); ok {
				if n, err := strconv.Atoi(strings.TrimPrefix(id, "asg-")); err == nil {
					start = n
				}
			}
		}
		var items []studentapi.WorkItem
		for i := start; i < total && len(items) < page; i++ {
			id := fmt.Sprintf("asg-%d", i+1)
			items = append(items, studentapi.WorkItem{
				Assignment: domain.StudentAssignment{
					ID: id, State: "available",
					UpdatedAt: time.Date(2024, 1, 1, 0, 0, i+1, 0, time.UTC),
				},
				Activity: domain.LearningActivity{Slug: "act-" + id},
				Revision: domain.LearningActivityRevision{ID: "rev-" + id, Content: map[string]any{}},
			})
		}
		cursor := ""
		if len(items) > 0 {
			last := items[len(items)-1]
			cursor = "c|" + last.Assignment.ID
		}
		hasMore := start+len(items) < total
		mode := studentapi.WorkModeIncremental
		if after == "" {
			mode = studentapi.WorkModeSnapshot
		}
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{
			Items: items, Cursor: cursor, Mode: mode, HasMore: hasMore,
		})
	})
	mux.HandleFunc("POST /student/device/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store, err := cache.Open(filepath.Join(t.TempDir(), "p.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.Equal(t, total, res.WorkItems)
	assert.GreaterOrEqual(t, len(requests), 2, "expected multi-page fetch")

	cached, err := store.ListWork(ctx)
	require.NoError(t, err)
	assert.Len(t, cached, total)

	st, err := store.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.False(t, st.InProgress)
	assert.NotEmpty(t, st.Cursor)
	assert.Equal(t, studentapi.WorkModeSnapshot, st.Mode)
}

func TestSyncWorkCursorIncremental(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var afters []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after")
		afters = append(afters, after)
		if after == "" {
			_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{
				Items: []studentapi.WorkItem{{
					Assignment: domain.StudentAssignment{ID: "a1", State: "available", UpdatedAt: time.Now().UTC()},
					Activity:   domain.LearningActivity{Slug: "s1"},
					Revision:   domain.LearningActivityRevision{ID: "r1", Content: map[string]any{}},
				}},
				Cursor: "cur-1", Mode: studentapi.WorkModeSnapshot, HasMore: false,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{
			Items: []studentapi.WorkItem{{
				Assignment: domain.StudentAssignment{ID: "a2", State: "available", UpdatedAt: time.Now().UTC()},
				Activity:   domain.LearningActivity{Slug: "s2"},
				Revision:   domain.LearningActivityRevision{ID: "r2", Content: map[string]any{}},
			}},
			Cursor: "cur-2", Mode: studentapi.WorkModeIncremental, HasMore: false,
		})
	})
	mux.HandleFunc("POST /student/device/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	store, err := cache.Open(filepath.Join(t.TempDir(), "c.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	loop := sync.New(studentapi.New(srv.URL, "tok"), store)

	res1 := loop.SyncOnce(ctx)
	require.NoError(t, res1.Err)
	st, err := store.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "cur-1", st.Cursor)

	res2 := loop.SyncOnce(ctx)
	require.NoError(t, res2.Err)
	require.GreaterOrEqual(t, len(afters), 2)
	assert.Equal(t, "", afters[0])
	assert.Equal(t, "cur-1", afters[1])
	st2, err := store.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "cur-2", st2.Cursor)

	items, err := store.ListWork(ctx)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestSyncCancelledAssignmentUpsert(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mux := http.NewServeMux()
	call := 0
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{
				Items: []studentapi.WorkItem{{
					Assignment: domain.StudentAssignment{ID: "asg-x", State: "available", UpdatedAt: time.Now().UTC()},
					Activity:   domain.LearningActivity{Slug: "nav"},
					Revision:   domain.LearningActivityRevision{ID: "rev-x", Content: map[string]any{}},
				}},
				Cursor: "c1", Mode: studentapi.WorkModeSnapshot,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{
			Items: []studentapi.WorkItem{{
				Assignment: domain.StudentAssignment{ID: "asg-x", State: "cancelled", UpdatedAt: time.Now().UTC()},
				Activity:   domain.LearningActivity{Slug: "nav"},
				Revision:   domain.LearningActivityRevision{ID: "rev-x", Content: map[string]any{}},
			}},
			Cursor: "c2", Mode: studentapi.WorkModeIncremental,
		})
	})
	mux.HandleFunc("POST /student/device/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	store, err := cache.Open(filepath.Join(t.TempDir(), "cancel.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	require.NoError(t, loop.SyncOnce(ctx).Err)
	require.NoError(t, loop.SyncOnce(ctx).Err)
	item, err := store.GetWork(ctx, "asg-x")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", item.Assignment.State)
}

func TestSyncCancelledCompletionPreservesEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{
			Items: nil, Cursor: "", Mode: studentapi.WorkModeSnapshot,
		})
	})
	mux.HandleFunc("POST /student/device/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /student/sessions/srv-1/events", func(w http.ResponseWriter, r *http.Request) {
		var body studentapi.EventsRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		var maxSeq int64 = -1
		for _, e := range body.Events {
			if e.Sequence > maxSeq {
				maxSeq = e.Sequence
			}
		}
		_ = json.NewEncoder(w).Encode(studentapi.EventsAck{AcknowledgedSequence: maxSeq})
	})
	mux.HandleFunc("POST /student/sessions/srv-1/complete", func(w http.ResponseWriter, r *http.Request) {
		var req contracts.CompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(contracts.CompletionResult{
			SchemaVersion: contracts.CompletionSchemaVersion,
			CompletionID:  req.CompletionID,
			Accepted:      false,
			RequestDigest: req.RequestDigest,
			Observations:  req.Observations,
			Message:       "assignment cancelled; evidence retained for parent review (cancelled-after-work)",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	store, err := cache.Open(filepath.Join(t.TempDir(), "ev2.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID: csid, ServerSessionID: "srv-1", AssignmentID: "asg-1",
		LastAckedSequence: -1, NextSequence: 0, State: "started",
	}))
	_, err = store.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventSessionStarted, Sequence: 0,
	})
	require.NoError(t, err)
	cmpID := uuid.NewString()
	require.NoError(t, store.SaveCompletionIntent(ctx, csid, "srv-1", contracts.CompletionRequest{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  cmpID,
		RequestDigest: "d",
		ClientTime:    time.Now().UTC(),
		Observations:  []contracts.Observation{{CheckID: "c1", Passed: true}},
	}))

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.Equal(t, 1, res.CompletionsSent)

	pending, err := store.GetPendingSync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, pending.CompleteCount)
	// Artifact may still be pending if PostArtifact missing — that's fine; completion rejected.
	sess, err := store.GetSession(ctx, csid)
	require.NoError(t, err)
	assert.Equal(t, "cancelled_after_work", sess.State)
}

func TestSyncCursorExpiryRestartsSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var afters []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after")
		afters = append(afters, after)
		if after == "stale-cursor" {
			http.Error(w, `{"title":"invalid after cursor"}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{
			Items: []studentapi.WorkItem{{
				Assignment: domain.StudentAssignment{ID: "fresh", State: "available", UpdatedAt: time.Now().UTC()},
				Activity:   domain.LearningActivity{Slug: "s"},
				Revision:   domain.LearningActivityRevision{ID: "r", Content: map[string]any{}},
			}},
			Cursor: "new-c", Mode: studentapi.WorkModeSnapshot,
		})
	})
	mux.HandleFunc("POST /student/device/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	store, err := cache.Open(filepath.Join(t.TempDir(), "exp.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(ctx, "tok"))
	require.NoError(t, store.SetMeta(ctx, cache.MetaWorkCursor, "stale-cursor"))

	loop := sync.New(studentapi.New(srv.URL, "tok"), store)
	res := loop.SyncOnce(ctx)
	require.NoError(t, res.Err)
	assert.Equal(t, 1, res.WorkItems)
	// First attempt with stale cursor, then snapshot restart with empty after.
	require.GreaterOrEqual(t, len(afters), 2)
	assert.Equal(t, "stale-cursor", afters[0])
	assert.Equal(t, "", afters[1])
	st, err := store.GetWorkSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, "new-c", st.Cursor)
}
