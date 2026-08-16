package stytch

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/discovery"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/discovery/intermediatesessions"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/magiclinks"
	magicdiscovery "github.com/stytchauth/stytch-go/v18/stytch/b2b/magiclinks/discovery"
	emaildiscovery "github.com/stytchauth/stytch-go/v18/stytch/b2b/magiclinks/email/discovery"
	otpemail "github.com/stytchauth/stytch-go/v18/stytch/b2b/otp/email"
	emaildiscoveryotp "github.com/stytchauth/stytch-go/v18/stytch/b2b/otp/email/discovery"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/sso"
	sdkconfig "github.com/stytchauth/stytch-go/v18/stytch/config"

	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/config"
)

const (
	maxEmailBytes    = 254
	maxArtifactBytes = 4096
	maxOTPRunes      = 16
	minOTPRunes      = 4
	maxPublicToken   = 256
)

// BrokerConfig injects the official adapter settings plus exact HTTPS
// callback/redirect URLs. Production forbids endpoint override.
type BrokerConfig struct {
	Stytch               config.StytchConfig
	DiscoveryRedirectURL string
	LoginRedirectURL     string
	SignupRedirectURL    string
	PublicToken          string
}

// Broker is the official v18.1.0-backed brokerprovider.Provider. It never
// exposes SDK types or raw provider session material.
type Broker struct {
	adapter *Adapter
	cfg     BrokerConfig
}

var (
	_ brokerprovider.Provider      = (*Broker)(nil)
	_ brokerprovider.TypedProvider = (*Broker)(nil)
)

// NewBroker constructs the production broker with the shared bounded HTTP client.
func NewBroker(cfg BrokerConfig) (*Broker, error) {
	return NewBrokerWithHTTPClient(cfg, nil)
}

// NewBrokerWithHTTPClient is the testable constructor. It reuses the existing
// timeout/no-redirect/1MiB UTF-8 client and does not create a second one.
func NewBrokerWithHTTPClient(cfg BrokerConfig, injected *http.Client) (*Broker, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	adapter, err := newAdapter(cfg.Stytch, injected)
	if err != nil {
		return nil, err
	}
	if adapter == nil {
		return nil, errors.New("stytch broker requires an enabled provider")
	}
	return &Broker{adapter: adapter, cfg: cfg}, nil
}

func (c BrokerConfig) validate() error {
	if !c.Stytch.Enabled {
		return errors.New("stytch broker requires an enabled provider")
	}
	if err := c.Stytch.Validate(); err != nil {
		return err
	}
	if err := validatePinnedHTTPSURL(c.DiscoveryRedirectURL); err != nil {
		return err
	}
	if err := validatePinnedHTTPSURL(c.LoginRedirectURL); err != nil {
		return err
	}
	if err := validatePinnedHTTPSURL(c.SignupRedirectURL); err != nil {
		return err
	}
	if !validPublicToken(c.PublicToken) {
		return errors.New("stytch broker public token is invalid")
	}
	return nil
}

func validatePinnedHTTPSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || !strings.EqualFold(u.Scheme, "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.ContainsAny(u.Path, "?#") || !utf8.ValidString(raw) {
		return errors.New("stytch broker redirect URL must be an exact https URL")
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return errors.New("stytch broker redirect URL must be an exact https URL")
		}
	}
	return nil
}

func validPublicToken(value string) bool {
	return validSnapshotText(value, maxPublicToken, maxPublicToken, false)
}

