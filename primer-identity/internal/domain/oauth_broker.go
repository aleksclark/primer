package domain

import (
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	BrokerStatusPending            = "pending"
	BrokerStatusProviderStarted    = "provider_started"
	BrokerStatusProviderValidating = "provider_validating"
	BrokerStatusAuthorized         = "authorized"
	BrokerStatusDenied             = "denied"
	BrokerStatusFailed             = "failed"
	BrokerStatusExpired            = "expired"
)

type OAuthClient struct {
	ID                                                  uuid.UUID
	ClientID, Name, ClientType, TokenEndpointAuthMethod string
	ClientSecretHash                                    []byte
	ClientSecretPepperVersion                           *int16
	AllowedGrants                                       []string
	Enabled                                             bool
	CreatedAt, UpdatedAt                                time.Time
	DisabledAt                                          *time.Time
}
type OAuthClientRedirect struct {
	ID, OAuthClientID                  uuid.UUID
	RedirectURI, ResourceURI, Audience string
	AllowedScopes                      []string
	Enabled                            bool
	CreatedAt, UpdatedAt               time.Time
}
type BrokerTransaction struct {
	ID, OAuthClientID, RedirectID                        uuid.UUID
	StateHash                                            []byte
	StatePepperVersion                                   int16
	StateSealed                                          []byte
	StateKeyVersion                                      int16
	StateLength                                          int16
	ProviderCodeSealed                                   []byte
	ProviderCodeKeyVersion                               *int16
	PKCEChallenge, PKCEMethod                            string
	RequestedScopes                                      []string
	ResourceURI, Audience                                string
	BrokerCookieHash                                     []byte
	BrokerCookiePepperVersion                            int16
	Status                                               string
	FailureCode                                          *string
	AccountID, ProviderSessionAssociationID              *uuid.UUID
	CreatedAt, ExpiresAt                                 time.Time
	ProviderStartedAt, ProviderValidatingAt, CompletedAt *time.Time
	Version                                              int64
}
type ProviderSessionAssociation struct {
	ID, AccountID, StytchMappingID                                                                 uuid.UUID
	Provider, ProviderProjectID, ProviderOrganizationID, ProviderMemberID, ProviderMemberSessionID string
	ProviderExpiresAt                                                                              time.Time
	Status                                                                                         string
	LastValidatedAt                                                                                time.Time
	RevokedAt                                                                                      *time.Time
	RevokeReasonCode                                                                               *string
	CreatedAt, UpdatedAt                                                                           time.Time
}
type OAuthGrant struct {
	ID                            uuid.UUID
	AccountID, ServicePrincipalID *uuid.UUID
	OAuthClientID                 uuid.UUID
	ProviderSessionAssociationID  *uuid.UUID
	ResourceURI, Audience         string
	Scopes                        []string
	SubjectClass, Status          string
	GrantedAt, NotAfter           time.Time
	RevokedAt                     *time.Time
	RevokeReasonCode              *string
	Version                       int64
}
type OAuthAuthorizationCode struct {
	ID                                          uuid.UUID
	CodeHash                                    []byte
	PepperVersion                               int16
	GrantID, BrokerTransactionID, OAuthClientID uuid.UUID
	RedirectURI, ResourceURI, Audience          string
	Scopes                                      []string
	PKCEChallenge, PKCEMethod                   string
	IssuedAt, ExpiresAt                         time.Time
	ConsumedAt                                  *time.Time
}
type CreateBrokerTransactionInput struct {
	OAuthClientID, RedirectID uuid.UUID
	StateHash                 []byte
	StatePepperVersion        int16
	StateSealed               []byte
	StateKeyVersion           int16
	StateLength               int16
	ProviderCodeSealed        []byte
	ProviderCodeKeyVersion    *int16
	PKCEChallenge, PKCEMethod string
	RequestedScopes           []string
	ResourceURI, Audience     string
	BrokerCookieHash          []byte
	BrokerCookiePepperVersion int16
	CreatedAt, ExpiresAt      time.Time
}

