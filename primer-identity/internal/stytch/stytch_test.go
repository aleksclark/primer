package stytch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/stytch"
)

const testToken = "opaque-session-token-that-must-not-leak"

func adapterConfig(baseURI string) config.StytchConfig {
	return config.StytchConfig{
		Enabled:               true,
		ProjectID:             "project-test-example",
		Secret:                "secret-value",
		Env:                   "test",
		BaseURI:               baseURI,
		RequestTimeout:        3 * time.Second,
		PositiveCacheTTL:      15 * time.Second,
		NegativeCacheTTL:      5 * time.Second,
		PositiveCacheCapacity: 10000,
		NegativeCacheCapacity: 2000,
	}
}

func TestDisabledStytchDoesNotConstructClient(t *testing.T) {
	cfg := adapterConfig("")
	cfg.Enabled = false
	cfg.ProjectID = ""
	cfg.Secret = ""

	client, err := stytch.New(cfg)
	require.NoError(t, err)
	assert.Nil(t, client)
}

func TestNewRejectsProjectEnvironmentMismatchBeforeClientConstruction(t *testing.T) {
	cfg := adapterConfig("https://stytch.test.example")
	cfg.ProjectID = "project-live-example"

	client, err := stytch.NewWithHTTPClient(cfg, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport must not be called when configuration is invalid")
		return nil, nil
	})})
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, strings.ToLower(err.Error()), "project")
}

func TestNewRejectsLiveEnvironmentBaseURIOverride(t *testing.T) {
	cfg := adapterConfig("https://stytch.test.example")
	cfg.Env = "live"
	cfg.ProjectID = "project-live-example"

	client, err := stytch.NewWithHTTPClient(cfg, nil)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, strings.ToLower(err.Error()), "base uri")
}

func TestNewAllowsLiveProjectWithoutBaseURI(t *testing.T) {
	cfg := adapterConfig("")
	cfg.Env = "live"
	cfg.ProjectID = "project-live-example"

	client, err := stytch.NewWithHTTPClient(cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestAuthenticateSessionUsesOpaqueTokenAndMapsBoundedSnapshot(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	var gotMethod, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-1","member_session":{"organization_id":"org-1","member_id":"member-1","expires_at":"` + expiresAt.Format(time.RFC3339) + `","roles":["admin","viewer"]}}`))
	}))
	defer server.Close()

	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)
	snapshot, err := client.AuthenticateSession(context.Background(), testToken)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/v1/b2b/sessions/authenticate", gotPath)
	assert.Equal(t, testToken, gotBody["session_token"])
	assert.NotContains(t, gotBody, "session_duration_minutes")
	assert.Equal(t, "project-test-example", snapshot.ProjectID)
	assert.Equal(t, "org-1", snapshot.OrganizationID)
	assert.Equal(t, "member-1", snapshot.MemberID)
	assert.True(t, snapshot.Active)
	assert.True(t, snapshot.Eligible)
	assert.Equal(t, expiresAt, snapshot.ExpiresAt)
	assert.NotContains(t, string(mustJSON(snapshot)), "email")
	assert.NotContains(t, string(mustJSON(snapshot)), "token")
}

func TestAuthenticateSessionRejectsTooManyRoles(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	roles := make([]string, stytch.MaxSessionRoles+10)
	for i := range roles {
		roles[i] = "role"
	}
	body, err := json.Marshal(map[string]any{"member_session": map[string]any{
		"organization_id": "org-1", "member_id": "member-1", "expires_at": expiresAt, "roles": roles,
	}})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)
	_, err = client.AuthenticateSession(context.Background(), testToken)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testToken)
	assert.NotContains(t, err.Error(), "org-1")
}

func TestAuthenticateSessionRejectsMalformedOrOversizedSnapshotData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		leak   string
	}{
		{
			name:   "missing organization",
			mutate: func(session map[string]any) { session["organization_id"] = "" },
			leak:   "member-1",
		},
		{
			name:   "organization id too many runes",
			mutate: func(session map[string]any) { session["organization_id"] = strings.Repeat("o", 256) },
			leak:   "organization_id",
		},
		{
			name:   "member id control",
			mutate: func(session map[string]any) { session["member_id"] = "member\u0001" },
			leak:   "member",
		},
		{
			name:   "role too many runes",
			mutate: func(session map[string]any) { session["roles"] = []string{strings.Repeat("r", 129)} },
			leak:   "role",
		},
		{
			name: "role aggregate too large",
			mutate: func(session map[string]any) {
				session["roles"] = []string{strings.Repeat("r", 4097), strings.Repeat("s", 4097)}
			},
			leak: "role",
		},
		{
			name:   "role control",
			mutate: func(session map[string]any) { session["roles"] = []string{"viewer\u0007"} },
			leak:   "viewer",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := map[string]any{
				"organization_id": "org-1",
				"member_id":       "member-1",
				"expires_at":      time.Now().UTC().Add(time.Hour),
				"roles":           []string{"viewer"},
			}
			tc.mutate(session)
			body, err := json.Marshal(map[string]any{"member_session": session})
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer server.Close()

			client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
			require.NoError(t, err)
			_, err = client.AuthenticateSession(context.Background(), testToken)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), testToken)
			assert.NotContains(t, err.Error(), tc.leak)
		})
	}
}

