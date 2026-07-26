package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/aleksclark/primer/server/internal/repo"
)

// Options configures API construction.
type Options struct {
	// CORSOrigins is the list of allowed origins; empty disables CORS headers.
	CORSOrigins []string
}

// New builds the Huma API and its HTTP handler. The Querier may be nil when
// the API is constructed purely for OpenAPI spec generation.
func New(q repo.Querier, opts Options) (huma.API, http.Handler) {
	router := chi.NewMux()
	router.Use(middleware.Recoverer)
	if len(opts.CORSOrigins) > 0 {
		router.Use(corsMiddleware(opts.CORSOrigins))
	}

	cfg := huma.DefaultConfig("Primer LMS API", "0.1.0")
	cfg.Info.Description = "LMS backend for the Primer AI tutoring system: students, curricula, standards, mastery tracking, and assessments."
	cfg.Servers = []*huma.Server{{URL: "/api/v1"}}

	humaAPI := humachi.New(router, cfg)
	RegisterRoutes(humaAPI, q)
	return humaAPI, router
}

// RegisterRoutes wires the health check and all resource CRUD endpoints into
// the given Huma API. Exposed for tests that use humatest.
func RegisterRoutes(humaAPI huma.API, q repo.Querier) {
	registerHealth(humaAPI)
	registerAll(humaAPI, q)
}

// healthOutput is the health check response.
type healthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok"`
	}
}

func registerHealth(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Tags:        []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*healthOutput, error) {
		out := &healthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})
}

// corsMiddleware sets permissive CORS headers for the configured origins.
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowed["*"] || allowed[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
