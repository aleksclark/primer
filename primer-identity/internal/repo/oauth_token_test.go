package repo_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
)

func TestIB2RecordClientAssertionReplayAtomicAndPurgeExpired(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	client := confidentialJWTClient(t)
	now := time.Now().UTC().Truncate(time.Second)
	jti := uniqueHash("jti-a")
	first, err := repo.RecordClientAssertionReplay(ctx, pool, domain.ClientAssertionReplay{
		OAuthClientID: client.ID, EndpointKind: domain.AssertionEndpointToken,
		JTIHash: jti, Audience: "https://identity.example/oauth/token",
		IssuedAt: now, ExpiresAt: now.Add(2 * time.Minute), ConsumedAt: now,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, first.ID)

	_, err = repo.RecordClientAssertionReplay(ctx, pool, domain.ClientAssertionReplay{
		OAuthClientID: client.ID, EndpointKind: domain.AssertionEndpointToken,
		JTIHash: jti, Audience: "https://identity.example/oauth/token",
		IssuedAt: now, ExpiresAt: now.Add(2 * time.Minute), ConsumedAt: now,
	})
	require.ErrorIs(t, err, domain.ErrConflict)

	_, err = repo.RecordClientAssertionReplay(ctx, pool, domain.ClientAssertionReplay{
		OAuthClientID: client.ID, EndpointKind: domain.AssertionEndpointRevocation,
		JTIHash: jti, Audience: "https://identity.example/oauth/revoke",
		IssuedAt: now, ExpiresAt: now.Add(2 * time.Minute), ConsumedAt: now,
	})
	require.NoError(t, err, "same jti on a different endpoint is a distinct ledger row")

	expired, err := repo.RecordClientAssertionReplay(ctx, pool, domain.ClientAssertionReplay{
		OAuthClientID: client.ID, EndpointKind: domain.AssertionEndpointToken,
		JTIHash: uniqueHash("jti-expired"), Audience: "https://identity.example/oauth/token",
		IssuedAt: now.Add(-10 * time.Minute), ExpiresAt: now.Add(-5 * time.Minute), ConsumedAt: now.Add(-9 * time.Minute),
	})
	require.NoError(t, err)
	purged, err := repo.PurgeExpiredClientAssertionReplays(ctx, pool, now)
	require.NoError(t, err)
	require.GreaterOrEqual(t, purged, int64(1))
	var leftover int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_client_assertion_replays WHERE id=$1`, expired.ID).Scan(&leftover))
	require.Zero(t, leftover)
}

func TestIB2ClaimAuthorizationCodeExactBindingAndConcurrentOneSuccess(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	fx := issuedCodeFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := s256Challenge(verifier)
	_, err := pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, fx.Code.ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	wrongClient := factory.OAuthClient(t, pool)
	wrong := claimInput(fx, verifier, now)
	wrong.OAuthClientID = wrongClient.ID
	_, err = repo.ClaimAuthorizationCode(ctx, pool, wrong)
	require.ErrorIs(t, err, domain.ErrInvalid)

	wrong = claimInput(fx, verifier, now)
	wrong.RedirectURI = "https://other.example/callback"
	_, err = repo.ClaimAuthorizationCode(ctx, pool, wrong)
	require.ErrorIs(t, err, domain.ErrInvalid)

	wrong = claimInput(fx, verifier, now)
	wrong.ResourceURI = "https://other.example/resource"
	_, err = repo.ClaimAuthorizationCode(ctx, pool, wrong)
	require.ErrorIs(t, err, domain.ErrInvalid)

	wrong = claimInput(fx, verifier, now)
	wrong.Audience = "other-audience"
	_, err = repo.ClaimAuthorizationCode(ctx, pool, wrong)
	require.ErrorIs(t, err, domain.ErrInvalid)

	_, err = repo.ClaimAuthorizationCode(ctx, pool, claimInput(fx, strings.Repeat("b", 43), now))
	require.ErrorIs(t, err, domain.ErrInvalid)

	var (
		mu      sync.Mutex
		success int
		claimed *domain.OAuthAuthorizationCode
		wg      sync.WaitGroup
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, claimErr := repo.ClaimAuthorizationCode(ctx, pool, claimInput(fx, verifier, now))
			if claimErr != nil {
				require.True(t, errors.Is(claimErr, domain.ErrConflict) || errors.Is(claimErr, domain.ErrInvalid) || errors.Is(claimErr, domain.ErrNotFound), claimErr)
				return
			}
			mu.Lock()
			success++
			claimed = got
			mu.Unlock()
		}()
	}
	wg.Wait()
	require.Equal(t, 1, success)
	require.NotNil(t, claimed)
	require.NotNil(t, claimed.ConsumedAt)

	_, err = repo.ClaimAuthorizationCode(ctx, pool, claimInput(fx, verifier, now))
	require.Error(t, err)
}

func TestIB2ClaimAuthorizationCodeUsesSuppliedNowIndependentOfHostDB(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	fx := issuedCodeFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := s256Challenge(verifier)
	_, err := pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, fx.Code.ID)
	require.NoError(t, err)

	frozen := time.Date(1999, 1, 2, 3, 4, 5, 0, time.UTC)
	_, err = pool.Exec(ctx, `
UPDATE oauth_authorization_codes
SET issued_at=$1::timestamptz, expires_at=$1::timestamptz + interval '60 seconds'
WHERE id=$2`, frozen, fx.Code.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
UPDATE oauth_grants
SET granted_at=$1::timestamptz, not_after=$1::timestamptz + interval '24 hours'
WHERE id=$2`, frozen, fx.Grant.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
UPDATE provider_session_associations
SET last_validated_at=$1::timestamptz, provider_expires_at=$1::timestamptz + interval '2 hours'
WHERE id=$2`, frozen, *fx.Grant.ProviderSessionAssociationID)
	require.NoError(t, err)

	got, err := repo.ClaimAuthorizationCode(ctx, pool, claimInput(fx, verifier, frozen))
	require.NoError(t, err)
	require.NotNil(t, got.ConsumedAt)
	require.True(t, got.ConsumedAt.UTC().Equal(frozen), "consumed_at=%s want supplied now=%s", got.ConsumedAt.UTC(), frozen)

	future := time.Date(2035, 6, 15, 12, 0, 0, 0, time.UTC)
	futureFx := issuedCodeFixture(t)
	_, err = pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, futureFx.Code.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
UPDATE oauth_authorization_codes
SET issued_at=$1::timestamptz, expires_at=$1::timestamptz + interval '60 seconds'
WHERE id=$2`, future, futureFx.Code.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
UPDATE oauth_grants
SET granted_at=$1::timestamptz, not_after=$1::timestamptz + interval '24 hours'
WHERE id=$2`, future, futureFx.Grant.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
UPDATE provider_session_associations
SET last_validated_at=$1::timestamptz, provider_expires_at=$1::timestamptz + interval '2 hours'
WHERE id=$2`, future, *futureFx.Grant.ProviderSessionAssociationID)
	require.NoError(t, err)
	futureGot, err := repo.ClaimAuthorizationCode(ctx, pool, claimInput(futureFx, verifier, future))
	require.NoError(t, err)
	require.NotNil(t, futureGot.ConsumedAt)
	require.True(t, futureGot.ConsumedAt.UTC().Equal(future), "consumed_at=%s want future=%s", futureGot.ConsumedAt.UTC(), future)
}

