package token_test

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/token"
)

const (
	testIssuer   = "https://identity.example.test"
	testAudience = "curriculum-studio"
	testClientID = "studio-bff"
)

type frozenClock struct{ now time.Time }

func (c frozenClock) Now() time.Time { return c.now }

type mutableClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func advanceMutableClock(c *mutableClock, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type testSigner struct{ mat *keys.Material }

func newTestSigner(t testing.TB) *testSigner {
	t.Helper()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	return &testSigner{mat: mat}
}

func (s *testSigner) Public() crypto.PublicKey { return s.mat.Public() }

func (s *testSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return s.mat.Sign(rand, digest, opts)
}

func (s *testSigner) PublicJWK() (domain.PublicJWK, error) { return s.mat.PublicJWK() }

type staticSignerSource struct {
	signer token.Signer
	err    error
}

func (s staticSignerSource) Current(context.Context) (token.Signer, *domain.SigningKey, error) {
	if s.err != nil {
		return nil, nil, s.err
	}
	jwk, err := s.signer.PublicJWK()
	if err != nil {
		return nil, nil, err
	}
	meta := &domain.SigningKey{
		Kid: jwk.Kid, Alg: domain.SigningAlgES256, Status: domain.SigningKeyStatusActive, PublicJWK: jwk,
	}
	return s.signer, meta, nil
}

type staticKeySource struct {
	mu         sync.Mutex
	keys       map[string]domain.PublicJWK
	refreshes  int
	onRefresh  map[string]domain.PublicJWK
	refreshErr error
	started    chan struct{}
	block      chan struct{}
}

func (s *staticKeySource) Lookup(_ context.Context, kid string) (domain.PublicJWK, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	jwk, ok := s.keys[kid]
	return jwk, ok, nil
}

func (s *staticKeySource) Refresh(ctx context.Context) error {
	s.mu.Lock()
	s.refreshes++
	started := s.started
	block := s.block
	s.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refreshErr != nil {
		return s.refreshErr
	}
	if s.onRefresh != nil {
		if s.keys == nil {
			s.keys = map[string]domain.PublicJWK{}
		}
		for kid, jwk := range s.onRefresh {
			s.keys[kid] = jwk
		}
	}
	return nil
}

func (s *staticKeySource) refreshCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshes
}

func mustJWK(t testing.TB, s token.Signer) domain.PublicJWK {
	t.Helper()
	jwk, err := s.PublicJWK()
	require.NoError(t, err)
	return jwk
}

func humanInput(t testing.TB) token.HumanInput {
	t.Helper()
	return token.HumanInput{
		Subject:  uuid.NewString(),
		Audience: testAudience,
		ClientID: testClientID,
		Scope:    "openid studio:read",
		TTL:      15 * time.Minute,
	}
}

func commitOK(context.Context, token.IssuedToken) error { return nil }

func issueHuman(t testing.TB, clock frozenClock) (token.IssuedToken, *testSigner, *token.Verifier, *staticKeySource) {
	t.Helper()
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	minter, err := token.NewMinter(staticSignerSource{signer: signer}, testIssuer, clock)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), commitOK)
	require.NoError(t, err)
	src := &staticKeySource{keys: map[string]domain.PublicJWK{jwk.Kid: jwk}}
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, nil)
	require.NoError(t, err)
	return issued, signer, verifier, src
}

func decodePayload(t *testing.T, compact string) map[string]any {
	t.Helper()
	parts := strings.Split(compact, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(raw, &obj))
	return obj
}

func decodeHeader(t *testing.T, compact string) map[string]any {
	t.Helper()
	parts := strings.Split(compact, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(raw, &obj))
	return obj
}

func requireDenied(t *testing.T, err error, principal token.Principal) {
	t.Helper()
	require.Error(t, err)
	assert.True(t, errors.Is(err, token.ErrInvalid) || errors.Is(err, token.ErrUnavailable), err)
	assert.Empty(t, principal)
	assert.NotContains(t, err.Error(), "eyJ")
	assert.NotContains(t, strings.ToLower(err.Error()), "private")
}

func assertRedactedPrincipal(t testing.TB, p token.Principal) {
	t.Helper()
	sensitive := []string{p.Subject, p.JTI, p.ClientID, p.Scope, p.Issuer, p.Audience}
	for _, ts := range []time.Time{p.IssuedAt, p.NotBefore, p.ExpiresAt} {
		if !ts.IsZero() {
			sensitive = append(sensitive, ts.UTC().Format(time.RFC3339Nano), fmt.Sprintf("%d", ts.Unix()))
		}
	}
	rendered := []string{
		p.String(), p.GoString(),
		fmt.Sprintf("%v", p), fmt.Sprintf("%+v", p), fmt.Sprintf("%#v", p),
		fmt.Sprintf("%v", &p), fmt.Sprintf("%+v", &p), fmt.Sprintf("%#v", &p),
	}
	for _, text := range rendered {
		assert.NotContains(t, text, "eyJ")
		for _, secret := range sensitive {
			if secret == "" {
				continue
			}
			assert.NotContains(t, text, secret)
		}
	}
	_, err := json.Marshal(p)
	require.Error(t, err)
	_, err = json.Marshal(&p)
	require.Error(t, err)
}

