package authn_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

func TestValidateAcceptsSignedStudioJWT(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	sub := uuid.NewString()
	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, sub))

	v, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)

	got, err := v.Validate(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, authn.KindHuman, got.Kind)
	assert.Equal(t, jwttest.HumanSubjectRef(sub), got.SubjectRef)
	assert.Equal(t, jwttest.ClientID, got.ClientID)
	assert.Equal(t, jwttest.Audience, got.Audience)
	assert.Contains(t, got.Scopes, "openid")
	assert.Empty(t, got.SessionID)
}

func TestI12ServiceValidatorProofBindsSignedPublicClientID(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	serviceID := uuid.NewString()
	claims := jwttest.ValidServiceClaims(now, serviceID, "studio.read")
	claims.ClientID = "studio-machine"
	tok := jwttest.Mint(t, key, claims)
	v, err := authn.NewValidator(authn.Options{Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwks.URL, Now: func() time.Time { return now }})
	require.NoError(t, err)
	got, err := v.Validate(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, authn.KindService, got.Kind)
	assert.Equal(t, "identity:svc:"+serviceID, got.SubjectRef)
	assert.Equal(t, "studio-machine", got.ClientID)
	assert.Contains(t, got.Scopes, "studio.read")
}

func TestValidateRejectsNegatives(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	other := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	sub := uuid.NewString()
	valid := jwttest.ValidHumanClaims(now, sub)

	v, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)

	cases := []struct {
		name string
		tok  string
	}{
		{"wrong aud", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) { c.Audience = "primer-lms" }))},
		{"expired", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.IssuedAt = now.Add(-20 * time.Minute)
			c.NotBefore = c.IssuedAt
			c.ExpiresAt = now.Add(-5 * time.Minute)
		}))},
		{"unknown kid", jwttest.Mint(t, other, valid)},
		{"missing client_id", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.ClientID = ""
			c.Extra = map[string]any{"client_id": nil}
		}))},
		{"wrong client_id empty-ish", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) { c.ClientID = " " }))},
		{"overlong client_id", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) { c.ClientID = strings.Repeat("a", 129) }))},
		{"control-bearing client_id", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) { c.ClientID = "studio\x00bff" }))},
		{"internal uuid client_id", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) { c.ClientID = uuid.NewString() }))},
		{"azp present", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.Extra = map[string]any{"azp": "studio-bff"}
		}))},
		{"bad signature", jwttest.Mint(t, other, valid)[:len(jwttest.Mint(t, other, valid))-4] + "aaaa"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := v.Validate(context.Background(), tc.tok)
			require.Error(t, err)
			assert.ErrorIs(t, err, authn.ErrUnauthorized)
			assert.Empty(t, got.SubjectRef)
			assert.Empty(t, got.ClientID)
			assert.NotContains(t, strings.ToLower(err.Error()), "workspace")
		})
	}
}

func TestValidateRejectsJWKSFetchFailureClosed(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, uuid.NewString()))

	v, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  "http://127.0.0.1:1/.well-known/jwks.json",
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	_, err = v.Validate(context.Background(), tok)
	require.Error(t, err)
	assert.ErrorIs(t, err, authn.ErrUnauthorized)
}

func TestValidateKeepsPreviousKeyDuringRotationGrace(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	previous := jwttest.GenerateKey(t)
	current := jwttest.GenerateKey(t)
	oldBody, err := json.Marshal(jwttest.JWKSDocument(previous))
	require.NoError(t, err)
	newBody, err := json.Marshal(jwttest.JWKSDocument(current))
	require.NoError(t, err)
	var body atomic.Value
	body.Store(oldBody)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body.Load().([]byte))
	}))
	t.Cleanup(jwks.Close)

	v, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	oldToken := jwttest.Mint(t, previous, jwttest.ValidHumanClaims(now, uuid.NewString()))
	_, err = v.Validate(context.Background(), oldToken)
	require.NoError(t, err)

	// A new kid forces a successful refresh. The old key is no longer
	// published, but remains accepted for the bounded rotation grace period.
	body.Store(newBody)
	newToken := jwttest.Mint(t, current, jwttest.ValidHumanClaims(now, uuid.NewString()))
	_, err = v.Validate(context.Background(), newToken)
	require.NoError(t, err)
	_, err = v.Validate(context.Background(), oldToken)
	require.NoError(t, err)
}

func TestValidateAcceptsRotatedKid(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	current := jwttest.GenerateKey(t)
	previous := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, current, previous)
	tok := jwttest.Mint(t, previous, jwttest.ValidHumanClaims(now, uuid.NewString()))

	v, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	got, err := v.Validate(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, authn.KindHuman, got.Kind)
}

func TestValidateServiceSubject(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))

	v, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	got, err := v.Validate(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, authn.KindService, got.Kind)
	assert.Equal(t, "identity:svc:primer-lms", got.SubjectRef)
	assert.Contains(t, got.Scopes, "materialize:write")
}

func TestValidateIgnoresXUserIDHeaderTrustSurface(t *testing.T) {
	// The validator must only accept a signed JWT — never a caller identity header.
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	v, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	_, err = v.Validate(context.Background(), "")
	require.Error(t, err)
	assert.ErrorIs(t, err, authn.ErrUnauthorized)
}

func serveJWKS(t testing.TB, keys ...*jwttest.Keypair) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(jwttest.JWKSDocument(keys...))
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mutate(in jwttest.Claims, fn func(*jwttest.Claims)) jwttest.Claims {
	fn(&in)
	return in
}
