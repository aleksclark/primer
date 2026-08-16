package broker_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func testSecrets(t *testing.T) config.BrokerSecretSet {
	t.Helper()
	mk := func(b byte) map[int][]byte {
		raw := make([]byte, 32)
		for i := range raw {
			raw[i] = b
		}
		return map[int][]byte{1: raw}
	}
	return config.BrokerSecretSet{
		StateSealKeys: mk(0x11), StateSealActiveVersion: 1,
		StateHashPeppers: mk(0x22), StateHashActiveVersion: 1,
		BrokerCookiePeppers: mk(0x33), BrokerCookieActiveVersion: 1,
		AuthorizationCodePeppers: mk(0x44), AuthorizationCodeActiveVersion: 1,
	}
}

func uniqueState(label string) string {
	return label + "-" + uuid.NewString()
}

func uniqueID(prefix string) string {
	return prefix + "-" + uuid.NewString()
}

func uniqueFixture() (project, org, member, session string) {
	suffix := uuid.NewString()[:8]
	return "project-" + suffix, "organization-" + suffix, "member-" + suffix, "member-session-" + suffix
}

func scriptedSuccess(t *testing.T, artifact string) *brokerprovider.ScriptedProvider {
	t.Helper()
	project, org, member, session := uniqueFixture()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{{
			Artifact: uniqueID(artifact), Method: brokerprovider.MethodEmailMagicLink,
			Outcome:   brokerprovider.OutcomeAuthenticated,
			ProjectID: project, OrganizationID: org,
			MemberID: member, MemberSessionID: session,
			ExpiresAt: time.Now().UTC().Add(time.Hour),
		}},
	})
	require.NoError(t, err)
	return p
}

func scriptedSuccessNamed(t *testing.T, artifact string) (*brokerprovider.ScriptedProvider, string) {
	t.Helper()
	project, org, member, session := uniqueFixture()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{{
			Artifact: artifact, Method: brokerprovider.MethodEmailMagicLink,
			Outcome:   brokerprovider.OutcomeAuthenticated,
			ProjectID: project, OrganizationID: org,
			MemberID: member, MemberSessionID: session,
			ExpiresAt: time.Now().UTC().Add(time.Hour),
		}},
	})
	require.NoError(t, err)
	return p, artifact
}

// newService builds a broker service over the real Identity Postgres harness.
func newService(t *testing.T, provider brokerprovider.Provider) *broker.Service {
	t.Helper()
	pool := testutil.DB(t)
	svc, err := broker.NewService(broker.ServiceConfig{
		Pool: pool, Secrets: testSecrets(t), Provider: provider,
		Issuer: "https://id.example",
	})
	require.NoError(t, err)
	return svc
}

// registerClient creates one enabled client with one exact registration tuple.
func registerClient(t *testing.T, clientID string) (redirect, resource, audience string) {
	t.Helper()
	pool := testutil.DB(t)
	ctx := context.Background()
	client, err := repo.CreateOAuthClient(ctx, pool, domain.OAuthClient{
		ClientID: clientID, Name: "Lane C test client", ClientType: "public",
		TokenEndpointAuthMethod: "none", AllowedGrants: []string{"authorization_code"},
		Enabled: true,
	})
	require.NoError(t, err)

	redirect = "https://" + clientID + ".example/callback"
	resource = "https://" + clientID + ".example/mcp"
	audience = clientID + "-aud"
	_, err = repo.CreateOAuthClientRedirect(ctx, pool, domain.OAuthClientRedirect{
		OAuthClientID: client.ID, RedirectURI: redirect, ResourceURI: resource,
		Audience: audience, AllowedScopes: []string{"openid"}, Enabled: true,
	})
	require.NoError(t, err)
	return redirect, resource, audience
}

func authorizeReq(clientID, redirect, resource, audience, state string) broker.AuthorizeRequest {
	return broker.AuthorizeRequest{
		ClientID: clientID, RedirectURI: redirect, ResourceURI: resource,
		Audience: audience, Scopes: []string{"openid"}, State: []byte(state),
		CodeChallenge: strings.Repeat("A", 43), CodeMethod: "S256",
	}
}

