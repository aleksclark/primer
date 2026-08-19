package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"primer-tasks/internal/agent/protocol"
)

func TestRunStateTransitionsAndFiniteLimits(t *testing.T) {
	if !CanTransition(RunQueued, RunRunning) || !CanTransition(RunRunning, RunSucceeded) {
		t.Fatal("expected normal transitions")
	}
	if CanTransition(RunSucceeded, RunRunning) {
		t.Fatal("terminal run resumed")
	}
	r := Run{MaxSteps: 0, MaxTokens: 100, Deadline: time.Now()}
	if ValidateRunLimits(r) == nil {
		t.Fatal("accepted zero step budget")
	}
}
func TestPreviewIsDigestBoundSingleUseAndTenantScoped(t *testing.T) {
	payload := []byte(`{"schedule":"s1"}`)
	p := ConfirmationPreview{TenantID: "a", ActorID: "parent", Action: "retire", ActionDigest: ActionDigest("retire", payload), ExpiresAt: time.Now().Add(time.Minute)}
	if err := ValidatePreview(p, "b", "parent", "retire", payload, time.Now()); !errors.Is(err, ErrPreviewExpired) {
		t.Fatal("foreign tenant preview accepted")
	}
	if err := ValidatePreview(p, "a", "parent", "retire", []byte(`{"schedule":"s2"}`), time.Now()); !errors.Is(err, ErrPreviewExpired) {
		t.Fatal("altered payload accepted")
	}
	used := time.Now()
	p.UsedAt = &used
	if err := ValidatePreview(p, "a", "parent", "retire", payload, time.Now()); !errors.Is(err, ErrPreviewExpired) {
		t.Fatal("replayed preview accepted")
	}
}
func TestProtocolEventsCannotCarryReasoningOrToolPayload(t *testing.T) {
	b, _ := json.Marshal(protocol.ThinkingStart("run", 1))
	if strings.Contains(string(b), "secret reasoning") {
		t.Fatal("reasoning leaked")
	}
	b, _ = json.Marshal(protocol.ToolProgress("run", 2, "Task editor", "completed"))
	if strings.Contains(string(b), "tenant-id") {
		t.Fatal("tool input leaked")
	}
}
