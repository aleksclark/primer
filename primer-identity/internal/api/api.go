// Package api wires Primer Identity HTTP routes: health, readiness, metrics,
// and request IDs. OAuth/OIDC routes land in later waves.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/identity/internal/broker"
)

// Options configures Identity API construction.
type Options struct {
	// Now overrides the clock for tests.
	Now func() time.Time
	// Broker, when set, registers the IB1 OAuth/broker HTTP routes.
	Broker *broker.Service
	// RequireBroker makes readiness fail closed unless the broker was composed.
	RequireBroker bool
	// BrokerHTTP configures cookie and CSRF origin policy for broker routes.
	BrokerHTTP BrokerHTTPOptions
}

const (
	// DefaultAuthorizeRequestTargetMax is the IB1 authorize request-target cap.
	DefaultAuthorizeRequestTargetMax = 8192
	// productionHSTS is sent only when BrokerHTTP Production is true.
	productionHSTS = "max-age=31536000; includeSubDomains"
)

// BrokerHTTPOptions is the HTTP-only broker security policy. It never carries
// provider secrets.
type BrokerHTTPOptions struct {
	// AllowedOrigin is the exact Origin accepted by mutating broker POSTs.
	AllowedOrigin string
	// InsecureTestCookie disables the Secure cookie flag. It is rejected when
	// Production is true.
	InsecureTestCookie bool
	// Production enables fail-closed cookie policy (__Host- + Secure) and HSTS.
	Production bool
	// MaxRequestTargetBytes is the raw authorize request-target cap received
	// from validated config. Zero means the hard maximum 8192. Values above
	// 8192 are rejected at construction.
	MaxRequestTargetBytes int
	// PublicHost is the exact Stytch public host allowed in SSO ContinueURL
	// (for example test.stytch.com or api.stytch.com). Never derived from Host.
	PublicHost string
	// PublicToken is the exact configured public token that an official SSO
	// start URL must carry. It is not a secret.
	PublicToken string
}

// Pinger is the subset of a DB pool needed for readiness.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Server holds shared handler dependencies.
type Server struct {
	pool                    Pinger
	now                     func() time.Time
	reqTotal                atomic.Int64
	broker                  *broker.Service
	requireBroker           bool
	brokerHTTP              BrokerHTTPOptions
	registerBrokerInventory bool
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
	if opts.BrokerHTTP.Production && opts.BrokerHTTP.InsecureTestCookie {
		panic("api: InsecureTestCookie is rejected when Production is true")
	}
	opts.BrokerHTTP.MaxRequestTargetBytes = validatedRequestTargetMax(opts.BrokerHTTP.MaxRequestTargetBytes)
	s := &Server{pool: pool, now: now, broker: opts.Broker, requireBroker: opts.RequireBroker, brokerHTTP: opts.BrokerHTTP}
	return s.build()
}

func validatedRequestTargetMax(n int) int {
	if n == 0 {
		return DefaultAuthorizeRequestTargetMax
	}
	if n < 0 || n > DefaultAuthorizeRequestTargetMax {
		panic("api: MaxRequestTargetBytes must be between 1 and 8192")
	}
	return n
}

func (s *Server) build() (huma.API, http.Handler) {
	if s.now == nil {
		s.now = time.Now
	}
	router := chi.NewMux()
	router.Use(middleware.Recoverer)
	router.Use(RequestIDMiddleware)
	router.Use(AccessLogMiddleware)
	router.Use(s.metricsMiddleware)

	cfg := huma.DefaultConfig("Primer Identity API", "0.1.0")
	cfg.Info.Description = "Primer Identity service: health, readiness, and (later) OIDC/OAuth."
	cfg.Servers = []*huma.Server{{URL: "/"}}

	humaAPI := humachi.New(router, cfg)
	s.RegisterRoutes(humaAPI)

	// Prometheus-style metrics outside Huma for simple scraping.
	router.Get("/metrics", s.handleMetrics)

	return humaAPI, router
}

// RegisterRoutes wires health and readiness into the Huma API.
func (s *Server) RegisterRoutes(api huma.API) {
	type healthOut struct {
		Body struct {
			Status string `json:"status" example:"ok"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "healthz",
		Method:      http.MethodGet,
		Path:        "/healthz",
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
		OperationID:   "readyz",
		Method:        http.MethodGet,
		Path:          "/readyz",
		Summary:       "Readiness probe (database ping)",
		Tags:          []string{"System"},
		DefaultStatus: http.StatusOK,
	}, func(ctx context.Context, _ *struct{}) (*readyOut, error) {
		if s.pool == nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable")
		}
		if s.requireBroker && s.broker == nil {
			return nil, huma.Error503ServiceUnavailable("unavailable")
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

	s.registerBrokerRoutes(api)
}

// RequestIDMiddleware ensures every response carries X-Request-ID.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", rid)
		ctx := context.WithValue(r.Context(), requestIDKey{}, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
	_, _ = fmt.Fprintf(w, "# HELP identity_http_requests_total Total HTTP requests handled.\n")
	_, _ = fmt.Fprintf(w, "# TYPE identity_http_requests_total counter\n")
	_, _ = fmt.Fprintf(w, "identity_http_requests_total %d\n", s.reqTotal.Load())
}
