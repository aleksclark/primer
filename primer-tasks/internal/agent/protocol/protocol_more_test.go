package protocol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCommandValidationRejectsMalformedAndAcceptsAllKinds(t *testing.T) {
	valid := []CommandEnvelope{
		{Protocol: Version, Kind: CommandHello},
		{Protocol: Version, Kind: CommandSubscribe, RunID: "run"},
		{Protocol: Version, Kind: CommandSubscribe, ConversationID: "conversation"},
		{Protocol: Version, Kind: CommandUnsubscribe, RunID: "run"},
		{Protocol: Version, Kind: CommandUserMessage, ConversationID: "conversation", ClientMessageID: "client", Text: "hello"},
		{Protocol: Version, Kind: CommandCancel, RunID: "run"},
		{Protocol: Version, Kind: CommandConfirm, RunID: "run", ConfirmationID: "handle"},
	}
	for _, command := range valid {
		if err := command.Validate(); err != nil {
			t.Fatalf("valid command %+v: %v", command, err)
		}
	}
	invalid := []CommandEnvelope{
		{Protocol: 99, Kind: CommandHello},
		{Protocol: Version, Kind: CommandSubscribe},
		{Protocol: Version, Kind: CommandUnsubscribe},
		{Protocol: Version, Kind: CommandCancel},
		{Protocol: Version, Kind: CommandUserMessage, ConversationID: "c", ClientMessageID: "m"},
		{Protocol: Version, Kind: CommandUserMessage, ConversationID: "c", Text: "x"},
		{Protocol: Version, Kind: CommandConfirm, RunID: "r"},
		{Protocol: Version, Kind: CommandKind("unknown")},
	}
	for _, command := range invalid {
		if err := command.Validate(); err == nil {
			t.Fatalf("invalid command accepted: %+v", command)
		}
	}
}

func TestEventConstructorsAndJSONOnlyExposeSafeFields(t *testing.T) {
	expires := time.Date(2028, 1, 2, 3, 4, 5, 0, time.UTC)
	events := []Event{
		Retry("run", 1, 2, 1500*time.Millisecond),
		Confirmation("run", 2, "opaque-handle", "Disable schedule", expires),
		Error("run", 3, "provider_unavailable"),
	}
	if events[0].Retry != 2 || events[0].RetryAfterMS != 1500 {
		t.Fatalf("retry=%+v", events[0])
	}
	if events[1].ConfirmationID != "opaque-handle" || !events[1].ExpiresAt.Equal(expires) {
		t.Fatalf("confirmation=%+v", events[1])
	}
	if events[2].Code != "provider_unavailable" {
		t.Fatalf("error=%+v", events[2])
	}
	b, err := json.Marshal(events[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "reasoning") || strings.Contains(string(b), "provider metadata") || !strings.Contains(string(b), "confirmationId") {
		t.Fatalf("unsafe event JSON=%s", b)
	}
	if _, err := SchemaJSON(); err != nil {
		t.Fatal(err)
	}
}
