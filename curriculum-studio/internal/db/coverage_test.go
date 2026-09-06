package db_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

func TestMigrateDownAndStatus(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, studiodb.Migrate(ctx, url))

	st, err := studiodb.Studio.Status(ctx, url)
	require.NoError(t, err)
	require.NotEmpty(t, st)

	// Sole destructive path is DownWithPolicy (no exported MigrateDown bypass).
	require.NoError(t, studiodb.Studio.DownWithPolicy(ctx, url, studiodb.Config{
		DatabaseURL:    url,
		MigrationsLive: false,
	}))
	v, err := studiodb.Studio.CurrentVersion(ctx, url)
	require.NoError(t, err)
	require.Equal(t, int64(13), v)
}

func TestConnectWithConfigMaxConns(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, studiodb.Migrate(ctx, url))

	pool, err := studiodb.ConnectWithConfig(ctx, studiodb.Config{
		DatabaseURL: url,
		MaxConns:    3,
	})
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))
	stats := pool.Stat()
	require.LessOrEqual(t, stats.MaxConns(), int32(3))
}

func TestConnectEmptyURL(t *testing.T) {
	t.Parallel()
	_, err := studiodb.ConnectWithConfig(context.Background(), studiodb.Config{})
	require.Error(t, err)
}

func TestMigratorNilFS(t *testing.T) {
	t.Parallel()
	m := studiodb.NewMigrator(nil, studiodb.VersionTable)
	err := m.Up(context.Background(), "postgres://x")
	require.Error(t, err)
}

func TestHashFSMissing(t *testing.T) {
	t.Parallel()
	empty := fstest.MapFS{}
	_, err := studiodb.HashFS(empty)
	require.Error(t, err)
}

func TestHashFSOK(t *testing.T) {
	t.Parallel()
	mfs := fstest.MapFS{}
	for _, name := range studiodb.BaselineFiles {
		mfs["migrations/"+name] = &fstest.MapFile{Data: []byte("-- test\n")}
	}
	sums, err := studiodb.HashFS(fs.FS(mfs))
	require.NoError(t, err)
	require.Len(t, sums, 4)
}

func TestLoadManifestInvalidJSON(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("{not-json"), 0o644))
	_, err := studiodb.LoadManifest(path)
	require.Error(t, err)
}

func TestVerifyEmptyManifest(t *testing.T) {
	t.Parallel()
	err := studiodb.VerifyManifest(t.TempDir(), studiodb.BaselineManifest{}, false)
	require.Error(t, err)
}

func TestDatabaseNameURLMissingPath(t *testing.T) {
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:studio@127.0.0.1:5432?sslmode=disable")
	_, err := studiodb.LoadConfig()
	require.Error(t, err)
}

func TestMigratorMissingMigrationsSubdir(t *testing.T) {
	t.Parallel()
	mfs := fstest.MapFS{"readme.txt": &fstest.MapFile{Data: []byte("x")}}
	m := studiodb.NewMigrator(mfs, studiodb.VersionTable)
	err := m.Up(context.Background(), "postgres://studio:studio@127.0.0.1:1/none?sslmode=disable")
	require.Error(t, err)
}
