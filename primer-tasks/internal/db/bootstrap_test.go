package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBootstrapRejectsInvalidMappingBeforeDatabase(t *testing.T) {
	valid := ParentLink{Issuer: "https://clerk.example", Subject: "user", TenantID: uuid.NewString(), ActorRef: "parent"}
	for name, edit := range map[string]func(*ParentLink){
		"insecure issuer":    func(p *ParentLink) { p.Issuer = "http://issuer" },
		"issuer credentials": func(p *ParentLink) { p.Issuer = "https://user@issuer" },
		"invalid household":  func(p *ParentLink) { p.TenantID = "not-uuid" },
		"missing subject":    func(p *ParentLink) { p.Subject = "" },
		"missing actor":      func(p *ParentLink) { p.ActorRef = "" },
		"missing new name":   func(p *ParentLink) { p.CreateHousehold = true },
	} {
		t.Run(name, func(t *testing.T) {
			p := valid
			edit(&p)
			if BootstrapParent(context.Background(), nil, p) == nil {
				t.Fatal("invalid mapping accepted")
			}
		})
	}
}

func testBootstrapParent(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	in := ParentLink{Issuer: "https://clerk.example", Subject: "new-user", TenantID: uuid.NewString(), ActorRef: "new-local-parent", HouseholdName: "New household"}
	if BootstrapParent(ctx, pool, in) == nil {
		t.Fatal("bootstrap silently enrolled household")
	}
	in.CreateHousehold = true
	if err := BootstrapParent(ctx, pool, in); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapParent(ctx, pool, in); err != nil {
		t.Fatal("initial provisioning not idempotent", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM parent_identities WHERE issuer=$1 AND subject=$2 AND tenant_id=$3 AND subject_ref=$4`, in.Issuer, in.Subject, in.TenantID, in.ActorRef).Scan(&count); err != nil || count != 1 {
		t.Fatal("canonical mapping mismatch")
	}
	changed := in
	changed.ActorRef = "another-parent"
	if BootstrapParent(ctx, pool, changed) == nil {
		t.Fatal("identity rebound to new actor")
	}
	if _, err := pool.Exec(ctx, `UPDATE parent_identities SET revoked_at=now() WHERE issuer=$1 AND subject=$2`, in.Issuer, in.Subject); err != nil {
		t.Fatal(err)
	}
	if BootstrapParent(ctx, pool, in) == nil {
		t.Fatal("bootstrap reactivated identity")
	}
	if _, err := pool.Exec(ctx, `UPDATE parent_memberships SET revoked_at=now() WHERE tenant_id=$1`, in.TenantID); err != nil {
		t.Fatal(err)
	}
	in.Subject = "another-user"
	if BootstrapParent(ctx, pool, in) == nil {
		t.Fatal("bootstrap reactivated membership")
	}
}