// Authorize must resolve the exact registration and create a 10-minute broker
// transaction with sealed recoverable state and a broker cookie.
func TestAuthorizeCreatesBrokerTransactionWithSealedState(t *testing.T) {
	svc := newService(t, scriptedSuccess(t, "artifact-authorize"))
	clientID := "authz-" + uuid.NewString()[:8]
	redirect, resource, audience := registerClient(t, clientID)

	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("state-value")))
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, started.TransactionID)
	assert.NotEmpty(t, started.CookieValue)

	pool := testutil.DB(t)
	var status string
	var sealed []byte
	var expires, created time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, state_sealed, expires_at, created_at FROM broker_transactions WHERE id=$1`,
		started.TransactionID).Scan(&status, &sealed, &expires, &created))

	assert.Equal(t, domain.BrokerStatusPending, status)
	assert.NotEmpty(t, sealed, "recoverable state must be sealed at rest")
	assert.Equal(t, 10*time.Minute, expires.Sub(created).Round(time.Second))

	// The raw state must never be persisted in plaintext.
	assert.NotContains(t, string(sealed), "state-value")
}

// An unregistered client/redirect/resource/audience tuple must be denied
// before any redirect is issued.
func TestAuthorizeRejectsUnregisteredTuple(t *testing.T) {
	svc := newService(t, scriptedSuccess(t, "artifact-unregistered"))
	clientID := "unreg-" + uuid.NewString()[:8]
	redirect, resource, audience := registerClient(t, clientID)

	cases := []struct {
		name                                string
		clientID, redirect, resource, audie string
	}{
		{"wrong client", "not-registered", redirect, resource, audience},
		{"wrong redirect", clientID, "https://evil.example/callback", resource, audience},
		{"redirect prefix attack", clientID, redirect + ".evil.example", resource, audience},
		{"wrong resource", clientID, redirect, "https://evil.example/mcp", audience},
		{"wrong audience", clientID, redirect, resource, "other-aud"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Authorize(context.Background(),
				authorizeReq(tc.clientID, tc.redirect, tc.resource, tc.audie, uniqueState("state")))
			require.Error(t, err)
			assert.False(t, broker.IsRedirectable(err),
				"registration failure must not produce a redirect error")
		})
	}
}

// Scopes outside the registration must be rejected.
func TestAuthorizeRejectsUnregisteredScope(t *testing.T) {
	svc := newService(t, scriptedSuccess(t, "artifact-scope"))
	clientID := "scope-" + uuid.NewString()[:8]
	redirect, resource, audience := registerClient(t, clientID)

	req := authorizeReq(clientID, redirect, resource, audience, uniqueState("state"))
	req.Scopes = []string{"openid", "studio.publish"}
	_, err := svc.Authorize(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, broker.ErrorInvalidScope, broker.ErrorCodeOf(err))
}

// The full happy path: authorize, then a scripted callback issues exactly one
// code and returns the exact original state plus iss.
func TestCompleteCallbackIssuesCodeAndRecoversExactState(t *testing.T) {
	artifact := uniqueID("artifact-complete")
	const state = "exact-original-state-\u00e9\u4e2d"
	provider, artifact := scriptedSuccessNamed(t, artifact)
	svc := newService(t, provider)
	clientID := uniqueID("cb")
	redirect, resource, audience := registerClient(t, clientID)

	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, state))
	require.NoError(t, err)

	result, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: artifact,
	})
	require.NoError(t, err)

	assert.Equal(t, redirect, result.RedirectURI)
	assert.Equal(t, state, string(result.State), "exact original state must be recovered")
	assert.Equal(t, "https://id.example", result.Issuer)
	assert.NotEmpty(t, result.Code)
	assert.Len(t, result.Code, 43)

	// The plaintext code must never be persisted; only its HMAC hash.
	pool := testutil.DB(t)
	var count int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`,
		started.TransactionID).Scan(&count))
	assert.Equal(t, 1, count)

	var persistedCode string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT encode(code_hash,'hex') FROM oauth_authorization_codes WHERE broker_transaction_id=$1`,
		started.TransactionID).Scan(&persistedCode))
	assert.NotContains(t, persistedCode, result.Code)

	// Terminal state must null sealed state and stay unconsumed (IB1).
	var status string
	var sealed []byte
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, state_sealed FROM broker_transactions WHERE id=$1`,
		started.TransactionID).Scan(&status, &sealed))
	assert.Equal(t, domain.BrokerStatusAuthorized, status)
	assert.Nil(t, sealed, "terminal transactions must null sealed state")

	var consumed *time.Time
	var expires, issued time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT consumed_at, issued_at, expires_at FROM oauth_authorization_codes WHERE broker_transaction_id=$1`,
		started.TransactionID).Scan(&consumed, &issued, &expires))
	assert.Nil(t, consumed, "IB1 must never consume a code")
	assert.Equal(t, 60*time.Second, expires.Sub(issued).Round(time.Second))
}

// IB1-E08: concurrent callbacks must commit and return exactly one code.
func TestConcurrentCallbacksIssueExactlyOneCode(t *testing.T) {
	artifact := uniqueID("artifact-concurrent")
	provider, artifact := scriptedSuccessNamed(t, artifact)
	svc := newService(t, provider)
	clientID := uniqueID("conc")
	redirect, resource, audience := registerClient(t, clientID)

	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("race-state")))
	require.NoError(t, err)

	const workers = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		codes    []string
		failures int
		lost     int
	)
	gate := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			res, callErr := svc.CompleteCallback(context.Background(), broker.CallbackInput{
				CookieValue: started.CookieValue, Artifact: artifact,
			})
			mu.Lock()
			defer mu.Unlock()
			if callErr != nil {
				failures++
				if errors.Is(callErr, broker.ErrLostRace) ||
					errors.Is(callErr, broker.ErrUnboundCallback) ||
					errors.Is(callErr, broker.ErrProviderDenied) ||
					errors.Is(callErr, brokerprovider.ErrDefinitiveDenial) {
					lost++
				}
				return
			}
			codes = append(codes, res.Code)
		}()
	}
	close(gate)
	wg.Wait()

	require.Len(t, codes, 1, "exactly one concurrent callback may return a code")
	assert.Equal(t, workers-1, failures)
	assert.Equal(t, workers-1, lost)

	pool := testutil.DB(t)
	var count int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`,
		started.TransactionID).Scan(&count))
	assert.Equal(t, 1, count, "exactly one code row may be committed")

	var status string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status FROM broker_transactions WHERE id=$1`, started.TransactionID).Scan(&status))
	assert.Equal(t, domain.BrokerStatusAuthorized, status)
}

// IB1-E06: the same email across two organizations must map to two distinct
// accounts, and no product membership is created.
func TestSameMemberAcrossOrganizationsProducesDistinctAccounts(t *testing.T) {
	project := uniqueID("project")
	orgA, orgB := uniqueID("org-a"), uniqueID("org-b")
	sharedMember := uniqueID("shared-member")
	artifactA, artifactB := uniqueID("artifact-org-a"), uniqueID("artifact-org-b")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{
			{Artifact: artifactA, Method: brokerprovider.MethodEmailOTP,
				Outcome: brokerprovider.OutcomeAuthenticated, ProjectID: project,
				OrganizationID: orgA, MemberID: sharedMember, MemberSessionID: uniqueID("session-a"),
				ExpiresAt: time.Now().UTC().Add(time.Hour)},
			{Artifact: artifactB, Method: brokerprovider.MethodEmailOTP,
				Outcome: brokerprovider.OutcomeAuthenticated, ProjectID: project,
				OrganizationID: orgB, MemberID: sharedMember, MemberSessionID: uniqueID("session-b"),
				ExpiresAt: time.Now().UTC().Add(time.Hour)},
		},
	})
	require.NoError(t, err)

	svc := newService(t, provider)
	clientID := uniqueID("xorg")
	redirect, resource, audience := registerClient(t, clientID)

	first, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("state-a")))
	require.NoError(t, err)
	resA, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: first.CookieValue, Artifact: artifactA})
	require.NoError(t, err)

	second, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("state-b")))
	require.NoError(t, err)
	resB, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: second.CookieValue, Artifact: artifactB})
	require.NoError(t, err)

	require.NotEqual(t, uuid.Nil, resA.AccountID)
	assert.NotEqual(t, resA.AccountID, resB.AccountID,
		"same member id in different organizations must not merge accounts")

	pool := testutil.DB(t)
	var memberships int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE '%membership%'`).Scan(&memberships))
	assert.Zero(t, memberships, "IB1 must not create product membership tables or rows")
}