func TestIB2ClaimAuthorizationCodeRejectsExpiredRelativeToSuppliedNow(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	fx := issuedCodeFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := s256Challenge(verifier)
	_, err := pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, fx.Code.ID)
	require.NoError(t, err)

	issuedAt := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	expiresAt := issuedAt.Add(60 * time.Second)
	_, err = pool.Exec(ctx, `
UPDATE oauth_authorization_codes
SET issued_at=$1::timestamptz, expires_at=$2::timestamptz
WHERE id=$3`, issuedAt, expiresAt, fx.Code.ID)
	require.NoError(t, err)

	_, err = repo.ClaimAuthorizationCode(ctx, pool, claimInput(fx, verifier, expiresAt.Add(time.Nanosecond)))
	require.ErrorIs(t, err, domain.ErrInvalid)

	got, err := repo.ClaimAuthorizationCode(ctx, pool, claimInput(fx, verifier, expiresAt))
	require.NoError(t, err)
	require.NotNil(t, got.ConsumedAt)
	require.True(t, got.ConsumedAt.UTC().Equal(expiresAt))
}

func claimInput(fx issuedCode, verifier string, now time.Time) domain.ClaimAuthorizationCodeInput {
	return domain.ClaimAuthorizationCodeInput{
		CodeHash: fx.Code.CodeHash, OAuthClientID: fx.Client.ID,
		RedirectURI: fx.Redirect.RedirectURI, ResourceURI: fx.Redirect.ResourceURI,
		Audience: fx.Redirect.Audience, CodeVerifier: verifier, Now: now,
	}
}

