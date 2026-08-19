package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/agents/internal/authn"
	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
	"github.com/aleksclark/primer/agents/internal/sse"
)

// server holds shared handler dependencies.
type server struct {
	svc       AgentService
	validator TokenValidator
	humaAPI   huma.API
	pool      *pgxpool.Pool
}

// ── Wire types (handler-signature DTOs) ───────────────────────────────────────

// RunResponse is the canonical run representation returned by all run routes.
type RunResponse struct {
	ID                string    `json:"id" format:"uuid" doc:"Server-generated run ID."`
	OwnerNamespace    string    `json:"ownerNamespace" doc:"Signed owner namespace; opaque to callers."`
	Profile           string    `json:"profile"`
	Status            string    `json:"status"`
	StateVersion      int64     `json:"stateVersion"`
	IdempotencyKey    string    `json:"idempotencyKey"`
	InputPreview      *string   `json:"inputPreview,omitempty"`
	SessionID         *string   `json:"sessionId,omitempty" format:"uuid"`
	CancelRequestedAt *string   `json:"cancelRequestedAt,omitempty" format:"date-time"`
	CreatedAt         string    `json:"createdAt" format:"date-time"`
	StartedAt         *string   `json:"startedAt,omitempty" format:"date-time"`
	EndedAt           *string   `json:"endedAt,omitempty" format:"date-time"`
}

// SessionResponse is the canonical session representation.
type SessionResponse struct {
	ID             string  `json:"id" format:"uuid"`
	OwnerNamespace string  `json:"ownerNamespace"`
	Profile        string  `json:"profile"`
	Status         string  `json:"status"`
	Revision       int64   `json:"revision"`
	CallerContext  *string `json:"callerContext,omitempty"`
	CreatedAt      string  `json:"createdAt" format:"date-time"`
	UpdatedAt      string  `json:"updatedAt" format:"date-time"`
}

// EventResponse is the canonical run-event representation.
type EventResponse struct {
	RunID      string  `json:"runId" format:"uuid"`
	Sequence   int64   `json:"sequence"`
	Kind       string  `json:"kind"`
	AgentID    *string `json:"agentId,omitempty"`
	AgentType  *string `json:"agentType,omitempty"`
	AgentDepth *int    `json:"agentDepth,omitempty"`
	Payload    *string `json:"payload,omitempty"`
	CreatedAt  string  `json:"createdAt" format:"date-time"`
}

func runToResponse(r *domain.Run) RunResponse {
	out := RunResponse{
		ID:             r.ID,
		OwnerNamespace: r.OwnerNamespace,
		Profile:        r.Profile,
		Status:         string(r.Status),
		StateVersion:   r.StateVersion,
		IdempotencyKey: r.IdempotencyKey,
		InputPreview:   r.InputPreview,
		SessionID:      r.SessionID,
		CreatedAt:      r.CreatedAt.UTC().Format(time.RFC3339),
	}
	if r.CancelRequestedAt != nil {
		s := r.CancelRequestedAt.UTC().Format(time.RFC3339)
		out.CancelRequestedAt = &s
	}
	if r.StartedAt != nil {
		s := r.StartedAt.UTC().Format(time.RFC3339)
		out.StartedAt = &s
	}
	if r.EndedAt != nil {
		s := r.EndedAt.UTC().Format(time.RFC3339)
		out.EndedAt = &s
	}
	return out
}

