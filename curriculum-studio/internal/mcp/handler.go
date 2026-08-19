package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

// Metrics are per-process counters exposed via MetricsText.
type Metrics struct {
	Requests  atomic.Int64
	ToolCalls atomic.Int64
	AuthFails atomic.Int64
	OriginRej atomic.Int64
}

// Handler is the http.Handler for the /mcp Streamable HTTP endpoint.
type Handler struct {
	cfg              MCPConfig
	services         Services
	validator        TokenValidator
	querier          repo.Querier
	membershipLoader MembershipLoader
	sdkHandler       http.Handler
	schemaCache      *sdkmcp.SchemaCache
	Metrics          Metrics
}

// TokenValidator is the authn seam — matches authn.Validator.Validate signature.
type TokenValidator interface {
	Validate(context.Context, string) (Principal, error)
}

// MembershipLoader is an injectable seam for loading workspace memberships.
// Tests supply a fake; production defaults to the Postgres querier path.
type MembershipLoader interface {
	LoadMemberships(ctx context.Context, subjectRef string) ([]MembershipView, error)
}

// Options configures the MCP handler.
type Options struct {
	Config    MCPConfig
	Services  Services
	Validator TokenValidator
	// Querier loads workspace memberships per request (Studio DB only).
	// Ignored when MembershipLoader is set.
	Querier repo.Querier
	// MembershipLoader overrides Querier-based membership loading.
	// If nil, memberships are loaded from Querier. If both are nil, the
	// principal receives no memberships (no tools visible).
	MembershipLoader MembershipLoader
}

// New returns a fully wired, stateless MCP http.Handler ready to mount at /mcp.
func New(opts Options) *Handler {
	h := &Handler{
		cfg:              opts.Config,
		services:         opts.Services,
		validator:        opts.Validator,
		querier:          opts.Querier,
		membershipLoader: opts.MembershipLoader,
		schemaCache:      sdkmcp.NewSchemaCache(),
	}

	maxBody := opts.Config.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 4 << 20 // 4 MiB default
	}
	sdkOpts := &sdkmcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		MaxRequestBodyBytes:          maxBody,
		PropagateRequestCancellation: true,
	}
	h.sdkHandler = sdkmcp.NewStreamableHTTPHandler(h.getServer, sdkOpts)
	return h
}

// ServeHTTP implements http.Handler: Origin check → auth → SDK dispatch.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Metrics.Requests.Add(1)

	if !h.originAllowed(r) {
		h.Metrics.OriginRej.Add(1)
		slog.Info("mcp: origin rejected", "origin", r.Header.Get("Origin"))
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if version := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version")); version != "" && version != ProtocolVersion {
		http.Error(w, "unsupported protocol version", http.StatusBadRequest)
		return
	}

	raw := extractBearer(r)
	if raw == "" || h.validator == nil {
		h.Metrics.AuthFails.Add(1)
		writeUnauthorized(w)
		return
	}

	principal, err := h.validator.Validate(r.Context(), raw)
	if err != nil {
		h.Metrics.AuthFails.Add(1)
		slog.Info("mcp: auth rejected")
		writeUnauthorized(w)
		return
	}

	mems, err := h.resolveMemberships(r.Context(), principal.SubjectRef)
	if err != nil {
		h.Metrics.AuthFails.Add(1)
		writeUnauthorized(w)
		return
	}

	ctx := withAuthContext(r.Context(), mcpAuthContext{Auth: principal, Memberships: mems})
	if h.cfg.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.cfg.RequestTimeout)
		defer cancel()
	}
	h.sdkHandler.ServeHTTP(w, r.WithContext(ctx))
}

// getServer is the SDK callback called per request (stateless). It returns a
// fresh *sdkmcp.Server whose tool list is filtered to what the principal may call.
func (h *Handler) getServer(r *http.Request) *sdkmcp.Server {
	authCtx, ok := authFromContext(r.Context())
	if !ok {
		return nil // unreachable: ServeHTTP rejects before here
	}

	srv := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "curriculum-studio", Version: "0.1.0"},
		&sdkmcp.ServerOptions{
			Capabilities: &sdkmcp.ServerCapabilities{
				Tools: &sdkmcp.ToolCapabilities{},
			},
			SchemaCache: h.schemaCache,
		},
	)

	registerAllowedTools(srv, h, authCtx)
	return srv
}

// MetricsText returns a Prometheus-format snippet appended to /metrics.
func (h *Handler) MetricsText() string {
	return fmt.Sprintf(
		"# HELP studio_mcp_requests_total Total MCP HTTP requests.\n"+
			"# TYPE studio_mcp_requests_total counter\n"+
			"studio_mcp_requests_total %d\n"+
			"# HELP studio_mcp_tool_calls_total Total MCP tool calls.\n"+
			"# TYPE studio_mcp_tool_calls_total counter\n"+
			"studio_mcp_tool_calls_total %d\n"+
			"# HELP studio_mcp_auth_failures_total Total MCP auth failures.\n"+
			"# TYPE studio_mcp_auth_failures_total counter\n"+
			"studio_mcp_auth_failures_total %d\n"+
			"# HELP studio_mcp_origin_rejects_total Total MCP origin rejections.\n"+
			"# TYPE studio_mcp_origin_rejects_total counter\n"+
			"studio_mcp_origin_rejects_total %d\n",
		h.Metrics.Requests.Load(),
		h.Metrics.ToolCalls.Load(),
		h.Metrics.AuthFails.Load(),
		h.Metrics.OriginRej.Load(),
	)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (h *Handler) originAllowed(r *http.Request) bool {
	allowed := h.cfg.AllowedOrigins()
	if len(allowed) == 0 {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true // non-browser requests without Origin are permitted
	}
	for _, o := range allowed {
		if o == origin {
			return true
		}
	}
	return false
}

func extractBearer(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	const pfx = "Bearer "
	if len(h) < len(pfx) || !strings.EqualFold(h[:len(pfx)], pfx) {
		return ""
	}
	return strings.TrimSpace(h[len(pfx):])
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="curriculum-studio"`)
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "unauthorized",
	})
}

func (h *Handler) resolveMemberships(ctx context.Context, subjectRef string) ([]MembershipView, error) {
	if h.membershipLoader != nil {
		return h.membershipLoader.LoadMemberships(ctx, subjectRef)
	}
	return h.loadMemberships(ctx, subjectRef)
}

func (h *Handler) loadMemberships(ctx context.Context, subjectRef string) ([]MembershipView, error) {
	if h.querier == nil {
		return nil, nil
	}
	rows, err := repo.NewMembershipRepo(h.querier).ListActiveBySubject(ctx, subjectRef)
	if err != nil {
		return nil, err
	}
	out := make([]MembershipView, 0, len(rows))
	for _, m := range rows {
		out = append(out, MembershipView{
			WorkspaceID: m.WorkspaceID.String(),
			Role:        m.Role,
		})
	}
	return out, nil
}
