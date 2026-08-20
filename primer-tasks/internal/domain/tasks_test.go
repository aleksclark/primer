package domain

import "testing"

func TestValidateRevisionAndTransitions(t *testing.T) {
	if ValidateRevision("", "", []VerificationRequirement{{Kind: "parent_approval", ConfigVersion: 1}}) == nil {
		t.Fatal("empty title accepted")
	}
	if ValidateRevision("x", "", nil) == nil {
		t.Fatal("empty requirements accepted")
	}
	if ValidateRevision("x", "", []VerificationRequirement{{Kind: "agent_dialogue", ConfigVersion: 1}}) != nil {
		t.Fatal("dialogue requirement rejected")
	}
	if ValidateRevision("x", "", []VerificationRequirement{{Kind: "unknown", ConfigVersion: 1}}) == nil {
		t.Fatal("unknown requirement accepted")
	}
	if ValidateRevision("x", "", []VerificationRequirement{{Kind: "parent_approval", ConfigVersion: 1}}) != nil {
		t.Fatal("valid revision rejected")
	}
	artifactConfig := map[string]any{"acceptedKinds": []any{"image"}, "criteria": []any{map[string]any{"id": "one", "label": "One", "description": "Show the work"}}, "passRule": "all_required", "reviewPolicy": "parent_review"}
	if ValidateRevision("x", "", []VerificationRequirement{{Kind: "agent_artifact_rubric", ConfigVersion: 1, Config: artifactConfig, Interaction: "artifact_upload", Executor: "fantasy"}}) != nil {
		t.Fatal("valid artifact rubric rejected")
	}
	external := VerificationRequirement{Kind: "external_callback", ConfigVersion: 1, Config: map[string]any{"verifierId": "verifier", "capability": "response", "schemaVersion": "external_callback.v1"}, Interaction: "external", Executor: "external"}
	if ValidateRevision("x", "", []VerificationRequirement{external}) != nil {
		t.Fatal("valid external requirement rejected")
	}
	for _, invalid := range []map[string]any{{}, {"verifierId": "", "capability": "response", "schemaVersion": "v1"}, {"verifierId": "v", "capability": "", "schemaVersion": "v1"}, {"verifierId": "v", "capability": "c"}} {
		external.Config = invalid
		if ValidateRevision("x", "", []VerificationRequirement{external}) == nil {
			t.Fatalf("invalid external config accepted: %#v", invalid)
		}
	}
	for _, tc := range []struct {
		a, b OccurrenceStatus
		ok   bool
	}{{OccurrencePending, OccurrencePending, true}, {OccurrencePending, OccurrenceInProgress, true}, {OccurrencePending, OccurrenceCanceled, true}, {OccurrencePending, OccurrenceCompleted, false}, {OccurrenceInProgress, OccurrenceAwaitingVerification, true}, {OccurrenceInProgress, OccurrenceCanceled, true}, {OccurrenceInProgress, OccurrenceCompleted, false}, {OccurrenceAwaitingVerification, OccurrenceCompleted, true}, {OccurrenceAwaitingVerification, OccurrencePending, true}, {OccurrenceAwaitingVerification, OccurrenceCanceled, true}, {OccurrenceCompleted, OccurrencePending, false}, {OccurrenceExcused, OccurrenceCompleted, false}, {OccurrenceCanceled, OccurrenceCanceled, true}} {
		if got := CanTransition(tc.a, tc.b); got != tc.ok {
			t.Errorf("%s to %s=%v", tc.a, tc.b, got)
		}
	}
}