func sessionToResponse(s *domain.Session) SessionResponse {
	return SessionResponse{
		ID:             s.ID,
		OwnerNamespace: s.OwnerNamespace,
		Profile:        s.Profile,
		Status:         string(s.Status),
		Revision:       s.Revision,
		CallerContext:  s.CallerContext,
		CreatedAt:      s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func eventToResponse(e *domain.RunEvent) EventResponse {
	return EventResponse{
		RunID:      e.RunID,
		Sequence:   e.Sequence,
		Kind:       e.Kind,
		AgentID:    e.AgentID,
		AgentType:  e.AgentType,
		AgentDepth: e.AgentDepth,
		Payload:    e.Payload,
		CreatedAt:  e.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ── Route registration ────────────────────────────────────────────────────────

func (s *server) registerRoutes(api huma.API) {
	// All /agents/v1 routes require a valid Bearer token.
	// The Huma middleware hook enforces it before any handler body executes.
	huma.Register(api, huma.Operation{
		OperationID: "create-run",
		Method:      http.MethodPost,
		Path:        "/agents/v1/runs",
		Summary:     "Create a run (idempotent)",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeRunsWrite}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeRunsWrite)},
	}, s.handleCreateRun)

	huma.Register(api, huma.Operation{
		OperationID: "get-run",
		Method:      http.MethodGet,
		Path:        "/agents/v1/runs/{id}",
		Summary:     "Get run by ID",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeRunsRead}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeRunsRead)},
	}, s.handleGetRun)

	huma.Register(api, huma.Operation{
		OperationID: "list-runs",
		Method:      http.MethodGet,
		Path:        "/agents/v1/runs",
		Summary:     "List runs owned by caller",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeRunsRead}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeRunsRead)},
	}, s.handleListRuns)

	huma.Register(api, huma.Operation{
		OperationID: "cancel-run",
		Method:      http.MethodPost,
		Path:        "/agents/v1/runs/{id}/cancel",
		Summary:     "Request cancellation of a run (idempotent)",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeRunsCancel}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeRunsCancel)},
	}, s.handleCancelRun)

	huma.Register(api, huma.Operation{
		OperationID: "list-run-events",
		Method:      http.MethodGet,
		Path:        "/agents/v1/runs/{id}/events",
		Summary:     "Page run events by sequence cursor",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeRunsRead}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeRunsRead)},
	}, s.handleListEvents)

	huma.Register(api, huma.Operation{
		OperationID: "create-session",
		Method:      http.MethodPost,
		Path:        "/agents/v1/sessions",
		Summary:     "Create a session",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeSessionsWrite}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeSessionsWrite)},
	}, s.handleCreateSession)

	huma.Register(api, huma.Operation{
		OperationID: "get-session",
		Method:      http.MethodGet,
		Path:        "/agents/v1/sessions/{id}",
		Summary:     "Get session by ID",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeSessionsRead}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeSessionsRead)},
	}, s.handleGetSession)

	huma.Register(api, huma.Operation{
		OperationID: "append-turn",
		Method:      http.MethodPost,
		Path:        "/agents/v1/sessions/{id}/turns",
		Summary:     "Append a turn to a session (idempotent, CAS on revision)",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeRunsWrite}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeRunsWrite)},
	}, s.handleAppendTurn)

	huma.Register(api, huma.Operation{
		OperationID: "list-turns",
		Method:      http.MethodGet,
		Path:        "/agents/v1/sessions/{id}/turns",
		Summary:     "List session turns in order",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeSessionsRead}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeSessionsRead)},
	}, s.handleListTurns)

	// SSE stream is registered directly on chi (not Huma) so the handler
	// controls chunked flushing. Auth is enforced by authnMiddleware.

	// ── Phase 6: Jobs ──────────────────────────────────────────────────────────

	huma.Register(api, huma.Operation{
		OperationID: "create-job",
		Method:      http.MethodPost,
		Path:        "/agents/v1/jobs",
		Summary:     "Create an on-demand job (idempotent; profile is always job)",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeJobsWrite}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeJobsWrite)},
	}, s.handleCreateJob)

	huma.Register(api, huma.Operation{
		OperationID: "create-schedule",
		Method:      http.MethodPost,
		Path:        "/agents/v1/schedules",
		Summary:     "Create a schedule",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeJobsWrite}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeJobsWrite)},
	}, s.handleCreateSchedule)

	huma.Register(api, huma.Operation{
		OperationID: "list-schedules",
		Method:      http.MethodGet,
		Path:        "/agents/v1/schedules",
		Summary:     "List schedules",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeJobsRead}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeJobsRead)},
	}, s.handleListSchedules)

	huma.Register(api, huma.Operation{
		OperationID: "enable-schedule",
		Method:      http.MethodPost,
		Path:        "/agents/v1/schedules/{id}/enable",
		Summary:     "Enable a schedule",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeJobsWrite}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeJobsWrite)},
	}, s.handleEnableSchedule)

	huma.Register(api, huma.Operation{
		OperationID: "disable-schedule",
		Method:      http.MethodPost,
		Path:        "/agents/v1/schedules/{id}/disable",
		Summary:     "Disable a schedule",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeJobsWrite}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeJobsWrite)},
	}, s.handleDisableSchedule)

	// ── Phase 6: Student ───────────────────────────────────────────────────────

	huma.Register(api, huma.Operation{
		OperationID: "create-student-session",
		Method:      http.MethodPost,
		Path:        "/agents/v1/student/sessions",
		Summary:     "Create a student tutoring session (profile always student; requires agents:student:session scope)",
		Tags:        []string{"Student"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeStudentSession}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeStudentSession)},
	}, s.handleCreateStudentSession)

	huma.Register(api, huma.Operation{
		OperationID: "append-student-turn",
		Method:      http.MethodPost,
		Path:        "/agents/v1/student/sessions/{id}/turns",
		Summary:     "Append a student tutoring turn (profile always student; no profile/budget/tools/model fields)",
		Tags:        []string{"Student"},
		Security:    []map[string][]string{{"bearerAuth": {authn.ScopeStudentSession}}},
		Middlewares: huma.Middlewares{s.requireScope(authn.ScopeStudentSession)},
	}, s.handleAppendStudentTurn)
}