// IB1-E05: incomplete MFA stays Identity-only — no code, no grant.
func TestIncompleteMFAIssuesNoAuthority(t *testing.T) {
	project, org, member, _ := uniqueFixture()
	artifact := uniqueID("artifact-mfa")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{{
			Artifact: artifact, Method: brokerprovider.MethodEmailOTP,
			Outcome: brokerprovider.OutcomeIncompleteMFA, ProjectID: project,
			OrganizationID: org, MemberID: member,
		}},
	})
	require.NoError(t, err)

	svc := newService(t, provider)
	clientID := uniqueID("mfa")
	redirect, resource, audience := registerClient(t, clientID)

	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("mfa-state")))
	require.NoError(t, err)

	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: artifact})
	require.Error(t, err)
	assert.True(t, broker.IsIncompleteMFA(err))

	pool := testutil.DB(t)
	var codes, grants int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`,
		started.TransactionID).Scan(&codes))
	assert.Zero(t, codes, "incomplete MFA must issue no authorization code")
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_grants WHERE oauth_client_id=(SELECT oauth_client_id FROM broker_transactions WHERE id=$1)`,
		started.TransactionID).Scan(&grants))
	assert.Zero(t, grants, "incomplete MFA must create no grant")

	var status string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status FROM broker_transactions WHERE id=$1`, started.TransactionID).Scan(&status))
	assert.Equal(t, domain.BrokerStatusProviderStarted, status)
}

// A transient provider fault must not terminalize the transaction, so the
// human can retry; a definitive denial must terminalize it.
func TestProviderFailureClassificationDrivesTerminalization(t *testing.T) {
	transientArtifact := uniqueID("artifact-transient")
	deniedArtifact := uniqueID("artifact-denied")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{
			{Artifact: transientArtifact, Method: brokerprovider.MethodSSOSAML, Outcome: brokerprovider.OutcomeUnavailable},
			{Artifact: deniedArtifact, Method: brokerprovider.MethodSSOSAML, Outcome: brokerprovider.OutcomeDenied},
		},
	})
	require.NoError(t, err)
	svc := newService(t, provider)
	pool := testutil.DB(t)

	clientID := uniqueID("fail")
	redirect, resource, audience := registerClient(t, clientID)

	transient, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("s1")))
	require.NoError(t, err)
	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: transient.CookieValue, Artifact: transientArtifact})
	require.Error(t, err)
	var status string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status FROM broker_transactions WHERE id=$1`, transient.TransactionID).Scan(&status))
	assert.NotContains(t, []string{domain.BrokerStatusDenied, domain.BrokerStatusFailed, domain.BrokerStatusAuthorized, domain.BrokerStatusExpired}, status,
		"a transient outage must not terminalize a recoverable transaction")

	denied, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("s2")))
	require.NoError(t, err)
	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: denied.CookieValue, Artifact: deniedArtifact})
	require.Error(t, err)
	var deniedStatus string
	var deniedSealed []byte
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, state_sealed FROM broker_transactions WHERE id=$1`,
		denied.TransactionID).Scan(&deniedStatus, &deniedSealed))
	assert.Equal(t, domain.BrokerStatusDenied, deniedStatus)
	assert.Nil(t, deniedSealed, "terminal denial must null sealed state")
}

// An unbound or forged broker cookie must be rejected without any oracle.
func TestCompleteCallbackRejectsUnboundCookie(t *testing.T) {
	svc := newService(t, scriptedSuccess(t, "artifact-unbound"))
	_, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: "forged-cookie-value", Artifact: uniqueID("artifact-unbound")})
	require.Error(t, err)
	assert.False(t, broker.IsRedirectable(err), "unbound callback must be a local error")
}

// An expired broker transaction cannot complete.
func TestCompleteCallbackRejectsExpiredTransaction(t *testing.T) {
	artifact := uniqueID("artifact-expired-txn")
	provider, artifact := scriptedSuccessNamed(t, artifact)
	svc := newService(t, provider)
	clientID := uniqueID("exp")
	redirect, resource, audience := registerClient(t, clientID)

	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("expired-state")))
	require.NoError(t, err)

	pool := testutil.DB(t)
	_, err = pool.Exec(context.Background(),
		`UPDATE broker_transactions SET expires_at = created_at + interval '1 second' WHERE id=$1`,
		started.TransactionID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var expired bool
		scanErr := pool.QueryRow(context.Background(),
			`SELECT expires_at <= now() FROM broker_transactions WHERE id=$1`, started.TransactionID).Scan(&expired)
		return scanErr == nil && expired
	}, 2*time.Second, 50*time.Millisecond)

	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: artifact})
	require.Error(t, err)
	assert.ErrorIs(t, err, broker.ErrUnboundCallback)
}

// Codes must be 256-bit and unique per issuance.
func TestIssuedCodesAreHighEntropyAndUnique(t *testing.T) {
	project, org, _, _ := uniqueFixture()
	a1, a2 := uniqueID("a1"), uniqueID("a2")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{
			{Artifact: a1, Method: brokerprovider.MethodEmailOTP, Outcome: brokerprovider.OutcomeAuthenticated,
				ProjectID: project, OrganizationID: org, MemberID: uniqueID("m1"),
				MemberSessionID: uniqueID("s1"), ExpiresAt: time.Now().UTC().Add(time.Hour)},
			{Artifact: a2, Method: brokerprovider.MethodEmailOTP, Outcome: brokerprovider.OutcomeAuthenticated,
				ProjectID: project, OrganizationID: org, MemberID: uniqueID("m2"),
				MemberSessionID: uniqueID("s2"), ExpiresAt: time.Now().UTC().Add(time.Hour)},
		},
	})
	require.NoError(t, err)
	svc := newService(t, provider)
	clientID := uniqueID("ent")
	redirect, resource, audience := registerClient(t, clientID)

	seen := map[string]struct{}{}
	for _, artifact := range []string{a1, a2} {
		started, startErr := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("st-"+artifact)))
		require.NoError(t, startErr)
		res, cbErr := svc.CompleteCallback(context.Background(), broker.CallbackInput{
			CookieValue: started.CookieValue, Artifact: artifact})
		require.NoError(t, cbErr)
		// 256 bits base64url unpadded = 43 characters.
		assert.Len(t, res.Code, 43)
		_, dup := seen[res.Code]
		assert.False(t, dup, "issued codes must be unique")
		seen[res.Code] = struct{}{}
	}
}

// Repeat login on the same provider session revalidates the association and
// still issues a distinct 60s code.
func TestRepeatLoginRevalidatesAndIssuesDistinctCode(t *testing.T) {
	project, org, member, session := uniqueFixture()
	firstArtifact, secondArtifact := uniqueID("reval-1"), uniqueID("reval-2")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{
			{Artifact: firstArtifact, Method: brokerprovider.MethodSSOOIDC, Outcome: brokerprovider.OutcomeAuthenticated,
				ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session,
				ExpiresAt: time.Now().UTC().Add(time.Hour)},
			{Artifact: secondArtifact, Method: brokerprovider.MethodSSOOIDC, Outcome: brokerprovider.OutcomeAuthenticated,
				ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session,
				ExpiresAt: time.Now().UTC().Add(2 * time.Hour)},
		},
	})
	require.NoError(t, err)
	svc := newService(t, provider)
	clientID := uniqueID("reval")
	redirect, resource, audience := registerClient(t, clientID)

	first, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("state-1")))
	require.NoError(t, err)
	res1, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: first.CookieValue, Artifact: firstArtifact})
	require.NoError(t, err)

	second, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("state-2")))
	require.NoError(t, err)
	res2, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: second.CookieValue, Artifact: secondArtifact})
	require.NoError(t, err)

	assert.NotEqual(t, res1.Code, res2.Code)
	assert.Equal(t, res1.AccountID, res2.AccountID)

	pool := testutil.DB(t)
	var associations, grants int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM provider_session_associations WHERE provider_member_session_id=$1`, session).Scan(&associations))
	assert.Equal(t, 1, associations)
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_grants WHERE account_id=$1 AND status='active'`, res1.AccountID).Scan(&grants))
	assert.Equal(t, 1, grants)
}

// Magic-link, OTP, and SSO fixtures all issue a code through the same broker.
func TestScriptedMethodsIssueAuthority(t *testing.T) {
	for _, method := range []brokerprovider.Method{
		brokerprovider.MethodEmailMagicLink,
		brokerprovider.MethodEmailOTP,
		brokerprovider.MethodSSOSAML,
		brokerprovider.MethodSSOOIDC,
	} {
		method := method
		t.Run(string(method), func(t *testing.T) {
			project, org, member, session := uniqueFixture()
			artifact := uniqueID("method-" + string(method))
			provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
				Fixtures: []brokerprovider.Fixture{{
					Artifact: artifact, Method: method, Outcome: brokerprovider.OutcomeAuthenticated,
					ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session,
					ExpiresAt: time.Now().UTC().Add(time.Hour),
				}},
			})
			require.NoError(t, err)
			svc := newService(t, provider)
			clientID := uniqueID("method")
			redirect, resource, audience := registerClient(t, clientID)
			started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("st")))
			require.NoError(t, err)
			res, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
				CookieValue: started.CookieValue, Artifact: artifact})
			require.NoError(t, err)
			assert.NotEmpty(t, res.Code)
			assert.Equal(t, redirect, res.RedirectURI)
			assert.Equal(t, "https://id.example", res.Issuer)
		})
	}
}

func TestNewServiceRejectsIncompleteConfig(t *testing.T) {
	pool := testutil.DB(t)
	provider := scriptedSuccess(t, "artifact-new-service")
	secrets := testSecrets(t)

	_, err := broker.NewService(broker.ServiceConfig{Secrets: secrets, Provider: provider, Issuer: "https://id.example"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool is required")

	_, err = broker.NewService(broker.ServiceConfig{Pool: pool, Secrets: secrets, Issuer: "https://id.example"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider is required")

	_, err = broker.NewService(broker.ServiceConfig{Pool: pool, Secrets: secrets, Provider: provider})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "issuer is required")

	emptyPeppers := secrets
	emptyPeppers.StateHashPeppers = nil
	_, err = broker.NewService(broker.ServiceConfig{Pool: pool, Secrets: emptyPeppers, Provider: provider, Issuer: "https://id.example"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret peppers unavailable")
	assert.NotContains(t, err.Error(), "0x")
}

func TestIsRedirectableClassifiesApplicationErrors(t *testing.T) {
	t.Parallel()
	assert.False(t, broker.IsRedirectable(nil))
	assert.False(t, broker.IsRedirectable(broker.ErrUnboundCallback))
	assert.False(t, broker.IsRedirectable(broker.ErrLostRace))
	assert.False(t, broker.IsRedirectable(broker.ErrProviderUnavailable))
	assert.False(t, broker.IsRedirectable(broker.ErrIncompleteMFA))
	assert.True(t, broker.IsRedirectable(broker.ErrProviderDenied))
}

// registerClientWithScopes creates one enabled client with an exact tuple and
// the supplied allowed scopes.
func registerClientWithScopes(t *testing.T, clientID string, scopes []string) (redirect, resource, audience string) {
	t.Helper()
	pool := testutil.DB(t)
	ctx := context.Background()
	client, err := repo.CreateOAuthClient(ctx, pool, domain.OAuthClient{
		ClientID: clientID, Name: "Lane C scoped client", ClientType: "public",
		TokenEndpointAuthMethod: "none", AllowedGrants: []string{"authorization_code"},
		Enabled: true,
	})
	require.NoError(t, err)

	redirect = "https://" + clientID + ".example/callback"
	resource = "https://" + clientID + ".example/mcp"
	audience = clientID + "-aud"
	_, err = repo.CreateOAuthClientRedirect(ctx, pool, domain.OAuthClientRedirect{
		OAuthClientID: client.ID, RedirectURI: redirect, ResourceURI: resource,
		Audience: audience, AllowedScopes: scopes, Enabled: true,
	})
	require.NoError(t, err)
	return redirect, resource, audience
}

// After registration succeeds, invalid_scope may be reported on the registered
// redirect. Unknown tuples stay local.
func TestAuthorizeInvalidScopeIsRedirectableAfterRegistration(t *testing.T) {
	svc := newService(t, scriptedSuccess(t, "artifact-scope-redirect"))
	clientID := uniqueID("scoperedirect")
	redirect, resource, audience := registerClient(t, clientID)

	req := authorizeReq(clientID, redirect, resource, audience, uniqueState("state"))
	req.Scopes = []string{"openid", "studio.publish"}
	_, err := svc.Authorize(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, broker.ErrorInvalidScope, broker.ErrorCodeOf(err))
	assert.True(t, broker.IsRedirectable(err))
	assert.NotContains(t, err.Error(), "studio.publish")
}

// Disabled clients and redirects never become a trusted redirect target.
func TestAuthorizeRejectsDisabledRegistration(t *testing.T) {
	svc := newService(t, scriptedSuccess(t, "artifact-disabled"))
	pool := testutil.DB(t)
	ctx := context.Background()
	clientID := uniqueID("disabled")
	now := time.Now().UTC()
	client, err := repo.CreateOAuthClient(ctx, pool, domain.OAuthClient{
		ClientID: clientID, Name: "disabled", ClientType: "public",
		TokenEndpointAuthMethod: "none", AllowedGrants: []string{"authorization_code"},
		Enabled: false, DisabledAt: &now,
	})
	require.NoError(t, err)
	redirect := "https://" + clientID + ".example/callback"
	resource := "https://" + clientID + ".example/mcp"
	audience := clientID + "-aud"
	_, err = repo.CreateOAuthClientRedirect(ctx, pool, domain.OAuthClientRedirect{
		OAuthClientID: client.ID, RedirectURI: redirect, ResourceURI: resource,
		Audience: audience, AllowedScopes: []string{"openid"}, Enabled: true,
	})
	require.NoError(t, err)

	_, err = svc.Authorize(ctx, authorizeReq(clientID, redirect, resource, audience, uniqueState("state")))
	require.Error(t, err)
	assert.ErrorIs(t, err, broker.ErrUnboundCallback)
	assert.False(t, broker.IsRedirectable(err))
}

// Success walks pending → provider_started → provider_validating → authorized.
func TestCompleteCallbackWalksExactBrokerPath(t *testing.T) {
	artifact := uniqueID("artifact-path")
	provider, artifact := scriptedSuccessNamed(t, artifact)
	svc := newService(t, provider)
	clientID := uniqueID("path")
	redirect, resource, audience := registerClient(t, clientID)

	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("path-state")))
	require.NoError(t, err)

	pool := testutil.DB(t)
	var status string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status FROM broker_transactions WHERE id=$1`, started.TransactionID).Scan(&status))
	assert.Equal(t, domain.BrokerStatusPending, status)

	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: artifact,
	})
	require.NoError(t, err)

	var startedAt, validatingAt, completedAt *time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, provider_started_at, provider_validating_at, completed_at
		 FROM broker_transactions WHERE id=$1`, started.TransactionID).
		Scan(&status, &startedAt, &validatingAt, &completedAt))
	assert.Equal(t, domain.BrokerStatusAuthorized, status)
	require.NotNil(t, startedAt)
	require.NotNil(t, validatingAt)
	require.NotNil(t, completedAt)
	assert.False(t, startedAt.After(*validatingAt))
	assert.False(t, validatingAt.After(*completedAt))
}

// MFA continuation leaves the transaction live; a later authenticated artifact
// on the same cookie may complete.
func TestIncompleteMFAThenAuthenticatedContinuationIssuesOneCode(t *testing.T) {
	project, org, member, session := uniqueFixture()
	mfaArtifact := uniqueID("artifact-mfa-cont")
	doneArtifact := uniqueID("artifact-mfa-done")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{
			{Artifact: mfaArtifact, Method: brokerprovider.MethodEmailOTP,
				Outcome: brokerprovider.OutcomeIncompleteMFA, ProjectID: project,
				OrganizationID: org, MemberID: member},
			{Artifact: doneArtifact, Method: brokerprovider.MethodEmailOTP,
				Outcome: brokerprovider.OutcomeAuthenticated, ProjectID: project,
				OrganizationID: org, MemberID: member, MemberSessionID: session,
				ExpiresAt: time.Now().UTC().Add(time.Hour)},
		},
	})
	require.NoError(t, err)

	svc := newService(t, provider)
	clientID := uniqueID("mfacont")
	redirect, resource, audience := registerClient(t, clientID)
	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("mfa-cont")))
	require.NoError(t, err)

	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: mfaArtifact})
	require.Error(t, err)
	assert.True(t, broker.IsIncompleteMFA(err))
	assert.False(t, broker.IsRedirectable(err))

	res, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: doneArtifact})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Code)

	pool := testutil.DB(t)
	var codes int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`,
		started.TransactionID).Scan(&codes))
	assert.Equal(t, 1, codes)
}

