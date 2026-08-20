package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"charm.land/fantasy"
	"primer-tasks/internal/verification"
)

type dialogueBackendFake struct {
	state               verification.DialogueState
	getErr, questionErr error
	evaluationErr       error
	lastEvaluation      verification.DialogueEvaluation
}

func (f *dialogueBackendFake) GetDialogueState(context.Context, verification.DialogueContext) (verification.DialogueState, error) {
	return f.state, f.getErr
}
func (f *dialogueBackendFake) RecordQuestion(_ context.Context, _ verification.DialogueContext, q verification.DialogueQuestion) (verification.DialogueQuestion, bool, error) {
	if f.questionErr != nil {
		return q, false, f.questionErr
	}
	f.state.Questions = append(f.state.Questions, q)
	return q, true, nil
}
func (f *dialogueBackendFake) RecordAnswerEvaluation(_ context.Context, _ verification.DialogueContext, _ string, e verification.DialogueEvaluation) (verification.DialogueEvaluation, verification.DecisionReady, bool, error) {
	if f.evaluationErr != nil {
		return e, verification.DecisionReady{}, false, f.evaluationErr
	}
	f.lastEvaluation = e
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

func TestDialogueStateAndEvaluationToolsUseSafeBoundaries(t *testing.T) {
	scope := verification.DialogueContext{TenantID: "tenant", StudentID: "student", OccurrenceID: "occ", RequirementID: "req", AttemptID: "attempt", PolicyVersion: "dialogue.v1", Provider: "scripted", Model: "fixture-model", MessageID: "message"}
	state := verification.DialogueState{
		Context:     scope,
		Config:      CuratedThreeQuestionFixture().Config(),
		Questions:   []verification.DialogueQuestion{{ID: "q1", QuestionKey: "conflict", Prompt: "What is the conflict?", Ordinal: 1}},
		Evaluations: []verification.DialogueEvaluation{{QuestionID: "q1", MessageID: "m", Accepted: false, Rationale: "needs a detail"}},
	}
	for i := 0; i < 101; i++ {
		state.Messages = append(state.Messages, verification.DialogueMessage{ID: "m", Role: "student", Content: "answer", Sequence: int64(i + 1)})
	}
	backend := &dialogueBackendFake{state: state}
	tools, err := NewDialogueTools(backend, scope)
	if err != nil {
		t.Fatal(err)
	}
	if got := DialogueToolNames(); len(got) != 3 {
		t.Fatalf("names=%v", got)
	}
	response, err := tools[0].Run(context.Background(), fantasy.ToolCall{ID: "state", Input: `{}`})
	if err != nil || response.IsError || !strings.Contains(response.Content, "History") && !strings.Contains(response.Content, "history") {
		t.Fatalf("state response=%+v err=%v", response, err)
	}
	response, err = tools[2].Run(context.Background(), fantasy.ToolCall{ID: "eval", Input: `{"questionKey":"q1","accepted":true,"criteria":["answers address the distinct question"],"rationale":"Names a source fact."}`})
	if err != nil || response.IsError || !strings.Contains(response.Content, "acceptedCount") {
		t.Fatalf("evaluation response=%+v err=%v", response, err)
	}
	if backend.lastEvaluation.Provider != "scripted" || backend.lastEvaluation.Model != "fixture-model" {
		t.Fatalf("evaluation provenance was not copied from the server scope: %+v", backend.lastEvaluation)
	}
	if _, err := NewDialogueTools(nil, scope); err == nil {
		t.Fatal("dialogue tools accepted a missing backend")
	}
}

func TestDialogueToolsFailClosedAtProviderAndInputBoundaries(t *testing.T) {
	scope := verification.DialogueContext{TenantID: "tenant", StudentID: "student", OccurrenceID: "occ", RequirementID: "req", AttemptID: "attempt", PolicyVersion: "dialogue.v1", MessageID: "message"}
	for _, tc := range []struct {
		name    string
		backend *dialogueBackendFake
		index   int
		input   string
	}{
		{"state provider error", &dialogueBackendFake{getErr: errors.New("database down")}, 0, `{}`},
		{"question missing key", &dialogueBackendFake{}, 1, `{"questionKey":"","prompt":"Question"}`},
		{"question missing prompt", &dialogueBackendFake{}, 1, `{"questionKey":"q","prompt":""}`},
		{"question too long", &dialogueBackendFake{}, 1, `{"questionKey":"q","prompt":"` + strings.Repeat("x", 2001) + `"}`},
		{"question provider error", &dialogueBackendFake{getErr: errors.New("database down")}, 1, `{"questionKey":"q","prompt":"Question"}`},
		{"question persistence error", &dialogueBackendFake{questionErr: errors.New("constraint")}, 1, `{"questionKey":"q","prompt":"Question"}`},
		{"evaluation missing key", &dialogueBackendFake{}, 2, `{"questionKey":"","accepted":false}`},
		{"evaluation unsafe rationale", &dialogueBackendFake{}, 2, `{"questionKey":"q","rationale":"answer key: secret"}`},
		{"evaluation provider error", &dialogueBackendFake{evaluationErr: errors.New("constraint")}, 2, `{"questionKey":"q","accepted":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tools, err := NewDialogueTools(tc.backend, scope)
			if err != nil {
				t.Fatal(err)
			}
			response, runErr := tools[tc.index].Run(context.Background(), fantasy.ToolCall{ID: tc.name, Input: tc.input})
			if runErr != nil || !response.IsError {
				t.Fatalf("response=%+v err=%v", response, runErr)
			}
		})
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
