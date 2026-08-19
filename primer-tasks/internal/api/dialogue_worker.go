package api

import (
	"charm.land/fantasy"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"primer-tasks/internal/agent"
	agentprotocol "primer-tasks/internal/agent/protocol"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
	"strings"
	"time"
)

// StartDialogueWorker owns evaluation after a websocket disconnects.
func (s *Server) StartDialogueWorker(ctx context.Context) { go s.runDialogueWorker(ctx) }
func (s *Server) runDialogueWorker(ctx context.Context) {
	q := jobs.NewPostgresRepository(s.DB)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_ = s.claimAndRunDialogue(ctx, q)
		}
	}
}
func (s *Server) claimAndRunDialogue(ctx context.Context, q *jobs.PostgresRepository) error {
	j, ok, err := q.ClaimDialogue(ctx, "dialogue-"+uuid.NewString(), 30*time.Second)
	if err != nil || !ok {
		return err
	}
	if err = s.runDialogueJob(ctx, j); err != nil {
		return q.FailDialogue(ctx, j.ID, j.LeaseOwner, err)
	}
	return q.CompleteDialogue(ctx, j.ID, j.LeaseOwner)
}

// runDialogueJob gives Fantasy only the three server-scoped dialogue tools.
// Completion is inferred solely from durable evaluation evidence.
func (s *Server) runDialogueJob(ctx context.Context, j jobs.DialogueJob) error {
	if j.TenantID == "" || j.AttemptID == "" || j.MessageID == "" {
		return errors.New("invalid dialogue job")
	}
	var student, occurrence, requirement string
	if err := s.DB.QueryRow(ctx, `SELECT o.student_id::text,a.occurrence_id::text,a.requirement_id::text FROM verification_attempts a JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=a.occurrence_id WHERE a.tenant_id=$1 AND a.id=$2`, j.TenantID, j.AttemptID).Scan(&student, &occurrence, &requirement); err != nil {
		return err
	}
	scope := verification.DialogueContext{TenantID: j.TenantID, StudentID: student, OccurrenceID: occurrence, RequirementID: requirement, AttemptID: j.AttemptID, PolicyVersion: "dialogue.v1", MessageID: j.MessageID}
	r := repo.NewDialogueRepository(s.DB)
	state, err := r.GetDialogueState(ctx, scope)
	if err != nil {
		return err
	}
	if evaluationForMessage(state, j.MessageID) != nil {
		return s.publishDialogueResult(ctx, scope, state)
	}
	cfg, err := parent.LoadProviderConfig()
	if err != nil {
		return err
	}
	answer := studentAnswer(state, j.MessageID)
	var model fantasy.LanguageModel
	if cfg.Mode == parent.ProviderScripted {
		model = &scriptedDialogueModel{answer: answer, questionKey: nextDialogueKey(state)}
	} else if cfg.Mode == parent.ProviderDisabled {
		return agent.ErrProviderDisabled
	} else {
		model, err = s.agentModel(ctx, cfg, answer)
		if err != nil {
			return err
		}
	}
	tools, err := agent.NewDialogueTools(r, scope)
	if err != nil {
		return err
	}
	rt, err := agent.NewFantasyAgent(model, tools, agent.Limits{MaxSteps: cfg.MaxSteps, MaxTokens: cfg.MaxTokens, Deadline: cfg.MaxDuration, MaxRetries: cfg.MaxRetries})
	if err != nil {
		return err
	}
	_, runErr := rt.Execute(ctx, j.AttemptID, dialoguePrompt(state, answer), func(agentprotocol.Event) error { return nil })
	state, err = r.GetDialogueState(ctx, scope)
	if err != nil {
		return err
	}
	if evaluationForMessage(state, j.MessageID) == nil {
		if runErr != nil {
			return runErr
		}
		return errors.New("dialogue provider returned no evaluation")
	}
	return s.publishDialogueResult(ctx, scope, state)
}
func evaluationForMessage(s verification.DialogueState, id string) *verification.DialogueEvaluation {
	for i := range s.Evaluations {
		if s.Evaluations[i].MessageID == id {
			return &s.Evaluations[i]
		}
	}
	return nil
}
func studentAnswer(s verification.DialogueState, id string) string {
	for _, m := range s.Messages {
		if m.ID == id && m.Role == "student" {
			return m.Content
		}
	}
	return ""
}
func nextDialogueKey(s verification.DialogueState) string {
	f := agent.CuratedThreeQuestionFixture()
	for i := len(s.Evaluations) - 1; i >= 0; i-- {
		if s.Evaluations[i].Accepted {
			break
		}
		for _, q := range s.Questions {
			if q.ID == s.Evaluations[i].QuestionID {
				return q.QuestionKey
			}
		}
	}
	for _, q := range f.Questions {
		seen := false
		for _, old := range s.Questions {
			if old.QuestionKey == q.Key {
				seen = true
				break
			}
		}
		if !seen {
			return q.Key
		}
	}
	return f.Questions[len(f.Questions)-1].Key
}
func dialoguePrompt(s verification.DialogueState, answer string) string {
	b, _ := json.Marshal(s.Config)
	return "Bounded reading verifier; server-owned source/rubric only; use dialogue tools and never mark completion directly.\nSERVER_POLICY=" + string(b) + "\nSTUDENT_ANSWER_BEGIN\n" + answer + "\nSTUDENT_ANSWER_END"
}
func (s *Server) publishDialogueResult(ctx context.Context, scope verification.DialogueContext, state verification.DialogueState) error {
	id, err := uuid.Parse(scope.StudentID)
	if err != nil {
		return err
	}
	b, err := s.bindStudentAttempt(ctx, studentIdentity{StudentID: id, TenantID: scope.TenantID}, scope.OccurrenceID, scope.AttemptID)
	if err != nil {
		return err
	}
	if state.Terminal {
		return s.persistStudentEvent(ctx, b, wireStudentEvent{Type: "completed", Status: "accepted", AcceptedCount: state.AcceptedCount, RequiredCount: state.Config.RequiredQuestions})
	}
	if e := evaluationForMessage(state, scope.MessageID); e != nil && !e.Accepted {
		return s.persistStudentEvent(ctx, b, wireStudentEvent{Type: "follow_up", Status: "retry", Text: "Please try again with a specific detail or reason.", AcceptedCount: state.AcceptedCount, RequiredCount: state.Config.RequiredQuestions})
	}
	return s.persistStudentEvent(ctx, b, wireStudentEvent{Type: "state", Status: "open", AcceptedCount: state.AcceptedCount, RequiredCount: state.Config.RequiredQuestions})
}

