package bff_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/bff"
)

func TestStudioSessionCookieUsesHostPrefixWhenSecure(t *testing.T) {
	t.Parallel()

	c := bff.SessionCookie(bff.ProductStudio, "opaque-session-id", bff.CookiePolicy{
		Secure: true,
		MaxAge: 3600,
	})
	require.Equal(t, bff.StudioSessionCookieName, c.Name)
	assert.Equal(t, "__Host-studio-session", c.Name)
	assert.Equal(t, "/", c.Path)
	assert.Empty(t, c.Domain)
	assert.True(t, c.HttpOnly)
	assert.True(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.NotContains(t, c.String(), "Domain=")
}

func TestInsecureTestCookieKeepsHostOnlyAttributesWithoutDomain(t *testing.T) {
	t.Parallel()

	c := bff.SessionCookie(bff.ProductStudio, "opaque-session-id", bff.CookiePolicy{
		Secure: false,
		MaxAge: 3600,
	})
	// HTTPS is unavailable in httptest; name stays host-only and documented.
	assert.Equal(t, bff.StudioSessionCookieName, c.Name)
	assert.Equal(t, "/", c.Path)
	assert.Empty(t, c.Domain)
	assert.True(t, c.HttpOnly)
	assert.False(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.NotContains(t, strings.ToLower(c.String()), "domain=")
}

func TestLMSSessionCookieIsDistinctFromStudio(t *testing.T) {
	t.Parallel()

	studio := bff.SessionCookie(bff.ProductStudio, "studio-sid", bff.CookiePolicy{Secure: true, MaxAge: 60})
	lms := bff.SessionCookie(bff.ProductLMS, "lms-sid", bff.CookiePolicy{Secure: true, MaxAge: 60})
	assert.Equal(t, "__Host-studio-session", studio.Name)
	assert.Equal(t, "__Host-lms-session", lms.Name)
	assert.NotEqual(t, studio.Name, lms.Name)
	assert.Empty(t, lms.Domain)
	assert.True(t, lms.HttpOnly)
	assert.True(t, lms.Secure)
}
