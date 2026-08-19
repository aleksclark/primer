package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"primer-tasks/internal/domain/parent"
)

func TestParentConfirmationSQLAndMemoryAreSingleUse(t *testing.T) {
	ctx := context.Background()
	action := parent.RetireTaskAction("task-1", 2)
	digest, err := parent.ActionDigest(action)
	if err != nil {
		t.Fatal(err)
	}
	preview := parent.ConfirmationPreview{TenantID: "00000000-0000-0000-0000-0000000000a1", ActorID: "parent-a", Action: action, ActionDigest: digest, Summary: "Retire task", ExpiresAt: time.Now().UTC().Add(time.Minute)}
	memory := parent.NewMemoryConfirmationStore()
	issued, err := memory.Issue(ctx, preview)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Consume(ctx, issued.Handle, "00000000-0000-0000-0000-0000000000a1", "parent-a", digest); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Consume(ctx, issued.Handle, "00000000-0000-0000-0000-0000000000a1", "parent-a", digest); !errors.Is(err, parent.ErrConfirmationReplay) {
		t.Fatalf("replay=%v", err)
	}
	pool := integrationPool(t)
	if _, err := pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES('00000000-0000-0000-0000-0000000000a1','Confirmation test') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM parent_confirmation_previews WHERE tenant_id='00000000-0000-0000-0000-0000000000a1'`)
	})
	// The SQL table is part of the migrated Tasks schema; the same preview is
	// persisted and consumed through the production confirmation boundary.
	sqlStore := &parent.SQLConfirmationStore{DB: pool, Now: func() time.Time { return time.Now().UTC() }}
	issued, err = sqlStore.Issue(ctx, preview)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlStore.Consume(ctx, issued.Handle, "00000000-0000-0000-0000-0000000000a1", "parent-a", digest); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlStore.Consume(ctx, issued.Handle, "00000000-0000-0000-0000-0000000000a1", "parent-a", digest); !errors.Is(err, parent.ErrConfirmationReplay) {
		t.Fatalf("sql replay=%v", err)
	}
}
