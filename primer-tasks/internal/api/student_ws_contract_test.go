package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"primer-tasks/internal/domain"
)

func contractStudentEvent(kind string) wireStudentEvent {
	return wireStudentEvent{Protocol: 1, Kind: kind, Sequence: 1, Cursor: 1, Time: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC), OccurrenceID: "00000000-0000-0000-0000-000000000001", AttemptID: "00000000-0000-0000-0000-000000000002", RequirementID: "00000000-0000-0000-0000-000000000003", QuestionID: "00000000-0000-0000-0000-000000000004", PolicyVersion: domain.DialoguePolicyVersion, SnapshotDigest: strings.Repeat("a", 64), Version: 2, RequiredCount: 3}
}
func TestStudentContractRuntimeShapesAndNegativeBindings(t *testing.T) {
	valid := contractStudentEvent("question")
	valid.Text = "What did the family repair after the storm?"
	if err := validateStudentEvent(valid); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*wireStudentEvent){
		"wrong kind":               func(e *wireStudentEvent) { e.Kind = "reasoning_delta" },
		"unknown variant property": func(e *wireStudentEvent) { e.Status = "accepted" },
		"missing requirement":      func(e *wireStudentEvent) { e.RequirementID = "" },
		"missing question":         func(e *wireStudentEvent) { e.QuestionID = "" },
		"bad policy":               func(e *wireStudentEvent) { e.PolicyVersion = "dialogue.v2" },
		"bad snapshot":             func(e *wireStudentEvent) { e.SnapshotDigest = "forged" },
		"mismatched cursor":        func(e *wireStudentEvent) { e.Cursor++ },
		"zero durable cursor":      func(e *wireStudentEvent) { e.Cursor, e.Sequence = 0, 0 },
		"empty time":               func(e *wireStudentEvent) { e.Time = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			e := valid
			change(&e)
			if validateStudentEvent(e) == nil {
				t.Fatal("invalid runtime event accepted")
			}
		})
	}
	complete := valid
	complete.Kind = "complete"
	complete.Text = ""
	complete.QuestionID = ""
	complete.Status = "accepted"
	complete.OccurrenceStatus = "completed"
	complete.DecisionID = "00000000-0000-0000-0000-000000000005"
	complete.DecisionSource = "verification_engine"
	complete.AcceptedCount = 3
	if err := validateStudentEvent(complete); err != nil {
		t.Fatal(err)
	}
	complete.AcceptedCount = 2
	if validateStudentEvent(complete) == nil {
		t.Fatal("model completion below policy accepted")
	}
	complete.DecisionSource = "parent_override"
	complete.AcceptedCount = 0
	if err := validateStudentEvent(complete); err != nil {
		t.Fatal("explicit parent override rejected")
	}
	complete.Status = "failed"
	if validateStudentEvent(complete) == nil {
		t.Fatal("failure mislabeled complete")
	}
	for _, raw := range []string{
		`{"protocol":1,"kind":"subscribe","occurrenceId":"00000000-0000-0000-0000-000000000001","attemptId":"00000000-0000-0000-0000-000000000002","text":"not allowed"}`,
		`{"protocol":1,"kind":"ack","cursor":1,"source":"client policy"}`,
		`{"protocol":1,"kind":"ack","kind":"unsubscribe"}`,
		`{"protocol":1,"kind":"ack","cursor":-1}`,
		`{"protocol":1,"kind":"ack","cursor":9007199254740992}`,
		`{"protocol":1,"kind":"complete"}`,
	} {
		if _, err := decodeStudentCommand([]byte(raw)); err == nil {
			t.Fatal("invalid runtime command accepted")
		}
	}
	if _, err := decodeStudentCommand([]byte(`{"protocol":1,"kind":"ack","cursor":0}`)); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(valid)
	unsafe := strings.TrimSuffix(string(encoded), "}") + `,"provider_metadata":{"reasoning":"secret"}}`
	if _, err := decodeStudentEvent([]byte(unsafe)); err == nil {
		t.Fatal("durable provider payload silently dropped instead of rejected")
	}
}

func TestStudentContractIsDeterministicAndReflectsRuntimeDeclarations(t *testing.T) {
	first, err := StudentSocketSchema()
	if err != nil {
		t.Fatal(err)
	}
	second, err := StudentSocketSchema()
	if err != nil || string(first) != string(second) {
		t.Fatal("nondeterministic student contract")
	}
	var schema map[string]any
	if json.Unmarshal(first, &schema) != nil {
		t.Fatal("invalid schema")
	}
	transport := schema["x-transport"].(map[string]any)
	if transport["readLimitBytes"] != float64(studentReadLimit) || transport["replayPageSize"] != float64(studentReplayPageSize) || transport["acknowledgmentWindow"] != float64(studentAckWindow) {
		t.Fatal("transport metadata not bound to runtime constants")
	}
	for _, field := range []string{"expectedVersion", "snapshotDigest", "questionId", "policyVersion", "requirementId"} {
		if !strings.Contains(StudentSocketTypeScript(), field) {
			t.Fatal("actual runtime binding omitted from generated types")
		}
	}
	if !reflect.DeepEqual(studentWireContract(reflect.TypeOf(studentCommand{}), studentCommandVariants), studentCommandContract) {
		t.Fatal("runtime contract is detached from actual struct")
	}
	config, err := DialogueConfigSchema()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `"x-manifest"`) || !strings.Contains(DialogueConfigTypeScript(), `retentionPolicy: "retain"`) {
		t.Fatal("actual parent input metadata omitted")
	}
}
