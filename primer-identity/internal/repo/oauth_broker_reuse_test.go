package repo_test

import (
	"context"
	"crypto/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
)

func randomHash(t *testing.T) []byte {
	t.Helper()
	out := make([]byte, 32)
	_, err := rand.Read(out)
	require.NoError(t, err)
	return out
}

// issuanceFixture builds one live broker transaction plus the account and
// mapping a callback needs, and returns a ready issuance input.
func issuanceFixture(t *testing.T, sessionID string) repo.IssueCallbackArtifactsInput {
	t.Helper()
	pool := testutil.DB(t)
	ctx := context.Background()

	client := factory.OAuthClient(t, pool)
	redirect := factory.OAuthClientRedirect(t, pool, client)
	brokerTxn := factory.BrokerTransaction(t, pool, client, redirect)

	// Drive the transaction to provider_validating exactly as a callback does.
	started, err := repo.TransitionBrokerTransaction(ctx, pool, brokerTxn.ID, brokerTxn.Version,
		domain.BrokerStatusPending, domain.BrokerStatusProviderStarted)
	require.NoError(t, err)
	validating, err := repo.TransitionBrokerTransaction(ctx, pool, brokerTxn.ID, started.Version,
		domain.BrokerStatusProviderStarted, domain.BrokerStatusProviderValidating)
	require.NoError(t, err)

	suffix := uuid.NewString()[:8]
	account := factory.Account(t, pool)
	principal := domain.StytchPrincipal{
		ProjectID: "project-" + suffix, OrganizationID: "org-" + suffix, MemberID: "member-" + suffix,
	}
	mapping, err := repo.CreateStytchMapping(ctx, pool, account.ID, principal)
	require.NoError(t, err)

	now := time.Now().UTC()
	return repo.IssueCallbackArtifactsInput{
		BrokerID: brokerTxn.ID, ExpectedVersion: validating.Version,
		AccountID: account.ID, MappingID: mapping.ID,
		ProviderProjectID: principal.ProjectID, ProviderOrganizationID: principal.OrganizationID,
		ProviderMemberID: principal.MemberID, ProviderMemberSessionID: sessionID,
		ProviderExpiresAt: now.Add(time.Hour), LastValidatedAt: now,
		OAuthClientID: client.ID, RedirectURI: redirect.RedirectURI,
		ResourceURI: redirect.ResourceURI, Audience: redirect.Audience,
		Scopes: []string{"openid"}, PKCEChallenge: strings.Repeat("A", 43), PKCEMethod: "S256",
		CodeHash: randomHash(t), PepperVersion: 1,
		IssuedAt: now, ExpiresAt: now.Add(30 * time.Second), GrantNotAfter: now.Add(24 * time.Hour),
	}
}

// A repeat login on the same provider member session must upsert the existing
// association rather than violating its uniqueness constraint.
func TestIssueCallbackArtifactsUpsertsExistingAssociation(t *testing.T) {
	sessionID := "shared-session-" + uuid.NewString()[:8]

	first := issuanceFixture(t, sessionID)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)
	require.NotNil(t, firstOut.Association)

	// A second authorization reuses the same provider member session, but is a
	// separate broker transaction for the same account.
	second := issuanceFixture(t, sessionID)
	second.AccountID = first.AccountID
	second.MappingID = first.MappingID
	second.ProviderProjectID = first.ProviderProjectID
	second.ProviderOrganizationID = first.ProviderOrganizationID
	second.ProviderMemberID = first.ProviderMemberID

	secondOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.NoError(t, err, "a repeat provider member session must upsert, not conflict")
	require.NotNil(t, secondOut.Association)

	assert.Equal(t, firstOut.Association.ID, secondOut.Association.ID,
		"the same provider member session must resolve to one association row")
	assert.True(t, secondOut.Association.LastValidatedAt.After(firstOut.Association.LastValidatedAt) ||
		secondOut.Association.LastValidatedAt.Equal(firstOut.Association.LastValidatedAt),
		"revalidation must refresh the association")

	var associations int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT count(*) FROM provider_session_associations
		 WHERE provider='stytch_b2b' AND provider_project_id=$1 AND provider_member_session_id=$2`,
		first.ProviderProjectID, sessionID).Scan(&associations))
	assert.Equal(t, 1, associations)
}

// Reusing an exact human grant must not widen its scope and must not create a
// second active grant for the same tuple.
func TestIssueCallbackArtifactsReusesExactHumanGrantWithoutWidening(t *testing.T) {
	sessionID := "grant-session-" + uuid.NewString()[:8]

	first := issuanceFixture(t, sessionID)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	second := issuanceFixture(t, sessionID)
	second.AccountID = first.AccountID
	second.MappingID = first.MappingID
	second.ProviderProjectID = first.ProviderProjectID
	second.ProviderOrganizationID = first.ProviderOrganizationID
	second.ProviderMemberID = first.ProviderMemberID
	second.OAuthClientID = first.OAuthClientID
	second.ResourceURI = first.ResourceURI
	second.Audience = first.Audience
	second.Scopes = first.Scopes

	secondOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.NoError(t, err)

	assert.Equal(t, firstOut.Grant.ID, secondOut.Grant.ID,
		"an exact repeat authorization must reuse the existing active human grant")
	assert.Equal(t, first.Scopes, secondOut.Grant.Scopes, "grant scope must not widen on reuse")

	var activeGrants int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_grants
		 WHERE account_id=$1 AND oauth_client_id=$2 AND provider_session_association_id=$3
		   AND resource_uri=$4 AND audience=$5 AND subject_class='human' AND status='active'`,
		first.AccountID, first.OAuthClientID, firstOut.Association.ID,
		first.ResourceURI, first.Audience).Scan(&activeGrants))
	assert.Equal(t, 1, activeGrants, "exactly one active human grant may exist for the tuple")
}

