package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	AssertionEndpointToken      = "token"
	AssertionEndpointRevocation = "revocation"

	RefreshFamilyStatusActive        = "active"
	RefreshFamilyStatusRevoked       = "revoked"
	RefreshFamilyStatusExpired       = "expired"
	RefreshFamilyStatusReuseDetected = "reuse_detected"

	IssuanceOutcomeCommitted = "committed"

	MaxRefreshIdle     = 14 * 24 * time.Hour
	MaxRefreshAbsolute = 90 * 24 * time.Hour
	MaxAccessTTL       = 15 * time.Minute
	MaxAssertionTTL    = 5 * time.Minute
	MaxClientIDLen     = 128
)

type OAuthClientKey struct {
	ID, OAuthClientID   uuid.UUID
	Kid                 string
	JWKJSON             []byte
	Alg, Use            string
	Enabled             bool
	NotBefore, NotAfter *time.Time
	CreatedAt           time.Time
	DisabledAt          *time.Time
}

type ClientAssertionReplay struct {
	ID, OAuthClientID uuid.UUID
	EndpointKind      string
	JTIHash           []byte
	Audience          string
	IssuedAt          time.Time
	ExpiresAt         time.Time
	ConsumedAt        time.Time
}

type OAuthRefreshFamily struct {
	ID, GrantID, OAuthClientID                      uuid.UUID
	ResourceURI, Status                             string
	AbsoluteExpiresAt, IdleExpiresAt, LastRotatedAt time.Time
	RevokedAt, ExpiredAt, ReuseDetectedAt           *time.Time
	RevokeReasonCode                                *string
	Version                                         int64
}

type OAuthRefreshToken struct {
	ID, FamilyID          uuid.UUID
	TokenHash             []byte
	PepperVersion         int16
	Sequence              int64
	IssuedAt, ExpiresAt   time.Time
	ConsumedAt, RevokedAt *time.Time
	ReplacedByID          *uuid.UUID
	ReuseDetectedAt       *time.Time
}

type TokenIssuanceAudit struct {
	ID, GrantID            uuid.UUID
	AuthorizationCodeID    *uuid.UUID
	AuthorizationCodeHash  []byte
	SubjectRef, ClientID   string
	ResourceURI, Audience  string
	Scopes                 []string
	JTIHash                []byte
	Kid                    string
	IssuedAt, ExpiresAt    time.Time
	Outcome                string
	RequestCorrelationHash []byte
}

type InitialRefreshIssuance struct {
	Family            OAuthRefreshFamily
	Token             OAuthRefreshToken
	ClientID          string
	GrantNotAfter     time.Time
	ProviderExpiresAt time.Time
}

type ClaimAuthorizationCodeInput struct {
	CodeHash                           []byte
	OAuthClientID                      uuid.UUID
	RedirectURI, ResourceURI, Audience string
	CodeVerifier                       string
	Now                                time.Time
}

func HumanSubjectRef(accountID uuid.UUID) string {
	return "identity:" + accountID.String()
}

func ValidateOAuthClientKey(k OAuthClientKey) error {
	if k.OAuthClientID == uuid.Nil {
		return invalidf("oauth_client_id", "must be present")
	}
	if err := validateBoundedText("kid", k.Kid, 128); err != nil {
		return err
	}
	if k.Alg != "ES256" || k.Use != "sig" {
		return invalidf("alg", "must be ES256 sig")
	}
	if len(k.JWKJSON) == 0 {
		return invalidf("jwk_json", "must be present")
	}
	var jwk map[string]any
	if err := json.Unmarshal(k.JWKJSON, &jwk); err != nil {
		return invalidf("jwk_json", "must be a public JWK object")
	}
	if _, hasD := jwk["d"]; hasD {
		return invalidf("jwk_json", "must not contain private material")
	}
	if kty, _ := jwk["kty"].(string); kty != "EC" {
		return invalidf("jwk_json", "must be an EC public JWK")
	}
	if k.NotAfter != nil && k.NotBefore != nil && !k.NotAfter.After(*k.NotBefore) {
		return invalidf("not_after", "must follow not_before")
	}
	return nil
}

