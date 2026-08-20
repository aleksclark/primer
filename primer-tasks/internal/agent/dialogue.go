package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"primer-tasks/internal/verification"
)

const (
	ToolGetDialogueState       = "get_dialogue_state"
	ToolRecordQuestion         = "record_question"
	ToolRecordAnswerEvaluation = "record_answer_evaluation"
)

var ErrDialogueToolContext = errors.New("dialogue tool context is invalid")

// DialogueBackend is the persistence seam for the three student tools. It is
// deliberately narrower than agent.Repository and has no task-management or
// completion operation.
type DialogueBackend interface {
	GetDialogueState(context.Context, verification.DialogueContext) (verification.DialogueState, error)
	RecordQuestion(context.Context, verification.DialogueContext, verification.DialogueQuestion) (verification.DialogueQuestion, bool, error)
	RecordAnswerEvaluation(context.Context, verification.DialogueContext, string, verification.DialogueEvaluation) (verification.DialogueEvaluation, verification.DecisionReady, bool, error)
}

// DialogueTools contains server-bound context. A caller must construct it
// after authenticating the student and durable message; model arguments never
// replace these IDs.
type DialogueTools struct {
	Backend DialogueBackend
	Scope   verification.DialogueContext
}

func (t DialogueTools) validate(read bool) error {
	if err := t.Scope.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrDialogueToolContext, err)
	}
	if t.Backend == nil {
		return ErrDialogueToolContext
	}
	if !read && strings.TrimSpace(t.Scope.MessageID) == "" {
		return fmt.Errorf("%w: student message is required", ErrDialogueToolContext)
	}
	return nil
}

type dialogueStateInput struct{}
type recordQuestionInput struct {
	QuestionKey string `json:"questionKey" jsonschema:"description=server-stable question key"`
	Prompt      string `json:"prompt" jsonschema:"description=the next concise question"`
}
type recordEvaluationInput struct {
	QuestionKey string   `json:"questionKey" jsonschema:"description=the server-stable question key"`
	Accepted    bool     `json:"accepted"`
	Criteria    []string `json:"criteria" jsonschema:"description=parent-authored rubric criteria addressed"`
	Rationale   string   `json:"rationale" jsonschema:"description=short safe feedback only"`
}

type dialogueStateOutput struct {
	SourceRef     string                     `json:"sourceRef,omitempty"`
	SourceText    string                     `json:"sourceText,omitempty"`
	LearningFocus string                     `json:"learningFocus"`
	Rubric        []string                   `json:"rubric"`
	AcceptedCount int                        `json:"acceptedCount"`
	RequiredCount int                        `json:"requiredCount"`
	TurnCount     int                        `json:"turnCount"`
	Terminal      bool                       `json:"terminal"`
	Questions     []dialogueQuestionOutput   `json:"questions"`
	Evaluations   []dialogueEvaluationOutput `json:"evaluations"`
	History       []dialogueMessageOutput    `json:"history"`
}
type dialogueQuestionOutput struct {
	ID          string `json:"id"`
	QuestionKey string `json:"questionKey"`
	Prompt      string `json:"prompt"`
	Ordinal     int    `json:"ordinal"`
}
type dialogueEvaluationOutput struct {
	QuestionID string `json:"questionId"`
	MessageID  string `json:"messageId"`
	Accepted   bool   `json:"accepted"`
	Rationale  string `json:"rationale"`
}
type dialogueMessageOutput struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Content  string `json:"content"`
	Sequence int64  `json:"sequence"`
}

func DialogueToolNames() []string {
	return []string{ToolGetDialogueState, ToolRecordQuestion, ToolRecordAnswerEvaluation}
}

// NewDialogueTools returns exactly the three tools allowed in a student
// dialogue. No generic agent tools, completion tool, or repository handle is
// exposed to Fantasy.
func NewDialogueTools(backend DialogueBackend, scope verification.DialogueContext) ([]fantasy.AgentTool, error) {
	t := DialogueTools{Backend: backend, Scope: scope}
	if err := t.validate(true); err != nil {
		return nil, err
	}
	return []fantasy.AgentTool{
		fantasy.NewAgentTool[dialogueStateInput](ToolGetDialogueState, "Read the current verification state for this attempt.", t.getState),
		fantasy.NewAgentTool[recordQuestionInput](ToolRecordQuestion, "Record one distinct, concise question for this attempt.", t.recordQuestion),
		fantasy.NewAgentTool[recordEvaluationInput](ToolRecordAnswerEvaluation, "Record the server-bound evaluation of the current student answer.", t.recordEvaluation),
	}, nil
}

