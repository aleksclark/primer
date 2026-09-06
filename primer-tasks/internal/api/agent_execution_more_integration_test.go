package api

import (
	"charm.land/fantasy"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/agent"
	agentprotocol "primer-tasks/internal/agent/protocol"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
)

func TestScriptedAgentExecutionUsesServerToolsAndPersistsSafeProgress(t *testing.T) {
	t.Setenv("TASKS_ENV", "test")
	t.Setenv("TASKS_AGENT_MODE", "scripted")
	t.Setenv("TASKS_MODEL_PROVIDER", "")
	t.Setenv("TASKS_AGENT_ACTIVE_TOOLS", strings.Join(defaultToolNames(), ","))
	t.Setenv("TASKS_AGENT_MAX_STEPS", "12")
	t.Setenv("TASKS_AGENT_MAX_TOKENS", "4096")
	t.Setenv("TASKS_AGENT_MAX_SECONDS", "30")
	t.Setenv("TASKS_AGENT_MAX_RETRIES", "0")
	pool := integrationPool(t)
	seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("scripted-agent"), IssuerSecret: []byte("scripted-agent")})
	ctx := context.Background()
	tenant, actor := tenantA, "parent-a"
	conversationID, messageID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM agent_run_events WHERE tenant_id=$1 AND run_id=$2`, tenant, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_runs WHERE tenant_id=$1 AND id=$2`, tenant, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_messages WHERE tenant_id=$1 AND (id=$2 OR conversation_id=$3)`, tenant, messageID, conversationID)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_conversations WHERE tenant_id=$1 AND id=$2`, tenant, conversationID)
	}
	t.Cleanup(cleanup)
	repo := agent.NewPostgresRepository(pool)
	if err := repo.CreateConversation(ctx, agent.Conversation{ID: conversationID, TenantID: tenant, ActorID: actor, Status: agent.ConversationActive, PolicyVersion: "parent.v1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, inserted, err := repo.AppendUserMessage(ctx, agent.Message{ID: messageID, TenantID: tenant, ConversationID: conversationID, ClientMessageID: "scripted-create", Role: agent.RoleUser, Content: "create and schedule a task", Sequence: 1, CreatedAt: now}); err != nil || !inserted {
		t.Fatalf("message inserted=%v err=%v", inserted, err)
	}
	if err := repo.CreateRun(ctx, agent.Run{ID: runID, TenantID: tenant, ConversationID: conversationID, UserMessageID: messageID, Status: agent.RunQueued, MaxSteps: 12, MaxTokens: 4096, Deadline: now.Add(time.Minute), CreatedAt: now, Provenance: agent.Provenance{Provider: "scripted", Model: "test", PolicyVersion: "parent.v1", PromptDigest: "test-digest"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.executeAgentRun(ctx, structJob(tenant, runID)); err != nil {
		t.Fatal(err)
	}
	run, err := repo.GetRun(ctx, tenant, runID)
	if err != nil || run.Status != agent.RunAwaitingConfirmation || run.DurableStep != 0 {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	var unconfirmed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_revisions WHERE tenant_id=$1`, tenant).Scan(&unconfirmed); err != nil || unconfirmed != 0 {
		t.Fatalf("unconfirmed effects=%d err=%v", unconfirmed, err)
	}
	confirmAgentTestRun(t, s, tenant, actor, runID)
	var drafted int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_revisions WHERE tenant_id=$1 AND title='Scripted parent task' AND status='published'`, tenant).Scan(&drafted); err != nil {
		t.Fatal(err)
	}
	if drafted < 1 {
		t.Fatal("scripted tool loop did not create the drafted task")
	}
	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_run_events WHERE tenant_id=$1 AND run_id=$2`, tenant, runID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount < 8 {
		t.Fatalf("expected streamed thinking/tool/text progress, got %d events", eventCount)
	}
	rows, err := pool.Query(ctx, `SELECT payload::text FROM agent_run_events WHERE tenant_id=$1 AND run_id=$2`, tenant, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(payload), "hidden") || strings.Contains(strings.ToLower(payload), "secret reasoning") {
			t.Fatalf("raw reasoning persisted: %s", payload)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

type targetedToolModel struct {
	*scriptedParentModel
	publishID string
	toolName  string
	toolInput string
}

func (m *targetedToolModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.calls++
	if m.calls == 1 {
		name, input := m.toolName, m.toolInput
		if name == "" {
			name, input = "publish_task", `{"id":"`+m.publishID+`"}`
		}
		return scriptedToolStream(ctx, name, input), nil
	}
	return scriptedStream(ctx, "Published through the server-owned parent tool."), nil
}

func TestFantasyToolAdaptersExecuteTargetedMutation(t *testing.T) {
	pool := integrationPool(t)
	seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("targeted-tools"), IssuerSecret: []byte("targeted-tools")})
	ctx := context.Background()
	scope := parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a", IdempotencyKey: "targeted-tool"}
	services := phase3Services{s: s}
	draft, err := services.DraftTask(ctx, scope, parent.TaskDraftInput{Title: "Targeted publish", Instructions: "Publish this revision"})
	if err != nil {
		t.Fatal(err)
	}
	runID := seedAgentTestRun(t, s, tenantA, "parent-a")
	model := &targetedToolModel{scriptedParentModel: &scriptedParentModel{prompt: "targeted"}, publishID: draft.ID}
	runtime, err := agent.NewFantasyAgent(model, s.fantasyTools(tenantA, "parent-a", defaultToolNames(), runID), agent.Limits{MaxSteps: 3, MaxTokens: 1000, Deadline: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Execute(ctx, runID, "publish", func(agentprotocol.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	before, err := services.GetTask(ctx, scope, draft.ID)
	if err != nil || before.Status != "draft" {
		t.Fatalf("unconfirmed publish: %+v %v", before, err)
	}
	confirmAgentTestRun(t, s, tenantA, "parent-a", runID)
	published, err := services.GetTask(ctx, scope, draft.ID)
	if err != nil || published.Status != "published" {
		t.Fatalf("targeted tool task=%+v err=%v", published, err)
	}
}

func TestFantasyToolAdaptersCoverReadAndPreviewTools(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("read-tools"), IssuerSecret: []byte("read-tools")})
	ctx := context.Background()
	scope := parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a", IdempotencyKey: "create-schedule-tool"}
	services := phase3Services{s: s}
	draft, err := services.DraftTask(ctx, scope, parent.TaskDraftInput{Title: "Create schedule tool task", Instructions: "Schedule this"})
	if err != nil {
		t.Fatal(err)
	}
	published, err := services.PublishTask(ctx, scope, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	createInput := fmt.Sprintf(`{"studentId":%q,"studentName":"","templateId":%q,"revisionId":%q,"kind":"one_off","timezone":"UTC","startAt":%q,"rrule":"","dueOffsetMinutes":0}`, alice, published.TemplateID, published.ID, time.Now().UTC().Add(time.Hour).Format(time.RFC3339))
	for _, tc := range []struct{ name, input string }{
		{"list_schedules", `{"includeDisabled":true,"limit":10}`},
		{"list_occurrences", `{"status":"","limit":10}`},
		{"preview_action", fmt.Sprintf(`{"kind":"retire_task","targetIds":[%q],"payload":{"expectedVersion":1},"summary":"Preview only"}`, published.ID)},
		{"create_schedule", createInput},
	} {
		runID := seedAgentTestRun(t, s, tenantA, "parent-a")
		model := &targetedToolModel{scriptedParentModel: &scriptedParentModel{prompt: tc.name}, toolName: tc.name, toolInput: tc.input}
		runtime, err := agent.NewFantasyAgent(model, s.fantasyTools(tenantA, "parent-a", defaultToolNames(), runID), agent.Limits{MaxSteps: 2, MaxTokens: 1000, Deadline: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = runtime.Execute(ctx, runID, tc.name, func(agentprotocol.Event) error { return nil }); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
}

func TestAgentExecutionHelpersFailClosedAndMapProtocolEvents(t *testing.T) {
	if len(defaultToolNames()) < 10 {
		t.Fatal("active parent tool surface unexpectedly narrowed")
	}
	if _, err := safeToolJSON(map[string]string{"secret": "not returned"}, errors.New("provider credential")); err == nil {
		t.Fatal("tool error was reported as success")
	}
	response, err := safeToolJSON(map[string]string{"ok": "yes"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") {
		t.Fatalf("unsafe tool response=%s", b)
	}
	if _, err := (&Server{}).agentModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderDisabled}, "x"); !errors.Is(err, agent.ErrProviderDisabled) {
		t.Fatalf("disabled model=%v", err)
	}
}

// structJob keeps the execution test focused on the durable tenant/run key;
// executeAgentRun does not require a claimed lease for this direct path.
func structJob(tenant, run string) jobs.Job { return jobs.Job{TenantID: tenant, RunID: run} }
