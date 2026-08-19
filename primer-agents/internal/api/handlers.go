package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/agents/internal/authn"
	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
)

// server holds shared handler dependencies.
type server struct {
	svc       AgentService
	validator TokenValidator
	humaAPI   huma.API
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
	ID         string  `json:"id" format:"uuid"`
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
		ID:         e.ID,
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
