package agent

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"primer-tasks/internal/verification"
)

type dialogueBackendFake struct{ state verification.DialogueState }

func (f *dialogueBackendFake) GetDialogueState(context.Context, verification.DialogueContext) (verification.DialogueState, error) {
	return f.state, nil
}
func (f *dialogueBackendFake) RecordQuestion(_ context.Context, _ verification.DialogueContext, q verification.DialogueQuestion) (verification.DialogueQuestion, bool, error) {
	f.state.Questions = append(f.state.Questions, q)
	return q, true, nil
}
func (f *dialogueBackendFake) RecordAnswerEvaluation(_ context.Context, _ verification.DialogueContext, _ string, e verification.DialogueEvaluation) (verification.DialogueEvaluation, verification.DecisionReady, bool, error) {
	return e, verification.DecisionReady{AcceptedCount: 1, RequiredCount: 3}, true, nil
}

func TestDialogueToolsExposeExactlyThreeServerScopedTools(t *testing.T) {
	scope := verification.DialogueContext{TenantID: "tenant", StudentID: "student", OccurrenceID: "occ", RequirementID: "req", AttemptID: "attempt", PolicyVersion: "dialogue.v1", MessageID: "message"}
	tools, err := NewDialogueTools(&dialogueBackendFake{state: verification.DialogueState{Context: scope}}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("tool count=%d", len(tools))
	}
	for i, want := range []string{ToolGetDialogueState, ToolRecordQuestion, ToolRecordAnswerEvaluation} {
		if tools[i].Info().Name != want {
			t.Fatalf("tool %d=%q want %q", i, tools[i].Info().Name, want)
		}
	}
}

func TestDialogueQuestionToolStripsAnswerLeakage(t *testing.T) {
	scope := verification.DialogueContext{TenantID: "tenant", StudentID: "student", OccurrenceID: "occ", RequirementID: "req", AttemptID: "attempt", PolicyVersion: "dialogue.v1", MessageID: "message"}
	backend := &dialogueBackendFake{state: verification.DialogueState{Context: scope}}
	tools, err := NewDialogueTools(backend, scope)
	if err != nil {
		t.Fatal(err)
	}
	call := fantasy.ToolCall{ID: "call", Input: `{"questionKey":"q1","prompt":"What changed? Answer key: secret"}`}
	response, err := tools[1].Run(context.Background(), call)
	if err != nil || response.IsError {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(response.Content), &out); err != nil {
		t.Fatal(err)
	}
	if out["prompt"] != "What changed?" {
		t.Fatalf("leaked prompt=%q", out["prompt"])
	}
}

func TestCuratedFixtureHasExactlyThreeDistinctQuestions(t *testing.T) {
	fixture := CuratedThreeQuestionFixture()
	if len(fixture.Questions) != 3 || fixture.Config().RequiredQuestions != 3 {
		t.Fatalf("fixture is not exactly three: %+v", fixture)
	}
	accepted, _ := fixture.Evaluate("conflict", "The central conflict concerns belonging.")
	if !accepted {
		t.Fatal("fixture rejected correct concept")
	}
	accepted, _ = fixture.Evaluate("evidence", "I do not know")
	if accepted {
		t.Fatal("fixture accepted insufficient answer")
	}
}