// Each issuance still creates its own distinct one-use authorization code.
func TestIssueCallbackArtifactsAlwaysCreatesDistinctCode(t *testing.T) {
	sessionID := "code-session-" + uuid.NewString()[:8]

	first := issuanceFixture(t, sessionID)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	second := issuanceFixture(t, sessionID)
	second.AccountID = first.AccountID
	second.MappingID = first.MappingID
	second.ProviderProjectID = first.ProviderProjectID
	second.ProviderOrganizationID = first.ProviderOrganizationID
	second.ProviderMemberID = first.ProviderMemberID

	secondOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.NoError(t, err)

	assert.NotEqual(t, firstOut.Code.ID, secondOut.Code.ID)
	assert.Nil(t, firstOut.Code.ConsumedAt, "IB1 never consumes a code")
	assert.Nil(t, secondOut.Code.ConsumedAt, "IB1 never consumes a code")
}

// Concurrent issuance against one broker transaction must commit exactly once.
func TestIssueCallbackArtifactsConcurrentSingleTransactionCommitsOnce(t *testing.T) {
	t.Parallel()
	in := issuanceFixture(t, "concurrent-session-"+uuid.NewString()[:8])

	const workers = 6
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		losers    []error
	)
	gate := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			attempt := in
			attempt.CodeHash = randomHash(t)
			<-gate
			if _, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), attempt); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
				return
			} else {
				mu.Lock()
				losers = append(losers, err)
				mu.Unlock()
			}
		}()
	}
	close(gate)
	wg.Wait()

	assert.Equal(t, 1, successes, "version CAS must admit exactly one issuance")
	for _, err := range losers {
		assert.ErrorIs(t, err, repo.ErrStaleCAS, "losing concurrent callback must not receive a code")
	}

	var codes int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`,
		in.BrokerID).Scan(&codes))
	assert.Equal(t, 1, codes)

	var status string
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT status FROM broker_transactions WHERE id=$1`, in.BrokerID).Scan(&status))
	assert.Equal(t, domain.BrokerStatusAuthorized, status, "losers must not terminalize the winner")
}

