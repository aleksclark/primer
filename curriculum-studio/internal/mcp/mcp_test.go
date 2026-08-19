package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	studiomcp "github.com/aleksclark/primer/curriculum-studio/internal/mcp"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

// ─── fakes ────────────────────────────────────────────────────────────────────

// realValidator spins up an in-process JWKS server and uses authn.Validator.
type realValidator struct {
	key *jwttest.Keypair
	now time.Time
}

func (v *realValidator) Validate(ctx context.Context, raw string) (studiomcp.Principal, error) {
	jwksBody, _ := json.Marshal(jwttest.JWKSDocument(v.key))
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_, _ = w.Write(jwksBody)
	}))
	defer jwksSrv.Close()
	val, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwksSrv.URL,
		Now:      func() time.Time { return v.now },
	})
	if err != nil {
		return studiomcp.Principal{}, err
	}
	return val.Validate(ctx, raw)
}

// fakeWorkspaceService is an in-memory WorkspaceService for tests.
type fakeWorkspaceService struct {
	workspaces []studiomcp.WorkspaceEntry
}

func (f *fakeWorkspaceService) ListForSubject(_ context.Context, _, _ string) ([]studiomcp.WorkspaceEntry, error) {
	return f.workspaces, nil
}

// fakeMembershipLoader returns fixed memberships for every subject.
type fakeMembershipLoader struct {
	mems []studiomcp.MembershipView
}

func (f *fakeMembershipLoader) LoadMemberships(_ context.Context, _ string) ([]studiomcp.MembershipView, error) {
	return f.mems, nil
}

// authorLoader returns a MembershipLoader granting a single author membership.
func authorLoader(wsID string) studiomcp.MembershipLoader {
	return &fakeMembershipLoader{mems: []studiomcp.MembershipView{
		{WorkspaceID: wsID, Role: "author"},
	}}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newTestHandler(t *testing.T, cfg studiomcp.MCPConfig, svc studiomcp.Services,
	v studiomcp.TokenValidator, loader studiomcp.MembershipLoader) *studiomcp.Handler {
	t.Helper()
	return studiomcp.New(studiomcp.Options{
		Config:           cfg,
		Services:         svc,
		Validator:        v,
		MembershipLoader: loader,
	})
}

func mintHuman(t *testing.T, key *jwttest.Keypair, now time.Time) string {
	t.Helper()
	return jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, uuid.NewString()))
}

func mintSvc(t *testing.T, key *jwttest.Keypair, now time.Time, scope string) string {
	t.Helper()
	return jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "test-svc", scope))
}

// postMCP sends a Streamable HTTP MCP POST with the required Accept header.
// The SDK server returns 400 if Accept does not include both content types.
func postMCP(t *testing.T, srv *httptest.Server, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	return resp
}

func slurp(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	_ = r.Body.Close()
	return string(b)
}

