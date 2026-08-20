package verification

import (
	"context"
	"encoding/json"
	"testing"
)

func TestArtifactRubricParserRejectsUnsafeSnapshots(t *testing.T) {
	good := testArtifactRubric()
	goodJSON, err := json.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseArtifactRubric(goodJSON); err != nil || len(parsed.Criteria) != 2 {
		t.Fatalf("good rubric=%+v err=%v", parsed, err)
	}
	cases := []ArtifactRubric{
		{Criteria: good.Criteria, PassRule: good.PassRule, ReviewPolicy: good.ReviewPolicy},
		{AcceptedKinds: good.AcceptedKinds, Criteria: good.Criteria, PassRule: "any", ReviewPolicy: good.ReviewPolicy},
		{AcceptedKinds: good.AcceptedKinds, Criteria: good.Criteria, PassRule: good.PassRule, ReviewPolicy: "auto"},
		{AcceptedKinds: good.AcceptedKinds, Criteria: []ArtifactCriterion{{ID: "", Label: "x", Description: "x", Required: true}}, PassRule: good.PassRule, ReviewPolicy: good.ReviewPolicy},
		{AcceptedKinds: good.AcceptedKinds, Criteria: []ArtifactCriterion{{ID: "same", Label: "x", Description: "x"}, {ID: "same", Label: "y", Description: "y"}}, PassRule: good.PassRule, ReviewPolicy: good.ReviewPolicy},
	}
	for i, rubric := range cases {
		data, marshalErr := json.Marshal(rubric)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, parseErr := ParseArtifactRubric(data); parseErr == nil {
			t.Errorf("case %d accepted invalid rubric", i)
		}
	}
	if _, err := ParseArtifactRubric([]byte("not-json")); err == nil {
		t.Fatal("malformed rubric snapshot accepted")
	}
}

func TestEvaluateArtifactRejectsProviderProtocolViolations(t *testing.T) {
	rubric := testArtifactRubric()
	badResults := [][]ArtifactCriterionResult{
		{{CriterionID: "shows-work", Required: false, Status: "accepted"}},
		{{CriterionID: "shows-work", Required: true, Status: "unknown"}},
		{{CriterionID: "shows-work", Required: true, Status: "accepted"}, {CriterionID: "shows-work", Required: true, Status: "accepted"}},
	}
	for i, results := range badResults {
		if _, err := EvaluateArtifact(rubric, results); err == nil {
			t.Errorf("protocol violation %d was accepted", i)
		}
	}
	ready, err := EvaluateArtifact(rubric, []ArtifactCriterionResult{{CriterionID: "shows-work", Required: true, Status: "pending"}})
	if err != nil || ready.Ready || ready.Accepted {
		t.Fatalf("pending criterion=%+v err=%v", ready, err)
	}
	if inserted, err := CommitDecision(context.Background(), nil, Decision{AttemptID: "attempt"}); err == nil || inserted {
		t.Fatalf("nil committer inserted=%v err=%v", inserted, err)
	}
	if inserted, err := CommitDecision(context.Background(), &countingCommitter{}, Decision{}); err == nil || inserted {
		t.Fatalf("blank decision inserted=%v err=%v", inserted, err)
	}
}
