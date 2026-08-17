package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/identity/internal/domain"
)

// IssueAuthorizationCodeTokensInput is the durable authorization-code
// consumption transaction. Signing stays in the caller: persist family, current
// token, and copied audit rows, then invoke BeforeCommit with the open
// transaction so a later issuer can sign and abort without the repo importing
// JWT or key material.
type IssueAuthorizationCodeTokensInput struct {
	Claim        domain.ClaimAuthorizationCodeInput
	Refresh      domain.InitialRefreshIssuance
	Audit        domain.TokenIssuanceAudit
	BeforeCommit func(tx pgx.Tx, issuance *IssuedAuthorizationCodeTokens) error
}

type IssuedAuthorizationCodeTokens struct {
	Code   *domain.OAuthAuthorizationCode
	Family *domain.OAuthRefreshFamily
	Token  *domain.OAuthRefreshToken
	Audit  *domain.TokenIssuanceAudit
}

type InitialRefreshFamily struct {
	Family *domain.OAuthRefreshFamily
	Token  *domain.OAuthRefreshToken
}

func CreateOAuthClientKey(ctx context.Context, q Querier, in domain.OAuthClientKey) (*domain.OAuthClientKey, error) {
	if err := domain.ValidateOAuthClientKey(in); err != nil {
		return nil, wrapf("create oauth client key", err)
	}
	out := &domain.OAuthClientKey{}
	err := q.QueryRow(ctx, `
INSERT INTO oauth_client_keys(oauth_client_id,kid,jwk_json,alg,use,enabled,not_before,not_after,disabled_at)
VALUES($1,$2,$3::jsonb,$4,$5,$6,$7,$8,$9)
RETURNING id,oauth_client_id,kid,jwk_json,alg,use,enabled,not_before,not_after,created_at,disabled_at`,
		in.OAuthClientID, in.Kid, in.JWKJSON, in.Alg, in.Use, in.Enabled, in.NotBefore, in.NotAfter, in.DisabledAt,
	).Scan(&out.ID, &out.OAuthClientID, &out.Kid, &out.JWKJSON, &out.Alg, &out.Use, &out.Enabled, &out.NotBefore, &out.NotAfter, &out.CreatedAt, &out.DisabledAt)
	return out, wrapf("create oauth client key", err)
}

func RecordClientAssertionReplay(ctx context.Context, q Querier, in domain.ClientAssertionReplay) (*domain.ClientAssertionReplay, error) {
	if err := domain.ValidateClientAssertionReplay(in); err != nil {
		return nil, wrapf("record client assertion replay", err)
	}
	if in.ConsumedAt.IsZero() {
		in.ConsumedAt = time.Now().UTC()
	}
	out := &domain.ClientAssertionReplay{}
	err := q.QueryRow(ctx, `
INSERT INTO oauth_client_assertion_replays(oauth_client_id,endpoint_kind,jti_hash,audience,issued_at,expires_at,consumed_at)
VALUES($1,$2,$3,$4,$5,$6,$7)
RETURNING id,oauth_client_id,endpoint_kind,jti_hash,audience,issued_at,expires_at,consumed_at`,
		in.OAuthClientID, in.EndpointKind, in.JTIHash, in.Audience, in.IssuedAt, in.ExpiresAt, in.ConsumedAt,
	).Scan(&out.ID, &out.OAuthClientID, &out.EndpointKind, &out.JTIHash, &out.Audience, &out.IssuedAt, &out.ExpiresAt, &out.ConsumedAt)
	return out, wrapf("record client assertion replay", err)
}

func PurgeExpiredClientAssertionReplays(ctx context.Context, q Querier, now time.Time) (int64, error) {
	tag, err := q.Exec(ctx, `DELETE FROM oauth_client_assertion_replays WHERE expires_at <= $1`, now)
	if err != nil {
		return 0, wrapf("purge client assertion replays", err)
	}
	return tag.RowsAffected(), nil
}

