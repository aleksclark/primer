package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEqualSecret(t *testing.T) {
	t.Parallel()

	assert.True(t, EqualSecret("s3cret", "s3cret"))
	assert.False(t, EqualSecret("s3cret", "other"))
	assert.False(t, EqualSecret("", "s3cret"))
	assert.False(t, EqualSecret("s3cret", ""))
	assert.False(t, EqualSecret("s3cre", "s3cret"), "a prefix is not a match")
	assert.True(t, EqualSecret("", ""), "two empty secrets are equal")
}

func TestBearerToken(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "abc123", BearerToken("Bearer abc123"))
	assert.Equal(t, "abc123", BearerToken("bearer abc123"), "the scheme is case-insensitive")
	assert.Equal(t, "abc123", BearerToken("Bearer  abc123 "), "surrounding space is trimmed")
	assert.Empty(t, BearerToken(""))
	assert.Empty(t, BearerToken("Basic abc123"), "only bearer is accepted")
	assert.Empty(t, BearerToken("Bearer"))
	assert.Empty(t, BearerToken("Bearer "))
	assert.Empty(t, BearerToken("abc123"), "a bare token is not accepted")
}

// guardedAPI registers a trivial operation behind a shared-secret guard.
func guardedAPI(t *testing.T, secret string) humatest.TestAPI {
	t.Helper()
	_, testAPI := humatest.New(t)
	huma.Register(testAPI, huma.Operation{
		OperationID: "guarded",
		Method:      http.MethodGet,
		Path:        "/guarded",
		Middlewares: huma.Middlewares{SharedSecretGuard(testAPI, secret, "X-Test-Key", "credentials required")},
	}, func(_ context.Context, _ *struct{}) (*struct{}, error) { return &struct{}{}, nil })
	return testAPI
}

func TestSharedSecretGuard(t *testing.T) {
	t.Parallel()
	h := guardedAPI(t, "s3cret")

	assert.Equal(t, http.StatusUnauthorized, h.Get("/guarded").Code,
		"a configured guard refuses an unauthenticated caller")
	assert.Equal(t, http.StatusUnauthorized, h.Get("/guarded", "X-Test-Key: wrong").Code)
	assert.Equal(t, http.StatusNoContent, h.Get("/guarded", "X-Test-Key: s3cret").Code)
	assert.Equal(t, http.StatusNoContent, h.Get("/guarded", "Authorization: Bearer s3cret").Code,
		"the secret may also arrive as a bearer token")
}

func TestSharedSecretGuardIsInertWithoutASecret(t *testing.T) {
	t.Parallel()
	h := guardedAPI(t, "")

	assert.Equal(t, http.StatusNoContent, h.Get("/guarded").Code,
		"an unconfigured guard must not lock a local checkout out of its own API")
}

func TestSharedSecretGuardRejectsOverHTTP(t *testing.T) {
	t.Parallel()
	_, handler := New(nil, Options{ServiceToken: "s3cret"})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/instruction-logs/ingest", "application/json", nil)
	if assert.NoError(t, err) {
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestSpecDocumentsTheServiceTokenScheme(t *testing.T) {
	t.Parallel()
	humaAPI, _ := New(nil, Options{})

	schemes := humaAPI.OpenAPI().Components.SecuritySchemes
	require.Contains(t, schemes, serviceSecurityScheme)
	assert.Equal(t, "apiKey", schemes[serviceSecurityScheme].Type)
	assert.Equal(t, serviceTokenHeader, schemes[serviceSecurityScheme].Name)

	ingest := humaAPI.OpenAPI().Paths["/instruction-logs/ingest"].Post
	require.NotNil(t, ingest)
	assert.Equal(t, []map[string][]string{{serviceSecurityScheme: {}}}, ingest.Security,
		"the machine ingest is the only credentialled LMS endpoint")
	assert.Contains(t, ingest.Responses, "200", "the idempotent replay answer is documented")

	// The parent's own surface stays open; guarding it would lock the admin SPA
	// out of the resource it exists to display.
	list := humaAPI.OpenAPI().Paths["/instruction-logs"].Get
	require.NotNil(t, list)
	assert.Empty(t, list.Security)
}