func ValidateClientAssertionReplay(in ClientAssertionReplay) error {
	if in.OAuthClientID == uuid.Nil {
		return invalidf("oauth_client_id", "must be present")
	}
	if in.EndpointKind != AssertionEndpointToken && in.EndpointKind != AssertionEndpointRevocation {
		return invalidf("endpoint_kind", "must be token or revocation")
	}
	if len(in.JTIHash) != 32 {
		return invalidf("jti_hash", "must be 32 bytes")
	}
	if err := validateBoundedText("audience", in.Audience, 2048); err != nil {
		return err
	}
	if in.IssuedAt.IsZero() || !in.ExpiresAt.After(in.IssuedAt) || in.ExpiresAt.After(in.IssuedAt.Add(MaxAssertionTTL)) {
		return invalidf("expires_at", "must be within five minutes of iat")
	}
	if !in.ConsumedAt.IsZero() && in.ExpiresAt.Before(in.ConsumedAt) && !in.ExpiresAt.Equal(in.ConsumedAt) {
		return invalidf("expires_at", "must be after consumed_at")
	}
	return nil
}

func ValidateInitialRefreshIssuance(in InitialRefreshIssuance) error {
	if err := ValidatePublicClientID(in.ClientID); err != nil {
		return err
	}
	if in.Family.OAuthClientID != uuid.Nil && in.ClientID == in.Family.OAuthClientID.String() {
		return invalidf("client_id", "must not be the internal oauth_clients.id UUID")
	}
	if in.Family.GrantID == uuid.Nil || in.Family.OAuthClientID == uuid.Nil {
		return invalidf("family", "grant and client must be present")
	}
	if err := validateBoundedText("resource_uri", in.Family.ResourceURI, 2048); err != nil {
		return err
	}
	if in.Family.Status != RefreshFamilyStatusActive {
		return invalidf("status", "initial family must be active")
	}
	if in.Family.RevokedAt != nil || in.Family.ExpiredAt != nil || in.Family.ReuseDetectedAt != nil || in.Family.RevokeReasonCode != nil {
		return invalidf("status", "initial family must have no terminal timestamps")
	}
	rotated := in.Family.LastRotatedAt
	if rotated.IsZero() {
		rotated = in.Token.IssuedAt
	}
	if !in.Family.AbsoluteExpiresAt.After(rotated) || !in.Family.IdleExpiresAt.After(rotated) || in.Family.IdleExpiresAt.After(in.Family.AbsoluteExpiresAt) {
		return invalidf("idle_expires_at", "must sit between last_rotated_at and absolute_expires_at")
	}
	if in.Family.IdleExpiresAt.After(rotated.Add(MaxRefreshIdle)) {
		return invalidf("idle_expires_at", "must be within the 14-day idle cap")
	}
	if in.Family.AbsoluteExpiresAt.After(rotated.Add(MaxRefreshAbsolute)) {
		return invalidf("absolute_expires_at", "must be within the 90-day absolute cap")
	}
	if !in.GrantNotAfter.IsZero() && in.Family.AbsoluteExpiresAt.After(in.GrantNotAfter) {
		return invalidf("absolute_expires_at", "must not exceed grant not_after")
	}
	if !in.ProviderExpiresAt.IsZero() && in.Family.AbsoluteExpiresAt.After(in.ProviderExpiresAt) {
		return invalidf("absolute_expires_at", "must not exceed provider expiry")
	}
	if len(in.Token.TokenHash) != 32 || in.Token.PepperVersion <= 0 {
		return invalidf("token_hash", "must be 32 bytes with positive pepper")
	}
	if in.Token.Sequence != 0 {
		return invalidf("sequence", "initial token sequence must be 0")
	}
	if in.Token.ConsumedAt != nil || in.Token.RevokedAt != nil || in.Token.ReplacedByID != nil || in.Token.ReuseDetectedAt != nil {
		return invalidf("token", "initial token must be the current unconsumed token")
	}
	if !in.Token.ExpiresAt.After(in.Token.IssuedAt) {
		return invalidf("expires_at", "must follow issued_at")
	}
	if !in.Token.ExpiresAt.Equal(in.Family.IdleExpiresAt) && in.Token.ExpiresAt.After(in.Family.IdleExpiresAt) {
		return invalidf("expires_at", "must not exceed idle expiry")
	}
	return nil
}

