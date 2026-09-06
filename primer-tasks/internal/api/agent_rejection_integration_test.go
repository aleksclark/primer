package api

import (
	"encoding/json"
	"github.com/google/uuid"
	"primer-tasks/internal/domain/parent"
	"strings"
	"testing"
	"time"
)

func TestAgentProposalValidationCannotPersistInvalidOrDisallowedActions(t *testing.T) {
	h := newAgentWSTest(t)
	run := seedAgentTestRun(t, h.s, tenantA, "parent-a")
	set, _ := parent.NewToolSet(defaultToolNames())
	ctx := parent.Context{TenantID: tenantA, ActorID: "parent-a", RunID: run, ToolStep: 1, IdempotencyKey: run, Tools: set}
	cases := []parent.Action{
		{Kind: parent.ToolDraftTask},
		{Kind: "unknown", TargetIDs: []string{"new"}},
		{Kind: parent.ActionRetireTask, TargetIDs: []string{uuid.NewString()}},
		{Kind: parent.ActionDisableSchedule, TargetIDs: []string{uuid.NewString()}},
		{Kind: parent.ToolUpdateSchedule, TargetIDs: []string{uuid.NewString()}},
		{Kind: parent.ToolPublishTask, TargetIDs: []string{uuid.NewString()}},
		{Kind: parent.ToolDraftTask, TargetIDs: []string{"new"}, Payload: map[string]any{"Title": 1}},
		{Kind: parent.ToolDraftTask, TargetIDs: []string{"new"}, Payload: map[string]any{"Title": strings.Repeat("x", 5000)}},
		{Kind: parent.ActionCreateTaskSchedule, TargetIDs: []string{uuid.NewString()}, Payload: map[string]any{"Title": "foreign", "StudentID": uuid.NewString(), "StartAt": time.Now().UTC()}},
		{Kind: parent.ToolCreateSchedule, TargetIDs: []string{uuid.NewString()}, Payload: map[string]any{"StartAt": "not-a-time"}},
	}
	for _, action := range cases {
		result, err := h.s.stageAgentAction(h.ctx, ctx, action)
		if err == nil || !result.IsError {
			t.Fatalf("invalid action %s accepted", action.Kind)
		}
	}
	if h.count(`SELECT count(*) FROM parent_confirmation_previews WHERE run_id=$1`, run) != 0 {
		t.Fatal("invalid proposal persisted")
	}
	t.Setenv("TASKS_AGENT_ACTIVE_TOOLS", "list_students")
	if _, err := h.s.stageAgentAction(h.ctx, ctx, parent.Action{Kind: parent.ToolDraftTask, TargetIDs: []string{"new"}, Payload: map[string]any{"Title": "disallowed"}}); err == nil {
		t.Fatal("disabled tool proposal accepted")
	}
	t.Setenv("TASKS_AGENT_MODE", "disabled")
	if _, err := h.s.stageAgentAction(h.ctx, ctx, parent.RetireTaskAction(uuid.NewString(), 1)); err == nil {
		t.Fatal("disabled model issued preview")
	}
	if h.count(`SELECT count(*) FROM task_revisions WHERE tenant_id=$1`, tenantA) != 0 {
		t.Fatal("invalid proposal changed domain")
	}
	var conversation string
	if err := h.db.QueryRow(h.ctx, `SELECT conversation_id FROM agent_runs WHERE id=$1`, run).Scan(&conversation); err != nil {
		t.Fatal(err)
	}
	if err := h.s.publishAgent(h.ctx, tenantA, conversation, wireAgentEvent{Type: "reasoning_delta", RunID: run, Text: "raw hidden reasoning"}); err == nil {
		t.Fatal("raw reasoning variant accepted")
	}
	if err := h.s.publishAgent(h.ctx, tenantA, conversation, wireAgentEvent{Type: "text_delta", RunID: run, Text: strings.Repeat("x", 32769)}); err == nil {
		t.Fatal("unbounded event accepted")
	}
	if err := h.s.publishAgent(h.ctx, tenantA, conversation, wireAgentEvent{Type: "thinking_start", RunID: run, Text: "raw hidden reasoning", Summary: "raw hidden reasoning"}); err != nil {
		t.Fatal(err)
	}
	if h.count(`SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND payload::text LIKE '%raw hidden reasoning%'`, run) != 0 {
		t.Fatal("reasoning marker contained content")
	}
	// Unknown action payload fields cannot escape a bound confirmation digest.
	var action parent.Action
	if err := json.Unmarshal([]byte(`{"kind":"retire_task","targetIds":["x"],"payload":{"expectedVersion":"bad"}}`), &action); err != nil {
		t.Fatal(err)
	}
	if err := applyAgentAction(h.ctx, phase3Services{h.s}, parent.ServiceContext{TenantID: tenantA}, action); err == nil {
		t.Fatal("ill-typed confirmed version accepted")
	}
}
