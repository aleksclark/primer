package repo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FailureClass is an internal, closed diagnostic vocabulary. Never classify by
// error text: it can contain SQL values, identifiers or credential material.
type FailureClass uint8

const (
	FailureUnknown FailureClass = iota
	FailureSerialization
	FailureDeadlock
	FailureNotFound
	FailureConflict
	FailureInvalid
	FailureCanceled
	FailureDeadline
	FailureCapacity
	FailureQueryCanceled
	FailureUnavailable
	FailureDenied
	FailureIncomplete
)

func (c FailureClass) String() string {
	switch c {
	case FailureSerialization:
		return "serialization_conflict"
	case FailureDeadlock:
		return "deadlock"
	case FailureNotFound:
		return "not_found"
	case FailureConflict:
		return "conflict"
	case FailureInvalid:
		return "invalid"
	case FailureCanceled:
		return "canceled"
	case FailureDeadline:
		return "deadline"
	case FailureCapacity:
		return "capacity"
	case FailureQueryCanceled:
		return "query_canceled"
	case FailureUnavailable:
		return "unavailable"
	case FailureDenied:
		return "denied"
	case FailureIncomplete:
		return "incomplete"
	default:
		return "unknown"
	}
}

func ClassifyFailure(err error) FailureClass {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "40001":
			return FailureSerialization
		case "40P01":
			return FailureDeadlock
		case "23505":
			return FailureConflict
		case "23503":
			return FailureNotFound
		case "23514":
			return FailureInvalid
		case "53300":
			return FailureCapacity
		case "57014":
			return FailureQueryCanceled
		default:
			return FailureUnknown
		}
	}
	switch {
	case errors.Is(err, domain.ErrRetryableSerialization):
		return FailureSerialization
	case errors.Is(err, context.Canceled):
		return FailureCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return FailureDeadline
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, domain.ErrNotFound):
		return FailureNotFound
	case errors.Is(err, domain.ErrConflict), errors.Is(err, ErrStaleCAS):
		return FailureConflict
	case errors.Is(err, domain.ErrInvalid):
		return FailureInvalid
	default:
		return FailureUnknown
	}
}

// RetryFailureInfo contains only safe scalars. Its private fields prevent raw
// strings from entering a diagnostic projection. Recovered retries emit nothing.
type RetryFailureInfo struct {
	class           FailureClass
	attempts, limit int
	exhausted       bool
}

func (d RetryFailureInfo) Class() FailureClass { return d.class }
func (d RetryFailureInfo) Attempts() int       { return d.attempts }
func (d RetryFailureInfo) Limit() int          { return d.limit }
func (d RetryFailureInfo) Exhausted() bool     { return d.exhausted }

type retryFailure struct {
	cause error
	info  RetryFailureInfo
}

func (e *retryFailure) Error() string { return e.cause.Error() }
func (e *retryFailure) Unwrap() error { return e.cause }
func withRetryFailure(err error, attempts, limit int, exhausted bool) error {
	if err == nil {
		return nil
	}
	return &retryFailure{cause: err, info: RetryFailureInfo{ClassifyFailure(err), attempts, limit, exhausted}}
}
func RetryFailureDetails(err error) RetryFailureInfo {
	var failure *retryFailure
	if errors.As(err, &failure) {
		return failure.info
	}
	return RetryFailureInfo{class: ClassifyFailure(err)}
}

var ErrStaleCAS = errors.New("stale CAS")

type IssueCallbackArtifactsInput struct {
	BrokerID                                  uuid.UUID
	ExpectedVersion                           int64
	AccountID, MappingID                      uuid.UUID
	ProviderProjectID, ProviderOrganizationID string
	ProviderMemberID, ProviderMemberSessionID string
	ProviderExpiresAt, LastValidatedAt        time.Time
	OAuthClientID                             uuid.UUID
	RedirectURI, ResourceURI, Audience        string
	Scopes                                    []string
	PKCEChallenge, PKCEMethod                 string
	CodeHash                                  []byte
	PepperVersion                             int16
	IssuedAt, ExpiresAt, GrantNotAfter        time.Time
}