// Same conceptual email across organizations still maps by exact tuple only.
func TestSameEmailCrossOrgDoesNotMergeAccounts(t *testing.T) {
	project := uniqueID("project-email")
	orgA, orgB := uniqueID("org-a"), uniqueID("org-b")
	artifactA, artifactB := uniqueID("email-a"), uniqueID("email-b")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{
			{Artifact: artifactA, Method: brokerprovider.MethodEmailMagicLink,
				Outcome: brokerprovider.OutcomeAuthenticated, ProjectID: project,
				OrganizationID: orgA, MemberID: uniqueID("member-a"), MemberSessionID: uniqueID("session-a"),
				ExpiresAt: time.Now().UTC().Add(time.Hour)},
			{Artifact: artifactB, Method: brokerprovider.MethodEmailMagicLink,
				Outcome: brokerprovider.OutcomeAuthenticated, ProjectID: project,
				OrganizationID: orgB, MemberID: uniqueID("member-b"), MemberSessionID: uniqueID("session-b"),
				ExpiresAt: time.Now().UTC().Add(time.Hour)},
		},
	})
	require.NoError(t, err)

	svc := newService(t, provider)
	clientID := uniqueID("emailxorg")
	redirect, resource, audience := registerClient(t, clientID)

	first, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("email-a")))
	require.NoError(t, err)
	resA, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: first.CookieValue, Artifact: artifactA})
	require.NoError(t, err)

	second, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("email-b")))
	require.NoError(t, err)
	resB, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: second.CookieValue, Artifact: artifactB})
	require.NoError(t, err)

	assert.NotEqual(t, resA.AccountID, resB.AccountID)

	pool := testutil.DB(t)
	var emailA, emailB *string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT primary_email FROM accounts WHERE id=$1`, resA.AccountID).Scan(&emailA))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT primary_email FROM accounts WHERE id=$1`, resB.AccountID).Scan(&emailB))
	assert.Nil(t, emailA, "tuple mapping must not persist an email join key")
	assert.Nil(t, emailB, "tuple mapping must not persist an email join key")
}

