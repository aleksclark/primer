package verification

import (
	"context"
	"testing"
)

func testArtifactRubric() ArtifactRubric {
	return ArtifactRubric{AcceptedKinds: []string{"image"}, Criteria: []ArtifactCriterion{
		{ID: "shows-work", Label: "Shows work", Description: "The work is visible.", Required: true},
		{ID: "neat", Label: "Neat", Description: "The work is legible.", Required: false},
	}, PassRule: "all_required", ReviewPolicy: "parent_review"}
}

func TestEvaluateArtifactRequiresEveryCriterionAndNeverInfersAcceptance(t *testing.T) {
	rubric := testArtifactRubric()
	ready, err := EvaluateArtifact(rubric, []ArtifactCriterionResult{{CriterionID: "shows-work", Required: true, Status: "accepted"}})
	if err != nil || !ready.Ready || !ready.Accepted {
		t.Fatalf("required-only result = %+v, err=%v", ready, err)
	}
	ready, err = EvaluateArtifact(rubric, nil)
	if err != nil || ready.Ready || ready.Accepted {
		t.Fatalf("empty result inferred a decision: %+v, err=%v", ready, err)
	}
	ready, err = EvaluateArtifact(rubric, []ArtifactCriterionResult{{CriterionID: "shows-work", Required: true, Status: "rejected"}})
	if err != nil || !ready.Ready || ready.Accepted {
		t.Fatalf("negative result = %+v, err=%v", ready, err)
	}
	if _, err = EvaluateArtifact(rubric, []ArtifactCriterionResult{{CriterionID: "unknown", Required: true, Status: "accepted"}}); err == nil {
		t.Fatal("unknown provider criterion accepted")
	}
}

type countingCommitter struct {
	calls    int
	decision Decision
}

func (c *countingCommitter) CommitDecision(_ context.Context, d Decision) (bool, error) {
	c.calls++
	c.decision = d
	return c.calls == 1, nil
}

func TestArtifactRubricConfigRejectsUnsafeShapesAndKinds(t *testing.T) {
	base := map[string]any{
		"acceptedKinds": []any{"image"},
		"criteria":      []any{map[string]any{"id": "shows-work", "label": "Shows work", "description": "Visible"}},
	}
	cases := []struct {
		name string
		mut  func(map[string]any)
	}{
		{"missing media", func(c map[string]any) { delete(c, "acceptedKinds") }},
		{"unsupported media", func(c map[string]any) { c["acceptedKinds"] = []any{"document"} }},
		{"missing criteria", func(c map[string]any) { delete(c, "criteria") }},
		{"invalid criterion", func(c map[string]any) { c["criteria"] = []any{"not-an-object"} }},
		{"empty criterion id", func(c map[string]any) {
			c["criteria"] = []any{map[string]any{"id": "", "label": "x", "description": "y"}}
		}},
		{"duplicate criterion", func(c map[string]any) {
			c["criteria"] = []any{map[string]any{"id": "x", "label": "x", "description": "y"}, map[string]any{"id": "x", "label": "x", "description": "y"}}
		}},
		{"missing label", func(c map[string]any) { c["criteria"] = []any{map[string]any{"id": "x", "description": "y"}} }},
		{"missing description", func(c map[string]any) { c["criteria"] = []any{map[string]any{"id": "x", "label": "x"}} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]any{}
			for key, value := range base {
				config[key] = value
			}
			tc.mut(config)
			if err := validateArtifactRubricConfig(config); err == nil {
				t.Fatal("invalid rubric config accepted")
			}
		})
	}
}

func TestGenericCommitDecisionDelegatesExactlyOnce(t *testing.T) {
	committer := &countingCommitter{}
	inserted, err := CommitDecision(context.Background(), committer, Decision{TenantID: "tenant", AttemptID: "attempt", OccurrenceID: "occurrence", Accepted: true, Reason: "rubric passed"})
	if err != nil || !inserted || committer.calls != 1 || !committer.decision.Accepted {
		t.Fatalf("commit = inserted:%v calls:%d decision:%+v err:%v", inserted, committer.calls, committer.decision, err)
	}
}
