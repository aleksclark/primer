package verification

// Original P4 authority helpers, adapted so questions AND messages are distinct
// bound evidence. These rules are shared by the durable engine, not a second
// client/model completion path.
import (
	"errors"
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
	ErrDialogueConflict    = errors.New("dialogue state changed")
)

type DialogueContext struct {
	TenantID, StudentID, OccurrenceID, RequirementID, AttemptID string
	PolicyVersion, SnapshotDigest                               string
}

func (c DialogueContext) Validate() error {
	for _, value := range []string{c.TenantID, c.StudentID, c.OccurrenceID, c.RequirementID, c.AttemptID, c.SnapshotDigest} {
		if strings.TrimSpace(value) == "" {
			return ErrDialogueContext
		}
	}
	if c.PolicyVersion != domain.DialoguePolicyVersion {
		return ErrDialogueContext
	}
	return nil
}

type DialogueState struct {
	Context        DialogueContext
	Snapshot       domain.DialogueSnapshot
	Version        int64
	Terminal       bool
	TerminalStatus string
	Questions      []DialogueQuestion
	Messages       []DialogueMessage
	Evaluations    []DialogueEvaluation
}

type DialogueQuestion struct {
	ID, AttemptID, QuestionKey, Prompt string
	Ordinal                            int
	Version                            int64
	CreatedAt                          time.Time
}

type DialogueMessage struct {
	ID, AttemptID, QuestionID, PolicyVersion, SnapshotDigest string
	Role, Content, ClientMessageID                           string
	Sequence, ExpectedVersion                                int64
	CreatedAt                                                time.Time
}

type DialogueEvaluation struct {
	ID, AttemptID, QuestionID, MessageID string
	PolicyVersion, SnapshotDigest        string
	Accepted                             bool
	Criteria                             []string
	Rationale, Provider, Model           string
	InputTokens, OutputTokens            int64
	Version                              int64
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
	if err := s.Snapshot.Validate(); err != nil {
		return err
	}
	if s.Version < 1 || s.Snapshot.RequirementID != s.Context.RequirementID || s.Snapshot.Digest != s.Context.SnapshotDigest || s.Snapshot.PolicyVersion != s.Context.PolicyVersion {
		return ErrDialogueContext
	}
	return nil
}

// Short server-owned rationale vocabulary: arbitrary provider prose, even if
// it evades a keyword blacklist, is never persisted as an evaluation rationale.
const (
	DialogueAcceptedRationale = "Answer addresses the current question using the assigned source."
	DialogueRetryRationale    = "Add a specific detail or reason from the assigned source."
)

func SafeRationale(raw string) (string, error) {
	if raw != DialogueAcceptedRationale && raw != DialogueRetryRationale {
		return "", ErrDialogueEvaluation
	}
	return raw, nil
}

// DialogueEvidence checks persisted evidence without trusting a mutable count.
// Each answer may evaluate exactly one question; each distinct question counts
// once. Every row must match the snapshotted policy and the durable message.
func DialogueEvidence(s DialogueState) (DecisionReady, error) {
	ready := DecisionReady{RequiredCount: domain.DialogueRequiredQuestions}
	if err := s.Validate(); err != nil {
		return ready, err
	}
	questions := map[string]DialogueQuestion{}
	keys, prompts := map[string]bool{}, map[string]bool{}
	for i, q := range s.Questions {
		prompt := strings.ToLower(strings.TrimSpace(q.Prompt))
		if q.ID == "" || q.AttemptID != s.Context.AttemptID || q.Ordinal != i+1 || q.Ordinal > ready.RequiredCount || q.Version < 1 || q.Version > s.Version || strings.TrimSpace(q.QuestionKey) == "" || prompt == "" || len([]rune(q.Prompt)) > 2000 {
			return ready, ErrDialogueQuestion
		}
		if _, exists := questions[q.ID]; exists || keys[q.QuestionKey] || prompts[prompt] {
			return ready, ErrDuplicateQuestion
		}
		questions[q.ID], keys[q.QuestionKey], prompts[prompt] = q, true, true
	}
	messages := map[string]DialogueMessage{}
	clientIDs := map[string]bool{}
	for i, m := range s.Messages {
		q, exists := questions[m.QuestionID]
		if !exists || m.ID == "" || m.AttemptID != s.Context.AttemptID || m.PolicyVersion != s.Context.PolicyVersion || m.SnapshotDigest != s.Context.SnapshotDigest || m.Role != "student" || strings.TrimSpace(m.Content) == "" || len(m.Content) > 12000 || m.ClientMessageID == "" || len(m.ClientMessageID) > 128 || m.Sequence != int64(i+1) || m.ExpectedVersion < q.Version || m.ExpectedVersion > s.Version {
			return ready, ErrDialogueEvaluation
		}
		if _, duplicate := messages[m.ID]; duplicate || clientIDs[m.ClientMessageID] {
			return ready, ErrDialogueEvaluation
		}
		messages[m.ID], clientIDs[m.ClientMessageID] = m, true
	}
	accepted, evaluated := map[string]bool{}, map[string]bool{}
	acceptedVersions := map[string]int64{}
	for _, e := range s.Evaluations {
		m, found := messages[e.MessageID]
		if !found || e.ID == "" || e.AttemptID != s.Context.AttemptID || e.QuestionID != m.QuestionID || e.PolicyVersion != s.Context.PolicyVersion || e.SnapshotDigest != s.Context.SnapshotDigest || e.Provider == "" || e.Model == "" || len(e.Provider) > 128 || len(e.Model) > 128 || e.InputTokens < 0 || e.OutputTokens < 0 || e.Version <= m.ExpectedVersion+1 || e.Version > s.Version {
			return ready, ErrDialogueEvaluation
		}
		if evaluated[e.MessageID] || accepted[e.QuestionID] {
			return ready, ErrDuplicateEvaluation
		}
		if (e.Accepted && e.Rationale != DialogueAcceptedRationale) || (!e.Accepted && e.Rationale != DialogueRetryRationale) {
			return ready, ErrDialogueEvaluation
		}
		if e.Accepted && len(e.Criteria) == 0 {
			return ready, ErrDialogueEvaluation
		}
		seen := map[string]bool{}
		for _, criterion := range e.Criteria {
			known := false
			for _, allowed := range s.Snapshot.Config.Rubric {
				known = known || criterion == allowed
			}
			if !known || seen[criterion] {
				return ready, ErrDialogueEvaluation
			}
			seen[criterion] = true
		}
		evaluated[e.MessageID] = true
		if e.Accepted {
			accepted[e.QuestionID] = true
			acceptedVersions[e.QuestionID] = e.Version
		}
	}
	// Later questions must never be issued before all preceding questions are
	// accepted. Pending/rejected answers to the current question do not advance.
	for i, q := range s.Questions[:max(0, len(s.Questions)-1)] {
		if !accepted[q.ID] || acceptedVersions[q.ID] >= s.Questions[i+1].Version {
			return ready, ErrDialogueQuestion
		}
	}
	ready.AcceptedCount = len(accepted)
	ready.Accepted = ready.AcceptedCount == ready.RequiredCount
	if ready.Accepted {
		ready.Reason = "required distinct dialogue questions accepted"
	}
	return ready, nil
}

