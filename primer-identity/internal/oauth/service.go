// Package oauth implements the IB2 authorization-code exchange and JWT
// issuance core. It has no HTTP surface: callers supply grant fields and a
// client-auth context, and receive plaintext tokens only after commit.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/secrethash"
	"github.com/aleksclark/primer/identity/internal/token"
)

const (
	GrantAuthorizationCode = "authorization_code"
	GrantRefreshToken      = "refresh_token"
	GrantClientCredentials = "client_credentials"

	AuthNone          = "none"
	AuthBasic         = "client_secret_basic"
	AuthPrivateKeyJWT = "private_key_jwt"

	ErrorInvalidRequest       = "invalid_request"
	ErrorInvalidClient        = "invalid_client"
	ErrorInvalidGrant         = "invalid_grant"
	ErrorUnauthorizedClient   = "unauthorized_client"
	ErrorUnsupportedGrantType = "unsupported_grant_type"
	ErrorTemporarilyUnavail   = "temporarily_unavailable"

	CodeHashContext         = "primer.oauth.authorization-code"
	clientSecretHashContext = "primer.oauth.client-secret"
	refreshTokenHashContext = "primer.oauth.refresh-token"
	assertionJTIHashContext = "primer.oauth.client-assertion-jti"
	accessJTIHashContext    = "primer.oauth.access-jti"
	assertionTypeJWTBearer  = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	refreshSecretBytes      = 32
	maxAssertionPurge       = 1024
	redactedResponse        = "oauth.TokenResponse{redacted}"
)

// Error is a stable OAuth-compatible class with a fixed description. It never
// embeds raw codes, secrets, or JWT material.
type Error struct {
	Code        string
	Description string
}

func (e *Error) Error() string {
	if e == nil {
		return ErrorTemporarilyUnavail
	}
	return e.Code + ": " + e.Description
}

func oauthErr(code, description string) error {
	return &Error{Code: code, Description: description}
}

// ErrorCodeOf extracts the OAuth class, or "" when err is not an OAuth error.
func ErrorCodeOf(err error) string {
	var oe *Error
	if errors.As(err, &oe) && oe != nil {
		return oe.Code
	}
	return ""
}

// RevokeRequest is the RFC7009 field set accepted by the core service.
type RevokeRequest struct {
	Token               string
	TokenTypeHint       string
	ClientID            string
	ClientAssertionType string
	ClientAssertion     string
}

// ExchangeRequest is the exact authorization-code grant field set.
type ExchangeRequest struct {
	GrantType           string
	Code                string
	ClientID            string
	RedirectURI         string
	Resource            string
	CodeVerifier        string
	RefreshToken        string
	Scope               string
	ClientAssertionType string
	ClientAssertion     string
}

// ClientAuth is the already-parsed client authentication context. HTTP maps
// Authorization / form fields into this type; this package never reads headers.
type ClientAuth struct {
	Method    string
	ClientID  string
	Secret    string
	Assertion string
}

// TokenResponse is the post-commit authorization-code success payload.
type TokenResponse struct {
	AccessToken  string
	TokenType    string
	ExpiresIn    int
	Scope        string
	RefreshToken string
}

func (TokenResponse) String() string   { return redactedResponse }
func (TokenResponse) GoString() string { return redactedResponse }
func (TokenResponse) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, redactedResponse)
}
func (TokenResponse) MarshalJSON() ([]byte, error) {
	return nil, oauthErr(ErrorTemporarilyUnavail, "token response is not serializable")
}

// Secrets is the injected, copy-safe pepper set used by the exchange service.
type Secrets struct {
	AuthorizationCodePeppers       map[int][]byte
	AuthorizationCodeActiveVersion int
	ClientSecretPeppers            map[int][]byte
	ClientSecretActiveVersion      int
	RefreshTokenPeppers            map[int][]byte
	RefreshTokenActiveVersion      int
	AssertionPeppers               map[int][]byte
	AssertionActiveVersion         int
}

// Config is the fail-closed issuer configuration for this service.
type Config struct {
	Issuer             string
	TokenEndpoint      string
	RevocationEndpoint string
	AccessTTL          time.Duration
}

// Dependencies are injected so app composition can stay in a later slice.
type Dependencies struct {
	Pool    *pgxpool.Pool
	Signer  token.SignerSource
	Secrets Secrets
	Clock   token.Clock
	Rand    io.Reader
	Config  Config
	// afterSign is a test-only hook invoked after signing and before commit.
	afterSign func(tx pgx.Tx, issued token.IssuedToken) error
}

