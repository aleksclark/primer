package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/oauth"
)

func TestNewOpenAPIRegistersTokenInventoryWithoutServingNilOAuth(t *testing.T) {
	t.Parallel()

	humaAPI, handler := api.NewOpenAPI()
	require.NotNil(t, humaAPI)
	require.NotNil(t, handler)

	spec, err := humaAPI.OpenAPI().YAML()
	require.NoError(t, err)
	require.NoError(t, api.CheckIdentityOpenAPIPolicy(spec))

	doc := parseOpenAPI(t, spec)
	paths := openAPIPaths(t, doc)
	assert.Contains(t, paths, "/oauth/token")
	assert.Contains(t, pathMethods(t, paths["/oauth/token"]), http.MethodPost)
	assert.Contains(t, paths, "/oauth/revoke")
	assert.Contains(t, pathMethods(t, paths["/oauth/revoke"]), http.MethodPost)
	assertTokenOpenAPIContract(t, doc)
	assertRevokeOpenAPIContract(t, doc)

	form := url.Values{
		"grant_type":    {oauth.GrantAuthorizationCode},
		"code":          {"schema-only-must-not-exchange"},
		"redirect_uri":  {"https://example.test/callback"},
		"resource":      {"https://example.test/resource"},
		"code_verifier": {strings.Repeat("a", 43)},
		"client_id":     {"schema-only-client"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	require.NotPanics(t, func() { handler.ServeHTTP(rr, req) })
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), oauth.ErrorTemporarilyUnavail)
	assert.NotContains(t, rr.Body.String(), "schema-only-must-not-exchange")
	assert.NotContains(t, rr.Body.String(), "access_token")
}

func TestGenerateOpenAPIYAMLIncludesTokenAndStaysPolicyClean(t *testing.T) {
	t.Parallel()

	spec, err := api.GenerateOpenAPIYAML()
	require.NoError(t, err)
	require.NoError(t, api.CheckIdentityOpenAPIPolicy(spec))
	assertTokenOpenAPIContract(t, parseOpenAPI(t, spec))
	assertRevokeOpenAPIContract(t, parseOpenAPI(t, spec))
}

func TestCheckIdentityOpenAPIPolicyRejectsPlantedRefreshGrantAndProviderSchema(t *testing.T) {
	t.Parallel()

	planted := plantedIB1Spec(t)
	planted["paths"].(map[string]any)["/oauth/token"].(map[string]any)["post"].(map[string]any)["description"] = "grant_type: refresh_token"
	raw, err := yaml.Marshal(planted)
	require.NoError(t, err)
	err = api.CheckIdentityOpenAPIPolicy(raw)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "refresh_token")

	planted = plantedIB1Spec(t)
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
	schemas["ProviderPayload"] = map[string]any{
		"properties": map[string]any{
			"session_jwt": map[string]any{"type": "string"},
		},
	}
	raw, err = yaml.Marshal(planted)
	require.NoError(t, err)
	err = api.CheckIdentityOpenAPIPolicy(raw)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "session_jwt")
}

func assertTokenOpenAPIContract(t *testing.T, doc map[string]any) {
	t.Helper()
	paths := openAPIPaths(t, doc)
	tokenPath, ok := paths["/oauth/token"].(map[string]any)
	require.True(t, ok, "token path")
	post, ok := lookupMethod(tokenPath, "post")
	require.True(t, ok, "token POST")
	assert.Contains(t, strings.ToLower(asString(post["operationId"])), "oauthtoken")
	security, _ := post["security"].([]any)
	require.NotEmpty(t, security)
	foundBasic := false
	foundOptional := false
	for _, item := range security {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := node["oauthTokenBasic"]; ok {
			foundBasic = true
		}
		if len(node) == 0 {
			foundOptional = true
		}
	}
	assert.True(t, foundBasic, "oauthTokenBasic security")
	assert.True(t, foundOptional, "optional empty security")

	responses, _ := post["responses"].(map[string]any)
	require.NotNil(t, responses)
	for _, status := range []string{"200", "400", "401", "413", "415", "503"} {
		assert.Contains(t, responses, status, status)
	}
	headers := collectResponseHeaders(responses)
	for _, name := range []string{"cache-control", "pragma", "x-content-type-options", "www-authenticate"} {
		assert.Contains(t, headers, name)
	}

	requestBody, ok := post["requestBody"].(map[string]any)
	require.True(t, ok, "token requestBody")
	content, ok := requestBody["content"].(map[string]any)
	require.True(t, ok, "token requestBody.content")
	assert.Contains(t, content, "application/x-www-form-urlencoded")
	assert.NotContains(t, content, "application/octet-stream")
	assert.Len(t, content, 1)

	form := collectNamedSchemas(doc, post)
	for _, field := range []string{
		"grant_type", "code", "redirect_uri", "resource", "code_verifier",
		"client_id", "client_assertion_type", "client_assertion", "client_secret",
		"refresh_token", "scope",
	} {
		assert.Contains(t, form, field)
	}
	for _, secret := range []string{"code", "code_verifier", "client_secret", "client_assertion"} {
		assert.True(t, isWriteOnly(form[secret]), "%s must be writeOnly", secret)
	}

	raw := strings.ToLower(flatten(doc))
	assert.Contains(t, raw, "access_token")
	assert.Contains(t, raw, "refresh_token")
	assert.NotContains(t, raw, "session_jwt")
	assert.NotContains(t, raw, "session_token")
	assert.NotContains(t, raw, "id_token")
	assert.NotContains(t, raw, "provider_payload")
	assert.Contains(t, paths, "/oauth/revoke")
}