func TestIB2ClaimAuthorizationCodeRollbackRestoresUnconsumed(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	fx := issuedCodeFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := s256Challenge(verifier)
	_, err := pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, fx.Code.ID)
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	got, err := repo.ClaimAuthorizationCode(ctx, tx, claimInput(fx, verifier, time.Now().UTC()))
	require.NoError(t, err)
	require.NotNil(t, got.ConsumedAt)
	require.NoError(t, tx.Rollback(ctx))

	var consumed *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.Code.ID).Scan(&consumed))
	require.Nil(t, consumed, "rollback must restore the unconsumed code")
}

func TestIB2IssueAuthorizationCodeTokensSignBeforeCommitAndLostResponse(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	fx := issuedCodeFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := s256Challenge(verifier)
	_, err := pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, fx.Code.ID)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)

	var signed atomic.Bool
	out, err := repo.IssueAuthorizationCodeTokens(ctx, pool, repo.IssueAuthorizationCodeTokensInput{
		Claim: claimInput(fx, verifier, now),
		Refresh: domain.InitialRefreshIssuance{
			Family: domain.OAuthRefreshFamily{
				GrantID: fx.Grant.ID, OAuthClientID: fx.Client.ID, ResourceURI: fx.Redirect.ResourceURI,
				Status: domain.RefreshFamilyStatusActive, AbsoluteExpiresAt: now.Add(30 * 24 * time.Hour),
				IdleExpiresAt: now.Add(7 * 24 * time.Hour), LastRotatedAt: now,
			},
			Token: domain.OAuthRefreshToken{
				TokenHash: uniqueHash("refresh"), PepperVersion: 1, Sequence: 0,
				IssuedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
			},
			ClientID: fx.Client.ClientID, GrantNotAfter: fx.Grant.NotAfter, ProviderExpiresAt: now.Add(40 * 24 * time.Hour),
		},
		Audit: domain.TokenIssuanceAudit{
			GrantID: fx.Grant.ID, AuthorizationCodeHash: fx.Code.CodeHash,
			SubjectRef: domain.HumanSubjectRef(*fx.Grant.AccountID), ClientID: fx.Client.ClientID,
			ResourceURI: fx.Redirect.ResourceURI, Audience: fx.Redirect.Audience,
			Scopes: fx.Grant.Scopes, JTIHash: uniqueHash("jti-access"), Kid: "active-kid",
			IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute), Outcome: domain.IssuanceOutcomeCommitted,
		},
		BeforeCommit: func(tx pgx.Tx, issuance *repo.IssuedAuthorizationCodeTokens) error {
			signed.Store(true)
			require.NotNil(t, issuance.Family)
			require.NotNil(t, issuance.Token)
			require.NotNil(t, issuance.Audit)
			require.NotNil(t, issuance.Code.ConsumedAt)
			var live int
			require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM oauth_refresh_tokens WHERE family_id=$1 AND consumed_at IS NULL AND revoked_at IS NULL`, issuance.Family.ID).Scan(&live))
			require.Equal(t, 1, live)
			return nil
		},
	})
	require.NoError(t, err)
	require.True(t, signed.Load())
	require.Equal(t, domain.RefreshFamilyStatusActive, out.Family.Status)
	require.Equal(t, int64(0), out.Token.Sequence)
	require.Nil(t, out.Token.ConsumedAt)
	require.Equal(t, domain.IssuanceOutcomeCommitted, out.Audit.Outcome)
	require.Equal(t, fx.Code.CodeHash, out.Audit.AuthorizationCodeHash)

	signFail := issuedCodeFixture(t)
	_, err = pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, signFail.Code.ID)
	require.NoError(t, err)
	_, err = repo.IssueAuthorizationCodeTokens(ctx, pool, repo.IssueAuthorizationCodeTokensInput{
		Claim:   claimInput(signFail, verifier, now),
		Refresh: validRefresh(signFail, now),
		Audit:   validAudit(signFail, now),
		BeforeCommit: func(pgx.Tx, *repo.IssuedAuthorizationCodeTokens) error {
			return errors.New("signer unavailable")
		},
	})
	require.Error(t, err)
	var families, tokens, audits int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, signFail.Grant.ID).Scan(&families))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM token_issuance_audit WHERE grant_id=$1`, signFail.Grant.ID).Scan(&audits))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_refresh_tokens t JOIN oauth_refresh_families f ON f.id=t.family_id WHERE f.grant_id=$1`, signFail.Grant.ID).Scan(&tokens))
	require.Zero(t, families)
	require.Zero(t, tokens)
	require.Zero(t, audits)
	var consumed *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, signFail.Code.ID).Scan(&consumed))
	require.Nil(t, consumed, "signer failure must restore the unconsumed code")

	commitFail := issuedCodeFixture(t)
	_, err = pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, commitFail.Code.ID)
	require.NoError(t, err)
	_, err = repo.IssueAuthorizationCodeTokens(ctx, pool, repo.IssueAuthorizationCodeTokensInput{
		Claim:   claimInput(commitFail, verifier, now),
		Refresh: validRefresh(commitFail, now),
		Audit:   validAudit(commitFail, now),
		BeforeCommit: func(tx pgx.Tx, _ *repo.IssuedAuthorizationCodeTokens) error {
			_, abortErr := tx.Exec(ctx, `SELECT 1/0`)
			require.Error(t, abortErr)
			return nil
		},
	})
	require.Error(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, commitFail.Grant.ID).Scan(&families))
	require.Zero(t, families)
	require.NoError(t, pool.QueryRow(ctx, `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, commitFail.Code.ID).Scan(&consumed))
	require.Nil(t, consumed, "commit failure discards the signed material and restores the code")

	lost := issuedCodeFixture(t)
	_, err = pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, lost.Code.ID)
	require.NoError(t, err)
	issued, err := repo.IssueAuthorizationCodeTokens(ctx, pool, repo.IssueAuthorizationCodeTokensInput{
		Claim:   claimInput(lost, verifier, now),
		Refresh: validRefresh(lost, now),
		Audit:   validAudit(lost, now),
	})
	require.NoError(t, err)
	require.NotNil(t, issued.Code.ConsumedAt)
	_, err = repo.ClaimAuthorizationCode(ctx, pool, claimInput(lost, verifier, now))
	require.Error(t, err, "lost response leaves the code consumed and requires restart")
}