// StartLogin begins an allowed ordinary-human method. Email start never
// distinguishes unknown accounts. SSO start is the official public URL
// because v18.1.0 has no server-side SSO Start method.
func (b *Broker) StartLogin(ctx context.Context, req brokerprovider.StartRequest) (brokerprovider.StartResult, error) {
	if err := ctx.Err(); err != nil {
		return brokerprovider.StartResult{}, brokerprovider.ErrProviderUnavailable
	}
	handle := uuid.NewString()
	switch req.Method {
	case brokerprovider.MethodEmailMagicLink:
		if err := validateEmail(req.EmailAddress); err != nil {
			return brokerprovider.StartResult{}, err
		}
		_, err := b.adapter.api.MagicLinks.Email.Discovery.Send(ctx, &emaildiscovery.SendParams{
			EmailAddress:         req.EmailAddress,
			DiscoveryRedirectURL: b.cfg.DiscoveryRedirectURL,
		})
		if err != nil {
			return brokerprovider.StartResult{}, classifyStartError(err)
		}
		return brokerprovider.StartResult{Method: req.Method, Handle: handle}, nil
	case brokerprovider.MethodEmailOTP:
		if err := validateEmail(req.EmailAddress); err != nil {
			return brokerprovider.StartResult{}, err
		}
		_, err := b.adapter.api.OTPs.Email.Discovery.Send(ctx, &emaildiscoveryotp.SendParams{
			EmailAddress: req.EmailAddress,
		})
		if err != nil {
			return brokerprovider.StartResult{}, classifyStartError(err)
		}
		return brokerprovider.StartResult{Method: req.Method, Handle: handle}, nil
	case brokerprovider.MethodSSOSAML, brokerprovider.MethodSSOOIDC:
		continueURL, err := b.publicSSOStartURL(req)
		if err != nil {
			return brokerprovider.StartResult{}, err
		}
		return brokerprovider.StartResult{Method: req.Method, Handle: handle, ContinueURL: continueURL}, nil
	default:
		return brokerprovider.StartResult{}, brokerprovider.ErrUnsupportedMethod
	}
}

