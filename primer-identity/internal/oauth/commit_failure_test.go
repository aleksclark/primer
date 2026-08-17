package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/secrethash"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
	"github.com/aleksclark/primer/identity/internal/token"
)

const (
	internalIssuer        = "https://identity.example.test"
	internalTokenEndpoint = "https://identity.example.test/oauth/token"
	internalVerifier      = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVW"
)

type frozenInternalClock struct{ now time.Time }

func (c frozenInternalClock) Now() time.Time { return c.now }

type keyServiceSignerSource struct{ svc *keys.Service }

func (s keyServiceSignerSource) Current(ctx context.Context) (token.Signer, *domain.SigningKey, error) {
	signer, meta, err := s.svc.ActiveSigner(ctx)
	if err != nil {
		return nil, nil, err
	}
	return signer, meta, nil
}

func (s keyServiceSignerSource) CurrentForTx(ctx context.Context, tx pgx.Tx) (token.Signer, *domain.SigningKey, error) {
	return s.svc.ActiveSignerForTx(ctx, tx)
}

func TestCommitFailureDiscardsSignedJWT(t *testing.T) {
	now := time.Date(2026, 8, 17, 16, 32, 0, 0, time.UTC)
	fx := issuedInternalPublicCode(t, now)
	var signed string
	svc, err := NewService(Dependencies{
		Pool:    testutil.DB(t),
		Signer:  keyServiceSignerSource{svc: fx.keys},
		Secrets: fx.secrets,
		Clock:   frozenInternalClock{now: now},
		Config:  Config{Issuer: internalIssuer, TokenEndpoint: internalTokenEndpoint, AccessTTL: 15 * time.Minute},
		afterSign: func(tx pgx.Tx, issued token.IssuedToken) error {
			signed = issued.Compact
			require.NotEmpty(t, signed)
			_, err := tx.Exec(context.Background(), `
CREATE TEMP TABLE oauth_commit_fail_probe (
  k int PRIMARY KEY DEFERRABLE INITIALLY DEFERRED
) ON COMMIT DROP`)
			require.NoError(t, err)
			_, err = tx.Exec(context.Background(), `INSERT INTO oauth_commit_fail_probe(k) VALUES (1)`)
			require.NoError(t, err)
			_, err = tx.Exec(context.Background(), `INSERT INTO oauth_commit_fail_probe(k) VALUES (1)`)
			require.NoError(t, err)
			return nil
		},
	})
	require.NoError(t, err)

	resp, err := svc.Exchange(context.Background(), ExchangeRequest{
		GrantType: GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: internalVerifier,
	}, ClientAuth{Method: AuthNone, ClientID: fx.client.ClientID})
	require.Error(t, err)
	assert.Equal(t, ErrorTemporarilyUnavail, ErrorCodeOf(err))
	assert.Empty(t, resp.AccessToken)
	assert.Empty(t, resp.RefreshToken)
	assert.NotContains(t, err.Error(), signed)
	assert.NotContains(t, err.Error(), fx.rawCode)

	pool := testutil.DB(t)
	var families, tokens, audits int
	var consumed *time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, fx.grant.ID).Scan(&families))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_tokens t JOIN oauth_refresh_families f ON f.id=t.family_id WHERE f.grant_id=$1`, fx.grant.ID).Scan(&tokens))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM token_issuance_audit WHERE grant_id=$1`, fx.grant.ID).Scan(&audits))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.code.ID).Scan(&consumed))
	require.Zero(t, families)
	require.Zero(t, tokens)
	require.Zero(t, audits)
	require.Nil(t, consumed)

	svc.after = nil
	retry, err := svc.Exchange(context.Background(), ExchangeRequest{
		GrantType: GrantAuthorizationCode, Code: fx.rawCode, ClientID: fx.client.ClientID,
		RedirectURI: fx.redirect.RedirectURI, Resource: fx.redirect.ResourceURI, CodeVerifier: internalVerifier,
	}, ClientAuth{Method: AuthNone, ClientID: fx.client.ClientID})
	require.NoError(t, err)
	require.NotEmpty(t, retry.AccessToken)
	require.NotEqual(t, signed, retry.AccessToken)
}