func TestIB2CodePurgeNullsAuditFKAndRetainsCopiedHash(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	fx := issuedCodeFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := s256Challenge(verifier)
	_, err := pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, fx.Code.ID)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	issued, err := repo.IssueAuthorizationCodeTokens(ctx, pool, repo.IssueAuthorizationCodeTokensInput{
		Claim:   claimInput(fx, verifier, now),
		Refresh: validRefresh(fx, now),
		Audit:   validAudit(fx, now),
	})
	require.NoError(t, err)
	require.NotNil(t, issued.Audit.AuthorizationCodeID)

	old := now.Add(-25 * time.Hour)
	_, err = pool.Exec(ctx, `UPDATE oauth_authorization_codes SET issued_at=$1::timestamptz, expires_at=$1::timestamptz + interval '30 seconds' WHERE id=$2`, old, fx.Code.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE broker_transactions SET created_at=$1::timestamptz, expires_at=$1::timestamptz + interval '1 minute' WHERE id=$2`, old, fx.Code.BrokerTransactionID)
	require.NoError(t, err)
	deleted, err := repo.PurgeBrokerTransactions(ctx, pool, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	var codeID *uuid.UUID
	var hash []byte
	var subject, clientID, resource, audience, kid string
	require.NoError(t, pool.QueryRow(ctx, `
SELECT authorization_code_id, authorization_code_hash, subject_ref, client_id, resource_uri, audience, kid
FROM token_issuance_audit WHERE id=$1`, issued.Audit.ID).Scan(&codeID, &hash, &subject, &clientID, &resource, &audience, &kid))
	require.Nil(t, codeID)
	require.Equal(t, fx.Code.CodeHash, hash)
	require.Equal(t, domain.HumanSubjectRef(*fx.Grant.AccountID), subject)
	require.Equal(t, fx.Client.ClientID, clientID)
	require.Equal(t, fx.Redirect.ResourceURI, resource)
	require.Equal(t, fx.Redirect.Audience, audience)
	require.NotEmpty(t, kid)
	require.NotEqual(t, fx.Client.ID.String(), clientID)
}

func TestIB2InitialIssuanceRejectsMissingHashPepperLifetimeAndSecondLiveToken(t *testing.T) {
	ctx := context.Background()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	fx := issuedCodeFixtureOn(t, q)
	now := time.Now().UTC().Truncate(time.Second)
	family, err := repo.CreateInitialRefreshFamily(ctx, q, domain.InitialRefreshIssuance{
		Family: domain.OAuthRefreshFamily{
			GrantID: fx.Grant.ID, OAuthClientID: fx.Client.ID, ResourceURI: fx.Redirect.ResourceURI,
			Status: domain.RefreshFamilyStatusActive, AbsoluteExpiresAt: now.Add(30 * 24 * time.Hour),
			IdleExpiresAt: now.Add(7 * 24 * time.Hour), LastRotatedAt: now,
		},
		Token: domain.OAuthRefreshToken{
			TokenHash: uniqueHash("live"), PepperVersion: 1, Sequence: 0,
			IssuedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
		},
		ClientID: fx.Client.ClientID, GrantNotAfter: fx.Grant.NotAfter, ProviderExpiresAt: now.Add(40 * 24 * time.Hour),
	})
	require.NoError(t, err)

	_, err = q.Exec(ctx, `
INSERT INTO oauth_refresh_tokens(family_id,token_hash,pepper_version,sequence,issued_at,expires_at)
VALUES ($1,$2,1,1,$3,$4)`, family.Family.ID, uniqueHash("dup-live"), now, now.Add(time.Hour))
	require.Error(t, err, "exactly one current unconsumed token")

	_, err = q.Exec(ctx, `
INSERT INTO oauth_refresh_tokens(family_id,token_hash,pepper_version,sequence,issued_at,expires_at)
VALUES ($1,$2,0,2,$3,$4)`, family.Family.ID, uniqueHash("bad-pepper"), now, now.Add(time.Hour))
	require.Error(t, err)

	_, err = repo.CreateInitialRefreshFamily(ctx, q, domain.InitialRefreshIssuance{
		Family: domain.OAuthRefreshFamily{
			GrantID: fx.Grant.ID, OAuthClientID: fx.Client.ID, ResourceURI: fx.Redirect.ResourceURI,
			Status: domain.RefreshFamilyStatusActive, AbsoluteExpiresAt: now.Add(30 * 24 * time.Hour),
			IdleExpiresAt: now.Add(7 * 24 * time.Hour), LastRotatedAt: now,
		},
		Token: domain.OAuthRefreshToken{
			TokenHash: make([]byte, 16), PepperVersion: 1, Sequence: 0,
			IssuedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
		},
		ClientID: fx.Client.ClientID, GrantNotAfter: fx.Grant.NotAfter, ProviderExpiresAt: now.Add(40 * 24 * time.Hour),
	})
	require.ErrorIs(t, err, domain.ErrInvalid)
}

func TestIB2PrivateKeyJWTClientRequiresRegisteredKeyAndPublicClientID(t *testing.T) {
	ctx := context.Background()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	client, err := repo.CreateOAuthClient(ctx, q, domain.OAuthClient{
		ClientID: "confidential-pkjwt-" + uuid.NewString()[:8], Name: "Confidential",
		ClientType: "confidential", TokenEndpointAuthMethod: "private_key_jwt",
		AllowedGrants: []string{"authorization_code"}, Enabled: true,
	})
	require.NoError(t, err)
	_, err = repo.CreateOAuthClientKey(ctx, q, domain.OAuthClientKey{
		OAuthClientID: client.ID, Kid: "k1",
		JWKJSON: []byte(`{"kty":"EC","crv":"P-256","x":"abc","y":"def"}`),
		Alg:     "ES256", Use: "sig", Enabled: true,
	})
	require.NoError(t, err)
	require.NotEqual(t, client.ID.String(), client.ClientID)

	_, err = repo.CreateOAuthClientKey(ctx, q, domain.OAuthClientKey{
		OAuthClientID: client.ID, Kid: "k-priv",
		JWKJSON: []byte(`{"kty":"EC","crv":"P-256","x":"abc","y":"def","d":"secret"}`),
		Alg:     "ES256", Use: "sig", Enabled: true,
	})
	require.ErrorIs(t, err, domain.ErrInvalid)
}

func TestIB2ClaimAuthorizationCodeRejectsInactiveGrantAndProvider(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	fx := issuedCodeFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := s256Challenge(verifier)
	_, err := pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, fx.Code.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE oauth_grants SET status='revoked', revoked_at=now(), revoke_reason_code='operator' WHERE id=$1`, fx.Grant.ID)
	require.NoError(t, err)
	_, err = repo.ClaimAuthorizationCode(ctx, pool, claimInput(fx, verifier, time.Now().UTC()))
	require.ErrorIs(t, err, domain.ErrInvalid)

	active := issuedCodeFixture(t)
	_, err = pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, active.Code.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE provider_session_associations SET status='revoked', revoked_at=now(), revoke_reason_code='operator' WHERE id=$1`, *active.Grant.ProviderSessionAssociationID)
	require.NoError(t, err)
	_, err = repo.ClaimAuthorizationCode(ctx, pool, claimInput(active, verifier, time.Now().UTC()))
	require.ErrorIs(t, err, domain.ErrInvalid)
}

func TestIB2IssueAuthorizationCodeTokensContextCancelRestoresNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pool := testutil.DB(t)
	fx := issuedCodeFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := s256Challenge(verifier)
	_, err := pool.Exec(context.Background(), `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, fx.Code.ID)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	cancel()
	_, err = repo.IssueAuthorizationCodeTokens(ctx, pool, repo.IssueAuthorizationCodeTokensInput{
		Claim:   claimInput(fx, verifier, now),
		Refresh: validRefresh(fx, now),
		Audit:   validAudit(fx, now),
	})
	require.Error(t, err)
	var families int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_refresh_families WHERE grant_id=$1`, fx.Grant.ID).Scan(&families))
	require.Zero(t, families)
	var consumed *time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT consumed_at FROM oauth_authorization_codes WHERE id=$1`, fx.Code.ID).Scan(&consumed))
	require.Nil(t, consumed)
}

