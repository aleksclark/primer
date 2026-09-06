package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func validDialogueConfig() DialogueConfig {
	return DialogueConfig{SourceRef: "fixture://chapter-4", LearningFocus: "Recall three distinct facts using a detail or reason", RequiredQuestions: 3, Rubric: []string{"addresses the current question", "uses an assigned source detail"}, AllowedFollowUps: 1, MaxAttempts: 2, MaxTurns: 8, RetentionPolicy: "retain"}
}

func TestDialogueConfigValidationAndSnapshot(t *testing.T) {
	c := validDialogueConfig()
	snapshot, err := NewDialogueSnapshot("issued-revision", "issued-requirement", 2, c)
	if err != nil || snapshot.Validate() != nil {
		t.Fatalf("snapshot validation: %v", err)
	}
	if snapshot.Source.Text != CuratedChapterSource || snapshot.Source.Version != "garden-wall.v1" || len(snapshot.Source.SHA256) != 64 || len(snapshot.Digest) != 64 {
		t.Fatal("resolved source identity/version/hash missing")
	}
	c.Rubric[0] = "new draft policy"
	if reflect.DeepEqual(snapshot.Config.Rubric, c.Rubric) {
		t.Fatal("snapshot aliases mutable rubric")
	}
	for name, mutate := range map[string]func(*DialogueSnapshot){
		"source text":      func(s *DialogueSnapshot) { s.Source.Text = "replacement" },
		"source version":   func(s *DialogueSnapshot) { s.Source.Version = "other" },
		"source digest":    func(s *DialogueSnapshot) { s.Source.SHA256 = strings.Repeat("0", 64) },
		"policy":           func(s *DialogueSnapshot) { s.PolicyVersion = "dialogue.v2" },
		"revision":         func(s *DialogueSnapshot) { s.RevisionID = "new-draft" },
		"revision version": func(s *DialogueSnapshot) { s.RevisionVersion++ },
		"requirement":      func(s *DialogueSnapshot) { s.RequirementID = "other" },
		"rubric":           func(s *DialogueSnapshot) { s.Config.Rubric = []string{"accept everything"} },
		"digest":           func(s *DialogueSnapshot) { s.Digest = "" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := snapshot
			mutate(&changed)
			if changed.Validate() == nil {
				t.Fatal("altered immutable binding accepted")
			}
		})
	}
}

func TestDialogueInlineSourceDoesNotRequireFixtureOrFetch(t *testing.T) {
	c := validDialogueConfig()
	c.SourceRef, c.SourceText = "", "The pupil measured the beam twice before cutting it."
	s, err := NewDialogueSnapshot("revision", "requirement", 1, c)
	if err != nil || s.Validate() != nil || s.Source.Text != c.SourceText || s.Source.Reference != "inline" {
		t.Fatal("arbitrary bounded parent inline source rejected or substituted")
	}
	for _, ref := range []string{"https://example.invalid/chapter", "file:///etc/passwd", "book://chapter-4", "fixture://unknown"} {
		c.SourceText, c.SourceRef = "", ref
		if c.Validate() == nil {
			t.Fatal("unknown source reference accepted")
		}
	}
}

