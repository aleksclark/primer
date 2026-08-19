package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/appservice"
	"github.com/aleksclark/primer/agents/internal/authn"
	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
	"github.com/aleksclark/primer/agents/internal/testutil"
)

type coveragePinger struct{ err error }

func (p coveragePinger) Ping(context.Context) error { return p.err }

func TestCoverageWireConversionsAndErrors(t *testing.T) {
	now := time.Now().UTC()
	p := "payload"
	id := "agent-1"
	depth := 2
	run := &domain.Run{ID: "run-1", OwnerNamespace: "human:identity:x/client", Profile: "admin", Status: domain.RunStatusQueued, CreatedAt: now, StartedAt: &now, EndedAt: &now, InputPreview: &p, IdempotencyKey: "k"}
	got := runToResponse(run)
	require.Equal(t, "run-1", got.ID)
	require.NotNil(t, got.StartedAt)
	e := eventToResponse(&domain.RunEvent{RunID: run.ID, Sequence: 4, Kind: "text", AgentID: &id, AgentDepth: &depth, Payload: &p, CreatedAt: now})
	require.Equal(t, int64(4), e.Sequence)
	s := scheduleToResponse(&domain.Schedule{ID: "s-1", OwnerNamespace: run.OwnerNamespace, Profile: "job", JobType: "sync", CronExpr: "1m", Timezone: "UTC", Enabled: true, NextDueAt: &now, CreatedAt: now, UpdatedAt: now})
	require.NotNil(t, s.NextDueAt)
	tr := turnToResponse(&domain.SessionTurn{ID: "t-1", SessionID: "s-1", TurnSequence: 1, RunID: &run.ID, IdempotencyKey: "turn", Status: "active", CreatedAt: now})
	require.Equal(t, "t-1", tr.ID)

	for _, err := range []error{repo.ErrNotFound, repo.ErrConflict, repo.ErrInvalidTransition, repo.ErrStaleVersion, repo.ErrForbidden, context.Canceled} {
		require.Error(t, mapServiceError(err))
	}
}

func TestCoverageReadyAndRequestIDEdges(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
	h := handleReadyz(logger, coveragePinger{})
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	h(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	h = handleReadyz(logger, coveragePinger{err: context.Canceled})
	w = httptest.NewRecorder()
	h(w, r)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	for _, raw := range []string{"", strings.Repeat("x", maxRequestIDLen+1), "bad value", "ümlaut"} {
		require.Empty(t, sanitizeRequestID(raw))
	}
	require.Equal(t, "ok-1", sanitizeRequestID(" ok-1 "))
	require.Empty(t, RequestIDFromContext(context.Background()))
}

func TestCoverageSSEHandlerDatabaseOwnershipAndStream(t *testing.T) {
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	p := authn.Principal{SubjectRef: "identity:human", Kind: authn.KindHuman, ClientID: "client", Scopes: []string{authn.ScopeRunsRead}}
	ns := p.Namespace()
	run, err := svc.CreateRun(context.Background(), appservice.CreateRunCmd{OwnerNamespace: ns, IdempotencyKey: "sse", Profile: "tutor"})
	require.NoError(t, err)
	old := sseConfig
	sseConfig.PollInterval = 5 * time.Millisecond
	sseConfig.HeartbeatInterval = 5 * time.Millisecond
	sseConfig.MaxLifetime = 25 * time.Millisecond
	defer func() { sseConfig = old }()
	s := &server{pool: pool}
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", run.ID)
	r := httptest.NewRequest(http.MethodGet, "/agents/v1/runs/"+run.ID+"/events/stream", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, p))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	w := httptest.NewRecorder()
	s.sseHandler()(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	rc.URLParams = chi.RouteParams{}
	rc.URLParams.Add("id", uuid.NewString())
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	w = httptest.NewRecorder()
	s.sseHandler()(w, r)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestCoverageSSEHandlerAuthAndAvailability(t *testing.T) {
	s := &server{}
	h := s.sseHandler()
	r := httptest.NewRequest(http.MethodGet, "/agents/v1/runs/run/events/stream", nil)
	w := httptest.NewRecorder()
	h(w, r)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", "run")
	p := authn.Principal{SubjectRef: "identity:human", Kind: authn.KindHuman, ClientID: "client", Scopes: []string{authn.ScopeRunsRead}}
	r = r.WithContext(context.WithValue(context.Background(), principalKey{}, p))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	w = httptest.NewRecorder()
	h(w, r)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	p.Scopes = []string{authn.ScopeRunsWrite}
	r = r.WithContext(context.WithValue(context.Background(), principalKey{}, p))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	w = httptest.NewRecorder()
	h(w, r)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestCoverageStudentAdmission(t *testing.T) {
	_, err := admitStudentFromPrincipal(authn.Principal{Scopes: []string{authn.ScopeRunsRead}})
	require.Error(t, err)
	profile, err := admitStudentFromPrincipal(authn.Principal{Scopes: []string{authn.ScopeStudentSession}})
	require.NoError(t, err)
	require.Equal(t, "student", profile)
}
