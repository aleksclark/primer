package verification

import (
	"errors"
	"strings"
	"testing"
)

func TestDialogueEvaluationCriteriaAndQuestionBoundaries(t *testing.T) {
	s := dialogueState()
	var err error
	s, err = RecordQuestion(s, DialogueQuestion{ID: "q-criteria", QuestionKey: "criteria", Prompt: "Name the source fact.", Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		criteria []string
	}{
		{"missing accepted criterion", nil},
		{"unknown criterion", []string{"not parent authored"}},
		{"empty criterion", []string{"   "}},
		{"duplicate criterion", []string{"criterion", "CRITERION"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := RecordEvaluation(s, DialogueEvaluation{
				QuestionID: "q-criteria", MessageID: tc.name, Accepted: true,
				Criteria: tc.criteria, Rationale: "Names the source fact.", PolicyVersion: "dialogue.v1",
			})
			if !errors.Is(err, ErrDialogueEvaluation) {
				t.Fatalf("criteria=%v error=%v", tc.criteria, err)
			}
		})
	}

	accepted, ready, err := RecordEvaluation(s, DialogueEvaluation{
		QuestionID: "q-criteria", MessageID: "accepted", Accepted: true,
		Criteria: []string{"criterion"}, Rationale: "Names the source fact.", PolicyVersion: "dialogue.v1",
	})
	if err != nil || ready.Accepted || ready.AcceptedCount != 1 || accepted.AcceptedCount != 1 {
		t.Fatalf("accepted=%+v ready=%+v err=%v", accepted, ready, err)
	}
	if _, _, err = RecordEvaluation(s, DialogueEvaluation{
		QuestionID: "q-criteria", MessageID: "rejected", Accepted: false,
		Rationale: "Needs a source detail.", PolicyVersion: "dialogue.v1",
	}); err != nil {
		t.Fatalf("rejected evaluation without criteria: %v", err)
	}
}

func TestDialogueAuthorityRejectsTerminalMalformedAndDuplicateEvidence(t *testing.T) {
	base := dialogueState()
	base.Terminal = true
	if _, err := RecordQuestion(base, DialogueQuestion{ID: "terminal", QuestionKey: "q", Prompt: "Question", Ordinal: 1}); !errors.Is(err, ErrDialogueTerminal) {
		t.Fatalf("terminal question error=%v", err)
	}
	base.Terminal = false
	for _, q := range []DialogueQuestion{
		{ID: "missing-key", Prompt: "Question", Ordinal: 1},
		{ID: "missing-prompt", QuestionKey: "q", Ordinal: 1},
		{ID: "bad-ordinal", QuestionKey: "q", Prompt: "Question", Ordinal: 0},
		{ID: "long-prompt", QuestionKey: "q", Prompt: strings.Repeat("x", 2001), Ordinal: 1},
	} {
		if _, err := RecordQuestion(base, q); !errors.Is(err, ErrDialogueQuestion) {
			t.Fatalf("question=%+v error=%v", q, err)
		}
	}
	base, err := RecordQuestion(base, DialogueQuestion{ID: "q1", QuestionKey: "one", Prompt: "Question one", Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, duplicate := range []DialogueQuestion{
		{ID: "q2", QuestionKey: "one", Prompt: "same key", Ordinal: 2},
		{ID: "q3", QuestionKey: "two", Prompt: "same ordinal", Ordinal: 1},
	} {
		if _, err := RecordQuestion(base, duplicate); !errors.Is(err, ErrDuplicateQuestion) {
			t.Fatalf("duplicate=%+v error=%v", duplicate, err)
		}
	}
	for _, rationale := range []string{"chain of thought", "system prompt", "answer key", "internal reasoning", "<think>"} {
		if _, err := SafeRationale(rationale); !errors.Is(err, ErrDialogueEvaluation) {
			t.Fatalf("forbidden rationale %q error=%v", rationale, err)
		}
	}
	if _, err := SafeRationale(strings.Repeat("x", 501)); !errors.Is(err, ErrDialogueEvaluation) {
		t.Fatalf("long rationale error=%v", err)
	}
}

func TestDialogueEvaluationRejectsTerminalAndUnsafeRationale(t *testing.T) {
	s := dialogueState()
	var err error
	s, err = RecordQuestion(s, DialogueQuestion{ID: "q1", QuestionKey: "one", Prompt: "Question one", Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, evaluation := range []DialogueEvaluation{
		{QuestionID: "q1", MessageID: "m", Accepted: false, Rationale: "answer\x00with control", PolicyVersion: "dialogue.v1"},
		{QuestionID: "q1", MessageID: "m", Accepted: true, Criteria: []string{"criterion"}, Rationale: "good", PolicyVersion: "wrong"},
	} {
		if _, _, err := RecordEvaluation(s, evaluation); !errors.Is(err, ErrDialogueEvaluation) {
			t.Fatalf("evaluation=%+v error=%v", evaluation, err)
		}
	}
	accepted := s
	accepted.AcceptedCount = accepted.Config.RequiredQuestions
	if _, _, err := RecordEvaluation(accepted, DialogueEvaluation{QuestionID: "q1", MessageID: "m", Accepted: false, PolicyVersion: "dialogue.v1"}); !errors.Is(err, ErrDialogueTerminal) {
		t.Fatalf("accepted-limit error=%v", err)
	}
}

func TestDialogueStateRejectsInvalidCounters(t *testing.T) {
	base := dialogueState()
	for name, mutate := range map[string]func(*DialogueState){
		"accepted overflow": func(s *DialogueState) { s.AcceptedCount = s.Config.RequiredQuestions + 1 },
		"negative turns":    func(s *DialogueState) { s.TurnCount = -1 },
		"invalid config":    func(s *DialogueState) { s.Config.RequiredQuestions = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			state := base
			mutate(&state)
			if err := state.Validate(); err == nil {
				t.Fatal("invalid dialogue state accepted")
			}
		})
	}
}

func TestDialogueQuestionLimitAndEvaluationIdentityValidation(t *testing.T) {
	s := dialogueState()
	s.Config.MaxTurns = 3
	var err error
	for _, q := range []DialogueQuestion{
		{ID: "q1", QuestionKey: "one", Prompt: "Question one", Ordinal: 1},
		{ID: "q2", QuestionKey: "two", Prompt: "Question two", Ordinal: 2},
		{ID: "q3", QuestionKey: "three", Prompt: "Question three", Ordinal: 3},
	} {
		if s, err = RecordQuestion(s, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = RecordQuestion(s, DialogueQuestion{ID: "q4", QuestionKey: "four", Prompt: "Question four", Ordinal: 4}); !errors.Is(err, ErrDialogueLimit) {
		t.Fatalf("question limit error=%v", err)
	}
	for _, e := range []DialogueEvaluation{
		{QuestionID: "missing", MessageID: "m", Accepted: false, PolicyVersion: "dialogue.v1"},
		{QuestionID: "q1", MessageID: "m", Accepted: false, PolicyVersion: "wrong"},
		{QuestionID: "q1", MessageID: "", Accepted: false, PolicyVersion: "dialogue.v1"},
	} {
		if _, _, err = RecordEvaluation(s, e); !errors.Is(err, ErrDialogueEvaluation) {
			t.Fatalf("evaluation=%+v error=%v", e, err)
		}
	}
}
