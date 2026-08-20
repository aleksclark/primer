package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"primer-tasks/internal/verification"
)

const ToolRecordArtifactCriterion = "record_artifact_criterion"

// ArtifactRubricBackend is intentionally narrower than a repository. The
// model can record one criterion result, but has no operation that can decide
// an attempt or complete an occurrence.
type ArtifactRubricBackend interface {
	RecordArtifactCriterion(context.Context, verification.ArtifactContext, verification.ArtifactCriterionResult) (verification.ArtifactCriterionResult, bool, error)
}

// The context is server-owned and never decoded from tool arguments; only
// the criterion fields below are provider-controlled.
type artifactCriterionInput struct {
	CriterionID string `json:"criterionId" jsonschema:"description=the stable rubric criterion id"`
	Status      string `json:"status" jsonschema:"description=accepted rejected or unavailable"`
	Evidence    string `json:"evidence" jsonschema:"description=short observable evidence only"`
	Feedback    string `json:"feedback" jsonschema:"description=short parent-safe feedback"`
}

type ArtifactRubricTools struct {
	Backend ArtifactRubricBackend
	Scope   verification.ArtifactContext
}

func NewArtifactRubricTools(backend ArtifactRubricBackend, scope verification.ArtifactContext) ([]fantasy.AgentTool, error) {
	if backend == nil || scope.TenantID == "" || scope.JobID == "" || scope.SubmissionID == "" || len(scope.Rubric.Criteria) == 0 {
		return nil, fmt.Errorf("artifact rubric tool context is invalid")
	}
	return []fantasy.AgentTool{
		fantasy.NewAgentTool[artifactCriterionInput](ToolRecordArtifactCriterion, "Record one structured result for a parent-authored artifact rubric criterion.", ArtifactRubricTools{Backend: backend, Scope: scope}.recordCriterion),
	}, nil
}

func ArtifactRubricToolNames() []string { return []string{ToolRecordArtifactCriterion} }

func (t ArtifactRubricTools) recordCriterion(ctx context.Context, in artifactCriterionInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	id := strings.TrimSpace(in.CriterionID)
	criterion, ok := t.Scope.Rubric.Criterion(id)
	if !ok || (in.Status != "accepted" && in.Status != "rejected" && in.Status != "unavailable") {
		return fantasy.NewTextErrorResponse("criterion result is invalid"), nil
	}
	if len([]rune(in.Evidence)) > 1000 || len([]rune(in.Feedback)) > 1000 {
		return fantasy.NewTextErrorResponse("criterion result is too long"), nil
	}
	result := verification.ArtifactCriterionResult{CriterionID: id, Required: criterion.Required, Status: in.Status, Evidence: strings.TrimSpace(in.Evidence), Feedback: strings.TrimSpace(in.Feedback)}
	recorded, _, err := t.Backend.RecordArtifactCriterion(ctx, t.Scope, result)
	if err != nil {
		return fantasy.NewTextErrorResponse("criterion result was not recorded"), nil
	}
	out, _ := json.Marshal(struct {
		CriterionID string `json:"criterionId"`
		Status      string `json:"status"`
		Recorded    bool   `json:"recorded"`
	}{recorded.CriterionID, recorded.Status, true})
	return fantasy.NewTextResponse(string(out)), nil
}
