// Package authn validates Primer Identity access tokens for Curriculum Studio.
// Studio is a validator only: it never mints passwords, OP sessions, or tokens.
package authn

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// AudienceCurriculumStudio is the only accepted JWT audience.
const AudienceCurriculumStudio = "curriculum-studio"

// Kind distinguishes human vs service principals.
type Kind string

const (
	// KindHuman is a human Identity account (sub is a UUID).
	KindHuman Kind = "human"
	// KindService is a service principal (sub is identity:svc:<id>).
	KindService Kind = "service"
)

// ErrUnauthorized is the generic public denial for invalid tokens.
var ErrUnauthorized = errors.New("studio authn: unauthorized")

// AuthContext is the validated principal attached to a request.
// ClientID is always the signed public client_id; azp and internal
// OAuth-client UUIDs are never mapped into it.
type AuthContext struct {
	SubjectRef string
	Kind       Kind
	Scopes     []string
	SessionID  string
	ClientID   string
	Audience   string
	Issuer     string
}

// Options configures a JWKS-backed validator.
type Options struct {
	Issuer   string
	Audience string
	JWKSURL  string
	Now      func() time.Time
	HTTP     HTTPDoer
}

// HTTPDoer is the JWKS fetch seam (tests inject httptest clients).
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Validator verifies compact Primer access tokens against JWKS.
type Validator struct {
	issuer   string
	audience string
	now      func() time.Time
	keys     *jwksCache
}

// NewValidator constructs a fail-closed JWT validator.
func NewValidator(opts Options) (*Validator, error) {
	issuer := trim(opts.Issuer)
	audience := trim(opts.Audience)
	if audience == "" {
		audience = AudienceCurriculumStudio
	}
	if issuer == "" || opts.JWKSURL == "" {
		return nil, ErrUnauthorized
	}
	if audience != AudienceCurriculumStudio {
		return nil, ErrUnauthorized
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Validator{
		issuer:   issuer,
		audience: audience,
		now:      now,
		keys:     newJWKSCache(opts.JWKSURL, opts.HTTP),
	}, nil
}

// Validate parses, verifies, and returns a copied AuthContext.
// The raw token is discarded; failures are always ErrUnauthorized.
func (v *Validator) Validate(ctx context.Context, raw string) (AuthContext, error) {
	if v == nil || v.keys == nil {
		return AuthContext{}, ErrUnauthorized
	}
	if ctx == nil || ctx.Err() != nil {
		return AuthContext{}, ErrUnauthorized
	}
	claims, err := v.verify(ctx, raw)
	if err != nil {
		return AuthContext{}, ErrUnauthorized
	}
	return claims, nil
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
