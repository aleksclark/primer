package token_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/token"
)

func replacePayload(t *testing.T, compact string, mutate func(map[string]any)) string {
	t.Helper()
	parts := strings.Split(compact, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(raw, &obj))
	mutate(obj)
	next, err := json.Marshal(obj)
	require.NoError(t, err)
	parts[1] = base64.RawURLEncoding.EncodeToString(next)
	return strings.Join(parts, ".")
}

func replaceHeader(t *testing.T, compact string, mutate func(map[string]any)) string {
	t.Helper()
	parts := strings.Split(compact, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(raw, &obj))
	mutate(obj)
	next, err := json.Marshal(obj)
	require.NoError(t, err)
	parts[0] = base64.RawURLEncoding.EncodeToString(next)
	return strings.Join(parts, ".")
}

func resign(t *testing.T, compact string, signer *testSigner) string {
	t.Helper()
	parts := strings.Split(compact, ".")
	require.Len(t, parts, 3)
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	der, err := signer.Sign(rand.Reader, sum[:], crypto.SHA256)
	require.NoError(t, err)
	var parsed struct{ R, S *big.Int }
	rest, err := asn1.Unmarshal(der, &parsed)
	require.NoError(t, err)
	require.Empty(t, rest)
	n := elliptic.P256().Params().N
	half := new(big.Int).Rsh(new(big.Int).Set(n), 1)
	if parsed.S.Cmp(half) > 0 {
		parsed.S = new(big.Int).Sub(n, parsed.S)
	}
	sig := make([]byte, 64)
	r := parsed.R.Bytes()
	s := parsed.S.Bytes()
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):], s)
	parts[2] = base64.RawURLEncoding.EncodeToString(sig)
	return strings.Join(parts, ".")
}

func TestIssueHumanRejectsMissingWrongOverlongControlAndInternalUUIDClientID(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)
	for _, clientID := range []string{
		"",
		"client\nid",
		"client\x00id",
		strings.Repeat("a", 129),
		uuid.NewString(),
		uuid.Nil.String(),
	} {
		in := humanInput(t)
		in.ClientID = clientID
		issued, err := minter.IssueHuman(context.Background(), in, commitOK)
		require.Error(t, err, clientID)
		assert.Empty(t, issued.Compact)
		assert.ErrorIs(t, err, token.ErrInvalid)
	}
}

func TestVerifyRejectsMissingWrongOverlongControlAndInternalUUIDClientID(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, signer, verifier, _ := issueHuman(t, clock)
	for _, clientID := range []string{"", "bad\nclient", strings.Repeat("x", 129), uuid.NewString()} {
		mutated := resign(t, replacePayload(t, issued.Compact, func(p map[string]any) {
			if clientID == "" {
				delete(p, "client_id")
			} else {
				p["client_id"] = clientID
			}
		}), signer)
		principal, err := verifier.Verify(context.Background(), mutated)
		requireDenied(t, err, principal)
	}
}

func TestVerifyRejectsAZPAndUnknownClaims(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, signer, verifier, _ := issueHuman(t, clock)
	for _, mutate := range []func(map[string]any){
		func(p map[string]any) { p["azp"] = testClientID },
		func(p map[string]any) { p["email"] = "a@example.test" },
		func(p map[string]any) { p["roles"] = []string{"admin"} },
		func(p map[string]any) { p["amr"] = []string{"pwd"} },
		func(p map[string]any) { p["sid"] = uuid.NewString() },
		func(p map[string]any) { p["auth_time"] = float64(clock.now.Unix()) },
		func(p map[string]any) { p["org"] = "acme" },
	} {
		principal, err := verifier.Verify(context.Background(), resign(t, replacePayload(t, issued.Compact, mutate), signer))
		requireDenied(t, err, principal)
	}
}

