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
	"primer-tasks/internal/domain"
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
	if _, err := dialogueQuestions(state.Config); err != nil {
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
		model = &scriptedDialogueModel{answer: answer, questionKey: nextDialogueKey(state), config: state.Config}
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

type dialogueQuestionSpec struct {
	Key, Prompt string
	Keywords    []string
}

var errDialogueSourceUnsupported = errors.New("dialogue source is unsupported")

const stacklaneChapterSource = "The family repaired the garden wall after the storm. The mortar must dry before the next course, or rushing will weaken the wall."

func dialogueQuestions(config domain.DialogueConfig) ([]dialogueQuestionSpec, error) {
	if config.SourceRef == "fixture://chapter-4" {
		f := agent.CuratedThreeQuestionFixture()
		out := make([]dialogueQuestionSpec, 0, len(f.Questions))
		for _, q := range f.Questions {
			out = append(out, dialogueQuestionSpec{Key: q.Key, Prompt: q.Prompt, Keywords: q.Keywords})
		}
		return out, nil
	}
	// The source text fixture is supported only by exact identity. An arbitrary
	// parent source must never silently select facts from another chapter.
	if config.SourceRef == "" && config.SourceText == stacklaneChapterSource {
		return []dialogueQuestionSpec{
			{Key: "wall", Prompt: "What did the family repair after the storm?", Keywords: []string{"wall", "garden"}},
			{Key: "mortar", Prompt: "Why must the mortar dry before the next course of stones?", Keywords: []string{"mortar", "dry"}},
			{Key: "rushing", Prompt: "What would rushing the work do to the wall?", Keywords: []string{"rushing", "weaken"}},
		}, nil
	}
	return nil, fmt.Errorf("%w: %q", errDialogueSourceUnsupported, config.SourceRef)
}

func dialogueStarterPrompt(raw []byte) (string, error) {
	var config domain.DialogueConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", errDialogueSourceUnsupported
	}
	questions, err := dialogueQuestions(config)
	if err != nil || len(questions) == 0 {
		return "", err
	}
	return questions[0].Prompt, nil
}

func nextDialogueKey(s verification.DialogueState) string {
	f, err := dialogueQuestions(s.Config)
	if err != nil || len(f) == 0 {
		return ""
	}
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
	for _, q := range f {
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
	return f[len(f)-1].Key
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
		status := state.TerminalStatus
		if status == "" {
			// Unit-created states predate TerminalStatus; do not turn a short
			// terminal state into a false acceptance.
			status = "rejected"
			if state.AcceptedCount >= state.Config.RequiredQuestions {
				status = "accepted"
			}
		}
		return s.persistStudentEvent(ctx, b, wireStudentEvent{Type: "complete", Phase: "complete", Status: status, AcceptedCount: state.AcceptedCount, RequiredCount: state.Config.RequiredQuestions})
	}
	if e := evaluationForMessage(state, scope.MessageID); e != nil && !e.Accepted {
		return s.persistStudentEvent(ctx, b, wireStudentEvent{Type: "progress", Phase: "retry", Status: "rejected", Text: "Please try again with a specific detail or reason.", AcceptedCount: state.AcceptedCount, RequiredCount: state.Config.RequiredQuestions})
	}
	if len(state.Questions) > 0 {
		q := state.Questions[len(state.Questions)-1]
		return s.persistStudentEvent(ctx, b, wireStudentEvent{Type: "question", QuestionKey: q.QuestionKey, Text: q.Prompt, AcceptedCount: state.AcceptedCount, RequiredCount: state.Config.RequiredQuestions})
	}
	return s.persistStudentEvent(ctx, b, wireStudentEvent{Type: "state", Status: "open", AcceptedCount: state.AcceptedCount, RequiredCount: state.Config.RequiredQuestions})
}

type scriptedDialogueModel struct {
	answer, questionKey string
	config              domain.DialogueConfig
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
	config := m.config
	if config.SourceRef == "" && config.SourceText == "" {
		// Keep direct unit construction deterministic; production always binds
		// the immutable attempt snapshot above.
		config = agent.CuratedThreeQuestionFixture().Config()
	}
	f, err := dialogueQuestions(config)
	if err != nil {
		return nil, err
	}
	if m.calls == 1 && m.questionKey != "" {
		p := ""
		for _, q := range f {
			if q.Key == m.questionKey {
				p = q.Prompt
			}
		}
		return scriptedToolStream(ctx, agent.ToolRecordQuestion, fmt.Sprintf(`{"questionKey":%q,"prompt":%q}`, m.questionKey, p)), nil
	}
	answer := strings.ToLower(strings.TrimSpace(m.answer))
	accepted := false
	for _, q := range f {
		if q.Key == m.questionKey {
			for _, keyword := range q.Keywords {
				if strings.Contains(answer, keyword) {
					accepted = true
					break
				}
			}
		}
	}
	rationale := "answer needs a specific detail from the parent-authored source"
	if accepted {
		rationale = "answer addresses a distinct source fact"
	}
	criteria := []string{}
	if accepted {
		criteria = []string{"answers address the distinct question"}
	}
	criteriaJSON, _ := json.Marshal(criteria)
	return scriptedToolStream(ctx, agent.ToolRecordAnswerEvaluation, fmt.Sprintf(`{"questionKey":%q,"accepted":%t,"criteria":%s,"rationale":%q}`, m.questionKey, accepted, criteriaJSON, rationale)), nil
}
