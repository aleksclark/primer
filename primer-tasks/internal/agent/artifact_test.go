package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"charm.land/fantasy"
	"primer-tasks/internal/verification"
)

type artifactToolBackend struct {
	result verification.ArtifactCriterionResult
	calls  int
	err    error
}

func (b *artifactToolBackend) RecordArtifactCriterion(_ context.Context, _ verification.ArtifactContext, result verification.ArtifactCriterionResult) (verification.ArtifactCriterionResult, bool, error) {
	b.calls++
	b.result = result
	if b.err != nil {
		return verification.ArtifactCriterionResult{}, false, b.err
	}
	return result, true, nil
}

func TestArtifactRubricToolRejectsUntrustedCriterionResults(t *testing.T) {
	rubric := verification.ArtifactRubric{Criteria: []verification.ArtifactCriterion{{ID: "shows-work", Required: true}}}
	if names := ArtifactRubricToolNames(); len(names) != 1 || names[0] != ToolRecordArtifactCriterion {
		t.Fatalf("tool names=%v", names)
	}
	for name, scope := range map[string]verification.ArtifactContext{
		"missing tenant": {JobID: "job", SubmissionID: "submission", Rubric: rubric},
		"missing job":    {TenantID: "tenant", SubmissionID: "submission", Rubric: rubric},
		"missing submit": {TenantID: "tenant", JobID: "job", Rubric: rubric},
		"missing rubric": {TenantID: "tenant", JobID: "job", SubmissionID: "submission"},
	} {
		if tools, err := NewArtifactRubricTools(&artifactToolBackend{}, scope); err == nil || tools != nil {
			t.Errorf("%s accepted invalid context tools=%v err=%v", name, tools, err)
		}
	}
	tools, err := NewArtifactRubricTools(&artifactToolBackend{}, verification.ArtifactContext{TenantID: "tenant", JobID: "job", SubmissionID: "submission", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string]string{
		"unknown criterion": `{"criterionId":"other","status":"accepted"}`,
		"bad status":        `{"criterionId":"shows-work","status":"pending"}`,
		"long evidence":     `{"criterionId":"shows-work","status":"accepted","evidence":"` + strings.Repeat("x", 1001) + `"}`,
	} {
		resp, runErr := tools[0].Run(context.Background(), fantasy.ToolCall{Name: ToolRecordArtifactCriterion, Input: input})
		if runErr != nil || !strings.Contains(resp.Content, "invalid") && !strings.Contains(resp.Content, "too long") {
			t.Errorf("%s response=%+v err=%v", name, resp, runErr)
		}
	}
	backend := &artifactToolBackend{err: errors.New("database unavailable")}
	tools, err = NewArtifactRubricTools(backend, verification.ArtifactContext{TenantID: "tenant", JobID: "job", SubmissionID: "submission", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	resp, runErr := tools[0].Run(context.Background(), fantasy.ToolCall{Name: ToolRecordArtifactCriterion, Input: `{"criterionId":"shows-work","status":"accepted","evidence":"visible"}`})
	if runErr != nil || !strings.Contains(resp.Content, "not recorded") || backend.calls != 1 {
		t.Fatalf("backend failure response=%+v calls=%d err=%v", resp, backend.calls, runErr)
	}
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