func TestVerifyRejectsWrongAndMultipleAudience(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, signer, src := func() (token.IssuedToken, *testSigner, *staticKeySource) {
		tok, s, _, source := issueHuman(t, clock)
		return tok, s, source
	}()
	wrongAud, err := token.NewVerifier(src, testIssuer, "primer-lms", clock, registeredClientLookup("primer-lms"))
	require.NoError(t, err)
	principal, err := wrongAud.Verify(context.Background(), issued.Compact)
	requireDenied(t, err, principal)

	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, registeredClientLookup(testAudience))
	require.NoError(t, err)
	arrayAud := resign(t, replacePayload(t, issued.Compact, func(p map[string]any) {
		p["aud"] = []string{testAudience, "primer-lms"}
	}), signer)
	principal, err = verifier.Verify(context.Background(), arrayAud)
	requireDenied(t, err, principal)
}

func TestIssueAndVerifyRejectNonCanonicalHumanSubject(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)
	for _, sub := range []string{"", "acct-1", uuid.Nil.String(), "identity:" + uuid.NewString(), strings.ToUpper(uuid.NewString())} {
		in := humanInput(t)
		in.Subject = sub
		issued, err := minter.IssueHuman(context.Background(), in, commitOK)
		require.Error(t, err, sub)
		assert.Empty(t, issued.Compact)
	}

	issued, signer, verifier, _ := issueHuman(t, clock)
	mutated := resign(t, replacePayload(t, issued.Compact, func(p map[string]any) {
		p["sub"] = "identity:svc:demo"
	}), signer)
	principal, err := verifier.Verify(context.Background(), mutated)
	requireDenied(t, err, principal)
}

func TestIssueHumanRejectsTTLOver900AndCapsByGrantAndProvider(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	clock := frozenClock{now: now}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)

	over := humanInput(t)
	over.TTL = 15*time.Minute + time.Second
	issued, err := minter.IssueHuman(context.Background(), over, commitOK)
	require.ErrorIs(t, err, token.ErrInvalid)
	assert.Empty(t, issued.Compact)

	capped := humanInput(t)
	capped.TTL = 15 * time.Minute
	capped.GrantNotAfter = now.Add(4 * time.Minute)
	capped.ProviderExpiresAt = now.Add(9 * time.Minute)
	issued, err = minter.IssueHuman(context.Background(), capped, commitOK)
	require.NoError(t, err)
	assert.Equal(t, 4*time.Minute, issued.Lifetime)
	assert.Equal(t, now.Add(4*time.Minute), issued.ExpiresAt)
}

func TestVerifyRejectsAlgSubstitutionUnknownKidExpiredAndFuture(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	issued, _, verifier, src := issueHuman(t, frozenClock{now: now})
	for _, mutate := range []func(map[string]any){
		func(h map[string]any) { h["alg"] = "none" },
		func(h map[string]any) { h["alg"] = "HS256" },
		func(h map[string]any) { h["alg"] = "RS256" },
		func(h map[string]any) { h["typ"] = "JWT" },
		func(h map[string]any) { h["jku"] = "https://evil.example" },
		func(h map[string]any) { delete(h, "kid") },
	} {
		principal, err := verifier.Verify(context.Background(), replaceHeader(t, issued.Compact, mutate))
		requireDenied(t, err, principal)
	}

	unknown, err := token.NewVerifier(src, testIssuer, testAudience, frozenClock{now: now}, registeredClientLookup(testAudience))
	require.NoError(t, err)
	principal, err := unknown.Verify(context.Background(), replaceHeader(t, issued.Compact, func(h map[string]any) {
		h["kid"] = uuid.NewString()
	}))
	requireDenied(t, err, principal)

	expired, err := token.NewVerifier(src, testIssuer, testAudience, frozenClock{now: now.Add(15*time.Minute + 6*time.Second)}, registeredClientLookup(testAudience))
	require.NoError(t, err)
	principal, err = expired.Verify(context.Background(), issued.Compact)
	requireDenied(t, err, principal)

	future, err := token.NewVerifier(src, testIssuer, testAudience, frozenClock{now: now.Add(-6 * time.Second)}, registeredClientLookup(testAudience))
	require.NoError(t, err)
	principal, err = future.Verify(context.Background(), issued.Compact)
	requireDenied(t, err, principal)
}

