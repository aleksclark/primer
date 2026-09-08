package jobs

import (
	"context"
	"testing"
	"time"
)

func TestDialogueLeaseHelpersRejectMissingAuthority(t *testing.T) {
	repo := NewPostgresRepository(nil)
	ctx := context.Background()
	if _, ok, err := repo.ClaimDialogue(ctx, ""); err == nil || ok {
		t.Fatal("nil repository accepted an empty owner")
	}
	if err := repo.RenewDialogue(ctx, DialogueJob{}); err == nil {
		t.Fatal("nil repository renewed an empty lease")
	}
	if err := repo.FailDialogue(ctx, DialogueJob{ID: "missing"}, "provider_unavailable"); err == nil {
		t.Fatal("nil repository failed a missing job")
	}
	if _, err := repo.lockDialogueJob(ctx, DialogueJob{ID: "x", TenantID: "y", LeaseOwner: "z", LeaseGeneration: 1}); err == nil {
		t.Fatal("nil repository locked a job")
	}
	_ = time.Second
}