func assertRevokeOpenAPIContract(t *testing.T, doc map[string]any) {
	t.Helper()
	paths := openAPIPaths(t, doc)
	revokePath, ok := paths["/oauth/revoke"].(map[string]any)
	require.True(t, ok, "revoke path")
	post, ok := lookupMethod(revokePath, "post")
	require.True(t, ok, "revoke POST")
	assert.Contains(t, strings.ToLower(asString(post["operationId"])), "oauthrevoke")
	security, _ := post["security"].([]any)
	require.NotEmpty(t, security)
	foundBasic := false
	foundOptional := false
	for _, item := range security {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := node["oauthTokenBasic"]; ok {
			foundBasic = true
		}
		if len(node) == 0 {
			foundOptional = true
		}
	}
	assert.True(t, foundBasic, "oauthTokenBasic security")
	assert.True(t, foundOptional, "optional empty security")

	responses, _ := post["responses"].(map[string]any)
	require.NotNil(t, responses)
	for _, status := range []string{"200", "400", "401", "413", "415", "503"} {
		assert.Contains(t, responses, status, status)
	}
	headers := collectResponseHeaders(responses)
	for _, name := range []string{"cache-control", "pragma", "x-content-type-options", "www-authenticate"} {
		assert.Contains(t, headers, name)
	}

	requestBody, ok := post["requestBody"].(map[string]any)
	require.True(t, ok, "revoke requestBody")
	content, ok := requestBody["content"].(map[string]any)
	require.True(t, ok, "revoke requestBody.content")
	assert.Contains(t, content, "application/x-www-form-urlencoded")
	assert.NotContains(t, content, "application/octet-stream")
	assert.Len(t, content, 1)

	form := collectNamedSchemas(doc, post)
	for _, field := range []string{
		"token", "token_type_hint", "client_id", "client_assertion_type",
		"client_assertion", "client_secret",
	} {
		assert.Contains(t, form, field)
	}
	for _, secret := range []string{"token", "client_secret", "client_assertion"} {
		assert.True(t, isWriteOnly(form[secret]), "%s must be writeOnly", secret)
	}
}

func lookupMethod(path map[string]any, method string) (map[string]any, bool) {
	for key, value := range path {
		if strings.EqualFold(key, method) {
			node, ok := value.(map[string]any)
			return node, ok
		}
	}
	return nil, false
}

func collectResponseHeaders(responses map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if headers, ok := n["headers"].(map[string]any); ok {
				for name := range headers {
					out[strings.ToLower(name)] = struct{}{}
				}
			}
			for _, child := range n {
				walk(child)
			}
		case []any:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(responses)
	return out
}

func collectNamedSchemas(doc map[string]any, post map[string]any) map[string]any {
	out := map[string]any{}
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if props, ok := n["properties"].(map[string]any); ok {
				for name, prop := range props {
					out[name] = prop
				}
			}
			for _, child := range n {
				walk(child)
			}
		case []any:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(post)
	walk(doc["components"])
	return out
}

func isWriteOnly(raw any) bool {
	node, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	if node["writeOnly"] == true {
		return true
	}
	if schema, ok := node["schema"].(map[string]any); ok && schema["writeOnly"] == true {
		return true
	}
	return false
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func flatten(v any) string {
	var b strings.Builder
	var walk func(any)
	walk = func(n any) {
		switch x := n.(type) {
		case map[string]any:
			for k, child := range x {
				b.WriteString(k)
				b.WriteByte(' ')
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		case string:
			b.WriteString(x)
			b.WriteByte(' ')
		}
	}
	walk(v)
	return b.String()
}
