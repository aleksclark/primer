package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/identityauth"
	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/auth"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// deviceContextKey keys the authenticated device on the request context.
type deviceContextKey struct{}

// DeviceFromContext returns the device authenticated for this request.
func DeviceFromContext(ctx context.Context) (*domain.Device, bool) {
	device, ok := ctx.Value(deviceContextKey{}).(*domain.Device)
	return device, ok
}

// deviceSecurityScheme names the bearer scheme documented on device
// operations in the OpenAPI spec.
const deviceSecurityScheme = "deviceToken"

// adminSecurityScheme names the API-key scheme documented on admin operations.
const adminSecurityScheme = "adminKey"

// adminJWTSecurityScheme names the JWT bearer scheme for human admin auth.
const adminJWTSecurityScheme = "adminJWT"

// adminKeyHeader carries the admin API key. The admin surface hands out device
// pairing codes, so it issues credentials and must not be left open. Primer's
// overseer and tutor agents present the same key when they read the grid or
// place curriculum-driven availability windows.
const adminKeyHeader = "X-Admin-Key"

// requireAdmin returns the operation middleware guarding the admin API.
//
// Authentication paths (checked in order):
//  1. Bearer token that is a JWT (3-part) → verified as a Primer Identity JWT.
//  2. X-Admin-Key header or Bearer opaque → checked as the shared service secret.
//
// Fail-closed semantics:
//   - When an identity verifier IS configured, JWT-shaped tokens MUST validate.
//   - When the admin key IS configured, non-JWT tokens MUST match.
//   - When BOTH are unconfigured, the guard is inert (spec generation, local dev).
//     Production deploys MUST configure at least one; tv-server/main.go logs a
//     warning when neither is set.
//
// Raw Stytch JWTs are rejected because the identityauth verifier requires
// typ=at+jwt (Stytch uses typ=JWT) and a Primer-issued iss/aud pair.
func (s *Server) requireAdmin() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		// Inert when nothing is configured (spec gen, local dev, test harness).
		if s.identityVerifier == nil && s.adminKey == "" {
			next(ctx)
			return
		}

		// Try JWT path first (Bearer token that looks like a JWT).
		bearer := baseapi.BearerToken(ctx.Header("Authorization"))
		if bearer != "" && identityauth.IsJWT(bearer) {
			if s.identityVerifier == nil {
				_ = huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "identity verification not configured")
				return
			}
			_, err := s.identityVerifier.Verify(ctx.Context(), bearer)
			if err != nil {
				_ = huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "invalid identity token")
				return
			}
			// JWT verified — no local role check; any valid Primer Identity
			// principal with the correct audience is an admin. Product
			// authorization (family-level) is enforced by the BFF/SPA, not
			// re-derived from Stytch organization roles.
			next(ctx)
			return
		}

		// Try shared-secret path (X-Admin-Key header or opaque Bearer).
		if s.adminKey == "" {
			_ = huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "admin credentials required")
			return
		}
		presented := ctx.Header(adminKeyHeader)
		if presented == "" {
			presented = bearer // non-JWT opaque bearer
		}
		if !baseapi.EqualSecret(presented, s.adminKey) {
			_ = huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "admin credentials required")
			return
		}
		next(ctx)
	}
}

// requireDevice returns the operation middleware that authenticates a device
// token and attaches the device to the request context.
func (s *Server) requireDevice() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		token := baseapi.BearerToken(ctx.Header("Authorization"))
		if token == "" {
			_ = huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "missing device token")
			return
		}
		device, err := tvrepo.DeviceByTokenHash(ctx.Context(), s.q, auth.HashToken(token))
		if err != nil {
			if errors.Is(err, baserepo.ErrNotFound) {
				_ = huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "unknown device token")
				return
			}
			_ = huma.WriteErr(s.api, ctx, http.StatusInternalServerError, "authenticate device")
			return
		}
		if err := tvrepo.TouchDevice(ctx.Context(), s.q, device.ID, s.now()); err != nil {
			_ = huma.WriteErr(s.api, ctx, http.StatusInternalServerError, "touch device")
			return
		}
		next(huma.WithValue(ctx, deviceContextKey{}, device))
	}
}

// device pulls the authenticated device out of a handler context. A missing
// device means the operation was registered without requireDevice.
func device(ctx context.Context) (*domain.Device, error) {
	d, ok := DeviceFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("device not authenticated")
	}
	return d, nil
}