// Repeat login may reuse the association and grant but must not widen scopes.
func TestRepeatLoginRejectsScopeWidening(t *testing.T) {
	project, org, member, session := uniqueFixture()
	firstArtifact, secondArtifact := uniqueID("wide-1"), uniqueID("wide-2")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{
			{Artifact: firstArtifact, Method: brokerprovider.MethodSSOOIDC, Outcome: brokerprovider.OutcomeAuthenticated,
				ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session,
				ExpiresAt: time.Now().UTC().Add(time.Hour)},
			{Artifact: secondArtifact, Method: brokerprovider.MethodSSOOIDC, Outcome: brokerprovider.OutcomeAuthenticated,
				ProjectID: project, OrganizationID: org, MemberID: member, MemberSessionID: session,
				ExpiresAt: time.Now().UTC().Add(2 * time.Hour)},
		},
	})
	require.NoError(t, err)
	svc := newService(t, provider)
	clientID := uniqueID("widen")
	redirect, resource, audience := registerClientWithScopes(t, clientID, []string{"openid", "studio.read"})

	narrow := authorizeReq(clientID, redirect, resource, audience, uniqueState("narrow"))
	narrow.Scopes = []string{"openid"}
	first, err := svc.Authorize(context.Background(), narrow)
	require.NoError(t, err)
	res1, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: first.CookieValue, Artifact: firstArtifact})
	require.NoError(t, err)

	wide := authorizeReq(clientID, redirect, resource, audience, uniqueState("wide"))
	wide.Scopes = []string{"openid", "studio.read"}
	second, err := svc.Authorize(context.Background(), wide)
	require.NoError(t, err)
	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: second.CookieValue, Artifact: secondArtifact})
	require.Error(t, err)
	assert.False(t, broker.IsRedirectable(err))
	assert.NotContains(t, err.Error(), res1.Code)
	assert.NotContains(t, err.Error(), session)

	pool := testutil.DB(t)
	var grants int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_grants WHERE account_id=$1 AND status='active'`,
		res1.AccountID).Scan(&grants))
	assert.Equal(t, 1, grants)
	var scopes []string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT scopes FROM oauth_grants WHERE account_id=$1 AND status='active'`,
		res1.AccountID).Scan(&scopes))
	assert.Equal(t, []string{"openid"}, scopes)
}

