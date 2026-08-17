package brokerprovider_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/brokerprovider"
)

const (
	testProject = "project-test-lane-c"
	testOrg     = "organization-lane-c"
	testMember  = "member-lane-c"
	testSession = "member-session-lane-c"
)

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func baseTime() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) }

func successFixture(artifact string, method brokerprovider.Method) brokerprovider.Fixture {
	return brokerprovider.Fixture{
		Artifact:        artifact,
		Method:          method,
		Outcome:         brokerprovider.OutcomeAuthenticated,
		ProjectID:       testProject,
		OrganizationID:  testOrg,
		MemberID:        testMember,
		MemberSessionID: testSession,
		ExpiresAt:       baseTime().Add(time.Hour),
	}
}

// Each scripted human method must yield the exact provider tuple with no
// additional provider material.
func TestScriptedProviderReturnsExactTupleForEveryFixtureMethod(t *testing.T) {
	t.Parallel()
	methods := []brokerprovider.Method{
		brokerprovider.MethodEmailMagicLink,
		brokerprovider.MethodEmailOTP,
		brokerprovider.MethodSSOSAML,
		brokerprovider.MethodSSOOIDC,
	}
	for _, method := range methods {
		t.Run(string(method), func(t *testing.T) {
			t.Parallel()
			p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
				Now:      fixedClock(baseTime()),
				Fixtures: []brokerprovider.Fixture{successFixture("artifact-"+string(method), method)},
			})
			require.NoError(t, err)

			start, err := p.StartLogin(context.Background(), brokerprovider.StartRequest{Method: method})
			require.NoError(t, err)
			assert.Equal(t, method, start.Method)
			assert.NotEmpty(t, start.Handle)

			got, err := p.CompleteCallback(context.Background(), "artifact-"+string(method))
			require.NoError(t, err)
			assert.Equal(t, brokerprovider.OutcomeAuthenticated, got.Outcome)
			assert.Equal(t, method, got.Method)
			assert.Equal(t, testProject, got.ProjectID)
			assert.Equal(t, testOrg, got.OrganizationID)
			assert.Equal(t, testMember, got.MemberID)
			assert.Equal(t, testSession, got.MemberSessionID)
			assert.True(t, got.MemberSessionExpiresAt.Equal(baseTime().Add(time.Hour)))
		})
	}
}

// Incomplete MFA must remain Identity-only: it is not an authenticated result
// and exposes no usable member session.
func TestScriptedProviderIncompleteMFAYieldsNoSessionAuthority(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now: fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{{
			Artifact:       "artifact-mfa",
			Method:         brokerprovider.MethodEmailOTP,
			Outcome:        brokerprovider.OutcomeIncompleteMFA,
			ProjectID:      testProject,
			OrganizationID: testOrg,
			MemberID:       testMember,
		}},
	})
	require.NoError(t, err)

	got, err := p.CompleteCallback(context.Background(), "artifact-mfa")
	require.NoError(t, err)
	assert.Equal(t, brokerprovider.OutcomeIncompleteMFA, got.Outcome)
	assert.False(t, got.Authenticated())
	assert.Empty(t, got.MemberSessionID)
	assert.True(t, got.MemberSessionExpiresAt.IsZero())
}

// Definitive denial and transient unavailability must be distinguishable so
// callers can fail closed without negative-caching an outage.
func TestScriptedProviderDistinguishesDefinitiveAndTransientFailures(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now: fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{
			{Artifact: "artifact-denied", Method: brokerprovider.MethodEmailMagicLink, Outcome: brokerprovider.OutcomeDenied},
			{Artifact: "artifact-transient", Method: brokerprovider.MethodEmailMagicLink, Outcome: brokerprovider.OutcomeUnavailable},
		},
	})
	require.NoError(t, err)

	_, denyErr := p.CompleteCallback(context.Background(), "artifact-denied")
	require.Error(t, denyErr)
	assert.ErrorIs(t, denyErr, brokerprovider.ErrDefinitiveDenial)
	assert.NotErrorIs(t, denyErr, brokerprovider.ErrProviderUnavailable)

	_, transientErr := p.CompleteCallback(context.Background(), "artifact-transient")
	require.Error(t, transientErr)
	assert.ErrorIs(t, transientErr, brokerprovider.ErrProviderUnavailable)
	assert.NotErrorIs(t, transientErr, brokerprovider.ErrDefinitiveDenial)
}

// An unknown artifact is a definitive denial, never a transient outage.
func TestScriptedProviderUnknownArtifactIsDefinitiveDenial(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now:      fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{successFixture("known", brokerprovider.MethodEmailOTP)},
	})
	require.NoError(t, err)

	_, err = p.CompleteCallback(context.Background(), "not-registered")
	assert.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)
}