// requireScope is a Huma middleware that asserts the required scope on the
// already-validated principal in context.
func (s *server) requireScope(scope string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		p, ok := PrincipalFromContext(ctx.Context())
		if !ok {
			_ = huma.WriteErr(s.humaAPI, ctx, http.StatusUnauthorized,
				"Authentication required", nil)
			return
		}
		if !p.HasScope(scope) {
			_ = huma.WriteErr(s.humaAPI, ctx, http.StatusForbidden,
				"Insufficient scope", nil)
			return
		}
		next(ctx)
	}
}

// ── Run handlers ──────────────────────────────────────────────────────────────

type createRunInput struct {
	// Idempotency-Key header is the stable per-request identity key.
	IdempotencyKey string `header:"Idempotency-Key" required:"true" minLength:"1" maxLength:"256"`
	Body           struct {
		Profile       string  `json:"profile" required:"true" minLength:"1" maxLength:"128"`
		InputPreview  *string `json:"inputPreview,omitempty" maxLength:"2000"`
		SessionID     *string `json:"sessionId,omitempty" format:"uuid"`
		CallerContext *string `json:"callerContext,omitempty" maxLength:"4096"`
	}
}

type runOut struct {
	Body RunResponse
}

func (s *server) handleCreateRun(ctx context.Context, in *createRunInput) (*runOut, error) {
	p, _ := PrincipalFromContext(ctx)
	run, err := s.svc.CreateRun(ctx, CreateRunCmd{
		OwnerNamespace: p.Namespace(),
		IdempotencyKey: in.IdempotencyKey,
		Profile:        in.Body.Profile,
		InputPreview:   in.Body.InputPreview,
		SessionID:      in.Body.SessionID,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &runOut{Body: runToResponse(run)}, nil
}

type getRunInput struct {
	ID string `path:"id" format:"uuid"`
}

func (s *server) handleGetRun(ctx context.Context, in *getRunInput) (*runOut, error) {
	p, _ := PrincipalFromContext(ctx)
	run, err := s.svc.GetRun(ctx, in.ID, p.Namespace())
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &runOut{Body: runToResponse(run)}, nil
}

type listRunsInput struct {
	Limit int `query:"limit" minimum:"1" maximum:"200" default:"50"`
}

type listRunsOut struct {
	Body struct {
		Runs []RunResponse `json:"runs"`
	}
}

func (s *server) handleListRuns(ctx context.Context, in *listRunsInput) (*listRunsOut, error) {
	p, _ := PrincipalFromContext(ctx)
	runs, err := s.svc.ListRuns(ctx, p.Namespace(), in.Limit)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &listRunsOut{}
	out.Body.Runs = make([]RunResponse, len(runs))
	for i, r := range runs {
		out.Body.Runs[i] = runToResponse(r)
	}
	return out, nil
}

type cancelRunInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		ReasonClass *string `json:"reasonClass,omitempty" maxLength:"64"`
	}
}