// A later callback that disagrees on account ownership must fail closed.
func TestIssueCallbackArtifactsRejectsAssociationAccountMismatch(t *testing.T) {
	sessionID := "acct-mismatch-" + uuid.NewString()[:8]

	first := issuanceFixture(t, sessionID)
	_, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	second := issuanceFixture(t, sessionID)
	second.ProviderProjectID = first.ProviderProjectID

	_, err = repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrConflict)

	var associations int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT count(*) FROM provider_session_associations
		 WHERE provider='stytch_b2b' AND provider_project_id=$1 AND provider_member_session_id=$2`,
		first.ProviderProjectID, sessionID).Scan(&associations))
	assert.Equal(t, 1, associations)
}

// Same provider session cannot be rewritten onto a different org/member tuple.
func TestIssueCallbackArtifactsRejectsAssociationTupleMismatch(t *testing.T) {
	sessionID := "tuple-mismatch-" + uuid.NewString()[:8]

	first := issuanceFixture(t, sessionID)
	_, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	second := issuanceFixture(t, sessionID)
	second.AccountID = first.AccountID
	second.ProviderProjectID = first.ProviderProjectID

	_, err = repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrConflict)
}

// An existing broader grant may satisfy a later equal/subset request.
func TestIssueCallbackArtifactsReusesGrantWhenRequestIsScopeSubset(t *testing.T) {
	sessionID := "grant-subset-" + uuid.NewString()[:8]

	first := issuanceFixture(t, sessionID)
	first.Scopes = []string{"openid", "studio.read"}
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	second := issuanceFixture(t, sessionID)
	second.AccountID = first.AccountID
	second.MappingID = first.MappingID
	second.ProviderProjectID = first.ProviderProjectID
	second.ProviderOrganizationID = first.ProviderOrganizationID
	second.ProviderMemberID = first.ProviderMemberID
	second.OAuthClientID = first.OAuthClientID
	second.ResourceURI = first.ResourceURI
	second.Audience = first.Audience
	second.Scopes = []string{"openid"}

	secondOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.NoError(t, err)
	assert.Equal(t, firstOut.Grant.ID, secondOut.Grant.ID)
	assert.Equal(t, []string{"openid", "studio.read"}, secondOut.Grant.Scopes,
		"subset reuse must keep the existing grant scopes")
	assert.Equal(t, []string{"openid"}, secondOut.Code.Scopes,
		"the new code still records the requested scopes")
}

// A later request must never silently widen an existing active human grant.
func TestIssueCallbackArtifactsRejectsSilentScopeWidening(t *testing.T) {
	sessionID := "grant-widen-" + uuid.NewString()[:8]

	first := issuanceFixture(t, sessionID)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	second := issuanceFixture(t, sessionID)
	second.AccountID = first.AccountID
	second.MappingID = first.MappingID
	second.ProviderProjectID = first.ProviderProjectID
	second.ProviderOrganizationID = first.ProviderOrganizationID
	second.ProviderMemberID = first.ProviderMemberID
	second.OAuthClientID = first.OAuthClientID
	second.ResourceURI = first.ResourceURI
	second.Audience = first.Audience
	second.Scopes = []string{"openid", "studio.read"}

	_, err = repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrConflict)

	var scopes []string
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT scopes FROM oauth_grants WHERE id=$1`, firstOut.Grant.ID).Scan(&scopes))
	assert.Equal(t, first.Scopes, scopes)

	var status string
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT status FROM broker_transactions WHERE id=$1`, second.BrokerID).Scan(&status))
	assert.Equal(t, domain.BrokerStatusProviderValidating, status)
}

// Terminal associations are not reactivated by a later callback upsert.
func TestIssueCallbackArtifactsDoesNotReactivateTerminalAssociation(t *testing.T) {
	sessionID := "assoc-terminal-" + uuid.NewString()[:8]

	first := issuanceFixture(t, sessionID)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	_, err = testutil.DB(t).Exec(context.Background(),
		`UPDATE provider_session_associations
		 SET status='revoked', revoked_at=now(), revoke_reason_code='test_revoke'
		 WHERE id=$1`, firstOut.Association.ID)
	require.NoError(t, err)

	second := issuanceFixture(t, sessionID)
	second.AccountID = first.AccountID
	second.MappingID = first.MappingID
	second.ProviderProjectID = first.ProviderProjectID
	second.ProviderOrganizationID = first.ProviderOrganizationID
	second.ProviderMemberID = first.ProviderMemberID

	_, err = repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrConflict)

	var status string
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT status FROM provider_session_associations WHERE id=$1`,
		firstOut.Association.ID).Scan(&status))
	assert.Equal(t, "revoked", status)
}

// Association expiry and last_validated move only under a monotonic policy.
func TestIssueCallbackArtifactsDoesNotShortenAssociationExpiry(t *testing.T) {
	sessionID := "assoc-mono-" + uuid.NewString()[:8]

	first := issuanceFixture(t, sessionID)
	first.ProviderExpiresAt = first.LastValidatedAt.Add(2 * time.Hour)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	second := issuanceFixture(t, sessionID)
	second.AccountID = first.AccountID
	second.MappingID = first.MappingID
	second.ProviderProjectID = first.ProviderProjectID
	second.ProviderOrganizationID = first.ProviderOrganizationID
	second.ProviderMemberID = first.ProviderMemberID
	second.ProviderExpiresAt = first.LastValidatedAt.Add(30 * time.Minute)
	second.LastValidatedAt = first.LastValidatedAt.Add(time.Minute)

	secondOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.NoError(t, err)
	assert.Equal(t, firstOut.Association.ID, secondOut.Association.ID)
	assert.True(t, secondOut.Association.ProviderExpiresAt.Equal(firstOut.Association.ProviderExpiresAt) ||
		secondOut.Association.ProviderExpiresAt.After(firstOut.Association.ProviderExpiresAt))
	assert.False(t, secondOut.Association.ProviderExpiresAt.Before(firstOut.Association.ProviderExpiresAt),
		"repeat login must not shorten remaining provider session expiry")
	assert.True(t, secondOut.Association.LastValidatedAt.After(firstOut.Association.LastValidatedAt) ||
		secondOut.Association.LastValidatedAt.Equal(first.LastValidatedAt.Add(time.Minute)))
}

