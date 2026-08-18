package bff_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/publicsuffix"

	"github.com/aleksclark/primer/curriculum-studio/internal/bff"
)

func completeStudioLogin(t *testing.T) (handler http.Handler, store *bff.MemoryStore, jar http.CookieJar, csrf string) {
	t.Helper()
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-hidden",
			"token_type":    "Bearer",
			"expires_in":    900,
			"scope":         "openid",
			"refresh_token": "refresh-hidden",
		})
	}))
	t.Cleanup(idp.Close)
	store = bff.NewMemoryStore()
	handler = studioBFF(t, idp.URL, store)
	state, _ := startLogin(t, handler, store)
	req := httptest.NewRequest(http.MethodGet, "http://studio.test/auth/callback?code=c&state="+url.QueryEscape(state)+"&iss="+url.QueryEscape(idp.URL), nil)
	req.Host = "studio.test"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusSeeOther, rr.Code)
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	require.NoError(t, err)
	u, err := url.Parse("http://studio.test/")
	require.NoError(t, err)
	jar.SetCookies(u, rr.Result().Cookies())
	sess, ok := store.GetSession(rr.Result().Cookies()[0].Value)
	require.True(t, ok)
	return handler, store, jar, sess.CSRF
}

func TestMeReturnsSubjectWithoutTokens(t *testing.T) {
	t.Parallel()
	handler, _, jar, _ := completeStudioLogin(t)
	req := httptest.NewRequest(http.MethodGet, "http://studio.test/auth/me", nil)
	req.Host = "studio.test"
	for _, c := range jar.Cookies(req.URL) {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, true, body["authenticated"])
	assert.NotEmpty(t, body["csrf"])
	raw, _ := json.Marshal(body)
	assert.NotContains(t, string(raw), "access-hidden")
	assert.NotContains(t, string(raw), "refresh-hidden")
	assert.NotContains(t, string(raw), "stytch")
}

func TestLogoutRequiresCSRFAndExactOrigin(t *testing.T) {
	t.Parallel()
	handler, store, jar, csrf := completeStudioLogin(t)
	u, _ := url.Parse("http://studio.test/")
	sid := jar.Cookies(u)[0].Value

	// Missing CSRF / Origin fails closed and keeps the session.
	req := httptest.NewRequest(http.MethodPost, "http://studio.test/auth/logout", strings.NewReader(""))
	req.Host = "studio.test"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range jar.Cookies(u) {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	_, still := store.GetSession(sid)
	assert.True(t, still)

	// Wrong Origin fails closed.
	form := url.Values{"csrf": {csrf}}
	bad := httptest.NewRequest(http.MethodPost, "http://studio.test/auth/logout", strings.NewReader(form.Encode()))
	bad.Host = "studio.test"
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bad.Header.Set("Origin", "http://evil.test")
	bad.Header.Set("X-CSRF-Token", csrf)
	for _, c := range jar.Cookies(u) {
		bad.AddCookie(c)
	}
	badRR := httptest.NewRecorder()
	handler.ServeHTTP(badRR, bad)
	assert.Equal(t, http.StatusForbidden, badRR.Code)
	_, still = store.GetSession(sid)
	assert.True(t, still)

	// Exact Origin + CSRF clears only the Studio cookie.
	okReq := httptest.NewRequest(http.MethodPost, "http://studio.test/auth/logout", strings.NewReader(form.Encode()))
	okReq.Host = "studio.test"
	okReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	okReq.Header.Set("Origin", "http://studio.test")
	okReq.Header.Set("X-CSRF-Token", csrf)
	for _, c := range jar.Cookies(u) {
		okReq.AddCookie(c)
	}
	okRR := httptest.NewRecorder()
	handler.ServeHTTP(okRR, okReq)
	require.Equal(t, http.StatusSeeOther, okRR.Code)
	_, gone := store.GetSession(sid)
	assert.False(t, gone)
	cleared := false
	for _, c := range okRR.Result().Cookies() {
		if c.Name == bff.StudioSessionCookieName {
			cleared = c.MaxAge < 0 || c.Value == ""
		}
		assert.NotEqual(t, bff.LMSSessionCookieName, c.Name)
	}
	assert.True(t, cleared)
}

func TestMeWithoutCookieIsUnauthenticated(t *testing.T) {
	t.Parallel()
	handler := studioBFF(t, "http://id.test", bff.NewMemoryStore())
	req := httptest.NewRequest(http.MethodGet, "http://studio.test/auth/me", nil)
	req.Host = "studio.test"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	body, _ := io.ReadAll(rr.Body)
	assert.NotContains(t, string(body), "access")
}

func TestStudioLoginDoesNotMintLMSCookie(t *testing.T) {
	t.Parallel()
	_, _, jar, _ := completeStudioLogin(t)
	u, _ := url.Parse("http://studio.test/")
	names := map[string]bool{}
	for _, c := range jar.Cookies(u) {
		names[c.Name] = true
	}
	assert.True(t, names[bff.StudioSessionCookieName])
	assert.False(t, names[bff.LMSSessionCookieName])
}

func TestMeCookieAloneDoesNotRevealToken(t *testing.T) {
	t.Parallel()
	handler, _, jar, _ := completeStudioLogin(t)
	u, _ := url.Parse("http://studio.test/")
	sid := jar.Cookies(u)[0].Value
	assert.NotContains(t, sid, "access-hidden")
	assert.NotContains(t, sid, "refresh-hidden")
	req := httptest.NewRequest(http.MethodGet, "http://studio.test/auth/me", nil)
	req.Host = "studio.test"
	for _, c := range jar.Cookies(u) {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.NotContains(t, rr.Body.String(), "access-hidden")
	assert.Greater(t, time.Now().Unix(), int64(0))
}