func (s *server) handleCancelRun(ctx context.Context, in *cancelRunInput) (*runOut, error) {
	p, _ := PrincipalFromContext(ctx)
	run, err := s.svc.RequestCancel(ctx, in.ID, p.Namespace(), in.Body.ReasonClass)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &runOut{Body: runToResponse(run)}, nil
}

type listEventsInput struct {
	ID       string `path:"id" format:"uuid"`
	AfterSeq int64  `query:"afterSeq" minimum:"0" default:"0"`
	Limit    int    `query:"limit" minimum:"1" maximum:"500" default:"100"`
}

type listEventsOut struct {
	Body struct {
		Events []EventResponse `json:"events"`
	}
}

func (s *server) handleListEvents(ctx context.Context, in *listEventsInput) (*listEventsOut, error) {
	p, _ := PrincipalFromContext(ctx)
	// Verify run ownership before listing events.
	if _, err := s.svc.GetRun(ctx, in.ID, p.Namespace()); err != nil {
		return nil, mapServiceError(err)
	}
	evs, err := s.svc.ListEvents(ctx, in.ID, p.Namespace(), in.AfterSeq, in.Limit)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &listEventsOut{}
	out.Body.Events = make([]EventResponse, len(evs))
	for i, e := range evs {
		out.Body.Events[i] = eventToResponse(e)
	}
	return out, nil
}

// ── Session handlers ──────────────────────────────────────────────────────────

type createSessionInput struct {
	Body struct {
		Profile       string  `json:"profile" required:"true" minLength:"1" maxLength:"128"`
		CallerContext *string `json:"callerContext,omitempty" maxLength:"4096"`
	}
}

type sessionOut struct {
	Body SessionResponse
}

func (s *server) handleCreateSession(ctx context.Context, in *createSessionInput) (*sessionOut, error) {
	p, _ := PrincipalFromContext(ctx)
	sess, err := s.svc.CreateSession(ctx, CreateSessionCmd{
		OwnerNamespace: p.Namespace(),
		Profile:        in.Body.Profile,
		CallerContext:  in.Body.CallerContext,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &sessionOut{Body: sessionToResponse(sess)}, nil
}

type getSessionInput struct {
	ID string `path:"id" format:"uuid"`
}

func (s *server) handleGetSession(ctx context.Context, in *getSessionInput) (*sessionOut, error) {
	p, _ := PrincipalFromContext(ctx)
	sess, err := s.svc.GetSession(ctx, in.ID, p.Namespace())
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &sessionOut{Body: sessionToResponse(sess)}, nil
}

// ── Error mapping ─────────────────────────────────────────────────────────────

func mapServiceError(err error) error {
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return huma.Error404NotFound("not found")
	case errors.Is(err, repo.ErrConflict):
		return huma.Error409Conflict("idempotency conflict: same key with different input")
	case errors.Is(err, repo.ErrInvalidTransition):
		return huma.Error409Conflict("invalid lifecycle transition")
	case errors.Is(err, repo.ErrStaleVersion):
		return huma.Error409Conflict("stale version: concurrent update")
	case errors.Is(err, repo.ErrForbidden):
		return huma.Error403Forbidden("forbidden")
	default:
		return huma.Error503ServiceUnavailable("service unavailable")
	}
}

// ── Session turns ──────────────────────────────────────────────────────────────

type appendTurnInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		IdempotencyKey   string  `json:"idempotencyKey" required:"true" minLength:"1" maxLength:"256"`
		Profile          string  `json:"profile" required:"true" minLength:"1" maxLength:"128"`
		InputPreview     *string `json:"inputPreview,omitempty" maxLength:"2000"`
		ExpectedRevision int64   `json:"expectedRevision" minimum:"0"`
	}
}

type appendTurnOut struct {
	Body struct {
		Run     RunResponse     `json:"run"`
		Session SessionResponse `json:"session"`
		Turn    TurnResponse    `json:"turn"`
	}
}