func forceExpiredActiveGrant(t *testing.T, grantID uuid.UUID, notAfter time.Time) {
	t.Helper()
	_, err := testutil.DB(t).Exec(context.Background(),
		`UPDATE oauth_grants SET not_after=$2 WHERE id=$1 AND status='active'`,
		grantID, notAfter)
	require.NoError(t, err)
}

func grantStatusAndVersion(t *testing.T, grantID uuid.UUID) (status string, version int64, revokedAt *time.Time, reason *string) {
	t.Helper()
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT status, version, revoked_at, revoke_reason_code FROM oauth_grants WHERE id=$1`,
		grantID).Scan(&status, &version, &revokedAt, &reason))
	return status, version, revokedAt, reason
}

func activeHumanGrantCount(t *testing.T, accountID, clientID, associationID uuid.UUID, resourceURI, audience string) int {
	t.Helper()
	var n int
	require.NoError(t, testutil.DB(t).QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_grants
		 WHERE account_id=$1 AND oauth_client_id=$2 AND provider_session_association_id=$3
		   AND resource_uri=$4 AND audience=$5 AND subject_class='human' AND status='active'`,
		accountID, clientID, associationID, resourceURI, audience).Scan(&n))
	return n
}

func repeatIssuance(t *testing.T, first repo.IssueCallbackArtifactsInput, sessionID string, issuedAt time.Time) repo.IssueCallbackArtifactsInput {
	t.Helper()
	second := issuanceFixture(t, sessionID)
	second.AccountID = first.AccountID
	second.MappingID = first.MappingID
	second.ProviderProjectID = first.ProviderProjectID
	second.ProviderOrganizationID = first.ProviderOrganizationID
	second.ProviderMemberID = first.ProviderMemberID
	second.OAuthClientID = first.OAuthClientID
	second.ResourceURI = first.ResourceURI
	second.Audience = first.Audience
	second.Scopes = first.Scopes
	second.IssuedAt = issuedAt
	second.ExpiresAt = issuedAt.Add(30 * time.Second)
	second.GrantNotAfter = issuedAt.Add(24 * time.Hour)
	return second
}

// An active human grant whose not_after is already in the past must be expired
// atomically and replaced; the new code must not attach to the stale grant.
func TestIssueCallbackArtifactsExpiresActiveGrantPastNotAfter(t *testing.T) {
	sessionID := "grant-expired-" + uuid.NewString()[:8]
	first := issuanceFixture(t, sessionID)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)
	require.Equal(t, "active", firstOut.Grant.Status)

	issuedAt := first.IssuedAt.Add(time.Hour)
	forceExpiredActiveGrant(t, firstOut.Grant.ID, issuedAt.Add(-time.Second))

	second := repeatIssuance(t, first, sessionID, issuedAt)
	secondOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.NoError(t, err)

	assert.NotEqual(t, firstOut.Grant.ID, secondOut.Grant.ID, "expired-but-active grant must not be reused")
	assert.Equal(t, "active", secondOut.Grant.Status)
	assert.True(t, secondOut.Grant.NotAfter.After(issuedAt))
	assert.Equal(t, firstOut.Association.ID, secondOut.Association.ID)
	assert.Equal(t, first.AccountID, *secondOut.Grant.AccountID)
	assert.Equal(t, first.Scopes, secondOut.Grant.Scopes)
	assert.NotEqual(t, firstOut.Code.ID, secondOut.Code.ID)
	assert.Equal(t, secondOut.Grant.ID, secondOut.Code.GrantID, "new code must bind the fresh grant")
	assert.NotEqual(t, firstOut.Grant.ID, secondOut.Code.GrantID, "new code must not bind the stale grant")

	status, version, revokedAt, reason := grantStatusAndVersion(t, firstOut.Grant.ID)
	assert.Equal(t, "expired", status)
	assert.Greater(t, version, firstOut.Grant.Version)
	require.NotNil(t, revokedAt)
	require.NotNil(t, reason)
	assert.NotEmpty(t, *reason)
	assert.Equal(t, 1, activeHumanGrantCount(t, first.AccountID, first.OAuthClientID, firstOut.Association.ID, first.ResourceURI, first.Audience))
}