// A replay after the winner committed cannot change the terminal outcome or
// return the winner's code.
func TestReplayAfterSuccessCannotTerminalizeOrReturnCode(t *testing.T) {
	artifact := uniqueID("artifact-replay")
	provider, artifact := scriptedSuccessNamed(t, artifact)
	svc := newService(t, provider)
	clientID := uniqueID("replay")
	redirect, resource, audience := registerClient(t, clientID)

	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("replay-state")))
	require.NoError(t, err)
	winner, err := svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: artifact})
	require.NoError(t, err)

	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: uniqueID("forged-replay")})
	require.Error(t, err)
	assert.ErrorIs(t, err, broker.ErrUnboundCallback)
	assert.False(t, broker.IsRedirectable(err))
	assert.NotContains(t, err.Error(), winner.Code)

	pool := testutil.DB(t)
	var status string
	var codes int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status FROM broker_transactions WHERE id=$1`, started.TransactionID).Scan(&status))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`,
		started.TransactionID).Scan(&codes))
	assert.Equal(t, domain.BrokerStatusAuthorized, status)
	assert.Equal(t, 1, codes)
}

func TestCompleteCallbackRejectsMalformedBinding(t *testing.T) {
	svc := newService(t, scriptedSuccess(t, "artifact-malformed"))
	cases := []broker.CallbackInput{
		{CookieValue: "", Artifact: uniqueID("a")},
		{CookieValue: uniqueID("cookie"), Artifact: ""},
		{CookieValue: strings.Repeat("c", 129), Artifact: uniqueID("a")},
		{CookieValue: uniqueID("cookie"), Artifact: strings.Repeat("a", 4097)},
	}
	for _, in := range cases {
		_, err := svc.CompleteCallback(context.Background(), in)
		require.Error(t, err)
		assert.ErrorIs(t, err, broker.ErrUnboundCallback)
		assert.False(t, broker.IsRedirectable(err))
		if in.CookieValue != "" {
			assert.NotContains(t, err.Error(), in.CookieValue)
		}
		if in.Artifact != "" {
			assert.NotContains(t, err.Error(), in.Artifact)
		}
	}
}

