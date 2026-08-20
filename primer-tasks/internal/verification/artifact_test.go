package verification

import "testing"

func TestArtifactRubricManifestRequiresStablePolicy(t *testing.T) {
	r := NewRegistry()
	good := map[string]any{"acceptedKinds": []any{"image", "audio"}, "criteria": []any{map[string]any{"id": "shows-work", "label": "Shows work", "description": "The work is visible."}}, "passRule": "all_required", "reviewPolicy": "parent_review"}
	if err := r.ValidateConfig("agent_artifact_rubric", 1, good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string]any{{}, {"acceptedKinds": []any{"pdf"}, "criteria": []any{map[string]any{"id": "a", "label": "A", "description": "B"}}, "passRule": "all_required", "reviewPolicy": "parent_review"}, {"acceptedKinds": []any{"image"}, "criteria": []any{}, "passRule": "all_required", "reviewPolicy": "parent_review"}, {"acceptedKinds": []any{"image"}, "criteria": []any{map[string]any{"id": "a", "label": "A", "description": "B"}}, "passRule": "any", "reviewPolicy": "parent_review"}} {
		if err := r.ValidateConfig("agent_artifact_rubric", 1, bad); err == nil {
			t.Fatalf("invalid rubric accepted: %#v", bad)
		}
	}
}