func ClaimAuthorizationCode(ctx context.Context, q Querier, in domain.ClaimAuthorizationCodeInput) (*domain.OAuthAuthorizationCode, error) {
	if err := domain.ValidateClaimAuthorizationCode(in); err != nil {
		return nil, wrapf("claim authorization code", err)
	}
	if pool, ok := q.(*pgxpool.Pool); ok {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, wrapf("claim authorization code", err)
		}
		out, err := claimAuthorizationCodeTx(ctx, tx, in)
		if err != nil {
			_ = tx.Rollback(context.Background())
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(context.Background())
			return nil, wrapf("claim authorization code", err)
		}
		return out, nil
	}
	return claimAuthorizationCodeTx(ctx, q, in)
}

func claimAuthorizationCodeTx(ctx context.Context, q Querier, in domain.ClaimAuthorizationCodeInput) (*domain.OAuthAuthorizationCode, error) {
	out := &domain.OAuthAuthorizationCode{}
	err := q.QueryRow(ctx, `
SELECT id,code_hash,pepper_version,grant_id,broker_transaction_id,oauth_client_id,redirect_uri,resource_uri,audience,scopes,pkce_challenge,pkce_method,issued_at,expires_at,consumed_at
FROM oauth_authorization_codes
WHERE code_hash=$1
FOR UPDATE`, in.CodeHash).Scan(
		&out.ID, &out.CodeHash, &out.PepperVersion, &out.GrantID, &out.BrokerTransactionID, &out.OAuthClientID,
		&out.RedirectURI, &out.ResourceURI, &out.Audience, &out.Scopes, &out.PKCEChallenge, &out.PKCEMethod,
		&out.IssuedAt, &out.ExpiresAt, &out.ConsumedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("claim authorization code", domain.ErrNotFound)
	}
	if err != nil {
		return nil, wrapf("claim authorization code", err)
	}
	if in.Audience == "" {
		in.Audience = out.Audience
	}
	if out.ConsumedAt != nil {
		return nil, wrapf("claim authorization code", fmt.Errorf("%w: already consumed", domain.ErrConflict))
	}
	now := time.Now().UTC()
	if now.After(out.ExpiresAt) {
		return nil, wrapf("claim authorization code", fmt.Errorf("%w: expired", domain.ErrInvalid))
	}
	grant, assoc, err := LoadConsumeBindings(ctx, q, out.GrantID)
	if err != nil {
		return nil, wrapf("claim authorization code", err)
	}
	if err := domain.ValidateConsumeBindings(domain.ConsumeBindings{
		Code: *out, Grant: *grant, Association: *assoc, Now: now,
	}, in); err != nil {
		return nil, wrapf("claim authorization code", err)
	}
	err = q.QueryRow(ctx, `
UPDATE oauth_authorization_codes
SET consumed_at=now()
WHERE id=$1 AND consumed_at IS NULL
RETURNING consumed_at`, out.ID).Scan(&out.ConsumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("claim authorization code", fmt.Errorf("%w: already consumed", domain.ErrConflict))
	}
	return out, wrapf("claim authorization code", err)
}

func CreateInitialRefreshFamily(ctx context.Context, q Querier, in domain.InitialRefreshIssuance) (*InitialRefreshFamily, error) {
	if err := domain.ValidateInitialRefreshIssuance(in); err != nil {
		return nil, wrapf("create initial refresh family", err)
	}
	if pool, ok := q.(*pgxpool.Pool); ok {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, wrapf("create initial refresh family", err)
		}
		out, err := createInitialRefreshFamilyTx(ctx, tx, in)
		if err != nil {
			_ = tx.Rollback(context.Background())
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(context.Background())
			return nil, wrapf("create initial refresh family", err)
		}
		return out, nil
	}
	return createInitialRefreshFamilyTx(ctx, q, in)
}