// A transient failure must not consume the one-time artifact, otherwise an
// outage would permanently destroy a recoverable login.
func TestScriptedProviderTransientFailureDoesNotConsumeArtifact(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now: fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{{
			Artifact: "artifact-flaky", Method: brokerprovider.MethodSSOSAML,
			Outcome: brokerprovider.OutcomeUnavailable,
		}},
	})
	require.NoError(t, err)

	_, first := p.CompleteCallback(context.Background(), "artifact-flaky")
	assert.ErrorIs(t, first, brokerprovider.ErrProviderUnavailable)
	_, second := p.CompleteCallback(context.Background(), "artifact-flaky")
	assert.ErrorIs(t, second, brokerprovider.ErrProviderUnavailable)
}

// The callback artifact is one-time: replay is a definitive denial.
func TestScriptedProviderArtifactIsSingleUse(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now:      fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{successFixture("artifact-once", brokerprovider.MethodEmailMagicLink)},
	})
	require.NoError(t, err)

	_, err = p.CompleteCallback(context.Background(), "artifact-once")
	require.NoError(t, err)

	_, replayErr := p.CompleteCallback(context.Background(), "artifact-once")
	assert.ErrorIs(t, replayErr, brokerprovider.ErrDefinitiveDenial)
}

// Concurrent consumption of one artifact must yield exactly one success.
func TestScriptedProviderConcurrentCallbackYieldsExactlyOneSuccess(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now:      fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{successFixture("artifact-race", brokerprovider.MethodSSOOIDC)},
	})
	require.NoError(t, err)

	const workers = 32
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		denials   int
	)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, callErr := p.CompleteCallback(context.Background(), "artifact-race")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case callErr == nil:
				successes++
			case errors.Is(callErr, brokerprovider.ErrDefinitiveDenial):
				denials++
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one concurrent caller may consume the artifact")
	assert.Equal(t, workers-1, denials)
}

// An expired scripted member session is a definitive denial, never a success.
func TestScriptedProviderExpiredMemberSessionIsDenied(t *testing.T) {
	t.Parallel()
	fixture := successFixture("artifact-expired", brokerprovider.MethodEmailOTP)
	fixture.ExpiresAt = baseTime().Add(-time.Second)
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now:      fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{fixture},
	})
	require.NoError(t, err)

	_, err = p.CompleteCallback(context.Background(), "artifact-expired")
	assert.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)
}

func TestNewScriptedRejectsInvalidFixtures(t *testing.T) {
	t.Parallel()
	valid := successFixture("artifact-valid", brokerprovider.MethodEmailMagicLink)

	mutate := func(f func(*brokerprovider.Fixture)) brokerprovider.Fixture {
		fixture := valid
		f(&fixture)
		return fixture
	}

	cases := []struct {
		name    string
		fixture brokerprovider.Fixture
	}{
		{"empty artifact", mutate(func(f *brokerprovider.Fixture) { f.Artifact = "" })},
		{"unknown method", mutate(func(f *brokerprovider.Fixture) { f.Method = "telepathy" })},
		{"unknown outcome", mutate(func(f *brokerprovider.Fixture) { f.Outcome = "maybe" })},
		{"empty project", mutate(func(f *brokerprovider.Fixture) { f.ProjectID = "" })},
		{"empty organization", mutate(func(f *brokerprovider.Fixture) { f.OrganizationID = "" })},
		{"empty member", mutate(func(f *brokerprovider.Fixture) { f.MemberID = "" })},
		{"empty member session on success", mutate(func(f *brokerprovider.Fixture) { f.MemberSessionID = "" })},
		{"control character in member", mutate(func(f *brokerprovider.Fixture) { f.MemberID = "member\x00id" })},
		{"control character in artifact", mutate(func(f *brokerprovider.Fixture) { f.Artifact = "artifact\nid" })},
		{"invalid utf8 in organization", mutate(func(f *brokerprovider.Fixture) { f.OrganizationID = "org-\xff\xfe" })},
		{"too many runes in member session", mutate(func(f *brokerprovider.Fixture) {
			f.MemberSessionID = strings.Repeat("a", 256)
		})},
		{"too many bytes in member session", mutate(func(f *brokerprovider.Fixture) {
			f.MemberSessionID = strings.Repeat("é", 254) // 508 bytes, 254 runes — ok
		})},
		{"missing expiry on success", mutate(func(f *brokerprovider.Fixture) { f.ExpiresAt = time.Time{} })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
				Now:      fixedClock(baseTime()),
				Fixtures: []brokerprovider.Fixture{tc.fixture},
			})
			if tc.name == "too many bytes in member session" {
				// 254 two-byte runes are inside both bounds and must be accepted.
				require.NoError(t, err)
				return
			}
			require.Error(t, err, "fixture %q must fail closed", tc.name)
			assert.ErrorIs(t, err, brokerprovider.ErrInvalidFixture)
		})
	}
}

