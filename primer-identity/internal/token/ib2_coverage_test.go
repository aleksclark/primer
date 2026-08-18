package token_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/token"
)

func TestHashRefreshSecretRoundTrip(t *testing.T) {
	secret, err := token.GenerateRefreshSecret()
	require.NoError(t, err)
	sum, err := token.HashRefreshSecret(secret)
	require.NoError(t, err)
	assert.Len(t, sum, 32)
	_, err = token.HashRefreshSecret(secret[:8])
	require.ErrorIs(t, err, token.ErrInvalid)
}

func TestVerifyRejectsUnsortedScopeAndNBFMismatch(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, signer, verifier, _ := issueHuman(t, clock)
	unsorted := resign(t, replacePayload(t, issued.Compact, func(p map[string]any) {
		p["scope"] = "studio:read openid"
	}), signer)
	principal, err := verifier.Verify(context.Background(), unsorted)
	requireDenied(t, err, principal)

	nbf := resign(t, replacePayload(t, issued.Compact, func(p map[string]any) {
		p["nbf"] = p["iat"].(float64) - 1
	}), signer)
	principal, err = verifier.Verify(context.Background(), nbf)
	requireDenied(t, err, principal)

	tooLong := resign(t, replacePayload(t, issued.Compact, func(p map[string]any) {
		p["exp"] = p["iat"].(float64) + 901
	}), signer)
	principal, err = verifier.Verify(context.Background(), tooLong)
	requireDenied(t, err, principal)
}

func TestMinterIssuerMatchesValidatedConfigIssuer(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL:           "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable",
		Host:                  "127.0.0.1",
		Port:                  0,
		Env:                   "test",
		Issuer:                testIssuer + "/",
		ShutdownTimeout:       time.Second,
		HTTPReadHeaderTimeout: time.Second,
		HTTPMaxBodyBytes:      1,
	}
	require.NoError(t, cfg.Validate())

	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, cfg.Issuer, clock)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), commitOK)
	require.NoError(t, err)

	assert.Equal(t, testIssuer, cfg.Issuer)
	assert.Equal(t, testIssuer, decodePayload(t, issued.Compact)["iss"])
}

func TestNewVerifierRejectsHTTPIssuerAndEmptyAudience(t *testing.T) {
	src := &staticKeySource{}
	_, err := token.NewVerifier(src, "http://id.example", testAudience, nil, nil)
	require.ErrorIs(t, err, token.ErrInvalid)
	_, err = token.NewVerifier(src, testIssuer, "", nil, nil)
	require.ErrorIs(t, err, token.ErrInvalid)
}

func TestClientLookupUnavailableAndScopeMismatch(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, _, _, src := issueHuman(t, clock)
	unavail, err := token.NewVerifier(src, testIssuer, testAudience, clock, func(context.Context, string) (token.ClientRegistration, error) {
		return token.ClientRegistration{}, token.ErrUnavailable
	})
	require.NoError(t, err)
	principal, err := unavail.Verify(context.Background(), issued.Compact)
	requireDenied(t, err, principal)

	mismatch, err := token.NewVerifier(src, testIssuer, testAudience, clock, func(_ context.Context, clientID string) (token.ClientRegistration, error) {
		return token.ClientRegistration{ClientID: clientID, Audience: testAudience, SubjectClass: token.KindHuman, Scope: "other:scope"}, nil
	})
	require.NoError(t, err)
	principal, err = mismatch.Verify(context.Background(), issued.Compact)
	requireDenied(t, err, principal)
}

func TestVerifyDeniesHumanTokenWithoutClientLookup(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, _, _, src := issueHuman(t, clock)
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, nil)
	require.NoError(t, err)

	principal, err := verifier.Verify(context.Background(), issued.Compact)
	requireDenied(t, err, principal)
}