func createInitialRefreshFamilyTx(ctx context.Context, q Querier, in domain.InitialRefreshIssuance) (*InitialRefreshFamily, error) {
	family := &domain.OAuthRefreshFamily{}
	err := q.QueryRow(ctx, `
INSERT INTO oauth_refresh_families(grant_id,oauth_client_id,resource_uri,status,absolute_expires_at,idle_expires_at,last_rotated_at)
VALUES($1,$2,$3,$4,$5,$6,$7)
RETURNING id,grant_id,oauth_client_id,resource_uri,status,absolute_expires_at,idle_expires_at,last_rotated_at,revoked_at,expired_at,reuse_detected_at,revoke_reason_code,version`,
		in.Family.GrantID, in.Family.OAuthClientID, in.Family.ResourceURI, in.Family.Status,
		in.Family.AbsoluteExpiresAt, in.Family.IdleExpiresAt, in.Family.LastRotatedAt,
	).Scan(&family.ID, &family.GrantID, &family.OAuthClientID, &family.ResourceURI, &family.Status,
		&family.AbsoluteExpiresAt, &family.IdleExpiresAt, &family.LastRotatedAt, &family.RevokedAt,
		&family.ExpiredAt, &family.ReuseDetectedAt, &family.RevokeReasonCode, &family.Version)
	if err != nil {
		return nil, wrapf("create initial refresh family", err)
	}
	token := &domain.OAuthRefreshToken{}
	err = q.QueryRow(ctx, `
INSERT INTO oauth_refresh_tokens(family_id,token_hash,pepper_version,sequence,issued_at,expires_at)
VALUES($1,$2,$3,$4,$5,$6)
RETURNING id,family_id,token_hash,pepper_version,sequence,issued_at,expires_at,consumed_at,revoked_at,replaced_by_id,reuse_detected_at`,
		family.ID, in.Token.TokenHash, in.Token.PepperVersion, in.Token.Sequence, in.Token.IssuedAt, in.Token.ExpiresAt,
	).Scan(&token.ID, &token.FamilyID, &token.TokenHash, &token.PepperVersion, &token.Sequence,
		&token.IssuedAt, &token.ExpiresAt, &token.ConsumedAt, &token.RevokedAt, &token.ReplacedByID, &token.ReuseDetectedAt)
	if err != nil {
		return nil, wrapf("create initial refresh token", err)
	}
	return &InitialRefreshFamily{Family: family, Token: token}, nil
}

func CreateTokenIssuanceAudit(ctx context.Context, q Querier, in domain.TokenIssuanceAudit) (*domain.TokenIssuanceAudit, error) {
	if err := domain.ValidateTokenIssuanceAudit(in); err != nil {
		return nil, wrapf("create token issuance audit", err)
	}
	out := &domain.TokenIssuanceAudit{}
	err := q.QueryRow(ctx, `
INSERT INTO token_issuance_audit(grant_id,authorization_code_id,authorization_code_hash,subject_ref,client_id,resource_uri,audience,scopes,jti_hash,kid,issued_at,expires_at,outcome,request_correlation_hash)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
RETURNING id,grant_id,authorization_code_id,authorization_code_hash,subject_ref,client_id,resource_uri,audience,scopes,jti_hash,kid,issued_at,expires_at,outcome,request_correlation_hash`,
		in.GrantID, in.AuthorizationCodeID, in.AuthorizationCodeHash, in.SubjectRef, in.ClientID,
		in.ResourceURI, in.Audience, in.Scopes, in.JTIHash, in.Kid, in.IssuedAt, in.ExpiresAt, in.Outcome, in.RequestCorrelationHash,
	).Scan(&out.ID, &out.GrantID, &out.AuthorizationCodeID, &out.AuthorizationCodeHash, &out.SubjectRef, &out.ClientID,
		&out.ResourceURI, &out.Audience, &out.Scopes, &out.JTIHash, &out.Kid, &out.IssuedAt, &out.ExpiresAt, &out.Outcome, &out.RequestCorrelationHash)
	return out, wrapf("create token issuance audit", err)
}