// Exact not_after == issuedAt is expired, not reusable.
func TestIssueCallbackArtifactsExpiresActiveGrantAtExactNotAfterBoundary(t *testing.T) {
	sessionID := "grant-boundary-" + uuid.NewString()[:8]
	first := issuanceFixture(t, sessionID)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	issuedAt := first.IssuedAt.Add(30 * time.Minute)
	forceExpiredActiveGrant(t, firstOut.Grant.ID, issuedAt)

	second := repeatIssuance(t, first, sessionID, issuedAt)
	secondOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.NoError(t, err)
	assert.NotEqual(t, firstOut.Grant.ID, secondOut.Grant.ID)

	status, _, _, _ := grantStatusAndVersion(t, firstOut.Grant.ID)
	assert.Equal(t, "expired", status)
	assert.Equal(t, secondOut.Grant.ID, secondOut.Code.GrantID)
	assert.Equal(t, 1, activeHumanGrantCount(t, first.AccountID, first.OAuthClientID, firstOut.Association.ID, first.ResourceURI, first.Audience))
}

// Concurrent expiry/reissue must leave exactly one new active grant.
func TestIssueCallbackArtifactsConcurrentExpiredGrantReissueCreatesOneActive(t *testing.T) {
	sessionID := "grant-conc-exp-" + uuid.NewString()[:8]
	first := issuanceFixture(t, sessionID)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	issuedAt := first.IssuedAt.Add(time.Hour)
	forceExpiredActiveGrant(t, firstOut.Grant.ID, issuedAt.Add(-time.Millisecond))

	const workers = 8
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		grantIDs  []uuid.UUID
		successes int
	)
	gate := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			attempt := repeatIssuance(t, first, sessionID, issuedAt)
			<-gate
			out, issueErr := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), attempt)
			if issueErr != nil {
				return
			}
			mu.Lock()
			successes++
			grantIDs = append(grantIDs, out.Grant.ID)
			mu.Unlock()
		}()
	}
	close(gate)
	wg.Wait()

	require.GreaterOrEqual(t, successes, 1)
	seen := map[uuid.UUID]struct{}{}
	for _, id := range grantIDs {
		assert.NotEqual(t, firstOut.Grant.ID, id)
		seen[id] = struct{}{}
	}
	assert.Len(t, seen, 1, "concurrent expiry/reissue must converge on one new grant")
	status, _, _, _ := grantStatusAndVersion(t, firstOut.Grant.ID)
	assert.Equal(t, "expired", status)
	assert.Equal(t, 1, activeHumanGrantCount(t, first.AccountID, first.OAuthClientID, firstOut.Association.ID, first.ResourceURI, first.Audience))
}

// A later issuance after expiry/reissue must reuse the fresh grant and leave
// the expired row terminal (restart must not resurrect it).
func TestIssueCallbackArtifactsRestartAfterExpiredGrantDoesNotResurrect(t *testing.T) {
	sessionID := "grant-restart-" + uuid.NewString()[:8]
	first := issuanceFixture(t, sessionID)
	firstOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), first)
	require.NoError(t, err)

	issuedAt := first.IssuedAt.Add(time.Hour)
	forceExpiredActiveGrant(t, firstOut.Grant.ID, issuedAt.Add(-time.Second))

	second := repeatIssuance(t, first, sessionID, issuedAt)
	secondOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), second)
	require.NoError(t, err)
	require.NotEqual(t, firstOut.Grant.ID, secondOut.Grant.ID)

	restartAt := issuedAt.Add(time.Minute)
	third := repeatIssuance(t, first, sessionID, restartAt)
	thirdOut, err := repo.IssueCallbackArtifacts(context.Background(), testutil.DB(t), third)
	require.NoError(t, err)
	assert.Equal(t, secondOut.Grant.ID, thirdOut.Grant.ID, "restart must reuse the still-valid replacement grant")
	assert.NotEqual(t, firstOut.Grant.ID, thirdOut.Grant.ID)
	assert.Equal(t, thirdOut.Grant.ID, thirdOut.Code.GrantID)

	status, _, _, _ := grantStatusAndVersion(t, firstOut.Grant.ID)
	assert.Equal(t, "expired", status)
	assert.Equal(t, 1, activeHumanGrantCount(t, first.AccountID, first.OAuthClientID, firstOut.Association.ID, first.ResourceURI, first.Audience))
}
