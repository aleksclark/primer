package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/domain"
)

type staticJWKS struct {
	pubs []domain.PublicJWK
	etag string
	err  error
}

func (s staticJWKS) PublicJWKS(context.Context) ([]domain.PublicJWK, error) {
	return s.pubs, s.err
}
func (s staticJWKS) PublicSetETag(context.Context) (string, error) { return s.etag, s.err }

func samplePublicJWK(kid string) domain.PublicJWK {
	return domain.PublicJWK{
		KTY: "EC", CRV: "P-256", Use: "sig", Alg: "ES256", Kid: kid,
		X: "f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU",
		Y: "x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0",
	}
}

func TestWellKnownRoutesRegisterOnceAndServeExactWire(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		_, _ = api.New(nil, api.Options{
			Issuer: "https://id.example.test/issuer/path",
			JWKS:   staticJWKS{pubs: []domain.PublicJWK{samplePublicJWK("kid-b"), samplePublicJWK("kid-a")}, etag: `W/"abc"`},
		})
	})

	humaAPI, handler := api.New(nil, api.Options{
		Issuer: "https://id.example.test/issuer/path",
		JWKS:   staticJWKS{pubs: []domain.PublicJWK{samplePublicJWK("kid-b"), samplePublicJWK("kid-a")}, etag: `W/"abc"`},
	})
	spec, err := humaAPI.OpenAPI().YAML()
	require.NoError(t, err)
	paths := openAPIPaths(t, parseOpenAPI(t, spec))
	assert.Contains(t, paths, "/.well-known/jwks.json")
	assert.Contains(t, paths, "/.well-known/oauth-authorization-server/issuer/path")
	assert.NotContains(t, paths, "/oauth/token")

	jwks := httptest.NewRecorder()
	handler.ServeHTTP(jwks, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	require.Equal(t, http.StatusOK, jwks.Code)
	assert.Equal(t, "application/jwk-set+json", jwks.Header().Get("Content-Type"))
	assert.Contains(t, jwks.Header().Get("Cache-Control"), "public")
	assert.Equal(t, `W/"abc"`, jwks.Header().Get("ETag"))
	assert.LessOrEqual(t, jwks.Body.Len(), 64*1024)
	assert.NotContains(t, jwks.Body.String(), `"d"`)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(jwks.Body.Bytes(), &doc))
	keys, _ := doc["keys"].([]any)
	require.Len(t, keys, 2)
	assert.Equal(t, "kid-a", keys[0].(map[string]any)["kid"])
	assert.Equal(t, "kid-b", keys[1].(map[string]any)["kid"])

	cached := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	req.Header.Set("If-None-Match", `W/"abc"`)
	handler.ServeHTTP(cached, req)
	assert.Equal(t, http.StatusNotModified, cached.Code)
	body, err := io.ReadAll(cached.Result().Body)
	require.NoError(t, err)
	assert.Empty(t, body)

	meta := httptest.NewRecorder()
	handler.ServeHTTP(meta, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server/issuer/path", nil))
	require.Equal(t, http.StatusOK, meta.Code)
	assert.Contains(t, meta.Header().Get("Content-Type"), "application/json")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(meta.Body.Bytes(), &parsed))
	assert.Equal(t, "https://id.example.test/issuer/path", parsed["issuer"])
	assert.Equal(t, "https://id.example.test/issuer/path/oauth/authorize", parsed["authorization_endpoint"])
	assert.Equal(t, "https://id.example.test/issuer/path/oauth/token", parsed["token_endpoint"])
	assert.NotContains(t, parsed, "revocation_endpoint")
	assert.NotContains(t, meta.Body.String(), "/oauth/revoke")
	assert.Equal(t, "https://id.example.test/issuer/path/.well-known/jwks.json", parsed["jwks_uri"])
	assert.Equal(t, []any{"authorization_code"}, parsed["grant_types_supported"])
	assert.NotContains(t, parsed, "registration_endpoint")
	assert.NotContains(t, parsed, "introspection_endpoint")

	root := httptest.NewRecorder()
	handler.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	assert.Equal(t, http.StatusNotFound, root.Code)
}

func TestLiveBrokerWithoutJWKSOmitsRuntimeWellKnown(t *testing.T) {
	svc := newBrokerService(t, scripted(t, successFixture(uniqueLabel("meta"), "email_magic_link")))
	handler, humaAPI := newBrokerAPI(t, svc)
	spec, err := humaAPI.OpenAPI().YAML()
	require.NoError(t, err)
	paths := openAPIPaths(t, parseOpenAPI(t, spec))
	assert.Contains(t, paths, "/.well-known/jwks.json")
	assert.Contains(t, paths, "/.well-known/oauth-authorization-server")

	rr := httptest.NewRecorder()
	require.NotPanics(t, func() {
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	})
	assert.Equal(t, http.StatusNotFound, rr.Code)
}
