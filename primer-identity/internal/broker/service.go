package broker

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/secrethash"
	"github.com/aleksclark/primer/identity/internal/stateseal"
)

// Lifetimes and sizes frozen by the IB0 contract.
const (
	// BrokerTransactionTTL is the exact broker transaction lifetime.
	BrokerTransactionTTL = 10 * time.Minute
	// AuthorizationCodeTTL is the exact one-use code lifetime.
	AuthorizationCodeTTL = 60 * time.Second
	// GrantLifetime bounds the human grant created alongside a code.
	GrantLifetime = 24 * time.Hour
	// CodeEntropyBytes is the 256-bit authorization code size.
	CodeEntropyBytes = 32
	// CookieEntropyBytes is the 256-bit broker cookie size.
	CookieEntropyBytes = 32

	// BrokerCookieName is the temporary host-prefixed broker cookie.
	BrokerCookieName = "__Host-primer-broker"

	stateHashContext   = "primer.oauth.state-hash"
	cookieHashContext  = "primer.broker.cookie"
	codeHashContext    = "primer.oauth.authorization-code"
	maxCookieValueLen  = 128
	maxArtifactByteLen = 4096
)

// Sentinel classes for callback outcomes.
var (
	// ErrUnboundCallback is a missing, forged, expired, or already-terminal
	// broker binding. It is non-oracular and always a local error.
	ErrUnboundCallback = errors.New("broker callback is not bound to a live transaction")
	// ErrIncompleteMFA means the human must finish MFA. No authority is issued
	// and the human remains Identity-side.
	ErrIncompleteMFA = errors.New("broker provider requires MFA continuation")
	// ErrProviderDenied is a definitive provider denial.
	ErrProviderDenied = errors.New("broker provider denied the authentication")
	// ErrProviderUnavailable is a transient provider fault; it never
	// terminalizes a recoverable transaction and is never negative-cached.
	ErrProviderUnavailable = errors.New("broker provider is unavailable")
	// ErrLostRace means a concurrent callback already committed this
	// transaction. The loser never receives the winner's code.
	ErrLostRace = errors.New("broker callback lost a concurrent race")
)

// IsIncompleteMFA reports an MFA-continuation outcome.
func IsIncompleteMFA(err error) bool { return errors.Is(err, ErrIncompleteMFA) }

// IsRedirectable reports whether an error may be reported to the client by
// redirect. Registration, binding, parse, and transient faults are local
// errors, never redirects, so an unvalidated redirect target can never be
// used. After an exact registration is known, invalid_scope and a definitive
// provider denial may be reported on that registered redirect.
func IsRedirectable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUnboundCallback) || errors.Is(err, ErrLostRace) ||
		errors.Is(err, ErrProviderUnavailable) || errors.Is(err, ErrIncompleteMFA) {
		return false
	}
	var oe *Error
	if errors.As(err, &oe) {
		switch oe.Code {
		case ErrorInvalidScope, ErrorAccessDenied:
			return true
		case ErrorInvalidRequest, ErrorInvalidTarget, ErrorUnsupportedResponseType:
			// Detected before redirect validation completes.
			return false
		}
	}
	return errors.Is(err, ErrProviderDenied)
}

// ServiceConfig wires the broker against real persistence and a provider.
type ServiceConfig struct {
	Pool     *pgxpool.Pool
	Secrets  config.BrokerSecretSet
	Provider brokerprovider.Provider
	Issuer   string
	// Now overrides the clock in tests.
	Now func() time.Time
	// Rand overrides the CSPRNG in tests. Production uses crypto/rand.
	Rand func([]byte) error
}

// Service is the IB1 broker. It issues authorization codes only: it performs
// no code redemption, no token exchange, and no JWT signing.
type Service struct {
	pool     *pgxpool.Pool
	secrets  config.BrokerSecretSet
	provider brokerprovider.Provider
	issuer   string
	sealer   *stateseal.Sealer
	now      func() time.Time
	random   func([]byte) error
}

