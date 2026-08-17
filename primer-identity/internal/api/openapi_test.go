package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/aleksclark/primer/identity/internal/api"
)

func TestNewOpenAPIRegistersIB1InventoryWithoutBrokerOrDatabase(t *testing.T) {
	t.Parallel()

	humaAPI, handler := api.NewOpenAPI()
	require.NotNil(t, humaAPI)
	require.NotNil(t, handler)

	spec, err := humaAPI.OpenAPI().YAML()
	require.NoError(t, err)
	require.NoError(t, api.CheckIB1OpenAPIPolicy(spec))

	doc := parseOpenAPI(t, spec)
	paths := openAPIPaths(t, doc)
	assert.Contains(t, paths, "/healthz")
	assert.Contains(t, paths, "/readyz")
	assert.Contains(t, paths, "/oauth/authorize")
	assert.Contains(t, paths, "/broker/stytch/login")
	assert.Contains(t, paths, "/broker/stytch/email/start")
	assert.Contains(t, paths, "/broker/stytch/email/verify")
	assert.Contains(t, paths, "/broker/stytch/sso/start")
	assert.Contains(t, paths, "/broker/stytch/callback")
	assert.NotContains(t, paths, "/oauth/token")

	assert.Contains(t, pathMethods(t, paths["/healthz"]), http.MethodGet)
	assert.Contains(t, pathMethods(t, paths["/readyz"]), http.MethodGet)
	assert.Contains(t, pathMethods(t, paths["/oauth/authorize"]), http.MethodGet)
	assert.Contains(t, pathMethods(t, paths["/broker/stytch/email/start"]), http.MethodPost)
	assert.Contains(t, pathMethods(t, paths["/broker/stytch/callback"]), http.MethodGet)
}

func TestNewOpenAPIHandlersFailClosedWithoutBroker(t *testing.T) {
	t.Parallel()

	_, handler := api.NewOpenAPI()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.GreaterOrEqual(t, rr.Code, 400)
	assert.Less(t, rr.Code, 500)
}

func TestNewWithoutBrokerDoesNotRegisterIB1Routes(t *testing.T) {
	t.Parallel()

	humaAPI, _ := api.New(nil, api.Options{})
	spec, err := humaAPI.OpenAPI().YAML()
	require.NoError(t, err)
	doc := parseOpenAPI(t, spec)
	paths := openAPIPaths(t, doc)
	assert.Contains(t, paths, "/healthz")
	assert.Contains(t, paths, "/readyz")
	assert.NotContains(t, paths, "/oauth/authorize")
	assert.NotContains(t, paths, "/broker/stytch/callback")
}

func TestGenerateOpenAPIYAMLIsDeterministicAndPolicyClean(t *testing.T) {
	t.Parallel()

	first, err := api.GenerateOpenAPIYAML()
	require.NoError(t, err)
	second, err := api.GenerateOpenAPIYAML()
	require.NoError(t, err)
	assert.Equal(t, first, second)
	require.NoError(t, api.CheckIB1OpenAPIPolicy(first))
	assert.True(t, strings.HasPrefix(string(first), "components:") || strings.Contains(string(first), "openapi:"))
	var spec map[string]any
	require.NoError(t, yaml.Unmarshal(first, &spec))
	assertCallbackArtifactsWriteOnly(t, spec)
}

func TestCheckIB1OpenAPIPolicyRejectsPlantedForbiddenRoute(t *testing.T) {
	t.Parallel()

	planted := plantedIB1Spec(t)
	planted["paths"].(map[string]any)["/oauth/token"] = map[string]any{
		"post": map[string]any{"summary": "forbidden"},
	}
	raw, err := yaml.Marshal(planted)
	require.NoError(t, err)
	err = api.CheckIB1OpenAPIPolicy(raw)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "/oauth/token")
}

func TestCheckIB1OpenAPIPolicyRejectsPlantedForbiddenSchema(t *testing.T) {
	t.Parallel()

	planted := plantedIB1Spec(t)
	components, _ := planted["components"].(map[string]any)
	if components == nil {
		components = map[string]any{}
		planted["components"] = components
	}
	schemas, _ := components["schemas"].(map[string]any)
	if schemas == nil {
		schemas = map[string]any{}
		components["schemas"] = schemas
	}
	schemas["AccessToken"] = map[string]any{
		"properties": map[string]any{
			"access_token": map[string]any{"type": "string"},
		},
	}
	raw, err := yaml.Marshal(planted)
	require.NoError(t, err)
	err = api.CheckIB1OpenAPIPolicy(raw)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "access_token")
}

func plantedIB1Spec(t *testing.T) map[string]any {
	t.Helper()
	spec, err := api.GenerateOpenAPIYAML()
	require.NoError(t, err)
	return parseOpenAPI(t, spec)
}

func TestNewOpenAPIMatchesLiveBrokerInventory(t *testing.T) {
	svc := newBrokerService(t, scripted(t, successFixture(uniqueLabel("oa"), "email_magic_link")))
	_, liveAPI := newBrokerAPI(t, svc)
	liveYAML, err := liveAPI.OpenAPI().YAML()
	require.NoError(t, err)

	schemaAPI, _ := api.NewOpenAPI()
	schemaYAML, err := schemaAPI.OpenAPI().YAML()
	require.NoError(t, err)
	assert.Equal(t, string(liveYAML), string(schemaYAML))
}

func parseOpenAPI(t *testing.T, spec []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(spec, &doc))
	return doc
}

func openAPIPaths(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok, "spec paths")
	return paths
}

func pathMethods(t *testing.T, raw any) map[string]struct{} {
	t.Helper()
	node, ok := raw.(map[string]any)
	require.True(t, ok)
	out := make(map[string]struct{})
	for method := range node {
		switch strings.ToLower(method) {
		case "get", "post", "put", "patch", "delete", "head", "options":
			out[strings.ToUpper(method)] = struct{}{}
		}
	}
	return out
}
