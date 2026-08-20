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

func TestGenericCommitDecisionDelegatesExactlyOnce(t *testing.T) {
	committer := &countingCommitter{}
	inserted, err := CommitDecision(context.Background(), committer, Decision{TenantID: "tenant", AttemptID: "attempt", OccurrenceID: "occurrence", Accepted: true, Reason: "rubric passed"})
	if err != nil || !inserted || committer.calls != 1 || !committer.decision.Accepted {
		t.Fatalf("commit = inserted:%v calls:%d decision:%+v err:%v", inserted, committer.calls, committer.decision, err)
	}
}
