package db_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	studiodbschema "github.com/aleksclark/primer/curriculum-studio/db"
	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

// Schema inventory from curriculum-studio/db/SCHEMA.md table list.
var schemaTables = []string{
	"tenants", "workspaces", "workspace_memberships", "integration_identities",
	"standard_frameworks", "catalog_standards", "standard_crosswalks",
	"catalog_standard_prerequisites", "resources",
	"curricula", "plan_revisions", "objectives", "outcomes",
	"outcome_standard_mappings", "outcome_prerequisites", "learning_arcs",
	"units", "projects", "unit_outcomes", "project_outcomes",
	"evidence_requirements", "scheduling_constraints", "plan_resources",
	"validation_reports", "validation_findings",
	"learner_profiles", "materialization_runs", "workflow_stages",
	"workflow_attempts", "materialized_items", "materialized_item_edits",
	"assessment_supports", "exports", "outbox_events", "webhook_endpoints",
	"webhook_deliveries", "idempotency_keys", "audit_events",
}

func studioMigrationsDir(t *testing.T) string {
	t.Helper()
	// internal/db -> ../../db/migrations
	dir := filepath.Join("..", "..", "db", "migrations")
	abs, err := filepath.Abs(dir)
	require.NoError(t, err)
	return abs
}

func startPostgres(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("STUDIO_TEST_DATABASE_URL"); u != "" {
		// Fail closed: only accept Studio-safe external DSNs.
		require.NoError(t, studiodb.ValidateDatabaseURL(u))
		return u
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("curriculum_studio_test"),
		tcpostgres.WithUsername("studio"),
		tcpostgres.WithPassword("studio"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "start postgres testcontainer")
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return url
}

func openPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := studiodb.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// P1-E1: fresh DB migrates to full schema inventory with studio_goose_db_version.
func TestP1E1_FreshMigrateFullInventory(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()

	require.NoError(t, studiodb.Migrate(ctx, url))

	pool := openPool(t, url)

	var versionTable string
	err := pool.QueryRow(ctx, `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public' AND tablename = $1`, studiodb.VersionTable).Scan(&versionTable)
	require.NoError(t, err)
	require.Equal(t, studiodb.VersionTable, versionTable)

	var n int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM `+studiodb.VersionTable+` WHERE version_id > 0 AND is_applied`).Scan(&n)
	require.NoError(t, err)
	require.Equal(t, 12, n, "expected 12 applied goose versions")

	// No Studio domain tables in public.
	var publicStudio int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_tables
		WHERE schemaname = 'public' AND tablename = ANY($1)`, schemaTables).Scan(&publicStudio)
	require.NoError(t, err)
	require.Zero(t, publicStudio)

	rows, err := pool.Query(ctx, `
		SELECT tablename FROM pg_tables
		WHERE schemaname = $1 ORDER BY tablename`, studiodb.SchemaName)
	require.NoError(t, err)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		got = append(got, name)
	}
	require.NoError(t, rows.Err())
	want := append([]string(nil), schemaTables...)
	sort.Strings(want)
	require.Equal(t, want, got, "SCHEMA.md inventory must match curriculum_studio tables")
}

// P1-E2: second Up is a no-op.
func TestP1E2_SecondUpIdempotent(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, studiodb.Migrate(ctx, url))
	v1, err := studiodb.Studio.CurrentVersion(ctx, url)
	require.NoError(t, err)
	require.NoError(t, studiodb.Migrate(ctx, url))
	v2, err := studiodb.Studio.CurrentVersion(ctx, url)
	require.NoError(t, err)
	require.Equal(t, v1, v2)
	require.Equal(t, int64(12), v2)
}

