// Package authn validates Primer Identity access tokens for primer-agents.
// This package is a validator only: it never mints tokens or imports Stytch.
package authn

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// AudiencePrimerAgents is the only accepted JWT audience.
const AudiencePrimerAgents = "primer-agents"

// Scopes required by each operation class.
const (
	ScopeRunsWrite     = "agents:runs:write"
	ScopeRunsRead      = "agents:runs:read"
	ScopeRunsCancel    = "agents:runs:cancel"
	ScopeSessionsWrite = "agents:sessions:write"
	ScopeSessionsRead  = "agents:sessions:read"
	// ScopeJobsWrite allows creating on-demand and scheduled jobs.
	ScopeJobsWrite = "agents:jobs:write"
	// ScopeJobsRead allows reading jobs and schedule firings.
	ScopeJobsRead = "agents:jobs:read"
	// ScopeStudentSession allows the dedicated student tutoring endpoint.
	// Production requires a reviewed Identity-issued credential with this scope;
	// absence is an explicit safe blocker that keeps the student feature off.
	ScopeStudentSession = "agents:student:session"
)

// Kind distinguishes human vs service principals.
type Kind string

const (
	KindHuman   Kind = "human"
	KindService Kind = "service"
)

// ErrUnauthorized is the only public denial response for invalid tokens.
// All failure modes produce this sentinel; callers must not inspect the cause.
var ErrUnauthorized = errors.New("agents authn: unauthorized")

// Principal is the validated, request-local identity derived from a signed JWT.
// The raw token is discarded after validation; it never appears in this struct.
type Principal struct {
	// SubjectRef is the canonical Identity subject reference (identity:<uuid>
	// or identity:svc:<id>). Used as a stable identity handle, never as a DB
	// FK into a foreign service.
	SubjectRef string
	Kind       Kind
	// ClientID is the signed public client_id claim. Never an internal UUID.
	ClientID string
	Scopes   []string
	Audience string
	Issuer   string
}

// Namespace returns the owner-namespace string scoped to this principal and
// client. It is derived entirely from signed claims; callers cannot supply or
// alter it. The format is opaque to external consumers.
func (p Principal) Namespace() string {
	return string(p.Kind) + ":" + p.SubjectRef + "/" + p.ClientID
}

// HasScope reports whether scope is among the validated scopes.
func (p Principal) HasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Options configures a JWKS-backed validator.
type Options struct {
	Issuer  string
	JWKSURL string
	Now     func() time.Time
	HTTP    HTTPDoer
}

// HTTPDoer is the JWKS fetch seam; tests inject an httptest server client.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Validator verifies compact Primer ES256 access tokens against JWKS.
type Validator struct {
	issuer string
	now    func() time.Time
	keys   *jwksCache
}

// NewValidator constructs a fail-closed validator. Returns ErrUnauthorized if
// any required field is missing or invalid.
func NewValidator(opts Options) (*Validator, error) {
	issuer := strings.TrimSpace(opts.Issuer)
	jwksURL := strings.TrimSpace(opts.JWKSURL)
	if issuer == "" || jwksURL == "" {
		return nil, ErrUnauthorized
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Validator{
		issuer: issuer,
		now:    now,
		keys:   newJWKSCache(jwksURL, opts.HTTP),
	}, nil
}

// Validate parses, verifies, and returns a copied Principal.
// The raw token is consumed and discarded; failures always return ErrUnauthorized.
func (v *Validator) Validate(ctx context.Context, raw string) (Principal, error) {
	if v == nil || v.keys == nil {
		return Principal{}, ErrUnauthorized
	}
	if ctx == nil || ctx.Err() != nil {
		return Principal{}, ErrUnauthorized
	}
	return v.verify(ctx, raw)
}
