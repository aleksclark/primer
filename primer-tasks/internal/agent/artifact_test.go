package agent

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"primer-tasks/internal/verification"
)

type artifactToolBackend struct {
	result verification.ArtifactCriterionResult
	calls  int
}

func (b *artifactToolBackend) RecordArtifactCriterion(_ context.Context, _ verification.ArtifactContext, result verification.ArtifactCriterionResult) (verification.ArtifactCriterionResult, bool, error) {
	b.calls++
	b.result = result
	return result, true, nil
}

func TestArtifactRubricHasOneStructuredServerBoundTool(t *testing.T) {
	rubric := verification.ArtifactRubric{AcceptedKinds: []string{"image"}, Criteria: []verification.ArtifactCriterion{{ID: "shows-work", Label: "Shows work", Description: "Visible", Required: true}}, PassRule: "all_required", ReviewPolicy: "reject"}
	backend := &artifactToolBackend{}
	tools, err := NewArtifactRubricTools(backend, verification.ArtifactContext{TenantID: "tenant", JobID: "job", SubmissionID: "submission", Rubric: rubric, Provider: "scripted", Model: "fixture", PolicyVersion: verification.ArtifactRubricPolicyVersion})
	if err != nil || len(tools) != 1 {
		t.Fatalf("tools=%d err=%v", len(tools), err)
	}
	info := tools[0].Info()
	if info.Name != ToolRecordArtifactCriterion || len(info.Required) != 4 {
		t.Fatalf("tool info=%+v", info)
	}
	resp, err := tools[0].Run(context.Background(), fantasy.ToolCall{ID: "call", Name: info.Name, Input: `{"criterionId":"shows-work","status":"accepted","evidence":"visible","feedback":"good"}`})
	if err != nil || backend.calls != 1 || backend.result.Status != "accepted" {
		t.Fatalf("tool response=%+v calls=%d result=%+v err=%v", resp, backend.calls, backend.result, err)
	}
	var out map[string]any
	if json.Unmarshal([]byte(resp.Content), &out) != nil || out["recorded"] != true {
		t.Fatalf("unsafe/unstructured tool output: %s", resp.Content)
	}
}