func ValidateTokenIssuanceAudit(in TokenIssuanceAudit) error {
	if in.GrantID == uuid.Nil {
		return invalidf("grant_id", "must be present")
	}
	if len(in.JTIHash) != 32 {
		return invalidf("jti_hash", "must be 32 bytes")
	}
	if in.AuthorizationCodeID != nil && len(in.AuthorizationCodeHash) != 32 {
		return invalidf("authorization_code_hash", "must be copied when authorization_code_id is set")
	}
	if in.AuthorizationCodeHash != nil && len(in.AuthorizationCodeHash) != 32 {
		return invalidf("authorization_code_hash", "must be 32 bytes")
	}
	if err := ValidatePublicClientID(in.ClientID); err != nil {
		return err
	}
	if !strings.HasPrefix(in.SubjectRef, "identity:") {
		return invalidf("subject_ref", "must be identity:<account-id>")
	}
	if err := validateBoundedText("subject_ref", in.SubjectRef, 160); err != nil {
		return err
	}
	if err := validateBoundedText("resource_uri", in.ResourceURI, 2048); err != nil {
		return err
	}
	if err := validateBoundedText("audience", in.Audience, 128); err != nil {
		return err
	}
	if err := ValidateCanonicalScopes(in.Scopes); err != nil {
		return err
	}
	if err := validateBoundedText("kid", in.Kid, 128); err != nil {
		return err
	}
	if in.Outcome != IssuanceOutcomeCommitted {
		return invalidf("outcome", "must be committed")
	}
	if !in.ExpiresAt.After(in.IssuedAt) || in.ExpiresAt.After(in.IssuedAt.Add(MaxAccessTTL)) {
		return invalidf("expires_at", "must be within 15 minutes")
	}
	if in.RequestCorrelationHash != nil && len(in.RequestCorrelationHash) != 32 {
		return invalidf("request_correlation_hash", "must be 32 bytes")
	}
	return nil
}

func ValidateClaimAuthorizationCode(in ClaimAuthorizationCodeInput) error {
	if len(in.CodeHash) != 32 || in.OAuthClientID == uuid.Nil {
		return invalidf("code_hash", "must be 32 bytes bound to a client")
	}
	if err := validateClaimNow(in.Now); err != nil {
		return err
	}
	if err := validateBoundedText("redirect_uri", in.RedirectURI, 2048); err != nil {
		return err
	}
	if err := validateBoundedText("resource_uri", in.ResourceURI, 2048); err != nil {
		return err
	}
	if in.Audience != "" {
		if err := validateBoundedText("audience", in.Audience, 128); err != nil {
			return err
		}
	}
	return validateCodeVerifier(in.CodeVerifier)
}

func validateClaimNow(now time.Time) error {
	if now.IsZero() {
		return invalidf("now", "must be present")
	}
	if now.Location() != time.UTC {
		return invalidf("now", "must be UTC")
	}
	return nil
}

func ValidatePublicClientID(clientID string) error {
	if err := validateBoundedText("client_id", clientID, MaxClientIDLen); err != nil {
		return err
	}
	if _, err := uuid.Parse(clientID); err == nil {
		return invalidf("client_id", "must not be an internal UUID")
	}
	return nil
}

