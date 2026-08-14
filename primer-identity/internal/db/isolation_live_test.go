package db_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	identitydb "github.com/aleksclark/primer/identity/internal/db"
)

// startNamedPostgres starts a disposable Postgres with the given database name.
func startNamedPostgres(t *testing.T, dbName string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername("identity"),
		tcpostgres.WithPassword("identity"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "start postgres testcontainer name=%s", dbName)
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return url
}

func countIdentityGooseArtifacts(t *testing.T, dsn string) (tables int, schemas int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	// Open with pgxpool directly (bypass Identity Connect isolation) so we can
	// inspect foreign DBs after a refused migrate attempt.
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_tables
		WHERE tablename = $1 OR tablename LIKE 'identity_goose%'
	`, identitydb.VersionTable).Scan(&tables)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.schemata
		WHERE schema_name ILIKE '%identity%' AND schema_name NOT IN ('information_schema')
	`).Scan(&schemas)
	require.NoError(t, err)
	return tables, schemas
}

// Live adversarial: direct library Migrate/Connect against foreign product DBs
// must leave zero identity_goose bookkeeping and must not create identity schema.
func TestLiveLibraryRefusesForeignDBsWithoutPollution(t *testing.T) {
	if testing.Short() {
		t.Skip("live foreign-DB isolation")
	}
	foreignNames := []string{
		"curriculum_studio_test",
		"primer_tv_test",
		"tv_test",
		"primer_test",
		"curriculum_studio",
	}
	ctx := context.Background()
	for _, name := range foreignNames {
		t.Run(name, func(t *testing.T) {
			dsn := startNamedPostgres(t, name)

			err := identitydb.Migrate(ctx, dsn)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "forbidden")

			err = identitydb.MigrateDown(ctx, dsn)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "forbidden")

			_, err = identitydb.Connect(ctx, dsn)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "forbidden")

			// Keyword/libpq form of the same foreign DB must also refuse.
			libpq := fmt.Sprintf("host=127.0.0.1 user=identity password=identity dbname=%s sslmode=disable", name)
			// Rebuild libpq from the live DSN host/port so we hit the same container.
			pcfg, err := pgxpool.ParseConfig(dsn)
			require.NoError(t, err)
			libpq = fmt.Sprintf(
				"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
				pcfg.ConnConfig.Host, pcfg.ConnConfig.Port,
				pcfg.ConnConfig.User, pcfg.ConnConfig.Password, name,
			)
			err = identitydb.Migrate(ctx, libpq)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "forbidden")

			// Double-slash URI form.
			uriDouble := fmt.Sprintf(
				"postgres://%s:%s@%s:%d//%s?sslmode=disable",
				pcfg.ConnConfig.User, pcfg.ConnConfig.Password,
				pcfg.ConnConfig.Host, pcfg.ConnConfig.Port, name,
			)
			err = identitydb.Migrate(ctx, uriDouble)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "forbidden")

			tables, schemas := countIdentityGooseArtifacts(t, dsn)
			require.Zero(t, tables, "foreign DB %s must have zero identity_goose tables after refused migrate", name)
			require.Zero(t, schemas, "foreign DB %s must have zero identity schemas after refused migrate", name)
		})
	}
}

// Live positive path: correct Identity DB migrates and exposes identity_goose_db_version.
func TestLiveCorrectIdentityDBMigrates(t *testing.T) {
	if testing.Short() {
		t.Skip("live identity migrate")
	}
	dsn := startNamedPostgres(t, "primer_identity_test")
	ctx := context.Background()

	require.NoError(t, identitydb.Migrate(ctx, dsn))
	pool, err := identitydb.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	var reg *string
	err = pool.QueryRow(ctx, `SELECT to_regclass('public.identity_goose_db_version')::text`).Scan(&reg)
	require.NoError(t, err)
	require.NotNil(t, reg)
	require.Equal(t, "identity_goose_db_version", *reg)

	var meta string
	err = pool.QueryRow(ctx, `SELECT value FROM schema_meta WHERE key = 'service'`).Scan(&meta)
	require.NoError(t, err)
	require.Equal(t, "primer-identity", meta)
}

// Root/module CLI path: identity-migrate must refuse foreign DSNs without pollution.
func TestLiveCLIRefusesForeignDBWithoutPollution(t *testing.T) {
	if testing.Short() {
		t.Skip("live CLI isolation")
	}
	dsn := startNamedPostgres(t, "curriculum_studio_test")

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	cmd := exec.Command("go", "run", "./cmd/identity-migrate", "up")
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(),
		"IDENTITY_DATABASE_URL="+dsn,
		"IDENTITY_ISSUER=http://localhost:8090",
		"IDENTITY_ENV=test",
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "CLI must fail on foreign DB; out=%s", string(out))
	combined := strings.ToLower(string(out))
	require.True(t,
		strings.Contains(combined, "forbidden") || strings.Contains(combined, "reserved"),
		"CLI output must cite isolation failure; out=%s", string(out),
	)

	tables, schemas := countIdentityGooseArtifacts(t, dsn)
	require.Zero(t, tables, "CLI must not create identity_goose on foreign DB")
	require.Zero(t, schemas)
}

// Root Makefile target path (when run from monorepo root).
func TestLiveRootMakeMigrateIdentityRefusesForeign(t *testing.T) {
	if testing.Short() {
		t.Skip("live root make isolation")
	}
	dsn := startNamedPostgres(t, "primer_tv_test")

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	makefile := filepath.Join(repoRoot, "Makefile")
	if _, err := os.Stat(makefile); err != nil {
		t.Skip("not in monorepo worktree with root Makefile")
	}

	cmd := exec.Command("make", "migrate-identity")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"IDENTITY_DATABASE_URL="+dsn,
		"IDENTITY_ISSUER=http://localhost:8090",
		"IDENTITY_ENV=test",
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "make migrate-identity must fail on foreign DB; out=%s", string(out))
	combined := strings.ToLower(string(out))
	require.True(t,
		strings.Contains(combined, "forbidden") ||
			strings.Contains(combined, "reserved") ||
			strings.Contains(combined, "database"),
		"make output must cite isolation failure; out=%s", string(out),
	)

	tables, _ := countIdentityGooseArtifacts(t, dsn)
	require.Zero(t, tables)
}