type scriptedDialogueModel struct {
	answer, questionKey string
	calls               int
}

func (m *scriptedDialogueModel) Provider() string { return "scripted" }
func (m *scriptedDialogueModel) Model() string    { return "primer-dialogue-fixture" }
func (m *scriptedDialogueModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("scripted dialogue model only streams")
}
func (m *scriptedDialogueModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, errors.New("scripted dialogue objects disabled")
}
func (m *scriptedDialogueModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, errors.New("scripted dialogue objects disabled")
}
func (m *scriptedDialogueModel) Stream(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	m.calls++
	f := agent.CuratedThreeQuestionFixture()
	if m.calls == 1 && m.questionKey != "" {
		p := ""
		for _, q := range f.Questions {
			if q.Key == m.questionKey {
				p = q.Prompt
			}
		}
		return scriptedToolStream(ctx, agent.ToolRecordQuestion, fmt.Sprintf(`{"questionKey":%q,"prompt":%q}`, m.questionKey, p)), nil
	}
	ok, r := f.Evaluate(m.questionKey, m.answer)
	return scriptedToolStream(ctx, agent.ToolRecordAnswerEvaluation, fmt.Sprintf(`{"questionKey":%q,"accepted":%t,"criteria":["answers address the distinct question"],"rationale":%q}`, m.questionKey, ok, strings.TrimSpace(r))), nil
}
