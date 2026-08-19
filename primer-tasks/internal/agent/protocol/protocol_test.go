package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCommandsAreTaggedAndValidateRequiredFields(t *testing.T) {
	c := CommandEnvelope{Protocol: Version, Kind: CommandUserMessage, ConversationID: "c", ClientMessageID: "m", Text: "hello"}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"kind":"user_message"`) {
		t.Fatal(string(b))
	}
	if err = c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Text = ""
	if c.Validate() == nil {
		t.Fatal("accepted empty user message")
	}
}
func TestSchemaIsOfflineAndVersioned(t *testing.T) {
	b, err := SchemaJSON()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"user_message", "text_delta", "thinking_start", "tool_progress", "replay_gap"} {
		if !strings.Contains(s, want) {
			t.Errorf("schema missing %q", want)
		}
	}
	if strings.Contains(s, "reasoning_delta") {
		t.Fatal("raw reasoning entered protocol schema")
	}
}
func TestEventHasOnlySafeFields(t *testing.T) {
	b, err := json.Marshal(ToolProgress("r", 4, "Task editor", "started"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"input", "result", "reasoning", "provider_metadata"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("unsafe field %q in %s", bad, b)
		}
	}
}
