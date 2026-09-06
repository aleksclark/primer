package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"primer-tasks/internal/agent"
	"primer-tasks/internal/domain/parent"
)

func TestAgentHubTenantConversationFilteringAndBoundedSlowSubscriber(t *testing.T) {
	h := newAgentHub()
	cancelled := make(chan struct{})
	allowed := &agentSubscriber{tenant: "tenant-a", conversation: "conversation-a", queue: make(chan wireAgentEvent, 64), wake: make(chan struct{}, 1), done: make(chan struct{}), cancel: func() { close(cancelled) }}
	otherConversation := &agentSubscriber{tenant: "tenant-a", conversation: "conversation-b", queue: make(chan wireAgentEvent, 64), done: make(chan struct{})}
	otherTenant := &agentSubscriber{tenant: "tenant-b", conversation: "conversation-a", queue: make(chan wireAgentEvent, 64), done: make(chan struct{})}
	h.add(allowed)
	h.add(otherConversation)
	h.add(otherTenant)
	e := wireAgentEvent{Type: "text_delta", TenantID: "tenant-a", ConversationID: "conversation-a", Sequence: 1, Time: time.Now()}
	h.publish(e)
	if len(allowed.wake) != 1 || len(allowed.queue) != 0 || len(otherConversation.queue) != 0 || len(otherTenant.queue) != 0 {
		t.Fatal("hub must wake only the exact subscribed conversation, never broadcast history")
	}
	// Fill a subscriber queue without a reader. The publisher evicts it at the
	// boundary instead of blocking or allocating an unbounded queue.
	for i := 0; i < cap(allowed.queue); i++ {
		if !allowed.enqueue(e) {
			t.Fatal("queue rejected before bound")
		}
	}
	if allowed.enqueue(e) {
		t.Fatal("queue exceeded bound")
	}
	allowed.close()
	select {
	case <-allowed.done:
		// The queue overflow closes the subscriber without blocking the
		// publisher. A reconnect can use the durable cursor to catch up.
	default:
		t.Fatal("slow subscriber was not closed at queue capacity")
	}
	select {
	case <-cancelled:
		// Eviction also cancels the socket reader, so a slow subscriber cannot
		// leave the HTTP handler blocked after its bounded queue is evicted.
	default:
		t.Fatal("slow subscriber cancellation was not propagated")
	}
	if h.subscriberCountForTest() != 3 {
		t.Fatalf("unrelated subscribers were removed: %d", h.subscriberCountForTest())
	}
	h.remove(allowed)
	h.remove(otherConversation)
	h.remove(otherTenant)
}

func (h *agentHub) subscriberCountForTest() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}

func TestAgentSafeHelpersFailClosedAndHonorCancellation(t *testing.T) {
	if _, err := safeToolJSON(func() {}, nil); err == nil {
		t.Fatal("unmarshalable safe response was accepted")
	}
	t.Setenv("TASKS_AGENT_SCRIPTED_DELAY_MS", "not-a-duration")
	if !scriptedDelay(context.Background()) {
		t.Fatal("invalid scripted delay did not fail open")
	}
	t.Setenv("TASKS_AGENT_SCRIPTED_DELAY_MS", "30001")
	if !scriptedDelay(context.Background()) {
		t.Fatal("overlong scripted delay did not fail open")
	}
	t.Setenv("TASKS_AGENT_SCRIPTED_DELAY_MS", "1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if scriptedDelay(ctx) {
		t.Fatal("canceled scripted delay did not stop")
	}
}

func TestAgentBoundaryHelpersRejectInvalidDurableInputs(t *testing.T) {
	if err := (&Server{}).publishAgent(context.Background(), "tenant", "conversation", wireAgentEvent{}); err == nil {
		t.Fatal("durable event without run was accepted")
	}
	if _, err := (&Server{}).agentModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderDisabled}, ""); !errors.Is(err, agent.ErrProviderDisabled) {
		t.Fatalf("disabled model err=%v", err)
	}
	if model, err := (&Server{}).agentModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderScripted}, "list"); err != nil || model == nil {
		t.Fatalf("scripted model=%v err=%v", model, err)
	}
	s := &Server{}
	s.agentSubscribe(context.Background(), scope{Tenant: "tenant"}, &agentSubscriber{queue: make(chan wireAgentEvent, 1), done: make(chan struct{})}, agentCommand{ConversationID: "not-a-uuid"})
}

func TestAgentOriginCSRFAndWireRedaction(t *testing.T) {
	s := &Server{Env: "production", Auth: AuthConfig{PublicOrigin: "https://tasks.example", RedirectURL: "https://tasks.example/auth/callback"}}
	for _, origin := range []string{"https://tasks.example", "https://tasks.example/", "https://evil.example"} {
		req, _ := http.NewRequest(http.MethodGet, "https://tasks.example/ws", nil)
		req.Header.Set("Origin", origin)
		if got := s.agentOriginAllowed(req); got != (origin != "https://evil.example") {
			t.Errorf("origin %q allowed=%v", origin, got)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://tasks.example/ws", nil)
	req.AddCookie(&http.Cookie{Name: "tasks_csrf", Value: "csrf-value"})
	req.Header.Set("Sec-WebSocket-Protocol", "primer-tasks.v1.csrf.csrf-value, primer-tasks.v1")
	if !s.validCSRF(req) {
		t.Fatal("matching csrf subprotocol rejected")
	}
	req.Header.Set("Sec-WebSocket-Protocol", "primer-tasks.v1")
	if s.validCSRF(req) {
		t.Fatal("missing csrf subprotocol accepted")
	}
	wire := wireAgentEvent{Type: "text_delta", Delta: "provider secret reasoning", Text: "safe final delta", ToolStatus: "raw input", TenantID: "tenant-a"}
	b, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == "" || string(b) == `null` || containsAny(string(b), "provider secret reasoning", "raw input") {
		t.Fatalf("unsafe wire payload: %s", b)
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if len(needle) > 0 && stringContains(value, needle) {
			return true
		}
	}
	return false
}

func stringContains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
