package verification

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"primer-tasks/internal/domain"
)

func dialogueState(t *testing.T) DialogueState {
	t.Helper()
	c := domain.DialogueConfig{SourceRef: "fixture://chapter-4", LearningFocus: "three distinct source facts", RequiredQuestions: 3, Rubric: []string{"source detail"}, AllowedFollowUps: 1, MaxAttempts: 2, MaxTurns: 8, RetentionPolicy: "retain"}
	snapshot, err := domain.NewDialogueSnapshot("revision", "requirement", 1, c)
	if err != nil {
		t.Fatal(err)
	}
	return DialogueState{Context: DialogueContext{TenantID: "tenant", StudentID: "student", OccurrenceID: "occurrence", RequirementID: "requirement", AttemptID: "attempt", PolicyVersion: domain.DialoguePolicyVersion, SnapshotDigest: snapshot.Digest}, Snapshot: snapshot, Version: 1}
}

func addQuestion(t *testing.T, s DialogueState) DialogueState {
	t.Helper()
	n := len(s.Questions) + 1
	next, err := RecordQuestion(s, DialogueQuestion{ID: fmt.Sprintf("q%d", n), AttemptID: s.Context.AttemptID, QuestionKey: s.Snapshot.Questions[n-1].Key, Prompt: s.Snapshot.Questions[n-1].Prompt, Ordinal: n, Version: s.Version + 1})
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func nextMessage(s DialogueState) DialogueMessage {
	n := len(s.Messages) + 1
	return DialogueMessage{ID: fmt.Sprintf("m%d", n), AttemptID: s.Context.AttemptID, QuestionID: s.Questions[len(s.Questions)-1].ID, PolicyVersion: s.Context.PolicyVersion, SnapshotDigest: s.Context.SnapshotDigest, Role: "student", Content: "A source-based answer.", ClientMessageID: fmt.Sprintf("client%d", n), Sequence: int64(n), ExpectedVersion: s.Version}
}
func addAnswer(t *testing.T, s DialogueState) DialogueState {
	t.Helper()
	next, err := RecordMessage(s, nextMessage(s))
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func nextEvaluation(s DialogueState, accepted bool) DialogueEvaluation {
	m := s.Messages[len(s.Messages)-1]
	e := DialogueEvaluation{ID: fmt.Sprintf("e%d", len(s.Evaluations)+1), AttemptID: s.Context.AttemptID, QuestionID: m.QuestionID, MessageID: m.ID, PolicyVersion: s.Context.PolicyVersion, SnapshotDigest: s.Context.SnapshotDigest, Provider: "scripted", Model: "three-concept-fixture", Version: s.Version + 1, Accepted: accepted, Rationale: DialogueRetryRationale}
	if accepted {
		e.Rationale = DialogueAcceptedRationale
		e.Criteria = []string{"source detail"}
	}
	return e
}
func evaluate(t *testing.T, s DialogueState, accepted bool) DialogueState {
	t.Helper()
	next, _, err := RecordEvaluation(s, nextEvaluation(s, accepted))
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestDialogueContextAndStateValidationRejectIncompleteBindings(t *testing.T) {
	if err := (DialogueContext{}).Validate(); err == nil {
		t.Fatal("empty dialogue context accepted")
	}
	s := dialogueState(t)
	s.Context.PolicyVersion = "dialogue.v0"
	if err := s.Context.Validate(); err == nil {
		t.Fatal("wrong policy version accepted")
	}
	s = dialogueState(t)
	s.Version = 0
	if err := s.Validate(); err == nil {
		t.Fatal("zero version accepted")
	}
	s = dialogueState(t)
	s.Snapshot.RequirementID = "other"
	if err := s.Validate(); err == nil {
		t.Fatal("mismatched snapshot requirement accepted")
	}
}

func TestFailDialogueJobRejectsUnknownCodes(t *testing.T) {
	if err := (DialogueEngine{}).FailDialogueJob(context.Background(), DialogueJobReference{}, "not-a-code"); err == nil {
		t.Fatal("unknown failure code accepted")
	}
}

func TestDialogueAuthorityRequiresThreeDistinctBoundAnswers(t *testing.T) {
	s := dialogueState(t)
	for question := 1; question <= 3; question++ {
		s = addQuestion(t, s)
		if question == 2 {
			s = evaluate(t, addAnswer(t, s), false)
			ready, err := DialogueEvidence(s)
			if err != nil || ready.AcceptedCount != 1 || ready.Accepted {
				t.Fatalf("rejected answer counted: %+v %v", ready, err)
			}
			if _, err := RecordQuestion(s, DialogueQuestion{ID: "early", AttemptID: "attempt", QuestionKey: "early", Prompt: "Early?", Ordinal: 3, Version: s.Version + 1}); err == nil {
				t.Fatal("rejected current question advanced")
			}
		}
		s = evaluate(t, addAnswer(t, s), true)
		ready, err := DialogueEvidence(s)
		if err != nil || ready.AcceptedCount != question || ready.Accepted != (question == 3) {
			t.Fatalf("question %d readiness %+v: %v", question, ready, err)
		}
		if _, err := RecordMessage(s, nextMessage(s)); err == nil {
			t.Fatal("already accepted question received a new answer")
		}
	}
	if len(s.Messages) != 4 || len(s.Evaluations) != 4 || len(s.Questions) != 3 {
		t.Fatal("lost immutable rejected/accepted history")
	}
	if _, _, err := RecordEvaluation(s, nextEvaluation(s, true)); !errors.Is(err, ErrDialogueTerminal) {
		t.Fatal("post-completion evaluation accepted")
	}
	if _, err := RecordQuestion(s, DialogueQuestion{}); !errors.Is(err, ErrDialogueTerminal) {
		t.Fatal("post-completion question accepted")
	}
}

func TestDialogueRejectsStaleAndCrossBoundEvidence(t *testing.T) {
	s := addQuestion(t, dialogueState(t))
	for name, mutate := range map[string]func(*DialogueMessage){
		"bypass version": func(m *DialogueMessage) { m.ExpectedVersion = 0 },
		"stale version":  func(m *DialogueMessage) { m.ExpectedVersion-- },
		"question":       func(m *DialogueMessage) { m.QuestionID = "foreign" },
		"attempt":        func(m *DialogueMessage) { m.AttemptID = "foreign" },
		"policy":         func(m *DialogueMessage) { m.PolicyVersion = "dialogue.v2" },
		"snapshot":       func(m *DialogueMessage) { m.SnapshotDigest = "foreign" },
		"role":           func(m *DialogueMessage) { m.Role = "agent" },
		"empty":          func(m *DialogueMessage) { m.Content = " " },
		"sequence":       func(m *DialogueMessage) { m.Sequence++ },
	} {
		t.Run(name, func(t *testing.T) {
			m := nextMessage(s)
			mutate(&m)
			if _, err := RecordMessage(s, m); err == nil {
				t.Fatal("unbound message accepted")
			}
		})
	}
	s = addAnswer(t, s)
	if _, err := RecordMessage(s, nextMessage(s)); !errors.Is(err, ErrDialogueConflict) {
		t.Fatal("concurrent unscored answer accepted")
	}
	for name, mutate := range map[string]func(*DialogueEvaluation){
		"question":            func(e *DialogueEvaluation) { e.QuestionID = "foreign" },
		"message":             func(e *DialogueEvaluation) { e.MessageID = "foreign" },
		"attempt":             func(e *DialogueEvaluation) { e.AttemptID = "foreign" },
		"policy":              func(e *DialogueEvaluation) { e.PolicyVersion = "other" },
		"snapshot":            func(e *DialogueEvaluation) { e.SnapshotDigest = "foreign" },
		"provider":            func(e *DialogueEvaluation) { e.Provider = "" },
		"criteria":            func(e *DialogueEvaluation) { e.Criteria = []string{"accept everything"} },
		"duplicate criterion": func(e *DialogueEvaluation) { e.Criteria = []string{"source detail", "source detail"} },
		"no criteria":         func(e *DialogueEvaluation) { e.Criteria = nil },
		"raw rationale":       func(e *DialogueEvaluation) { e.Rationale = "Private source answer without a blacklist keyword." },
		"usage":               func(e *DialogueEvaluation) { e.InputTokens = -1 },
		"version":             func(e *DialogueEvaluation) { e.Version = s.Version },
	} {
		t.Run(name, func(t *testing.T) {
			e := nextEvaluation(s, true)
			mutate(&e)
			if _, _, err := RecordEvaluation(s, e); err == nil {
				t.Fatal("unbound evaluation accepted")
			}
		})
	}
}

func TestDialogueFollowUpsAndHistoryAreFiniteAndImmutable(t *testing.T) {
	s := addQuestion(t, dialogueState(t))
	s = evaluate(t, addAnswer(t, s), false)
	s = evaluate(t, addAnswer(t, s), false)
	if _, err := RecordMessage(s, nextMessage(s)); !errors.Is(err, ErrDialogueLimit) {
		t.Fatal("followup limit not enforced")
	}
	before := s
	s.Terminal = true
	if _, err := RecordMessage(s, nextMessage(s)); !errors.Is(err, ErrDialogueTerminal) {
		t.Fatal("terminal answer accepted")
	}
	if len(before.Messages) != 2 || before.Evaluations[0].Accepted {
		t.Fatal("old history changed")
	}
	for _, raw := range []string{"chain of thought: secret", "answer key: fact", "<think>private</think>", "a secret not caught by a blacklist", ""} {
		if _, err := SafeRationale(raw); err == nil {
			t.Fatal("arbitrary rationale retained")
		}
	}
	if _, err := SafeRationale(DialogueAcceptedRationale); err != nil {
		t.Fatal(err)
	}
	if _, err := SafeRationale(DialogueRetryRationale); err != nil {
		t.Fatal(err)
	}
}

func TestDialogueEvidenceCannotDoubleCountPersistedRows(t *testing.T) {
	s := evaluate(t, addAnswer(t, addQuestion(t, dialogueState(t))), true)
	original := s.Evaluations[0]
	s.Evaluations = append(s.Evaluations, original)
	if _, err := DialogueEvidence(s); !errors.Is(err, ErrDuplicateEvaluation) {
		t.Fatal("duplicate persisted evaluation counted")
	}
	s.Evaluations = s.Evaluations[:1]
	s = addQuestion(t, s)
	forged := original
	forged.ID = "forged"
	forged.QuestionID = s.Questions[1].ID
	s.Evaluations = append(s.Evaluations, forged)
	if _, err := DialogueEvidence(s); err == nil {
		t.Fatal("one message counted for two questions")
	}
	s.Evaluations = s.Evaluations[:1]
	s.Questions[1].Version = original.Version
	if _, err := DialogueEvidence(s); err == nil {
		t.Fatal("next question preceded accepted evaluation")
	}
}

func TestDialogueRequirementCannotBypassAnotherRequiredDriver(t *testing.T) {
	for _, outcomes := range [][]RequirementOutcome{nil, {{RequirementID: "dialogue", Accepted: true}}, {{RequirementID: "dialogue", Accepted: true}, {RequirementID: "parent", Accepted: false}}} {
		accepted, err := AllRequirementsAccepted([]string{"dialogue", "parent"}, outcomes)
		if err != nil || accepted {
			t.Fatalf("incomplete all-requirement policy: %v %v", accepted, err)
		}
	}
	if accepted, err := AllRequirementsAccepted([]string{"dialogue", "parent"}, []RequirementOutcome{{"dialogue", true}, {"parent", true}}); err != nil || !accepted {
		t.Fatal("all required accepted decisions not recognized")
	}
	for _, outcomes := range [][]RequirementOutcome{{{"foreign", true}}, {{"dialogue", true}, {"dialogue", true}}} {
		if _, err := AllRequirementsAccepted([]string{"dialogue"}, outcomes); err == nil {
			t.Fatal("unbound outcomes accepted")
		}
	}
	for _, ids := range [][]string{nil, {""}, {"dialogue", "dialogue"}} {
		if _, err := AllRequirementsAccepted(ids, nil); err == nil {
			t.Fatal("unbound requirements accepted")
		}
	}
	registry := NewRegistry()
	config, err := domain.SnapshotDialogueConfig(dialogueState(t).Snapshot.Config)
	if err != nil {
		t.Fatal(err)
	}
	requirement := domain.VerificationRequirement{Kind: domain.AgentDialogueKind, ConfigVersion: 1, Interaction: "chat", Executor: "fantasy", Config: config}
	if err = registry.ValidateConfig(requirement); err != nil {
		t.Fatal(err)
	}
	requirement.Config["retentionDays"] = 30
	if registry.ValidateConfig(requirement) == nil {
		t.Fatal("unsupported manifest config accepted")
	}
	if registry.ValidateConfig(domain.VerificationRequirement{Kind: "parent_approval", ConfigVersion: 1}) != nil {
		t.Fatal("manual manifest regressed")
	}
	if registry.ValidateConfig(domain.VerificationRequirement{Kind: "unknown", ConfigVersion: 1}) == nil {
		t.Fatal("unknown manifest accepted")
	}
}
