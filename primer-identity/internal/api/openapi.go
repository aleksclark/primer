package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gopkg.in/yaml.v3"
)

// requiredIB1Paths is the complete IB1 route inventory that OpenAPI must emit.
var requiredIB1Paths = []string{
	"/healthz",
	"/readyz",
	"/oauth/authorize",
	"/oauth/token",
	"/broker/stytch/login",
	"/broker/stytch/email/start",
	"/broker/stytch/email/verify",
	"/broker/stytch/sso/start",
	"/broker/stytch/callback",
	"/.well-known/jwks.json",
	"/.well-known/oauth-authorization-server",
	"/oauth/revoke",
}

var requiredIdentityOperations = []string{
	"healthz",
	"readyz",
	"oauthAuthorize",
	"oauthToken",
	"brokerLogin",
	"brokerEmailStart",
	"brokerEmailVerify",
	"brokerSSOStart",
	"brokerCallback",
	"jwks",
	"oauthAuthorizationServer",
	"oauthRevoke",
}

var forbiddenIB1Paths = []string{
	"/jwks",
	"/oauth/jwks",
}

var forbiddenIB1SchemaNeedles = []string{
	"session_jwt",
	"session_token",
	"id_token",
	"provider_payload",
}

// NewOpenAPI builds the Huma API and HTTP handler with the complete IB1
// inventory registered from handler signatures. It does not construct a
// broker.Service, open a database, or contact a provider.
func NewOpenAPI() (huma.API, http.Handler) {
	s := &Server{
		registerBrokerInventory: true,
		registerMetadata:        true,
		registerTokenInventory:  true,
		issuer:                  "https://identity.example.test",
	}
	return s.build()
}

// GenerateOpenAPIYAML emits a deterministic OpenAPI 3.1 document for the IB1
// inventory without DB, provider, or network access.
func GenerateOpenAPIYAML() ([]byte, error) {
	humaAPI, _ := NewOpenAPI()
	spec, err := humaAPI.OpenAPI().YAML()
	if err != nil {
		return nil, err
	}
	if err := CheckIdentityOpenAPIPolicy(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// CheckIB1OpenAPIPolicy is the historical name for CheckIdentityOpenAPIPolicy.
func CheckIB1OpenAPIPolicy(spec []byte) error {
	return CheckIdentityOpenAPIPolicy(spec)
}

// CheckIdentityOpenAPIPolicy rejects missing IB1+IB2 inventory routes and
// planted JWKS-private/provider payload surfaces.
func CheckIdentityOpenAPIPolicy(spec []byte) error {
	var doc map[string]any
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		return fmt.Errorf("openapi policy: parse: %w", err)
	}
	paths, _ := doc["paths"].(map[string]any)
	if paths == nil {
		return fmt.Errorf("openapi policy: missing paths")
	}
	for _, p := range requiredIB1Paths {
		if _, ok := paths[p]; !ok {
			return fmt.Errorf("openapi policy: missing required path %s", p)
		}
	}
	for _, p := range forbiddenIB1Paths {
		if _, ok := paths[p]; ok {
			return fmt.Errorf("openapi policy: forbidden path %s", p)
		}
	}
	ops := collectOperationIDs(paths)
	for _, id := range requiredIdentityOperations {
		if _, ok := ops[strings.ToLower(id)]; !ok {
			return fmt.Errorf("openapi policy: missing required operation %s", id)
		}
	}
	raw := strings.ToLower(string(spec))
	for _, needle := range forbiddenIB1SchemaNeedles {
		if strings.Contains(raw, needle) {
			return fmt.Errorf("openapi policy: forbidden schema %s", needle)
		}
	}
	for _, needle := range []string{"grant_type: refresh_token"} {
		if strings.Contains(raw, needle) {
			return fmt.Errorf("openapi policy: forbidden surface %s", needle)
		}
	}
	return nil
}

func collectOperationIDs(paths map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if id, ok := n["operationId"].(string); ok && id != "" {
				out[strings.ToLower(id)] = struct{}{}
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
	walk(paths)
	return out
}
