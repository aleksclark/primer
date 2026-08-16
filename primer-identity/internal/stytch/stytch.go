// Package stytch provides Primer's vendor-neutral B2B session boundary and
// its production adapter for the official Stytch v18 SDK.
package stytch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/b2bstytchapi"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/sessions"
	sdkconfig "github.com/stytchauth/stytch-go/v18/stytch/config"
	"github.com/stytchauth/stytch-go/v18/stytch/stytcherror"
)

const (
	// MaxSessionRoles prevents provider-controlled role data from becoming an
	// unbounded internal response.
	MaxSessionRoles = 64

	maxProviderResponseBodyBytes = 1 << 20
	maxSnapshotIDRunes           = 255
	maxSnapshotIDBytes           = 1024
	maxRoleRunes                 = 128
	maxRoleBytes                 = 512
	maxAggregateRoleBytes        = 8192
	// Opaque session tokens are bounded before any SDK/HTTP work so an
	// oversized bearer value cannot become an outbound request.
	maxSessionTokenBytes      = 4096
	maxReturnedMemberSessions = 256
	revalidationDeadline      = 2 * time.Second
)

var (
	ErrDisabled             = errors.New("stytch is disabled")
	ErrEmptyToken           = errors.New("stytch session token is empty")
	ErrSessionTokenTooLarge = errors.New("stytch session token exceeds maximum size")
	ErrDefinitive           = errors.New("stytch session is definitively invalid")
	// ErrProviderUnavailable is a non-oracular transient, malformed, or
	// fail-closed provider result and must never be negative-cached.
	ErrProviderUnavailable = errors.New("stytch provider is unavailable")
)

// StytchSessionSnapshot is the only provider data allowed past this package.
// It intentionally excludes email, profile, claims, JWTs, and opaque tokens.
type StytchSessionSnapshot struct {
	ProjectID               string    `json:"project_id"`
	OrganizationID          string    `json:"organization_id"`
	MemberID                string    `json:"member_id"`
	ProviderMemberSessionID string    `json:"provider_member_session_id"`
	Active                  bool      `json:"active"`
	Eligible                bool      `json:"eligible"`
	ExpiresAt               time.Time `json:"expires_at"`
	Roles                   []string  `json:"roles"`
}

// SessionSnapshot is the concise name used by callers that do not need the
// provider-prefixed type name.
type SessionSnapshot = StytchSessionSnapshot

// StytchClient is the vendor-neutral session boundary used by Identity.
type StytchClient interface {
	AuthenticateSession(context.Context, string) (StytchSessionSnapshot, error)
	InvalidateSession(context.Context, string) error
}

// MemberSessionRevalidator is the token-free provider proof boundary. It never
// exposes provider payloads, tokens, or SDK response types.
type MemberSessionRevalidator interface {
	RevalidateMemberSession(context.Context, string, string, string, string) (SessionSnapshot, error)
}

// DefinitiveSessionError identifies only an official provider error that proves
// the presented session is invalid, revoked, or expired. Other failures remain
// transient so callers do not negative-cache outages.
type DefinitiveSessionError struct {
	Kind      string
	RequestID string
}

func (e *DefinitiveSessionError) Error() string {
	return "stytch session is definitively invalid"
}

func (e *DefinitiveSessionError) Unwrap() error { return ErrDefinitive }

// Adapter wraps the official SDK without exposing SDK response types.
type Adapter struct {
	api       *b2bstytchapi.API
	projectID string
}

// New constructs the official adapter, or returns (nil, nil) when explicitly
// disabled. Disabled mode never constructs an SDK client.
func New(cfg config.StytchConfig) (StytchClient, error) {
	return newAdapter(cfg, nil)
}

// NewWithHTTPClient is the testable constructor. The injected transport is
// cloned and bounded by the same timeout/no-redirect policy as production.
func NewWithHTTPClient(cfg config.StytchConfig, injected *http.Client) (StytchClient, error) {
	return newAdapter(cfg, injected)
}

func newAdapter(cfg config.StytchConfig, injected *http.Client) (*Adapter, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	httpClient := boundedHTTPClient(cfg.RequestTimeout, injected)
	baseURI := cfg.BaseURI
	if baseURI == "" {
		if cfg.Env == "live" {
			baseURI = string(sdkconfig.BaseURILive)
		} else {
			baseURI = string(sdkconfig.BaseURITest)
		}
	}
	api, err := b2bstytchapi.NewClient(
		cfg.ProjectID,
		cfg.Secret,
		b2bstytchapi.WithBaseURI(baseURI),
		b2bstytchapi.WithHTTPClient(httpClient),
		b2bstytchapi.WithSkipJWKSInitialization(),
	)
	if err != nil {
		return nil, errors.New("construct stytch client failed")
	}
	return &Adapter{api: api, projectID: cfg.ProjectID}, nil
}

