package verification

import "testing"

func TestParentApprovalAndDialogueAreRegistered(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Lookup("parent_approval"); !ok {
		t.Fatal("parent approval missing")
	}
	if m, ok := r.Lookup("agent_dialogue"); !ok || m.Interaction != "chat" || m.Executor != "fantasy" {
		t.Fatal("dialogue manifest missing or incorrectly scoped")
	}
	if e := r.Validate("parent_approval", 1); e != nil {
		t.Fatal(e)
	}
	if e := r.Validate("agent_dialogue", 1); e != nil {
		t.Fatal(e)
	}
	if e := r.ValidateConfig("agent_dialogue", 1, map[string]any{"sourceRef": "book://chapter-4", "learningFocus": "focus", "requiredQuestions": 3, "rubric": []string{"criterion"}, "allowedFollowUps": 1, "maxAttempts": 2, "maxTurns": 8, "retentionPolicy": "retain"}); e != nil {
		t.Fatal(e)
	}
	if e := r.Validate("parent_approval", 2); e == nil {
		t.Fatal("unsupported schema accepted")
	}
}
func TestRegistryRejectsUnknownAndAnyPolicy(t *testing.T) {
	r := NewRegistry()
	if err := r.Validate("missing", 1); err == nil {
		t.Fatal("unknown kind accepted")
	}
	r.Register(Manifest{Kind: "fixture", ConfigVersion: 2})
	if err := r.Validate("fixture", 1); err == nil {
		t.Fatal("wrong version accepted")
	}
	if ok, err := Apply(Policy{All: false}, nil); err != nil || ok {
		t.Fatalf("empty policy=%v %v", ok, err)
	}
	if ok, err := Apply(Policy{All: false}, []Decision{{Accepted: false}, {Accepted: true}}); err != nil || !ok {
		t.Fatalf("any policy=%v %v", ok, err)
	}
}
func TestAllPolicyRequiresEveryDecision(t *testing.T) {
	ok, e := Apply(Policy{All: true}, []Decision{{Accepted: true}, {Accepted: false}})
	if e != nil || ok {
		t.Fatalf("accepted mixed decisions: %v %v", ok, e)
	}
	ok, e = Apply(Policy{All: true}, []Decision{{Accepted: true}, {Accepted: true}})
	if e != nil || !ok {
		t.Fatalf("did not accept all: %v %v", ok, e)
	}
}
