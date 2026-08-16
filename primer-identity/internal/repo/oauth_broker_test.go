package repo_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
)

func TestIB1RepositoryRegistrationTransactionAndCAS(t *testing.T) {
	ctx := context.Background()
	tx := testutil.NewSavepointQuerier(testutil.Tx(t))
	client := factory.OAuthClient(t, tx)
	redirect := factory.OAuthClientRedirect(t, tx, client)
	got, err := repo.ResolveRegistration(ctx, tx, client.ClientID, redirect.RedirectURI, redirect.ResourceURI, redirect.Audience)
	require.NoError(t, err)
	require.Equal(t, redirect.ID, got.ID)
	_, err = repo.ResolveRegistration(ctx, tx, client.ClientID, "https://bff.example/other", redirect.ResourceURI, redirect.Audience)
	require.ErrorIs(t, err, domain.ErrNotFound)

	broker := factory.BrokerTransaction(t, tx, client, redirect)
	transitioned, err := repo.TransitionBrokerTransaction(ctx, tx, broker.ID, broker.Version, domain.BrokerStatusPending, domain.BrokerStatusProviderStarted)
	require.NoError(t, err)
	require.Equal(t, domain.BrokerStatusProviderStarted, transitioned.Status)
	_, err = repo.TransitionBrokerTransaction(ctx, tx, broker.ID, broker.Version, domain.BrokerStatusPending, domain.BrokerStatusProviderStarted)
	require.ErrorIs(t, err, repo.ErrStaleCAS)
}

func TestIB1WrongAccountAssociationAndGrantFailClosed(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	client := factory.OAuthClient(t, pool)
	redirect := factory.OAuthClientRedirect(t, pool, client)
	owner := factory.Account(t, pool)
	intruder := factory.Account(t, pool)
	mapping := mustCreateMapping(t, pool, owner.ID, "proj-owner-"+uuid.NewString(), "org-owner-"+uuid.NewString(), "member-owner-"+uuid.NewString())
	assoc := mustCreateAssociation(t, pool, owner.ID, mapping.ID, "sess-owner-"+uuid.NewString())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_grants WHERE oauth_client_id=$1`, client.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM broker_transactions WHERE oauth_client_id=$1`, client.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM provider_session_associations WHERE id=$1`, assoc.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM stytch_mappings WHERE id=$1`, mapping.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_client_redirects WHERE id=$1`, redirect.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_clients WHERE id=$1`, client.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id IN ($1,$2)`, owner.ID, intruder.ID)
	})

	broker, err := repo.CreateBrokerTransaction(ctx, pool, brokerInput(client, redirect, uniqueHash("state"), uniqueHash("cookie")))
	require.NoError(t, err)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
UPDATE broker_transactions SET account_id=$1, provider_session_association_id=$2
WHERE id=$3`, intruder.ID, assoc.ID, broker.ID)
	require.NoError(t, err, "deferred FK is checked at commit")
	err = tx.Commit(ctx)
	require.Error(t, err, "wrong-account broker attach must fail at commit")

	_, err = repo.CreateOAuthGrant(ctx, pool, humanGrant(client.ID, owner.ID, assoc.ID, redirect))
	require.NoError(t, err)
	_, err = repo.CreateOAuthGrant(ctx, pool, humanGrant(client.ID, intruder.ID, assoc.ID, redirect))
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInvalid) || errors.Is(err, domain.ErrConflict), err)

	tx, err = pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