// TxSignerSource yields a transaction-scoped signer on the caller's existing
// transaction. Implementations must not open an additional database connection.
type TxSignerSource interface {
	CurrentForTx(ctx context.Context, tx pgx.Tx) (token.Signer, *domain.SigningKey, error)
}

// Service exchanges one authorization code for an access JWT and opaque refresh.
type Service struct {
	pool    *pgxpool.Pool
	signer  token.SignerSource
	secrets Secrets
	clock   token.Clock
	rand    io.Reader
	minter  *token.Minter
	cfg     Config
	after   func(tx pgx.Tx, issued token.IssuedToken) error
}

// NewService constructs a fail-closed exchange service.
func NewService(in Dependencies) (*Service, error) {
	if in.Pool == nil || in.Signer == nil {
		return nil, oauthErr(ErrorTemporarilyUnavail, "service dependencies are unavailable")
	}
	secrets, err := copySecrets(in.Secrets)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Config.Issuer) == "" || strings.TrimSpace(in.Config.TokenEndpoint) == "" {
		return nil, oauthErr(ErrorTemporarilyUnavail, "issuer configuration is unavailable")
	}
	ttl := in.Config.AccessTTL
	if ttl <= 0 {
		ttl = domain.MaxAccessTTL
	}
	if ttl > domain.MaxAccessTTL || ttl%time.Second != 0 {
		return nil, oauthErr(ErrorInvalidRequest, "access token lifetime is invalid")
	}
	clock := in.Clock
	if clock == nil {
		clock = realClock{}
	}
	random := in.Rand
	if random == nil {
		random = rand.Reader
	}
	minter, err := token.NewMinter(in.Signer, in.Config.Issuer, clock)
	if err != nil {
		return nil, oauthErr(ErrorTemporarilyUnavail, "signer is unavailable")
	}
	return &Service{
		pool:    in.Pool,
		signer:  in.Signer,
		secrets: secrets,
		clock:   clock,
		rand:    random,
		minter:  minter,
		cfg: Config{
			Issuer:             strings.TrimSpace(in.Config.Issuer),
			TokenEndpoint:      strings.TrimSpace(in.Config.TokenEndpoint),
			RevocationEndpoint: strings.TrimSpace(in.Config.RevocationEndpoint),
			AccessTTL:          ttl,
		},
		after: in.afterSign,
	}, nil
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Exchange performs one short serializable authorization-code issuance.
func (s *Service) Exchange(ctx context.Context, req ExchangeRequest, auth ClientAuth) (TokenResponse, error) {
	if s == nil || s.pool == nil || s.minter == nil {
		return TokenResponse{}, oauthErr(ErrorTemporarilyUnavail, "service is unavailable")
	}
	if ctx == nil || ctx.Err() != nil {
		return TokenResponse{}, oauthErr(ErrorTemporarilyUnavail, "request canceled")
	}
	if req.GrantType == GrantClientCredentials {
		if auth.Method == "" || auth.Method == AuthNone {
			return TokenResponse{}, oauthErr(ErrorInvalidRequest, "client authentication is required")
		}
		return s.exchangeClientCredentials(ctx, req, auth)
	}
	if req.GrantType == GrantRefreshToken {
		return TokenResponse{}, oauthErr(ErrorUnsupportedGrantType, "grant type is not supported")
	}
	if req.GrantType != GrantAuthorizationCode {
		return TokenResponse{}, oauthErr(ErrorUnsupportedGrantType, "grant type is not supported")
	}
	if req.Code == "" || req.RedirectURI == "" || req.Resource == "" || req.CodeVerifier == "" {
		return TokenResponse{}, oauthErr(ErrorInvalidRequest, "authorization_code grant is incomplete")
	}

	now := s.clock.Now().UTC()

	refreshSecret, err := randomBytes(s.rand, refreshSecretBytes)
	if err != nil {
		return TokenResponse{}, oauthErr(ErrorTemporarilyUnavail, "token material is unavailable")
	}
	refreshHash, err := secrethash.Hash(secrethash.Peppers(s.secrets.RefreshTokenPeppers), s.secrets.RefreshTokenActiveVersion, refreshTokenHashContext, refreshSecret)
	if err != nil {
		return TokenResponse{}, oauthErr(ErrorTemporarilyUnavail, "token material is unavailable")
	}
	opaqueRefresh := base64.RawURLEncoding.EncodeToString(refreshSecret)
	zeroBytes(refreshSecret)

	var (
		issuedJWT token.IssuedToken
		response  TokenResponse
	)
	err = repo.WithSerializableRetry(ctx, s.pool, func(tx pgx.Tx) error {
		client, err := s.authenticateClient(ctx, tx, clientPresentation{
			ClientID:            req.ClientID,
			ClientAssertionType: req.ClientAssertionType,
			CodeVerifier:        req.CodeVerifier,
			RequirePKCE:         true,
		}, auth, now, s.cfg.TokenEndpoint, domain.AssertionEndpointToken)
		if err != nil {
			return err
		}
		if err := requireAuthorizationCodeGrant(client); err != nil {
			return err
		}
		if err := domain.ValidatePublicClientID(client.ClientID); err != nil {
			return oauthErr(ErrorInvalidClient, "client_id is invalid")
		}
		if client.ClientID == client.ID.String() {
			return oauthErr(ErrorInvalidClient, "client_id is invalid")
		}

		registration, err := repo.GetEnabledOAuthClientRedirect(ctx, tx, client.ID, req.RedirectURI, req.Resource)
		if err != nil {
			return oauthErr(ErrorInvalidGrant, "authorization code is invalid")
		}

		codeHashes, err := hashAcrossPeppers(s.secrets.AuthorizationCodePeppers, s.secrets.AuthorizationCodeActiveVersion, CodeHashContext, []byte(req.Code))
		if err != nil {
			return oauthErr(ErrorInvalidGrant, "authorization code is invalid")
		}
		var (
			code     *domain.OAuthAuthorizationCode
			claimErr error
		)
		for i, codeHash := range codeHashes {
			code, claimErr = repo.ClaimAuthorizationCode(ctx, tx, domain.ClaimAuthorizationCodeInput{
				CodeHash: codeHash, OAuthClientID: client.ID,
				RedirectURI: req.RedirectURI, ResourceURI: req.Resource,
				Audience: registration.Audience, CodeVerifier: req.CodeVerifier,
				Now: now,
			})
			if claimErr == nil {
				break
			}
			if errors.Is(claimErr, domain.ErrNotFound) && i < len(codeHashes)-1 {
				continue
			}
			return mapGrantError(claimErr)
		}
		if claimErr != nil || code == nil {
			return mapGrantError(claimErr)
		}
		grant, assoc, bindErr := repo.LoadConsumeBindings(ctx, tx, code.GrantID)
		if bindErr != nil {
			return mapGrantError(bindErr)
		}
		if grant.AccountID == nil {
			return oauthErr(ErrorInvalidGrant, "authorization code is invalid")
		}
		if grant.Audience != registration.Audience || grant.ResourceURI != req.Resource {
			return oauthErr(ErrorInvalidGrant, "authorization code is invalid")
		}

		rotated := now
		idle := capTime(rotated.Add(domain.MaxRefreshIdle), grant.NotAfter, assoc.ProviderExpiresAt)
		absolute := capTime(rotated.Add(domain.MaxRefreshAbsolute), grant.NotAfter, assoc.ProviderExpiresAt)
		if !idle.After(rotated) || !absolute.After(rotated) || idle.After(absolute) {
			return oauthErr(ErrorInvalidGrant, "authorization code is invalid")
		}

		refresh := domain.InitialRefreshIssuance{
			Family: domain.OAuthRefreshFamily{
				GrantID: code.GrantID, OAuthClientID: client.ID, ResourceURI: code.ResourceURI,
				Status: domain.RefreshFamilyStatusActive, AbsoluteExpiresAt: absolute,
				IdleExpiresAt: idle, LastRotatedAt: rotated,
			},
			Token: domain.OAuthRefreshToken{
				TokenHash: refreshHash, PepperVersion: int16(s.secrets.RefreshTokenActiveVersion),
				Sequence: 0, IssuedAt: rotated, ExpiresAt: idle,
			},
			ClientID: client.ClientID, GrantNotAfter: grant.NotAfter, ProviderExpiresAt: assoc.ProviderExpiresAt,
		}
		if _, famErr := repo.CreateInitialRefreshFamily(ctx, tx, refresh); famErr != nil {
			return mapUnavailable(famErr)
		}

		txSigner, txMeta, signerErr := s.txSigner(ctx, tx)
		if signerErr != nil {
			if isRetryableSerialization(signerErr) {
				return signerErr
			}
			return oauthErr(ErrorTemporarilyUnavail, "signer is unavailable")
		}
		if closer, ok := txSigner.(io.Closer); ok {
			defer func() { _ = closer.Close() }()
		}

		subject := grant.AccountID.String()
		scope := domain.FormatScopes(code.Scopes)
		var persistErr error
		issuedJWT, persistErr = s.minter.IssueHumanWithSigner(ctx, token.HumanInput{
			Subject:           subject,
			Audience:          code.Audience,
			ClientID:          client.ClientID,
			Scope:             scope,
			TTL:               s.cfg.AccessTTL,
			GrantNotAfter:     grant.NotAfter,
			ProviderExpiresAt: assoc.ProviderExpiresAt,
		}, txSigner, txMeta, func(_ context.Context, issued token.IssuedToken) error {
			jtiHash, hashErr := secrethash.Hash(secrethash.Peppers(s.secrets.AssertionPeppers), s.secrets.AssertionActiveVersion, accessJTIHashContext, []byte(issued.JTI))
			if hashErr != nil {
				return oauthErr(ErrorTemporarilyUnavail, "token issuance is unavailable")
			}
			_, auditErr := repo.CreateTokenIssuanceAudit(ctx, tx, domain.TokenIssuanceAudit{
				GrantID: code.GrantID, AuthorizationCodeID: &code.ID, AuthorizationCodeHash: append([]byte(nil), code.CodeHash...),
				SubjectRef: domain.HumanSubjectRef(*grant.AccountID), ClientID: client.ClientID,
				ResourceURI: code.ResourceURI, Audience: code.Audience, Scopes: append([]string(nil), code.Scopes...),
				JTIHash: jtiHash, Kid: issued.Kid, IssuedAt: issued.IssuedAt, ExpiresAt: issued.ExpiresAt,
				Outcome: domain.IssuanceOutcomeCommitted,
			})
			return auditErr
		})
		if persistErr != nil {
			if isRetryableSerialization(persistErr) {
				return persistErr
			}
			if errors.Is(persistErr, token.ErrInvalid) {
				return oauthErr(ErrorInvalidGrant, "authorization code is invalid")
			}
			return oauthErr(ErrorTemporarilyUnavail, "signer is unavailable")
		}
		if s.after != nil {
			if afterErr := s.after(tx, issuedJWT); afterErr != nil {
				return afterErr
			}
		}
		response = TokenResponse{
			AccessToken:  issuedJWT.Compact,
			TokenType:    "Bearer",
			ExpiresIn:    int(issuedJWT.Lifetime / time.Second),
			Scope:        scope,
			RefreshToken: opaqueRefresh,
		}
		return nil
	})
	if err != nil {
		zeroString(&issuedJWT.Compact)
		zeroString(&response.AccessToken)
		zeroString(&response.RefreshToken)
		if ErrorCodeOf(err) != "" {
			return TokenResponse{}, err
		}
		if ctx != nil && ctx.Err() != nil {
			return TokenResponse{}, oauthErr(ErrorTemporarilyUnavail, "request canceled")
		}
		return TokenResponse{}, oauthErr(ErrorTemporarilyUnavail, "token issuance is unavailable")
	}
	return response, nil
}

