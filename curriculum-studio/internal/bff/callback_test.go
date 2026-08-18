package bff_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/bff"
)

func studioBFF(t *testing.T, issuer string, store bff.Store) http.Handler {
	t.Helper()
	return bff.NewHandler(bff.Config{
		Product:     bff.ProductStudio,
		Issuer:      issuer,
		PublicURL:   "http://studio.test",
		ClientID:    "studio-bff",
		RedirectURI: "http://studio.test/auth/callback",
		Resource:    "https://studio.test/mcp",
		Audience:    "curriculum-studio",
		Scopes:      []string{"openid"},
		Origins:     []string{"http://studio.test"},
		Cookie:      bff.CookiePolicy{Secure: false, MaxAge: 3600},
		Store:       store,
		Now:         func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})
}

func startLogin(t *testing.T, handler http.Handler, store bff.Store) (state, verifier string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://studio.test/auth/login", nil)
	req.Host = "studio.test"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusSeeOther, rr.Code)
	loc, err := url.Parse(rr.Header().Get("Location"))
	require.NoError(t, err)
	state = loc.Query().Get("state")
	rec, ok := store.GetPreAuth(state)
	require.True(t, ok)
	return state, rec.CodeVerifier
}

func TestCallbackExchangesCodeServerSideAndSetsOnlyStudioSession(t *testing.T) {
	t.Parallel()

	var seen url.Values
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/oauth/token", r.URL.Path)
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		form, err := url.ParseQuery(string(body))
		require.NoError(t, err)
		seen = form
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "primer-access-must-stay-server-side",
			"token_type":    "Bearer",
			"expires_in":    900,
			"scope":         "openid",
			"refresh_token": "primer-refresh-must-stay-server-side",
		})
	}))
	t.Cleanup(idp.Close)

	store := bff.NewMemoryStore()
	handler := studioBFF(t, idp.URL, store)
	state, verifier := startLogin(t, handler, store)

	cb := "/auth/callback?code=one-use-code&state=" + url.QueryEscape(state) + "&iss=" + url.QueryEscape(idp.URL)
	req := httptest.NewRequest(http.MethodGet, "http://studio.test"+cb, nil)
	req.Host = "studio.test"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusSeeOther, rr.Code)
	assert.Equal(t, "/", rr.Header().Get("Location"))
	assert.Equal(t, "authorization_code", seen.Get("grant_type"))
	assert.Equal(t, "one-use-code", seen.Get("code"))
	assert.Equal(t, "http://studio.test/auth/callback", seen.Get("redirect_uri"))
	assert.Equal(t, "https://studio.test/mcp", seen.Get("resource"))
	assert.Equal(t, verifier, seen.Get("code_verifier"))
	assert.Equal(t, "studio-bff", seen.Get("client_id"))

	cookies := rr.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, bff.StudioSessionCookieName, cookies[0].Name)
	assert.True(t, cookies[0].HttpOnly)
	assert.Empty(t, cookies[0].Domain)
	assert.Equal(t, "/", cookies[0].Path)
	assert.NotContains(t, cookies[0].Value, "primer-access")
	assert.NotContains(t, cookies[0].Value, "primer-refresh")

	body := rr.Body.String()
	assert.NotContains(t, body, "primer-access-must-stay-server-side")
	assert.NotContains(t, body, "primer-refresh-must-stay-server-side")
	assert.NotContains(t, rr.Header().Get("Location"), "access_token")
	assert.NotContains(t, rr.Header().Get("Location"), "refresh_token")

	sess, ok := store.GetSession(cookies[0].Value)
	require.True(t, ok)
	assert.Equal(t, bff.ProductStudio, sess.Product)
	assert.Equal(t, "primer-access-must-stay-server-side", sess.AccessToken)
	assert.Equal(t, "primer-refresh-must-stay-server-side", sess.RefreshToken)
	assert.NotEmpty(t, sess.CSRF)
	_, leftover := store.GetPreAuth(state)
	assert.False(t, leftover)
}

func TestCallbackRejectsWrongIssuer(t *testing.T) {
	t.Parallel()
	store := bff.NewMemoryStore()
	handler := studioBFF(t, "http://id.test", store)
	state, _ := startLogin(t, handler, store)

	req := httptest.NewRequest(http.MethodGet, "http://studio.test/auth/callback?code=x&state="+url.QueryEscape(state)+"&iss=http://evil.test", nil)
	req.Host = "studio.test"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Empty(t, rr.Result().Cookies())
	assert.NotContains(t, strings.ToLower(rr.Header().Get("Location")), "evil")
}

func TestCallbackRejectsUnknownState(t *testing.T) {
	t.Parallel()
	store := bff.NewMemoryStore()
	handler := studioBFF(t, "http://id.test", store)
	req := httptest.NewRequest(http.MethodGet, "http://studio.test/auth/callback?code=x&state=not-ours&iss=http://id.test", nil)
	req.Host = "studio.test"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Empty(t, rr.Result().Cookies())
}

func TestCallbackRejectsOpenRedirectQuery(t *testing.T) {
	t.Parallel()
	store := bff.NewMemoryStore()
	handler := studioBFF(t, "http://id.test", store)
	state, _ := startLogin(t, handler, store)
	req := httptest.NewRequest(http.MethodGet, "http://studio.test/auth/callback?code=x&state="+url.QueryEscape(state)+"&iss=http://id.test&redirect=https://evil.test/", nil)
	req.Host = "studio.test"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.NotEqual(t, "https://evil.test/", rr.Header().Get("Location"))
	if rr.Code >= 300 && rr.Code < 400 {
		loc := rr.Header().Get("Location")
		assert.True(t, loc == "" || loc == "/" || strings.HasPrefix(loc, "/auth/"), loc)
		assert.NotContains(t, loc, "evil.test")
	}
}
