// Package testutil provides the TV server's integration-test harness. It
// reuses the shared container bootstrap and savepoint querier from
// internal/testutil, pointing them at the TV schema.
package testutil

import (
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	baserepo "github.com/aleksclark/primer/server/internal/repo"
	basetestutil "github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/tv/api"
	tvdb "github.com/aleksclark/primer/server/internal/tv/db"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	"github.com/aleksclark/primer/server/internal/tv/primer"
)

// harness owns the TV schema's migrated pool.
var harness = &basetestutil.Harness{Migrate: tvdb.Migrate, DBName: "primer_tv_test"}

// DB returns a migrated connection pool for the TV schema, backed by a shared
// PostgreSQL testcontainer (or TEST_DATABASE_URL if set).
func DB(t *testing.T) *pgxpool.Pool { return harness.DB(t) }

// Tx begins a transaction on the shared TV pool, rolled back on cleanup.
func Tx(t *testing.T) pgx.Tx { return harness.Tx(t) }

// Options tunes the test API.
type Options struct {
	// Jellyfin overrides the media source; defaults to an empty fake.
	Jellyfin jellyfin.Client
	// Now overrides the server clock.
	Now func() time.Time
	// GrantTTL overrides the grant lifetime.
	GrantTTL time.Duration
	// AdminKey enables admin API authentication; empty leaves it open.
	AdminKey string
	// ChannelTimezone overrides the zone the programmed grid's days are
	// bucketed in; empty selects the server default.
	ChannelTimezone string
	// Primer wires the instructional-hours reporter; nil leaves the admin
	// reporting action reporting itself unconfigured.
	Primer *primer.Reporter
	// ReleaseDir points at a published APK; empty switches self-update off.
	ReleaseDir string
}

// API returns a humatest API wired to the full TV route set, plus the
// transaction-backed querier used for factories and the media source fake.
// Statements run inside savepoints so deliberate constraint violations don't
// abort the test transaction. All state is rolled back when the test ends.
func API(t *testing.T, opts ...Options) (humatest.TestAPI, baserepo.Querier, jellyfin.Client) {
	t.Helper()
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Jellyfin == nil {
		o.Jellyfin = jellyfin.NewFake()
	}

	q := basetestutil.NewSavepointQuerier(Tx(t))
	_, testAPI := humatest.New(t)
	api.RegisterRoutes(testAPI, q, api.Options{
		Jellyfin:        o.Jellyfin,
		Now:             o.Now,
		GrantTTL:        o.GrantTTL,
		AdminKey:        o.AdminKey,
		ChannelTimezone: o.ChannelTimezone,
		Primer:          o.Primer,
		ReleaseDir:      o.ReleaseDir,
	})
	return testAPI, q, o.Jellyfin
}