func (t DialogueTools) getState(ctx context.Context, _ dialogueStateInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if err := t.validate(true); err != nil {
		return fantasy.NewTextErrorResponse("dialogue state is unavailable"), nil
	}
	state, err := t.Backend.GetDialogueState(ctx, t.Scope)
	if err != nil {
		return fantasy.NewTextErrorResponse("dialogue state is unavailable"), nil
	}
	out := dialogueStateOutput{SourceRef: state.Config.SourceRef, SourceText: state.Config.SourceText, LearningFocus: state.Config.LearningFocus, Rubric: state.Config.Criteria(), AcceptedCount: state.AcceptedCount, RequiredCount: state.Config.RequiredQuestions, TurnCount: state.TurnCount, Terminal: state.Terminal}
	for _, q := range state.Questions {
		out.Questions = append(out.Questions, dialogueQuestionOutput{ID: q.ID, QuestionKey: q.QuestionKey, Prompt: q.Prompt, Ordinal: q.Ordinal})
	}
	for _, e := range state.Evaluations {
		out.Evaluations = append(out.Evaluations, dialogueEvaluationOutput{QuestionID: e.QuestionID, MessageID: e.MessageID, Accepted: e.Accepted, Rationale: e.Rationale})
	}
	start := 0
	if len(state.Messages) > 100 {
		start = len(state.Messages) - 100
	}
	for _, m := range state.Messages[start:] {
		out.History = append(out.History, dialogueMessageOutput{ID: m.ID, Role: m.Role, Content: m.Content, Sequence: m.Sequence})
	}
	b, _ := json.Marshal(out)
	return fantasy.NewTextResponse(string(b)), nil
}

var answerLeak = regexp.MustCompile(`(?i)(answer\s*key|correct\s+answer|model\s+answer|expected\s+answer|solution\s*:).*`)

func safeQuestionPrompt(prompt string) (string, error) {
	p := strings.TrimSpace(prompt)
	if p == "" || len([]rune(p)) > 2000 {
		return "", verification.ErrDialogueQuestion
	}
	// The model cannot persist a key separately, but it can accidentally emit
	// one in a prompt. Drop the compromised suffix and fail closed if nothing
	// useful remains.
	p = answerLeak.ReplaceAllString(p, "")
	p = strings.TrimSpace(p)
	if p == "" {
		return "", verification.ErrDialogueQuestion
	}
	return p, nil
}

func (t DialogueTools) recordQuestion(ctx context.Context, in recordQuestionInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if err := t.validate(false); err != nil {
		return fantasy.NewTextErrorResponse("question recording is unavailable"), nil
	}
	prompt, err := safeQuestionPrompt(in.Prompt)
	if err != nil || strings.TrimSpace(in.QuestionKey) == "" {
		return fantasy.NewTextErrorResponse("question is invalid"), nil
	}
	state, err := t.Backend.GetDialogueState(ctx, t.Scope)
	if err != nil {
		return fantasy.NewTextErrorResponse("dialogue state is unavailable"), nil
	}
	q := verification.DialogueQuestion{ID: uuid.NewString(), QuestionKey: strings.TrimSpace(in.QuestionKey), Prompt: prompt, Ordinal: len(state.Questions) + 1}
	q, _, err = t.Backend.RecordQuestion(ctx, t.Scope, q)
	if err != nil {
		return fantasy.NewTextErrorResponse("question was not recorded"), nil
	}
	b, _ := json.Marshal(map[string]any{"questionId": q.ID, "questionKey": q.QuestionKey, "ordinal": q.Ordinal, "prompt": q.Prompt})
	return fantasy.NewTextResponse(string(b)), nil
}

func (t DialogueTools) recordEvaluation(ctx context.Context, in recordEvaluationInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if err := t.validate(false); err != nil {
		return fantasy.NewTextErrorResponse("evaluation is unavailable"), nil
	}
	if strings.TrimSpace(in.QuestionKey) == "" {
		return fantasy.NewTextErrorResponse("evaluation is invalid"), nil
	}
	rationale, err := verification.SafeRationale(in.Rationale)
	if err != nil {
		return fantasy.NewTextErrorResponse("evaluation is invalid"), nil
	}
	// Provenance is fixed by the worker's authenticated server scope; the
	// model can propose only the evaluation fields above.
	e := verification.DialogueEvaluation{ID: uuid.NewString(), MessageID: t.Scope.MessageID, Accepted: in.Accepted, Criteria: in.Criteria, Rationale: rationale, Provider: t.Scope.Provider, Model: t.Scope.Model, PolicyVersion: t.Scope.PolicyVersion}
	_, ready, _, err := t.Backend.RecordAnswerEvaluation(ctx, t.Scope, strings.TrimSpace(in.QuestionKey), e)
	if err != nil {
		return fantasy.NewTextErrorResponse("evaluation was not recorded"), nil
	}
	b, _ := json.Marshal(map[string]any{"accepted": in.Accepted, "acceptedCount": ready.AcceptedCount, "requiredCount": ready.RequiredCount, "decisionReady": ready.Accepted})
	return fantasy.NewTextResponse(string(b)), nil
}
