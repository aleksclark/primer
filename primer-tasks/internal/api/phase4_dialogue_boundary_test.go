package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"primer-tasks/internal/jobs"
)

func TestDialogueHTTPAndWorkerFailureBoundaries(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	ctx := context.Background()
	route := chi.NewRouteContext()
	routeID := uuid.NewString()
	route.URLParams.Add("id", routeID)
	req := httptest.NewRequest(http.MethodGet, "/student/occurrences/x/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	rec := httptest.NewRecorder()
	s.studentDialogueState(rec, req, uuid.New())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing student state status=%d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/occurrences/x/inspect", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	rec = httptest.NewRecorder()
	s.dialogueInspect(rec, req, scope{Tenant: uuid.NewString(), Subject: "parent"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing inspect status=%d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/occurrences/x/override", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	rec = httptest.NewRecorder()
	s.dialogueOverride(rec, req, scope{Tenant: uuid.NewString(), Subject: "parent"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty override status=%d", rec.Code)
	}
	for _, body := range []string{
		`{"accepted":true,"reason":"` + strings.Repeat("x", 1001) + `"}`,
		`not-json`,
	} {
		req = httptest.NewRequest(http.MethodPost, "/occurrences/x/override", strings.NewReader(body)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
		rec = httptest.NewRecorder()
		s.dialogueOverride(rec, req, scope{Tenant: uuid.NewString(), Subject: "parent"})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid override body status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	// A well-formed override cannot proceed without a current verification
	// attempt; this is a conflict rather than an internal error.
	req = httptest.NewRequest(http.MethodPost, "/occurrences/x/override", strings.NewReader(`{"accepted":true,"reason":"attempt is missing"}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	rec = httptest.NewRecorder()
	s.dialogueOverride(rec, req, scope{Tenant: uuid.NewString(), Subject: "parent"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("missing attempt override status=%d", rec.Code)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	s.StartDialogueWorker(cancelled)
	if err := s.claimAndRunDialogue(ctx, jobs.NewPostgresRepository(pool)); err != nil {
		t.Fatal(err)
	}

	// Exercise the registered HTTP boundary as well as the direct handlers. An
	// unauthenticated request must stop at the session guard, before any
	// dialogue-specific database operation is attempted.
	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/occurrences/" + routeID + "/inspect"},
		{http.MethodPost, "/occurrences/" + routeID + "/override"},
		{http.MethodPost, "/student/occurrences/" + routeID + "/dialogue"},
		{http.MethodGet, "/student/occurrences/" + routeID + "/dialogue"},
	} {
		rec := requestJSON(t, s.Routes(), tc.method, tc.path, "", `{}`)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	// A closed pool is a real infrastructure failure, not a nil-dependency
	// smoke test. Each handler must classify it as an internal failure rather
	// than leaking a database error or claiming the dialogue exists.
	pool.Close()
	req = httptest.NewRequest(http.MethodGet, "/occurrences/x/inspect", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	rec = httptest.NewRecorder()
	s.dialogueInspect(rec, req, scope{Tenant: uuid.NewString(), Subject: "parent"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("inspect database failure status=%d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/occurrences/x/override", strings.NewReader(`{"accepted":true,"reason":"database failure boundary"}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	rec = httptest.NewRecorder()
	s.dialogueOverride(rec, req, scope{Tenant: uuid.NewString(), Subject: "parent"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("override database failure status=%d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/student/occurrences/x/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	rec = httptest.NewRecorder()
	s.studentDialogueState(rec, req, uuid.New())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("student state database failure status=%d", rec.Code)
	}
	if err := s.claimAndRunDialogue(ctx, jobs.NewPostgresRepository(pool)); err == nil {
		t.Fatal("closed database claim unexpectedly succeeded")
	}
	if err := s.runDialogueJob(ctx, jobs.DialogueJob{TenantID: "tenant", AttemptID: "attempt", MessageID: "message"}); err == nil {
		t.Fatal("closed database worker unexpectedly succeeded")
	}
}