type IssuedCallbackArtifacts struct {
	Association *domain.ProviderSessionAssociation
	Grant       *domain.OAuthGrant
	Code        *domain.OAuthAuthorizationCode
	Broker      *domain.BrokerTransaction
}

func CreateOAuthClient(ctx context.Context, q Querier, c domain.OAuthClient) (*domain.OAuthClient, error) {
	if c.ClientID == "" || c.Name == "" || c.ClientType == "" || c.TokenEndpointAuthMethod == "" || len(c.AllowedGrants) == 0 {
		return nil, wrapf("create oauth client", domain.ErrInvalid)
	}
	r := &domain.OAuthClient{}
	err := q.QueryRow(ctx, `INSERT INTO oauth_clients(client_id,name,client_type,token_endpoint_auth_method,client_secret_hash,client_secret_pepper_version,allowed_grants,enabled,disabled_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,client_id,name,client_type,token_endpoint_auth_method,client_secret_hash,client_secret_pepper_version,allowed_grants,enabled,created_at,updated_at,disabled_at`, c.ClientID, c.Name, c.ClientType, c.TokenEndpointAuthMethod, c.ClientSecretHash, c.ClientSecretPepperVersion, c.AllowedGrants, c.Enabled, c.DisabledAt).Scan(&r.ID, &r.ClientID, &r.Name, &r.ClientType, &r.TokenEndpointAuthMethod, &r.ClientSecretHash, &r.ClientSecretPepperVersion, &r.AllowedGrants, &r.Enabled, &r.CreatedAt, &r.UpdatedAt, &r.DisabledAt)
	return r, wrapf("create oauth client", err)
}

func CreateOAuthClientRedirect(ctx context.Context, q Querier, r domain.OAuthClientRedirect) (*domain.OAuthClientRedirect, error) {
	if err := domain.ValidateCanonicalScopes(r.AllowedScopes); err != nil {
		return nil, wrapf("create redirect", err)
	}
	if err := validateRegistration(r.RedirectURI, r.ResourceURI, r.Audience); err != nil {
		return nil, err
	}
	out := &domain.OAuthClientRedirect{}
	err := q.QueryRow(ctx, `INSERT INTO oauth_client_redirects(oauth_client_id,redirect_uri,resource_uri,audience,allowed_scopes,enabled) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,oauth_client_id,redirect_uri,resource_uri,audience,allowed_scopes,enabled,created_at,updated_at`, r.OAuthClientID, r.RedirectURI, r.ResourceURI, r.Audience, r.AllowedScopes, r.Enabled).Scan(&out.ID, &out.OAuthClientID, &out.RedirectURI, &out.ResourceURI, &out.Audience, &out.AllowedScopes, &out.Enabled, &out.CreatedAt, &out.UpdatedAt)
	return out, wrapf("create redirect", err)
}

func ResolveRegistration(ctx context.Context, q Querier, clientID, redirectURI, resourceURI, audience string) (*domain.OAuthClientRedirect, error) {
	out := &domain.OAuthClientRedirect{}
	err := q.QueryRow(ctx, `SELECT r.id,r.oauth_client_id,r.redirect_uri,r.resource_uri,r.audience,r.allowed_scopes,r.enabled,r.created_at,r.updated_at FROM oauth_clients c JOIN oauth_client_redirects r ON r.oauth_client_id=c.id WHERE c.client_id=$1 AND c.enabled AND r.enabled AND r.redirect_uri=$2 AND r.resource_uri=$3 AND r.audience=$4`, clientID, redirectURI, resourceURI, audience).Scan(&out.ID, &out.OAuthClientID, &out.RedirectURI, &out.ResourceURI, &out.Audience, &out.AllowedScopes, &out.Enabled, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("resolve registration", domain.ErrNotFound)
	}
	return out, wrapf("resolve registration", err)
}

