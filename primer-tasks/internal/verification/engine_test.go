package verification

import "testing"

func TestOnlyParentApprovalIsRegistered(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Lookup("parent_approval"); !ok {
		t.Fatal("parent approval missing")
	}
	if _, ok := r.Lookup("agent_dialogue"); ok {
		t.Fatal("future driver registered")
	}
	if e := r.Validate("parent_approval", 1); e != nil {
		t.Fatal(e)
	}
	if e := r.Validate("parent_approval", 2); e == nil {
		t.Fatal("unsupported schema accepted")
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
