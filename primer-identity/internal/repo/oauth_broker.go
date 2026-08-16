package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrStaleCAS = errors.New("stale CAS")

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
	err := q.QueryRow(ctx, `UPDATE broker_transactions SET status=$1::varchar,version=version+1,provider_started_at=CASE WHEN $1::varchar='provider_started' THEN now() ELSE provider_started_at END,provider_validating_at=CASE WHEN $1::varchar='provider_validating' THEN now() ELSE provider_validating_at END,completed_at=CASE WHEN $1::varchar IN ('authorized','denied','failed','expired') THEN now() ELSE completed_at END,state_sealed=CASE WHEN $1::varchar IN ('authorized','denied','failed','expired') THEN NULL ELSE state_sealed END,provider_code_sealed=CASE WHEN $1::varchar IN ('authorized','denied','failed','expired') THEN NULL ELSE provider_code_sealed END WHERE id=$2 AND version=$3 AND status=$4 RETURNING id,status,version,state_sealed,provider_code_sealed`, to, id, version, from).Scan(&b.ID, &b.Status, &b.Version, &b.StateSealed, &b.ProviderCodeSealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStaleCAS
	}
	return b, wrapf("transition broker", err)
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
func isTerminal(s string) bool {
	return s == domain.BrokerStatusDenied || s == domain.BrokerStatusFailed || s == domain.BrokerStatusExpired
}
func validateRegistration(a, b, c string) error {
	if a == "" || b == "" || c == "" || len(a) > 2048 || len(b) > 2048 || len(c) > 128 {
		return wrapf("registration", domain.ErrInvalid)
	}
	return nil
}
func WithSerializableRetry(ctx context.Context, p *pgxpool.Pool, fn func(pgx.Tx) error) error {
	for i := 0; i < 3; i++ {
		tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return err
		}
		err = fn(tx)
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		var pe *pgconn.PgError
		if errors.As(err, &pe) && (pe.Code == "40001" || pe.Code == "40P01") && i < 2 {
			continue
		}
		return err
	}
	return fmt.Errorf("serializable retry exhausted")
}
func PurgeBrokerTransactions(ctx context.Context, q Querier, now time.Time) (int64, error) {
	tag, err := q.Exec(ctx, `WITH deleted_codes AS (DELETE FROM oauth_authorization_codes WHERE issued_at <= $1 - interval '24 hours') DELETE FROM broker_transactions WHERE created_at <= $1 - interval '24 hours'`, now)
	return tag.RowsAffected(), wrapf("purge broker transactions", err)
}
