package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"charm.land/fantasy"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/securityreview"
	"primer-tasks/internal/verification"
)

func TestAgentHubTenantConversationFilteringAndBoundedSlowSubscriber(t *testing.T) {
	h := newAgentHub()
	cancelled := make(chan struct{})
	allowed := &agentSubscriber{tenant: "tenant-a", conversation: "conversation-a", queue: make(chan wireAgentEvent, 64), done: make(chan struct{}), cancel: func() { close(cancelled) }}
	otherConversation := &agentSubscriber{tenant: "tenant-a", conversation: "conversation-b", queue: make(chan wireAgentEvent, 64), done: make(chan struct{})}
	otherTenant := &agentSubscriber{tenant: "tenant-b", conversation: "conversation-a", queue: make(chan wireAgentEvent, 64), done: make(chan struct{})}
	h.add(allowed)
	h.add(otherConversation)
	h.add(otherTenant)
	e := wireAgentEvent{Type: "text_delta", TenantID: "tenant-a", ConversationID: "conversation-a", Sequence: 1, Time: time.Now()}
	h.publish(e)
	if len(allowed.queue) != 1 || len(otherConversation.queue) != 0 || len(otherTenant.queue) != 0 {
		t.Fatalf("routing allowed=%d other-conversation=%d other-tenant=%d", len(allowed.queue), len(otherConversation.queue), len(otherTenant.queue))
	}
	// Fill a subscriber queue without a reader. The publisher evicts it at the
	// boundary instead of blocking or allocating an unbounded queue.
	for i := 0; i < cap(allowed.queue); i++ {
		h.publish(wireAgentEvent{Type: "text_delta", TenantID: "tenant-a", ConversationID: "conversation-a", Sequence: int64(i + 2)})
	}
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

func TestAgentSubscriberClosedAndFullQueuesFailClosed(t *testing.T) {
	closed := &agentSubscriber{queue: make(chan wireAgentEvent, 1), done: make(chan struct{})}
	close(closed.done)
	if closed.enqueue(wireAgentEvent{Type: "text_delta"}) {
		t.Fatal("closed subscriber accepted an event")
	}

	full := &agentSubscriber{queue: make(chan wireAgentEvent, 1), done: make(chan struct{})}
	full.queue <- wireAgentEvent{Type: "existing"}
	if full.enqueue(wireAgentEvent{Type: "text_delta"}) {
		t.Fatal("full subscriber queue accepted an event")
	}
	// Closing an evicted subscriber must remain idempotent; this is the race
	// boundary that prevents a publisher from sending into a closed queue.
	full.close()
	full.close()
	if full.enqueue(wireAgentEvent{Type: "after-close"}) {
		t.Fatal("evicted subscriber accepted an event")
	}
}

func TestAgentDevelopmentOriginAllowlistIsStillBounded(t *testing.T) {
	s := &Server{Env: "development", Auth: AuthConfig{PublicOrigin: "https://tasks.example"}}
	for _, tc := range []struct {
		origin string
		want   bool
	}{
		{"", false},
		{"http://127.0.0.1:4173", true},
		{"http://localhost:5173/", true},
		{"https://evil.example", false},
	} {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.Header.Set("Origin", tc.origin)
		if got := s.agentOriginAllowed(req); got != tc.want {
			t.Errorf("development origin %q allowed=%v, want %v", tc.origin, got, tc.want)
		}
	}
	t.Setenv("TASKS_ALLOWED_ORIGINS", " https://preview.example/ , ")
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Origin", "https://preview.example")
	if !s.agentOriginAllowed(req) {
		t.Fatal("configured preview origin was rejected")
	}
}

func TestScriptedAgentStreamsStopBeforeLeakingCanceledProviderOutput(t *testing.T) {
	t.Setenv("TASKS_AGENT_SCRIPTED_DELAY_MS", "")
	for _, tc := range []struct {
		name   string
		stream fantasy.StreamResponse
		want   int
	}{
		{"tool", scriptedToolStream(context.Background(), "list_students", `{"limit":1}`), 8},
		{"text", scriptedStream(context.Background(), "safe response"), 7},
	} {
		count := 0
		tc.stream(func(part fantasy.StreamPart) bool {
			count++
			return true
		})
		if count != tc.want {
			t.Fatalf("%s stream parts=%d, want %d", tc.name, count, tc.want)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	count := 0
	scriptedStream(ctx, "must not emit")(func(fantasy.StreamPart) bool {
		count++
		return true
	})
	if count != 0 {
		t.Fatalf("canceled stream emitted %d provider parts", count)
	}

	// A consumer may stop before the provider reaches its next durable
	// boundary. Both scripted paths must honor that backpressure.
	for _, stream := range []fantasy.StreamResponse{
		scriptedToolStream(context.Background(), "list_students", `{}`),
		scriptedStream(context.Background(), "safe"),
	} {
		count := 0
		stream(func(fantasy.StreamPart) bool {
			count++
			return false
		})
		if count != 1 {
			t.Fatalf("stream ignored consumer stop after %d parts", count)
		}
	}

	// Cancellation after the first part must prevent the artificial delay from
	// emitting the rest of a provider response.
	t.Setenv("TASKS_AGENT_SCRIPTED_DELAY_MS", "30000")
	for _, makeStream := range []func(context.Context) fantasy.StreamResponse{
		func(ctx context.Context) fantasy.StreamResponse {
			return scriptedToolStream(ctx, "list_students", `{}`)
		},
		func(ctx context.Context) fantasy.StreamResponse { return scriptedStream(ctx, "safe") },
	} {
		ctx, cancel := context.WithCancel(context.Background())
		stream := makeStream(ctx)
		count := 0
		stream(func(part fantasy.StreamPart) bool {
			count++
			cancel()
			return true
		})
		if count != 1 {
			t.Fatalf("canceled delayed stream emitted %d parts", count)
		}
	}
}

func TestScriptedAgentModelAndToolInputRejectUnsafeInvocationShapes(t *testing.T) {
	model := &scriptedParentModel{prompt: "disable the schedule"}
	for i := 0; i < 3; i++ {
		if _, err := model.Stream(context.Background(), fantasy.Call{}); err != nil {
			t.Fatal(err)
		}
	}
	tools := (&Server{}).fantasyTools("tenant", "parent", []string{parent.ToolCreateSchedule}, "run")
	if len(tools) != 1 {
		t.Fatalf("create schedule tools=%d", len(tools))
	}
	response, err := tools[0].Run(context.Background(), fantasy.ToolCall{Input: `{"startAt":"not-an-rfc3339-time"}`})
	if err == nil || !response.IsError || response.Content == "" {
		t.Fatalf("invalid schedule input response=%+v err=%v", response, err)
	}
}

func TestAgentAndArtifactWireSecurityRejectsProviderMaterial(t *testing.T) {
	for _, payload := range [][]byte{
		[]byte(`{"reasoning_delta":"private"}`),
		[]byte(`{"tool_input":{"authorization":"Bearer secret"}}`),
		[]byte(`{"url":"https://bucket.s3.amazonaws.com/tenant/a"}`),
	} {
		if err := securityreview.SafeWirePayload(payload); err == nil {
			t.Fatalf("unsafe provider payload accepted: %s", payload)
		}
	}
	if err := securityreview.TenantObjectKey("tenant/tenant-a/student-file.jpg", "tenant-a"); err == nil {
		t.Fatal("filename-shaped object key accepted")
	}
	if err := securityreview.ShortLivedURL("https://objects.example/opaque", 300); err == nil {
		t.Fatal("unsigned object URL accepted")
	}
}

func TestArtifactDecisionBoundaryRejectsIncompleteProviderResults(t *testing.T) {
	rubric := verification.ArtifactRubric{
		AcceptedKinds: []string{"image"},
		Criteria:      []verification.ArtifactCriterion{{ID: "shows-work", Label: "Shows work", Description: "Visible", Required: true}},
		PassRule:      "all_required", ReviewPolicy: "parent_review",
	}
	ready, err := verification.EvaluateArtifact(rubric, nil)
	if err != nil || ready.Ready || ready.Accepted {
		t.Fatalf("empty provider result inferred acceptance: ready=%+v err=%v", ready, err)
	}
	if _, err := verification.EvaluateArtifact(rubric, []verification.ArtifactCriterionResult{{CriterionID: "shows-work", Required: true, Status: "unknown"}}); err == nil {
		t.Fatal("unknown provider criterion status accepted")
	}
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
