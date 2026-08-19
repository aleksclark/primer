package conformance

import (
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	studiomcp "github.com/aleksclark/primer/curriculum-studio/internal/mcp"
)

// realValidator builds the production authn.Validator against a test JWKS URL.
// Kept in a non-test .go file so that coverage.go doesn't need to import authn.
func realValidator(jwksURL string, now time.Time) (studiomcp.TokenValidator, error) {
	return authn.NewValidator(authn.Options{
		Issuer:   "https://identity.example.test",
		Audience: "curriculum-studio",
		JWKSURL:  jwksURL,
		Now:      func() time.Time { return now },
	})
}