// PurgeExpiredAssertionReplays deletes expired private_key_jwt JTIs, bounded.
func (s *Service) exchangeClientCredentials(ctx context.Context, req ExchangeRequest, auth ClientAuth) (TokenResponse, error) {
	if req.Resource == "" {
		return TokenResponse{}, oauthErr(ErrorInvalidRequest, "resource is required")
	}
	if auth.Method != AuthBasic && auth.Method != AuthPrivateKeyJWT {
		return TokenResponse{}, oauthErr(ErrorInvalidClient, "client authentication failed")
	}
	now := s.clock.Now().UTC()
	var response TokenResponse
	err := repo.WithSerializableRetry(ctx, s.pool, func(tx pgx.Tx) error {
		client, err := s.authenticateClient(ctx, tx, clientPresentation{ClientID: req.ClientID, ClientAssertionType: req.ClientAssertionType}, auth, now, s.cfg.TokenEndpoint, domain.AssertionEndpointToken)
		if err != nil {
			return err
		}
		if !hasGrant(client, GrantClientCredentials) {
			return oauthErr(ErrorUnauthorizedClient, "client is not authorized for this grant")
		}
		cred, err := repo.GetActiveServiceCredential(ctx, tx, client.ID)
		if err != nil {
			return oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if auth.Method == AuthBasic {
			computed, hashErr := secrethash.Hash(secrethash.Peppers(s.secrets.ClientSecretPeppers), int(cred.PepperVersion), clientSecretHashContext, []byte(auth.Secret))
			if hashErr != nil || !secrethash.Equal(computed, cred.SecretHash) {
				return oauthErr(ErrorInvalidClient, "client authentication failed")
			}
		}
		// A credential carries exactly one registered resource/audience tuple.
		// Never derive an audience from an independently configured list.
		if cred.ResourceURI != req.Resource {
			return oauthErr("invalid_target", "the requested target is invalid")
		}
		scopes, scopeErr := requestedServiceScopes(req.Scope, cred.AllowedScopes)
		if scopeErr != nil {
			return scopeErr
		}
		audience := cred.Audience
		notAfter := now.Add(s.cfg.AccessTTL)
		if cred.NotAfter != nil && cred.NotAfter.Before(notAfter) {
			notAfter = cred.NotAfter.UTC()
		}
		if !notAfter.After(now) {
			return oauthErr(ErrorInvalidClient, "client credential is expired")
		}
		grantID, err := repo.CreateServiceGrant(ctx, tx, cred.ServicePrincipalID, client.ID, req.Resource, audience, scopes, now, notAfter)
		if err != nil {
			return oauthErr(ErrorTemporarilyUnavail, "token issuance is unavailable")
		}
		txSigner, txMeta, err := s.txSigner(ctx, tx)
		if err != nil {
			return oauthErr(ErrorTemporarilyUnavail, "signer is unavailable")
		}
		issued, err := s.minter.IssueServiceWithSigner(ctx, token.ServiceInput{Subject: cred.SubjectRef, Audience: audience, ClientID: client.ClientID, Scope: domain.FormatScopes(scopes), TTL: s.cfg.AccessTTL, GrantNotAfter: notAfter}, txSigner, txMeta, func(_ context.Context, issued token.IssuedToken) error {
			jtiHash, hashErr := secrethash.Hash(secrethash.Peppers(s.secrets.AssertionPeppers), s.secrets.AssertionActiveVersion, accessJTIHashContext, []byte(issued.JTI))
			if hashErr != nil {
				return hashErr
			}
			_, auditErr := repo.CreateTokenIssuanceAudit(ctx, tx, domain.TokenIssuanceAudit{GrantID: grantID, SubjectRef: cred.SubjectRef, ClientID: client.ClientID, ResourceURI: req.Resource, Audience: audience, Scopes: scopes, JTIHash: jtiHash, Kid: issued.Kid, IssuedAt: issued.IssuedAt, ExpiresAt: issued.ExpiresAt, Outcome: domain.IssuanceOutcomeCommitted})
			return auditErr
		})
		if err != nil {
			return oauthErr(ErrorTemporarilyUnavail, "token issuance is unavailable")
		}
		response = TokenResponse{AccessToken: issued.Compact, TokenType: "Bearer", ExpiresIn: int(issued.Lifetime / time.Second), Scope: domain.FormatScopes(scopes)}
		return nil
	})
	if err != nil {
		if ErrorCodeOf(err) != "" {
			return TokenResponse{}, err
		}
		return TokenResponse{}, oauthErr(ErrorTemporarilyUnavail, "token issuance is unavailable")
	}
	return response, nil
}

func hasGrant(client *domain.OAuthClient, want string) bool {
	for _, got := range client.AllowedGrants {
		if got == want {
			return true
		}
	}
	return false
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func requestedServiceScopes(requested string, allowed []string) ([]string, error) {
	// Service authority is deliberately narrower than human authorization:
	// only explicit read/draft capabilities may be registered or requested.
	for _, scope := range allowed {
		if scope != "studio.read" && scope != "studio.draft" || strings.Contains(scope, "*") {
			return nil, oauthErr("invalid_scope", "the requested scope is invalid")
		}
	}
	if requested == "" {
		return domain.CanonicalScopes(allowed), nil
	}
	scopes := strings.Fields(requested)
	if err := domain.ValidateCanonicalScopes(domain.CanonicalScopes(scopes)); err != nil {
		return nil, oauthErr("invalid_scope", "the requested scope is invalid")
	}
	canon := domain.CanonicalScopes(scopes)
	for _, scope := range canon {
		if (scope != "studio.read" && scope != "studio.draft") || strings.Contains(scope, "*") || !contains(allowed, scope) {
			return nil, oauthErr("invalid_scope", "the requested scope is invalid")
		}
	}
	return canon, nil
}

func (s *Service) PurgeExpiredAssertionReplays(ctx context.Context, now time.Time) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, oauthErr(ErrorTemporarilyUnavail, "service is unavailable")
	}
	if now.IsZero() {
		now = s.clock.Now().UTC()
	}
	return repo.PurgeExpiredClientAssertionReplaysBounded(ctx, s.pool, now, maxAssertionPurge)
}

