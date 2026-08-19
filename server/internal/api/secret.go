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
// An empty secret leaves the generic guard inert. Product boundaries that
// require fail-closed behavior (for example, the LMS service ingest) use
// FailClosedSharedSecretGuard below rather than weakening local/spec harnesses.
func SharedSecretGuard(api huma.API, secret, headerName, message string) func(huma.Context, func(huma.Context)) {
	return sharedSecretGuard(api, secret, headerName, message, false)
}

// FailClosedSharedSecretGuard authenticates a machine boundary and rejects
// every request when the configured secret is empty. OpenAPI generation does
// not execute operation middleware, so this remains compatible with contract
// generation while ensuring a running service endpoint is never anonymous.
func FailClosedSharedSecretGuard(api huma.API, secret, headerName, message string) func(huma.Context, func(huma.Context)) {
	return sharedSecretGuard(api, secret, headerName, message, true)
}

func sharedSecretGuard(api huma.API, secret, headerName, message string, failClosed bool) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if secret == "" {
			if failClosed {
				_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, message)
				return
			}
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
