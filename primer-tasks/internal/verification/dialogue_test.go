package verification

import (
	"errors"
	"testing"

	"primer-tasks/internal/domain"
)

func dialogueState() DialogueState {
	return DialogueState{Context: DialogueContext{TenantID: "t", StudentID: "s", OccurrenceID: "o", RequirementID: "r", AttemptID: "a", PolicyVersion: "dialogue.v1"}, Config: domain.DialogueConfig{SourceRef: "fixture://chapter-4", LearningFocus: "focus", RequiredQuestions: 3, Rubric: []string{"criterion"}, AllowedFollowUps: 1, MaxAttempts: 2, MaxTurns: 8, RetentionPolicy: "retain"}}
}

func TestDialogueAuthorityRequiresDistinctQuestionsAndCountsAcceptedOnly(t *testing.T) {
	s := dialogueState()
	var err error
	s, err = RecordQuestion(s, DialogueQuestion{ID: "q1", QuestionKey: "one", Prompt: "Question one", Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = RecordQuestion(s, DialogueQuestion{ID: "q1b", QuestionKey: "one", Prompt: "Repeat", Ordinal: 2}); !errors.Is(err, ErrDuplicateQuestion) {
		t.Fatalf("duplicate question error=%v", err)
	}
	var ready DecisionReady
	s, ready, err = RecordEvaluation(s, DialogueEvaluation{ID: "e1", QuestionID: "q1", MessageID: "m1", Accepted: false, Rationale: "needs a detail", PolicyVersion: "dialogue.v1"})
	if err != nil || ready.AcceptedCount != 0 {
		t.Fatalf("rejected evaluation state=%v ready=%+v err=%v", s, ready, err)
	}
	if _, _, err = RecordEvaluation(s, DialogueEvaluation{ID: "e1b", QuestionID: "q1", MessageID: "m1", Accepted: true, Rationale: "changed", PolicyVersion: "dialogue.v1"}); !errors.Is(err, ErrDuplicateEvaluation) {
		t.Fatalf("idempotency error=%v", err)
	}
}

func TestDialogueRationaleNeverAcceptsRawReasoning(t *testing.T) {
	for _, raw := range []string{"chain of thought: secret", "answer key: chapter answer", "<think>private</think>"} {
		if _, err := SafeRationale(raw); !errors.Is(err, ErrDialogueEvaluation) {
			t.Fatalf("unsafe rationale accepted: %q", raw)
		}
	}
}
