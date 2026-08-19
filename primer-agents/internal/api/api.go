// Package api wires primer-agents HTTP routes: /healthz and /readyz.
// No run or session routes are registered in Phase 1.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const maxRequestIDLen = 128

// Pinger is the subset of a DB pool needed for readiness.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Options configures the API handler.
type Options struct {
	// Pool is used for the /readyz ping.
	Pool Pinger
	// Env is the deployment environment label logged on each request.
	Env string
	// Service is the service name label logged on each request.
	Service string
}

// New builds a chi HTTP handler with /healthz and /readyz routes.
func New(opts Options) http.Handler {
	env := opts.Env
	if env == "" {
		env = "development"
	}
	svc := opts.Service
	if svc == "" {
		svc = "primer-agents"
	}

	r := chi.NewMux()
	r.Use(middleware.Recoverer)
	r.Use(RequestIDMiddleware)
	r.Use(accessLogMiddleware(env, svc))

	r.Get("/healthz", handleHealthz)
	r.Get("/readyz", handleReadyz(opts.Pool))

	return r
}

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
			slog.Error("readiness database ping failed",
				"error", err,
				"request_id", RequestIDFromContext(r.Context()),
			)
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

type requestIDKey struct{}

// RequestIDFromContext returns the request-id set by RequestIDMiddleware.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

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
