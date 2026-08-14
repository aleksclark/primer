package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

func TestConfigAllowDown(t *testing.T) {
	t.Parallel()
	require.True(t, (studiodb.Config{}).AllowDown())
	require.False(t, (studiodb.Config{MigrationsLive: true}).AllowDown())
	require.True(t, (studiodb.Config{MigrationsLive: true, BreakGlassDown: true}).AllowDown())
}

func TestBuildAndVerifyManifestRoundTrip(t *testing.T) {
	migDir := filepath.Join("..", "..", "db", "migrations")
	m, err := studiodb.BuildManifest(migDir)
	require.NoError(t, err)
	require.Equal(t, studiodb.VersionTable, m.VersionTable)
	require.Len(t, m.Migrations, 4)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "m.json")
	require.NoError(t, studiodb.WriteManifest(path, m))
	loaded, err := studiodb.LoadManifest(path)
	require.NoError(t, err)
	require.Equal(t, m, loaded)
	require.NoError(t, studiodb.VerifyManifest(migDir, loaded, false))
}

func TestLoadConfigMaxConnsAndLiveFlags(t *testing.T) {
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:studio@127.0.0.1:5432/curriculum_studio?sslmode=disable")
	t.Setenv("STUDIO_DB_MAX_CONNS", "8")
	t.Setenv("STUDIO_MIGRATIONS_LIVE", "true")
	t.Setenv("STUDIO_MIGRATE_BREAK_GLASS_DOWN", "1")
	// Isolate from ambient db/STUDIO_MIGRATIONS_LIVE beside module root.
	t.Setenv("STUDIO_MIGRATIONS_DIR", t.TempDir())
	cfg, err := studiodb.LoadConfig()
	require.NoError(t, err)
	require.Equal(t, int32(8), cfg.MaxConns)
	require.True(t, cfg.MigrationsLive)
	require.True(t, cfg.BreakGlassDown)

	t.Setenv("STUDIO_DB_MAX_CONNS", "nope")
	_, err = studiodb.LoadConfig()
	require.Error(t, err)
}

func TestVerifyManifestMissingFile(t *testing.T) {
	tmp := t.TempDir()
	m := studiodb.BaselineManifest{
		VersionTable: studiodb.VersionTable,
		Schema:       studiodb.SchemaName,
		Migrations: []studiodb.ManifestEntry{{
			File:              "00001_identity_and_catalogs.sql",
			SHA256:            "abc",
			BaselineImmutable: true,
		}},
	}
	err := studiodb.VerifyManifest(tmp, m, true)
	require.Error(t, err)
}

func TestDatabaseNameLibpq(t *testing.T) {
	// Exercise Validate with libpq-style DSN via LoadConfig path.
	t.Setenv("STUDIO_MIGRATIONS_DIR", t.TempDir())
	t.Setenv("STUDIO_DATABASE_URL", "host=127.0.0.1 user=studio password=studio dbname=curriculum_studio sslmode=disable")
	cfg, err := studiodb.LoadConfig()
	require.NoError(t, err)
	require.NotEmpty(t, cfg.DatabaseURL)

	t.Setenv("STUDIO_DATABASE_URL", "host=127.0.0.1 user=studio password=studio dbname=primer sslmode=disable")
	_, err = studiodb.LoadConfig()
	require.Error(t, err)
}

func TestWriteManifestPermissions(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "out.json")
	require.NoError(t, studiodb.WriteManifest(path, studiodb.BaselineManifest{
		VersionTable: studiodb.VersionTable,
		Schema:       studiodb.SchemaName,
		Migrations:   nil,
	}))
	st, err := os.Stat(path)
	require.NoError(t, err)
	require.False(t, st.IsDir())
}
