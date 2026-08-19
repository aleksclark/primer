package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/fantasy"
	"primer-tasks/internal/verification"
)

type dialogueFailureBackend struct {
	state verification.DialogueState
	err   error
}

func (b *dialogueFailureBackend) GetDialogueState(context.Context, verification.DialogueContext) (verification.DialogueState, error) {
	return b.state, b.err
}
func (b *dialogueFailureBackend) RecordQuestion(_ context.Context, _ verification.DialogueContext, q verification.DialogueQuestion) (verification.DialogueQuestion, bool, error) {
	if b.err != nil {
		return q, false, b.err
	}
	return q, true, nil
}
func (b *dialogueFailureBackend) RecordAnswerEvaluation(_ context.Context, _ verification.DialogueContext, _ string, e verification.DialogueEvaluation) (verification.DialogueEvaluation, verification.DecisionReady, bool, error) {
	if b.err != nil {
		return e, verification.DecisionReady{}, false, b.err
	}
	return e, verification.DecisionReady{AcceptedCount: 1, RequiredCount: 3}, true, nil
}

func dialogueTestScope(withMessage bool) verification.DialogueContext {
	s := verification.DialogueContext{TenantID: "tenant", StudentID: "student", OccurrenceID: "occurrence", RequirementID: "requirement", AttemptID: "attempt", PolicyVersion: "dialogue.v1"}
	if withMessage {
		s.MessageID = "message"
	}
	return s
}

func TestDialogueToolsRejectMalformedAndProviderFailureInputs(t *testing.T) {
	scope := dialogueTestScope(false)
	backend := &dialogueFailureBackend{state: verification.DialogueState{Context: scope, Config: CuratedThreeQuestionFixture().Config()}}
	tools, err := NewDialogueTools(backend, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		tool  int
		input string
	}{
		{"missing question key", 1, `{"questionKey":"","prompt":"A useful question"}`},
		{"empty question", 1, `{"questionKey":"q","prompt":""}`},
		{"missing evaluation key", 2, `{"questionKey":"","accepted":false,"rationale":"not enough"}`},
		{"unsafe evaluation rationale", 2, `{"questionKey":"q","accepted":false,"rationale":"answer key: hidden"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, runErr := tools[tc.tool].Run(context.Background(), fantasy.ToolCall{ID: tc.name, Input: tc.input})
			if runErr != nil || !response.IsError {
				t.Fatalf("response=%+v err=%v", response, runErr)
			}
		})
	}

	noMessageTools, err := NewDialogueTools(&dialogueFailureBackend{state: verification.DialogueState{Context: scope, Config: CuratedThreeQuestionFixture().Config()}}, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range noMessageTools[1:] {
		response, runErr := tool.Run(context.Background(), fantasy.ToolCall{ID: "no-message", Input: `{ "questionKey":"q", "prompt":"Question" }`})
		if runErr != nil || !response.IsError {
			t.Fatalf("missing message response=%+v err=%v", response, runErr)
		}
	}

	failing := &dialogueFailureBackend{state: verification.DialogueState{Context: dialogueTestScope(true), Config: CuratedThreeQuestionFixture().Config()}, err: errors.New("provider persistence unavailable")}
	failingTools, err := NewDialogueTools(failing, dialogueTestScope(true))
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range failingTools {
		input := `{}`
		if tool.Info().Name == ToolRecordQuestion {
			input = `{"questionKey":"q","prompt":"Question"}`
		} else if tool.Info().Name == ToolRecordAnswerEvaluation {
			input = `{"questionKey":"q","accepted":false,"rationale":"needs evidence"}`
		}
		response, runErr := tool.Run(context.Background(), fantasy.ToolCall{ID: "backend-failure", Input: input})
		if runErr != nil || !response.IsError {
			t.Fatalf("backend failure tool=%s response=%+v err=%v", tool.Info().Name, response, runErr)
		}
	}
}

func TestDialogueToolsPersistSafeQuestionAndEvaluation(t *testing.T) {
	scope := dialogueTestScope(true)
	state := verification.DialogueState{Context: scope, Config: CuratedThreeQuestionFixture().Config(), Messages: []verification.DialogueMessage{{ID: "m-1", Role: "student", Content: "answer", Sequence: 1}}}
	backend := &dialogueFailureBackend{state: state}
	tools, err := NewDialogueTools(backend, scope)
	if err != nil {
		t.Fatal(err)
	}
	stateResponse, runErr := tools[0].Run(context.Background(), fantasy.ToolCall{ID: "state", Input: `{}`})
	if runErr != nil || stateResponse.IsError || !strings.Contains(stateResponse.Content, `"history"`) {
		t.Fatalf("state response=%+v err=%v", stateResponse, runErr)
	}
	questionResponse, runErr := tools[1].Run(context.Background(), fantasy.ToolCall{ID: "question", Input: `{"questionKey":"conflict","prompt":"What is the central conflict?"}`})
	if runErr != nil || questionResponse.IsError || !strings.Contains(questionResponse.Content, `"questionKey":"conflict"`) {
		t.Fatalf("question response=%+v err=%v", questionResponse, runErr)
	}
	evaluationResponse, runErr := tools[2].Run(context.Background(), fantasy.ToolCall{ID: "evaluation", Input: `{"questionKey":"conflict","accepted":true,"criteria":["answers address the distinct question"],"rationale":"Addresses the source fact."}`})
	if runErr != nil || evaluationResponse.IsError || !strings.Contains(evaluationResponse.Content, `"accepted":true`) {
		t.Fatalf("evaluation response=%+v err=%v", evaluationResponse, runErr)
	}
}

func TestDialogueStateToolBoundsHistoryToRecentEvidence(t *testing.T) {
	scope := dialogueTestScope(true)
	state := verification.DialogueState{Context: scope, Config: CuratedThreeQuestionFixture().Config()}
	for i := 0; i < 105; i++ {
		state.Messages = append(state.Messages, verification.DialogueMessage{ID: "message-" + string(rune('a'+i%26)), Role: "student", Content: "answer", Sequence: int64(i + 1)})
	}
	tools, err := NewDialogueTools(&dialogueFailureBackend{state: state}, scope)
	if err != nil {
		t.Fatal(err)
	}
	response, err := tools[0].Run(context.Background(), fantasy.ToolCall{ID: "history", Input: `{}`})
	if err != nil || response.IsError {
		t.Fatalf("history response=%+v err=%v", response, err)
	}
	if strings.Contains(response.Content, `"sequence":1}`) || !strings.Contains(response.Content, `"sequence":105}`) {
		t.Fatalf("history was not bounded to the latest 100 messages: %s", response.Content)
	}
}
