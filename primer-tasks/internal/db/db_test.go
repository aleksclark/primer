package db

import (
	"strings"
	"testing"
)

func TestSafeDatabaseNameRejectsOtherProductDatabases(t *testing.T) {
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