func CreateBrokerTransaction(ctx context.Context, q Querier, in domain.CreateBrokerTransactionInput) (*domain.BrokerTransaction, error) {
	if err := domain.ValidateCreateBrokerTransactionInput(in); err != nil {
		return nil, wrapf("create broker transaction", err)
	}
	b := &domain.BrokerTransaction{}
	err := q.QueryRow(ctx, `INSERT INTO broker_transactions(oauth_client_id,redirect_id,state_hash,state_pepper_version,state_sealed,state_key_version,state_length,provider_code_sealed,provider_code_key_version,pkce_challenge,pkce_method,requested_scopes,resource_uri,audience,broker_cookie_hash,broker_cookie_pepper_version,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING id,oauth_client_id,redirect_id,state_hash,state_pepper_version,state_sealed,state_key_version,state_length,provider_code_sealed,provider_code_key_version,pkce_challenge,pkce_method,requested_scopes,resource_uri,audience,broker_cookie_hash,broker_cookie_pepper_version,status,created_at,expires_at,version`, in.OAuthClientID, in.RedirectID, in.StateHash, in.StatePepperVersion, in.StateSealed, in.StateKeyVersion, in.StateLength, in.ProviderCodeSealed, in.ProviderCodeKeyVersion, in.PKCEChallenge, in.PKCEMethod, in.RequestedScopes, in.ResourceURI, in.Audience, in.BrokerCookieHash, in.BrokerCookiePepperVersion, in.CreatedAt, in.ExpiresAt).Scan(&b.ID, &b.OAuthClientID, &b.RedirectID, &b.StateHash, &b.StatePepperVersion, &b.StateSealed, &b.StateKeyVersion, &b.StateLength, &b.ProviderCodeSealed, &b.ProviderCodeKeyVersion, &b.PKCEChallenge, &b.PKCEMethod, &b.RequestedScopes, &b.ResourceURI, &b.Audience, &b.BrokerCookieHash, &b.BrokerCookiePepperVersion, &b.Status, &b.CreatedAt, &b.ExpiresAt, &b.Version)
	return b, wrapf("create broker transaction", err)
}

func TransitionBrokerTransaction(ctx context.Context, q Querier, id uuid.UUID, version int64, from, to string) (*domain.BrokerTransaction, error) {
	if !validTransition(from, to) {
		return nil, wrapf("transition broker", domain.ErrInvalid)
	}
	b := &domain.BrokerTransaction{}
	err := q.QueryRow(ctx, `UPDATE broker_transactions SET status=$1::varchar,version=version+1,provider_started_at=CASE WHEN $1::varchar='provider_started' THEN now() ELSE provider_started_at END,provider_validating_at=CASE WHEN $1::varchar='provider_validating' THEN now() ELSE provider_validating_at END,completed_at=CASE WHEN $1::varchar IN ('authorized','denied','failed','expired') THEN now() ELSE completed_at END,state_sealed=CASE WHEN $1::varchar IN ('authorized','denied','failed','expired') THEN NULL ELSE state_sealed END,provider_code_sealed=CASE WHEN $1::varchar IN ('authorized','denied','failed','expired') THEN NULL ELSE provider_code_sealed END,provider_code_key_version=CASE WHEN $1::varchar IN ('authorized','denied','failed','expired') THEN NULL ELSE provider_code_key_version END WHERE id=$2 AND version=$3 AND status=$4 RETURNING id,status,version,state_sealed,provider_code_sealed`, to, id, version, from).Scan(&b.ID, &b.Status, &b.Version, &b.StateSealed, &b.ProviderCodeSealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStaleCAS
	}
	return b, wrapf("transition broker", err)
}