// NewService validates dependencies and composes the sealer before use, so a
// misconfigured broker fails before the process can serve traffic.
func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("broker service: pool is required")
	}
	if cfg.Provider == nil {
		return nil, fmt.Errorf("broker service: provider is required")
	}
	if cfg.Issuer == "" {
		return nil, fmt.Errorf("broker service: issuer is required")
	}
	sealer, err := stateseal.New(stateseal.Config{
		ActiveKeyVersion: cfg.Secrets.StateSealActiveVersion,
		Keys:             cfg.Secrets.StateSealKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("broker service: state seal unavailable")
	}
	if len(cfg.Secrets.StateHashPeppers) == 0 ||
		len(cfg.Secrets.BrokerCookiePeppers) == 0 ||
		len(cfg.Secrets.AuthorizationCodePeppers) == 0 {
		return nil, fmt.Errorf("broker service: secret peppers unavailable")
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	random := cfg.Rand
	if random == nil {
		random = func(b []byte) error { _, err := rand.Read(b); return err }
	}
	return &Service{
		pool: cfg.Pool, secrets: cfg.Secrets, provider: cfg.Provider,
		issuer: cfg.Issuer, sealer: sealer, now: now, random: random,
	}, nil
}

// StartedAuthorization is the result of a validated authorization request.
type StartedAuthorization struct {
	TransactionID uuid.UUID
	// CookieValue is the plaintext temporary broker cookie value. Only its
	// HMAC hash is persisted.
	CookieValue string
	ExpiresAt   time.Time
}

// Authorize resolves the exact registration, then creates a 10-minute broker
// transaction whose recoverable state is sealed with state-seal-v1. The
// plaintext state buffer is zeroed before returning on every path.
func (s *Service) Authorize(ctx context.Context, req AuthorizeRequest) (StartedAuthorization, error) {
	defer stateseal.Zero(req.State)

	registration, err := repo.ResolveRegistration(ctx, s.pool, req.ClientID, req.RedirectURI, req.ResourceURI, req.Audience)
	if err != nil {
		// Registration lookup failure is local: no redirect target is trusted.
		return StartedAuthorization{}, fmt.Errorf("%w: %s", ErrUnboundCallback, "registration not found")
	}
	if !scopesWithin(req.Scopes, registration.AllowedScopes) {
		return StartedAuthorization{}, oauthErr(ErrorInvalidScope, "scope is not registered")
	}

	transactionID := uuid.New()
	sealed, keyVersion, err := s.sealer.Seal(req.State, stateseal.Bindings{
		TransactionID: transactionID, OAuthClientID: registration.OAuthClientID,
		RedirectURI: registration.RedirectURI, ResourceURI: registration.ResourceURI,
		Audience: registration.Audience,
	})
	if err != nil {
		return StartedAuthorization{}, fmt.Errorf("broker authorize: state seal failed")
	}

	stateHash, err := secrethash.Hash(secrethash.Peppers(s.secrets.StateHashPeppers),
		s.secrets.StateHashActiveVersion, stateHashContext, req.State)
	if err != nil {
		return StartedAuthorization{}, fmt.Errorf("broker authorize: state hash failed")
	}

	cookieValue, err := s.randomToken(CookieEntropyBytes)
	if err != nil {
		return StartedAuthorization{}, fmt.Errorf("broker authorize: cookie generation failed")
	}
	cookieHash, err := secrethash.Hash(secrethash.Peppers(s.secrets.BrokerCookiePeppers),
		s.secrets.BrokerCookieActiveVersion, cookieHashContext, []byte(cookieValue))
	if err != nil {
		return StartedAuthorization{}, fmt.Errorf("broker authorize: cookie hash failed")
	}

	created := s.now().UTC()
	expires := created.Add(BrokerTransactionTTL)

	// The transaction id is generated here so it can bind the seal AAD; the
	// insert must use that exact id.
	txn, err := s.createTransaction(ctx, transactionID, domain.CreateBrokerTransactionInput{
		OAuthClientID: registration.OAuthClientID, RedirectID: registration.ID,
		StateHash: stateHash, StatePepperVersion: int16(s.secrets.StateHashActiveVersion),
		StateSealed: sealed, StateKeyVersion: int16(keyVersion), StateLength: int16(len(req.State)),
		PKCEChallenge: req.CodeChallenge, PKCEMethod: req.CodeMethod,
		RequestedScopes: req.Scopes, ResourceURI: registration.ResourceURI,
		Audience: registration.Audience, BrokerCookieHash: cookieHash,
		BrokerCookiePepperVersion: int16(s.secrets.BrokerCookieActiveVersion),
		CreatedAt:                 created, ExpiresAt: expires,
	})
	if err != nil {
		return StartedAuthorization{}, fmt.Errorf("broker authorize: transaction not created")
	}

	return StartedAuthorization{TransactionID: txn, CookieValue: cookieValue, ExpiresAt: expires}, nil
}

// createTransaction inserts the row with an explicit id so the sealed-state
// AAD binding matches the persisted transaction exactly.
func (s *Service) createTransaction(ctx context.Context, id uuid.UUID, in domain.CreateBrokerTransactionInput) (uuid.UUID, error) {
	if err := domain.ValidateCreateBrokerTransactionInput(in); err != nil {
		return uuid.Nil, err
	}
	var out uuid.UUID
	err := s.pool.QueryRow(ctx, `
INSERT INTO broker_transactions(
  id,oauth_client_id,redirect_id,state_hash,state_pepper_version,state_sealed,state_key_version,
  state_length,pkce_challenge,pkce_method,requested_scopes,resource_uri,audience,
  broker_cookie_hash,broker_cookie_pepper_version,created_at,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
RETURNING id`,
		id, in.OAuthClientID, in.RedirectID, in.StateHash, in.StatePepperVersion,
		in.StateSealed, in.StateKeyVersion, in.StateLength, in.PKCEChallenge, in.PKCEMethod,
		in.RequestedScopes, in.ResourceURI, in.Audience, in.BrokerCookieHash,
		in.BrokerCookiePepperVersion, in.CreatedAt, in.ExpiresAt,
	).Scan(&out)
	return out, err
}

// CallbackInput is the Identity-owned callback request.
type CallbackInput struct {
	// CookieValue is the presented temporary broker cookie value.
	CookieValue string
	// Artifact is the one-time provider callback artifact. It is consumed
	// server-side in memory and never persisted or reflected.
	Artifact string
	// Type is the recognized callback artifact class (token vs sso_token plus
	// the allowed Stytch authenticate surface). Production adapters require it.
	Type brokerprovider.ArtifactType
	// EmailAddress is required to verify an email OTP. It is never persisted.
	EmailAddress string
	// OrganizationID selects organization-scoped OTP or magic-link completion.
	OrganizationID string
}

// CallbackResult is returned only after the issuance transaction commits.
type CallbackResult struct {
	RedirectURI string
	// Code is the plaintext one-use Primer authorization code. Only its HMAC
	// hash is persisted; this value is not recoverable for response replay.
	Code string
	// State is the exact original state recovered from the sealed envelope.
	State  []byte
	Issuer string

	AccountID     uuid.UUID
	TransactionID uuid.UUID
}

// CompleteCallback validates the broker binding, consumes the provider
// artifact in memory, maps the exact tuple without email merge, and issues
// exactly one authorization code in a serializable transaction that commits
// before any redirect is constructed.
func (s *Service) CompleteCallback(ctx context.Context, in CallbackInput) (CallbackResult, error) {
	if in.CookieValue == "" || len(in.CookieValue) > maxCookieValueLen ||
		in.Artifact == "" || len(in.Artifact) > maxArtifactByteLen {
		return CallbackResult{}, ErrUnboundCallback
	}

	cookieHash, err := secrethash.Hash(secrethash.Peppers(s.secrets.BrokerCookiePeppers),
		s.secrets.BrokerCookieActiveVersion, cookieHashContext, []byte(in.CookieValue))
	if err != nil {
		return CallbackResult{}, ErrUnboundCallback
	}

	binding, err := s.loadLiveBinding(ctx, cookieHash)
	if err != nil {
		return CallbackResult{}, err
	}

	// Advance through the exact state machine
	// (pending → provider_started → provider_validating) before calling the
	// provider, so a crash cannot leave a pending transaction that looks
	// untouched. Each hop is a CAS; losing one means a concurrent callback is
	// already driving this transaction.
	if err := s.advanceToValidating(ctx, &binding); err != nil {
		return CallbackResult{}, err
	}

	result, providerErr := s.completeProviderCallback(ctx, in)
	if providerErr != nil {
		switch {
		case errors.Is(providerErr, brokerprovider.ErrProviderUnavailable):
			// Transient: release ownership and leave the transaction
			// recoverable. Never negative-cached, never terminalized.
			if err := s.releaseValidating(ctx, &binding); err != nil {
				return CallbackResult{}, ErrProviderUnavailable
			}
			return CallbackResult{}, ErrProviderUnavailable
		default:
			s.terminalize(ctx, binding, domain.BrokerStatusDenied)
			return CallbackResult{}, ErrProviderDenied
		}
	}
	if result.Outcome == brokerprovider.OutcomeIncompleteMFA {
		// Identity-only continuation: no authority, transaction stays live.
		if err := s.releaseValidating(ctx, &binding); err != nil {
			return CallbackResult{}, ErrIncompleteMFA
		}
		return CallbackResult{}, ErrIncompleteMFA
	}
	if !result.Authenticated() {
		s.terminalize(ctx, binding, domain.BrokerStatusDenied)
		return CallbackResult{}, ErrProviderDenied
	}

	// Exact tuple mapping. Email never links identities. Serializable first-login
	// races are retried here so a concurrent sibling test or login cannot surface
	// a raw SQLSTATE to the caller.
	var account *domain.Account
	for attempt := 1; attempt <= 5; attempt++ {
		account, err = repo.ResolveOrCreateStytchMapping(ctx, s.pool, domain.StytchMappingInput{
			StytchPrincipal: domain.StytchPrincipal{
				ProjectID: result.ProjectID, OrganizationID: result.OrganizationID,
				MemberID: result.MemberID,
			},
			DisplayName: "Primer member",
		})
		if err == nil {
			break
		}
		if !isRetryableMapping(err) || attempt == 5 {
			return CallbackResult{}, fmt.Errorf("broker callback: tuple mapping unavailable")
		}
	}
	mappingID, err := s.mappingID(ctx, result)
	if err != nil {
		return CallbackResult{}, fmt.Errorf("broker callback: tuple mapping unavailable")
	}

	code, err := s.randomToken(CodeEntropyBytes)
	if err != nil {
		return CallbackResult{}, fmt.Errorf("broker callback: code generation failed")
	}
	codeHash, err := secrethash.Hash(secrethash.Peppers(s.secrets.AuthorizationCodePeppers),
		s.secrets.AuthorizationCodeActiveVersion, codeHashContext, []byte(code))
	if err != nil {
		return CallbackResult{}, fmt.Errorf("broker callback: code hash failed")
	}

	issuedAt := s.now().UTC()
	grantNotAfter, codeExpiresAt, err := capGrantAndCode(issuedAt, result.MemberSessionExpiresAt)
	if err != nil {
		s.terminalize(ctx, binding, domain.BrokerStatusDenied)
		return CallbackResult{}, ErrProviderDenied
	}
	_, err = repo.IssueCallbackArtifacts(ctx, s.pool, repo.IssueCallbackArtifactsInput{
		BrokerID: binding.transactionID, ExpectedVersion: binding.version,
		AccountID: account.ID, MappingID: mappingID,
		ProviderProjectID: result.ProjectID, ProviderOrganizationID: result.OrganizationID,
		ProviderMemberID: result.MemberID, ProviderMemberSessionID: result.MemberSessionID,
		ProviderExpiresAt: result.MemberSessionExpiresAt, LastValidatedAt: issuedAt,
		OAuthClientID: binding.oauthClientID, RedirectURI: binding.redirectURI,
		ResourceURI: binding.resourceURI, Audience: binding.audience,
		Scopes: binding.scopes, PKCEChallenge: binding.pkceChallenge,
		PKCEMethod: binding.pkceMethod, CodeHash: codeHash,
		PepperVersion: int16(s.secrets.AuthorizationCodeActiveVersion),
		IssuedAt:      issuedAt, ExpiresAt: codeExpiresAt,
		GrantNotAfter: grantNotAfter,
	})
	if err != nil {
		if errors.Is(err, repo.ErrStaleCAS) {
			// A concurrent callback committed first. The loser never receives
			// the winner's code.
			return CallbackResult{}, ErrLostRace
		}
		return CallbackResult{}, fmt.Errorf("broker callback: issuance failed")
	}

	// Only after commit is the exact original state recovered for the redirect.
	state, err := s.sealer.Open(binding.stateSealed, int(binding.stateKeyVersion), stateseal.Bindings{
		TransactionID: binding.transactionID, OAuthClientID: binding.oauthClientID,
		RedirectURI: binding.redirectURI, ResourceURI: binding.resourceURI,
		Audience: binding.audience,
	})
	if err != nil {
		return CallbackResult{}, fmt.Errorf("broker callback: state recovery failed")
	}
	recovered := append([]byte(nil), state...)
	stateseal.Zero(state)

	return CallbackResult{
		RedirectURI: binding.redirectURI, Code: code, State: recovered,
		Issuer: s.issuer, AccountID: account.ID, TransactionID: binding.transactionID,
	}, nil
}

func (s *Service) completeProviderCallback(ctx context.Context, in CallbackInput) (brokerprovider.CallbackResult, error) {
	if typed, ok := s.provider.(brokerprovider.TypedProvider); ok {
		return typed.CompleteTypedCallback(ctx, brokerprovider.CallbackRequest{
			Type:           in.Type,
			Artifact:       in.Artifact,
			EmailAddress:   in.EmailAddress,
			OrganizationID: in.OrganizationID,
		})
	}
	return s.provider.CompleteCallback(ctx, in.Artifact)
}

func capGrantAndCode(issuedAt, providerExpiresAt time.Time) (grantNotAfter, codeExpiresAt time.Time, err error) {
	if !providerExpiresAt.After(issuedAt) {
		return time.Time{}, time.Time{}, ErrProviderDenied
	}
	grantNotAfter = issuedAt.Add(GrantLifetime)
	if providerExpiresAt.Before(grantNotAfter) {
		grantNotAfter = providerExpiresAt
	}
	codeExpiresAt = issuedAt.Add(AuthorizationCodeTTL)
	if providerExpiresAt.Before(codeExpiresAt) {
		codeExpiresAt = providerExpiresAt
	}
	if !codeExpiresAt.After(issuedAt) || !grantNotAfter.After(issuedAt) {
		return time.Time{}, time.Time{}, ErrProviderDenied
	}
	return grantNotAfter, codeExpiresAt, nil
}

type liveBinding struct {
	transactionID   uuid.UUID
	oauthClientID   uuid.UUID
	redirectURI     string
	resourceURI     string
	audience        string
	scopes          []string
	pkceChallenge   string
	pkceMethod      string
	stateSealed     []byte
	stateKeyVersion int16
	status          string
	version         int64
}

// loadLiveBinding resolves a nonterminal, unexpired transaction by exact
// cookie hash. Anything else is a single non-oracular unbound error.
func (s *Service) loadLiveBinding(ctx context.Context, cookieHash []byte) (liveBinding, error) {
	var b liveBinding
	err := s.pool.QueryRow(ctx, `
SELECT t.id, t.oauth_client_id, r.redirect_uri, t.resource_uri, t.audience,
       t.requested_scopes, t.pkce_challenge, t.pkce_method, t.state_sealed,
       t.state_key_version, t.status, t.version
FROM broker_transactions t
JOIN oauth_client_redirects r ON r.id = t.redirect_id
WHERE t.broker_cookie_hash = $1
  AND t.status IN ('pending','provider_started','provider_validating')
  AND t.expires_at > now()`, cookieHash).Scan(
		&b.transactionID, &b.oauthClientID, &b.redirectURI, &b.resourceURI, &b.audience,
		&b.scopes, &b.pkceChallenge, &b.pkceMethod, &b.stateSealed,
		&b.stateKeyVersion, &b.status, &b.version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return liveBinding{}, ErrUnboundCallback
		}
		return liveBinding{}, ErrUnboundCallback
	}
	return b, nil
}

