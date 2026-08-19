package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
)

func TestScriptedFantasyDurableRunCreatesAndSchedulesThroughServices(t *testing.T) {
	t.Setenv("TASKS_AGENT_MODE", string(parent.ProviderScripted))
	t.Setenv("TASKS_AGENT_ACTIVE_TOOLS", strings.Join(defaultToolNames(), ","))
	pool := integrationPool(t)
	ctx := context.Background()
	tenant, student := uuid.NewString(), uuid.NewString()
	conversation, message, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,'Scripted Run Household')`, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO parent_memberships(tenant_id,subject_ref,role) VALUES($1,'scripted-parent','admin')`, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO students(id,tenant_id,display_name) VALUES($2,$1,'Scripted Student')`, tenant, student); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_run_events WHERE tenant_id=$1; DELETE FROM agent_jobs WHERE tenant_id=$1; DELETE FROM agent_runs WHERE tenant_id=$1; DELETE FROM agent_messages WHERE tenant_id=$1; DELETE FROM agent_conversations WHERE tenant_id=$1; DELETE FROM task_schedules WHERE tenant_id=$1; DELETE FROM verification_requirements WHERE tenant_id=$1; DELETE FROM task_revisions WHERE tenant_id=$1; DELETE FROM task_templates WHERE tenant_id=$1; DELETE FROM students WHERE tenant_id=$1; DELETE FROM parent_memberships WHERE tenant_id=$1; DELETE FROM tenants WHERE id=$1`, tenant)
	})
	store := agent.NewPostgresRepository(pool)
	now := time.Now().UTC()
	if err := store.CreateConversation(ctx, agent.Conversation{ID: conversation, TenantID: tenant, ActorID: "scripted-parent", Status: agent.ConversationActive, PolicyVersion: "parent.v1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, inserted, err := store.AppendUserMessage(ctx, agent.Message{ID: message, TenantID: tenant, ConversationID: conversation, ClientMessageID: "scripted-run", Role: agent.RoleUser, Content: "create and schedule a task", Sequence: 1, CreatedAt: now}); err != nil || !inserted {
		t.Fatalf("message inserted=%v err=%v", inserted, err)
	}
	if err := store.CreateRun(ctx, agent.Run{ID: runID, TenantID: tenant, ConversationID: conversation, UserMessageID: message, Status: agent.RunQueued, MaxSteps: 12, MaxTokens: 4096, Deadline: now.Add(time.Minute), Provenance: agent.Provenance{Provider: "scripted", Model: "primer-tasks-scripted", PolicyVersion: "parent.v1", PromptDigest: "digest"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	s := New(pool, "test")
	if err := s.executeAgentRun(ctx, jobs.Job{ID: uuid.NewString(), TenantID: tenant, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM agent_runs WHERE tenant_id=$1 AND id=$2`, tenant, runID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(agent.RunSucceeded) {
		rows, _ := pool.Query(ctx, `SELECT event_type,payload FROM agent_run_events WHERE tenant_id=$1 ORDER BY sequence`)
		defer rows.Close()
		var transcript strings.Builder
		for rows.Next() {
			var kind string
			var payload []byte
			_ = rows.Scan(&kind, &payload)
			transcript.WriteString(kind + ":" + string(payload) + "\\n")
		}
		t.Fatalf("run status=%s\\n%s", status, transcript.String())
	}
	var schedules int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_schedules WHERE tenant_id=$1`, tenant).Scan(&schedules); err != nil {
		t.Fatal(err)
	}
	if schedules != 1 {
		var tasks, events int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM task_revisions WHERE tenant_id=$1`, tenant).Scan(&tasks)
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM agent_run_events WHERE tenant_id=$1`, tenant).Scan(&events)
		rows, _ := pool.Query(ctx, `SELECT event_type,payload FROM agent_run_events WHERE tenant_id=$1 ORDER BY sequence`, tenant)
		defer rows.Close()
		var transcript strings.Builder
		for rows.Next() {
			var kind string
			var payload []byte
			_ = rows.Scan(&kind, &payload)
			transcript.WriteString(kind + ":" + string(payload) + "\\n")
		}
		t.Fatalf("scripted run created %d schedules (tasks=%d events=%d)\\n%s", schedules, tasks, events, transcript.String())
	}
	var assistant string
	if err := pool.QueryRow(ctx, `SELECT content FROM agent_messages WHERE tenant_id=$1 AND conversation_id=$2 AND role='assistant'`, tenant, conversation).Scan(&assistant); err != nil {
		t.Fatal(err)
	}
	if assistant == "" || strings.Contains(strings.ToLower(assistant), "reasoning") || strings.Contains(assistant, "hidden") {
		t.Fatalf("unsafe assistant message=%q", assistant)
	}
}