func TestAuthenticateSessionRejectsOversizedValidPrefixAndClosesResponseBody(t *testing.T) {
	validPrefix := []byte(`{"member_session":{"organization_id":"org-1","member_id":"member-1","roles":[]}}`)
	body := append(validPrefix, bytes.Repeat([]byte(" "), 1<<20)...)
	trackingBody := &closeTrackingBody{Reader: bytes.NewReader(body)}
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       trackingBody,
		}, nil
	})
	client, err := stytch.NewWithHTTPClient(adapterConfig("https://test.invalid"), &http.Client{Transport: transport})
	require.NoError(t, err)

	_, err = client.AuthenticateSession(context.Background(), testToken)
	require.Error(t, err)
	assert.EqualError(t, err, "stytch session authenticate failed")
	assert.True(t, trackingBody.closed)
	assert.NotContains(t, err.Error(), testToken)
	assert.NotContains(t, err.Error(), "org-1")
}

func TestAuthenticateSessionRejectsInvalidUTF8BeforeSnapshotMapping(t *testing.T) {
	body := []byte(`{"member_session":{"organization_id":"org-`)
	body = append(body, 0xff)
	body = append(body, []byte(`1","member_id":"member-1","roles":[]}}}`)...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)
	_, err = client.AuthenticateSession(context.Background(), testToken)
	require.Error(t, err)
	assert.EqualError(t, err, "stytch session authenticate failed")
	assert.NotContains(t, err.Error(), testToken)
	assert.NotContains(t, err.Error(), "org-")
}

func TestAuthenticateAndInvalidateRejectOversizedTokensBeforeSDK(t *testing.T) {
	const maxSessionTokenBytes = 4096
	oversized := strings.Repeat("x", maxSessionTokenBytes+1)
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Errorf("oversized token must not create an outbound request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)

	_, authErr := client.AuthenticateSession(context.Background(), oversized)
	require.Error(t, authErr)
	assert.NotContains(t, authErr.Error(), oversized)
	assert.NotContains(t, authErr.Error(), "xxxx")

	revokeErr := client.InvalidateSession(context.Background(), oversized)
	require.Error(t, revokeErr)
	assert.NotContains(t, revokeErr.Error(), oversized)
	assert.NotContains(t, revokeErr.Error(), "xxxx")
	assert.Zero(t, requests)
}

func TestAuthenticateAndInvalidateKeepEmptyAndWhitespaceBehavior(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Errorf("empty token must not create an outbound request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)

	emptyTokens := []string{"", "   ", string([]byte{'	', '\n'})}
	for _, token := range emptyTokens {
		_, authErr := client.AuthenticateSession(context.Background(), token)
		require.ErrorIs(t, authErr, stytch.ErrEmptyToken)

		revokeErr := client.InvalidateSession(context.Background(), token)
		require.ErrorIs(t, revokeErr, stytch.ErrEmptyToken)
	}
	assert.Zero(t, requests)
}

func TestAuthenticateSessionAcceptsBoundaryTokenLength(t *testing.T) {
	const maxSessionTokenBytes = 4096
	boundary := strings.Repeat("é", maxSessionTokenBytes/2)
	require.Equal(t, maxSessionTokenBytes, len(boundary))

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-bound","member_session":{"organization_id":"org-1","member_id":"member-1","expires_at":"2099-01-01T00:00:00Z","roles":[]}}`))
	}))
	defer server.Close()

	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)
	_, err = client.AuthenticateSession(context.Background(), boundary)
	require.NoError(t, err)
	assert.Equal(t, 1, requests)
}

func TestInvalidateSessionUsesOpaqueTokenWithoutExtendingIt(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-revoke"}`))
	}))
	defer server.Close()

	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)
	require.NoError(t, client.InvalidateSession(context.Background(), testToken))
	assert.Equal(t, "/v1/b2b/sessions/revoke", gotPath)
	assert.Equal(t, testToken, gotBody["session_token"])
	assert.NotContains(t, gotBody, "session_duration_minutes")
}

func TestAuthenticateSessionClassifiesOnlyDefinitiveProviderErrors(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		errorType  string
		definitive bool
	}{
		{"invalid", http.StatusBadRequest, "session_not_found", true},
		{"revoked", http.StatusBadRequest, "session_revoked", true},
		{"expired", http.StatusBadRequest, "session_expired", true},
		{"rate limited", http.StatusTooManyRequests, "session_expired", false},
		{"server", http.StatusBadGateway, "session_not_found", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"request_id":"req-2","error_type":"` + tc.errorType + `","error_message":"rejected ` + testToken + `"}`))
			}))
			defer server.Close()

			client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
			require.NoError(t, err)
			_, err = client.AuthenticateSession(context.Background(), testToken)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), testToken)
			var definitive *stytch.DefinitiveSessionError
			assert.Equal(t, tc.definitive, errors.As(err, &definitive))
		})
	}
}

func TestAuthenticateSessionTransportErrorIsTransientAndDoesNotLeakToken(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream transport failed")
	})
	client, err := stytch.NewWithHTTPClient(adapterConfig("https://test.invalid"), &http.Client{Transport: transport})
	require.NoError(t, err)
	_, err = client.AuthenticateSession(context.Background(), testToken)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testToken)
	var definitive *stytch.DefinitiveSessionError
	assert.False(t, errors.As(err, &definitive))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type closeTrackingBody struct {
	*bytes.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}

var _ io.ReadCloser = (*closeTrackingBody)(nil)

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
