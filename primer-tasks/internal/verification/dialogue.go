package verification

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"primer-tasks/internal/domain"
)

var (
	ErrDialogueContext     = errors.New("invalid dialogue server context")
	ErrDialogueQuestion    = errors.New("invalid dialogue question")
	ErrDialogueEvaluation  = errors.New("invalid dialogue evaluation")
	ErrDialogueTerminal    = errors.New("dialogue attempt is terminal")
	ErrDuplicateQuestion   = errors.New("dialogue question is not distinct")
	ErrDuplicateEvaluation = errors.New("dialogue evaluation is already recorded")
	ErrDialogueLimit       = errors.New("dialogue policy limit reached")
)

// Context is constructed by an authenticated server adapter. None of these
// values are accepted as model/tool arguments.
type DialogueContext struct {
	TenantID      string
	StudentID     string
	OccurrenceID  string
	RequirementID string
	AttemptID     string
	PolicyVersion string
	MessageID     string // the durable student message currently being evaluated
}

func (c DialogueContext) Validate() error {
	for name, value := range map[string]string{
		"tenant": c.TenantID, "student": c.StudentID, "occurrence": c.OccurrenceID,
		"requirement": c.RequirementID, "attempt": c.AttemptID, "policy": c.PolicyVersion,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s is required", ErrDialogueContext, name)
		}
	}
	return nil
}

type DialogueState struct {
	Context       DialogueContext
	Config        domain.DialogueConfig
	AcceptedCount int
	TurnCount     int
	Terminal      bool
	Questions     []DialogueQuestion
	Evaluations   []DialogueEvaluation
	Messages      []DialogueMessage
}

type DialogueMessage struct {
	ID, Role, Content string
	Sequence          int64
	CreatedAt         time.Time
}

type DialogueQuestion struct {
	ID, AttemptID, QuestionKey, Prompt string
	Ordinal                            int
	CreatedAt                          time.Time
}

type DialogueEvaluation struct {
	ID, AttemptID, QuestionID, MessageID string
	Accepted                             bool
	Criteria                             []string
	Rationale                            string
	Provider, Model, PolicyVersion       string
	Usage                                map[string]any
	CreatedAt                            time.Time
}

type DecisionReady struct {
	Accepted      bool
	AcceptedCount int
	RequiredCount int
	Reason        string
}

func (s DialogueState) Validate() error {
	if err := s.Context.Validate(); err != nil {
		return err
	}
	if err := s.Config.Validate(); err != nil {
		return err
	}
	if s.AcceptedCount < 0 || s.AcceptedCount > s.Config.RequiredQuestions || s.TurnCount < 0 {
		return ErrDialogueLimit
	}
	return nil
}

// RecordQuestion applies the generic authority rules before persistence. A
// question key can occur once per attempt and only the engine may count it.
func RecordQuestion(s DialogueState, q DialogueQuestion) (DialogueState, error) {
	if err := s.Validate(); err != nil {
		return s, err
	}
	if s.Terminal {
		return s, ErrDialogueTerminal
	}
	if strings.TrimSpace(q.QuestionKey) == "" || strings.TrimSpace(q.Prompt) == "" || len([]rune(q.Prompt)) > 2000 || q.Ordinal < 1 {
		return s, ErrDialogueQuestion
	}
	if s.TurnCount >= s.Config.MaxTurns {
		return s, ErrDialogueLimit
	}
	for _, old := range s.Questions {
		if old.QuestionKey == q.QuestionKey || old.Ordinal == q.Ordinal {
			return s, ErrDuplicateQuestion
		}
	}
	q.AttemptID = s.Context.AttemptID
	s.Questions = append(s.Questions, q)
	s.TurnCount++
	return s, nil
}

// SafeRationale is intentionally a short, plain-language explanation. It
// rejects likely hidden reasoning/answer-key material rather than persisting
// provider prose. The engine stores this value, never a reasoning delta.
func SafeRationale(raw string) (string, error) {
	r := strings.TrimSpace(raw)
	if len([]rune(r)) > 500 {
		return "", ErrDialogueEvaluation
	}
	lower := strings.ToLower(r)
	for _, forbidden := range []string{"chain of thought", "system prompt", "answer key", "internal reasoning", "<think>"} {
		if strings.Contains(lower, forbidden) {
			return "", ErrDialogueEvaluation
		}
	}
	return r, nil
}

var rubricToken = regexp.MustCompile(`^[[:alnum:][:space:][:punct:]]+$`)

func validateEvaluation(s DialogueState, e DialogueEvaluation) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Terminal || s.AcceptedCount >= s.Config.RequiredQuestions {
		return ErrDialogueTerminal
	}
	if strings.TrimSpace(e.QuestionID) == "" || strings.TrimSpace(e.MessageID) == "" || e.PolicyVersion != s.Context.PolicyVersion {
		return ErrDialogueEvaluation
	}
	if _, err := SafeRationale(e.Rationale); err != nil {
		return err
	}
	if e.Rationale != "" && !rubricToken.MatchString(e.Rationale) {
		return ErrDialogueEvaluation
	}
	allowedCriteria := s.Config.Criteria()
	for _, criterion := range e.Criteria {
		matched := false
		for _, allowed := range allowedCriteria {
			if strings.EqualFold(strings.TrimSpace(criterion), strings.TrimSpace(allowed)) {
				matched = true
				break
			}
		}
		if !matched {
			return ErrDialogueEvaluation
		}
	}
	found := false
	for _, q := range s.Questions {
		if q.ID == e.QuestionID {
			found = true
			break
		}
	}
	if !found {
		return ErrDialogueEvaluation
	}
	for _, old := range s.Evaluations {
		if old.QuestionID == e.QuestionID && old.MessageID == e.MessageID {
			return ErrDuplicateEvaluation
		}
	}
	return nil
}

// RecordEvaluation is the generic verification authority. A rejected answer
// remains evidence but never contributes to acceptance. A terminal result is
// only decision-ready; callers must persist it through the generic decision
// repository, not let the model or tool mutate an occurrence.
func RecordEvaluation(s DialogueState, e DialogueEvaluation) (DialogueState, DecisionReady, error) {
	if err := validateEvaluation(s, e); err != nil {
		return s, DecisionReady{}, err
	}
	e.AttemptID = s.Context.AttemptID
	s.Evaluations = append(s.Evaluations, e)
	if e.Accepted {
		s.AcceptedCount++
	}
	ready := DecisionReady{Accepted: s.AcceptedCount >= s.Config.RequiredQuestions, AcceptedCount: s.AcceptedCount, RequiredCount: s.Config.RequiredQuestions}
	if ready.Accepted {
		ready.Reason = "required distinct dialogue questions accepted"
	}
	return s, ready, nil
}