func TestParseClientAssertionAcceptsFutureNBFWithinAssertionSkew(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	clientID := "studio-confidential"
	aud := "https://identity.example.test/oauth/token"
	compact := mintAssertion(t, signer, clientID, aud, now, 5*time.Minute)
	in := token.AssertionInput{
		ClientID: clientID, Audience: aud, Registered: []domain.PublicJWK{jwk}, Now: now, MaxLifetime: 5 * time.Minute,
	}

	futureNBF := resign(t, replacePayload(t, compact, func(p map[string]any) {
		p["nbf"] = now.Add(30 * time.Second).Unix()
	}), signer)
	got, err := token.ParseClientAssertion(futureNBF, in)
	require.NoError(t, err)
	assert.Equal(t, clientID, got.ClientID)

	skewOK := resign(t, replacePayload(t, compact, func(p map[string]any) {
		p["nbf"] = now.Add(token.AssertionClockSkew).Unix()
	}), signer)
	got, err = token.ParseClientAssertion(skewOK, in)
	require.NoError(t, err)
	assert.Equal(t, clientID, got.ClientID)

	justExpired := token.AssertionInput{
		ClientID: clientID, Audience: aud, Registered: []domain.PublicJWK{jwk},
		Now: now.Add(5*time.Minute + token.AssertionClockSkew + time.Second), MaxLifetime: 5 * time.Minute,
	}
	_, err = token.ParseClientAssertion(compact, justExpired)
	require.ErrorIs(t, err, token.ErrInvalid)
}

func TestParseClientAssertionAcceptsOpaqueJTIAndRejectsUnsafeValues(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	clientID := "studio-confidential"
	aud := "https://identity.example.test/oauth/token"
	base := mintAssertion(t, signer, clientID, aud, now, time.Minute)
	in := token.AssertionInput{ClientID: clientID, Audience: aud, Registered: []domain.PublicJWK{jwk}, Now: now, MaxLifetime: 5 * time.Minute}

	opaque := "request-7f2c"
	got, err := token.ParseClientAssertion(resign(t, replacePayload(t, base, func(p map[string]any) {
		p["jti"] = opaque
	}), signer), in)
	require.NoError(t, err)
	assert.Equal(t, opaque, got.JTI)

	for _, tc := range []struct {
		name string
		jti  string
	}{
		{name: "empty", jti: ""},
		{name: "control", jti: "request-\n7f2c"},
		{name: "overlong", jti: strings.Repeat("a", 129)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := token.ParseClientAssertion(resign(t, replacePayload(t, base, func(p map[string]any) {
				p["jti"] = tc.jti
			}), signer), in)
			require.ErrorIs(t, err, token.ErrInvalid)
		})
	}
}

func TestParseClientAssertionOptionalNBFAndMissingKid(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	clientID := "studio-confidential"
	aud := "https://identity.example.test/oauth/token"
	compact := mintAssertion(t, signer, clientID, aud, now, time.Minute)
	withNBF := resign(t, replacePayload(t, compact, func(p map[string]any) {
		p["nbf"] = p["iat"]
	}), signer)
	got, err := token.ParseClientAssertion(withNBF, token.AssertionInput{
		ClientID: clientID, Audience: aud, Registered: []domain.PublicJWK{jwk}, Now: now, MaxLifetime: 5 * time.Minute,
	})
	require.NoError(t, err)
	assert.Equal(t, clientID, got.ClientID)

	_, err = token.ParseClientAssertion(compact, token.AssertionInput{
		ClientID: clientID, Audience: aud, Registered: []domain.PublicJWK{mustJWK(t, newTestSigner(t))}, Now: now,
	})
	require.ErrorIs(t, err, token.ErrInvalid)

	_, err = token.ParseClientAssertion(replaceHeader(t, compact, func(h map[string]any) {
		h["typ"] = "at+jwt"
	}), token.AssertionInput{ClientID: clientID, Audience: aud, Registered: []domain.PublicJWK{jwk}, Now: now})
	require.ErrorIs(t, err, token.ErrInvalid)
}