func TestNewScriptedRejectsOverlongMemberSessionBytes(t *testing.T) {
	t.Parallel()
	fixture := successFixture("artifact-bytes", brokerprovider.MethodEmailOTP)
	// 250 four-byte runes = 1000 bytes (ok); 260 four-byte runes exceeds runes.
	fixture.MemberSessionID = strings.Repeat("𝔘", 260)
	_, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now:      fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{fixture},
	})
	assert.ErrorIs(t, err, brokerprovider.ErrInvalidFixture)
}

func TestNewScriptedRejectsDuplicateArtifactsAndEmptyScript(t *testing.T) {
	t.Parallel()
	_, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now: fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{
			successFixture("dupe", brokerprovider.MethodEmailOTP),
			successFixture("dupe", brokerprovider.MethodSSOSAML),
		},
	})
	assert.ErrorIs(t, err, brokerprovider.ErrInvalidFixture)

	_, err = brokerprovider.NewScripted(brokerprovider.ScriptConfig{Now: fixedClock(baseTime())})
	assert.ErrorIs(t, err, brokerprovider.ErrInvalidFixture)
}

func TestStartLoginRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now:      fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{successFixture("artifact-start", brokerprovider.MethodEmailOTP)},
	})
	require.NoError(t, err)

	_, err = p.StartLogin(context.Background(), brokerprovider.StartRequest{Method: "carrier-pigeon"})
	assert.ErrorIs(t, err, brokerprovider.ErrUnsupportedMethod)
}

// Errors must never echo the artifact or provider payload back to a caller.
func TestProviderErrorsAreNonOracular(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now:      fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{successFixture("super-secret-artifact", brokerprovider.MethodEmailOTP)},
	})
	require.NoError(t, err)

	_, denyErr := p.CompleteCallback(context.Background(), "super-secret-artifact-guess")
	require.Error(t, denyErr)
	assert.NotContains(t, denyErr.Error(), "super-secret-artifact")
	assert.NotContains(t, denyErr.Error(), testMember)
	assert.NotContains(t, denyErr.Error(), testSession)
}

// The result type must not carry provider tokens or email material.
func TestCallbackResultExposesNoProviderSecretFields(t *testing.T) {
	t.Parallel()
	forbidden := []string{"token", "jwt", "secret", "password", "payload", "email", "credential", "cookie", "assertion"}
	typ := reflect.TypeOf(brokerprovider.CallbackResult{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.ToLower(field.Name)
		tag := strings.ToLower(string(field.Tag))
		for _, bad := range forbidden {
			assert.NotContains(t, name, bad, "CallbackResult must not expose %q field", field.Name)
			assert.NotContains(t, tag, bad, "CallbackResult must not tag %q with %q", field.Name, bad)
		}
	}
}

// The scripted provider must satisfy the vendor-neutral facade.
func TestScriptedProviderSatisfiesProviderInterface(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now:      fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{successFixture("iface", brokerprovider.MethodEmailOTP)},
	})
	require.NoError(t, err)
	var facade brokerprovider.Provider = p
	var typed brokerprovider.TypedProvider = p
	assert.NotNil(t, facade)
	assert.NotNil(t, typed)

	got, err := typed.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: brokerprovider.ArtifactTypeEmailOTP, Artifact: "iface",
	})
	require.NoError(t, err)
	assert.Equal(t, brokerprovider.OutcomeAuthenticated, got.Outcome)

	_, err = typed.CompleteTypedCallback(context.Background(), brokerprovider.CallbackRequest{
		Type: "password", Artifact: "iface",
	})
	assert.ErrorIs(t, err, brokerprovider.ErrDefinitiveDenial)
}

func TestCompleteCallbackHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	p, err := brokerprovider.NewScripted(brokerprovider.ScriptConfig{
		Now:      fixedClock(baseTime()),
		Fixtures: []brokerprovider.Fixture{successFixture("artifact-ctx", brokerprovider.MethodEmailOTP)},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.CompleteCallback(ctx, "artifact-ctx")
	assert.ErrorIs(t, err, brokerprovider.ErrProviderUnavailable)
}