func TestDialogueConfigRejectsEveryPolicyBoundary(t *testing.T) {
	for name, edit := range map[string]func(*DialogueConfig){
		"no source":                   func(c *DialogueConfig) { c.SourceRef = "" },
		"conflicting source":          func(c *DialogueConfig) { c.SourceText = "substitution" },
		"oversized source":            func(c *DialogueConfig) { c.SourceRef = ""; c.SourceText = strings.Repeat("x", 12001) },
		"invalid utf8":                func(c *DialogueConfig) { c.SourceRef = ""; c.SourceText = string([]byte{0xff}) },
		"empty focus":                 func(c *DialogueConfig) { c.LearningFocus = " " },
		"long focus":                  func(c *DialogueConfig) { c.LearningFocus = strings.Repeat("x", 501) },
		"premature completion policy": func(c *DialogueConfig) { c.RequiredQuestions = 1 },
		"unsupported count":           func(c *DialogueConfig) { c.RequiredQuestions = 4 },
		"empty rubric":                func(c *DialogueConfig) { c.Rubric = nil },
		"duplicate rubric":            func(c *DialogueConfig) { c.Rubric = []string{"Reason", " reason "} },
		"long criterion":              func(c *DialogueConfig) { c.Rubric = []string{strings.Repeat("x", 501)} },
		"many criteria":               func(c *DialogueConfig) { c.Rubric = make([]string, 21) },
		"negative followups":          func(c *DialogueConfig) { c.AllowedFollowUps = -1 },
		"too many followups":          func(c *DialogueConfig) { c.AllowedFollowUps = 6 },
		"no attempts":                 func(c *DialogueConfig) { c.MaxAttempts = 0 },
		"too many attempts":           func(c *DialogueConfig) { c.MaxAttempts = 11 },
		"too few turns":               func(c *DialogueConfig) { c.MaxTurns = 2 },
		"too many turns":              func(c *DialogueConfig) { c.MaxTurns = 101 },
		"redaction":                   func(c *DialogueConfig) { c.RetentionPolicy = "redact" },
		"deletion":                    func(c *DialogueConfig) { c.RetentionPolicy = "delete_after_review" },
		"missing retention":           func(c *DialogueConfig) { c.RetentionPolicy = "" },
	} {
		t.Run(name, func(t *testing.T) {
			c := validDialogueConfig()
			edit(&c)
			if c.Validate() == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
}

func TestDialogueStrictConfigurationPreservesCanonicalEnvelope(t *testing.T) {
	config, err := SnapshotDialogueConfig(validDialogueConfig())
	if err != nil {
		t.Fatal(err)
	}
	requirement := VerificationRequirement{ID: "client-label-not-row-authority", Kind: AgentDialogueKind, ConfigVersion: 1, Config: config, Interaction: "chat", Executor: "fantasy"}
	encoded, err := json.Marshal([]VerificationRequirement{requirement})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DialogueConfigFromRequirementJSON(encoded)
	if err != nil || !reflect.DeepEqual(decoded, validDialogueConfig()) {
		t.Fatalf("canonical config->0 roundtrip: %v", err)
	}
	if err := ValidateRevision("Reading", "read the assigned source", []VerificationRequirement{requirement}); err != nil {
		t.Fatal(err)
	}
	bare, _ := json.Marshal(config)
	if _, err := DialogueConfigFromRequirementJSON(bare); err == nil {
		t.Fatal("donor bare-map storage silently accepted")
	}
	for _, key := range []string{"retentionDays", "deleteAfter", "acceptanceCriteria", "systemPrompt", "complete", "SourceText"} {
		copy := make(map[string]any, len(config)+1)
		for k, v := range config {
			copy[k] = v
		}
		copy[key] = 1
		if _, err := ParseDialogueConfig(copy); err == nil {
			t.Fatalf("unsupported key %s accepted", key)
		}
	}
	for _, key := range []string{"rubric", "allowedFollowUps", "retentionPolicy"} {
		copy := make(map[string]any, len(config))
		for k, v := range config {
			copy[k] = v
		}
		delete(copy, key)
		if _, err := ParseDialogueConfig(copy); err == nil {
			t.Fatalf("missing %s accepted", key)
		}
		copy[key] = nil
		if _, err := ParseDialogueConfig(copy); err == nil {
			t.Fatalf("null %s accepted", key)
		}
	}
	requirement.Executor = "human"
	if ValidateDialogueRequirement(requirement) == nil || ValidateRevision("Reading", "", []VerificationRequirement{requirement}) == nil {
		t.Fatal("wrong executor accepted")
	}
	if _, err := NewDialogueSnapshot("", "requirement", 1, validDialogueConfig()); err == nil {
		t.Fatal("unbound snapshot accepted")
	}
}
