package bff_test

import (
	"crypto/sha256"
	"encoding/base64"
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

func TestLoginRedirectsToIdentityWithExactTupleAndPKCE(t *testing.T) {
	t.Parallel()

	store := bff.NewMemoryStore()
	handler := bff.NewHandler(bff.Config{
		Product:     bff.ProductStudio,
		Issuer:      "http://id.test",
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

	req := httptest.NewRequest(http.MethodGet, "http://studio.test/auth/login", nil)
	req.Host = "studio.test"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusSeeOther, rr.Code)
	loc, err := url.Parse(rr.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "http", loc.Scheme)
	assert.Equal(t, "id.test", loc.Host)
	assert.Equal(t, "/oauth/authorize", loc.Path)
	q := loc.Query()
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "studio-bff", q.Get("client_id"))
	assert.Equal(t, "http://studio.test/auth/callback", q.Get("redirect_uri"))
	assert.Equal(t, "https://studio.test/mcp", q.Get("resource"))
	assert.Equal(t, "curriculum-studio", q.Get("audience"))
	assert.Equal(t, "openid", q.Get("scope"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	state := q.Get("state")
	challenge := q.Get("code_challenge")
	require.NotEmpty(t, state)
	require.Len(t, challenge, 43)

	rec, ok := store.GetPreAuth(state)
	require.True(t, ok)
	assert.Equal(t, bff.ProductStudio, rec.Product)
	assert.Equal(t, "studio-bff", rec.ClientID)
	assert.Equal(t, "http://studio.test/auth/callback", rec.RedirectURI)
	assert.Equal(t, "https://studio.test/mcp", rec.Resource)
	assert.Equal(t, "curriculum-studio", rec.Audience)
	assert.Equal(t, challenge, rec.CodeChallenge)
	require.GreaterOrEqual(t, len(rec.CodeVerifier), 43)
	require.LessOrEqual(t, len(rec.CodeVerifier), 128)
	sum := sha256.Sum256([]byte(rec.CodeVerifier))
	assert.Equal(t, challenge, base64.RawURLEncoding.EncodeToString(sum[:]))
	assert.Equal(t, time.Unix(1_700_000_000, 0).UTC().Add(10*time.Minute), rec.ExpiresAt)

	// Login must not mint a product session cookie.
	for _, c := range rr.Result().Cookies() {
		assert.NotEqual(t, bff.StudioSessionCookieName, c.Name)
		assert.NotEqual(t, bff.LMSSessionCookieName, c.Name)
	}
}

func TestLoginRejectsHostPoisoningBeforeRedirect(t *testing.T) {
	t.Parallel()

	store := bff.NewMemoryStore()
	handler := bff.NewHandler(bff.Config{
		Product:     bff.ProductStudio,
		Issuer:      "http://id.test",
		PublicURL:   "http://studio.test",
		ClientID:    "studio-bff",
		RedirectURI: "http://studio.test/auth/callback",
		Resource:    "https://studio.test/mcp",
		Audience:    "curriculum-studio",
		Scopes:      []string{"openid"},
		Origins:     []string{"http://studio.test"},
		Cookie:      bff.CookiePolicy{Secure: false, MaxAge: 3600},
		Store:       store,
	})

	req := httptest.NewRequest(http.MethodGet, "http://evil.test/auth/login", nil)
	req.Host = "evil.test"
	req.Header.Set("X-Forwarded-Host", "evil.test")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Empty(t, rr.Header().Get("Location"))
	assert.Equal(t, 0, store.PreAuthCount())
	assert.NotContains(t, strings.ToLower(rr.Body.String()), "http://evil.test")
}