// RevokeInitialFamily idempotently revokes an initial refresh family. It does
// not implement IB6 rotation/reuse detection.
func (s *Service) RevokeInitialFamily(ctx context.Context, familyID uuid.UUID, reason string) error {
	if s == nil || s.pool == nil {
		return oauthErr(ErrorTemporarilyUnavail, "service is unavailable")
	}
	if familyID == uuid.Nil {
		return oauthErr(ErrorInvalidRequest, "family is invalid")
	}
	if reason == "" {
		reason = "operator"
	}
	return repo.RevokeInitialRefreshFamily(ctx, s.pool, familyID, reason, s.clock.Now().UTC())
}

// Revoke authenticates the registered owning client and idempotently revokes
// an IB2 initial refresh family plus its grant. Unknown, already-revoked,
// access-token, and cross-client presentations are oracle-free successes.
func (s *Service) Revoke(ctx context.Context, req RevokeRequest, auth ClientAuth) error {
	if s == nil || s.pool == nil {
		return oauthErr(ErrorTemporarilyUnavail, "service is unavailable")
	}
	if ctx == nil || ctx.Err() != nil {
		return oauthErr(ErrorTemporarilyUnavail, "request canceled")
	}
	if strings.TrimSpace(req.Token) == "" {
		return oauthErr(ErrorInvalidRequest, "token is required")
	}
	now := s.clock.Now().UTC()
	hashes, ok, err := refreshHashesFromPresented(s.secrets.RefreshTokenPeppers, s.secrets.RefreshTokenActiveVersion, req.Token)
	if err != nil {
		return oauthErr(ErrorTemporarilyUnavail, "token material is unavailable")
	}
	err = repo.WithSerializableRetry(ctx, s.pool, func(tx pgx.Tx) error {
		client, authErr := s.authenticateClient(ctx, tx, clientPresentation{
			ClientID:            req.ClientID,
			ClientAssertionType: req.ClientAssertionType,
			RequirePKCE:         false,
		}, auth, now, s.cfg.RevocationEndpoint, domain.AssertionEndpointRevocation)
		if authErr != nil {
			return authErr
		}
		if !ok {
			return nil
		}
		return repo.RevokeOwnedInitialRefresh(ctx, tx, hashes, client.ID, "client_revoked", now)
	})
	if err != nil {
		if ErrorCodeOf(err) != "" {
			return err
		}
		if ctx.Err() != nil {
			return oauthErr(ErrorTemporarilyUnavail, "request canceled")
		}
		if isRetryableSerialization(err) {
			return oauthErr(ErrorTemporarilyUnavail, "token revocation is unavailable")
		}
		return oauthErr(ErrorTemporarilyUnavail, "token revocation is unavailable")
	}
	return nil
}

