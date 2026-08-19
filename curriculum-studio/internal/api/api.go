// Package api wires Curriculum Studio HTTP routes: health, readiness, metrics,
// and request IDs under /studio/v1. Domain routes land in later waves.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

// maxRequestIDLen bounds client-supplied X-Request-ID values accepted as-is.
const maxRequestIDLen = 128

// Options configures Studio API construction.
type Options struct {
	// Now overrides the clock for tests.
	Now func() time.Time
	// Validator verifies signed Bearer JWTs. A nil validator fails closed on
	// protected routes; health/readiness remain available.
	Validator TokenValidator
	// AcceptServiceTokenAlias enables the migration-only X-Service-Token JWT
	// alias. It is false by default and must never be enabled as an end state.
	AcceptServiceTokenAlias bool
	// MatStub marks newly requested runs ready with zero items. Tests only.
	MatStub bool
	// Querier supplies the local Studio database authorization projection. New
	// normally derives it from the pgx pool; this seam supports non-pool tests.
	Querier repo.Querier
}

// Pinger is the subset of a DB pool needed for readiness.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Server holds shared handler dependencies.
type Server struct {
	pool                    Pinger
	querier                 repo.Querier
	validator               TokenValidator
	acceptServiceTokenAlias bool
	matStub                 bool
	now                     func() time.Time
	reqTotal                atomic.Int64
}

// New builds the Huma API and chi HTTP handler.
func New(pool *pgxpool.Pool, opts Options) (huma.API, http.Handler) {
	return NewWithPinger(pool, opts)
}

// NewWithPinger builds the API against any Pinger (tests may pass a closed pool).
func NewWithPinger(pool Pinger, opts Options) (huma.API, http.Handler) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	s := &Server{pool: pool, querier: opts.Querier, validator: opts.Validator, acceptServiceTokenAlias: opts.AcceptServiceTokenAlias, matStub: opts.MatStub, now: now}
	if s.querier == nil {
		if q, ok := pool.(repo.Querier); ok {
			s.querier = q
		}
	}

	router := chi.NewMux()
	router.Use(middleware.Recoverer)
	router.Use(RequestIDMiddleware)
	router.Use(AccessLogMiddleware)
	router.Use(s.metricsMiddleware)
	router.Use(s.authMiddleware)

	cfg := huma.DefaultConfig("Curriculum Studio API", "0.1.0")
	cfg.Info.Description = "Curriculum Studio service: health, readiness, and (later) authoring APIs."
	cfg.Servers = []*huma.Server{{URL: "/"}}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "Primer Identity JWT with aud=curriculum-studio.",
		},
		"serviceCredential": {
			Type:        "apiKey",
			In:          "header",
			Name:        "X-Service-Token",
			Description: "Migration-only JWT alias; disabled by default and never a static-secret production path.",
		},
	}

	// Mount Huma under /studio/v1 for all authoring/system routes.
	humaAPI := humachi.New(router, cfg)
	// Register the contract baseline first; implemented platform handlers then
	// replace fixture-only operations as each platform wave lands. This keeps
	// C6's offline DTO/OpenAPI surface while ensuring runtime routes use the
	// real S3+ handlers rather than empty fixture responses.
	registerAuthoringRoutes(humaAPI)
	registerEnumComponents(humaAPI)
	// Register at absolute paths with the version prefix so OpenAPI and
	// handlers share /studio/v1/*.
	s.RegisterRoutes(humaAPI)
	s.registerPlanRoutes(humaAPI)
	s.registerMaterializationRoutes(humaAPI)

	// Prometheus-style metrics outside Huma for simple scraping.
	router.Get("/metrics", s.handleMetrics)
	router.Get("/studio/v1/metrics", s.handleMetrics)

	return humaAPI, router
}

// RegisterRoutes wires health and readiness into the Huma API under /studio/v1.
func (s *Server) RegisterRoutes(api huma.API) {
	s.registerAuthRoutes(api)
	s.RegisterWorkspaceRoutes(api)
	s.registerStandardsRoutes(api)
	s.registerResourcesRoutes(api)

	type healthOut struct {
		Body struct {
			Status string `json:"status" example:"ok"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/studio/v1/health",
		Summary:     "Liveness probe",
		Tags:        []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*healthOut, error) {
		out := &healthOut{}
		out.Body.Status = "ok"
		return out, nil
	})

	type readyOut struct {
		Body struct {
			Status string `json:"status" example:"ready"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID:   "ready",
		Method:        http.MethodGet,
		Path:          "/studio/v1/ready",
		Summary:       "Readiness probe (database ping)",
		Tags:          []string{"System"},
		DefaultStatus: http.StatusOK,
	}, func(ctx context.Context, _ *struct{}) (*readyOut, error) {
		if s.pool == nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := s.pool.Ping(pingCtx); err != nil {
			// Log the real ping failure server-side only (redacting handler
			// scrubs DSN/secret material). Public response stays generic.
			slog.Error("readiness database ping failed",
				"error", err,
				"request_id", RequestIDFromContext(ctx),
			)
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}
		out := &readyOut{}
		out.Body.Status = "ready"
		return out, nil
	})
}

// RequestIDMiddleware ensures every response carries X-Request-ID.
// Client-supplied IDs are accepted only when bounded and printable; otherwise
// a fresh UUID is assigned. IDs never include query strings or auth material.
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
	for _, r := range raw {
		// Allow common token alphabet only — reject control/space and high
		// cardinality junk that could pollute logs.
		if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r) &&
			r != '-' && r != '_' && r != '.' && r != ':') {
			return ""
		}
	}
	return raw
}

type requestIDKey struct{}

// RequestIDFromContext returns the request id set by RequestIDMiddleware.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// AccessLogMiddleware emits one structured access log line per request via the
// process slog default (app wires the redacting JSON handler). Fields are
// bounded-cardinality only: method, path (no query), status, duration_ms,
// request_id. No headers, bodies, or DSNs are logged.
func AccessLogMiddleware(next http.Handler) http.Handler {
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
		)
	})
}

func (s *Server) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.reqTotal.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintf(w, "# HELP studio_http_requests_total Total HTTP requests handled.\n")
	_, _ = fmt.Fprintf(w, "# TYPE studio_http_requests_total counter\n")
	_, _ = fmt.Fprintf(w, "studio_http_requests_total %d\n", s.reqTotal.Load())
}
