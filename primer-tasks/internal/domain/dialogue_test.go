package domain

import (
	"reflect"
	"strings"
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

func TestDialogueConfigAliasesAndCriteriaFallback(t *testing.T) {
	c := validDialogueConfig()
	c.Rubric = nil
	c.AcceptanceCriteria = []string{"use a reason"}
	if got := c.Criteria(); len(got) != 1 || got[0] != "use a reason" {
		t.Fatalf("criteria=%v", got)
	}
	if err := ValidateAgentDialogueConfig(c); err != nil {
		t.Fatal(err)
	}
	if _, err := SnapshotAgentDialogueConfig(c); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDialogueConfig(map[string]any{"sourceText": "chapter", "learningFocus": "focus", "requiredQuestions": 1, "acceptanceCriteria": []string{"reason"}, "allowedFollowUps": 0, "maxAttempts": 1, "maxTurns": 1, "retentionPolicy": "retain"})
	if err != nil || parsed.SourceText != "chapter" {
		t.Fatalf("parsed=%+v err=%v", parsed, err)
	}
}

func TestDialogueConfigRejectsEveryPolicyBoundary(t *testing.T) {
	base := validDialogueConfig()
	cases := []struct {
		name string
		edit func(*DialogueConfig)
	}{
		{"source text too long", func(c *DialogueConfig) { c.SourceRef = ""; c.SourceText = strings.Repeat("x", 12001) }},
		{"focus too long", func(c *DialogueConfig) { c.LearningFocus = strings.Repeat("x", 501) }},
		{"questions too many", func(c *DialogueConfig) { c.RequiredQuestions = 11 }},
		{"rubric too many", func(c *DialogueConfig) { c.Rubric = make([]string, 21) }},
		{"criterion too long", func(c *DialogueConfig) { c.Rubric = []string{strings.Repeat("x", 501)} }},
		{"followups negative", func(c *DialogueConfig) { c.AllowedFollowUps = -1 }},
		{"followups too many", func(c *DialogueConfig) { c.AllowedFollowUps = 6 }},
		{"attempts invalid", func(c *DialogueConfig) { c.MaxAttempts = 0 }},
		{"turns below questions", func(c *DialogueConfig) { c.MaxTurns = 2 }},
		{"turns too many", func(c *DialogueConfig) { c.MaxTurns = 101 }},
		{"retention invalid", func(c *DialogueConfig) { c.RetentionPolicy = "erase" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.edit(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("invalid dialogue policy accepted")
			}
		})
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
