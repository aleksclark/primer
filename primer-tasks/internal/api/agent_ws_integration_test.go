package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"primer-tasks/internal/jobs"
)

func TestAgentWSOriginCSRFDisabledRunAndSafeTerminal(t *testing.T) {
	for _, key := range []string{"TASKS_AGENT_MODE", "TASKS_MODEL_PROVIDER", "TASKS_AGENT_ACTIVE_TOOLS"} {
		t.Setenv(key, "")
	}
	pool := integrationPool(t)
	seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{PublicOrigin: "https://tasks.example", RedirectURL: "https://tasks.example/auth/callback", SessionSecret: []byte("ws-secret"), IssuerSecret: []byte("ws-secret")})
	h := s.Routes()
	// Reject both cross-origin handshakes and same-origin handshakes without
	// the cookie-bound CSRF subprotocol before authentication or upgrade.
	for _, tc := range []struct {
		origin, protocols string
	}{
		{"https://evil.example", "primer-tasks.v1.csrf.any"},
		{"https://tasks.example", "primer-tasks.v1"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Sec-WebSocket-Protocol", tc.protocols)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("invalid websocket boundary origin=%q protocols=%q status=%d", tc.origin, tc.protocols, rec.Code)
		}
	}
	conversationRec := requestJSON(t, h, http.MethodPost, "/agent/conversations", "parent-a", "")
	if conversationRec.Code != http.StatusCreated {
		t.Fatalf("conversation=%d %s", conversationRec.Code, conversationRec.Body.String())
	}
	var conversation AgentConversation
	if err := json.Unmarshal(conversationRec.Body.Bytes(), &conversation); err != nil {
		t.Fatal(err)
	}
	csrfRec := requestJSON(t, h, http.MethodGet, "/auth/session", "parent-a", "")
	if csrfRec.Code != http.StatusOK {
		t.Fatalf("session=%d %s", csrfRec.Code, csrfRec.Body.String())
	}
	var csrf string
	for _, cookie := range csrfRec.Result().Cookies() {
		if cookie.Name == "tasks_csrf" {
			csrf = cookie.Value
		}
	}
	if csrf == "" {
		t.Fatal("session did not issue csrf cookie")
	}
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	header := http.Header{}
	header.Set("Cookie", "tasks_parent=parent-a; tasks_csrf="+csrf)
	header.Set("Origin", "http://127.0.0.1:tasks-test")
	conn, _, err := websocket.Dial(context.Background(), "ws"+server.URL[len("http"):]+"/ws", &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{"primer-tasks.v1.csrf." + csrf, "primer-tasks.v1"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test done") })
	var hello wireAgentEvent
	if err := wsjson.Read(context.Background(), conn, &hello); err != nil || hello.Type != "hello" || hello.ProtocolVersion != agentProtocolVersion || hello.ConnectionID == "" {
		t.Fatalf("hello=%+v err=%v", hello, err)
	}
	if err := wsjson.Write(context.Background(), conn, agentCommand{Type: "subscribe", ProtocolVersion: agentProtocolVersion, ConversationID: conversation.ID, Cursor: 0}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(context.Background(), conn, agentCommand{Type: "user_message", ProtocolVersion: agentProtocolVersion, ConversationID: conversation.ID, ClientMessageID: "ws-disabled-once", Text: "list students"}); err != nil {
		t.Fatal(err)
	}
	var runID string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if err := pool.QueryRow(context.Background(), `SELECT id FROM agent_runs WHERE tenant_id=$1 AND conversation_id=$2 ORDER BY created_at DESC LIMIT 1`, tenantA, conversation.ID).Scan(&runID); err == nil {
			break
		}
	}
	if runID == "" {
		t.Fatal("websocket user message did not enqueue a durable run")
	}
	// Execute through the same durable run entry point the worker owns. The
	// disabled provider must produce an intentional terminal state, not fake
	// success, and the wire text must not contain provider internals.
	if err := s.executeAgentRun(context.Background(), jobs.Job{TenantID: tenantA, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var terminal wireAgentEvent
	for {
		var event wireAgentEvent
		if err := wsjson.Read(readCtx, conn, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "terminal" {
			terminal = event
			break
		}
	}
	if terminal.Status != "disabled" || terminal.Code != "" || terminal.Text == "" || containsAny(terminal.Text, "reasoning", "secret", "api_key", "authorization") {
		t.Fatalf("unsafe/incorrect disabled terminal=%+v", terminal)
	}
}