INSERT INTO oauth_grants(account_id,oauth_client_id,provider_session_association_id,resource_uri,audience,scopes,subject_class,not_after)
VALUES ($1,$2,$3,$4,$5,$6,'human', now() + interval '1 hour')`,
		intruder.ID, client.ID, assoc.ID, redirect.ResourceURI, redirect.Audience, []string{"openid"})
	if err == nil {
		err = tx.Commit(ctx)
	} else {
		_ = tx.Rollback(ctx)
	}
	require.Error(t, err, "wrong-account grant insert must fail at the composite FK")
}

func TestIB1AssociationTupleMismatchAndMemberSessionBounds(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	owner := factory.Account(t, pool)
	other := factory.Account(t, pool)
	mapping := mustCreateMapping(t, pool, owner.ID, "proj-tuple-"+uuid.NewString(), "org-tuple-"+uuid.NewString(), "member-tuple-"+uuid.NewString())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM provider_session_associations WHERE stytch_mapping_id=$1`, mapping.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM stytch_mappings WHERE id=$1`, mapping.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id IN ($1,$2)`, owner.ID, other.ID)
	})
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = repo.CreateProviderSessionAssociation(ctx, tx, domain.ProviderSessionAssociation{
		AccountID: other.ID, StytchMappingID: mapping.ID, Provider: "stytch_b2b",
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: "sess-mismatch",
		ProviderExpiresAt: time.Now().UTC().Add(time.Hour), Status: "active",
		LastValidatedAt: time.Now().UTC(),
	})
	if err == nil {
		err = tx.Commit(ctx)
	} else {
		_ = tx.Rollback(ctx)
	}
	require.Error(t, err)

	assoc, err := repo.CreateProviderSessionAssociation(ctx, pool, domain.ProviderSessionAssociation{
		AccountID: owner.ID, StytchMappingID: mapping.ID, Provider: "stytch_b2b",
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: "sess-ok",
		ProviderExpiresAt: time.Now().UTC().Add(time.Hour), Status: "active",
		LastValidatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.Equal(t, "sess-ok", assoc.ProviderMemberSessionID)

	_, err = repo.CreateProviderSessionAssociation(ctx, pool, domain.ProviderSessionAssociation{
		AccountID: owner.ID, StytchMappingID: mapping.ID, Provider: "stytch_b2b",
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: strings.Repeat("s", 256),
		ProviderExpiresAt: time.Now().UTC().Add(time.Hour), Status: "active",
		LastValidatedAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, domain.ErrInvalid)
}

func TestIB1SnapshotMemberSessionSurvivesAssociationWithoutRawToken(t *testing.T) {
	ctx := context.Background()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	owner := factory.Account(t, q)
	mapping := mustCreateMapping(t, q, owner.ID, "proj-e04", "org-e04", "member-e04")
	now := time.Now().UTC()
	fromSnapshot, err := domain.AssociationFromSnapshot(owner.ID, mapping.ID, mapping.ProjectID, mapping.OrganizationID, mapping.MemberID, "member-session-e04", now.Add(time.Hour), now)
	require.NoError(t, err)
	assoc, err := repo.CreateProviderSessionAssociation(ctx, q, fromSnapshot)
	require.NoError(t, err)
	require.Equal(t, "member-session-e04", assoc.ProviderMemberSessionID)
	loaded, err := repo.GetProviderSessionAssociation(ctx, q, assoc.ID)
	require.NoError(t, err)
	require.Equal(t, "member-session-e04", loaded.ProviderMemberSessionID)
	require.NotContains(t, loaded.ProviderMemberSessionID, "session-jwt")
	require.NotContains(t, loaded.ProviderMemberSessionID, "token")
}

func TestIB1HumanOnlyGrantAndOneCodePerBroker(t *testing.T) {
	ctx := context.Background()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	client := factory.OAuthClient(t, q)
	redirect := factory.OAuthClientRedirect(t, q, client)
	owner := factory.Account(t, q)
	mapping := mustCreateMapping(t, q, owner.ID, "proj-code", "org-code", "member-code")
	assoc := mustCreateAssociation(t, q, owner.ID, mapping.ID, "sess-code")
	broker := factory.BrokerTransaction(t, q, client, redirect)
	grant, err := repo.CreateOAuthGrant(ctx, q, humanGrant(client.ID, owner.ID, assoc.ID, redirect))
	require.NoError(t, err)

	svc := uuid.New()
	_, err = repo.CreateOAuthGrant(ctx, q, domain.OAuthGrant{
		ServicePrincipalID: &svc, OAuthClientID: client.ID, ResourceURI: redirect.ResourceURI,
		Audience: redirect.Audience, Scopes: []string{"openid"}, SubjectClass: "service",
		Status: "active", GrantedAt: time.Now().UTC(), NotAfter: time.Now().UTC().Add(time.Hour),
	})
	require.ErrorIs(t, err, domain.ErrInvalid)

	first, err := repo.CreateAuthorizationCode(ctx, q, authCode(grant.ID, broker.ID, client.ID, redirect, uniqueHash("code-a")))
	require.NoError(t, err)
	require.Nil(t, first.ConsumedAt)
	_, err = repo.CreateAuthorizationCode(ctx, q, authCode(grant.ID, broker.ID, client.ID, redirect, uniqueHash("code-b")))
	require.ErrorIs(t, err, domain.ErrConflict)

	_, err = q.Exec(ctx, `UPDATE oauth_authorization_codes SET consumed_at=now() WHERE id=$1`, first.ID)
	require.NoError(t, err, "schema still has consumed_at for later waves")
	loaded, err := repo.GetAuthorizationCode(ctx, q, first.ID)
	require.NoError(t, err)
	require.Nil(t, loaded.ConsumedAt, "IB1 repo must not expose or drive code consumption")
}

func TestIB1IssueCallbackArtifactsCASOneCodeAndPurgeOrder(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	client := factory.OAuthClient(t, pool)
	redirect := factory.OAuthClientRedirect(t, pool, client)
	owner := factory.Account(t, pool)
	mapping := mustCreateMapping(t, pool, owner.ID, "proj-cas-"+uuid.NewString(), "org-cas-"+uuid.NewString(), "member-cas-"+uuid.NewString())
	now := time.Now().UTC()
	brokerIn := brokerInput(client, redirect, uniqueHash("cas-state"), uniqueHash("cas-cookie"))
	broker, err := repo.CreateBrokerTransaction(ctx, pool, brokerIn)
	require.NoError(t, err)
	started, err := repo.TransitionBrokerTransaction(ctx, pool, broker.ID, broker.Version, domain.BrokerStatusPending, domain.BrokerStatusProviderStarted)
	require.NoError(t, err)
	validating, err := repo.TransitionBrokerTransaction(ctx, pool, started.ID, started.Version, domain.BrokerStatusProviderStarted, domain.BrokerStatusProviderValidating)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_authorization_codes WHERE broker_transaction_id=$1`, validating.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_grants WHERE oauth_client_id=$1`, client.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM broker_transactions WHERE id=$1`, validating.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM provider_session_associations WHERE stytch_mapping_id=$1`, mapping.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM stytch_mappings WHERE id=$1`, mapping.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_client_redirects WHERE id=$1`, redirect.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_clients WHERE id=$1`, client.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id=$1`, owner.ID)
	})

	input := repo.IssueCallbackArtifactsInput{
		BrokerID: validating.ID, ExpectedVersion: validating.Version,
		AccountID: owner.ID, MappingID: mapping.ID,
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: "sess-cas",
		ProviderExpiresAt: now.Add(time.Hour), LastValidatedAt: now,
		OAuthClientID: client.ID, RedirectURI: redirect.RedirectURI,
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: strings.Repeat("A", 43),
		PKCEMethod: "S256", CodeHash: uniqueHash("primer-code"), PepperVersion: 1,
		IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
		GrantNotAfter: now.Add(24 * time.Hour),
	}

	var (
		mu      sync.Mutex
		success int
		codes   []uuid.UUID
		wg      sync.WaitGroup
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, issueErr := repo.IssueCallbackArtifacts(ctx, pool, input)
			if issueErr != nil {
				require.True(t, errors.Is(issueErr, repo.ErrStaleCAS) || errors.Is(issueErr, domain.ErrConflict), issueErr)
				return
			}
			mu.Lock()
			success++
			codes = append(codes, out.Code.ID)
			mu.Unlock()
		}()
	}
	wg.Wait()
	require.Equal(t, 1, success)
	require.Len(t, codes, 1)

	var sealed []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT state_sealed FROM broker_transactions WHERE id=$1`, validating.ID).Scan(&sealed))
	require.Nil(t, sealed)
	var codeCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`, validating.ID).Scan(&codeCount))
	require.Equal(t, 1, codeCount)
	var consumed *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT consumed_at FROM oauth_authorization_codes WHERE broker_transaction_id=$1`, validating.ID).Scan(&consumed))
	require.Nil(t, consumed)

	old := now.Add(-25 * time.Hour)
	_, err = pool.Exec(ctx, `UPDATE oauth_authorization_codes SET issued_at=$1::timestamptz, expires_at=$1::timestamptz + interval '30 seconds' WHERE broker_transaction_id=$2`, old, validating.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE broker_transactions SET created_at=$1::timestamptz, expires_at=$1::timestamptz + interval '1 minute' WHERE id=$2`, old, validating.ID)
	require.NoError(t, err)
	deleted, err := repo.PurgeBrokerTransactions(ctx, pool, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`, validating.ID).Scan(&codeCount))
	require.Zero(t, codeCount)
	var brokerCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM broker_transactions WHERE id=$1`, validating.ID).Scan(&brokerCount))
	require.Zero(t, brokerCount)
}

