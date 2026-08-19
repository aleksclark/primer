package repo

import (
	"context"
	"testing"

	"primer-tasks/internal/verification"
)

func TestDialogueRepositoryValidationFailsClosed(t *testing.T) {
	var r *DialogueRepository
	ctx := context.Background()
	if _, err := r.GetDialogueState(ctx, verification.DialogueContext{}); err == nil {
		t.Fatal("nil state lookup accepted")
	}
	if _, _, err := r.AppendMessage(ctx, verification.DialogueContext{}, "id", "student", "answer", "client"); err == nil {
		t.Fatal("nil message append accepted")
	}
	if _, _, err := r.RecordQuestion(ctx, verification.DialogueContext{}, verification.DialogueQuestion{}); err == nil {
		t.Fatal("nil question record accepted")
	}
	if _, _, _, err := r.RecordAnswerEvaluation(ctx, verification.DialogueContext{}, "q", verification.DialogueEvaluation{}); err == nil {
		t.Fatal("nil evaluation accepted")
	}
	if err := r.CreateDialogueAttempt(ctx, DialogueAttempt{}); err == nil {
		t.Fatal("nil attempt accepted")
	}
	if _, err := decodeConfig([]byte("not-json")); err == nil {
		t.Fatal("malformed config accepted")
	}
	if !isUniqueViolation(testError("duplicate key value violates unique constraint")) || isUniqueViolation(testError("other")) {
		t.Fatal("unique violation classifier failed")
	}
}

type testError string

func (e testError) Error() string { return string(e) }