func TestIB2HashedSecretsNeverPersistRawMaterial(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	fx := issuedCodeFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := s256Challenge(verifier)
	_, err := pool.Exec(ctx, `UPDATE oauth_authorization_codes SET pkce_challenge=$1 WHERE id=$2`, challenge, fx.Code.ID)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	rawRefresh := []byte("refresh-secret-" + uuid.NewString())
	refresh := validRefresh(fx, now)
	refresh.Token.TokenHash = uniqueHash(string(rawRefresh))
	issued, err := repo.IssueAuthorizationCodeTokens(ctx, pool, repo.IssueAuthorizationCodeTokensInput{
		Claim:   claimInput(fx, verifier, now),
		Refresh: refresh,
		Audit:   validAudit(fx, now),
	})
	require.NoError(t, err)
	var storedHash, storedCodeHash, storedJTI []byte
	var storedClient string
	require.NoError(t, pool.QueryRow(ctx, `SELECT token_hash FROM oauth_refresh_tokens WHERE id=$1`, issued.Token.ID).Scan(&storedHash))
	require.Equal(t, refresh.Token.TokenHash, storedHash)
	require.NotEqual(t, rawRefresh, storedHash)
	require.NoError(t, pool.QueryRow(ctx, `SELECT authorization_code_hash, client_id, jti_hash FROM token_issuance_audit WHERE id=$1`, issued.Audit.ID).Scan(&storedCodeHash, &storedClient, &storedJTI))
	require.Equal(t, fx.Code.CodeHash, storedCodeHash)
	require.Equal(t, fx.Client.ClientID, storedClient)
	require.Len(t, storedJTI, 32)
	require.NotContains(t, string(storedHash), "refresh-secret-")
	require.NotContains(t, storedClient, fx.Client.ID.String())
}

