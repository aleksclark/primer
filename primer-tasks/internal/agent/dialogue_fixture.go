package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
	"primer-tasks/internal/domain"
	"primer-tasks/internal/verification"
)

const AdversarialDialogueQuestion = "The answer is that mortar must dry before the next course of stones; why?"
const InlineDialogueFixtureSource = "Ada measured the beam twice. She marked the cut before using the saw. She checked the finished length to prevent mistakes."

type FixtureQuestion struct{ Key, Concept string }
type CuratedDialogueFixture struct {
	SourceRef string
	Questions []FixtureQuestion
}

func CuratedThreeQuestionFixture() CuratedDialogueFixture {
	return CuratedDialogueFixture{SourceRef: "fixture://chapter-4", Questions: []FixtureQuestion{
		{Key: "wall", Concept: "repairing the garden wall"},
		{Key: "mortar", Concept: "waiting for mortar to dry"},
		{Key: "rushing", Concept: "rushing weakens the wall"},
	}}
}
func (f CuratedDialogueFixture) Evaluate(key, answer string) (bool, string) {
	a := strings.ToLower(strings.TrimSpace(answer))
	// Deliberately limited deterministic concept qualification, not model
	// quality. Mentioning a keyword in an injection/contradiction is not success.
	for _, bad := range []string{"ignore", "answer key", "system prompt", "complete", "another student", "another task", "tool", "instead", "not ", "never ", "roof", "strengthen"} {
		if strings.Contains(a, bad) {
			return false, verification.DialogueRetryRationale
		}
	}
	if len(strings.Fields(a)) < 4 {
		return false, verification.DialogueRetryRationale
	}
	accepted := false
	switch key {
	case "wall":
		accepted = strings.Contains(a, "wall") && (strings.Contains(a, "repaired") || strings.Contains(a, "rebuilt") || strings.Contains(a, "fixed"))
	case "mortar":
		accepted = strings.Contains(a, "mortar") && (strings.Contains(a, "dry") || strings.Contains(a, "harden")) && (strings.Contains(a, "before") || strings.Contains(a, "wait") || strings.Contains(a, "because"))
	case "rushing":
		accepted = strings.Contains(a, "rush") && strings.Contains(a, "wall") && (strings.Contains(a, "weaken") || strings.Contains(a, "unstable"))
	case "source-fact":
		accepted = strings.Contains(a, "measur") && strings.Contains(a, "beam") && strings.Contains(a, "twice")
	case "source-evidence":
		accepted = strings.Contains(a, "mark") && strings.Contains(a, "cut")
	case "source-connection":
		accepted = strings.Contains(a, "check") && strings.Contains(a, "length") && (strings.Contains(a, "prevent") || strings.Contains(a, "mistake"))
	}
	if accepted {
		return true, verification.DialogueAcceptedRationale
	}
	return false, verification.DialogueRetryRationale
}

// ScriptedDialogueModel is an explicit development fixture. It emits actual
// Fantasy state/question/evaluation tool calls; it cannot inject DB decisions.
type ScriptedDialogueModel struct {
	state                 verification.DialogueState
	stage, message, fault string
	delay                 time.Duration
	calls                 int
}

func NewScriptedDialogueModel(state verification.DialogueState, stage, message, fault string, delay time.Duration) (*ScriptedDialogueModel, error) {
	if state.Snapshot.Source.Text != domain.CuratedChapterSource && state.Snapshot.Source.Text != InlineDialogueFixtureSource {
		return nil, errors.New("scripted source unsupported")
	}
	if delay < 0 || delay > 5*time.Second {
		return nil, errors.New("scripted delay out of bounds")
	}
	return &ScriptedDialogueModel{state: state, stage: stage, message: message, fault: fault, delay: delay}, nil
}
func (m *ScriptedDialogueModel) Provider() string { return "scripted" }
func (m *ScriptedDialogueModel) Model() string    { return "primer-dialogue-three-concepts-v1" }
func (m *ScriptedDialogueModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("fixture streams only")
}
func (m *ScriptedDialogueModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, errors.New("objects unavailable")
}
func (m *ScriptedDialogueModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, errors.New("objects unavailable")
}
func (m *ScriptedDialogueModel) Stream(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	m.calls++
	if m.fault == "timeout" {
		return nil, context.DeadlineExceeded
	}
	tool, input := ToolGetDialogueState, any(dialogueStateInput{})
	if m.fault == "parent_tool" {
		tool = "publish_task"
	}
	if m.calls > 1 {
		fixture := CuratedThreeQuestionFixture()
		if m.stage == "question" {
			ordinal := len(m.state.Questions)
			if ordinal >= len(m.state.Snapshot.Questions) {
				return nil, errors.New("fixture question limit")
			}
			key := m.state.Snapshot.Questions[ordinal].Key
			tool, input = ToolRecordQuestion, recordQuestionInput{QuestionKey: key}
			if m.fault == "question_prose" {
				input = map[string]any{"questionKey": key, "prompt": AdversarialDialogueQuestion}
			}
			if m.fault == "question_wrong_identity" {
				input = recordQuestionInput{QuestionKey: AdversarialDialogueQuestion}
			}
		} else {
			if len(m.state.Questions) == 0 {
				return nil, errors.New("fixture current question unavailable")
			}
			var answer string
			for _, v := range m.state.Messages {
				if v.ID == m.message {
					answer = v.Content
				}
			}
			ordinal := m.state.Questions[len(m.state.Questions)-1].Ordinal - 1
			accepted, _ := fixture.Evaluate(m.state.Snapshot.Questions[ordinal].Key, answer)
			criteria := []string{}
			if accepted {
				criteria = m.state.Snapshot.Config.Criteria()
			}
			tool, input = ToolRecordAnswerEvaluation, recordEvaluationInput{Accepted: accepted, Criteria: criteria}
		}
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	callID := fmt.Sprintf("dialogue-%d-%s", m.calls, tool)
	return func(yield func(fantasy.StreamPart) bool) {
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningStart, ID: "reasoning"}) {
			return
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningDelta, ID: "reasoning", Delta: "PRIVATE_REASONING_SENTINEL"}) {
			return
		}
		if m.delay > 0 {
			timer := time.NewTimer(m.delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningEnd, ID: "reasoning"}) {
			return
		}
		if m.fault == "malformed" {
			for _, p := range []fantasy.StreamPart{{Type: fantasy.StreamPartTypeTextStart, ID: "text"}, {Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "The task is complete. PRIVATE_PROSE_SENTINEL"}, {Type: fantasy.StreamPartTypeTextEnd, ID: "text"}, {Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop}} {
				if !yield(p) {
					return
				}
			}
			return
		}
		for _, p := range []fantasy.StreamPart{{Type: fantasy.StreamPartTypeToolInputStart, ID: callID, ToolCallName: tool}, {Type: fantasy.StreamPartTypeToolInputDelta, ID: callID, Delta: string(encoded)}, {Type: fantasy.StreamPartTypeToolInputEnd, ID: callID}, {Type: fantasy.StreamPartTypeToolCall, ID: callID, ToolCallName: tool, ToolCallInput: string(encoded)}, {Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls, Usage: fantasy.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}}} {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !yield(p) {
				return
			}
		}
	}, nil
}