func validateCodeVerifier(verifier string) error {
	if len(verifier) < 43 || len(verifier) > 128 || !utf8.ValidString(verifier) {
		return invalidf("code_verifier", "must be 43..128 RFC 7636 unreserved characters")
	}
	for _, r := range verifier {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' || r == '~') {
			return invalidf("code_verifier", "must be RFC 7636 unreserved")
		}
	}
	return nil
}

func VerifyS256PKCE(verifier, challenge string) error {
	if err := validateCodeVerifier(verifier); err != nil {
		return err
	}
	if err := validatePKCE(challenge, "S256"); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) != 1 {
		return invalidf("code_verifier", "does not match stored S256 challenge")
	}
	return nil
}

func ValidateConsumedAuthorizationCode(c OAuthAuthorizationCode) error {
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

func BindingMatches(code OAuthAuthorizationCode, in ClaimAuthorizationCodeInput) error {
	if code.OAuthClientID != in.OAuthClientID {
		return invalidf("oauth_client_id", "does not match stored code")
	}
	if code.RedirectURI != in.RedirectURI {
		return invalidf("redirect_uri", "does not match stored code binding")
	}
	if code.ResourceURI != in.ResourceURI {
		return invalidf("resource_uri", "does not match stored code binding")
	}
	if code.Audience != in.Audience {
		return invalidf("audience", "does not match stored code binding")
	}
	return VerifyS256PKCE(in.CodeVerifier, code.PKCEChallenge)
}

// ConsumeBindings is the durable grant/provider snapshot required to consume
// an authorization code. The later issuer keeps signing outside the repo and
// only persists after these bindings succeed.
type ConsumeBindings struct {
	Code        OAuthAuthorizationCode
	Grant       OAuthGrant
	Association ProviderSessionAssociation
	Now         time.Time
}

func ValidateConsumeBindings(in ConsumeBindings, claim ClaimAuthorizationCodeInput) error {
	if err := BindingMatches(in.Code, claim); err != nil {
		return err
	}
	if err := ValidateOAuthGrant(in.Grant); err != nil {
		return err
	}
	if in.Grant.ID != in.Code.GrantID {
		return invalidf("grant_id", "does not match stored code")
	}
	if in.Grant.OAuthClientID != claim.OAuthClientID || in.Grant.OAuthClientID != in.Code.OAuthClientID {
		return invalidf("oauth_client_id", "does not match stored grant")
	}
	if in.Grant.ResourceURI != claim.ResourceURI || in.Grant.Audience != claim.Audience {
		return invalidf("resource_uri", "does not match stored grant binding")
	}
	if in.Grant.Status != "active" {
		return invalidf("grant", "must be active")
	}
	if err := validateClaimNow(in.Now); err != nil {
		return err
	}
	now := in.Now
	if !in.Grant.NotAfter.After(now) {
		return invalidf("grant", "must not be expired")
	}
	if in.Grant.ProviderSessionAssociationID == nil || *in.Grant.ProviderSessionAssociationID != in.Association.ID {
		return invalidf("provider", "grant must reference the active provider session")
	}
	if err := ValidateProviderSessionAssociation(in.Association); err != nil {
		return err
	}
	if in.Association.Status != "active" {
		return invalidf("provider", "must be an active provider session")
	}
	if in.Grant.AccountID == nil || *in.Grant.AccountID != in.Association.AccountID {
		return invalidf("provider", "association account must match the grant")
	}
	return nil
}

func FormatScopes(scopes []string) string {
	return strings.Join(CanonicalScopes(scopes), " ")
}

func Must32(label string, b []byte) error {
	if len(b) != 32 {
		return invalidf(label, "must be 32 bytes")
	}
	return nil
}

func FormatClientBinding(clientID string, internalID uuid.UUID) error {
	if clientID == internalID.String() {
		return fmt.Errorf("%w: client_id must stay distinct from oauth_clients.id", ErrInvalid)
	}
	return ValidatePublicClientID(clientID)
}
