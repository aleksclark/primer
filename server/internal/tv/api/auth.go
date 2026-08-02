package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	baseapi "github.com/aleksclark/primer/server/internal/api"
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

// adminKeyHeader carries the admin API key. The admin surface hands out device
// pairing codes, so it issues credentials and must not be left open. Primer's
// overseer and tutor agents present the same key when they read the grid or
// place curriculum-driven availability windows.
const adminKeyHeader = "X-Admin-Key"

// requireAdmin returns the operation middleware guarding the admin API. With no
// key configured the guard is inert, which keeps spec generation and a bare
// local checkout working; deployments set TV_ADMIN_API_KEY.
func (s *Server) requireAdmin() func(huma.Context, func(huma.Context)) {
	return baseapi.SharedSecretGuard(s.api, s.adminKey, adminKeyHeader, "admin credentials required")
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