func TestIB1IssueCallbackArtifactsRollsBackOnConflict(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	client := factory.OAuthClient(t, pool)
	redirect := factory.OAuthClientRedirect(t, pool, client)
	owner := factory.Account(t, pool)
	mapping := mustCreateMapping(t, pool, owner.ID, "proj-rb-"+uuid.NewString(), "org-rb-"+uuid.NewString(), "member-rb-"+uuid.NewString())
	now := time.Now().UTC()
	broker, err := repo.CreateBrokerTransaction(ctx, pool, brokerInput(client, redirect, uniqueHash("rb-state"), uniqueHash("rb-cookie")))
	require.NoError(t, err)
	started, err := repo.TransitionBrokerTransaction(ctx, pool, broker.ID, broker.Version, domain.BrokerStatusPending, domain.BrokerStatusProviderStarted)
	require.NoError(t, err)
	validating, err := repo.TransitionBrokerTransaction(ctx, pool, started.ID, started.Version, domain.BrokerStatusProviderStarted, domain.BrokerStatusProviderValidating)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_authorization_codes WHERE broker_transaction_id=$1`, validating.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_grants WHERE oauth_client_id=$1`, client.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM broker_transactions WHERE id=$1`, validating.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM provider_session_associations WHERE stytch_mapping_id=$1`, mapping.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM stytch_mappings WHERE id=$1`, mapping.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_client_redirects WHERE id=$1`, redirect.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_clients WHERE id=$1`, client.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id=$1`, owner.ID)
	})

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "_")
	fn := "ib1_code_fail_" + suffix
	trg := "ib1_code_fail_trg_" + suffix
	_, err = pool.Exec(ctx, fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'forced code insert failure' USING ERRCODE = '23514';
END$$;`, fn))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON oauth_authorization_codes FOR EACH ROW EXECUTE FUNCTION %s()`, trg, fn))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON oauth_authorization_codes`, trg))
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, fn))
	})

	_, err = repo.IssueCallbackArtifacts(ctx, pool, repo.IssueCallbackArtifactsInput{
		BrokerID: validating.ID, ExpectedVersion: validating.Version,
		AccountID: owner.ID, MappingID: mapping.ID,
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: "sess-rb",
		ProviderExpiresAt: now.Add(time.Hour), LastValidatedAt: now,
		OAuthClientID: client.ID, RedirectURI: redirect.RedirectURI,
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: strings.Repeat("A", 43),
		PKCEMethod: "S256", CodeHash: uniqueHash("rb-code"), PepperVersion: 1,
		IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
		GrantNotAfter: now.Add(24 * time.Hour),
	})
	require.Error(t, err)
	var assocCount, grantCount, codeCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM provider_session_associations WHERE provider_member_session_id='sess-rb'`).Scan(&assocCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_grants WHERE account_id=$1 AND oauth_client_id=$2`, owner.ID, client.ID).Scan(&grantCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`, validating.ID).Scan(&codeCount))
	require.Zero(t, assocCount)
	require.Zero(t, grantCount)
	require.Zero(t, codeCount)
	var status string
	var sealed []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, state_sealed FROM broker_transactions WHERE id=$1`, validating.ID).Scan(&status, &sealed))
	require.Equal(t, domain.BrokerStatusProviderValidating, status)
	require.NotEmpty(t, sealed)
}

func TestIB1SerializableRetryAndNonretryableDoNotOrphan(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	client := factory.OAuthClient(t, pool)
	redirect := factory.OAuthClientRedirect(t, pool, client)
	owner := factory.Account(t, pool)
	mapping := mustCreateMapping(t, pool, owner.ID, "proj-retry-"+uuid.NewString(), "org-retry-"+uuid.NewString(), "member-retry-"+uuid.NewString())
	now := time.Now().UTC()
	broker, err := repo.CreateBrokerTransaction(ctx, pool, brokerInput(client, redirect, uniqueHash("retry-state"), uniqueHash("retry-cookie")))
	require.NoError(t, err)
	started, err := repo.TransitionBrokerTransaction(ctx, pool, broker.ID, broker.Version, domain.BrokerStatusPending, domain.BrokerStatusProviderStarted)
	require.NoError(t, err)
	validating, err := repo.TransitionBrokerTransaction(ctx, pool, started.ID, started.Version, domain.BrokerStatusProviderStarted, domain.BrokerStatusProviderValidating)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_authorization_codes WHERE broker_transaction_id=$1`, validating.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_grants WHERE oauth_client_id=$1`, client.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM broker_transactions WHERE id=$1`, validating.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM provider_session_associations WHERE stytch_mapping_id=$1`, mapping.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM stytch_mappings WHERE id=$1`, mapping.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_client_redirects WHERE id=$1`, redirect.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_clients WHERE id=$1`, client.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id=$1`, owner.ID)
	})

	seq := installAssociationRetryTrigger(t, pool, "sess-retry", "40001", false)
	out, err := repo.IssueCallbackArtifacts(ctx, pool, repo.IssueCallbackArtifactsInput{
		BrokerID: validating.ID, ExpectedVersion: validating.Version,
		AccountID: owner.ID, MappingID: mapping.ID,
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: "sess-retry",
		ProviderExpiresAt: now.Add(time.Hour), LastValidatedAt: now,
		OAuthClientID: client.ID, RedirectURI: redirect.RedirectURI,
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: strings.Repeat("A", 43),
		PKCEMethod: "S256", CodeHash: uniqueHash("retry-code"), PepperVersion: 1,
		IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
		GrantNotAfter: now.Add(24 * time.Hour),
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, out.Code.ID)
	var attempts int64
	require.NoError(t, pool.QueryRow(ctx, fmt.Sprintf(`SELECT last_value FROM %s`, seq)).Scan(&attempts))
	require.Equal(t, int64(2), attempts)
}

func TestIB1RepoHasNoTokenConsumeOrEmailMergeAPI(t *testing.T) {
	t.Parallel()
	src, err := osReadRepoSources()
	require.NoError(t, err)
	require.NotContains(t, src, "ConsumeAuthorizationCode")
	require.NotContains(t, src, "/oauth/token")
	require.NotContains(t, src, "FindOrCreateByEmail")
	require.NotContains(t, src, "session_jwt")
}

func mustCreateMapping(t *testing.T, q repo.Querier, accountID uuid.UUID, project, org, member string) *domain.StytchMapping {
	t.Helper()
	mapping, err := repo.CreateStytchMapping(context.Background(), q, accountID, domain.StytchPrincipal{
		ProjectID: project, OrganizationID: org, MemberID: member,
	})
	require.NoError(t, err)
	return mapping
}

func mustCreateAssociation(t *testing.T, q repo.Querier, accountID, mappingID uuid.UUID, sessionID string) *domain.ProviderSessionAssociation {
	t.Helper()
	mapping, err := repo.GetStytchMapping(context.Background(), q, mappingID)
	require.NoError(t, err)
	assoc, err := repo.CreateProviderSessionAssociation(context.Background(), q, domain.ProviderSessionAssociation{
		AccountID: accountID, StytchMappingID: mappingID, Provider: "stytch_b2b",
		ProviderProjectID: mapping.ProjectID, ProviderOrganizationID: mapping.OrganizationID,
		ProviderMemberID: mapping.MemberID, ProviderMemberSessionID: sessionID,
		ProviderExpiresAt: time.Now().UTC().Add(time.Hour), Status: "active",
		LastValidatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	return assoc
}

func brokerInput(client *domain.OAuthClient, redirect *domain.OAuthClientRedirect, stateHash, cookieHash []byte) domain.CreateBrokerTransactionInput {
	now := time.Now().UTC()
	return domain.CreateBrokerTransactionInput{
		OAuthClientID: client.ID, RedirectID: redirect.ID, StateHash: stateHash,
		StatePepperVersion: 1, StateSealed: []byte(strings.Repeat("x", 30)), StateKeyVersion: 1,
		StateLength: 1, PKCEChallenge: strings.Repeat("A", 43), PKCEMethod: "S256",
		RequestedScopes: []string{"openid"}, ResourceURI: redirect.ResourceURI,
		Audience: redirect.Audience, BrokerCookieHash: cookieHash, BrokerCookiePepperVersion: 1,
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
}

func humanGrant(clientID, accountID, assocID uuid.UUID, redirect *domain.OAuthClientRedirect) domain.OAuthGrant {
	now := time.Now().UTC()
	return domain.OAuthGrant{
		AccountID: &accountID, OAuthClientID: clientID, ProviderSessionAssociationID: &assocID,
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience, Scopes: []string{"openid"},
		SubjectClass: "human", Status: "active", GrantedAt: now, NotAfter: now.Add(time.Hour),
	}
}

func authCode(grantID, brokerID, clientID uuid.UUID, redirect *domain.OAuthClientRedirect, hash []byte) domain.OAuthAuthorizationCode {
	now := time.Now().UTC()
	return domain.OAuthAuthorizationCode{
		CodeHash: hash, PepperVersion: 1, GrantID: grantID, BrokerTransactionID: brokerID,
		OAuthClientID: clientID, RedirectURI: redirect.RedirectURI, ResourceURI: redirect.ResourceURI,
		Audience: redirect.Audience, Scopes: []string{"openid"}, PKCEChallenge: strings.Repeat("A", 43),
		PKCEMethod: "S256", IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
	}
}

func uniqueHash(label string) []byte {
	sum := sha256.Sum256([]byte(label + uuid.NewString()))
	out := make([]byte, 32)
	copy(out, sum[:])
	return out
}

func installAssociationRetryTrigger(t *testing.T, pool *pgxpool.Pool, sessionID, sqlState string, alwaysFail bool) string {
	t.Helper()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "_")
	sequenceName := "ib1_assoc_retry_seq_" + suffix
	functionName := "ib1_assoc_retry_fn_" + suffix
	triggerName := "ib1_assoc_retry_trg_" + suffix
	_, err := pool.Exec(context.Background(), fmt.Sprintf(`CREATE SEQUENCE %s START WITH 1`, sequenceName))
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $function$
DECLARE attempt bigint;
BEGIN
  IF NEW.provider_member_session_id = '%s' THEN
    IF current_setting('transaction_isolation') <> 'serializable' THEN
      RAISE EXCEPTION 'callback transaction isolation was %%', current_setting('transaction_isolation') USING ERRCODE = '55000';
    END IF;
    attempt := nextval('%s');
    IF %t OR attempt = 1 THEN
      RAISE EXCEPTION 'forced association serialization retry' USING ERRCODE = '%s';
    END IF;
  END IF;
  RETURN NEW;
END
$function$`, functionName, sessionID, sequenceName, alwaysFail, sqlState))
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON provider_session_associations FOR EACH ROW EXECUTE FUNCTION %s()`, triggerName, functionName))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON provider_session_associations`, triggerName))
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP SEQUENCE IF EXISTS %s`, sequenceName))
	})
	return sequenceName
}

func osReadRepoSources() (string, error) {
	body, err := os.ReadFile("oauth_broker.go")
	if err != nil {
		return "", err
	}
	return string(body), nil
}