func TestVerifyAcceptsExactSkewBoundaries(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	issued, _, _, src := issueHuman(t, frozenClock{now: now})
	atExp, err := token.NewVerifier(src, testIssuer, testAudience, frozenClock{now: now.Add(15*time.Minute + 5*time.Second)}, registeredClientLookup(testAudience))
	require.NoError(t, err)
	_, err = atExp.Verify(context.Background(), issued.Compact)
	require.NoError(t, err)
	atFuture, err := token.NewVerifier(src, testIssuer, testAudience, frozenClock{now: now.Add(-5 * time.Second)}, registeredClientLookup(testAudience))
	require.NoError(t, err)
	_, err = atFuture.Verify(context.Background(), issued.Compact)
	require.NoError(t, err)
}

func TestVerifyRejectsDuplicateJSONKeysAndMalformedEncodings(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, signer, verifier, _ := issueHuman(t, clock)
	parts := strings.Split(issued.Compact, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	dup := bytes.Replace(payload, []byte(`"aud":`), []byte(`"aud":"x","aud":`), 1)
	parts[1] = base64.RawURLEncoding.EncodeToString(dup)
	principal, err := verifier.Verify(context.Background(), resign(t, strings.Join(parts, "."), signer))
	requireDenied(t, err, principal)

	cases := []string{
		issued.Compact + ".",
		issued.Compact + ".extra",
		parts[0] + "." + parts[1],
		strings.Repeat("a", 9000),
		"",
	}
	for _, compact := range cases {
		principal, err = verifier.Verify(context.Background(), compact)
		requireDenied(t, err, principal)
	}
}

func TestVerifyRejectsRawStytchMaterial(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	_, _, verifier, _ := issueHuman(t, clock)
	for _, raw := range []string{
		"stytch_session_token_live_abcdefghijklmnopqrstuvwxyz",
		"eyJhbGciOiJub25lIn0.eyJzdWIiOiJzdHl0Y2gifQ.",
	} {
		principal, err := verifier.Verify(context.Background(), raw)
		requireDenied(t, err, principal)
	}
}

func TestVerifyUsesClientRegistrationCallback(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, _, _, src := issueHuman(t, clock)
	var seen string
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, func(_ context.Context, clientID string) (token.ClientRegistration, error) {
		seen = clientID
		return token.ClientRegistration{
			ClientID: clientID, Audience: testAudience, SubjectClass: token.KindHuman, Scope: "openid studio:read",
		}, nil
	})
	require.NoError(t, err)
	got, err := verifier.Verify(context.Background(), issued.Compact)
	require.NoError(t, err)
	assert.Equal(t, testClientID, seen)
	assert.Equal(t, token.KindHuman, got.Kind)

	denied, err := token.NewVerifier(src, testIssuer, testAudience, clock, func(context.Context, string) (token.ClientRegistration, error) {
		return token.ClientRegistration{}, token.ErrInvalid
	})
	require.NoError(t, err)
	principal, err := denied.Verify(context.Background(), issued.Compact)
	requireDenied(t, err, principal)
}

func TestNewMinterRejectsHTTPIssuerAndMissingSource(t *testing.T) {
	_, err := token.NewMinter(nil, testIssuer, frozenClock{now: time.Now()})
	require.ErrorIs(t, err, token.ErrUnavailable)
	_, err = token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, "http://localhost:8090", nil)
	require.ErrorIs(t, err, token.ErrInvalid)
}

func TestIssueHumanFailsClosedOnSignerMismatchAndCancel(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	good := newTestSigner(t)
	minter, err := token.NewMinter(staticSignerSource{signer: &mismatchSigner{testSigner: *good}}, testIssuer, clock)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), commitOK)
	require.ErrorIs(t, err, token.ErrUnavailable)
	assert.Empty(t, issued.Compact)

	failing, err := token.NewMinter(staticSignerSource{err: errors.New("boom")}, testIssuer, clock)
	require.NoError(t, err)
	issued, err = failing.IssueHuman(context.Background(), humanInput(t), commitOK)
	require.ErrorIs(t, err, token.ErrUnavailable)
	assert.Empty(t, issued.Compact)
	assert.NotContains(t, err.Error(), "boom")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	live, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)
	issued, err = live.IssueHuman(ctx, humanInput(t), commitOK)
	require.ErrorIs(t, err, token.ErrUnavailable)
	assert.Empty(t, issued.Compact)
}

