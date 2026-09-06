package db

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAppliedClerkMigrationIsImmutable(t *testing.T) {
	b, err := migrations.ReadFile("migrations/00005_clerk_parents.sql")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != "c1b3b82cc4b66a85215dd5781a29871e61293702b31201562703bb3be751d720" {
		t.Fatal("applied Clerk migration changed")
	}
}

// Run alongside the fresh real-PG migration gate. Build exactly the released
// five-migration schema, including local identities, then apply incoming
// P3/P4 migrations with the production migrator. No donor schema is substituted.
func testParentAgentUpgrade(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE SCHEMA p3_upgrade`); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DROP SCHEMA p3_upgrade CASCADE`)
	cfg := pool.Config().Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = "p3_upgrade"
	upgrade, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer upgrade.Close()
	if _, err = upgrade.Exec(ctx, `CREATE TABLE tasks_schema_migrations(version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() > "00005_clerk_parents.sql" {
			continue
		}
		content, e := migrations.ReadFile("migrations/" + entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		if _, e = upgrade.Exec(ctx, string(content)); e != nil {
			t.Fatal(e)
		}
		if _, e = upgrade.Exec(ctx, `INSERT INTO tasks_schema_migrations(version) VALUES($1)`, entry.Name()); e != nil {
			t.Fatal(e)
		}
	}
	if _, err = upgrade.Exec(ctx, `INSERT INTO tenants(id,name) VALUES('00000000-0000-0000-0000-0000000000a1','Preserved household');
 INSERT INTO parent_memberships(tenant_id,subject_ref,role) VALUES('00000000-0000-0000-0000-0000000000a1','local-parent','admin');
 INSERT INTO parent_identities(issuer,subject,tenant_id,subject_ref) VALUES('https://fixture.invalid','clerk-parent','00000000-0000-0000-0000-0000000000a1','local-parent');
 INSERT INTO students(id,tenant_id,display_name) VALUES('00000000-0000-0000-0000-0000000000a2','00000000-0000-0000-0000-0000000000a1','Preserved student');
 INSERT INTO task_templates(id,tenant_id,title) VALUES('00000000-0000-0000-0000-0000000000a3','00000000-0000-0000-0000-0000000000a1','Preserved task');
 INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions) VALUES('00000000-0000-0000-0000-0000000000a4','00000000-0000-0000-0000-0000000000a1','00000000-0000-0000-0000-0000000000a3',1,'Preserved task','Immutable original instructions');`); err != nil {
		t.Fatal(err)
	}
	const snapshot = `SELECT jsonb_build_object('identities',(SELECT jsonb_agg(to_jsonb(i)) FROM parent_identities i),'members',(SELECT jsonb_agg(to_jsonb(m)) FROM parent_memberships m),'students',(SELECT jsonb_agg(to_jsonb(s)) FROM students s),'revisions',(SELECT jsonb_agg(to_jsonb(r)) FROM task_revisions r),'applied',(SELECT jsonb_agg(to_jsonb(v) ORDER BY version) FROM tasks_schema_migrations v WHERE version<='00005_clerk_parents.sql'))::text`
	var before, after string
	if err = upgrade.QueryRow(ctx, snapshot).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, upgrade); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, upgrade); err != nil {
		t.Fatal(err)
	}
	if err = upgrade.QueryRow(ctx, snapshot).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("P3 upgrade changed released identity/student/revision/applied history")
	}
	var n int
	if err = upgrade.QueryRow(ctx, `SELECT count(*) FROM tasks_schema_migrations`).Scan(&n); err != nil || n != 11 {
		t.Fatalf("incoming migration count %d: %v", n, err)
	}
}
