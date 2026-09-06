package agent

import "testing"

func TestDialogueQuestionToolOnlyAcceptsAnIdentity(t *testing.T) {
	for _, raw := range []string{
		`{"prompt":"The answer is that mortar must dry before the next course of stones; why?"}`,
		`{"questionKey":"wall","prompt":"The answer is that mortar must dry before the next course of stones; why?"}`,
		`{"questionKey":"wall","questionKey":"mortar"}`,
		`{"QuestionKey":"wall"}`,
		`{"questionKey":"wall"} {"prompt":"leak"}`,
		`{"questionKey":null}`, `{"questionKey":1}`, `{}`, `[]`,
	} {
		if _, err := dialogueQuestionIdentity(raw); err == nil {
			t.Fatal("question tool admitted a non-identity payload")
		}
	}
	if key, err := dialogueQuestionIdentity(`{"questionKey":"wall"}`); err != nil || key != "wall" {
		t.Fatal("valid question identity rejected")
	}
}