func TestIssueHumanCapsByProviderWhenShorterThanGrant(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, frozenClock{now: now})
	require.NoError(t, err)
	in := humanInput(t)
	in.GrantNotAfter = now.Add(10 * time.Minute)
	in.ProviderExpiresAt = now.Add(3 * time.Minute)
	issued, err := minter.IssueHuman(context.Background(), in, commitOK)
	require.NoError(t, err)
	assert.Equal(t, 3*time.Minute, issued.Lifetime)
}

func TestIssueHumanRejectsProviderAlreadyExpired(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, frozenClock{now: now})
	require.NoError(t, err)
	in := humanInput(t)
	in.ProviderExpiresAt = now
	issued, err := minter.IssueHuman(context.Background(), in, commitOK)
	require.ErrorIs(t, err, token.ErrInvalid)
	assert.Empty(t, issued.Compact)
}

func TestKeySetRejectsEmptyDuplicateAndCanceledRefresh(t *testing.T) {
	good := mustJWK(t, newTestSigner(t))
	set, err := token.NewKeySet(func(context.Context) ([]domain.PublicJWK, error) {
		return nil, nil
	})
	require.NoError(t, err)
	require.ErrorIs(t, set.Refresh(context.Background()), token.ErrInvalid)

	set, err = token.NewKeySet(func(context.Context) ([]domain.PublicJWK, error) {
		return []domain.PublicJWK{good, good}, nil
	})
	require.NoError(t, err)
	require.ErrorIs(t, set.Refresh(context.Background()), token.ErrInvalid)

	set, err = token.NewKeySet(func(context.Context) ([]domain.PublicJWK, error) {
		return []domain.PublicJWK{good}, nil
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, set.Refresh(ctx), token.ErrUnavailable)
}

func TestUnknownKidRefreshCooldown(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	src := &staticKeySource{}
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, registeredClientLookup(testAudience))
	require.NoError(t, err)
	issued, _, _, _ := issueHuman(t, clock)
	principal, err := verifier.Verify(context.Background(), issued.Compact)
	requireDenied(t, err, principal)
	assert.Equal(t, 1, src.refreshCount())
	principal, err = verifier.Verify(context.Background(), issued.Compact)
	requireDenied(t, err, principal)
	assert.Equal(t, 1, src.refreshCount())
}

func TestIssueHumanRejectsNonSecondTTL(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)
	in := humanInput(t)
	in.TTL = 90*time.Second + 50*time.Millisecond
	issued, err := minter.IssueHuman(context.Background(), in, commitOK)
	require.ErrorIs(t, err, token.ErrInvalid)
	assert.Empty(t, issued.Compact)
}

func TestVerifyRejectsPaddedBase64AndControlKid(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, _, verifier, _ := issueHuman(t, clock)
	parts := strings.Split(issued.Compact, ".")
	principal, err := verifier.Verify(context.Background(), parts[0]+"=."+parts[1]+"."+parts[2])
	requireDenied(t, err, principal)
	principal, err = verifier.Verify(context.Background(), replaceHeader(t, issued.Compact, func(h map[string]any) {
		h["kid"] = "kid\n"
	}))
	requireDenied(t, err, principal)
}

func TestParseJWKSAcceptsCanonicalDocuments(t *testing.T) {
	good := mustJWK(t, newTestSigner(t))
	raw, err := good.MarshalJSON()
	require.NoError(t, err)
	doc := []byte(`{"keys":[` + string(raw) + `]}`)
	got, err := token.ParseJWKS(doc)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, good.Kid, got[0].Kid)
}

func TestIssueHumanRejectsEmptySubjectAndScopeToken(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)
	in := humanInput(t)
	in.Subject = uuid.Nil.String()
	issued, err := minter.IssueHuman(context.Background(), in, commitOK)
	require.ErrorIs(t, err, token.ErrInvalid)
	assert.Empty(t, issued.Compact)
}
