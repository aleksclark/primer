package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/identityauth"
	"github.com/aleksclark/primer/server/internal/repo"
)

// parentContextKey keys the authenticated educator on the request context.
type parentContextKey struct{}

// parentSessionSecurityScheme names the parent bearer scheme in OpenAPI.
const parentSessionSecurityScheme = "parentSession"

// ParentFromContext returns the educator authenticated for this request.
func ParentFromContext(ctx context.Context) (*domain.Educator, bool) {
	ed, ok := ctx.Value(parentContextKey{}).(*domain.Educator)
	return ed, ok
}

// ParentSessionGuard authenticates parent/admin routes via either:
//   - Legacy path: opaque Bearer session token (DB lookup)
//   - Primer JWT path: Identity-minted ES256 access token (signature + claims)
//
// The guard inspects the token format: JWTs have 3 dot-separated segments;
// opaque tokens do not. Both paths enforce local educator roles — the LMS
// never uses external provider roles for authorization.
//
// Single-family deployment: any authenticated parent or admin may manage all students.
func ParentSessionGuard(api huma.API, q repo.Querier, iv ...*identityauth.Verifier) func(huma.Context, func(huma.Context)) {
	var verifier *identityauth.Verifier
	if len(iv) > 0 {
		verifier = iv[0]
	}
	return func(ctx huma.Context, next func(huma.Context)) {
		token := BearerToken(ctx.Header("Authorization"))
		if token == "" {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing parent session token")
			return
		}

		var ed *domain.Educator

		if identityauth.IsJWT(token) {
			// Primer JWT path.
			principal, err := verifier.Verify(ctx.Context(), token)
			if err != nil {
				_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid identity token")
				return
			}
			// Map identity subject to local educator.
			ed, err = repo.EducatorByIdentitySubject(ctx.Context(), q, principal.Subject)
			if err != nil {
				if errors.Is(err, repo.ErrNotFound) {
					_ = huma.WriteErr(api, ctx, http.StatusForbidden, "no local educator linked to this identity")
					return
				}
				_ = huma.WriteErr(api, ctx, http.StatusInternalServerError, "look up educator")
				return
			}
		} else {
			// Legacy opaque token path.
			_, educator, err := repo.ParentSessionByToken(ctx.Context(), q, token, time.Now().UTC())
			if err != nil {
				if errors.Is(err, repo.ErrNotFound) {
					_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid or expired session")
					return
				}
				_ = huma.WriteErr(api, ctx, http.StatusInternalServerError, "authenticate parent")
				return
			}
			ed = educator
		}

		// Local role enforcement — the LMS is the source of truth for product roles.
		if ed.Role != "parent" && ed.Role != "admin" {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "parent or admin role required")
			return
		}
		next(huma.WithValue(ctx, parentContextKey{}, ed))
	}
}

func parentEducator(ctx context.Context) (*domain.Educator, error) {
	ed, ok := ParentFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("parent not authenticated")
	}
	return ed, nil
}

// LoginRequest is the body for POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email" format:"email" minLength:"3"`
	Password string `json:"password" minLength:"1"`
}

// LoginResponse returns the session token for API clients/tests.
type LoginResponse struct {
	Token     string          `json:"token" doc:"Opaque session token; present as Authorization Bearer."`
	Educator  domain.Educator `json:"educator"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

type loginInput struct {
	Body LoginRequest
}

type loginOutput struct {
	Body LoginResponse
}

func registerParentAuth(h huma.API, q repo.Querier) {
	// Document parent session scheme once.
	if h.OpenAPI().Components.SecuritySchemes == nil {
		h.OpenAPI().Components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}
	h.OpenAPI().Components.SecuritySchemes[parentSessionSecurityScheme] = &huma.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "opaque",
		Description:  "Parent/admin session token from POST /auth/login.",
	}
	h.OpenAPI().Components.SecuritySchemes[deviceTokenSecurityScheme] = &huma.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "opaque",
		Description:  "Student device token from POST /student-devices/pair. Also accepted via X-Device-Token.",
	}

	huma.Register(h, huma.Operation{
		OperationID:   "parent-login",
		Method:        http.MethodPost,
		Path:          "/auth/login",
		Summary:       "Parent login",
		Description:   "Authenticate an educator with email and password. Returns a session token for parent-guarded routes.",
		Tags:          []string{"Auth"},
		DefaultStatus: http.StatusOK,
		Errors:        []int{http.StatusUnauthorized},
	}, func(ctx context.Context, in *loginInput) (*loginOutput, error) {
		ed, err := repo.EducatorByEmail(ctx, q, in.Body.Email)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return nil, huma.Error401Unauthorized("invalid email or password")
			}
			return nil, MapError(err)
		}
		if !repo.CheckPassword(ed.PasswordHash, in.Body.Password) {
			return nil, huma.Error401Unauthorized("invalid email or password")
		}
		now := time.Now().UTC()
		token, sess, err := repo.CreateParentSession(ctx, q, ed.ID, now)
		if err != nil {
			return nil, MapError(err)
		}
		// Clear hash before returning.
		ed.PasswordHash = ""
		return &loginOutput{Body: LoginResponse{
			Token:     token,
			Educator:  *ed,
			ExpiresAt: sess.ExpiresAt,
		}}, nil
	})
}
