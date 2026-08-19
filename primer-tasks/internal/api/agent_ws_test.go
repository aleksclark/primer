package api

import (
	"context"
	"testing"
	"time"

	"charm.land/fantasy"
	"primer-tasks/internal/agent"
	agentprotocol "primer-tasks/internal/agent/protocol"
	"primer-tasks/internal/domain/parent"
)

func TestScriptedToolResultJSONReadsFantasyTextResult(t *testing.T) {
	call := fantasy.Call{Prompt: fantasy.Prompt{
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.ToolCallPart{ToolCallID: "call-1", ToolName: "draft_task"}}},
		{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{fantasy.ToolResultPart{ToolCallID: "call-1", Output: fantasy.ToolResultOutputContentText{Text: `{"id":"revision-1","templateId":"template-1"}`}}}},
	}}
	result := toolResultJSON(call, "draft_task")
	if result["id"] != "revision-1" || result["templateId"] != "template-1" {
		t.Fatalf("text result was not decoded: %#v", result)
	}
}

func TestScriptedModelCarriesFantasyToolResultsAcrossSteps(t *testing.T) {
	var publishedID, scheduledStudent, scheduledRevision string
	tools := []fantasy.AgentTool{
		fantasy.NewAgentTool("draft_task", "draft", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse(`{"id":"revision-1","templateId":"template-1"}`), nil
		}),
		fantasy.NewAgentTool("publish_task", "publish", func(_ context.Context, in struct {
			ID string `json:"id"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			publishedID = in.ID
			return fantasy.NewTextResponse(`{"id":"revision-1","templateId":"template-1"}`), nil
		}),
		fantasy.NewAgentTool("list_students", "students", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse(`[{"id":"student-1"}]`), nil
		}),
		fantasy.NewAgentTool("create_schedule", "schedule", func(_ context.Context, in struct {
			StudentID  string `json:"studentId"`
			RevisionID string `json:"revisionId"`
		}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			scheduledStudent, scheduledRevision = in.StudentID, in.RevisionID
			return fantasy.NewTextResponse(`{"id":"schedule-1"}`), nil
		}),
	}
	runtime, err := agent.NewFantasyAgent(&scriptedParentModel{prompt: "create a task"}, tools, agent.Limits{MaxSteps: 5, MaxTokens: 100, Deadline: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Execute(context.Background(), "run-1", "create a task", func(agentprotocol.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if publishedID != "revision-1" || scheduledStudent != "student-1" || scheduledRevision != "revision-1" {
		t.Fatalf("scripted Fantasy loop lost tool output: publish=%q student=%q revision=%q", publishedID, scheduledStudent, scheduledRevision)
	}
}

func TestParentScheduleToolCommitsThroughPhase3Service(t *testing.T) {
	pool := integrationPool(t)
	student, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("test-session"), IssuerSecret: []byte("test-issuer")})
	set, err := parent.NewToolSet(defaultToolNames())
	if err != nil {
		t.Fatal(err)
	}
	ctx := parent.Context{TenantID: tenantA, ActorID: "parent-a", IdempotencyKey: "schedule-tool-test", Tools: set}
	tools := s.parentTools()
	draft, err := tools.DraftTask(context.Background(), ctx, parent.TaskDraftInput{Title: "Agent schedule test", Instructions: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	published, err := tools.PublishTask(context.Background(), ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := tools.CreateSchedule(context.Background(), ctx, parent.ScheduleInput{StudentID: student, TemplateID: published.TemplateID, RevisionID: published.ID, Kind: "one_off", Timezone: "UTC", StartAt: time.Now().UTC().Add(time.Hour)}, "")
	if err != nil {
		t.Fatal(err)
	}
	if schedule.ID == "" || schedule.StudentID != student || schedule.RevisionID != published.ID {
		t.Fatalf("unexpected persisted schedule: %#v", schedule)
	}
}

func TestFantasyToolsAdvertiseOnlyConfiguredAllowlist(t *testing.T) {
	s := &Server{}
	tools := s.fantasyTools("tenant-a", "parent-a", []string{"list_students"}, "")
	if len(tools) != 1 || tools[0].Info().Name != "list_students" {
		t.Fatalf("advertised tools=%v", toolNames(tools))
	}
	if got := s.fantasyTools("tenant-a", "parent-a", []string{"not-a-tool"}, ""); got != nil {
		t.Fatalf("unknown allowlist opened tools: %v", toolNames(got))
	}
}

func toolNames(tools []fantasy.AgentTool) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Info().Name)
	}
	return out
}