func (b *Broker) publicSSOStartURL(req brokerprovider.StartRequest) (string, error) {
	if req.ConnectionID == "" && req.OrganizationID == "" {
		return "", brokerprovider.ErrDefinitiveDenial
	}
	if req.ConnectionID != "" && !validSnapshotText(req.ConnectionID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) {
		return "", brokerprovider.ErrDefinitiveDenial
	}
	if req.OrganizationID != "" && !validSnapshotText(req.OrganizationID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) {
		return "", brokerprovider.ErrDefinitiveDenial
	}
	base := strings.TrimRight(b.providerBaseURI(), "/")
	u, err := url.Parse(base + "/v1/public/sso/start")
	if err != nil {
		return "", brokerprovider.ErrProviderUnavailable
	}
	q := u.Query()
	q.Set("public_token", b.cfg.PublicToken)
	if req.ConnectionID != "" {
		q.Set("connection_id", req.ConnectionID)
	} else {
		q.Set("organization_id", req.OrganizationID)
	}
	q.Set("login_redirect_url", b.cfg.LoginRedirectURL)
	q.Set("signup_redirect_url", b.cfg.SignupRedirectURL)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (b *Broker) providerBaseURI() string {
	if b.cfg.Stytch.BaseURI != "" {
		return b.cfg.Stytch.BaseURI
	}
	if b.cfg.Stytch.Env == "live" {
		return string(sdkconfig.BaseURILive)
	}
	return string(sdkconfig.BaseURITest)
}

// CompleteCallback without a recognized artifact type is a definitive denial
// and creates no outbound call.
func (b *Broker) CompleteCallback(context.Context, string) (brokerprovider.CallbackResult, error) {
	return brokerprovider.CallbackResult{}, brokerprovider.ErrDefinitiveDenial
}

// CompleteTypedCallback consumes one recognized one-time artifact, then
// produces a fresh member-session snapshot through the existing authenticate
// and unpaginated Sessions.Get revalidation path.
func (b *Broker) CompleteTypedCallback(ctx context.Context, req brokerprovider.CallbackRequest) (brokerprovider.CallbackResult, error) {
	if err := ctx.Err(); err != nil {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrProviderUnavailable
	}
	if err := validateCallbackRequest(req); err != nil {
		return brokerprovider.CallbackResult{}, err
	}
	switch req.Type {
	case brokerprovider.ArtifactTypeDiscoveryMagicLink:
		return b.completeDiscoveryMagicLink(ctx, req.Artifact)
	case brokerprovider.ArtifactTypeMagicLink:
		return b.completeOrgMagicLink(ctx, req.Artifact)
	case brokerprovider.ArtifactTypeDiscoveryEmailOTP:
		return b.completeDiscoveryEmailOTP(ctx, req)
	case brokerprovider.ArtifactTypeEmailOTP:
		return b.completeOrgEmailOTP(ctx, req)
	case brokerprovider.ArtifactTypeSSOToken:
		return b.completeSSO(ctx, req.Artifact)
	default:
		return brokerprovider.CallbackResult{}, brokerprovider.ErrDefinitiveDenial
	}
}

func (b *Broker) completeDiscoveryMagicLink(ctx context.Context, artifact string) (brokerprovider.CallbackResult, error) {
	resp, err := b.adapter.api.MagicLinks.Discovery.Authenticate(ctx, &magicdiscovery.AuthenticateParams{
		DiscoveryMagicLinksToken: artifact,
	})
	if err != nil {
		return brokerprovider.CallbackResult{}, classifyCallbackError(err)
	}
	if resp == nil {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrProviderUnavailable
	}
	return b.finishDiscovery(ctx, brokerprovider.MethodEmailMagicLink, resp.IntermediateSessionToken, resp.DiscoveredOrganizations, "")
}

func (b *Broker) completeOrgMagicLink(ctx context.Context, artifact string) (brokerprovider.CallbackResult, error) {
	resp, err := b.adapter.api.MagicLinks.Authenticate(ctx, &magiclinks.AuthenticateParams{
		MagicLinksToken: artifact,
	})
	if err != nil {
		return brokerprovider.CallbackResult{}, classifyCallbackError(err)
	}
	if resp == nil {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrProviderUnavailable
	}
	return b.finishMemberAuth(ctx, brokerprovider.MethodEmailMagicLink, memberAuth{
		authenticated:  resp.MemberAuthenticated,
		memberID:       firstNonEmpty(resp.MemberID, resp.Member.MemberID),
		organizationID: firstNonEmpty(resp.OrganizationID, resp.Organization.OrganizationID, resp.Member.OrganizationID),
		sessionToken:   resp.SessionToken,
		intermediate:   resp.IntermediateSessionToken,
	})
}

func (b *Broker) completeDiscoveryEmailOTP(ctx context.Context, req brokerprovider.CallbackRequest) (brokerprovider.CallbackResult, error) {
	resp, err := b.adapter.api.OTPs.Email.Discovery.Authenticate(ctx, &emaildiscoveryotp.AuthenticateParams{
		EmailAddress: req.EmailAddress,
		Code:         req.Artifact,
	})
	if err != nil {
		return brokerprovider.CallbackResult{}, classifyCallbackError(err)
	}
	if resp == nil {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrProviderUnavailable
	}
	return b.finishDiscovery(ctx, brokerprovider.MethodEmailOTP, resp.IntermediateSessionToken, resp.DiscoveredOrganizations, req.OrganizationID)
}

func (b *Broker) completeOrgEmailOTP(ctx context.Context, req brokerprovider.CallbackRequest) (brokerprovider.CallbackResult, error) {
	resp, err := b.adapter.api.OTPs.Email.Authenticate(ctx, &otpemail.AuthenticateParams{
		OrganizationID: req.OrganizationID,
		EmailAddress:   req.EmailAddress,
		Code:           req.Artifact,
	})
	if err != nil {
		return brokerprovider.CallbackResult{}, classifyCallbackError(err)
	}
	if resp == nil {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrProviderUnavailable
	}
	return b.finishMemberAuth(ctx, brokerprovider.MethodEmailOTP, memberAuth{
		authenticated:  resp.MemberAuthenticated,
		memberID:       firstNonEmpty(resp.MemberID, resp.Member.MemberID),
		organizationID: firstNonEmpty(resp.OrganizationID, resp.Organization.OrganizationID, resp.Member.OrganizationID),
		sessionToken:   resp.SessionToken,
		intermediate:   resp.IntermediateSessionToken,
	})
}

func (b *Broker) completeSSO(ctx context.Context, artifact string) (brokerprovider.CallbackResult, error) {
	resp, err := b.adapter.api.SSO.Authenticate(ctx, &sso.AuthenticateParams{
		SSOToken: artifact,
	})
	if err != nil {
		return brokerprovider.CallbackResult{}, classifyCallbackError(err)
	}
	if resp == nil {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrProviderUnavailable
	}
	return b.finishMemberAuth(ctx, brokerprovider.MethodSSOSAML, memberAuth{
		authenticated:  resp.MemberAuthenticated,
		memberID:       firstNonEmpty(resp.MemberID, resp.Member.MemberID),
		organizationID: firstNonEmpty(resp.OrganizationID, resp.Organization.OrganizationID, resp.Member.OrganizationID),
		sessionToken:   resp.SessionToken,
		intermediate:   resp.IntermediateSessionToken,
	})
}

func (b *Broker) finishDiscovery(ctx context.Context, method brokerprovider.Method, intermediate string, orgs []discovery.DiscoveredOrganization, wantOrg string) (brokerprovider.CallbackResult, error) {
	if !validArtifact(intermediate) {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrDefinitiveDenial
	}
	orgID, memberID, ok := selectDiscoveredMembership(orgs, wantOrg)
	if !ok {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrDefinitiveDenial
	}
	resp, err := b.adapter.api.Discovery.IntermediateSessions.Exchange(ctx, &intermediatesessions.ExchangeParams{
		IntermediateSessionToken: intermediate,
		OrganizationID:           orgID,
	})
	if err != nil {
		return brokerprovider.CallbackResult{}, classifyCallbackError(err)
	}
	if resp == nil {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrProviderUnavailable
	}
	return b.finishMemberAuth(ctx, method, memberAuth{
		authenticated:  resp.MemberAuthenticated,
		memberID:       firstNonEmpty(resp.MemberID, resp.Member.MemberID, memberID),
		organizationID: firstNonEmpty(resp.Organization.OrganizationID, orgID),
		sessionToken:   resp.SessionToken,
		intermediate:   resp.IntermediateSessionToken,
	})
}

type memberAuth struct {
	authenticated  bool
	memberID       string
	organizationID string
	sessionToken   string
	intermediate   string
}

func (b *Broker) finishMemberAuth(ctx context.Context, method brokerprovider.Method, auth memberAuth) (brokerprovider.CallbackResult, error) {
	if !auth.authenticated {
		if !validSnapshotText(auth.organizationID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) ||
			!validSnapshotText(auth.memberID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) {
			return brokerprovider.CallbackResult{}, brokerprovider.ErrDefinitiveDenial
		}
		return brokerprovider.CallbackResult{
			Method:         method,
			Outcome:        brokerprovider.OutcomeIncompleteMFA,
			ProjectID:      b.adapter.projectID,
			OrganizationID: auth.organizationID,
			MemberID:       auth.memberID,
		}, nil
	}
	if !validArtifact(auth.sessionToken) {
		return brokerprovider.CallbackResult{}, brokerprovider.ErrProviderUnavailable
	}
	return b.proveSession(ctx, method, auth.sessionToken)
}

func (b *Broker) proveSession(ctx context.Context, method brokerprovider.Method, sessionToken string) (brokerprovider.CallbackResult, error) {
	snapshot, err := b.adapter.AuthenticateSession(ctx, sessionToken)
	if err != nil {
		return brokerprovider.CallbackResult{}, mapSessionError(err)
	}
	proved, err := b.adapter.RevalidateMemberSession(ctx, snapshot.ProjectID, snapshot.OrganizationID, snapshot.MemberID, snapshot.ProviderMemberSessionID)
	if err != nil {
		return brokerprovider.CallbackResult{}, mapSessionError(err)
	}
	return brokerprovider.CallbackResult{
		Method:                 method,
		Outcome:                brokerprovider.OutcomeAuthenticated,
		ProjectID:              proved.ProjectID,
		OrganizationID:         proved.OrganizationID,
		MemberID:               proved.MemberID,
		MemberSessionID:        proved.ProviderMemberSessionID,
		MemberSessionExpiresAt: proved.ExpiresAt,
	}, nil
}

func selectDiscoveredMembership(orgs []discovery.DiscoveredOrganization, wantOrg string) (orgID, memberID string, ok bool) {
	for _, item := range orgs {
		if item.Organization == nil || item.Membership == nil || item.Membership.Member == nil {
			continue
		}
		switch item.Membership.Type {
		case "active_member", "pending_member", "invited_member", "":
		default:
			continue
		}
		candidateOrg := firstNonEmpty(item.Organization.OrganizationID, item.Membership.Member.OrganizationID)
		candidateMember := item.Membership.Member.MemberID
		if !validSnapshotText(candidateOrg, maxSnapshotIDRunes, maxSnapshotIDBytes, false) ||
			!validSnapshotText(candidateMember, maxSnapshotIDRunes, maxSnapshotIDBytes, false) {
			continue
		}
		if wantOrg != "" && candidateOrg != wantOrg {
			continue
		}
		return candidateOrg, candidateMember, true
	}
	return "", "", false
}

func validateCallbackRequest(req brokerprovider.CallbackRequest) error {
	switch req.Type {
	case brokerprovider.ArtifactTypeMagicLink, brokerprovider.ArtifactTypeDiscoveryMagicLink, brokerprovider.ArtifactTypeSSOToken:
		if !validArtifact(req.Artifact) {
			return brokerprovider.ErrDefinitiveDenial
		}
	case brokerprovider.ArtifactTypeDiscoveryEmailOTP:
		if err := validateEmail(req.EmailAddress); err != nil {
			return err
		}
		if !validOTP(req.Artifact) {
			return brokerprovider.ErrDefinitiveDenial
		}
	case brokerprovider.ArtifactTypeEmailOTP:
		if err := validateEmail(req.EmailAddress); err != nil {
			return err
		}
		if !validOTP(req.Artifact) || !validSnapshotText(req.OrganizationID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) {
			return brokerprovider.ErrDefinitiveDenial
		}
	default:
		return brokerprovider.ErrDefinitiveDenial
	}
	if req.OrganizationID != "" && !validSnapshotText(req.OrganizationID, maxSnapshotIDRunes, maxSnapshotIDBytes, false) {
		return brokerprovider.ErrDefinitiveDenial
	}
	return nil
}

func validateEmail(value string) error {
	if value == "" || !utf8.ValidString(value) || len(value) > maxEmailBytes {
		return brokerprovider.ErrDefinitiveDenial
	}
	at := strings.IndexByte(value, '@')
	if at <= 0 || at != strings.LastIndexByte(value, '@') || at == len(value)-1 {
		return brokerprovider.ErrDefinitiveDenial
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == ' ' {
			return brokerprovider.ErrDefinitiveDenial
		}
	}
	return nil
}

func validArtifact(value string) bool {
	return validSnapshotText(value, maxArtifactBytes, maxArtifactBytes, false)
}

func validOTP(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	n := utf8.RuneCountInString(value)
	if n < minOTPRunes || n > maxOTPRunes || len(value) > maxArtifactBytes {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func classifyStartError(err error) error {
	if err == nil {
		return nil
	}
	return brokerprovider.ErrProviderUnavailable
}

func classifyCallbackError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return brokerprovider.ErrProviderUnavailable
	}
	providerErr, ok := providerError(err)
	if !ok {
		return brokerprovider.ErrProviderUnavailable
	}
	if providerErr.StatusCode == http.StatusTooManyRequests || providerErr.StatusCode >= 500 || providerErr.StatusCode == 0 {
		return brokerprovider.ErrProviderUnavailable
	}
	if providerErr.StatusCode >= 400 && providerErr.StatusCode < 500 {
		return brokerprovider.ErrDefinitiveDenial
	}
	return brokerprovider.ErrProviderUnavailable
}

func mapSessionError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrDefinitive) {
		return brokerprovider.ErrDefinitiveDenial
	}
	var definitive *DefinitiveSessionError
	if errors.As(err, &definitive) {
		return brokerprovider.ErrDefinitiveDenial
	}
	return brokerprovider.ErrProviderUnavailable
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
