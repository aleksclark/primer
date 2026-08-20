package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"primer-tasks/internal/agent"
)

func TestAgentHTTPHelperBranches(t *testing.T) {
	s := &Server{Env: "production", SecureCookie: true}
	req := httptest.NewRequest("GET", "/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: "tasks_csrf", Value: "existing"})
	rec := httptest.NewRecorder()
	if got := s.csrfToken(rec, req); got != "existing" || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("existing csrf got=%q cookies=%v", got, rec.Result().Cookies())
	}
	generated := httptest.NewRecorder()
	if token := s.csrfToken(generated, httptest.NewRequest("GET", "/auth/session", nil)); len(token) != 48 || len(generated.Result().Cookies()) != 1 {
		t.Fatalf("csrf token generation token-length=%d cookies=%v", len(token), generated.Result().Cookies())
	}
	if maxInt(1, 2) != 2 || maxInt(3, 2) != 3 {
		t.Fatal("maxInt branch incorrect")
	}
}

func TestAgentCommandHandlersFailClosedBeforeDatabaseAccess(t *testing.T) {
	s := &Server{}
	sub := &agentSubscriber{tenant: "tenant-a", conversation: "", queue: make(chan wireAgentEvent, 1), done: make(chan struct{})}
	s.agentSubscribe(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, sub, agentCommand{ConversationID: "not-a-uuid"})
	if sub.conversation != "" {
		t.Fatal("invalid conversation was subscribed")
	}
	s.agentMessage(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, agentCommand{})
	s.agentMessage(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, agentCommand{ConversationID: "not-a-uuid", ClientMessageID: "client", Text: "message"})
	s.agentCancel(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, agentCommand{})
	s.agentConfirm(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, agentCommand{})
	if agent.NewID() == "" {
		t.Fatal("NewID returned empty id")
	}
}