func TestOAuthErrorExposesFixedClassOnly(t *testing.T) {
	fixed := oauthErr(ErrorTemporarilyUnavail, "token issuance is unavailable")
	var oe *Error
	require.True(t, errors.As(fixed, &oe))
	assert.Equal(t, ErrorTemporarilyUnavail+": token issuance is unavailable", oe.Error())
	assert.Nil(t, errors.Unwrap(oe))

	mapped := mapUnavailable(errors.New("create token issuance audit: ERROR: could not serialize access (SQLSTATE 40001)"))
	require.Equal(t, ErrorTemporarilyUnavail, ErrorCodeOf(mapped))
	assert.Equal(t, ErrorTemporarilyUnavail+": token issuance is unavailable", mapped.Error())
	assert.NotContains(t, mapped.Error(), "SQLSTATE")
	assert.NotContains(t, mapped.Error(), "audit")
	var mappedOE *Error
	require.True(t, errors.As(mapped, &mappedOE))
	assert.Nil(t, errors.Unwrap(mappedOE))
}

type issuedInternalFixture struct {
	client   *domain.OAuthClient
	redirect *domain.OAuthClientRedirect
	account  *domain.Account
	grant    *domain.OAuthGrant
	code     *domain.OAuthAuthorizationCode
	rawCode  string
	secrets  Secrets
	keys     *keys.Service
}

func issuedInternalPublicCode(t *testing.T, now time.Time) issuedInternalFixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.DB(t)
	secrets := Secrets{
		AuthorizationCodePeppers:       map[int][]byte{1: pepperBytes(0x41)},
		AuthorizationCodeActiveVersion: 1,
		ClientSecretPeppers:            map[int][]byte{1: pepperBytes(0x51)},
		ClientSecretActiveVersion:      1,
		RefreshTokenPeppers:            map[int][]byte{1: pepperBytes(0x61)},
		RefreshTokenActiveVersion:      1,
		AssertionPeppers:               map[int][]byte{1: pepperBytes(0x71)},
		AssertionActiveVersion:         1,
	}
	client := factory.OAuthClient(t, pool)
	redirect := factory.OAuthClientRedirect(t, pool, client)
	account := factory.Account(t, pool)
	mapping, err := repo.CreateStytchMapping(ctx, pool, account.ID, domain.StytchPrincipal{
		ProjectID: "proj-" + uuid.NewString()[:8], OrganizationID: "org-" + uuid.NewString()[:8], MemberID: "member-" + uuid.NewString()[:8],
	})
	require.NoError(t, err)
	assoc, err := repo.CreateProviderSessionAssociation(ctx, pool, domain.ProviderSessionAssociation{
		AccountID: account.ID, StytchMappingID: mapping.ID, Provider: "stytch_b2b",
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: "sess-" + uuid.NewString()[:8],
		ProviderExpiresAt: now.Add(2 * time.Hour), Status: "active", LastValidatedAt: now,
	})
	require.NoError(t, err)
	broker := factory.BrokerTransaction(t, pool, client, redirect)
	grant, err := repo.CreateOAuthGrant(ctx, pool, domain.OAuthGrant{
		AccountID: &account.ID, OAuthClientID: client.ID, ProviderSessionAssociationID: &assoc.ID,
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience, Scopes: []string{"openid"},
		SubjectClass: "human", Status: "active", GrantedAt: now, NotAfter: now.Add(24 * time.Hour),
	})
	require.NoError(t, err)
	rawCode := "code-" + uuid.NewString()
	codeHash, err := secrethash.Hash(secrethash.Peppers(secrets.AuthorizationCodePeppers), secrets.AuthorizationCodeActiveVersion, CodeHashContext, []byte(rawCode))
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(internalVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	code, err := repo.CreateAuthorizationCode(ctx, pool, domain.OAuthAuthorizationCode{
		CodeHash: codeHash, PepperVersion: int16(secrets.AuthorizationCodeActiveVersion),
		GrantID: grant.ID, BrokerTransactionID: broker.ID, OAuthClientID: client.ID,
		RedirectURI: redirect.RedirectURI, ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: challenge, PKCEMethod: "S256",
		IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
	})
	require.NoError(t, err)

	var cfg config.KeyConfig
	cfg.Enabled = true
	cfg.SetSealSecretForTest("UExBTlRfU0VBTF9TRUNSRVRfVkFMVUVfQUFBQSEhISE")
	require.NoError(t, cfg.Validate("test"))
	keySvc := keys.NewService(pool, cfg, "test")
	t.Cleanup(func() { _ = keySvc.Close() })
	_, err = keySvc.CreateInitialActive(ctx)
	require.NoError(t, err)
	return issuedInternalFixture{
		client: client, redirect: redirect, account: account, grant: grant, code: code,
		rawCode: rawCode, secrets: secrets, keys: keySvc,
	}
}

func pepperBytes(b byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = b
	}
	return out
}