func TestProviderDeniedIsRedirectableWithoutArtifacts(t *testing.T) {
	deniedArtifact := uniqueID("artifact-denied-redir")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{
			{Artifact: deniedArtifact, Method: brokerprovider.MethodSSOSAML, Outcome: brokerprovider.OutcomeDenied},
		},
	})
	require.NoError(t, err)
	svc := newService(t, provider)
	clientID := uniqueID("denyr")
	redirect, resource, audience := registerClient(t, clientID)
	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("deny")))
	require.NoError(t, err)
	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: deniedArtifact})
	require.Error(t, err)
	assert.ErrorIs(t, err, broker.ErrProviderDenied)
	assert.True(t, broker.IsRedirectable(err))
	assert.NotContains(t, err.Error(), deniedArtifact)
	assert.NotContains(t, err.Error(), started.CookieValue)
}

func TestProviderUnavailableIsNotRedirectable(t *testing.T) {
	transientArtifact := uniqueID("artifact-unavail")
	provider, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Fixtures: []brokerprovider.Fixture{
			{Artifact: transientArtifact, Method: brokerprovider.MethodSSOSAML, Outcome: brokerprovider.OutcomeUnavailable},
		},
	})
	require.NoError(t, err)
	svc := newService(t, provider)
	clientID := uniqueID("unavail")
	redirect, resource, audience := registerClient(t, clientID)
	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("unavail")))
	require.NoError(t, err)
	_, err = svc.CompleteCallback(context.Background(), broker.CallbackInput{
		CookieValue: started.CookieValue, Artifact: transientArtifact})
	require.Error(t, err)
	assert.ErrorIs(t, err, broker.ErrProviderUnavailable)
	assert.False(t, broker.IsRedirectable(err))
	assert.NotContains(t, err.Error(), transientArtifact)
}

