// Package brokerprovider is Primer Identity's vendor-neutral broker provider
// boundary plus a credential-free scripted implementation used by IB1.
//
// Nothing in this package performs a live provider call, reads credentials, or
// touches the network. The facade deliberately returns only a bounded identity
// tuple snapshot: provider session tokens, session JWTs, raw provider payloads,
// email join keys, and provider roles never cross this boundary.
package brokerprovider

import (
	"context"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Method is a supported human authentication method.
type Method string

// The exact IB1 human methods. Local passwords are not a broker method.
const (
	MethodEmailMagicLink Method = "email_magic_link"
	MethodEmailOTP       Method = "email_otp"
	MethodSSOSAML        Method = "sso_saml"
	MethodSSOOIDC        Method = "sso_oidc"
)

// Outcome classifies a callback result.
type Outcome string

const (
	// OutcomeAuthenticated is the only outcome that carries a member session.
	OutcomeAuthenticated Outcome = "authenticated"
	// OutcomeIncompleteMFA means the human must finish MFA. It is Identity-only
	// and never yields product authority.
	OutcomeIncompleteMFA Outcome = "incomplete_mfa"
	// OutcomeDenied is a definitive provider denial.
	OutcomeDenied Outcome = "denied"
	// OutcomeUnavailable is a transient provider fault and is never cached as
	// negative authority.
	OutcomeUnavailable Outcome = "unavailable"
)

// Sentinel errors are non-oracular: they never echo an artifact, tuple value,
// or provider payload.
var (
	// ErrDefinitiveDenial is an invalid, unknown, replayed, revoked, or expired
	// callback artifact.
	ErrDefinitiveDenial = errors.New("broker provider definitively denied the callback")
	// ErrProviderUnavailable is a transient provider fault.
	ErrProviderUnavailable = errors.New("broker provider is unavailable")
	// ErrUnsupportedMethod rejects a method outside the registered set.
	ErrUnsupportedMethod = errors.New("broker provider method is unsupported")
	// ErrInvalidFixture rejects a malformed credential-free script.
	ErrInvalidFixture = errors.New("broker provider fixture is invalid")
)

// Bounds mirror domain.ValidateProviderMemberSessionID so a scripted value can
// never be accepted here and then rejected at persistence.
const (
	maxIDRunes = 255
	maxIDBytes = 1024
)

// ArtifactType names the one-time callback artifact the provider may consume.
// Unrecognized types are a definitive denial and create no outbound call.
type ArtifactType string

const (
	ArtifactTypeMagicLink          ArtifactType = "magic_link"
	ArtifactTypeDiscoveryMagicLink ArtifactType = "discovery_magic_link"
	ArtifactTypeEmailOTP           ArtifactType = "email_otp"
	ArtifactTypeDiscoveryEmailOTP  ArtifactType = "discovery_email_otp"
	ArtifactTypeSSOToken           ArtifactType = "sso_token"
)

// StartRequest begins one provider authentication attempt.
type StartRequest struct {
	Method Method
	// EmailAddress is required for email magic-link and email OTP start. It is
	// never persisted, logged, or returned.
	EmailAddress string
	// OrganizationID selects an organization-scoped start when already known.
	OrganizationID string
	// ConnectionID selects a registered SAML/OIDC connection for SSO start.
	ConnectionID string
}

// StartResult is the bounded, token-free result of beginning authentication.
type StartResult struct {
	Method Method
	// Handle correlates the attempt for logs/tests. It is not an authority
	// bearer and is never accepted in place of a callback artifact.
	Handle string
	// ContinueURL is an optional browser continuation (SSO public start). It
	// is never an authority bearer and is never accepted as a callback artifact.
	ContinueURL string
}

// CallbackRequest consumes one recognized one-time callback artifact.
type CallbackRequest struct {
	Type ArtifactType
	// Artifact is the server-side one-time token or OTP code. It is consumed
	// in memory and never returned or stored.
	Artifact string
	// EmailAddress is required to verify an email OTP. It is never returned.
	EmailAddress string
	// OrganizationID selects organization-scoped OTP or magic-link completion.
	OrganizationID string
}

// CallbackResult is the only provider data allowed past this boundary. It
// deliberately excludes session tokens, session JWTs, raw provider payloads,
// email addresses, and provider roles.
type CallbackResult struct {
	Method                 Method
	Outcome                Outcome
	ProjectID              string
	OrganizationID         string
	MemberID               string
	MemberSessionID        string
	MemberSessionExpiresAt time.Time
}

// Authenticated reports whether the result carries a usable member session.
func (r CallbackResult) Authenticated() bool { return r.Outcome == OutcomeAuthenticated }

// Provider is the vendor-neutral broker provider facade.
type Provider interface {
	// StartLogin begins a provider authentication attempt for one method.
	StartLogin(ctx context.Context, req StartRequest) (StartResult, error)
	// CompleteCallback consumes a one-time callback artifact in memory and
	// returns only a bounded identity tuple snapshot. Production adapters
	// require a recognized ArtifactType via CompleteTypedCallback.
	CompleteCallback(ctx context.Context, artifact string) (CallbackResult, error)
}

// TypedProvider is the production callback boundary. Scripted tests may keep
// using CompleteCallback; live adapters must reject an unrecognized type.
type TypedProvider interface {
	Provider
	CompleteTypedCallback(ctx context.Context, req CallbackRequest) (CallbackResult, error)
}

// Fixture scripts one credential-free callback artifact.
type Fixture struct {
	Artifact        string
	Method          Method
	Outcome         Outcome
	ProjectID       string
	OrganizationID  string
	MemberID        string
	MemberSessionID string
	ExpiresAt       time.Time
}

// ScriptConfig configures a ScriptedProvider. It holds no credentials.
type ScriptConfig struct {
	Now      func() time.Time
	Fixtures []Fixture
}

// ScriptedProvider is a deterministic, credential-free Provider. Artifacts are
// single-use and consumption is safe for concurrent callers.
type ScriptedProvider struct {
	now       func() time.Time
	mu        sync.Mutex
	fixtures  map[string]Fixture
	consumed  map[string]struct{}
	supported map[Method]struct{}
}

var (
	_ Provider      = (*ScriptedProvider)(nil)
	_ TypedProvider = (*ScriptedProvider)(nil)
)

// NewScripted validates every fixture and fails closed on malformed input.
func NewScripted(cfg ScriptConfig) (*ScriptedProvider, error) {
	if len(cfg.Fixtures) == 0 {
		return nil, ErrInvalidFixture
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	fixtures := make(map[string]Fixture, len(cfg.Fixtures))
	for _, fixture := range cfg.Fixtures {
		if err := validateFixture(fixture); err != nil {
			return nil, err
		}
		if _, duplicate := fixtures[fixture.Artifact]; duplicate {
			return nil, ErrInvalidFixture
		}
		fixtures[fixture.Artifact] = fixture
	}
	return &ScriptedProvider{
		now:      now,
		fixtures: fixtures,
		consumed: make(map[string]struct{}, len(fixtures)),
		supported: map[Method]struct{}{
			MethodEmailMagicLink: {}, MethodEmailOTP: {},
			MethodSSOSAML: {}, MethodSSOOIDC: {},
		},
	}, nil
}

// StartLogin validates the requested method without contacting a provider.
func (p *ScriptedProvider) StartLogin(ctx context.Context, req StartRequest) (StartResult, error) {
	if err := ctx.Err(); err != nil {
		return StartResult{}, ErrProviderUnavailable
	}
	if _, ok := p.supported[req.Method]; !ok {
		return StartResult{}, ErrUnsupportedMethod
	}
	return StartResult{Method: req.Method, Handle: uuid.NewString()}, nil
}

// CompleteTypedCallback consumes a recognized one-time artifact. An unknown
// type is a definitive denial and does not consult fixtures.
func (p *ScriptedProvider) CompleteTypedCallback(ctx context.Context, req CallbackRequest) (CallbackResult, error) {
	switch req.Type {
	case ArtifactTypeMagicLink, ArtifactTypeDiscoveryMagicLink, ArtifactTypeEmailOTP, ArtifactTypeDiscoveryEmailOTP, ArtifactTypeSSOToken:
		return p.CompleteCallback(ctx, req.Artifact)
	default:
		return CallbackResult{}, ErrDefinitiveDenial
	}
}

// CompleteCallback consumes the artifact exactly once. A transient fault does
// not consume it, so an outage cannot destroy a recoverable login.
func (p *ScriptedProvider) CompleteCallback(ctx context.Context, artifact string) (CallbackResult, error) {
	if err := ctx.Err(); err != nil {
		return CallbackResult{}, ErrProviderUnavailable
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	fixture, known := p.fixtures[artifact]
	if !known {
		return CallbackResult{}, ErrDefinitiveDenial
	}
	if fixture.Outcome == OutcomeUnavailable {
		return CallbackResult{}, ErrProviderUnavailable
	}
	if _, spent := p.consumed[artifact]; spent {
		return CallbackResult{}, ErrDefinitiveDenial
	}
	p.consumed[artifact] = struct{}{}

	switch fixture.Outcome {
	case OutcomeDenied:
		return CallbackResult{}, ErrDefinitiveDenial
	case OutcomeIncompleteMFA:
		return CallbackResult{
			Method: fixture.Method, Outcome: OutcomeIncompleteMFA,
			ProjectID: fixture.ProjectID, OrganizationID: fixture.OrganizationID,
			MemberID: fixture.MemberID,
		}, nil
	case OutcomeAuthenticated:
		if !fixture.ExpiresAt.After(p.now()) {
			return CallbackResult{}, ErrDefinitiveDenial
		}
		return CallbackResult{
			Method: fixture.Method, Outcome: OutcomeAuthenticated,
			ProjectID: fixture.ProjectID, OrganizationID: fixture.OrganizationID,
			MemberID: fixture.MemberID, MemberSessionID: fixture.MemberSessionID,
			MemberSessionExpiresAt: fixture.ExpiresAt.UTC(),
		}, nil
	default:
		return CallbackResult{}, ErrDefinitiveDenial
	}
}

func validateFixture(f Fixture) error {
	if !validBoundedID(f.Artifact) {
		return ErrInvalidFixture
	}
	switch f.Method {
	case MethodEmailMagicLink, MethodEmailOTP, MethodSSOSAML, MethodSSOOIDC:
	default:
		return ErrInvalidFixture
	}
	switch f.Outcome {
	case OutcomeDenied, OutcomeUnavailable:
		// Failure fixtures carry no tuple requirement.
		return nil
	case OutcomeAuthenticated, OutcomeIncompleteMFA:
	default:
		return ErrInvalidFixture
	}
	if !validBoundedID(f.ProjectID) || !validBoundedID(f.OrganizationID) || !validBoundedID(f.MemberID) {
		return ErrInvalidFixture
	}
	if f.Outcome == OutcomeAuthenticated {
		if !validBoundedID(f.MemberSessionID) || f.ExpiresAt.IsZero() {
			return ErrInvalidFixture
		}
	}
	return nil
}

func validBoundedID(value string) bool {
	if value == "" || !utf8.ValidString(value) ||
		len(value) > maxIDBytes || utf8.RuneCountInString(value) > maxIDRunes {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
