package api

import (
	"context"
	"github.com/google/uuid"
	"primer-tasks/internal/agent"
	"strings"
	"testing"
	"time"
)

func seedAgentTestRun(t *testing.T, s *Server, tenant, actor string) string {
	t.Helper()
	t.Setenv("TASKS_ENV", "test")
	t.Setenv("TASKS_AGENT_MODE", "scripted")
	t.Setenv("TASKS_AGENT_ACTIVE_TOOLS", strings.Join(defaultToolNames(), ","))
	id := uuid.NewString()
	ctx := context.Background()
	if err := agent.NewPostgresRepository(s.DB).CreateConversation(ctx, agent.Conversation{ID: id, TenantID: tenant, ActorID: actor, Status: agent.ConversationActive, PolicyVersion: "parent.v1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	s.agentMessage(ctx, scope{Tenant: tenant, Subject: actor}, agentCommand{ConversationID: id, ClientMessageID: uuid.NewString(), Text: "test command"})
	var run string
	if err := s.DB.QueryRow(ctx, `SELECT id FROM agent_runs WHERE conversation_id=$1`, id).Scan(&run); err != nil {
		t.Fatal(err)
	}
	return run
}
func confirmAgentTestRun(t *testing.T, s *Server, tenant, actor, run string) {
	t.Helper()
	ctx := context.Background()
	var handle string
	if err := s.DB.QueryRow(ctx, `SELECT payload->>'confirmationId' FROM agent_run_events WHERE run_id=$1 AND payload->>'phase'='awaiting_confirmation'`, run).Scan(&handle); err != nil {
		t.Fatal(err)
	}
	if err := s.confirmAgentAction(ctx, scope{Tenant: tenant, Subject: actor}, agentCommand{RunID: run, ConfirmationID: handle}); err != nil {
		t.Fatal(err)
	}
}
