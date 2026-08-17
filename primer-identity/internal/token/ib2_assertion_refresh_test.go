package token_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/token"
)

func TestParseClientAssertionAcceptsExactES256Form(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	clientID := "studio-confidential"
	aud := "https://identity.example.test/oauth/token"
	compact := mintAssertion(t, signer, clientID, aud, now, 5*time.Minute)

	got, err := token.ParseClientAssertion(compact, token.AssertionInput{
		ClientID:    clientID,
		Audience:    aud,
		Registered:  []domain.PublicJWK{jwk},
		Now:         now,
		MaxLifetime: 5 * time.Minute,
	})
	require.NoError(t, err)
	assert.Equal(t, clientID, got.ClientID)
	assert.Equal(t, clientID, got.Subject)
	assert.Equal(t, aud, got.Audience)
	assert.Equal(t, jwk.Kid, got.Kid)
	_, err = uuid.Parse(got.JTI)
	require.NoError(t, err)
}

func TestParseClientAssertionRejectsWrongFormAndLifetime(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	clientID := "studio-confidential"
	aud := "https://identity.example.test/oauth/token"
	good := mintAssertion(t, signer, clientID, aud, now, 5*time.Minute)
	in := token.AssertionInput{
		ClientID: clientID, Audience: aud, Registered: []domain.PublicJWK{jwk}, Now: now, MaxLifetime: 5 * time.Minute,
	}

	_, err := token.ParseClientAssertion(replaceHeader(t, good, func(h map[string]any) { h["alg"] = "none" }), in)
	require.ErrorIs(t, err, token.ErrInvalid)

	wrongISS := resign(t, replacePayload(t, good, func(p map[string]any) { p["iss"] = "other-client" }), signer)
	_, err = token.ParseClientAssertion(wrongISS, in)
	require.ErrorIs(t, err, token.ErrInvalid)

	long := mintAssertion(t, signer, clientID, aud, now, 5*time.Minute+time.Second)
	_, err = token.ParseClientAssertion(long, in)
	require.ErrorIs(t, err, token.ErrInvalid)

	_, err = token.ParseClientAssertion(good, token.AssertionInput{
		ClientID: clientID, Audience: "https://identity.example.test/oauth/revoke", Registered: []domain.PublicJWK{jwk}, Now: now, MaxLifetime: 5 * time.Minute,
	})
	require.ErrorIs(t, err, token.ErrInvalid)
}

func mintAssertion(t *testing.T, signer *testSigner, clientID, aud string, now time.Time, ttl time.Duration) string {
	t.Helper()
	jwk := mustJWK(t, signer)
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": jwk.Kid, "typ": "JWT"})
	require.NoError(t, err)
	iat := now.UTC().Unix()
	payload, err := json.Marshal(map[string]any{
		"iss": clientID, "sub": clientID, "aud": aud,
		"iat": iat, "exp": iat + int64(ttl/time.Second), "jti": uuid.NewString(),
	})
	require.NoError(t, err)
	compact := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
	return resign(t, compact, signer)
}

func TestGenerateRefreshSecretIsOpaqueAndHashable(t *testing.T) {
	secret, err := token.GenerateRefreshSecret()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(secret), 16)
	other, err := token.GenerateRefreshSecret()
	require.NoError(t, err)
	assert.NotEqual(t, secret, other)
	sum := sha256.Sum256(secret)
	assert.Len(t, sum, 32)
	assert.NotContains(t, string(secret), "eyJ")
}

func TestUnknownKidForcesExactlyOneRefresh(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	minter, err := token.NewMinter(staticSignerSource{signer: signer}, testIssuer, clock)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), commitOK)
	require.NoError(t, err)
	src := &staticKeySource{onRefresh: map[string]domain.PublicJWK{jwk.Kid: jwk}}
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, nil)
	require.NoError(t, err)
	got, err := verifier.Verify(context.Background(), issued.Compact)
	require.NoError(t, err)
	assert.Equal(t, 1, src.refreshCount())
	assert.Equal(t, token.KindHuman, got.Kind)
}