func RecordQuestion(s DialogueState, q DialogueQuestion) (DialogueState, error) {
	ready, err := DialogueEvidence(s)
	if err != nil {
		return s, err
	}
	if s.Terminal || ready.Accepted {
		return s, ErrDialogueTerminal
	}
	if len(s.Questions) != ready.AcceptedCount || q.Ordinal != len(s.Questions)+1 || q.AttemptID != s.Context.AttemptID || q.Version != s.Version+1 {
		return s, ErrDialogueQuestion
	}
	next := s
	next.Version++
	next.Questions = append(append([]DialogueQuestion(nil), s.Questions...), q)
	if _, err = DialogueEvidence(next); err != nil {
		return s, err
	}
	return next, nil
}

func RecordMessage(s DialogueState, m DialogueMessage) (DialogueState, error) {
	ready, err := DialogueEvidence(s)
	if err != nil {
		return s, err
	}
	if s.Terminal || ready.Accepted {
		return s, ErrDialogueTerminal
	}
	if m.ExpectedVersion != s.Version || len(s.Questions) == 0 || ready.AcceptedCount == len(s.Questions) || m.QuestionID != s.Questions[len(s.Questions)-1].ID {
		return s, ErrDialogueConflict
	}
	answers, evaluations := 0, 0
	for _, previous := range s.Messages {
		if previous.QuestionID == m.QuestionID {
			answers++
		}
	}
	for _, previous := range s.Evaluations {
		if previous.QuestionID == m.QuestionID {
			evaluations++
		}
	}
	if answers != evaluations {
		return s, ErrDialogueConflict
	}
	if answers >= 1+s.Snapshot.Config.AllowedFollowUps || len(s.Messages) >= s.Snapshot.Config.MaxTurns {
		return s, ErrDialogueLimit
	}
	next := s
	next.Version++
	next.Messages = append(append([]DialogueMessage(nil), s.Messages...), m)
	if _, err = DialogueEvidence(next); err != nil {
		return s, err
	}
	return next, nil
}

func RecordEvaluation(s DialogueState, e DialogueEvaluation) (DialogueState, DecisionReady, error) {
	ready, err := DialogueEvidence(s)
	if err != nil {
		return s, ready, err
	}
	if s.Terminal || ready.Accepted {
		return s, ready, ErrDialogueTerminal
	}
	if len(s.Questions) == 0 || len(s.Messages) == 0 || e.Version != s.Version+1 || e.QuestionID != s.Questions[len(s.Questions)-1].ID || e.MessageID != s.Messages[len(s.Messages)-1].ID {
		return s, ready, ErrDialogueEvaluation
	}
	next := s
	next.Version++
	e.Criteria = append([]string(nil), e.Criteria...)
	next.Evaluations = append(append([]DialogueEvaluation(nil), s.Evaluations...), e)
	ready, err = DialogueEvidence(next)
	if err != nil {
		return s, ready, err
	}
	return next, ready, nil
}
