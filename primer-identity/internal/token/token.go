// Package token implements the strict IB2 Primer access-token profile:
// compact JWS ES256, typ=at+jwt, required public client_id, and a single audience.
package token

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aleksclark/primer/identity/internal/domain"
)

const (
	// MaxLifetime is the hard access-token ceiling. Callers may request less.
	MaxLifetime = 15 * time.Minute
	// MintOperationTimeout bounds one mint independently of token lifetime.
	MintOperationTimeout = 30 * time.Second
	// MaxClockSkew is the hard maximum future/expiry skew accepted by verify.
	MaxClockSkew = 5 * time.Second
	// AssertionClockSkew is the private_key_jwt assertion time tolerance.
	AssertionClockSkew = 60 * time.Second
	// MaxTokenBytes is the compact JWS size cap.
	MaxTokenBytes = 8 * 1024
	// HeaderType is the RFC 9068 access-token typ.
	HeaderType = "at+jwt"
	// Algorithm is the only accepted JWS algorithm.
	Algorithm = domain.SigningAlgES256

	maxAudienceBytes     = 128
	maxClientIDBytes     = domain.MaxClientIDLen
	maxScopeBytes        = 256
	maxScopeTokens       = 16
	maxScopeTokenBytes   = 64
	maxHeaderBytes       = 512
	maxPayloadBytes      = 4096
	maxSignatureBytes    = 256
	maxSegmentBytes      = 6144
	jwsSignatureRawLen   = 64
	numericDateMaxUnix   = 4102444800 // 2100-01-01T00:00:00Z
	numericDateMinUnix   = 0
	maxIssuerBytes       = 256
	maxAssertionJTIBytes = 128
	maxJWKSBytes         = 64 * 1024
	maxPublishedKeys     = 16
	maxServiceSubject    = 160
	refreshSecretBytes   = 32
	assertionTyp         = "JWT"
)

// Kind distinguishes fail-closed human and service subject classes.
type Kind string

const (
	// KindHuman is a human account access token.
	KindHuman Kind = "human"
	// KindService is a service-principal subject class. IB2 has no service mint path.
	KindService Kind = "service"
)

var (
	// ErrInvalid is the generic denial for malformed or unauthorized tokens.
	ErrInvalid = errors.New("token invalid")
	// ErrUnavailable is the generic denial when signing or key material cannot be used.
	ErrUnavailable = errors.New("token unavailable")
)

// Clock supplies the mint/verify time. Production uses the real clock.
type Clock interface {
	Now() time.Time
}

// Signer is a public-JWK-aware crypto.Signer. Implementations must not expose
// *ecdsa.PrivateKey to this package.
type Signer interface {
	crypto.Signer
	PublicJWK() (domain.PublicJWK, error)
}

// SignerSource yields the current fenced active signer. Current must honor ctx.
type SignerSource interface {
	Current(ctx context.Context) (Signer, *domain.SigningKey, error)
}

// PublicKeySource looks up verification keys and can force one refresh.
type PublicKeySource interface {
	Lookup(ctx context.Context, kid string) (domain.PublicJWK, bool, error)
	Refresh(ctx context.Context) error
}

// HumanInput is the fail-closed human mint request.
type HumanInput struct {
	Subject           string
	Audience          string
	ClientID          string
	Scope             string
	TTL               time.Duration
	GrantNotAfter     time.Time
	ProviderExpiresAt time.Time
}

// IssuedToken is the signed compact access token plus the exact lifetime used.
type IssuedToken struct {
	Compact   string
	Lifetime  time.Duration
	IssuedAt  time.Time
	NotBefore time.Time
	ExpiresAt time.Time
	JTI       string
	Kid       string
}

const issuedRedacted = "token.IssuedToken{redacted}"

func (IssuedToken) String() string   { return issuedRedacted }
func (IssuedToken) GoString() string { return issuedRedacted }
func (IssuedToken) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, issuedRedacted)
}
func (IssuedToken) MarshalJSON() ([]byte, error) { return nil, denyInvalid() }

// PersistFunc is invoked after signing and before the compact token is returned.
// Implementations persist inside the future consume transaction. A non-nil error
// discards the signed token.
type PersistFunc func(context.Context, IssuedToken) error

// Principal is the safe, copied verify result. It never retains the raw token.
type Principal struct {
	Kind      Kind
	Issuer    string
	Subject   string
	Audience  string
	IssuedAt  time.Time
	NotBefore time.Time
	ExpiresAt time.Time
	JTI       string
	ClientID  string
	Scope     string
}

const principalRedacted = "token.Principal{redacted}"

func (Principal) String() string   { return principalRedacted }
func (Principal) GoString() string { return principalRedacted }
func (Principal) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, principalRedacted)
}
func (Principal) MarshalJSON() ([]byte, error) { return nil, denyInvalid() }

// ClientRegistration is the verifier's exact registered client view.
type ClientRegistration struct {
	ClientID     string
	Audience     string
	SubjectClass Kind
	Scope        string
}

// ClientLookup resolves the registered public client_id during verify.
type ClientLookup func(ctx context.Context, clientID string) (ClientRegistration, error)

// AssertionInput is the library-level private_key_jwt validator input.
type AssertionInput struct {
	ClientID    string
	Audience    string
	Registered  []domain.PublicJWK
	Now         time.Time
	MaxLifetime time.Duration
}

// ClientAssertion is the validated private_key_jwt result. Durable replay is out of scope.
type ClientAssertion struct {
	ClientID  string
	Subject   string
	Audience  string
	Kid       string
	JTI       string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func resolveClock(clock Clock) Clock {
	if clock == nil {
		return realClock{}
	}
	return clock
}

func denyInvalid() error     { return ErrInvalid }
func denyUnavailable() error { return ErrUnavailable }