func TestVerifyRejectsHighSAndDERSignatures(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, _, verifier, _ := issueHuman(t, clock)
	parts := strings.Split(issued.Compact, ".")
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	require.NoError(t, err)
	require.Len(t, sig, 64)
	flipped := append([]byte(nil), sig...)
	flipped[63] ^= 0x01
	parts[2] = base64.RawURLEncoding.EncodeToString(flipped)
	principal, err := verifier.Verify(context.Background(), strings.Join(parts, "."))
	requireDenied(t, err, principal)
}

func TestIssueHumanRejectsAlreadyExpiredGrant(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, frozenClock{now: now})
	require.NoError(t, err)
	in := humanInput(t)
	in.GrantNotAfter = now
	issued, err := minter.IssueHuman(context.Background(), in, commitOK)
	require.ErrorIs(t, err, token.ErrInvalid)
	assert.Empty(t, issued.Compact)
}

func TestPrincipalFormattingAndJSONDoNotExposeClaims(t *testing.T) {
	issued, _, verifier, _ := issueHuman(t, frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)})
	got, err := verifier.Verify(context.Background(), issued.Compact)
	require.NoError(t, err)
	assertRedactedPrincipal(t, got)
}

func TestNilClockUsesRealTime(t *testing.T) {
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	minter, err := token.NewMinter(staticSignerSource{signer: signer}, testIssuer, nil)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), commitOK)
	require.NoError(t, err)
	src := &staticKeySource{keys: map[string]domain.PublicJWK{jwk.Kid: jwk}}
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, nil, nil)
	require.NoError(t, err)
	got, err := verifier.Verify(context.Background(), issued.Compact)
	require.NoError(t, err)
	assert.Equal(t, token.KindHuman, got.Kind)
	assert.WithinDuration(t, time.Now().UTC(), got.IssuedAt, 2*time.Second)
}

func TestVerifyRejectsServiceSubjectWithoutRegistration(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, signer, verifier, _ := issueHuman(t, clock)
	mutated := resign(t, replacePayload(t, issued.Compact, func(p map[string]any) {
		p["sub"] = "identity:svc:jobs"
	}), signer)
	principal, err := verifier.Verify(context.Background(), mutated)
	requireDenied(t, err, principal)
}

func TestVerifyAcceptsServiceSubjectWithRegistration(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, signer, _, src := issueHuman(t, clock)
	serviceTok := resign(t, replacePayload(t, issued.Compact, func(p map[string]any) {
		p["sub"] = "identity:svc:jobs"
		p["client_id"] = "tv-ingest"
		p["scope"] = "tv:ingest"
	}), signer)
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, func(_ context.Context, clientID string) (token.ClientRegistration, error) {
		return token.ClientRegistration{
			ClientID: clientID, Audience: testAudience, SubjectClass: token.KindService, Scope: "tv:ingest",
		}, nil
	})
	require.NoError(t, err)
	got, err := verifier.Verify(context.Background(), serviceTok)
	require.NoError(t, err)
	assert.Equal(t, token.KindService, got.Kind)
	assert.Equal(t, "identity:svc:jobs", got.Subject)
}

func TestMinterAndIssuedTokenRefuseSerialization(t *testing.T) {
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, frozenClock{now: time.Now()})
	require.NoError(t, err)
	assert.Equal(t, "token.Minter{redacted}", minter.String())
	_, err = json.Marshal(minter)
	require.Error(t, err)
	_, err = json.Marshal(token.IssuedToken{Compact: "eyJ"})
	require.Error(t, err)
}

func TestIssueHumanRejectsEmptyScopeAndAudience(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)
	for _, mutate := range []func(*token.HumanInput){
		func(in *token.HumanInput) { in.Audience = "" },
		func(in *token.HumanInput) { in.Audience = "lms,studio" },
		func(in *token.HumanInput) { in.Scope = "" },
		func(in *token.HumanInput) { in.TTL = 0 },
	} {
		in := humanInput(t)
		mutate(&in)
		issued, err := minter.IssueHuman(context.Background(), in, commitOK)
		require.ErrorIs(t, err, token.ErrInvalid)
		assert.Empty(t, issued.Compact)
	}
}
