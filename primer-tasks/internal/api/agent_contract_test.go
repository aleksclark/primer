package api

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSocketContractDerivesActualBoundaryDeterministically(t *testing.T) {
	a, err := AgentSocketSchema()
	if err != nil {
		t.Fatal(err)
	}
	b, err := AgentSocketSchema()
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("nondeterministic socket schema")
	}
	ts := AgentSocketTypeScript()
	if ts != AgentSocketTypeScript() {
		t.Fatal("nondeterministic socket types")
	}
	for name := range socketFields(reflect.TypeOf(wireAgentEvent{})) {
		if !strings.Contains(ts, name+":") && !strings.Contains(ts, name+"?:") {
			t.Fatalf("socket boundary field %s absent from generated types", name)
		}
	}
	for _, forbidden := range []string{"TenantID", "Delta", "ToolStatus", "authorization", "bearer"} {
		if strings.Contains(ts, forbidden) {
			t.Fatal("private field in generated contract")
		}
	}
	var schema map[string]any
	if json.Unmarshal(a, &schema) != nil || schema["$schema"] == nil {
		t.Fatal("invalid JSON Schema")
	}
	now := time.Now().UTC()
	for _, e := range []wireAgentEvent{{Type: "hello", ProtocolVersion: 1, Time: now}, {Type: "retry", ProtocolVersion: 1, RunID: "run", Time: now, Attempt: 1, RetryAfterMS: 0}, {Type: "text_end", ProtocolVersion: 1, RunID: "run", Time: now, Text: "The complete final answer."}} {
		if err := validateSocketEvent(e); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if e.Type == "retry" && !bytes.Contains(data, []byte(`"retryAfterMs":0`)) {
			t.Fatal("zero-delay retry violated wire contract")
		}
	}
	for _, e := range []wireAgentEvent{{Type: "provider_reasoning", ProtocolVersion: 1, Time: now}, {Type: "text_delta", ProtocolVersion: 1, Time: now, RunID: "run"}, {Type: "hello", ProtocolVersion: 9, Time: now}, {Type: "hello", ProtocolVersion: 1}} {
		if validateSocketEvent(e) == nil {
			t.Fatal("invalid outbound event accepted")
		}
	}
}
