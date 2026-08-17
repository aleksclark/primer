// Package oauth implements the IB2 authorization-code exchange and JWT
// issuance core. It has no HTTP surface: callers supply grant fields and a
// client-auth context, and receive plaintext tokens only after commit.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// ExchangeRequest is the exact authorization-code grant field set.
type ExchangeRequest struct {
	GrantType           string
	Code                string
	ClientID            string
	RedirectURI         string
	Resource            string
	CodeVerifier        string
	RefreshToken        string
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
	Issuer        string
	TokenEndpoint string
	AccessTTL     time.Duration
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
			Issuer:        strings.TrimSpace(in.Config.Issuer),
			TokenEndpoint: strings.TrimSpace(in.Config.TokenEndpoint),
			AccessTTL:     ttl,
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
	if req.GrantType == GrantRefreshToken || req.GrantType == GrantClientCredentials {
		return TokenResponse{}, oauthErr(ErrorUnsupportedGrantType, "grant type is not supported")
	}
	if req.GrantType != GrantAuthorizationCode {
		return TokenResponse{}, oauthErr(ErrorUnsupportedGrantType, "grant type is not supported")
	}
	if req.Code == "" || req.RedirectURI == "" || req.Resource == "" || req.CodeVerifier == "" {
		return TokenResponse{}, oauthErr(ErrorInvalidRequest, "authorization_code grant is incomplete")
	}

	now := s.clock.Now().UTC()
	client, err := s.authenticateClient(ctx, req, auth, now)
	if err != nil {
		return TokenResponse{}, err
	}
	if err := requireAuthorizationCodeGrant(client); err != nil {
		return TokenResponse{}, err
	}
	if err := domain.ValidatePublicClientID(client.ClientID); err != nil {
		return TokenResponse{}, oauthErr(ErrorInvalidClient, "client_id is invalid")
	}
	if client.ClientID == client.ID.String() {
		return TokenResponse{}, oauthErr(ErrorInvalidClient, "client_id is invalid")
	}

	registration, err := repo.GetEnabledOAuthClientRedirect(ctx, s.pool, client.ID, req.RedirectURI, req.Resource)
	if err != nil {
		return TokenResponse{}, oauthErr(ErrorInvalidGrant, "authorization code is invalid")
	}

	codeHashes, err := hashAcrossPeppers(s.secrets.AuthorizationCodePeppers, s.secrets.AuthorizationCodeActiveVersion, CodeHashContext, []byte(req.Code))
	if err != nil {
		return TokenResponse{}, oauthErr(ErrorInvalidGrant, "authorization code is invalid")
	}

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
		var (
			code     *domain.OAuthAuthorizationCode
			claimErr error
		)
		for i, codeHash := range codeHashes {
			code, claimErr = repo.ClaimAuthorizationCode(ctx, tx, domain.ClaimAuthorizationCodeInput{
				CodeHash: codeHash, OAuthClientID: client.ID,
				RedirectURI: req.RedirectURI, ResourceURI: req.Resource,
				Audience: registration.Audience, CodeVerifier: req.CodeVerifier,
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

		subject := grant.AccountID.String()
		scope := domain.FormatScopes(code.Scopes)
		var persistErr error
		issuedJWT, persistErr = s.minter.IssueHuman(ctx, token.HumanInput{
			Subject:           subject,
			Audience:          code.Audience,
			ClientID:          client.ClientID,
			Scope:             scope,
			TTL:               s.cfg.AccessTTL,
			GrantNotAfter:     grant.NotAfter,
			ProviderExpiresAt: assoc.ProviderExpiresAt,
		}, func(_ context.Context, issued token.IssuedToken) error {
			jtiHash, hashErr := secrethash.Hash(secrethash.Peppers(s.secrets.AssertionPeppers), s.secrets.AssertionActiveVersion, accessJTIHashContext, []byte(issued.JTI))
			if hashErr != nil {
				sum := sha256.Sum256([]byte(issued.JTI))
				jtiHash = append([]byte(nil), sum[:]...)
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

func (s *Service) authenticateClient(ctx context.Context, req ExchangeRequest, auth ClientAuth, now time.Time) (*domain.OAuthClient, error) {
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
		client, err := repo.GetEnabledOAuthClientByClientID(ctx, s.pool, publicID)
		if err != nil {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if client.TokenEndpointAuthMethod != AuthNone || client.ClientType != "public" {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if req.CodeVerifier == "" {
			return nil, oauthErr(ErrorInvalidRequest, "code_verifier is required")
		}
		return client, nil
	case AuthBasic:
		if publicID == "" || auth.Secret == "" {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		client, err := repo.GetEnabledOAuthClientByClientID(ctx, s.pool, publicID)
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
		client, err := repo.GetEnabledOAuthClientByClientID(ctx, s.pool, publicID)
		if err != nil {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		if client.TokenEndpointAuthMethod != AuthPrivateKeyJWT || client.ClientType != "confidential" {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		keys, err := repo.ListEnabledOAuthClientKeys(ctx, s.pool, client.ID, now)
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
			Audience:    s.cfg.TokenEndpoint,
			Registered:  jwks,
			Now:         now,
			MaxLifetime: domain.MaxAssertionTTL,
		})
		if err != nil {
			return nil, oauthErr(ErrorInvalidClient, "client authentication failed")
		}
		jtiHash, err := secrethash.Hash(secrethash.Peppers(s.secrets.AssertionPeppers), s.secrets.AssertionActiveVersion, assertionJTIHashContext, []byte(parsed.JTI))
		if err != nil {
			sum := sha256.Sum256([]byte(parsed.JTI))
			jtiHash = append([]byte(nil), sum[:]...)
		}
		_, err = repo.RecordClientAssertionReplay(ctx, s.pool, domain.ClientAssertionReplay{
			OAuthClientID: client.ID, EndpointKind: domain.AssertionEndpointToken,
			JTIHash: jtiHash, Audience: s.cfg.TokenEndpoint,
			IssuedAt: parsed.IssuedAt, ExpiresAt: parsed.ExpiresAt, ConsumedAt: now,
		})
		if err != nil {
			if errors.Is(err, domain.ErrConflict) {
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
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 40001") || strings.Contains(msg, "SQLSTATE 40P01")
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