// High-count concurrent callbacks still produce exactly one committed code.
func TestHighCountConcurrentCallbacksIssueExactlyOneCode(t *testing.T) {
	artifact := uniqueID("artifact-highcount")
	provider, artifact := scriptedSuccessNamed(t, artifact)
	svc := newService(t, provider)
	clientID := uniqueID("high")
	redirect, resource, audience := registerClient(t, clientID)
	started, err := svc.Authorize(context.Background(), authorizeReq(clientID, redirect, resource, audience, uniqueState("high-state")))
	require.NoError(t, err)

	const workers = 32
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		codes []string
	)
	gate := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			res, callErr := svc.CompleteCallback(context.Background(), broker.CallbackInput{
				CookieValue: started.CookieValue, Artifact: artifact,
			})
			if callErr != nil {
				return
			}
			mu.Lock()
			codes = append(codes, res.Code)
			mu.Unlock()
		}()
	}
	close(gate)
	wg.Wait()

	require.Len(t, codes, 1)
	pool := testutil.DB(t)
	var count int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_codes WHERE broker_transaction_id=$1`,
		started.TransactionID).Scan(&count))
	assert.Equal(t, 1, count)
	var status string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status FROM broker_transactions WHERE id=$1`, started.TransactionID).Scan(&status))
	assert.Equal(t, domain.BrokerStatusAuthorized, status)
}
