package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// EqualSecret compares a presented shared secret against the expected one in
// constant time, so a caller cannot recover the secret by timing rejections.
// Hashing both sides first keeps the comparison independent of their lengths.
func EqualSecret(presented, expected string) bool {
	a := sha256.Sum256([]byte(presented))
	b := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// BearerToken extracts the credential from an "Authorization: Bearer …"
// header, returning "" when the header is absent or shaped differently.
func BearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// SharedSecretGuard returns operation middleware that authenticates a
// machine-to-machine caller holding a pre-shared secret, presented either in
// headerName or as a bearer token.
//
// An empty secret leaves the guard inert. That keeps OpenAPI generation and a
// bare local checkout working without ceremony; every deployment sets the
// secret and the binaries warn at startup when one is missing.
func SharedSecretGuard(api huma.API, secret, headerName, message string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if secret == "" {
			next(ctx)
			return
		}
		presented := ctx.Header(headerName)
		if presented == "" {
			presented = BearerToken(ctx.Header("Authorization"))
		}
		if !EqualSecret(presented, secret) {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, message)
			return
		}
		next(ctx)
	}
}