func TestIB2RepoHasNoJWTSigningRefreshRedemptionOrIB6Rotation(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("oauth_token.go")
	require.NoError(t, err)
	body := string(src)
	require.Contains(t, body, "BeforeCommit")
	require.Contains(t, body, "SET consumed_at=$2")
	require.NotContains(t, body, "consumed_at=now()")
	require.NotContains(t, body, "jwt.")
	require.NotContains(t, body, "session_jwt")
	require.NotContains(t, body, "RotateRefresh")
	require.NotContains(t, body, "RedeemRefresh")
	require.NotContains(t, body, "/oauth/token")
	require.NotContains(t, body, "func Rotate")
	require.NotContains(t, body, "func Redeem")
}

type issuedCode struct {
	Client   *domain.OAuthClient
	Redirect *domain.OAuthClientRedirect
	Grant    *domain.OAuthGrant
	Code     *domain.OAuthAuthorizationCode
}

func issuedCodeFixture(t *testing.T) issuedCode {
	t.Helper()
	return issuedCodeFixtureOn(t, testutil.DB(t))
}

func issuedCodeFixtureOn(t *testing.T, q repo.Querier) issuedCode {
	t.Helper()
	ctx := context.Background()
	client := factory.OAuthClient(t, q)
	redirect := factory.OAuthClientRedirect(t, q, client)
	owner := factory.Account(t, q)
	mapping, err := repo.CreateStytchMapping(ctx, q, owner.ID, domain.StytchPrincipal{
		ProjectID: "proj-" + uuid.NewString()[:8], OrganizationID: "org-" + uuid.NewString()[:8], MemberID: "member-" + uuid.NewString()[:8],
	})
	require.NoError(t, err)
	assoc, err := repo.CreateProviderSessionAssociation(ctx, q, domain.ProviderSessionAssociation{
		AccountID: owner.ID, StytchMappingID: mapping.ID, Provider: "stytch_b2b",
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: "sess-" + uuid.NewString()[:8],
		ProviderExpiresAt: time.Now().UTC().Add(time.Hour), Status: "active", LastValidatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	broker := factory.BrokerTransaction(t, q, client, redirect)
	grant, err := repo.CreateOAuthGrant(ctx, q, humanGrant(client.ID, owner.ID, assoc.ID, redirect))
	require.NoError(t, err)
	code, err := repo.CreateAuthorizationCode(ctx, q, authCode(grant.ID, broker.ID, client.ID, redirect, uniqueHash("ib2-code")))
	require.NoError(t, err)
	return issuedCode{Client: client, Redirect: redirect, Grant: grant, Code: code}
}

func confidentialJWTClient(t *testing.T) *domain.OAuthClient {
	t.Helper()
	pool := testutil.DB(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	client, err := repo.CreateOAuthClient(ctx, tx, domain.OAuthClient{
		ClientID: "pkjwt-" + uuid.NewString()[:8], Name: "JWT client",
		ClientType: "confidential", TokenEndpointAuthMethod: "private_key_jwt",
		AllowedGrants: []string{"authorization_code"}, Enabled: true,
	})
	require.NoError(t, err)
	_, err = repo.CreateOAuthClientKey(ctx, tx, domain.OAuthClientKey{
		OAuthClientID: client.ID, Kid: "k-" + uuid.NewString()[:6],
		JWKJSON: []byte(`{"kty":"EC","crv":"P-256","x":"abc","y":"def"}`),
		Alg:     "ES256", Use: "sig", Enabled: true,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return client
}

func validRefresh(fx issuedCode, now time.Time) domain.InitialRefreshIssuance {
	return domain.InitialRefreshIssuance{
		Family: domain.OAuthRefreshFamily{
			GrantID: fx.Grant.ID, OAuthClientID: fx.Client.ID, ResourceURI: fx.Redirect.ResourceURI,
			Status: domain.RefreshFamilyStatusActive, AbsoluteExpiresAt: now.Add(30 * 24 * time.Hour),
			IdleExpiresAt: now.Add(7 * 24 * time.Hour), LastRotatedAt: now,
		},
		Token: domain.OAuthRefreshToken{
			TokenHash: uniqueHash("refresh"), PepperVersion: 1, Sequence: 0,
			IssuedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
		},
		ClientID: fx.Client.ClientID, GrantNotAfter: fx.Grant.NotAfter, ProviderExpiresAt: now.Add(40 * 24 * time.Hour),
	}
}

func validAudit(fx issuedCode, now time.Time) domain.TokenIssuanceAudit {
	return domain.TokenIssuanceAudit{
		GrantID: fx.Grant.ID, AuthorizationCodeHash: fx.Code.CodeHash,
		SubjectRef: domain.HumanSubjectRef(*fx.Grant.AccountID), ClientID: fx.Client.ClientID,
		ResourceURI: fx.Redirect.ResourceURI, Audience: fx.Redirect.Audience,
		Scopes: fx.Grant.Scopes, JTIHash: uniqueHash("audit-jti"), Kid: "active-kid",
		IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute), Outcome: domain.IssuanceOutcomeCommitted,
	}
}

func s256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