func boundedHTTPClient(timeout time.Duration, injected *http.Client) *http.Client {
	var client http.Client
	if injected != nil {
		client = *injected
	}
	client.Timeout = timeout
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = responseBodyLimitTransport{base: transport}
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client
}

type responseBodyLimitTransport struct {
	base http.RoundTripper
}

func (t responseBodyLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(req)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	response.Body = &limitedResponseBody{
		body:      response.Body,
		remaining: maxProviderResponseBodyBytes,
	}
	return response, nil
}

var errProviderResponseBodyTooLarge = errors.New("stytch response body exceeds maximum size")
var errProviderResponseInvalidUTF8 = errors.New("stytch response body is not valid utf-8")

type limitedResponseBody struct {
	body          io.ReadCloser
	remaining     int64
	tooLarge      bool
	invalidUTF8   bool
	utf8Remainder []byte
}

func (b *limitedResponseBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.tooLarge {
		return 0, errProviderResponseBodyTooLarge
	}
	if b.invalidUTF8 {
		return 0, errProviderResponseInvalidUTF8
	}
	if b.remaining == 0 {
		var probe [1]byte
		n, err := b.body.Read(probe[:])
		if n > 0 {
			b.tooLarge = true
			return 0, errProviderResponseBodyTooLarge
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(b.utf8Remainder) != 0 {
				b.invalidUTF8 = true
				return 0, errProviderResponseInvalidUTF8
			}
			return 0, err
		}
		return 0, io.EOF
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.body.Read(p)
	b.remaining -= int64(n)
	if utf8Err := b.validateUTF8(p[:n]); utf8Err != nil {
		b.invalidUTF8 = true
		return n, utf8Err
	}
	if errors.Is(err, io.EOF) && len(b.utf8Remainder) != 0 {
		b.invalidUTF8 = true
		return n, errProviderResponseInvalidUTF8
	}
	return n, err
}

func (b *limitedResponseBody) Close() error { return b.body.Close() }

func (b *limitedResponseBody) validateUTF8(chunk []byte) error {
	data := make([]byte, 0, len(b.utf8Remainder)+len(chunk))
	data = append(data, b.utf8Remainder...)
	data = append(data, chunk...)
	b.utf8Remainder = nil
	for offset := 0; offset < len(data); {
		r, size := utf8.DecodeRune(data[offset:])
		if r == utf8.RuneError && size == 1 {
			if !utf8.FullRune(data[offset:]) {
				b.utf8Remainder = append([]byte(nil), data[offset:]...)
				return nil
			}
			return errProviderResponseInvalidUTF8
		}
		offset += size
	}
	return nil
}

// AuthenticateSession sends only the opaque token and maps the bounded
// MemberSession fields. The authenticate request omits session_duration_minutes
// so authenticating cannot extend the provider session.
func (a *Adapter) AuthenticateSession(ctx context.Context, token string) (StytchSessionSnapshot, error) {
	if err := validateSessionToken(token); err != nil {
		return StytchSessionSnapshot{}, err
	}
	response, err := a.api.Sessions.Authenticate(ctx, &sessions.AuthenticateParams{
		SessionToken: token,
	})
	if err != nil {
		return StytchSessionSnapshot{}, classifyError(err, "authenticate")
	}
	if response == nil {
		return StytchSessionSnapshot{}, errors.New("stytch session provider returned no response")
	}

	session := response.MemberSession
	snapshot := StytchSessionSnapshot{
		ProjectID:               a.projectID,
		OrganizationID:          session.OrganizationID,
		MemberID:                session.MemberID,
		ProviderMemberSessionID: session.MemberSessionID,
		Roles:                   append([]string(nil), session.Roles...),
	}
	if session.ExpiresAt != nil {
		snapshot.ExpiresAt = session.ExpiresAt.UTC()
		snapshot.Active = time.Now().UTC().Before(snapshot.ExpiresAt)
		snapshot.Eligible = snapshot.Active && snapshot.OrganizationID != "" && snapshot.MemberID != ""
	}
	if !validSnapshot(snapshot) {
		return StytchSessionSnapshot{}, errors.New("stytch session provider returned invalid snapshot")
	}
	return snapshot, nil
}

// InvalidateSession revokes the opaque provider session without exposing the
// provider response to callers.
func (a *Adapter) InvalidateSession(ctx context.Context, token string) error {
	if err := validateSessionToken(token); err != nil {
		return err
	}
	_, err := a.api.Sessions.Revoke(ctx, &sessions.RevokeParams{SessionToken: token})
	if err != nil {
		return classifyError(err, "revoke")
	}
	return nil
}

// RevalidateMemberSession makes exactly one official unpaginated Sessions.Get
// call. Any malformed result or tuple inconsistency is unavailable rather than
// a denial, so provider faults cannot become cached negative authority.
func (a *Adapter) RevalidateMemberSession(ctx context.Context, exactProjectID, exactOrganizationID, exactMemberID, exactMemberSessionID string) (SessionSnapshot, error) {
	if a == nil || exactProjectID != a.projectID || !validSnapshotText(exactProjectID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) || !validSnapshotText(exactOrganizationID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) || !validSnapshotText(exactMemberID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) || !validSnapshotText(exactMemberSessionID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) {
		return SessionSnapshot{}, ErrProviderUnavailable
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, revalidationDeadline)
	defer cancel()
	response, err := a.api.Sessions.Get(deadlineCtx, &sessions.GetParams{OrganizationID: exactOrganizationID, MemberID: exactMemberID})
	if err != nil || deadlineCtx.Err() != nil || response == nil {
		return SessionSnapshot{}, ErrProviderUnavailable
	}
	if len(response.MemberSessions) > maxReturnedMemberSessions {
		return SessionSnapshot{}, ErrProviderUnavailable
	}
	now := time.Now().UTC()
	seen := make(map[string]struct{}, len(response.MemberSessions))
	var matched *sessions.MemberSession
	for i := range response.MemberSessions {
		s := &response.MemberSessions[i]
		if !validSnapshotText(s.MemberSessionID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) || s.ExpiresAt == nil || !validSnapshotText(s.OrganizationID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) || !validSnapshotText(s.MemberID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) {
			return SessionSnapshot{}, ErrProviderUnavailable
		}
		if _, duplicate := seen[s.MemberSessionID]; duplicate {
			return SessionSnapshot{}, ErrProviderUnavailable
		}
		seen[s.MemberSessionID] = struct{}{}
		if s.OrganizationID != exactOrganizationID || s.MemberID != exactMemberID {
			return SessionSnapshot{}, ErrProviderUnavailable
		}
		if s.MemberSessionID == exactMemberSessionID {
			copy := *s
			matched = &copy
		}
	}
	if matched == nil || !matched.ExpiresAt.UTC().After(now) {
		return SessionSnapshot{}, ErrDefinitive
	}
	return SessionSnapshot{ProjectID: exactProjectID, OrganizationID: exactOrganizationID, MemberID: exactMemberID, ProviderMemberSessionID: exactMemberSessionID, Active: true, Eligible: true, ExpiresAt: matched.ExpiresAt.UTC()}, nil
}

func validateSessionToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrEmptyToken
	}
	if len(token) > maxSessionTokenBytes {
		return ErrSessionTokenTooLarge
	}
	return nil
}