func IssueAuthorizationCodeTokens(ctx context.Context, pool *pgxpool.Pool, in IssueAuthorizationCodeTokensInput) (*IssuedAuthorizationCodeTokens, error) {
	if pool == nil {
		return nil, wrapf("issue authorization code tokens", fmt.Errorf("%w: nil pool", domain.ErrInvalid))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, wrapf("issue authorization code tokens", err)
	}
	issued, err := issueAuthorizationCodeTokensTx(ctx, tx, in)
	if err != nil {
		_ = tx.Rollback(context.Background())
		return nil, err
	}
	if in.BeforeCommit != nil {
		if cbErr := in.BeforeCommit(tx, issued); cbErr != nil {
			_ = tx.Rollback(context.Background())
			return nil, wrapf("issue authorization code tokens", cbErr)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(context.Background())
		return nil, wrapf("issue authorization code tokens", err)
	}
	return issued, nil
}

func issueAuthorizationCodeTokensTx(ctx context.Context, tx pgx.Tx, in IssueAuthorizationCodeTokensInput) (*IssuedAuthorizationCodeTokens, error) {
	code, err := ClaimAuthorizationCode(ctx, tx, in.Claim)
	if err != nil {
		return nil, err
	}
	in.Refresh.Family.GrantID = code.GrantID
	in.Refresh.Family.OAuthClientID = code.OAuthClientID
	in.Refresh.Family.ResourceURI = code.ResourceURI
	refresh, err := CreateInitialRefreshFamily(ctx, tx, in.Refresh)
	if err != nil {
		return nil, err
	}
	in.Audit.GrantID = code.GrantID
	in.Audit.AuthorizationCodeID = &code.ID
	in.Audit.AuthorizationCodeHash = append([]byte(nil), code.CodeHash...)
	in.Audit.ResourceURI = code.ResourceURI
	in.Audit.Audience = code.Audience
	in.Audit.Scopes = append([]string(nil), code.Scopes...)
	audit, err := CreateTokenIssuanceAudit(ctx, tx, in.Audit)
	if err != nil {
		return nil, err
	}
	return &IssuedAuthorizationCodeTokens{Code: code, Family: refresh.Family, Token: refresh.Token, Audit: audit}, nil
}

func LoadConsumeBindings(ctx context.Context, q Querier, grantID uuid.UUID) (*domain.OAuthGrant, *domain.ProviderSessionAssociation, error) {
	grant, err := GetOAuthGrant(ctx, q, grantID)
	if err != nil {
		return nil, nil, err
	}
	if grant.ProviderSessionAssociationID == nil {
		return nil, nil, fmt.Errorf("%w: grant missing provider session", domain.ErrInvalid)
	}
	assoc, err := GetProviderSessionAssociation(ctx, q, *grant.ProviderSessionAssociationID)
	if err != nil {
		return nil, nil, err
	}
	return grant, assoc, nil
}

func GetEnabledOAuthClientByClientID(ctx context.Context, q Querier, clientID string) (*domain.OAuthClient, error) {
	if err := domain.ValidatePublicClientID(clientID); err != nil {
		return nil, wrapf("get oauth client", err)
	}
	out := &domain.OAuthClient{}
	err := q.QueryRow(ctx, `
SELECT id,client_id,name,client_type,token_endpoint_auth_method,client_secret_hash,client_secret_pepper_version,allowed_grants,enabled,created_at,updated_at,disabled_at
FROM oauth_clients
WHERE client_id=$1 AND enabled AND disabled_at IS NULL`, clientID).Scan(
		&out.ID, &out.ClientID, &out.Name, &out.ClientType, &out.TokenEndpointAuthMethod,
		&out.ClientSecretHash, &out.ClientSecretPepperVersion, &out.AllowedGrants, &out.Enabled,
		&out.CreatedAt, &out.UpdatedAt, &out.DisabledAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("get oauth client", domain.ErrNotFound)
	}
	return out, wrapf("get oauth client", err)
}

func GetEnabledOAuthClientRedirect(ctx context.Context, q Querier, clientID uuid.UUID, redirectURI, resourceURI string) (*domain.OAuthClientRedirect, error) {
	if clientID == uuid.Nil {
		return nil, wrapf("get oauth client redirect", fmt.Errorf("%w: nil client", domain.ErrInvalid))
	}
	out := &domain.OAuthClientRedirect{}
	err := q.QueryRow(ctx, `
SELECT id,oauth_client_id,redirect_uri,resource_uri,audience,allowed_scopes,enabled,created_at,updated_at
FROM oauth_client_redirects
WHERE oauth_client_id=$1 AND enabled AND redirect_uri=$2 AND resource_uri=$3`, clientID, redirectURI, resourceURI).Scan(
		&out.ID, &out.OAuthClientID, &out.RedirectURI, &out.ResourceURI, &out.Audience, &out.AllowedScopes, &out.Enabled, &out.CreatedAt, &out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("get oauth client redirect", domain.ErrNotFound)
	}
	return out, wrapf("get oauth client redirect", err)
}

func ListEnabledOAuthClientKeys(ctx context.Context, q Querier, clientID uuid.UUID, now time.Time) ([]domain.OAuthClientKey, error) {
	if clientID == uuid.Nil {
		return nil, wrapf("list oauth client keys", fmt.Errorf("%w: nil client", domain.ErrInvalid))
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := q.Query(ctx, `
SELECT id,oauth_client_id,kid,jwk_json,alg,use,enabled,not_before,not_after,created_at,disabled_at
FROM oauth_client_keys
WHERE oauth_client_id=$1 AND enabled AND disabled_at IS NULL
  AND (not_before IS NULL OR not_before <= $2)
  AND (not_after IS NULL OR not_after > $2)`, clientID, now)
	if err != nil {
		return nil, wrapf("list oauth client keys", err)
	}
	defer rows.Close()
	var out []domain.OAuthClientKey
	for rows.Next() {
		var rec domain.OAuthClientKey
		if scanErr := rows.Scan(&rec.ID, &rec.OAuthClientID, &rec.Kid, &rec.JWKJSON, &rec.Alg, &rec.Use, &rec.Enabled, &rec.NotBefore, &rec.NotAfter, &rec.CreatedAt, &rec.DisabledAt); scanErr != nil {
			return nil, wrapf("list oauth client keys", scanErr)
		}
		out = append(out, rec)
	}
	return out, wrapf("list oauth client keys", rows.Err())
}

func PurgeExpiredClientAssertionReplaysBounded(ctx context.Context, q Querier, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 256
	}
	tag, err := q.Exec(ctx, `
DELETE FROM oauth_client_assertion_replays
WHERE id IN (
  SELECT id FROM oauth_client_assertion_replays
  WHERE expires_at <= $1
  ORDER BY expires_at
  LIMIT $2
)`, now, limit)
	if err != nil {
		return 0, wrapf("purge client assertion replays", err)
	}
	return tag.RowsAffected(), nil
}

func RevokeInitialRefreshFamily(ctx context.Context, q Querier, familyID uuid.UUID, reason string, now time.Time) error {
	if familyID == uuid.Nil {
		return wrapf("revoke refresh family", fmt.Errorf("%w: nil family", domain.ErrInvalid))
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tag, err := q.Exec(ctx, `
UPDATE oauth_refresh_tokens
SET revoked_at=$2
WHERE family_id=$1 AND consumed_at IS NULL AND revoked_at IS NULL`, familyID, now)
	if err != nil {
		return wrapf("revoke refresh family", err)
	}
	_, err = q.Exec(ctx, `
UPDATE oauth_refresh_families
SET status='revoked', revoked_at=$2, revoke_reason_code=$3, version=version+1
WHERE id=$1 AND status='active'`, familyID, now, reason)
	if err != nil {
		return wrapf("revoke refresh family", err)
	}
	_ = tag
	return nil
}

func GetOAuthGrant(ctx context.Context, q Querier, id uuid.UUID) (*domain.OAuthGrant, error) {
	if id == uuid.Nil {
		return nil, wrapf("get oauth grant", fmt.Errorf("%w: nil id", domain.ErrInvalid))
	}
	out := &domain.OAuthGrant{}
	err := q.QueryRow(ctx, `
SELECT id,account_id,service_principal_id,oauth_client_id,provider_session_association_id,resource_uri,audience,scopes,subject_class,status,granted_at,not_after,revoked_at,revoke_reason_code,version
FROM oauth_grants
WHERE id=$1
FOR UPDATE`, id).Scan(
		&out.ID, &out.AccountID, &out.ServicePrincipalID, &out.OAuthClientID, &out.ProviderSessionAssociationID,
		&out.ResourceURI, &out.Audience, &out.Scopes, &out.SubjectClass, &out.Status, &out.GrantedAt, &out.NotAfter,
		&out.RevokedAt, &out.RevokeReasonCode, &out.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("get oauth grant", domain.ErrNotFound)
	}
	return out, wrapf("get oauth grant", err)
}
