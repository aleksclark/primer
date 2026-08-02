// Package api wires the TV server's resources into a Huma REST API. It reuses
// the generic CRUD registration from internal/api for admin resources and adds
// the device-facing endpoints (pairing, catalog, grants, heartbeats) that carry
// the access-control rules.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	"github.com/aleksclark/primer/server/internal/tv/primer"
)

// DefaultGrantTTL is the grant lifetime used when Options leaves it unset.
// Grants only need to survive long enough for a client to start playback.
const DefaultGrantTTL = 5 * time.Minute

// DefaultPairingTTL is the pairing-code lifetime used when Options leaves it
// unset.
const DefaultPairingTTL = 15 * time.Minute

// Options configures TV API construction.
type Options struct {
	// CORSOrigins is the list of allowed origins; empty disables CORS headers.
	CORSOrigins []string
	// Jellyfin is the media source client. It may be nil when the API is
	// constructed purely for OpenAPI spec generation.
	Jellyfin jellyfin.Client
	// AdminKey guards the admin API. Empty leaves the admin surface open, which
	// suits spec generation and a bare local checkout but not a deployment.
	AdminKey string
	// GrantTTL is how long an issued play grant stays redeemable.
	GrantTTL time.Duration
	// PairingTTL is how long an unclaimed pairing code stays valid.
	PairingTTL time.Duration
	// ChannelTimezone is the IANA zone the programmed grid's calendar days are
	// bucketed in. Empty selects DefaultChannelTimezone.
	ChannelTimezone string
	// Primer reports instructional time to the LMS. Nil leaves the admin
	// "report now" action reporting itself unconfigured, which is the right
	// answer for a household running the channel without an LMS.
	Primer *primer.Reporter
	// ReleaseDir holds the published APK and its version file. Empty switches
	// self-update off, which is correct for a checkout with no build to serve.
	ReleaseDir string
	// Now overrides the clock, for tests that need a fixed instant.
	Now func() time.Time
}

// Server holds the dependencies shared by the TV API handlers.
type Server struct {
	api             huma.API
	q               baserepo.Querier
	jellyfin        jellyfin.Client
	adminKey        string
	grantTTL        time.Duration
	pairingTTL      time.Duration
	channelLocation *time.Location
	reporter        *primer.Reporter
	releaseDir      string
	clock           func() time.Time
}

// now returns the server's current time.
func (s *Server) now() time.Time { return s.clock().UTC() }

// New builds the TV Huma API and its HTTP handler. The Querier may be nil when
// the API is constructed purely for OpenAPI spec generation.
func New(q baserepo.Querier, opts Options) (huma.API, http.Handler) {
	router := chi.NewMux()
	router.Use(middleware.Recoverer)
	if len(opts.CORSOrigins) > 0 {
		router.Use(baseapi.CORSMiddleware(opts.CORSOrigins))
	}

	cfg := huma.DefaultConfig("Primer TV API", "0.1.0")
	cfg.Info.Description = "Content-access backend for the Primer virtual TV channel: catalog, availability windows, watch-once ledger, play grants, and playback sessions."
	cfg.Servers = []*huma.Server{{URL: "/api/v1"}}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		deviceSecurityScheme: {
			Type:         "http",
			Scheme:       "bearer",
			Description:  "Device token issued by POST /devices/pair.",
			BearerFormat: "opaque",
		},
		adminSecurityScheme: {
			Type:        "apiKey",
			In:          "header",
			Name:        adminKeyHeader,
			Description: "Admin API key, presented by the admin SPA and by Primer.",
		},
	}

	humaAPI := humachi.New(router, cfg)
	RegisterRoutes(humaAPI, q, opts)
	return humaAPI, router
}

// RegisterRoutes wires the health check, device endpoints, and admin CRUD into
// the given Huma API. Exposed for tests that use humatest.
func RegisterRoutes(humaAPI huma.API, q baserepo.Querier, opts Options) {
	s := &Server{
		api:             humaAPI,
		q:               q,
		jellyfin:        opts.Jellyfin,
		adminKey:        opts.AdminKey,
		grantTTL:        opts.GrantTTL,
		pairingTTL:      opts.PairingTTL,
		channelLocation: ChannelLocation(opts.ChannelTimezone),
		reporter:        opts.Primer,
		releaseDir:      opts.ReleaseDir,
		clock:           opts.Now,
	}
	if s.grantTTL <= 0 {
		s.grantTTL = DefaultGrantTTL
	}
	if s.pairingTTL <= 0 {
		s.pairingTTL = DefaultPairingTTL
	}
	if s.clock == nil {
		s.clock = time.Now
	}

	baseapi.RegisterHealth(humaAPI)
	s.registerDeviceRoutes()
	s.registerAppRelease()
	s.registerAdminRoutes()
}

// ChannelLocation resolves the configured channel timezone, falling back to
// UTC when the zone database does not know it. A misconfigured zone must not
// stop the server from booting — an EPG bucketed in UTC is wrong by a few
// hours, whereas a server that refuses to start takes the channel off air.
func ChannelLocation(name string) *time.Location {
	if name == "" {
		name = DefaultChannelTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		slog.Warn("unknown channel timezone; falling back to UTC", "timezone", name, "error", err)
		return time.UTC
	}
	return loc
}
