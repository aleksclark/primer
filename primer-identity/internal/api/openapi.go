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
	"/broker/stytch/login",
	"/broker/stytch/email/start",
	"/broker/stytch/email/verify",
	"/broker/stytch/sso/start",
	"/broker/stytch/callback",
	"/.well-known/jwks.json",
	"/.well-known/oauth-authorization-server",
}

var forbiddenIB1Paths = []string{
	"/oauth/token",
	"/jwks",
	"/oauth/jwks",
}

var forbiddenIB1SchemaNeedles = []string{
	"access_token",
	"refresh_token",
	"session_jwt",
	"session_token",
	"id_token",
	"provider_payload",
}

// NewOpenAPI builds the Huma API and HTTP handler with the complete IB1
// inventory registered from handler signatures. It does not construct a
// broker.Service, open a database, or contact a provider.
func NewOpenAPI() (huma.API, http.Handler) {
	s := &Server{registerBrokerInventory: true, registerMetadata: true, issuer: "https://identity.example.test"}
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
	if err := CheckIB1OpenAPIPolicy(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// CheckIB1OpenAPIPolicy rejects missing IB1 routes and planted token/JWKS/
// provider payload surfaces. Ordinary generation must pass this check.
func CheckIB1OpenAPIPolicy(spec []byte) error {
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
	raw := strings.ToLower(string(spec))
	for _, needle := range forbiddenIB1SchemaNeedles {
		if strings.Contains(raw, needle) {
			return fmt.Errorf("openapi policy: forbidden schema %s", needle)
		}
	}
	return nil
}