type clientPresentation struct {
	ClientID            string
	ClientAssertionType string
	CodeVerifier        string
	RequirePKCE         bool
}

func (s *Service) authenticateClient(ctx context.Context, q repo.Querier, req clientPresentation, auth ClientAuth, now time.Time, audience, endpointKind string) (*domain.OAuthClient, error) {
	if q == nil {
		q = s.pool
	}
	method := strings.TrimSpace(auth.Method)
	publicID := strings.TrimSpace(auth.ClientID)
	if publicID == "" {
		publicID = strings.TrimSpace(req.ClientID)
	}
	switch method {
	case AuthNone:
		if publicID == "" {
			return nil, oauthErr(ErrorInvalidRequest, "client_id is required")
		}
		if req.ClientID != "" && req.ClientID != publicID {
			return nil, oauthErr(ErrorInvalidRequest, "client_id must appear exactly once")
		}
		client, err := repo.GetEnabledOAuthClientByClientID(ctx, q, publicID)
		if err != nil {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if client.TokenEndpointAuthMethod != AuthNone || client.ClientType != "public" {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if req.RequirePKCE && req.CodeVerifier == "" {
			return nil, oauthErr(ErrorInvalidRequest, "code_verifier is required")
		}
		return client, nil
	case AuthBasic:
		if publicID == "" || auth.Secret == "" {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		client, err := repo.GetEnabledOAuthClientByClientID(ctx, q, publicID)
		if err != nil {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if client.TokenEndpointAuthMethod != AuthBasic || client.ClientType != "confidential" {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if client.ClientSecretPepperVersion == nil || len(client.ClientSecretHash) != 32 {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		computed, err := secrethash.Hash(secrethash.Peppers(s.secrets.ClientSecretPeppers), int(*client.ClientSecretPepperVersion), clientSecretHashContext, []byte(auth.Secret))
		if err != nil || !secrethash.Equal(computed, client.ClientSecretHash) {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		return client, nil
	case AuthPrivateKeyJWT:
		if publicID == "" || auth.Assertion == "" {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if req.ClientAssertionType != "" && req.ClientAssertionType != assertionTypeJWTBearer {
			return nil, oauthErr(ErrorInvalidRequest, "client_assertion_type is invalid")
		}
		if strings.TrimSpace(audience) == "" || (endpointKind != domain.AssertionEndpointToken && endpointKind != domain.AssertionEndpointRevocation) {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		client, err := repo.GetEnabledOAuthClientByClientID(ctx, q, publicID)
		if err != nil {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if client.TokenEndpointAuthMethod != AuthPrivateKeyJWT || client.ClientType != "confidential" {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		keys, err := repo.ListEnabledOAuthClientKeys(ctx, q, client.ID, now)
		if err != nil || len(keys) == 0 {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		jwks := make([]domain.PublicJWK, 0, len(keys))
		for _, key := range keys {
			jwk, parseErr := domain.ParsePublicJWK(key.JWKJSON)
			if parseErr != nil {
				return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
			}
			jwks = append(jwks, jwk)
		}
		parsed, err := token.ParseClientAssertion(auth.Assertion, token.AssertionInput{
			ClientID:    client.ClientID,
			Audience:    audience,
			Registered:  jwks,
			Now:         now,
			MaxLifetime: domain.MaxAssertionTTL,
		})
		if err != nil {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		jtiHash, err := secrethash.Hash(secrethash.Peppers(s.secrets.AssertionPeppers), s.secrets.AssertionActiveVersion, assertionJTIHashContext, []byte(parsed.JTI))
		if err != nil {
			return nil, oauthErr(ErrorTemporarilyUnavail, "client authentication failed")
		}
		// Retain through the parser accept window (JWT exp + clock skew) so
		// purge-at-exp cannot open a post-expiry replay oracle.
		retention := parsed.ExpiresAt.Add(domain.AssertionClockSkew)
		_, err = repo.RecordClientAssertionReplay(ctx, q, domain.ClientAssertionReplay{
			OAuthClientID: client.ID, EndpointKind: endpointKind,
			JTIHash: jtiHash, Audience: audience,
			IssuedAt: parsed.IssuedAt, ExpiresAt: retention, ConsumedAt: now,
		})
		if err != nil {
			// Conflict and lifetime-edge validation are client-auth failures.
			// Never surface temporarily_unavailable for crypto-valid late material
			// near expiry (avoids a post-exp skew oracle).
			if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrInvalid) {
				return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
			}
			return nil, oauthErr(ErrorTemporarilyUnavail, "client authentication failed")
		}
		return client, nil
	default:
		return nil, oauthErr(ErrorInvalidRequest, "client authentication method is unsupported")
	}
}

func requireAuthorizationCodeGrant(client *domain.OAuthClient) error {
	for _, grant := range client.AllowedGrants {
		if grant == GrantAuthorizationCode {
			return nil
		}
	}
	return oauthErr(ErrorUnauthorizedClient, "client is not allowed this grant")
}

func hashAcrossPeppers(peppers map[int][]byte, active int, context string, secret []byte) ([][]byte, error) {
	if len(peppers) == 0 || active <= 0 {
		return nil, secrethash.ErrInvalid
	}
	versions := make([]int, 0, len(peppers))
	if _, ok := peppers[active]; ok {
		versions = append(versions, active)
	}
	rest := make([]int, 0, len(peppers))
	for version := range peppers {
		if version == active {
			continue
		}
		rest = append(rest, version)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(rest)))
	versions = append(versions, rest...)
	out := make([][]byte, 0, len(versions))
	var last error
	for _, version := range versions {
		sum, err := secrethash.Hash(secrethash.Peppers(peppers), version, context, secret)
		if err != nil {
			last = err
			continue
		}
		out = append(out, sum)
	}
	if len(out) == 0 {
		if last == nil {
			last = secrethash.ErrInvalid
		}
		return nil, last
	}
	return out, nil
}

func refreshHashesFromPresented(peppers map[int][]byte, active int, presented string) ([][]byte, bool, error) {
	raw, err := base64.RawURLEncoding.DecodeString(presented)
	if err != nil || len(raw) != refreshSecretBytes {
		return nil, false, nil
	}
	hashes, err := hashAcrossPeppers(peppers, active, refreshTokenHashContext, raw)
	zeroBytes(raw)
	if err != nil {
		return nil, false, err
	}
	return hashes, true, nil
}

func capTime(candidate time.Time, caps ...time.Time) time.Time {
	out := candidate
	for _, capAt := range caps {
		if capAt.IsZero() {
			continue
		}
		if capAt.Before(out) {
			out = capAt
		}
	}
	return out
}

func randomBytes(r io.Reader, n int) ([]byte, error) {
	if r == nil {
		r = rand.Reader
	}
	out := make([]byte, n)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}

func copySecrets(in Secrets) (Secrets, error) {
	out := Secrets{
		AuthorizationCodeActiveVersion: in.AuthorizationCodeActiveVersion,
		ClientSecretActiveVersion:      in.ClientSecretActiveVersion,
		RefreshTokenActiveVersion:      in.RefreshTokenActiveVersion,
		AssertionActiveVersion:         in.AssertionActiveVersion,
	}
	var err error
	if out.AuthorizationCodePeppers, err = copyPepperMap(in.AuthorizationCodePeppers, in.AuthorizationCodeActiveVersion); err != nil {
		return Secrets{}, err
	}
	if out.ClientSecretPeppers, err = copyPepperMap(in.ClientSecretPeppers, in.ClientSecretActiveVersion); err != nil {
		return Secrets{}, err
	}
	if out.RefreshTokenPeppers, err = copyPepperMap(in.RefreshTokenPeppers, in.RefreshTokenActiveVersion); err != nil {
		return Secrets{}, err
	}
	if out.AssertionPeppers, err = copyPepperMap(in.AssertionPeppers, in.AssertionActiveVersion); err != nil {
		return Secrets{}, err
	}
	return out, nil
}

func copyPepperMap(in map[int][]byte, active int) (map[int][]byte, error) {
	if len(in) == 0 || active <= 0 {
		return nil, oauthErr(ErrorTemporarilyUnavail, "secret material is unavailable")
	}
	if _, ok := in[active]; !ok {
		return nil, oauthErr(ErrorTemporarilyUnavail, "secret material is unavailable")
	}
	out := make(map[int][]byte, len(in))
	for version, material := range in {
		if version <= 0 || len(material) != 32 {
			return nil, oauthErr(ErrorTemporarilyUnavail, "secret material is unavailable")
		}
		out[version] = append([]byte(nil), material...)
	}
	return out, nil
}

func mapGrantError(err error) error {
	if isRetryableSerialization(err) {
		return err
	}
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrConflict):
		return oauthErr(ErrorInvalidGrant, "authorization code is invalid")
	default:
		return oauthErr(ErrorTemporarilyUnavail, "token issuance is unavailable")
	}
}

func isRetryableSerialization(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrRetryableSerialization) {
		return true
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code == "40001" || pe.Code == "40P01"
	}
	return false
}

func mapUnavailable(err error) error {
	if isRetryableSerialization(err) {
		return err
	}
	if errors.Is(err, domain.ErrInvalid) {
		return oauthErr(ErrorInvalidGrant, "authorization code is invalid")
	}
	return oauthErr(ErrorTemporarilyUnavail, "token issuance is unavailable")
}

func (s *Service) txSigner(ctx context.Context, tx pgx.Tx) (token.Signer, *domain.SigningKey, error) {
	if source, ok := s.signer.(TxSignerSource); ok {
		return source.CurrentForTx(ctx, tx)
	}
	return s.signer.Current(ctx)
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func zeroString(s *string) {
	if s == nil {
		return
	}
	*s = ""
}