func TestIssueHumanAndVerifyRoundTrip(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 4, 5, 123456789, time.UTC)
	clock := frozenClock{now: now}
	signer := newTestSigner(t)
	jwk := mustJWK(t, signer)
	minter, err := token.NewMinter(staticSignerSource{signer: signer}, testIssuer, clock)
	require.NoError(t, err)

	in := humanInput(t)
	var persisted token.IssuedToken
	issued, err := minter.IssueHuman(context.Background(), in, func(_ context.Context, tok token.IssuedToken) error {
		persisted = tok
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, issued.Compact)
	require.LessOrEqual(t, len(issued.Compact), 8*1024)
	assert.Equal(t, 15*time.Minute, issued.Lifetime)
	assert.Equal(t, issued.Compact, persisted.Compact)
	assert.Equal(t, issued.Lifetime, persisted.Lifetime)
	assert.Equal(t, now.UTC().Truncate(time.Second), issued.IssuedAt)
	assert.Equal(t, issued.IssuedAt, issued.NotBefore)
	assert.Equal(t, issued.IssuedAt.Add(15*time.Minute), issued.ExpiresAt)
	assert.Equal(t, jwk.Kid, issued.Kid)
	_, err = uuid.Parse(issued.JTI)
	require.NoError(t, err)

	src := &staticKeySource{keys: map[string]domain.PublicJWK{jwk.Kid: jwk}}
	verifier, err := token.NewVerifier(src, testIssuer, testAudience, clock, nil)
	require.NoError(t, err)
	got, err := verifier.Verify(context.Background(), issued.Compact)
	require.NoError(t, err)
	assert.Equal(t, token.KindHuman, got.Kind)
	assert.Equal(t, in.Subject, got.Subject)
	assert.Equal(t, testAudience, got.Audience)
	assert.Equal(t, testIssuer, got.Issuer)
	assert.Equal(t, in.ClientID, got.ClientID)
	assert.Equal(t, "openid studio:read", got.Scope)
	assert.Equal(t, issued.JTI, got.JTI)
	assert.Equal(t, issued.IssuedAt, got.IssuedAt)
	assert.Equal(t, issued.ExpiresAt, got.ExpiresAt)
	assertRedactedPrincipal(t, got)

	header := decodeHeader(t, issued.Compact)
	assert.Equal(t, map[string]any{"alg": "ES256", "typ": "at+jwt", "kid": header["kid"]}, header)
	payload := decodePayload(t, issued.Compact)
	assert.Equal(t, in.Subject, payload["sub"])
	aud, ok := payload["aud"].(string)
	require.True(t, ok)
	assert.Equal(t, testAudience, aud)
	assert.Equal(t, testClientID, payload["client_id"])
	assert.Equal(t, float64(900), payload["exp"].(float64)-payload["iat"].(float64))
	for _, forbidden := range []string{
		"azp", "amr", "auth_time", "sid", "email", "email_verified", "roles",
		"org", "organization_id", "workspace", "tenant", "provider",
		"provider_member_id", "session_id", "stytch",
	} {
		_, present := payload[forbidden]
		assert.False(t, present, forbidden)
	}
}

func TestIssueHumanRequiresPersistCallback(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), nil)
	require.ErrorIs(t, err, token.ErrUnavailable)
	assert.Empty(t, issued.Compact)
}

func TestIssueHumanDiscardsTokenWhenPersistFails(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), func(context.Context, token.IssuedToken) error {
		return errors.New("commit failed")
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, token.ErrUnavailable), err)
	assert.Empty(t, issued.Compact)
	assert.NotContains(t, err.Error(), "commit failed")
	assert.NotContains(t, err.Error(), "eyJ")
}

func TestIssueHumanSurfacesRetryablePersistFailureForCallerRetry(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)
	issued, err := minter.IssueHuman(context.Background(), humanInput(t), func(context.Context, token.IssuedToken) error {
		return errors.New("create token issuance audit: ERROR: could not serialize access due to read/write dependencies among transactions (SQLSTATE 40001)")
	})
	require.ErrorIs(t, err, domain.ErrRetryableSerialization)
	assert.Equal(t, domain.ErrRetryableSerialization.Error(), err.Error())
	assert.Empty(t, issued.Compact)
	assert.NotContains(t, err.Error(), "SQLSTATE")
	assert.NotContains(t, err.Error(), "audit")
	assert.NotContains(t, err.Error(), "eyJ")
}

func TestIssueHumanHasNoServiceMintPath(t *testing.T) {
	_, ok := any((*token.Minter)(nil)).(interface {
		MintService(context.Context, any) (string, error)
	})
	assert.False(t, ok)
	_, ok = any((*token.Minter)(nil)).(interface {
		IssueService(context.Context, any, func(context.Context, token.IssuedToken) error) (token.IssuedToken, error)
	})
	assert.False(t, ok)
}

func TestIssueHumanWithSignerUsesPreparedSignerAndRejectsCanceledContext(t *testing.T) {
	clock := frozenClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	minter, err := token.NewMinter(staticSignerSource{signer: newTestSigner(t)}, testIssuer, clock)
	require.NoError(t, err)

	prepared := newTestSigner(t)
	jwk := mustJWK(t, prepared)
	meta := &domain.SigningKey{Kid: jwk.Kid, Alg: domain.SigningAlgES256, Status: domain.SigningKeyStatusActive, PublicJWK: jwk}
	issued, err := minter.IssueHumanWithSigner(context.Background(), humanInput(t), prepared, meta, commitOK)
	require.NoError(t, err)
	require.Equal(t, jwk.Kid, issued.Kid)
	require.NotEmpty(t, issued.Compact)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = minter.IssueHumanWithSigner(canceled, humanInput(t), prepared, meta, commitOK)
	require.ErrorIs(t, err, token.ErrUnavailable)
}
