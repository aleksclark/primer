package verification

import "fmt"

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
