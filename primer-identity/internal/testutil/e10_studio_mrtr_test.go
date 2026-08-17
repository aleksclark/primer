package testutil_test

import (
	"context"
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/token"
)

// E10 is Identity-local Studio-like MRTR conformance evidence for later
// S19/IB8. It is not a Studio runtime or production MCP implementation.

const (
	e10Issuer   = "https://identity.example.test"
	e10Audience = "curriculum-studio"
	e10ClientID = "studio-bff"
	e10Tool     = "studio.publish.confirm"
)

type e10Clock struct{ now time.Time }

func (c e10Clock) Now() time.Time { return c.now }

type e10Signer struct{ mat *keys.Material }

func newE10Signer(t *testing.T) *e10Signer {
	t.Helper()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	return &e10Signer{mat: mat}
}

func (s *e10Signer) Public() crypto.PublicKey { return s.mat.Public() }
func (s *e10Signer) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return s.mat.Sign(rand, digest, opts)
}
func (s *e10Signer) PublicJWK() (domain.PublicJWK, error) { return s.mat.PublicJWK() }

type e10SignerSource struct{ signer token.Signer }

func (s e10SignerSource) Current(context.Context) (token.Signer, *domain.SigningKey, error) {
	jwk, err := s.signer.PublicJWK()
	if err != nil {
		return nil, nil, err
	}
	meta := &domain.SigningKey{
		Kid: jwk.Kid, Alg: domain.SigningAlgES256, Status: domain.SigningKeyStatusActive, PublicJWK: jwk,
	}
	return s.signer, meta, nil
}

func e10HumanInput(subject string) token.HumanInput {
	return token.HumanInput{
		Subject:  subject,
		Audience: e10Audience,
		ClientID: e10ClientID,
		Scope:    "openid studio:read",
		TTL:      15 * time.Minute,
	}
}

