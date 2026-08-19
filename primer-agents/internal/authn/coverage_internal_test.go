package authn

import (
	"context"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCoverageValidatorGuards(t *testing.T) {
	_, err := NewValidator(Options{})
	require.ErrorIs(t, err, ErrUnauthorized)
	var v *Validator
	_, err = v.Validate(context.Background(), "")
	require.ErrorIs(t, err, ErrUnauthorized)
	valid, err := NewValidator(Options{Issuer: "issuer", JWKSURL: "http://127.0.0.1:1"})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = valid.Validate(ctx, "")
	require.ErrorIs(t, err, ErrUnauthorized)

	kind, subject, err := classifySubject("identity:11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Equal(t, KindHuman, kind)
	require.Equal(t, "identity:11111111-1111-4111-8111-111111111111", subject)
}

func TestCoverageECPublicParsing(t *testing.T) {
	x, y := elliptic.P256().ScalarBaseMult([]byte{1})
	xb := base64.RawURLEncoding.EncodeToString(x.FillBytes(make([]byte, 32)))
	yb := base64.RawURLEncoding.EncodeToString(y.FillBytes(make([]byte, 32)))
	pub, err := parseECPublic(xb, yb)
	require.NoError(t, err)
	require.NotNil(t, pub)
	_, err = parseECPublic("bad", yb)
	require.Error(t, err)
	_, err = parseECPublic(xb, base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	require.Error(t, err)
}

func TestCoverageJWTParsingHelpers(t *testing.T) {
	kid, err := parseHeader([]byte(`{"alg":"ES256","typ":"at+jwt","kid":"key-1"}`))
	require.NoError(t, err)
	require.Equal(t, "key-1", kid)
	_, err = parseHeader([]byte(`{"alg":"none","typ":"JWT","kid":"x"}`))
	require.Error(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	claims := map[string]any{
		"iss": "https://identity.example", "sub": "identity:svc:worker-1", "aud": AudiencePrimerAgents,
		"exp": now.Add(time.Minute).Unix(), "iat": now.Unix(), "nbf": now.Unix(),
		"jti": "11111111-1111-4111-8111-111111111111", "client_id": "agents-client", "scope": "agents:runs:read agents:runs:write",
	}
	raw, err := json.Marshal(claims)
	require.NoError(t, err)
	parsed, err := parseClaims(raw)
	require.NoError(t, err)
	require.Equal(t, "identity:svc:worker-1", parsed.sub)
	require.NoError(t, validateLifetime(parsed, now))

	_, _, err = classifySubject("identity:svc:bad id")
	require.Error(t, err)
	_, _, err = classifySubject("identity:not-a-uuid")
	require.Error(t, err)
	require.NoError(t, validateServiceID("worker_1.v2"))
	require.Error(t, validateServiceID("bad id"))
	require.NoError(t, requirePublicClientID("public-client"))
	require.Error(t, requirePublicClientID("11111111-1111-4111-8111-111111111111"))
	_, err = splitScopes("one one two")
	require.NoError(t, err)
	_, err = splitScopes("")
	require.Error(t, err)

	_, err = decodeSeg("not-base64!", 32)
	require.Error(t, err)
	_, err = decodeStrictObject([]byte(`{"unknown":true}`), map[string]struct{}{"known": {}}, 100)
	require.Error(t, err)
	_, ok := asInt(json.Number("42"))
	require.True(t, ok)
	_, ok = asInt("42")
	require.False(t, ok)
}
