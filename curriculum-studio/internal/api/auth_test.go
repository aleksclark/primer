package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

func TestAuthMeReturnsSubjectAndMemberships(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	pool := testutil.DB(t)

	sub := uuid.New()
	ws := factory.Workspace(t, pool, func(w *domain.Workspace) {
		w.Name = "Homeschool G6"
	})
	factory.Membership(t, pool, func(m *domain.WorkspaceMembership) {
		m.WorkspaceID = ws.ID
		m.SubjectRef = domain.HumanSubjectRef(sub)
		m.Role = domain.MembershipRoleAuthor
		m.DisplayName = "Ada"
	})

	validator, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)

	_, handler := api.New(pool, api.Options{
		Validator: validator,
		Now:       func() time.Time { return now },
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, sub.String()))
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/auth/me", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	var got struct {
		SubjectRef  string `json:"subjectRef"`
		Kind        string `json:"kind"`
		ClientID    string `json:"clientId"`
		Memberships []struct {
			WorkspaceID   string `json:"workspaceId"`
			WorkspaceName string `json:"workspaceName"`
			Role          string `json:"role"`
			Status        string `json:"status"`
		} `json:"memberships"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, domain.HumanSubjectRef(sub), got.SubjectRef)
	assert.Equal(t, string(authn.KindHuman), got.Kind)
	assert.Equal(t, jwttest.ClientID, got.ClientID)
	require.Len(t, got.Memberships, 1)
	assert.Equal(t, ws.ID.String(), got.Memberships[0].WorkspaceID)
	assert.Equal(t, "Homeschool G6", got.Memberships[0].WorkspaceName)
	assert.Equal(t, domain.MembershipRoleAuthor, got.Memberships[0].Role)
	assert.Equal(t, domain.MembershipStatusActive, got.Memberships[0].Status)
}

func TestAuthMeRejectsInvalidJWT(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	other := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	pool := testutil.DB(t)
	sub := uuid.NewString()
	valid := jwttest.ValidHumanClaims(now, sub)

	validator, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	_, handler := api.New(pool, api.Options{Validator: validator, Now: func() time.Time { return now }})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cases := []struct {
		name string
		tok  string
	}{
		{"missing bearer", ""},
		{"wrong aud", jwttest.Mint(t, key, mutateClaims(valid, func(c *jwttest.Claims) { c.Audience = "primer-lms" }))},
		{"expired", jwttest.Mint(t, key, mutateClaims(valid, func(c *jwttest.Claims) {
			c.IssuedAt = now.Add(-20 * time.Minute)
			c.NotBefore = c.IssuedAt
			c.ExpiresAt = now.Add(-5 * time.Minute)
		}))},
		{"unknown kid", jwttest.Mint(t, other, valid)},
		{"missing client_id", jwttest.Mint(t, key, mutateClaims(valid, func(c *jwttest.Claims) {
			c.ClientID = ""
			c.Extra = map[string]any{"client_id": nil}
		}))},
		{"wrong client_id", jwttest.Mint(t, key, mutateClaims(valid, func(c *jwttest.Claims) { c.ClientID = " " }))},
		{"azp present", jwttest.Mint(t, key, mutateClaims(valid, func(c *jwttest.Claims) {
			c.Extra = map[string]any{"azp": "studio-bff"}
		}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/auth/me", nil)
			require.NoError(t, err)
			if tc.tok != "" {
				req.Header.Set("Authorization", "Bearer "+tc.tok)
			}
			req.Header.Set("X-User-Id", "identity:"+uuid.NewString())
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(body))
			assert.NotContains(t, strings.ToLower(string(body)), "workspace")
			assert.NotContains(t, string(body), "subjectRef")
		})
	}
}

func TestAuthMeDeniesMissingMembership(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	pool := testutil.DB(t)
	_ = factory.Workspace(t, pool)

	validator, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	_, handler := api.New(pool, api.Options{Validator: validator, Now: func() time.Time { return now }})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, uuid.NewString()))
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/auth/me", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
	assert.NotContains(t, strings.ToLower(string(body)), "workspace")
}

func serveJWKS(t testing.TB, keys ...*jwttest.Keypair) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(jwttest.JWKSDocument(keys...))
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mutateClaims(in jwttest.Claims, fn func(*jwttest.Claims)) jwttest.Claims {
	fn(&in)
	return in
}

func TestWorkspaceMutationRequiresLocalAuthorRoleAndAuditsActor(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	pool := testutil.DB(t)
	sub := uuid.New()
	ws := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, ws.ID, domain.HumanSubjectRef(sub), domain.MembershipRoleAuthor)
	validator, err := authn.NewValidator(authn.Options{Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwks.URL, Now: func() time.Time { return now }})
	require.NoError(t, err)
	_, handler := api.New(pool, api.Options{Validator: validator, Now: func() time.Time { return now }})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, sub.String()))
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/studio/v1/workspaces/"+ws.ID.String()+"/probes/mutate", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var actor string
	err = pool.QueryRow(t.Context(), `SELECT actor_subject_ref FROM curriculum_studio.audit_events WHERE workspace_id = $1 ORDER BY created_at DESC LIMIT 1`, ws.ID).Scan(&actor)
	require.NoError(t, err)
	assert.Equal(t, domain.HumanSubjectRef(sub), actor)
}

func TestWorkspaceMutationRejectsViewerAndCrossWorkspaceGet(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	pool := testutil.DB(t)
	sub := uuid.New()
	ws1 := factory.Workspace(t, pool)
	ws2 := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, ws1.ID, domain.HumanSubjectRef(sub), domain.MembershipRoleViewer)
	validator, err := authn.NewValidator(authn.Options{Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwks.URL, Now: func() time.Time { return now }})
	require.NoError(t, err)
	_, handler := api.New(pool, api.Options{Validator: validator, Now: func() time.Time { return now }})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, sub.String()))

	mutateReq, err := http.NewRequest(http.MethodPost, srv.URL+"/studio/v1/workspaces/"+ws1.ID.String()+"/probes/mutate", nil)
	require.NoError(t, err)
	mutateReq.Header.Set("Authorization", "Bearer "+tok)
	mutateResp, err := http.DefaultClient.Do(mutateReq)
	require.NoError(t, err)
	mutateResp.Body.Close()
	assert.Equal(t, http.StatusForbidden, mutateResp.StatusCode)
	var auditCount int
	err = pool.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.audit_events WHERE workspace_id = $1`, ws1.ID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Zero(t, auditCount)

	getReq, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/workspaces/"+ws2.ID.String(), nil)
	require.NoError(t, err)
	getReq.Header.Set("Authorization", "Bearer "+tok)
	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	getResp.Body.Close()
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

func TestMachineProbeRequiresServiceScope(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	pool := testutil.DB(t)
	validator, err := authn.NewValidator(authn.Options{Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwks.URL, Now: func() time.Time { return now }})
	require.NoError(t, err)
	_, handler := api.New(pool, api.Options{Validator: validator, Now: func() time.Time { return now }})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	call := func(scope string) int {
		claims := jwttest.ValidServiceClaims(now, "primer-lms", scope)
		tok := jwttest.Mint(t, key, claims)
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/machine/probes/materialize", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}
	assert.Equal(t, http.StatusOK, call("materialize:write"))
	assert.Equal(t, http.StatusForbidden, call("openid"))
}
