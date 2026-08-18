package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

type authContextKey struct{}
type membershipsKey struct{}

// TokenValidator is the validator seam used at the HTTP boundary. The
// production implementation is authn.Validator; tests may provide a narrow
// validator double, but neither implementation may trust caller headers.
type TokenValidator interface {
	Validate(context.Context, string) (authn.AuthContext, error)
}

// MembershipView is the public membership projection on /auth/me.
type MembershipView struct {
	WorkspaceID   uuid.UUID `json:"workspaceId"`
	WorkspaceName string    `json:"workspaceName"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
}

// AuthFromContext returns the validated principal attached by middleware.
func AuthFromContext(ctx context.Context) (authn.AuthContext, bool) {
	if ctx == nil {
		return authn.AuthContext{}, false
	}
	got, ok := ctx.Value(authContextKey{}).(authn.AuthContext)
	return got, ok
}

// MembershipsFromContext returns active memberships loaded from Postgres.
func MembershipsFromContext(ctx context.Context) []MembershipView {
	if ctx == nil {
		return nil
	}
	got, _ := ctx.Value(membershipsKey{}).([]MembershipView)
	return got
}

func isPublicPath(method, path string) bool {
	// Authentication is deliberately limited to the signed Bearer path. BFF
	// login/session/cookie routes belong to I7 and are not mounted by Studio.
	switch path {
	case "/studio/v1/health", "/studio/v1/ready", "/metrics", "/studio/v1/metrics":
		return true
	default:
		return false
	}
}

func bearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// authMiddleware validates every protected request before loading local
// membership state. It has no test/header/cookie identity fallback.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if s.validator == nil {
			writeAuthProblem(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		raw := bearerToken(r)
		if raw == "" {
			writeAuthProblem(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		principal, err := s.validator.Validate(r.Context(), raw)
		if err != nil {
			writeAuthProblem(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), authContextKey{}, principal)

		// Machine probes authorize with an explicit signed scope and do not
		// require a workspace membership. Human/product routes always require
		// an active local membership before their handlers run.
		if r.URL.Path == "/studio/v1/machine/probes/materialize" {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		views, err := s.loadMemberships(r.Context(), principal.SubjectRef)
		if err != nil || len(views) == 0 {
			writeAuthProblem(w, http.StatusForbidden, "forbidden")
			return
		}
		ctx = context.WithValue(ctx, membershipsKey{}, views)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) loadMemberships(ctx context.Context, subjectRef string) ([]MembershipView, error) {
	if s.querier == nil {
		return nil, authn.ErrUnauthorized
	}
	rows, err := repo.NewMembershipRepo(s.querier).ListActiveBySubject(ctx, subjectRef)
	if err != nil {
		return nil, err
	}
	out := make([]MembershipView, 0, len(rows))
	wsRepo := repo.NewWorkspaceRepo(s.querier)
	for _, m := range rows {
		name := ""
		if ws, werr := wsRepo.GetByID(ctx, m.WorkspaceID); werr == nil && ws != nil {
			name = ws.Name
		}
		out = append(out, MembershipView{
			WorkspaceID:   m.WorkspaceID,
			WorkspaceName: name,
			Role:          m.Role,
			Status:        m.Status,
		})
	}
	return out, nil
}

func (s *Server) membershipFor(ctx context.Context, workspaceID uuid.UUID) (MembershipView, bool) {
	for _, m := range MembershipsFromContext(ctx) {
		if m.WorkspaceID == workspaceID {
			return m, true
		}
	}
	return MembershipView{}, false
}

func (s *Server) registerAuthRoutes(api huma.API) {
	type meOut struct {
		Body struct {
			SubjectRef  string           `json:"subjectRef"`
			Kind        string           `json:"kind"`
			ClientID    string           `json:"clientId"`
			Scopes      []string         `json:"scopes"`
			Memberships []MembershipView `json:"memberships"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "authMe",
		Method:      http.MethodGet,
		Path:        "/studio/v1/auth/me",
		Summary:     "Current authenticated subject and memberships",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, _ *struct{}) (*meOut, error) {
		principal, ok := AuthFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}
		out := &meOut{}
		out.Body.SubjectRef = principal.SubjectRef
		out.Body.Kind = string(principal.Kind)
		out.Body.ClientID = principal.ClientID
		out.Body.Scopes = principal.Scopes
		out.Body.Memberships = MembershipsFromContext(ctx)
		if out.Body.Memberships == nil {
			out.Body.Memberships = []MembershipView{}
		}
		return out, nil
	})

	type workspaceGetIn struct {
		WorkspaceID uuid.UUID `path:"workspaceID"`
	}
	type workspaceGetOut struct {
		Body struct {
			ID   uuid.UUID `json:"id"`
			Name string    `json:"name"`
			Slug string    `json:"slug"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "getWorkspaceProbe",
		Method:      http.MethodGet,
		Path:        "/studio/v1/workspaces/{workspaceID}",
		Summary:     "Workspace isolation probe",
		Tags:        []string{"Authz"},
	}, func(ctx context.Context, in *workspaceGetIn) (*workspaceGetOut, error) {
		if _, ok := s.membershipFor(ctx, in.WorkspaceID); !ok {
			return nil, huma.Error404NotFound("not found")
		}
		if s.querier == nil {
			return nil, huma.Error404NotFound("not found")
		}
		ws, err := repo.NewWorkspaceRepo(s.querier).GetByID(ctx, in.WorkspaceID)
		if err != nil || ws == nil {
			return nil, huma.Error404NotFound("not found")
		}
		out := &workspaceGetOut{}
		out.Body.ID = ws.ID
		out.Body.Name = ws.Name
		out.Body.Slug = ws.Slug
		return out, nil
	})

	type mutateIn struct {
		WorkspaceID uuid.UUID `path:"workspaceID"`
	}
	type mutateOut struct {
		Body struct {
			OK      bool      `json:"ok"`
			AuditID uuid.UUID `json:"auditId"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID:   "mutateWorkspaceProbe",
		Method:        http.MethodPost,
		Path:          "/studio/v1/workspaces/{workspaceID}/probes/mutate",
		Summary:       "Author mutation probe",
		Tags:          []string{"Authz"},
		DefaultStatus: http.StatusOK,
	}, func(ctx context.Context, in *mutateIn) (*mutateOut, error) {
		principal, ok := AuthFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}
		mem, ok := s.membershipFor(ctx, in.WorkspaceID)
		if !ok {
			return nil, huma.Error404NotFound("not found")
		}
		if !authz.CanMutate(mem.Role) {
			return nil, huma.Error403Forbidden("forbidden")
		}
		if s.querier == nil {
			return nil, huma.Error403Forbidden("forbidden")
		}
		wsID := in.WorkspaceID
		ev, err := repo.NewAuditRepo(s.querier).Insert(ctx, &repo.AuditEvent{
			WorkspaceID:     &wsID,
			ActorSubjectRef: principal.SubjectRef,
			Action:          "probe.mutate",
			EntityKind:      "workspace",
			EntityID:        &wsID,
			After:           json.RawMessage(`{"probe":true}`),
		})
		if err != nil {
			return nil, huma.Error403Forbidden("forbidden")
		}
		out := &mutateOut{}
		out.Body.OK = true
		out.Body.AuditID = ev.ID
		return out, nil
	})

	type machineOut struct {
		Body struct {
			OK         bool   `json:"ok"`
			SubjectRef string `json:"subjectRef"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "machineMaterializeProbe",
		Method:      http.MethodGet,
		Path:        "/studio/v1/machine/probes/materialize",
		Summary:     "Service scope probe",
		Tags:        []string{"Authz"},
	}, func(ctx context.Context, _ *struct{}) (*machineOut, error) {
		principal, ok := AuthFromContext(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}
		if principal.Kind != authn.KindService || !authz.HasScope(principal.Scopes, "materialize:write") {
			return nil, huma.Error403Forbidden("forbidden")
		}
		out := &machineOut{}
		out.Body.OK = true
		out.Body.SubjectRef = principal.SubjectRef
		return out, nil
	})
}

func writeAuthProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	title := "Unauthorized"
	if status == http.StatusForbidden {
		title = "Forbidden"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title":  title,
		"status": status,
		"detail": detail,
	})
}
