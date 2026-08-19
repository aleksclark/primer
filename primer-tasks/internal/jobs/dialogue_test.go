package jobs

import (
	"context"
	"testing"
	"time"
)

func TestDialogueJobValidationFailsClosed(t *testing.T) {
	var r *PostgresRepository
	if err := r.EnqueueDialogue(context.Background(), DialogueJob{TenantID: "t", AttemptID: "a", MessageID: "m", MaxAttempts: 1}); err == nil {
		t.Fatal("nil repository accepted dialogue job")
	}
	if _, _, err := r.ClaimDialogue(context.Background(), "owner", time.Second); err == nil {
		t.Fatal("nil repository claimed dialogue job")
	}
	if err := r.CompleteDialogue(context.Background(), "job", "owner"); err == nil {
		t.Fatal("nil repository completed dialogue job")
	}
	if err := r.FailDialogue(context.Background(), "job", "owner", context.Canceled); err == nil {
		t.Fatal("nil repository failed dialogue job")
	}
	if err := r.RequeueExpiredDialogue(context.Background(), time.Now()); err == nil {
		t.Fatal("nil repository requeued dialogue job")
	}
	for _, job := range []DialogueJob{{TenantID: "", AttemptID: "a", MessageID: "m", MaxAttempts: 1}, {TenantID: "t", AttemptID: "a", MessageID: "m", MaxAttempts: 11}} {
		if err := r.EnqueueDialogue(context.Background(), job); err == nil {
			t.Fatal("invalid dialogue job accepted")
		}
	}
}

func TestDialogueEventValidation(t *testing.T) {
	var r *PostgresRepository
	if err := r.AppendDialogueEvent(context.Background(), DialogueEvent{TenantID: "t", AttemptID: "a", Kind: "state"}); err == nil {
		t.Fatal("zero sequence accepted")
	}
	if _, err := r.ReplayDialogueEvents(context.Background(), "t", "a", 0, 0); err == nil {
		t.Fatal("zero replay limit accepted")
	}
}
