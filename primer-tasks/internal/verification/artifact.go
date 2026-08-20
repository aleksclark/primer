package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const ArtifactRubricPolicyVersion = "agent_artifact_rubric.v1"

type ArtifactCriterion struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type ArtifactRubric struct {
	AcceptedKinds []string            `json:"acceptedKinds"`
	Criteria      []ArtifactCriterion `json:"criteria"`
	PassRule      string              `json:"passRule"`
	ReviewPolicy  string              `json:"reviewPolicy"`
}

type ArtifactCriterionResult struct {
	CriterionID string
	Required    bool
	Status      string
	Evidence    string
	Feedback    string
}

// ArtifactContext is server-owned context for the single rubric tool.
type ArtifactContext struct {
	TenantID, JobID, SubmissionID  string
	Rubric                         ArtifactRubric
	Provider, Model, PolicyVersion string
}

type ArtifactDecisionReady struct {
	Ready         bool
	Accepted      bool
	AcceptedCount int
	RequiredCount int
	Reason        string
}

func ParseArtifactRubric(raw []byte) (ArtifactRubric, error) {
	var rubric ArtifactRubric
	if err := json.Unmarshal(raw, &rubric); err != nil {
		return rubric, fmt.Errorf("invalid artifact rubric: %w", err)
	}
	if err := validateArtifactRubric(rubric); err != nil {
		return rubric, err
	}
	return rubric, nil
}

func validateArtifactRubric(r ArtifactRubric) error {
	if len(r.AcceptedKinds) == 0 || len(r.Criteria) == 0 {
		return fmt.Errorf("artifact rubric requires media and criteria")
	}
	if r.PassRule != "all_required" {
		return fmt.Errorf("artifact rubric passRule must be all_required")
	}
	if r.ReviewPolicy != "reject" && r.ReviewPolicy != "parent_review" {
		return fmt.Errorf("artifact rubric reviewPolicy is invalid")
	}
	seen := make(map[string]struct{}, len(r.Criteria))
	for _, c := range r.Criteria {
		if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Label) == "" || strings.TrimSpace(c.Description) == "" {
			return fmt.Errorf("artifact rubric criterion is incomplete")
		}
		if _, ok := seen[c.ID]; ok {
			return fmt.Errorf("artifact rubric criterion ids must be unique")
		}
		seen[c.ID] = struct{}{}
	}
	return nil
}

func (r ArtifactRubric) Criterion(id string) (ArtifactCriterion, bool) {
	for _, c := range r.Criteria {
		if c.ID == id {
			return c, true
		}
	}
	return ArtifactCriterion{}, false
}

// EvaluateArtifact is the model-independent rubric authority. A provider can
// record criterion evidence, but it cannot decide whether an attempt or task
// is complete. Missing criteria are deliberately not decision-ready.
func EvaluateArtifact(r ArtifactRubric, results []ArtifactCriterionResult) (ArtifactDecisionReady, error) {
	if err := validateArtifactRubric(r); err != nil {
		return ArtifactDecisionReady{}, err
	}
	byID := make(map[string]ArtifactCriterionResult, len(results))
	for _, result := range results {
		criterion, ok := r.Criterion(result.CriterionID)
		if !ok || result.Required != criterion.Required {
			return ArtifactDecisionReady{}, fmt.Errorf("criterion result is not in the rubric")
		}
		if result.Status != "accepted" && result.Status != "rejected" && result.Status != "unavailable" && result.Status != "pending" {
			return ArtifactDecisionReady{}, fmt.Errorf("criterion result status is invalid")
		}
		if _, duplicate := byID[result.CriterionID]; duplicate {
			return ArtifactDecisionReady{}, fmt.Errorf("duplicate criterion result")
		}
		byID[result.CriterionID] = result
	}
	ready := ArtifactDecisionReady{RequiredCount: 0}
	for _, criterion := range r.Criteria {
		if !criterion.Required {
			continue
		}
		ready.RequiredCount++
		result, ok := byID[criterion.ID]
		if !ok || result.Status == "pending" {
			return ready, nil
		}
		if result.Status == "accepted" {
			ready.AcceptedCount++
		}
	}
	ready.Ready = true
	ready.Accepted = ready.AcceptedCount == ready.RequiredCount
	if ready.Accepted {
		ready.Reason = "all required artifact rubric criteria accepted"
	} else {
		ready.Reason = "one or more required artifact rubric criteria were not accepted"
	}
	return ready, nil
}

// DecisionCommitter is the generic persistence boundary. Implementations must
// insert one decision and apply one completion transition transactionally;
// artifact and dialogue workers never perform those mutations themselves.
type DecisionCommitter interface {
	CommitDecision(context.Context, Decision) (inserted bool, err error)
}

func CommitDecision(ctx context.Context, committer DecisionCommitter, decision Decision) (bool, error) {
	if committer == nil || strings.TrimSpace(decision.AttemptID) == "" {
		return false, fmt.Errorf("verification decision committer is unavailable")
	}
	return committer.CommitDecision(ctx, decision)
}

func validateArtifactRubricConfig(config map[string]any) error {
	accepted, ok := config["acceptedKinds"].([]any)
	if !ok || len(accepted) == 0 {
		return fmt.Errorf("artifact rubric requires acceptedKinds")
	}
	for _, value := range accepted {
		kind, ok := value.(string)
		if !ok || (kind != "image" && kind != "audio" && kind != "video") {
			return fmt.Errorf("artifact rubric has unsupported media kind")
		}
	}
	criteria, ok := config["criteria"].([]any)
	if !ok || len(criteria) == 0 {
		return fmt.Errorf("artifact rubric requires criteria")
	}
	seen := map[string]bool{}
	for _, value := range criteria {
		item, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("artifact rubric criterion is invalid")
		}
		id, ok := item["id"].(string)
		if !ok || id == "" || seen[id] {
			return fmt.Errorf("artifact rubric criterion ids must be unique")
		}
		seen[id] = true
		if label, ok := item["label"].(string); !ok || label == "" {
			return fmt.Errorf("artifact rubric criterion label is required")
		}
		if description, ok := item["description"].(string); !ok || description == "" {
			return fmt.Errorf("artifact rubric criterion description is required")
		}
	}
	if pass, ok := config["passRule"].(string); !ok || pass != "all_required" {
		return fmt.Errorf("artifact rubric passRule must be all_required")
	}
	policy, ok := config["reviewPolicy"].(string)
	if !ok || (policy != "reject" && policy != "parent_review") {
		return fmt.Errorf("artifact rubric reviewPolicy is invalid")
	}
	return nil
}