func startE10JWKS(t *testing.T, pubs []domain.PublicJWK) *httptest.Server {
	t.Helper()
	_, handler := api.New(nil, api.Options{
		Issuer: e10Issuer,
		JWKS:   staticE10JWKS{pubs: pubs, etag: `W/"e10"`},
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

type staticE10JWKS struct {
	pubs []domain.PublicJWK
	etag string
}

func (s staticE10JWKS) PublicJWKS(context.Context) ([]domain.PublicJWK, error) { return s.pubs, nil }
func (s staticE10JWKS) PublicSetETag(context.Context) (string, error)          { return s.etag, nil }

func issueE10Human(t *testing.T, clock e10Clock) (token.IssuedToken, *e10Signer, string, [32]byte) {
	t.Helper()
	signer := newE10Signer(t)
	minter, err := token.NewMinter(e10SignerSource{signer: signer}, e10Issuer, clock)
	require.NoError(t, err)
	subject := uuid.NewString()
	issued, err := minter.IssueHuman(context.Background(), e10HumanInput(subject), func(context.Context, token.IssuedToken) error { return nil })
	require.NoError(t, err)
	var draft [32]byte
	_, err = rand.Read(draft[:])
	require.NoError(t, err)
	return issued, signer, subject, draft
}

func newE10Harness(t *testing.T, clock e10Clock, signer *e10Signer) *testutil.E10StudioMRTR {
	t.Helper()
	jwk, err := signer.PublicJWK()
	require.NoError(t, err)
	srv := startE10JWKS(t, []domain.PublicJWK{jwk})
	harness, err := testutil.NewE10StudioMRTR(context.Background(), srv.Client(), srv.URL+"/.well-known/jwks.json", e10Issuer, e10Audience, clock)
	require.NoError(t, err)
	return harness
}

func validE10Request(issued token.IssuedToken, subject string, draft [32]byte, jsonrpcID string) testutil.E10ConfirmRequest {
	return testutil.E10ConfirmRequest{
		AccessToken:    issued.Compact,
		PublicClientID: e10ClientID,
		HumanSubject:   subject,
		Workspace:      "ws-curriculum-lab",
		DraftDigest:    draft,
		Tool:           e10Tool,
		RequestState:   "confirm-state-alpha",
		JSONRPCID:      jsonrpcID,
	}
}

func e10ReplacePayload(t *testing.T, compact string, mutate func(map[string]any)) string {
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

func e10Resign(t *testing.T, compact string, signer *e10Signer) string {
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

func TestE10CorrectHumanConfirmSucceedsOnce(t *testing.T) {
	clock := e10Clock{now: time.Date(2026, 8, 17, 16, 4, 5, 0, time.UTC)}
	issued, signer, subject, draft := issueE10Human(t, clock)
	harness := newE10Harness(t, clock, signer)

	audit, err := harness.Confirm(context.Background(), validE10Request(issued, subject, draft, "rpc-1"))
	require.NoError(t, err)
	assert.Equal(t, e10ClientID, audit.PublicClientID)
	assert.Equal(t, "success", audit.Outcome)
	assert.NotContains(t, audit.PublicClientID, "eyJ")
	assert.NotContains(t, audit.String(), issued.Compact)
	assert.NotContains(t, audit.String(), subject)
	assert.NotContains(t, audit.String(), "confirm-state-alpha")

	_, err = harness.Confirm(context.Background(), validE10Request(issued, subject, draft, "rpc-1"))
	require.Error(t, err)
}

func TestE10MissingWrongOverlongControlAndInternalUUIDClientIDFail(t *testing.T) {
	clock := e10Clock{now: time.Date(2026, 8, 17, 16, 4, 5, 0, time.UTC)}
	issued, signer, subject, draft := issueE10Human(t, clock)
	harness := newE10Harness(t, clock, signer)

	for _, clientID := range []string{"", "other-bff", "client\nid", "client\x00id", strings.Repeat("a", 129), uuid.NewString()} {
		req := validE10Request(issued, subject, draft, "rpc-deny-"+clientID)
		req.PublicClientID = clientID
		audit, err := harness.Confirm(context.Background(), req)
		require.Error(t, err, clientID)
		assert.Equal(t, "denied", audit.Outcome)
		if clientID != "" && !strings.ContainsAny(clientID, "\n\x00") && len(clientID) <= 128 {
			if _, parseErr := uuid.Parse(clientID); parseErr != nil {
				assert.Equal(t, clientID, audit.PublicClientID)
			} else {
				assert.Empty(t, audit.PublicClientID)
			}
		} else {
			assert.Empty(t, audit.PublicClientID)
		}
		assert.NotContains(t, audit.String(), issued.Compact)
		assert.NotContains(t, audit.String(), subject)
	}
}

func TestE10TamperedAZPAndServiceSubjectFail(t *testing.T) {
	clock := e10Clock{now: time.Date(2026, 8, 17, 16, 4, 5, 0, time.UTC)}
	issued, signer, subject, draft := issueE10Human(t, clock)
	harness := newE10Harness(t, clock, signer)

	azp := e10Resign(t, e10ReplacePayload(t, issued.Compact, func(p map[string]any) {
		p["azp"] = e10ClientID
	}), signer)
	req := validE10Request(issued, subject, draft, "rpc-azp")
	req.AccessToken = azp
	audit, err := harness.Confirm(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, "denied", audit.Outcome)
	assert.NotContains(t, audit.String(), azp)

	svc := e10Resign(t, e10ReplacePayload(t, issued.Compact, func(p map[string]any) {
		p["sub"] = "identity:svc:demo"
	}), signer)
	req = validE10Request(issued, subject, draft, "rpc-svc")
	req.AccessToken = svc
	req.HumanSubject = "identity:svc:demo"
	audit, err = harness.Confirm(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, "denied", audit.Outcome)
	assert.NotContains(t, audit.String(), "identity:svc:demo")
}

func TestE10WrongRequestStateToolWorkspaceDraftAndClientFail(t *testing.T) {
	clock := e10Clock{now: time.Date(2026, 8, 17, 16, 4, 5, 0, time.UTC)}
	issued, signer, subject, draft := issueE10Human(t, clock)
	harness := newE10Harness(t, clock, signer)
	base := validE10Request(issued, subject, draft, "rpc-ok")
	_, err := harness.Confirm(context.Background(), base)
	require.NoError(t, err)

	cases := []struct {
		name string
		mut  func(*testutil.E10ConfirmRequest)
	}{
		{"wrong requestState", func(r *testutil.E10ConfirmRequest) { r.RequestState = "other-state"; r.JSONRPCID = "rpc-state" }},
		{"wrong tool", func(r *testutil.E10ConfirmRequest) { r.Tool = "studio.publish.propose"; r.JSONRPCID = "rpc-tool" }},
		{"wrong workspace", func(r *testutil.E10ConfirmRequest) { r.Workspace = "other-ws"; r.JSONRPCID = "rpc-ws" }},
		{"wrong draft", func(r *testutil.E10ConfirmRequest) { r.DraftDigest[0] ^= 0xff; r.JSONRPCID = "rpc-draft" }},
		{"wrong client", func(r *testutil.E10ConfirmRequest) { r.PublicClientID = "studio-other"; r.JSONRPCID = "rpc-client" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validE10Request(issued, subject, draft, "unused")
			tc.mut(&req)
			audit, confirmErr := harness.Confirm(context.Background(), req)
			require.Error(t, confirmErr)
			assert.Equal(t, "denied", audit.Outcome)
			assert.NotContains(t, audit.String(), req.RequestState)
			assert.NotContains(t, audit.String(), issued.Compact)
		})
	}
}

func TestE10ReplayFailsAndNewJSONRPCIDDoesNotAlterBinding(t *testing.T) {
	clock := e10Clock{now: time.Date(2026, 8, 17, 16, 4, 5, 0, time.UTC)}
	issued, signer, subject, draft := issueE10Human(t, clock)
	harness := newE10Harness(t, clock, signer)

	first := validE10Request(issued, subject, draft, "rpc-initial")
	second := validE10Request(issued, subject, draft, "rpc-retry-same-tool")
	assert.Equal(t, harness.BindingDigest(first), harness.BindingDigest(second))
	firstHash, err := testutil.E10JSONRPCIDHash(first.JSONRPCID)
	require.NoError(t, err)
	secondHash, err := testutil.E10JSONRPCIDHash(second.JSONRPCID)
	require.NoError(t, err)
	assert.NotEqual(t, firstHash, secondHash)

	_, err = harness.Confirm(context.Background(), first)
	require.NoError(t, err)
	audit, err := harness.Confirm(context.Background(), second)
	require.Error(t, err)
	assert.Equal(t, "denied", audit.Outcome)
	assert.Equal(t, e10ClientID, audit.PublicClientID)
	assert.NotContains(t, audit.String(), second.JSONRPCID)
	assert.NotContains(t, audit.String(), second.RequestState)
}

func TestE10AuditExposesSanitizedPublicClientIDOnly(t *testing.T) {
	clock := e10Clock{now: time.Date(2026, 8, 17, 16, 4, 5, 0, time.UTC)}
	issued, signer, subject, draft := issueE10Human(t, clock)
	harness := newE10Harness(t, clock, signer)
	_, err := harness.Confirm(context.Background(), validE10Request(issued, subject, draft, "rpc-audit"))
	require.NoError(t, err)

	audits := harness.Audits()
	require.NotEmpty(t, audits)
	raw, err := json.Marshal(audits)
	require.NoError(t, err)
	body := string(raw)
	assert.Contains(t, body, e10ClientID)
	assert.NotContains(t, body, issued.Compact)
	assert.NotContains(t, body, subject)
	assert.NotContains(t, body, "confirm-state-alpha")
	assert.NotContains(t, body, "ws-curriculum-lab")
	assert.NotContains(t, strings.ToLower(body), "jwt")
	assert.NotContains(t, body, "rpc-audit")
}

func TestE10RequiresHTTPFetchOfPublicJWKS(t *testing.T) {
	clock := e10Clock{now: time.Date(2026, 8, 17, 16, 4, 5, 0, time.UTC)}
	_, err := testutil.NewE10StudioMRTR(context.Background(), http.DefaultClient, "http://127.0.0.1:1/.well-known/jwks.json", e10Issuer, e10Audience, clock)
	require.Error(t, err)
}
