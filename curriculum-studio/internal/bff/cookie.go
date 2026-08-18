// Package bff implements the product-owned Studio browser session boundary:
// host-only session cookies, CSRF, server-side PKCE, and Identity token custody.
// Identity remains the issuer only and never sets a product cookie.
package bff

import "net/http"

// Product identifies a Primer product BFF cookie namespace.
type Product string

const (
	// ProductStudio is Curriculum Studio.
	ProductStudio Product = "studio"
	// ProductLMS is the LMS product cookie namespace. Studio login must never
	// mint this cookie; LMS BFF wiring may share this library later.
	ProductLMS Product = "lms"
	// ProductTV is the TV admin cookie namespace.
	ProductTV Product = "tv"

	// StudioSessionCookieName is the Studio host-only session cookie.
	// Production HTTPS uses the __Host- prefix; httptest HTTP keeps the same
	// name with Secure=false so the jar can store it.
	StudioSessionCookieName = "__Host-studio-session"
	// LMSSessionCookieName is the LMS host-only session cookie.
	LMSSessionCookieName = "__Host-lms-session"
	// TVSessionCookieName is the TV host-only session cookie.
	TVSessionCookieName = "__Host-tv-session"
)

// CookiePolicy is the HTTP cookie attribute set. Domain is never set.
type CookiePolicy struct {
	Secure bool
	MaxAge int
}

// SessionCookieName returns the product-specific host-only cookie name.
func SessionCookieName(product Product) string {
	switch product {
	case ProductLMS:
		return LMSSessionCookieName
	case ProductTV:
		return TVSessionCookieName
	default:
		return StudioSessionCookieName
	}
}

// SessionCookie builds a host-only HttpOnly SameSite=Lax Path=/ cookie with no
// Domain attribute. Secure follows policy: production HTTPS must set it.
func SessionCookie(product Product, value string, policy CookiePolicy) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName(product),
		Value:    value,
		Path:     "/",
		MaxAge:   policy.MaxAge,
		HttpOnly: true,
		Secure:   policy.Secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ClearSessionCookie expires the product session cookie with the same host-only
// attributes so browsers drop it.
func ClearSessionCookie(product Product, policy CookiePolicy) *http.Cookie {
	c := SessionCookie(product, "", policy)
	c.MaxAge = -1
	return c
}