type mismatchSigner struct{ testSigner }

func (s *mismatchSigner) Public() crypto.PublicKey {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil
	}
	return &priv.PublicKey
}

func TestIssueHumanFreshnessRejectsStaleSignerClock(t *testing.T) {
	clock := &mutableClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(advancingSignerSource{
		signer: newTestSigner(t),
		clock:  clock,
		delta:  31 * time.Second,
	}, testIssuer, clock)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), commitOK)
	require.ErrorIs(t, err, token.ErrUnavailable)
	assert.Empty(t, issued.Compact)
}

type advancingSignerSource struct {
	signer token.Signer
	clock  *mutableClock
	delta  time.Duration
}

func (s advancingSignerSource) Current(context.Context) (token.Signer, *domain.SigningKey, error) {
	advanceMutableClock(s.clock, s.delta)
	jwk, err := s.signer.PublicJWK()
	if err != nil {
		return nil, nil, err
	}
	meta := &domain.SigningKey{Kid: jwk.Kid, Alg: domain.SigningAlgES256, Status: domain.SigningKeyStatusActive, PublicJWK: jwk}
	return s.signer, meta, nil
}

func TestKeySetParsesJWKSAndRejectsDuplicates(t *testing.T) {
	good := mustJWK(t, newTestSigner(t))
	other := mustJWK(t, newTestSigner(t))
	raw, err := json.Marshal(map[string]any{"keys": []domain.PublicJWK{good, other}})
	require.NoError(t, err)
	parsed, err := token.ParseJWKS(raw)
	require.NoError(t, err)
	require.Len(t, parsed, 2)

	dupDoc, err := json.Marshal(map[string]any{"keys": []domain.PublicJWK{good, good}})
	require.NoError(t, err)
	_, err = token.ParseJWKS(dupDoc)
	require.ErrorIs(t, err, token.ErrInvalid)

	oversized := make([]domain.PublicJWK, 17)
	for i := range oversized {
		oversized[i] = mustJWK(t, newTestSigner(t))
	}
	tooBig, err := json.Marshal(map[string]any{"keys": oversized})
	require.NoError(t, err)
	_, err = token.ParseJWKS(tooBig)
	require.ErrorIs(t, err, token.ErrInvalid)

	set, err := token.NewKeySet(func(context.Context) ([]domain.PublicJWK, error) {
		return []domain.PublicJWK{good}, nil
	})
	require.NoError(t, err)
	require.NoError(t, set.Refresh(context.Background()))
	got, ok, err := set.Lookup(context.Background(), good.Kid)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, good.Kid, got.Kid)
}

func TestConcurrentIssueVerifyRace(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	minter, err := token.NewMinter(staticSignerSource{signer: signer}, testIssuer, clock)
	require.NoError(t, err)
	src := &staticKeySource{keys: map[string]domain.PublicJWK{jwk.Kid: jwk}}
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, registeredClientLookup(testAudience))
	require.NoError(t, err)
	var wg sync.WaitGroup
	errCh := make(chan error, 64)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			issued, err := minter.IssueHuman(context.Background(), humanInput(t), commitOK)
			if err != nil {
				errCh <- err
				return
			}
			if _, err := verifier.Verify(context.Background(), issued.Compact); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func FuzzVerifyDoesNotPanicOrLeak(f *testing.F) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	issued, _, verifier, _ := issueHuman(f, clock)
	f.Add(issued.Compact)
	f.Add("")
	f.Add("a.b.c")
	f.Fuzz(func(t *testing.T, compact string) {
		principal, err := verifier.Verify(context.Background(), compact)
		if err != nil {
			requireDenied(t, err, principal)
			return
		}
		assertRedactedPrincipal(t, principal)
	})
}