// advanceToValidating acquires exclusive ownership of the callback by winning
// the CAS chain into provider_validating. A transaction already in
// provider_validating is owned by another in-flight callback, so this loses
// the race rather than calling the provider twice. Exclusive ownership is what
// makes concurrent callbacks resolve to exactly one committed code.
func (s *Service) advanceToValidating(ctx context.Context, b *liveBinding) error {
	if b.status == domain.BrokerStatusProviderValidating {
		return ErrLostRace
	}
	for b.status != domain.BrokerStatusProviderValidating {
		var next string
		switch b.status {
		case domain.BrokerStatusPending:
			next = domain.BrokerStatusProviderStarted
		case domain.BrokerStatusProviderStarted:
			next = domain.BrokerStatusProviderValidating
		default:
			return ErrUnboundCallback
		}
		if _, err := repo.TransitionBrokerTransaction(ctx, s.pool, b.transactionID,
			b.version, b.status, next); err != nil {
			return ErrLostRace
		}
		b.status = next
		b.version++
	}
	return nil
}

// releaseValidating hands ownership back after a non-terminal outcome
// (transient provider fault or MFA continuation) so the human can retry. It
// deliberately does not terminalize and does not clear sealed state.
func (s *Service) releaseValidating(ctx context.Context, b *liveBinding) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE broker_transactions
SET status='provider_started', version=version+1
WHERE id=$1 AND version=$2 AND status='provider_validating'`,
		b.transactionID, b.version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrLostRace
	}
	b.status = domain.BrokerStatusProviderStarted
	b.version++
	return nil
}

// terminalize best-effort moves a transaction to a terminal status, nulling
// sealed state. A lost CAS is ignored: the winner already terminalized it.
func (s *Service) terminalize(ctx context.Context, b liveBinding, status string) {
	_, _ = repo.TransitionBrokerTransaction(ctx, s.pool, b.transactionID, b.version,
		domain.BrokerStatusProviderValidating, status)
}

// mappingID reads the stytch mapping row id for the exact tuple.
func (s *Service) mappingID(ctx context.Context, result brokerprovider.CallbackResult) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
SELECT id FROM stytch_mappings
WHERE project_id=$1 AND organization_id=$2 AND member_id=$3`,
		result.ProjectID, result.OrganizationID, result.MemberID).Scan(&id)
	return id, err
}

func isRetryableMapping(err error) bool {
	if err == nil {
		return false
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code == "40001" || pe.Code == "40P01"
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 40001") || strings.Contains(msg, "SQLSTATE 40P01")
}

// randomToken returns an unpadded base64url CSPRNG token.
func (s *Service) randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if err := s.random(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// scopesWithin requires every requested scope to be registered.
func scopesWithin(requested, allowed []string) bool {
	permitted := make(map[string]struct{}, len(allowed))
	for _, scope := range allowed {
		permitted[scope] = struct{}{}
	}
	for _, scope := range requested {
		if _, ok := permitted[scope]; !ok {
			return false
		}
	}
	return len(requested) > 0
}
