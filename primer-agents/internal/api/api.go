// Package api wires the primer-agents HTTP surface: health/readiness probes
// (unauthenticated) and the authenticated /agents/v1 control-plane routes.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/agents/internal/authn"
)

const maxRequestIDLen = 128

// Pinger is the DB-reachability seam for /readyz.
type Pinger interface {
	Ping(ctx context.Context) error
}

// TokenValidator validates raw Bearer tokens.
type TokenValidator interface {
	Validate(ctx context.Context, raw string) (authn.Principal, error)
}

// Options configures the API handler.
type Options struct {
	Pool      Pinger
	Validator TokenValidator
	Service   AgentService
	Env       string
	Service_  string // service label for logs (field clash: use ServiceName)
}

// New builds a chi HTTP handler with /healthz, /readyz, and /agents/v1 routes.
func New(opts Options) http.Handler {
	_, handler := newAPI(opts)
	return handler
}

// NewSpec returns the Huma API only (for offline OpenAPI emission). The
// returned API has all routes registered but is not bound to a listener;
// passing nil service and validator is safe for generation-only use.
func NewSpec(opts Options) huma.API {
	api, _ := newAPI(opts)
	return api
}

func newAPI(opts Options) (huma.API, http.Handler) {
	env := opts.Env
	if env == "" {
		env = "development"
	}
	svcName := "primer-agents"

	r := chi.NewMux()
	r.Use(middleware.Recoverer)
	r.Use(RequestIDMiddleware)
	r.Use(accessLogMiddleware(env, svcName))
	r.Use(authnMiddleware(opts.Validator))

	// Unprotected probes.
	r.Get("/healthz", handleHealthz)
	r.Get("/readyz", handleReadyz(opts.Pool))

	// Authenticated control-plane under /agents/v1.
	cfg := huma.DefaultConfig("Primer Agents API", "0.1.0")
	cfg.Info.Description = "Primer Agents durable run/session control plane."
	cfg.Servers = []*huma.Server{{URL: "/"}}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "Primer Identity ES256 JWT, aud=primer-agents.",
		},
	}
	humaAPI := humachi.New(r, cfg)

	s := &server{svc: opts.Service, validator: opts.Validator, humaAPI: humaAPI}
	s.registerRoutes(humaAPI)

	return humaAPI, r
}

// ── Health / readiness ────────────────────────────────────────────────────────

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func handleReadyz(pool Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pool == nil {
			http.Error(w, `{"status":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			slog.Error("readiness ping failed", "error", err,
				"request_id", RequestIDFromContext(r.Context()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}

// ── Request-ID ────────────────────────────────────────────────────────────────

type requestIDKey struct{}

// RequestIDMiddleware ensures every request carries an X-Request-ID.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := sanitizeRequestID(r.Header.Get("X-Request-ID"))
		if rid == "" {
			rid = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", rid)
		ctx := context.WithValue(r.Context(), requestIDKey{}, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func sanitizeRequestID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxRequestIDLen {
		return ""
	}
	for _, c := range raw {
		if c > unicode.MaxASCII || (!unicode.IsLetter(c) && !unicode.IsDigit(c) &&
			c != '-' && c != '_' && c != '.' && c != ':') {
			return ""
		}
	}
	return raw
}

// RequestIDFromContext returns the request-id set by RequestIDMiddleware.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// ── Access log ────────────────────────────────────────────────────────────────

func accessLogMiddleware(env, service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			path := r.URL.Path
			if path == "" {
				path = "/"
			}
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			slog.Info("request",
				"method", r.Method,
				"path", path,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFromContext(r.Context()),
				"env", env,
				"service", service,
			)
		})
	}
}

// ── Principal context ─────────────────────────────────────────────────────────

type principalKey struct{}

// PrincipalFromContext returns the validated principal or zero value.
func PrincipalFromContext(ctx context.Context) (authn.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(authn.Principal)
	return p, ok && p.SubjectRef != ""
}

func withPrincipal(ctx context.Context, p authn.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// authnMiddleware validates Bearer tokens for all non-public paths and stores
// the principal in context. Health/readyz paths pass through unauthenticated.
func authnMiddleware(v TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if v == nil {
				writeUnauth(w)
				return
			}
			raw := bearerToken(r)
			if raw == "" {
				writeUnauth(w)
				return
			}
			p, err := v.Validate(r.Context(), raw)
			if err != nil {
				writeUnauth(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), p)))
		})
	}
}

func isPublicPath(path string) bool {
	switch path {
	case "/healthz", "/readyz":
		return true
	default:
		return false
	}
}

func bearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

func writeUnauth(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"title":"Unauthorized","status":401}`))
}
