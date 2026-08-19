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

	"github.com/aleksclark/primer/agents/internal/authn"
	"github.com/aleksclark/primer/agents/internal/authn/jwttest"
)

// serveJWKS starts a test JWKS server for the given keypairs.
func serveJWKS(t *testing.T, keys ...*jwttest.Keypair) *httptest.Server {
	t.Helper()
	doc, err := json.Marshal(jwttest.JWKSDoc(keys...))
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(doc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newValidator(t *testing.T, jwksURL string, now func() time.Time) *authn.Validator {
	t.Helper()
	v, err := authn.NewValidator(authn.Options{
		Issuer:  jwttest.DefaultIssuer,
		JWKSURL: jwksURL,
		Now:     now,
	})
	require.NoError(t, err)
	return v
}

func TestValidateAcceptsSignedAgentsJWT(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	sub := "identity:" + uuid.NewString()

	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, sub))
	v := newValidator(t, srv.URL, func() time.Time { return now })

	got, err := v.Validate(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, authn.KindHuman, got.Kind)
	assert.Equal(t, sub, got.SubjectRef)
	assert.Equal(t, jwttest.DefaultClientID, got.ClientID)
	assert.Equal(t, authn.AudiencePrimerAgents, got.Audience)
	assert.True(t, got.HasScope(authn.ScopeRunsWrite))
	assert.True(t, got.HasScope(authn.ScopeRunsRead))

	ns := got.Namespace()
	assert.Contains(t, ns, sub)
	assert.Contains(t, ns, jwttest.DefaultClientID)
	assert.NotContains(t, ns, "Bearer")
}

func TestValidateAcceptsServicePrincipal(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)

	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", authn.ScopeRunsWrite))
	v := newValidator(t, srv.URL, func() time.Time { return now })

	got, err := v.Validate(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, authn.KindService, got.Kind)
	assert.Contains(t, got.SubjectRef, "identity:svc:")
}

func TestValidateRejectsNegatives(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	other := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	sub := "identity:" + uuid.NewString()
	v := newValidator(t, srv.URL, func() time.Time { return now })
	valid := jwttest.ValidHumanClaims(now, sub)

	mutate := func(base jwttest.Claims, fn func(*jwttest.Claims)) jwttest.Claims {
		fn(&base)
		return base
	}

	cases := []struct {
		name string
		tok  string
	}{
		{"empty", ""},
		{"not a jwt", "notajwt"},
		{"wrong audience", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.Audience = "primer-lms"
		}))},
		{"wrong issuer", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.Issuer = "https://evil.example"
		}))},
		{"expired", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.IssuedAt = now.Add(-20 * time.Minute)
			c.NotBefore = c.IssuedAt
			c.ExpiresAt = now.Add(-5 * time.Minute)
		}))},
		{"not yet valid", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.IssuedAt = now.Add(10 * time.Minute)
			c.NotBefore = c.IssuedAt
			c.ExpiresAt = c.IssuedAt.Add(5 * time.Minute)
		}))},
		{"unknown key", jwttest.Mint(t, other, valid)},
		{"missing jti", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.JTI = ""
			c.Extra = map[string]any{"jti": nil}
		}))},
		{"uuid jti", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.JTI = "not-a-uuid"
		}))},
		{"missing client_id", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.ClientID = ""
			c.Extra = map[string]any{"client_id": nil}
		}))},
		{"bare-uuid client_id", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.ClientID = uuid.NewString() // UUID is disallowed as client_id
		}))},
		{"client_id whitespace", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.ClientID = " " + jwttest.DefaultClientID
		}))},
		{"overlong client_id", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.ClientID = strings.Repeat("x", 129)
		}))},
		{"no scopes", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.Scope = ""
		}))},
		{"overlong lifetime", jwttest.Mint(t, key, mutate(valid, func(c *jwttest.Claims) {
			c.ExpiresAt = c.IssuedAt.Add(24 * time.Hour)
		}))},
		{"raw opaque bearer", "stytch_session_token_v1_opaque_bytes_here"},
		{"only two parts", key.Kid + ".payload"},
		{"trailing whitespace", jwttest.Mint(t, key, valid) + " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := v.Validate(context.Background(), tc.tok)
			require.ErrorIs(t, err, authn.ErrUnauthorized, "case %q must be rejected", tc.name)
		})
	}
}

func TestNamespaceIsDeterministicAndScopedToPrincipal(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	v := newValidator(t, srv.URL, func() time.Time { return now })

	subA := "identity:" + uuid.NewString()
	subB := "identity:" + uuid.NewString()

	tokA := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, subA))
	tokB := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, subB))

	pA, err := v.Validate(context.Background(), tokA)
	require.NoError(t, err)
	pB, err := v.Validate(context.Background(), tokB)
	require.NoError(t, err)

	assert.NotEqual(t, pA.Namespace(), pB.Namespace(), "different subjects must have different namespaces")

	// Same principal, same client → stable across re-validation.
	tokA2 := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, subA))
	pA2, err := v.Validate(context.Background(), tokA2)
	require.NoError(t, err)
	assert.Equal(t, pA.Namespace(), pA2.Namespace(), "namespace must be stable for same principal+client")
}

func TestValidatorFailsClosedWithoutJWKS(t *testing.T) {
	t.Parallel()
	_, err := authn.NewValidator(authn.Options{Issuer: jwttest.DefaultIssuer, JWKSURL: ""})
	require.ErrorIs(t, err, authn.ErrUnauthorized)

	_, err = authn.NewValidator(authn.Options{Issuer: "", JWKSURL: "http://localhost/jwks"})
	require.ErrorIs(t, err, authn.ErrUnauthorized)
}

func TestValidateFailsOnJWKSOutage(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	var reqs atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	v := newValidator(t, srv.URL, func() time.Time { return now })
	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, "identity:"+uuid.NewString()))

	_, err := v.Validate(context.Background(), tok)
	require.ErrorIs(t, err, authn.ErrUnauthorized)
	assert.Positive(t, reqs.Load(), "JWKS outage must be attempted, not silently skipped")
}

func TestValidateRotatesKeyGracefully(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	old := jwttest.GenerateKey(t)
	fresh := jwttest.GenerateKey(t)

	var mu atomic.Value
	mu.Store(jwttest.JWKSDoc(old))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := mu.Load().(map[string]any)
		b, _ := json.Marshal(doc)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	v := newValidator(t, srv.URL, func() time.Time { return now })
	tok := jwttest.Mint(t, old, jwttest.ValidHumanClaims(now, "identity:"+uuid.NewString()))

	// Token signed with old key is valid when old key is in JWKS.
	_, err := v.Validate(context.Background(), tok)
	require.NoError(t, err)

	// Rotate: JWKS now serves only fresh key.
	mu.Store(jwttest.JWKSDoc(fresh))

	// Old-key token still valid within rotation grace window (staleUntil not yet expired).
	_, err = v.Validate(context.Background(), tok)
	require.NoError(t, err)

	// New token with fresh key is accepted.
	tok2 := jwttest.Mint(t, fresh, jwttest.ValidHumanClaims(now, "identity:"+uuid.NewString()))
	_, err = v.Validate(context.Background(), tok2)
	require.NoError(t, err)
}