func validSnapshot(snapshot StytchSessionSnapshot) bool {
	if !validSnapshotText(snapshot.ProjectID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) ||
		!validSnapshotText(snapshot.OrganizationID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) ||
		!validSnapshotText(snapshot.MemberID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) ||
		!validSnapshotText(snapshot.ProviderMemberSessionID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) ||
		len(snapshot.Roles) > MaxSessionRoles {
		return false
	}

	roleBytes := 0
	for _, role := range snapshot.Roles {
		if !validSnapshotText(role, maxRoleRunes, maxRoleBytes, true) {
			return false
		}
		roleBytes += len(role)
		if roleBytes > maxAggregateRoleBytes {
			return false
		}
	}
	return true
}

func validSnapshotText(value string, maxRunes, maxBytes int, allowEmpty bool) bool {
	if (!allowEmpty && value == "") || !utf8.ValidString(value) || len(value) > maxBytes || utf8.RuneCountInString(value) > maxRunes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func classifyError(err error, operation string) error {
	providerErr, ok := providerError(err)
	if ok && providerErr.StatusCode >= 400 && providerErr.StatusCode < 500 && providerErr.StatusCode != http.StatusTooManyRequests && definitiveKinds[string(providerErr.ErrorType)] {
		return &DefinitiveSessionError{
			Kind:      string(providerErr.ErrorType),
			RequestID: providerErr.RequestID,
		}
	}
	return fmt.Errorf("stytch session %s failed", operation)
}

func providerError(err error) (stytcherror.Error, bool) {
	var value stytcherror.Error
	if errors.As(err, &value) {
		return value, true
	}
	var pointer *stytcherror.Error
	if errors.As(err, &pointer) && pointer != nil {
		return *pointer, true
	}
	return stytcherror.Error{}, false
}

var definitiveKinds = map[string]bool{
	"invalid_session_token":   true,
	"session_token_invalid":   true,
	"session_jwt_invalid":     true,
	"session_not_found":       true,
	"session_token_not_found": true,
	"session_revoked":         true,
	"session_expired":         true,
	"session_jwt_expired":     true,
}