func CreateProviderSessionAssociation(ctx context.Context, q Querier, in domain.ProviderSessionAssociation) (*domain.ProviderSessionAssociation, error) {
	if in.Provider == "" {
		in.Provider = "stytch_b2b"
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if err := domain.ValidateProviderSessionAssociation(in); err != nil {
		return nil, wrapf("create provider session association", err)
	}
	out := &domain.ProviderSessionAssociation{}
	err := q.QueryRow(ctx, `
INSERT INTO provider_session_associations(
  account_id,stytch_mapping_id,provider,provider_project_id,provider_organization_id,
  provider_member_id,provider_member_session_id,provider_expires_at,status,last_validated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id,account_id,stytch_mapping_id,provider,provider_project_id,provider_organization_id,provider_member_id,provider_member_session_id,provider_expires_at,status,last_validated_at,revoked_at,revoke_reason_code,created_at,updated_at`,
		in.AccountID, in.StytchMappingID, in.Provider, in.ProviderProjectID, in.ProviderOrganizationID,
		in.ProviderMemberID, in.ProviderMemberSessionID, in.ProviderExpiresAt, in.Status, in.LastValidatedAt,
	).Scan(&out.ID, &out.AccountID, &out.StytchMappingID, &out.Provider, &out.ProviderProjectID, &out.ProviderOrganizationID, &out.ProviderMemberID, &out.ProviderMemberSessionID, &out.ProviderExpiresAt, &out.Status, &out.LastValidatedAt, &out.RevokedAt, &out.RevokeReasonCode, &out.CreatedAt, &out.UpdatedAt)
	return out, wrapf("create provider session association", err)
}

func GetProviderSessionAssociation(ctx context.Context, q Querier, id uuid.UUID) (*domain.ProviderSessionAssociation, error) {
	if id == uuid.Nil {
		return nil, wrapf("get provider session association", fmt.Errorf("%w: nil id", domain.ErrInvalid))
	}
	out := &domain.ProviderSessionAssociation{}
	err := q.QueryRow(ctx, `
SELECT id,account_id,stytch_mapping_id,provider,provider_project_id,provider_organization_id,provider_member_id,provider_member_session_id,provider_expires_at,status,last_validated_at,revoked_at,revoke_reason_code,created_at,updated_at
FROM provider_session_associations WHERE id=$1`, id).Scan(
		&out.ID, &out.AccountID, &out.StytchMappingID, &out.Provider, &out.ProviderProjectID, &out.ProviderOrganizationID, &out.ProviderMemberID, &out.ProviderMemberSessionID, &out.ProviderExpiresAt, &out.Status, &out.LastValidatedAt, &out.RevokedAt, &out.RevokeReasonCode, &out.CreatedAt, &out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("get provider session association", domain.ErrNotFound)
	}
	return out, wrapf("get provider session association", err)
}

func CreateOAuthGrant(ctx context.Context, q Querier, in domain.OAuthGrant) (*domain.OAuthGrant, error) {
	if in.Status == "" {
		in.Status = "active"
	}
	if in.GrantedAt.IsZero() {
		in.GrantedAt = time.Now().UTC()
	}
	if err := domain.ValidateOAuthGrant(in); err != nil {
		return nil, wrapf("create oauth grant", err)
	}
	out := &domain.OAuthGrant{}
	err := q.QueryRow(ctx, `
INSERT INTO oauth_grants(account_id,service_principal_id,oauth_client_id,provider_session_association_id,resource_uri,audience,scopes,subject_class,status,granted_at,not_after)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING id,account_id,service_principal_id,oauth_client_id,provider_session_association_id,resource_uri,audience,scopes,subject_class,status,granted_at,not_after,revoked_at,revoke_reason_code,version`,
		in.AccountID, in.ServicePrincipalID, in.OAuthClientID, in.ProviderSessionAssociationID, in.ResourceURI, in.Audience, in.Scopes, in.SubjectClass, in.Status, in.GrantedAt, in.NotAfter,
	).Scan(&out.ID, &out.AccountID, &out.ServicePrincipalID, &out.OAuthClientID, &out.ProviderSessionAssociationID, &out.ResourceURI, &out.Audience, &out.Scopes, &out.SubjectClass, &out.Status, &out.GrantedAt, &out.NotAfter, &out.RevokedAt, &out.RevokeReasonCode, &out.Version)
	return out, wrapf("create oauth grant", err)
}

func CreateAuthorizationCode(ctx context.Context, q Querier, in domain.OAuthAuthorizationCode) (*domain.OAuthAuthorizationCode, error) {
	if in.IssuedAt.IsZero() {
		in.IssuedAt = time.Now().UTC()
	}
	if err := domain.ValidateAuthorizationCode(in); err != nil {
		return nil, wrapf("create authorization code", err)
	}
	out := &domain.OAuthAuthorizationCode{}
	err := q.QueryRow(ctx, `
INSERT INTO oauth_authorization_codes(code_hash,pepper_version,grant_id,broker_transaction_id,oauth_client_id,redirect_uri,resource_uri,audience,scopes,pkce_challenge,pkce_method,issued_at,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id,code_hash,pepper_version,grant_id,broker_transaction_id,oauth_client_id,redirect_uri,resource_uri,audience,scopes,pkce_challenge,pkce_method,issued_at,expires_at,consumed_at`,
		in.CodeHash, in.PepperVersion, in.GrantID, in.BrokerTransactionID, in.OAuthClientID, in.RedirectURI, in.ResourceURI, in.Audience, in.Scopes, in.PKCEChallenge, in.PKCEMethod, in.IssuedAt, in.ExpiresAt,
	).Scan(&out.ID, &out.CodeHash, &out.PepperVersion, &out.GrantID, &out.BrokerTransactionID, &out.OAuthClientID, &out.RedirectURI, &out.ResourceURI, &out.Audience, &out.Scopes, &out.PKCEChallenge, &out.PKCEMethod, &out.IssuedAt, &out.ExpiresAt, &out.ConsumedAt)
	return out, wrapf("create authorization code", err)
}

func GetAuthorizationCode(ctx context.Context, q Querier, id uuid.UUID) (*domain.OAuthAuthorizationCode, error) {
	if id == uuid.Nil {
		return nil, wrapf("get authorization code", fmt.Errorf("%w: nil id", domain.ErrInvalid))
	}
	out := &domain.OAuthAuthorizationCode{}
	err := q.QueryRow(ctx, `
SELECT id,code_hash,pepper_version,grant_id,broker_transaction_id,oauth_client_id,redirect_uri,resource_uri,audience,scopes,pkce_challenge,pkce_method,issued_at,expires_at
FROM oauth_authorization_codes WHERE id=$1`, id).Scan(
		&out.ID, &out.CodeHash, &out.PepperVersion, &out.GrantID, &out.BrokerTransactionID, &out.OAuthClientID, &out.RedirectURI, &out.ResourceURI, &out.Audience, &out.Scopes, &out.PKCEChallenge, &out.PKCEMethod, &out.IssuedAt, &out.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("get authorization code", domain.ErrNotFound)
	}
	return out, wrapf("get authorization code", err)
}

func IssueCallbackArtifacts(ctx context.Context, pool *pgxpool.Pool, in IssueCallbackArtifactsInput) (*IssuedCallbackArtifacts, error) {
	if pool == nil {
		return nil, wrapf("issue callback artifacts", fmt.Errorf("%w: nil pool", domain.ErrInvalid))
	}
	if err := domain.ValidateProviderMemberSessionID(in.ProviderMemberSessionID); err != nil {
		return nil, wrapf("issue callback artifacts", err)
	}
	if err := domain.ValidateAuthorizationCode(domain.OAuthAuthorizationCode{
		CodeHash: in.CodeHash, PepperVersion: in.PepperVersion, PKCEChallenge: in.PKCEChallenge,
		PKCEMethod: in.PKCEMethod, Scopes: in.Scopes, IssuedAt: in.IssuedAt, ExpiresAt: in.ExpiresAt,
	}); err != nil {
		return nil, wrapf("issue callback artifacts", err)
	}

	var out *IssuedCallbackArtifacts
	err := WithSerializableRetry(ctx, pool, func(tx pgx.Tx) error {
		issued, issueErr := issueCallbackArtifactsTx(ctx, tx, in)
		if issueErr != nil {
			return issueErr
		}
		out = issued
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func issueCallbackArtifactsTx(ctx context.Context, tx pgx.Tx, in IssueCallbackArtifactsInput) (*IssuedCallbackArtifacts, error) {
	claimed, err := claimBrokerTransaction(ctx, tx, in.BrokerID, in.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	assoc, err := upsertProviderSessionAssociation(ctx, tx, domain.ProviderSessionAssociation{
		AccountID: in.AccountID, StytchMappingID: in.MappingID, Provider: "stytch_b2b",
		ProviderProjectID: in.ProviderProjectID, ProviderOrganizationID: in.ProviderOrganizationID,
		ProviderMemberID: in.ProviderMemberID, ProviderMemberSessionID: in.ProviderMemberSessionID,
		ProviderExpiresAt: in.ProviderExpiresAt, Status: "active", LastValidatedAt: in.LastValidatedAt,
	})
	if err != nil {
		return nil, err
	}
	grant, err := reuseOrCreateActiveHumanGrant(ctx, tx, domain.OAuthGrant{
		AccountID: &in.AccountID, OAuthClientID: in.OAuthClientID, ProviderSessionAssociationID: &assoc.ID,
		ResourceURI: in.ResourceURI, Audience: in.Audience, Scopes: in.Scopes, SubjectClass: "human",
		Status: "active", GrantedAt: in.IssuedAt, NotAfter: in.GrantNotAfter,
	})
	if err != nil {
		return nil, err
	}
	code, err := CreateAuthorizationCode(ctx, tx, domain.OAuthAuthorizationCode{
		CodeHash: in.CodeHash, PepperVersion: in.PepperVersion, GrantID: grant.ID,
		BrokerTransactionID: in.BrokerID, OAuthClientID: in.OAuthClientID, RedirectURI: in.RedirectURI,
		ResourceURI: in.ResourceURI, Audience: in.Audience, Scopes: in.Scopes,
		PKCEChallenge: in.PKCEChallenge, PKCEMethod: in.PKCEMethod, IssuedAt: in.IssuedAt, ExpiresAt: in.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `
UPDATE broker_transactions
SET account_id=$1, provider_session_association_id=$2, status='authorized', version=version+1,
    completed_at=now(), state_sealed=NULL, provider_code_sealed=NULL, provider_code_key_version=NULL
WHERE id=$3 AND version=$4 AND status='provider_validating'`,
		in.AccountID, assoc.ID, claimed.ID, claimed.Version)
	if err != nil {
		return nil, wrapf("issue callback artifacts", err)
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrStaleCAS
	}
	broker := &domain.BrokerTransaction{ID: claimed.ID, Status: domain.BrokerStatusAuthorized, Version: claimed.Version + 1}
	return &IssuedCallbackArtifacts{Association: assoc, Grant: grant, Code: code, Broker: broker}, nil
}

func claimBrokerTransaction(ctx context.Context, tx pgx.Tx, id uuid.UUID, expectedVersion int64) (*domain.BrokerTransaction, error) {
	out := &domain.BrokerTransaction{}
	err := tx.QueryRow(ctx, `
SELECT id, version, status
FROM broker_transactions
WHERE id=$1 AND version=$2 AND status='provider_validating'
FOR UPDATE`, id, expectedVersion).Scan(&out.ID, &out.Version, &out.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStaleCAS
	}
	if err != nil {
		return nil, wrapf("issue callback artifacts", err)
	}
	return out, nil
}

func upsertProviderSessionAssociation(ctx context.Context, q Querier, in domain.ProviderSessionAssociation) (*domain.ProviderSessionAssociation, error) {
	if in.Provider == "" {
		in.Provider = "stytch_b2b"
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if err := domain.ValidateProviderSessionAssociation(in); err != nil {
		return nil, wrapf("upsert provider session association", err)
	}
	out := &domain.ProviderSessionAssociation{}
	err := q.QueryRow(ctx, `
INSERT INTO provider_session_associations(
  account_id,stytch_mapping_id,provider,provider_project_id,provider_organization_id,
  provider_member_id,provider_member_session_id,provider_expires_at,status,last_validated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (provider,provider_project_id,provider_member_session_id) DO UPDATE
SET last_validated_at = GREATEST(provider_session_associations.last_validated_at, EXCLUDED.last_validated_at),
    provider_expires_at = GREATEST(provider_session_associations.provider_expires_at, EXCLUDED.provider_expires_at),
    updated_at = now()
WHERE provider_session_associations.account_id = EXCLUDED.account_id
  AND provider_session_associations.stytch_mapping_id = EXCLUDED.stytch_mapping_id
  AND provider_session_associations.provider_organization_id = EXCLUDED.provider_organization_id
  AND provider_session_associations.provider_member_id = EXCLUDED.provider_member_id
  AND provider_session_associations.status = 'active'
RETURNING id,account_id,stytch_mapping_id,provider,provider_project_id,provider_organization_id,provider_member_id,provider_member_session_id,provider_expires_at,status,last_validated_at,revoked_at,revoke_reason_code,created_at,updated_at`,
		in.AccountID, in.StytchMappingID, in.Provider, in.ProviderProjectID, in.ProviderOrganizationID,
		in.ProviderMemberID, in.ProviderMemberSessionID, in.ProviderExpiresAt, in.Status, in.LastValidatedAt,
	).Scan(&out.ID, &out.AccountID, &out.StytchMappingID, &out.Provider, &out.ProviderProjectID, &out.ProviderOrganizationID, &out.ProviderMemberID, &out.ProviderMemberSessionID, &out.ProviderExpiresAt, &out.Status, &out.LastValidatedAt, &out.RevokedAt, &out.RevokeReasonCode, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("upsert provider session association", fmt.Errorf("%w: association account, tuple, or status mismatch", domain.ErrConflict))
	}
	return out, wrapf("upsert provider session association", err)
}

func reuseOrCreateActiveHumanGrant(ctx context.Context, q Querier, in domain.OAuthGrant) (*domain.OAuthGrant, error) {
	if in.AccountID == nil || in.ProviderSessionAssociationID == nil {
		return nil, wrapf("reuse oauth grant", domain.ErrInvalid)
	}
	if err := expireActiveHumanGrants(ctx, q, *in.AccountID, in.OAuthClientID, *in.ProviderSessionAssociationID, in.ResourceURI, in.Audience, in.GrantedAt); err != nil {
		return nil, err
	}
	existing, err := getReusableActiveHumanGrant(ctx, q, *in.AccountID, in.OAuthClientID, *in.ProviderSessionAssociationID, in.ResourceURI, in.Audience, in.GrantedAt)
	if err == nil {
		return reuseActiveHumanGrant(existing, in.Scopes)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	created, createErr := CreateOAuthGrant(ctx, q, in)
	if createErr == nil {
		return created, nil
	}
	if !errors.Is(createErr, domain.ErrConflict) {
		return nil, createErr
	}
	existing, err = getReusableActiveHumanGrant(ctx, q, *in.AccountID, in.OAuthClientID, *in.ProviderSessionAssociationID, in.ResourceURI, in.Audience, in.GrantedAt)
	if err != nil {
		return nil, createErr
	}
	return reuseActiveHumanGrant(existing, in.Scopes)
}

func expireActiveHumanGrants(ctx context.Context, q Querier, accountID, clientID, associationID uuid.UUID, resourceURI, audience string, issuedAt time.Time) error {
	_, err := q.Exec(ctx, `
UPDATE oauth_grants
SET status='expired', version=version+1, revoked_at=$6, revoke_reason_code='expired'
WHERE account_id=$1 AND oauth_client_id=$2 AND provider_session_association_id=$3
  AND resource_uri=$4 AND audience=$5 AND subject_class='human' AND status='active'
  AND not_after <= $6`,
		accountID, clientID, associationID, resourceURI, audience, issuedAt,
	)
	return wrapf("expire oauth grant", err)
}

func getReusableActiveHumanGrant(ctx context.Context, q Querier, accountID, clientID, associationID uuid.UUID, resourceURI, audience string, issuedAt time.Time) (*domain.OAuthGrant, error) {
	out := &domain.OAuthGrant{}
	err := q.QueryRow(ctx, `
SELECT id,account_id,service_principal_id,oauth_client_id,provider_session_association_id,resource_uri,audience,scopes,subject_class,status,granted_at,not_after,revoked_at,revoke_reason_code,version
FROM oauth_grants
WHERE account_id=$1 AND oauth_client_id=$2 AND provider_session_association_id=$3
  AND resource_uri=$4 AND audience=$5 AND subject_class='human' AND status='active'
  AND not_after > $6
FOR UPDATE`,
		accountID, clientID, associationID, resourceURI, audience, issuedAt,
	).Scan(&out.ID, &out.AccountID, &out.ServicePrincipalID, &out.OAuthClientID, &out.ProviderSessionAssociationID, &out.ResourceURI, &out.Audience, &out.Scopes, &out.SubjectClass, &out.Status, &out.GrantedAt, &out.NotAfter, &out.RevokedAt, &out.RevokeReasonCode, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("get oauth grant", domain.ErrNotFound)
	}
	return out, wrapf("get oauth grant", err)
}

func reuseActiveHumanGrant(existing *domain.OAuthGrant, requested []string) (*domain.OAuthGrant, error) {
	if !scopesSatisfiedBy(existing.Scopes, requested) {
		return nil, wrapf("reuse oauth grant", fmt.Errorf("%w: grant scope widening is forbidden", domain.ErrConflict))
	}
	return existing, nil
}

func scopesSatisfiedBy(existing, requested []string) bool {
	have := make(map[string]struct{}, len(existing))
	for _, scope := range existing {
		have[scope] = struct{}{}
	}
	for _, scope := range requested {
		if _, ok := have[scope]; !ok {
			return false
		}
	}
	return len(requested) > 0
}

func validTransition(from, to string) bool {
	if from == domain.BrokerStatusPending {
		return to == domain.BrokerStatusProviderStarted || isTerminal(to)
	}
	if from == domain.BrokerStatusProviderStarted {
		return to == domain.BrokerStatusProviderValidating || isTerminal(to)
	}
	if from == domain.BrokerStatusProviderValidating {
		return to == domain.BrokerStatusAuthorized || isTerminal(to)
	}
	return false
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
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 40001") || strings.Contains(msg, "SQLSTATE 40P01")
}

func isTerminal(s string) bool {
	return s == domain.BrokerStatusDenied || s == domain.BrokerStatusFailed || s == domain.BrokerStatusExpired || s == domain.BrokerStatusAuthorized
}

func validateRegistration(a, b, c string) error {
	if a == "" || b == "" || c == "" || len(a) > 2048 || len(b) > 2048 || len(c) > 128 {
		return wrapf("registration", domain.ErrInvalid)
	}
	return nil
}

func WithSerializableRetry(ctx context.Context, p *pgxpool.Pool, fn func(pgx.Tx) error) error {
	const (
		maxAttempts   = 8
		baseBackoff   = 5 * time.Millisecond
		maxBackoff    = 80 * time.Millisecond
		jitterCeiling = 16
	)
	var last error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return withRetryFailure(err, attempt-1, maxAttempts, false)
		}
		tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			retryable := isRetryableSerialization(err)
			if retryable && attempt < maxAttempts {
				last = err
				if sleepErr := sleepSerializableBackoff(ctx, attempt, baseBackoff, maxBackoff, jitterCeiling); sleepErr != nil {
					return withRetryFailure(sleepErr, attempt, maxAttempts, false)
				}
				continue
			}
			return withRetryFailure(err, attempt, maxAttempts, retryable && attempt == maxAttempts)
		}
		err = fn(tx)
		if err == nil {
			err = tx.Commit(ctx)
			if err != nil {
				_ = tx.Rollback(context.Background())
			}
		} else {
			_ = tx.Rollback(context.Background())
		}
		if err == nil {
			return nil
		}
		last = err
		retryable := isRetryableSerialization(err)
		if retryable && attempt < maxAttempts {
			if sleepErr := sleepSerializableBackoff(ctx, attempt, baseBackoff, maxBackoff, jitterCeiling); sleepErr != nil {
				return withRetryFailure(sleepErr, attempt, maxAttempts, false)
			}
			continue
		}
		return withRetryFailure(err, attempt, maxAttempts, retryable && attempt == maxAttempts)
	}
	return withRetryFailure(last, maxAttempts, maxAttempts, true)
}

func sleepSerializableBackoff(ctx context.Context, attempt int, base, capDelay time.Duration, jitterCeiling int) error {
	shift := attempt - 1
	if shift > 4 {
		shift = 4
	}
	delay := base << shift
	if delay > capDelay {
		delay = capDelay
	}
	delay += time.Duration(randIntn(jitterCeiling)) * time.Millisecond
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func randIntn(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func PurgeBrokerTransactions(ctx context.Context, q Querier, now time.Time) (int64, error) {
	tag, err := q.Exec(ctx, `
WITH deleted_codes AS (
  DELETE FROM oauth_authorization_codes WHERE issued_at <= $1::timestamptz - interval '24 hours'
)
DELETE FROM broker_transactions WHERE created_at <= $1::timestamptz - interval '24 hours'`, now)
	return tag.RowsAffected(), wrapf("purge broker transactions", err)
}