// P1-E3: freeze inventory matches committed manifest and embedded FS.
func TestP1E3_FreezeInventoryMatches(t *testing.T) {
	migDir := studioMigrationsDir(t)
	built, err := studiodb.BuildManifest(migDir)
	require.NoError(t, err)

	manifestPath := filepath.Join(migDir, "..", studiodb.BaselineManifestName)
	committed, err := studiodb.LoadManifest(manifestPath)
	require.NoError(t, err, "baseline_manifest.json must be checked in")
	require.Equal(t, built, committed)

	require.NoError(t, studiodb.VerifyManifest(migDir, committed, false))

	// Embedded FS must match disk (single source of truth).
	sums, err := studiodb.HashFS(studiodbschema.MigrationsFS)
	require.NoError(t, err)
	for _, e := range committed.Migrations {
		require.Equal(t, e.SHA256, sums[e.File], e.File)
	}
}

// P1-E4: mutating a baseline file fails freeze checker (esp. with live marker).
func TestP1E4_FreezeRejectsMutatedBaseline(t *testing.T) {
	migDir := studioMigrationsDir(t)
	manifest, err := studiodb.LoadManifest(filepath.Join(migDir, "..", studiodb.BaselineManifestName))
	require.NoError(t, err)

	tmp := t.TempDir()
	// Copy all baseline files then mutate 00002.
	for _, name := range studiodb.BaselineFiles {
		b, err := os.ReadFile(filepath.Join(migDir, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), b, 0o644))
	}
	mut := filepath.Join(tmp, "00002_plan_domain.sql")
	b, err := os.ReadFile(mut)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(mut, append(b, []byte("\n-- drift\n")...), 0o644))

	err = studiodb.VerifyManifest(tmp, manifest, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "00002_plan_domain.sql")
}

// P1-E5: non-live down succeeds one step; live down refused without break-glass.
func TestP1E5_DownPolicyLiveVsNonLive(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, studiodb.Migrate(ctx, url))

	nonLive := studiodb.Config{DatabaseURL: url, MigrationsLive: false}
	require.NoError(t, studiodb.Studio.DownWithPolicy(ctx, url, nonLive))
	v, err := studiodb.Studio.CurrentVersion(ctx, url)
	require.NoError(t, err)
	require.Equal(t, int64(11), v, "down should roll back exactly one version (00012)")

	// Re-up then refuse live down.
	require.NoError(t, studiodb.Migrate(ctx, url))
	live := studiodb.Config{DatabaseURL: url, MigrationsLive: true, BreakGlassDown: false}
	err = studiodb.Studio.DownWithPolicy(ctx, url, live)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "live")
	v, err = studiodb.Studio.CurrentVersion(ctx, url)
	require.NoError(t, err)
	require.Equal(t, int64(12), v)

	// Break-glass allows down.
	live.BreakGlassDown = true
	require.NoError(t, studiodb.Studio.DownWithPolicy(ctx, url, live))
	v, err = studiodb.Studio.CurrentVersion(ctx, url)
	require.NoError(t, err)
	require.Equal(t, int64(11), v)
}

// P1-E6: config refuses missing STUDIO_DATABASE_URL and LMS DB names; no DATABASE_URL fallback.
func TestP1E6_ConfigDSNIsolation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://primer:primer@127.0.0.1:5432/primer?sslmode=disable")
	t.Setenv("STUDIO_DATABASE_URL", "")
	_, err := studiodb.LoadConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "STUDIO_DATABASE_URL")

	t.Setenv("STUDIO_DATABASE_URL", "postgres://primer:primer@127.0.0.1:5432/primer?sslmode=disable")
	_, err = studiodb.LoadConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "forbidden")

	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:studio@127.0.0.1:5432/curriculum_studio?sslmode=disable")
	cfg, err := studiodb.LoadConfig()
	require.NoError(t, err)
	require.Contains(t, cfg.DatabaseURL, "curriculum_studio")
}

func TestConnectInvalidURL(t *testing.T) {
	t.Parallel()
	_, err := studiodb.Connect(context.Background(), "not-a-url://%%%")
	require.Error(t, err)
}

func TestIsLiveEnvMarker(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, studiodb.LiveMarkerFilename)
	require.False(t, studiodb.IsLiveEnv(false, marker))
	require.NoError(t, os.WriteFile(marker, []byte("1\n"), 0o644))
	require.True(t, studiodb.IsLiveEnv(false, marker))
	require.True(t, studiodb.IsLiveEnv(true, ""))
}
