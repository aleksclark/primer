package db

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestSafeDatabaseNameRejectsOtherProductDatabases(t *testing.T) {
	t.Setenv("TASKS_DATABASE_URL", "postgres://tasks@localhost:5432/primer_tasks_from_env")
	if got := DSN(); got != "postgres://tasks@localhost:5432/primer_tasks_from_env" {
		t.Fatalf("DSN from environment = %q", got)
	}
	t.Setenv("TASKS_DATABASE_URL", "")
	if got := DSN(); got == "" {
		t.Fatal("default DSN is empty")
	}
	for _, dsn := range []string{
		"postgres://u:p@localhost:5432/primer_tv",
		"postgres://u:p@localhost:5432/curriculum_studio",
		"postgres://u:p@localhost:5432/primer_identity",
	} {
		if err := SafeDatabaseName(dsn); err == nil {
			t.Errorf("accepted forbidden DSN %q", dsn)
		}
	}
	if err := SafeDatabaseName("postgres://tasks@localhost:5432/primer_tasks"); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateIsIdempotentAgainstRealPostgres(t *testing.T) {
	if dsn := os.Getenv("TASKS_TEST_DATABASE_URL"); dsn != "" {
		testMigrateAgainstURL(t, dsn)
		return
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		if os.Getenv("PRIMER_TASKS_COVERAGE_GATE") == "1" {
			t.Fatalf("Docker is required for the Tasks coverage gate: %v", err)
		}
		t.Skipf("Docker is unavailable; skipping PostgreSQL integration test: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("primer_tasks_migration_test"),
		tcpostgres.WithUsername("tasks"),
		tcpostgres.WithPassword("tasks"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	testMigrateAgainstURL(t, dsn)
}

func testMigrateAgainstURL(t *testing.T, dsn string) {
	t.Helper()
	if err := SafeDatabaseName(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var applied int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tasks_schema_migrations`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 9 {
		t.Fatalf("applied migrations = %d, want 9", applied)
	}
	var tables int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('tenants','students','auth_states','student_sessions')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 4 {
		t.Fatalf("Tasks schema table count = %d, want 4", tables)
	}
	testBootstrapParent(t, pool)
	testParentAgentUpgrade(t, pool)
}

func TestMigrationTableAndOwnershipConstraintsAreProductLocal(t *testing.T) {
	if err := SafeDatabaseName(DSN()); err != nil {
		t.Fatal(err)
	}
	b, err := migrations.ReadFile("migrations/00002_auth_pairing_hardening.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS auth_states", "pairing_codes_student_tenant_fk", "student_devices_student_tenant_fk", "student_sessions_student_tenant_fk"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("migration missing %q", fragment)
		}
	}
	if strings.Contains(text, "goose_db_version") {
		t.Fatal("generic goose table is not allowed")
	}
}
