package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Schema-upgrade evidence only. The synthetic canonical P3/P4 rows below are
// preservation preconditions, not public dialogue or native-device acceptance.
func TestManagementUpgradePreservesCanonicalEleven(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("primer_tasks_management_upgrade"),
		tcpostgres.WithUsername("tasks"), tcpostgres.WithPassword("tasks"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatalf("owned PostgreSQL is required for upgrade proof: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Errorf("owned PostgreSQL cleanup: %v", err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	execDialogueSQL(t, ctx, pool, `CREATE TABLE tasks_schema_migrations(version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`)
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	const canonicalEnd = "00011_dialogue_evaluation_criteria.sql"
	for _, entry := range entries {
		if entry.Name() > canonicalEnd {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		execDialogueSQL(t, ctx, pool, string(body))
		execDialogueSQL(t, ctx, pool, `INSERT INTO tasks_schema_migrations(version) VALUES($1)`, entry.Name())
	}
	fixture := seedDialogueLegacy(t, ctx, pool)
	insertDialogueProjection(t, ctx, pool, fixture)
	rows, err := pool.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		out := make(map[string]string, len(tables))
		for _, table := range tables {
			filter := ""
			if table == "tasks_schema_migrations" {
				filter = " WHERE version<='" + canonicalEnd + "'"
			}
			query := fmt.Sprintf(`SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM %s r%s`, pgx.Identifier{table}.Sanitize(), filter)
			var body string
			if err := pool.QueryRow(ctx, query).Scan(&body); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256([]byte(body))
			out[table] = hex.EncodeToString(hash[:])
		}
		return out
	}
	before := snapshot()
	for i := 0; i < 2; i++ {
		if err := Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	if after := snapshot(); !reflect.DeepEqual(before, after) {
		for name, hash := range before {
			if after[name] != hash {
				t.Errorf("management upgrade changed canonical table %s", name)
			}
		}
	}
	var canonical, management, total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE version<=$1), count(*) FILTER (WHERE version>$1), count(*) FROM tasks_schema_migrations`, canonicalEnd).Scan(&canonical, &management, &total); err != nil {
		t.Fatal(err)
	}
	if canonical != 11 || management != 4 || total != 15 {
		t.Fatalf("migration ledger canonical=%d management=%d total=%d, want 11/4/15", canonical, management, total)
	}
	var devices int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM management_devices`).Scan(&devices); err != nil || devices != 0 {
		t.Fatalf("new management devices = %d: %v", devices, err)
	}
}
