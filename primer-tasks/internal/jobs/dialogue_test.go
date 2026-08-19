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