func ValidateCreateBrokerTransactionInput(in CreateBrokerTransactionInput) error {
	if in.OAuthClientID == uuid.Nil || in.RedirectID == uuid.Nil {
		return invalidf("registration", "must be present")
	}
	if len(in.StateHash) != 32 || len(in.BrokerCookieHash) != 32 {
		return invalidf("hash", "must be 32 bytes")
	}
	if in.StatePepperVersion <= 0 || in.StateKeyVersion <= 0 || in.BrokerCookiePepperVersion <= 0 {
		return invalidf("version", "must be positive")
	}
	if in.StateLength < 1 || in.StateLength > 1024 || len(in.StateSealed) < 30 || len(in.StateSealed) > 1053 {
		return invalidf("state", "invalid sealed state bounds")
	}
	if (in.ProviderCodeSealed == nil) != (in.ProviderCodeKeyVersion == nil) {
		return invalidf("provider_code", "ciphertext and key version must agree")
	}
	if err := validatePKCE(in.PKCEChallenge, in.PKCEMethod); err != nil {
		return err
	}
	if err := validateCanonicalArray("requested_scopes", in.RequestedScopes); err != nil {
		return err
	}
	if err := validateBoundedText("resource_uri", in.ResourceURI, 2048); err != nil {
		return err
	}
	if err := validateBoundedText("audience", in.Audience, 128); err != nil {
		return err
	}
	if in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || in.ExpiresAt.After(in.CreatedAt.Add(10*time.Minute)) {
		return invalidf("expires_at", "must be within ten minutes")
	}
	return nil
}
func validatePKCE(challenge, method string) error {
	if method != "S256" || len(challenge) != 43 {
		return invalidf("pkce", "must be S256 base64url length 43")
	}
	for _, r := range challenge {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return invalidf("pkce", "must be base64url")
		}
	}
	return nil
}
func validateBoundedText(field, s string, max int) error {
	if s == "" || !utf8.ValidString(s) || len(s) > max {
		return invalidf(field, "must be strict UTF-8, non-empty and within %d octets", max)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return invalidf(field, "must not contain control characters")
		}
	}
	return nil
}
func validateCanonicalArray(field string, values []string) error {
	if len(values) < 1 || len(values) > 32 {
		return invalidf(field, "must contain 1..32 values")
	}
	prev := ""
	for i, v := range values {
		if err := validateBoundedText(field, v, 128); err != nil {
			return err
		}
		if i > 0 && prev >= v {
			return invalidf(field, "must be sorted and unique")
		}
		prev = v
	}
	return nil
}
func ValidateCanonicalScopes(scopes []string) error { return validateCanonicalArray("scopes", scopes) }
func ValidateBrokerStatus(s string) error {
	switch s {
	case BrokerStatusPending, BrokerStatusProviderStarted, BrokerStatusProviderValidating, BrokerStatusAuthorized, BrokerStatusDenied, BrokerStatusFailed, BrokerStatusExpired:
		return nil
	default:
		return invalidf("status", "unsupported %q", s)
	}
}
func CanonicalScopes(scopes []string) []string {
	out := append([]string(nil), scopes...)
	sort.Strings(out)
	return out
}
func ValidateProviderSessionAssociation(a ProviderSessionAssociation) error {
	if a.ID == uuid.Nil || a.AccountID == uuid.Nil || a.StytchMappingID == uuid.Nil {
		return invalidf("association", "IDs must be present")
	}
	if a.Provider != "stytch_b2b" {
		return invalidf("provider", "must be stytch_b2b")
	}
	for n, v := range map[string]string{"provider_project_id": a.ProviderProjectID, "provider_organization_id": a.ProviderOrganizationID, "provider_member_id": a.ProviderMemberID, "provider_member_session_id": a.ProviderMemberSessionID} {
		if err := validateBoundedText(n, v, 255); err != nil {
			return err
		}
	}
	return nil
}
func ValidateAuthorizationCode(c OAuthAuthorizationCode) error {
	if len(c.CodeHash) != 32 || c.PepperVersion <= 0 {
		return invalidf("code_hash", "must be 32 bytes with positive pepper")
	}
	if !c.ExpiresAt.After(c.IssuedAt) || c.ExpiresAt.After(c.IssuedAt.Add(time.Minute)) {
		return invalidf("expires_at", "must be <=60 seconds")
	}
	if err := validatePKCE(c.PKCEChallenge, c.PKCEMethod); err != nil {
		return err
	}
	return ValidateCanonicalScopes(c.Scopes)
}
func ValidateOAuthGrant(g OAuthGrant) error {
	if g.SubjectClass != "human" || g.AccountID == nil || g.ProviderSessionAssociationID == nil || g.ServicePrincipalID != nil {
		return invalidf("grant", "only complete human grants are allowed in IB1")
	}
	if !g.NotAfter.After(g.GrantedAt) {
		return invalidf("not_after", "must follow grant")
	}
	return ValidateCanonicalScopes(g.Scopes)
}
func validateRegistrationStrings(redirect, resource, audience string) error {
	for _, x := range []struct {
		n, s string
		m    int
	}{{"redirect_uri", redirect, 2048}, {"resource_uri", resource, 2048}, {"audience", audience, 128}} {
		if err := validateBoundedText(x.n, x.s, x.m); err != nil {
			return err
		}
	}
	return nil
}
func registrationError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("registration: %w", err)
}
