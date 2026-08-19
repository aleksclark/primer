package domain

import (
	"reflect"
	"testing"
)

func validDialogueConfig() DialogueConfig {
	return DialogueConfig{SourceRef: "book://chapter-4", LearningFocus: "claim and evidence", RequiredQuestions: 3, Rubric: []string{"answer addresses the question"}, AllowedFollowUps: 1, MaxAttempts: 2, MaxTurns: 8, RetentionPolicy: "retain"}
}

func TestDialogueConfigValidationAndSnapshot(t *testing.T) {
	c := validDialogueConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := SnapshotDialogueConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["sourceRef"] != c.SourceRef || snapshot["requiredQuestions"] != float64(3) {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	c.Rubric[0] = "changed"
	if reflect.DeepEqual(snapshot["rubric"], []any{"changed"}) {
		t.Fatal("snapshot aliases mutable rubric")
	}
	for _, bad := range []DialogueConfig{
		{LearningFocus: "x", RequiredQuestions: 3, Rubric: []string{"x"}, MaxAttempts: 1, MaxTurns: 3, RetentionPolicy: "retain"},
		{SourceRef: "x", LearningFocus: "x", RequiredQuestions: 0, Rubric: []string{"x"}, MaxAttempts: 1, MaxTurns: 3, RetentionPolicy: "retain"},
		{SourceRef: "x", LearningFocus: "x", RequiredQuestions: 3, Rubric: nil, MaxAttempts: 1, MaxTurns: 3, RetentionPolicy: "retain"},
	} {
		if bad.Validate() == nil {
			t.Fatalf("invalid config accepted: %#v", bad)
		}
	}
}

func TestDialogueRequirementManifest(t *testing.T) {
	c := validDialogueConfig()
	if err := ValidateDialogueRequirement(VerificationRequirement{Kind: AgentDialogueKind, ConfigVersion: 1, Config: map[string]any{
		"sourceRef": c.SourceRef, "learningFocus": c.LearningFocus, "requiredQuestions": c.RequiredQuestions,
		"rubric": c.Rubric, "allowedFollowUps": c.AllowedFollowUps, "maxAttempts": c.MaxAttempts, "maxTurns": c.MaxTurns, "retentionPolicy": c.RetentionPolicy,
	}, Interaction: "chat", Executor: "fantasy"}); err != nil {
		t.Fatal(err)
	}
}
