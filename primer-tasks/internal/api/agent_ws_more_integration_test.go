package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"primer-tasks/internal/agent"
	agentprotocol "primer-tasks/internal/agent/protocol"
	"primer-tasks/internal/domain/parent"
)

func TestAgentCommandsCancelConfirmAndWorkerStartupUseDurableState(t *testing.T) {
	pool := integrationPool(t)
	seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("agent-commands"), IssuerSecret: []byte("agent-commands")})
	ctx := context.Background()
	serviceScope := parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a", IdempotencyKey: "command-test"}
	tools := s.parentTools()
	toolContext, err := parent.NewContext(tenantA, "parent-a", "command-test", []string{parent.ToolListTasks, parent.ToolDraftTask, parent.ToolPublishTask, parent.ToolPreviewAction, parent.ToolConfirmAction})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tools.DraftTask(ctx, toolContext, parent.TaskDraftInput{Title: "Command confirmation task", Instructions: "Retire after confirmation"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tools.PublishTask(ctx, toolContext, draft.ID); err != nil {
		t.Fatal(err)
	}
	preview, err := tools.PreviewRetireTask(ctx, toolContext, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.agentConfirm(ctx, scope{Tenant: serviceScope.TenantID, Subject: serviceScope.ActorID}, agentCommand{ConversationID: uuid.NewString(), ConfirmationID: preview.Handle})
	var taskStatus string
	if err = pool.QueryRow(ctx, `SELECT status FROM task_templates WHERE tenant_id=$1 AND id=$2`, tenantA, draft.TemplateID).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "retired" {
		t.Fatalf("confirmed task status=%s", taskStatus)
	}

	conversationID, messageID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	repo := agent.NewPostgresRepository(pool)
	now := time.Now().UTC()
	if err = repo.CreateConversation(ctx, agent.Conversation{ID: conversationID, TenantID: tenantA, ActorID: "parent-a", Status: agent.ConversationActive, PolicyVersion: "p", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, inserted, e := repo.AppendUserMessage(ctx, agent.Message{ID: messageID, TenantID: tenantA, ConversationID: conversationID, ClientMessageID: "cancel-command", Role: agent.RoleUser, Content: "cancel", Sequence: 1, CreatedAt: now}); e != nil || !inserted {
		t.Fatalf("message inserted=%v err=%v", inserted, e)
	}
	if err = repo.CreateRun(ctx, agent.Run{ID: runID, TenantID: tenantA, ConversationID: conversationID, UserMessageID: messageID, Status: agent.RunQueued, MaxSteps: 1, MaxTokens: 10, Deadline: now.Add(time.Minute), CreatedAt: now, Provenance: agent.Provenance{Provider: "scripted", Model: "test", PolicyVersion: "p", PromptDigest: "d"}}); err != nil {
		t.Fatal(err)
	}
	s.agentCancel(ctx, scope{Tenant: serviceScope.TenantID, Subject: serviceScope.ActorID}, agentCommand{RunID: runID})
	run, err := repo.GetRun(ctx, tenantA, runID)
	if err != nil || !run.CancelRequested || run.Status != agent.RunCancelRequested {
		t.Fatalf("cancelled run=%+v err=%v", run, err)
	}
	workerCtx, cancel := context.WithCancel(ctx)
	cancel()
	s.StartAgentWorker(workerCtx)
}

func TestScriptedProviderShapeAndProtocolMappingBranches(t *testing.T) {
	model := &scriptedParentModel{prompt: "retire this"}
	if model.Provider() != "scripted" || model.Model() == "" {
		t.Fatal("scripted provider metadata missing")
	}
	if _, err := model.Generate(context.Background(), fantasy.Call{}); err == nil {
		t.Fatal("Generate unexpectedly enabled")
	}
	if _, err := model.GenerateObject(context.Background(), fantasy.ObjectCall{}); err == nil {
		t.Fatal("GenerateObject unexpectedly enabled")
	}
	if _, err := model.StreamObject(context.Background(), fantasy.ObjectCall{}); err == nil {
		t.Fatal("StreamObject unexpectedly enabled")
	}
	if scriptedPreviewInput(fantasy.Call{}) == "" {
		t.Fatal("preview helper returned empty input")
	}
	if _, err := model.Stream(context.Background(), fantasy.Call{}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Server{}).agentModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderScripted}, "list"); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []parent.ProviderConfig{
		{Mode: parent.ProviderOpenRouter, Primary: parent.ProviderOpenRouter, Fallback: parent.ProviderDisabled, OpenRouterModel: ""},
		{Mode: parent.ProviderBedrock, Primary: parent.ProviderBedrock, Fallback: parent.ProviderDisabled, BedrockRegion: "us-east-1", BedrockModel: ""},
	} {
		if _, err := (&Server{}).agentModel(context.Background(), cfg, "list"); err == nil {
			t.Fatalf("unconfigured live provider %q accepted", cfg.Primary)
		}
	}
	for _, prompt := range []string{"ambiguous Alex", "retire this", "create and schedule", "list students", "inspect"} {
		fixture := &scriptedParentModel{prompt: prompt}
		for i := 0; i < 5; i++ {
			if _, err := fixture.Stream(context.Background(), fantasy.Call{}); err != nil {
				t.Fatalf("scripted prompt %q: %v", prompt, err)
			}
		}
	}
	if _, err := (&Server{}).agentModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderDisabled}, "list"); !errors.Is(err, agent.ErrProviderDisabled) {
		t.Fatalf("disabled model=%v", err)
	}

	// Each protocol event maps to the safe transport event kind; the default
	// branch is intentionally ignored rather than forwarding provider data.
	pool := integrationPool(t)
	seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("protocol-map"), IssuerSecret: []byte("protocol-map")})
	ctx := context.Background()
	conversationID, messageID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	repo := agent.NewPostgresRepository(pool)
	now := time.Now().UTC()
	if err := repo.CreateConversation(ctx, agent.Conversation{ID: conversationID, TenantID: tenantA, ActorID: "parent-a", Status: agent.ConversationActive, PolicyVersion: "p", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, inserted, err := repo.AppendUserMessage(ctx, agent.Message{ID: messageID, TenantID: tenantA, ConversationID: conversationID, ClientMessageID: "protocol-map", Role: agent.RoleUser, Content: "map", Sequence: 1, CreatedAt: now}); err != nil || !inserted {
		t.Fatalf("message inserted=%v err=%v", inserted, err)
	}
	if err := repo.CreateRun(ctx, agent.Run{ID: runID, TenantID: tenantA, ConversationID: conversationID, UserMessageID: messageID, Status: agent.RunQueued, MaxSteps: 1, MaxTokens: 10, Deadline: now.Add(time.Minute), CreatedAt: now, Provenance: agent.Provenance{Provider: "scripted", Model: "test", PolicyVersion: "p", PromptDigest: "d"}}); err != nil {
		t.Fatal(err)
	}
	for _, event := range []agentprotocol.Event{
		agentprotocol.TextStart(runID, 1), agentprotocol.TextDelta(runID, 2, "safe"), agentprotocol.TextEnd(runID, 3),
		agentprotocol.ThinkingStart(runID, 4), agentprotocol.ThinkingEnd(runID, 5), agentprotocol.ToolProgress(runID, 6, "List tasks", "completed"),
		agentprotocol.Retry(runID, 7, 1, time.Second), agentprotocol.Confirmation(runID, 8, "opaque", "Preview", time.Now().Add(time.Minute)),
		agentprotocol.Terminal(runID, 9, "completed"), agentprotocol.Error(runID, 10, "provider_unavailable"),
	} {
		if err := s.emitProtocolEvent(ctx, tenantA, conversationID, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.emitProtocolEvent(ctx, tenantA, conversationID, agentprotocol.Event{Kind: agentprotocol.EventKind("unknown"), RunID: runID, Sequence: 11}); err != nil {
		t.Fatal(err)
	}
}