const toolsListRPC = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`

// toolNames returns the list of tool names from tools/list for the given handler+token.
func toolNames(t *testing.T, h http.Handler, token string) []string {
	t.Helper()
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)
	resp := postMCP(t, srv, token, toolsListRPC)
	body := slurp(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list returned %d: %s", resp.StatusCode, body)
	}
	var env struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	names := make([]string, 0, len(env.Result.Tools))
	for _, tool := range env.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// ─── P19-S6: auth negatives ───────────────────────────────────────────────────

func TestP19_S6_NoToken(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	h := newTestHandler(t, studiomcp.MCPConfig{}, studiomcp.Services{},
		&realValidator{key: key, now: now}, nil)
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)
	// No token: middleware rejects before the SDK even sees the request.
	// Use a plain request (no Accept header needed — we expect 401 from middleware).
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(toolsListRPC))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestP19_S6_WrongAud(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	claims := jwttest.ValidHumanClaims(now, uuid.NewString())
	claims.Audience = "primer-lms"
	token := jwttest.Mint(t, key, claims)
	h := newTestHandler(t, studiomcp.MCPConfig{}, studiomcp.Services{},
		&realValidator{key: key, now: now}, nil)
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)
	resp := postMCP(t, srv, token, toolsListRPC)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestP19_S6_ExpiredToken(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	// Issued 10 minutes ago; 5-minute lifetime → expired.
	claims := jwttest.ValidHumanClaims(now.Add(-10*time.Minute), uuid.NewString())
	token := jwttest.Mint(t, key, claims)
	// Validator uses the real current time so the token is expired.
	h := newTestHandler(t, studiomcp.MCPConfig{}, studiomcp.Services{},
		&realValidator{key: key, now: now}, nil)
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)
	resp := postMCP(t, srv, token, toolsListRPC)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestP19_S6_InternalUUIDClientID(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	claims := jwttest.ValidHumanClaims(now, uuid.NewString())
	// Replace client_id with a bare UUID; authn.requirePublicClientID rejects it.
	claims.Extra = map[string]any{"client_id": uuid.NewString()}
	token := jwttest.Mint(t, key, claims)
	h := newTestHandler(t, studiomcp.MCPConfig{}, studiomcp.Services{},
		&realValidator{key: key, now: now}, nil)
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)
	resp := postMCP(t, srv, token, toolsListRPC)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestP19_S6_MissingClientID(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	claims := jwttest.ValidHumanClaims(now, uuid.NewString())
	claims.Extra = map[string]any{"client_id": nil} // drops client_id
	token := jwttest.Mint(t, key, claims)
	h := newTestHandler(t, studiomcp.MCPConfig{}, studiomcp.Services{},
		&realValidator{key: key, now: now}, nil)
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)
	resp := postMCP(t, srv, token, toolsListRPC)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ─── P19-S7: Origin allowlist ─────────────────────────────────────────────────

func TestP19_S7_BadOrigin(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	cfg := studiomcp.MCPConfig{OriginAllowlist: "https://studio.example.com"}
	h := newTestHandler(t, cfg, studiomcp.Services{},
		&realValidator{key: key, now: now}, nil)
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)

	token := mintHuman(t, key, now)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(toolsListRPC))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://evil.example.com") // disallowed
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestP19_S7_AllowedOrigin(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	wsID := uuid.NewString()
	cfg := studiomcp.MCPConfig{OriginAllowlist: "https://studio.example.com"}
	h := newTestHandler(t, cfg,
		studiomcp.Services{Workspaces: &fakeWorkspaceService{}},
		&realValidator{key: key, now: now},
		authorLoader(wsID))
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)

	token := mintHuman(t, key, now)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(toolsListRPC))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://studio.example.com") // allowed
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	// Origin passes → must not be 403.
	assert.NotEqual(t, http.StatusForbidden, resp.StatusCode)
}

// ─── P19-S1: deterministic filtered tools/list ───────────────────────────────

func TestP19_S1_ToolsListDeterministic(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	wsID := uuid.NewString()
	h := newTestHandler(t,
		studiomcp.MCPConfig{},
		studiomcp.Services{Workspaces: &fakeWorkspaceService{}},
		&realValidator{key: key, now: now},
		authorLoader(wsID))

	first := toolNames(t, h, mintHuman(t, key, now))
	second := toolNames(t, h, mintHuman(t, key, now))
	assert.Equal(t, first, second, "tools/list must be deterministic across requests")
	for i := 1; i < len(first); i++ {
		assert.LessOrEqual(t, first[i-1], first[i], "tool names must be sorted")
	}
	assert.NotEmpty(t, first, "human author must see at least one tool")
	t.Logf("human author tools (%d): %v", len(first), first)
}

func TestP19_S1_ServiceNoPublishTools(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	wsID := uuid.NewString()
	h := newTestHandler(t,
		studiomcp.MCPConfig{},
		studiomcp.Services{Workspaces: &fakeWorkspaceService{}},
		&realValidator{key: key, now: now},
		// service membership with viewer role (not mutating)
		&fakeMembershipLoader{mems: []studiomcp.MembershipView{
			{WorkspaceID: wsID, Role: "viewer"},
		}})

	svcTools := toolNames(t, h, mintSvc(t, key, now, "studio:mcp studio:read"))
	for _, name := range svcTools {
		assert.False(t, strings.HasPrefix(name, "studio.publish."),
			"service principal must not see publish tool %s", name)
	}
	t.Logf("service tools: %v", svcTools)
}

// ─── P19-S1: stateless — no Mcp-Session-Id ───────────────────────────────────

func TestP19_S1_StatelessNoSessionID(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	wsID := uuid.NewString()
	h := newTestHandler(t,
		studiomcp.MCPConfig{},
		studiomcp.Services{Workspaces: &fakeWorkspaceService{}},
		&realValidator{key: key, now: now},
		authorLoader(wsID))
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)

	resp := postMCP(t, srv, mintHuman(t, key, now), toolsListRPC)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Mcp-Session-Id"),
		"stateless mode must not emit Mcp-Session-Id")
}

// ─── P19-S8: official Go SDK client interop ──────────────────────────────────

// TestP19_S8_OfficialSDKClient exercises the official sdkmcp.StreamableClientTransport
// against our handler end-to-end without a database.
// SDK type: sdkmcp.StreamableClientTransport{Endpoint, HTTPClient, DisableStandaloneSSE}.
// Connect: sdkClient.Connect(ctx, transport, *sdkmcp.ClientSessionOptions).
func TestP19_S8_OfficialSDKClient(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	wsID := uuid.NewString()

	h := newTestHandler(t,
		studiomcp.MCPConfig{},
		studiomcp.Services{
			Workspaces: &fakeWorkspaceService{workspaces: []studiomcp.WorkspaceEntry{
				{ID: wsID, Name: "SDK Test Workspace", Kind: "teacher"},
			}},
		},
		&realValidator{key: key, now: now},
		authorLoader(wsID))

	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)

	token := mintHuman(t, key, now)

	// Wrap the test-server client to inject Accept + Authorization on every request.
	tokenClient := &http.Client{
		Transport: &headerRoundTripper{
			wrapped: srv.Client().Transport,
			headers: map[string]string{
				"Authorization": "Bearer " + token,
				"Accept":        "application/json, text/event-stream",
			},
		},
	}

	transport := &sdkmcp.StreamableClientTransport{
		Endpoint:             srv.URL + "/mcp",
		HTTPClient:           tokenClient,
		DisableStandaloneSSE: true,
	}

	sdkClient := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "test-client", Version: "0.0.1"},
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := sdkClient.Connect(ctx, transport, nil)
	require.NoError(t, err, "SDK Connect must succeed")
	defer session.Close()

	// tools/list — must be non-empty and sorted.
	result, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	require.NoError(t, err, "tools/list must succeed")
	require.NotEmpty(t, result.Tools, "tools/list must return tools for human author")
	for i := 1; i < len(result.Tools); i++ {
		assert.LessOrEqual(t, result.Tools[i-1].Name, result.Tools[i].Name,
			"tools must be sorted")
	}

	// studio.workspaces.list — must return the seeded workspace.
	callRes, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "studio.workspaces.list",
		Arguments: map[string]any{"q": ""},
	})
	require.NoError(t, err, "studio.workspaces.list call must not error")
	assert.False(t, callRes.IsError, "studio.workspaces.list must not be an error result")
	require.NotEmpty(t, callRes.Content, "studio.workspaces.list must return content")
	if callRes.StructuredContent != nil {
		raw, _ := json.Marshal(callRes.StructuredContent)
		assert.Contains(t, string(raw), "SDK Test Workspace",
			"structuredContent must include seeded workspace name")
	}
	t.Logf("P19-S8: %d tools via official SDK, first=%q", len(result.Tools), result.Tools[0].Name)
}

// headerRoundTripper injects a fixed set of headers on every request.
type headerRoundTripper struct {
	wrapped http.RoundTripper
	headers map[string]string
}

func (rt *headerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	for k, v := range rt.headers {
		r2.Header.Set(k, v)
	}
	return rt.wrapped.RoundTrip(r2)
}

// ─── authz filter unit tests ──────────────────────────────────────────────────

func TestAllowedToolNames_HumanSeesPublish(t *testing.T) {
	human := authn.AuthContext{Kind: authn.KindHuman, Scopes: []string{"studio:mcp", "studio:read"}}
	mems := []studiomcp.MembershipView{{WorkspaceID: "ws_1", Role: "author"}}
	names := studiomcp.AllowedToolNames(human, mems)

	assert.Contains(t, names, "studio.publish.propose")
	assert.Contains(t, names, "studio.publish.confirm")
	assert.Contains(t, names, "studio.workspaces.list")
	for i := 1; i < len(names); i++ {
		assert.LessOrEqual(t, names[i-1], names[i], "must be sorted")
	}
}

func TestAllowedToolNames_ServiceNoPublish(t *testing.T) {
	svc := authn.AuthContext{Kind: authn.KindService, Scopes: []string{"studio:mcp", "studio:read"}}
	mems := []studiomcp.MembershipView{{WorkspaceID: "ws_1", Role: "viewer"}}
	names := studiomcp.AllowedToolNames(svc, mems)
	for _, n := range names {
		assert.False(t, strings.HasPrefix(n, "studio.publish."),
			"service must not see publish tool %s", n)
	}
}

func TestAllowedToolNames_ServiceWithDraftScope(t *testing.T) {
	svc := authn.AuthContext{Kind: authn.KindService, Scopes: []string{"studio:mcp", "studio:draft"}}
	mems := []studiomcp.MembershipView{{WorkspaceID: "ws_1", Role: "author"}}
	names := studiomcp.AllowedToolNames(svc, mems)
	hasDraft := false
	for _, n := range names {
		if strings.HasPrefix(n, "studio.drafts.") {
			hasDraft = true
		}
	}
	assert.True(t, hasDraft, "service with studio:draft scope must see draft tools")
}

func TestAllowedToolNames_NoMembership(t *testing.T) {
	human := authn.AuthContext{Kind: authn.KindHuman, Scopes: []string{"studio:mcp"}}
	assert.Empty(t, studiomcp.AllowedToolNames(human, nil),
		"principal with no memberships must see no tools")
}

// ─── config unit tests ────────────────────────────────────────────────────────

func TestMCPConfigValidateProduction(t *testing.T) {
	t.Run("enabled requires origins", func(t *testing.T) {
		err := (studiomcp.MCPConfig{Enabled: true}).Validate("production")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "origin")
	})
	t.Run("disabled may omit origins", func(t *testing.T) {
		require.NoError(t, (studiomcp.MCPConfig{Enabled: false}).Validate("production"))
	})
	t.Run("negative limits rejected", func(t *testing.T) {
		require.Error(t, (studiomcp.MCPConfig{MaxBodyBytes: -1}).Validate("test"))
	})
}

func TestOriginAllowlistParsing(t *testing.T) {
	cfg := studiomcp.MCPConfig{OriginAllowlist: " https://a.com , https://b.com , https://a.com "}
	got := cfg.AllowedOrigins()
	assert.Equal(t, []string{"https://a.com", "https://b.com"}, got)
	assert.Nil(t, studiomcp.MCPConfig{}.AllowedOrigins())
}

// ─── metrics ─────────────────────────────────────────────────────────────────

func TestMetricsCounters(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	h := newTestHandler(t, studiomcp.MCPConfig{}, studiomcp.Services{},
		&realValidator{key: key, now: now}, nil)
	srv := httptest.NewServer(http.StripPrefix("/mcp", h))
	t.Cleanup(srv.Close)

	// No token → auth fail (middleware intercepts before SDK, so no Accept needed).
	req1, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(toolsListRPC))
	req1.Header.Set("Content-Type", "application/json")
	_, _ = srv.Client().Do(req1)

	// Bad token → auth fail.
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(toolsListRPC))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer bad-token")
	_, _ = srv.Client().Do(req2)

	assert.GreaterOrEqual(t, h.Metrics.Requests.Load(), int64(2))
	assert.GreaterOrEqual(t, h.Metrics.AuthFails.Load(), int64(2))
}

var _ = fmt.Sprintf // suppress unused import