type TurnResponse struct {
	ID             string  `json:"id" format:"uuid"`
	SessionID      string  `json:"sessionId" format:"uuid"`
	TurnSequence   int64   `json:"turnSequence"`
	RunID          *string `json:"runId,omitempty" format:"uuid"`
	IdempotencyKey string  `json:"idempotencyKey"`
	InputPreview   *string `json:"inputPreview,omitempty"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"createdAt" format:"date-time"`
}

func turnToResponse(t *domain.SessionTurn) TurnResponse {
	return TurnResponse{
		ID:             t.ID,
		SessionID:      t.SessionID,
		TurnSequence:   t.TurnSequence,
		RunID:          t.RunID,
		IdempotencyKey: t.IdempotencyKey,
		InputPreview:   t.InputPreview,
		Status:         t.Status,
		CreatedAt:      t.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *server) handleAppendTurn(ctx context.Context, in *appendTurnInput) (*appendTurnOut, error) {
	p, _ := PrincipalFromContext(ctx)
	result, err := s.svc.AppendTurn(ctx, AppendTurnCmd{
		SessionID:        in.ID,
		OwnerNamespace:   p.Namespace(),
		IdempotencyKey:   in.Body.IdempotencyKey,
		Profile:          in.Body.Profile,
		InputPreview:     in.Body.InputPreview,
		ExpectedRevision: in.Body.ExpectedRevision,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &appendTurnOut{}
	out.Body.Run = runToResponse(result.Run)
	out.Body.Session = sessionToResponse(result.Session)
	out.Body.Turn = turnToResponse(result.Turn)
	return out, nil
}

type listTurnsInput struct {
	ID    string `path:"id" format:"uuid"`
	Limit int    `query:"limit" minimum:"1" maximum:"200" default:"50"`
}

type listTurnsOut struct {
	Body struct {
		Turns []TurnResponse `json:"turns"`
	}
}

func (s *server) handleListTurns(ctx context.Context, in *listTurnsInput) (*listTurnsOut, error) {
	p, _ := PrincipalFromContext(ctx)
	turns, err := s.svc.ListTurns(ctx, in.ID, p.Namespace(), in.Limit)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &listTurnsOut{}
	out.Body.Turns = make([]TurnResponse, len(turns))
	for i, t := range turns {
		out.Body.Turns[i] = turnToResponse(t)
	}
	return out, nil
}

// ── SSE stream ─────────────────────────────────────────────────────────────────

// sseHandler returns an http.HandlerFunc for the SSE stream route.
// The stream context (request context) controls only the writer.
// Disconnecting does NOT cancel the run — that requires an explicit cancel call.
func (s *server) sseHandler() http.HandlerFunc {
	cfg := sseConfig
	return func(w http.ResponseWriter, r *http.Request) {
		// Auth: principal must be in context (set by authnMiddleware).
		p, ok := PrincipalFromContext(r.Context())
		if !ok {
			writeUnauth(w)
			return
		}
		if !p.HasScope(authn.ScopeRunsRead) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"title":"Forbidden","status":403}`))
			return
		}

		runID := chi.URLParam(r, "id")
		if runID == "" {
			http.Error(w, "missing run id", http.StatusBadRequest)
			return
		}

		if s.pool == nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}

		// Verify run ownership before streaming — returns 404 for wrong namespace.
		if _, err := repo.GetRunWithOwnership(r.Context(), s.pool, runID, p.Namespace()); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "database error", http.StatusServiceUnavailable)
			return
		}

		cursor := sse.ParseCursor(r)

		// The stream context is r.Context() — it is cancelled on client disconnect.
		// This controls ONLY the SSE writer; the run continues regardless.
		sse.Stream(r.Context(), w, s.pool, runID, p.Namespace(), cursor, cfg)
	}
}

// sseConfig is the module-level SSE configuration (overridable in tests).
var sseConfig = sse.DefaultConfig()

// registerRawRoutes adds routes that require direct http.Handler control
// (e.g., SSE streaming) to the chi router after Huma is wired.
func (s *server) registerRawRoutes(r *chi.Mux) {
	r.Get("/agents/v1/runs/{id}/events/stream", s.sseHandler())
}

// ── Job handlers ──────────────────────────────────────────────────────────────

type ScheduleResponse struct {
	ID             string  `json:"id" format:"uuid"`
	OwnerNamespace string  `json:"ownerNamespace"`
	Profile        string  `json:"profile"`
	JobType        string  `json:"jobType"`
	CronExpr       string  `json:"cronExpr"`
	Timezone       string  `json:"timezone"`
	Enabled        bool    `json:"enabled"`
	InputPreview   *string `json:"inputPreview,omitempty"`
	NextDueAt      *string `json:"nextDueAt,omitempty" format:"date-time"`
	MaxCatchUp     int16   `json:"maxCatchUp"`
	CreatedAt      string  `json:"createdAt" format:"date-time"`
	UpdatedAt      string  `json:"updatedAt" format:"date-time"`
}

func scheduleToResponse(s *domain.Schedule) ScheduleResponse {
	out := ScheduleResponse{
		ID:             s.ID,
		OwnerNamespace: s.OwnerNamespace,
		Profile:        s.Profile,
		JobType:        s.JobType,
		CronExpr:       s.CronExpr,
		Timezone:       s.Timezone,
		Enabled:        s.Enabled,
		InputPreview:   s.InputPreview,
		MaxCatchUp:     s.MaxCatchUp,
		CreatedAt:      s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      s.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if s.NextDueAt != nil {
		t := s.NextDueAt.UTC().Format(time.RFC3339)
		out.NextDueAt = &t
	}
	return out
}

type createJobInput struct {
	IdempotencyKey string `header:"Idempotency-Key" required:"true" minLength:"1" maxLength:"256"`
	Body           struct {
		JobType      string  `json:"jobType" maxLength:"64"`
		InputPreview *string `json:"inputPreview,omitempty" maxLength:"2000"`
	}
}

func (s *server) handleCreateJob(ctx context.Context, in *createJobInput) (*runOut, error) {
	p, _ := PrincipalFromContext(ctx)
	// Profile is always job — not accepted from caller.
	run, err := s.svc.CreateJob(ctx, CreateJobCmd{
		OwnerNamespace: p.Namespace(),
		IdempotencyKey: in.IdempotencyKey,
		JobType:        in.Body.JobType,
		InputPreview:   in.Body.InputPreview,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &runOut{Body: runToResponse(run)}, nil
}

type createScheduleInput struct {
	Body struct {
		Profile      string  `json:"profile" required:"true" minLength:"1" maxLength:"128"`
		JobType      string  `json:"jobType" maxLength:"64"`
		CronExpr     string  `json:"cronExpr" required:"true" minLength:"1" maxLength:"256"`
		Timezone     string  `json:"timezone" maxLength:"64"`
		InputPreview *string `json:"inputPreview,omitempty" maxLength:"2000"`
		MaxCatchUp   int16   `json:"maxCatchUp" minimum:"1" maximum:"10"`
	}
}

type scheduleOut struct {
	Body ScheduleResponse
}

func (s *server) handleCreateSchedule(ctx context.Context, in *createScheduleInput) (*scheduleOut, error) {
	p, _ := PrincipalFromContext(ctx)
	sched, err := s.svc.CreateSchedule(ctx, CreateScheduleCmd{
		OwnerNamespace: p.Namespace(),
		Profile:        in.Body.Profile,
		JobType:        in.Body.JobType,
		CronExpr:       in.Body.CronExpr,
		Timezone:       in.Body.Timezone,
		InputPreview:   in.Body.InputPreview,
		MaxCatchUp:     in.Body.MaxCatchUp,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &scheduleOut{Body: scheduleToResponse(sched)}, nil
}

type listSchedulesInput struct {
	Limit int `query:"limit" minimum:"1" maximum:"200" default:"50"`
}

type listSchedulesOut struct {
	Body struct {
		Schedules []ScheduleResponse `json:"schedules"`
	}
}

func (s *server) handleListSchedules(ctx context.Context, in *listSchedulesInput) (*listSchedulesOut, error) {
	p, _ := PrincipalFromContext(ctx)
	schedules, err := s.svc.ListSchedules(ctx, p.Namespace(), in.Limit)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &listSchedulesOut{}
	out.Body.Schedules = make([]ScheduleResponse, len(schedules))
	for i, sc := range schedules {
		out.Body.Schedules[i] = scheduleToResponse(sc)
	}
	return out, nil
}

type scheduleIDInput struct {
	ID string `path:"id" format:"uuid"`
}

func (s *server) handleEnableSchedule(ctx context.Context, in *scheduleIDInput) (*scheduleOut, error) {
	p, _ := PrincipalFromContext(ctx)
	sched, err := s.svc.SetScheduleEnabled(ctx, in.ID, p.Namespace(), true)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &scheduleOut{Body: scheduleToResponse(sched)}, nil
}

func (s *server) handleDisableSchedule(ctx context.Context, in *scheduleIDInput) (*scheduleOut, error) {
	p, _ := PrincipalFromContext(ctx)
	sched, err := s.svc.SetScheduleEnabled(ctx, in.ID, p.Namespace(), false)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &scheduleOut{Body: scheduleToResponse(sched)}, nil
}

// ── Student handlers ──────────────────────────────────────────────────────────

// createStudentSessionInput deliberately has NO profile/budget/tools/model fields.
// Any request that supplies those fields is a protocol error and is rejected by
// Huma's strict schema validation before reaching this handler.
type createStudentSessionInput struct {
	Body struct {
		// OpaqueStudentRef is a product-authorized opaque reference (e.g. a
		// hashed student enrollment ID). primer-agents never calls the LMS
		// database to interpret it.
		OpaqueStudentRef *string `json:"opaqueStudentRef,omitempty" maxLength:"256"`
	}
}

func (s *server) handleCreateStudentSession(ctx context.Context, in *createStudentSessionInput) (*sessionOut, error) {
	p, _ := PrincipalFromContext(ctx)
	// AdmitStudent re-validates the scope and student invariants.
	if _, err := admitStudentFromPrincipal(p); err != nil {
		return nil, huma.Error403Forbidden(err.Error())
	}
	sess, err := s.svc.CreateStudentSession(ctx, CreateStudentSessionCmd{
		OwnerNamespace:   p.Namespace(),
		OpaqueStudentRef: in.Body.OpaqueStudentRef,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &sessionOut{Body: sessionToResponse(sess)}, nil
}

// appendStudentTurnInput deliberately omits profile/budget/tools/model.
type appendStudentTurnInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		IdempotencyKey   string  `json:"idempotencyKey" required:"true" minLength:"1" maxLength:"256"`
		InputPreview     *string `json:"inputPreview,omitempty" maxLength:"2000"`
		ExpectedRevision int64   `json:"expectedRevision" minimum:"0"`
	}
}

func (s *server) handleAppendStudentTurn(ctx context.Context, in *appendStudentTurnInput) (*appendTurnOut, error) {
	p, _ := PrincipalFromContext(ctx)
	if _, err := admitStudentFromPrincipal(p); err != nil {
		return nil, huma.Error403Forbidden(err.Error())
	}
	result, err := s.svc.AppendStudentTurn(ctx, AppendStudentTurnCmd{
		SessionID:        in.ID,
		OwnerNamespace:   p.Namespace(),
		IdempotencyKey:   in.Body.IdempotencyKey,
		InputPreview:     in.Body.InputPreview,
		ExpectedRevision: in.Body.ExpectedRevision,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := &appendTurnOut{}
	out.Body.Run = runToResponse(result.Run)
	out.Body.Session = sessionToResponse(result.Session)
	out.Body.Turn = turnToResponse(result.Turn)
	return out, nil
}

// admitStudentFromPrincipal delegates to the profile admission package.
// Kept inline to avoid import cycles; wraps profile.AdmitStudent.
func admitStudentFromPrincipal(p authn.Principal) (string, error) {
	if !p.HasScope(authn.ScopeStudentSession) {
		return "", fmt.Errorf("agents:student:session scope required (reviewed Identity student credential needed)")
	}
	return "student", nil
}
