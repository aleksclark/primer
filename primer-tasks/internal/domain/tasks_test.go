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
	for _, tc := range []struct {
		a, b OccurrenceStatus
		ok   bool
	}{{OccurrencePending, OccurrencePending, true}, {OccurrencePending, OccurrenceInProgress, true}, {OccurrencePending, OccurrenceCanceled, true}, {OccurrencePending, OccurrenceCompleted, false}, {OccurrenceInProgress, OccurrenceAwaitingVerification, true}, {OccurrenceInProgress, OccurrenceCanceled, true}, {OccurrenceInProgress, OccurrenceCompleted, false}, {OccurrenceAwaitingVerification, OccurrenceCompleted, true}, {OccurrenceAwaitingVerification, OccurrencePending, true}, {OccurrenceAwaitingVerification, OccurrenceCanceled, true}, {OccurrenceCompleted, OccurrencePending, false}, {OccurrenceExcused, OccurrenceCompleted, false}, {OccurrenceCanceled, OccurrenceCanceled, true}} {
		if got := CanTransition(tc.a, tc.b); got != tc.ok {
			t.Errorf("%s to %s=%v", tc.a, tc.b, got)
		}
	}
}
