package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/domain/parent"
)

// applyAgentAction returns semantic receipt clauses from the actual domain
// results, not a model prediction or the untrusted preview payload. The caller
// commits effects, these text events and the final message together: subscribers
// cannot see success until the same PostgreSQL transaction commits. No second
// inference/tool turn or fake paced typing is needed to report a confirmed fact.
func applyAgentAction(ctx context.Context, svc phase3Services, c parent.ServiceContext, a parent.Action) ([]string, error) {
	if len(a.TargetIDs) != 1 {
		return nil, parent.ErrConfirmationRejected
	}
	switch a.Kind {
	case parent.ActionRetireTask, parent.ActionDisableSchedule:
		var v struct {
			ExpectedVersion int `json:"expectedVersion"`
		}
		if err := decodeAction(a, &v); err != nil {
			return nil, err
		}
		if a.Kind == parent.ActionRetireTask {
			task, err := svc.RetireTask(ctx, c, a.TargetIDs[0], v.ExpectedVersion)
			if err != nil {
				return nil, err
			}
			return []string{fmt.Sprintf("Archived task %q. Previously assigned work is unchanged.", task.Title)}, nil
		}
		schedule, err := svc.DisableSchedule(ctx, c, a.TargetIDs[0], v.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return scheduleReceipt(ctx, svc, c, schedule, "Disabled")
	case parent.ToolDraftTask:
		var in parent.TaskDraftInput
		if err := decodeAction(a, &in); err != nil {
			return nil, err
		}
		task, err := svc.DraftTask(ctx, c, in)
		if err != nil {
			return nil, err
		}
		return []string{fmt.Sprintf("Created draft task %q.", task.Title)}, nil
	case parent.ToolUpdateTask:
		var in parent.TaskUpdateInput
		if err := decodeAction(a, &in); err != nil {
			return nil, err
		}
		in.TaskID = a.TargetIDs[0]
		task, err := svc.UpdateTask(ctx, c, in)
		if err != nil {
			return nil, err
		}
		return []string{fmt.Sprintf("Created draft revision %d of %q. Previously assigned work is unchanged.", task.Version, task.Title)}, nil
	case parent.ToolPublishTask:
		task, err := svc.PublishTask(ctx, c, a.TargetIDs[0])
		if err != nil {
			return nil, err
		}
		return []string{fmt.Sprintf("Published task %q, revision %d.", task.Title, task.Version)}, nil
	case parent.ToolCreateSchedule:
		var in parent.ScheduleInput
		if err := decodeAction(a, &in); err != nil {
			return nil, err
		}
		schedule, err := svc.CreateSchedule(ctx, c, in)
		if err != nil {
			return nil, err
		}
		return scheduleReceipt(ctx, svc, c, schedule, "Created")
	case parent.ToolUpdateSchedule:
		var in parent.ScheduleUpdateInput
		if err := decodeAction(a, &in); err != nil {
			return nil, err
		}
		in.ScheduleID = a.TargetIDs[0]
		schedule, err := svc.UpdateSchedule(ctx, c, in)
		if err != nil {
			return nil, err
		}
		return scheduleReceipt(ctx, svc, c, schedule, "Updated")
	case parent.ActionCreateTaskSchedule:
		var in struct {
			Title, Instructions, StudentID string
			StartAt                        time.Time
		}
		if err := decodeAction(a, &in); err != nil {
			return nil, err
		}
		task, err := svc.DraftTask(ctx, c, parent.TaskDraftInput{Title: in.Title, Instructions: in.Instructions})
		if err != nil {
			return nil, err
		}
		c.ToolStep++
		task, err = svc.PublishTask(ctx, c, task.ID)
		if err != nil {
			return nil, err
		}
		c.ToolStep++
		schedule, err := svc.CreateSchedule(ctx, c, parent.ScheduleInput{StudentID: in.StudentID, TemplateID: task.TemplateID, RevisionID: task.ID, Kind: "one_off", Timezone: "UTC", StartAt: in.StartAt})
		if err != nil {
			return nil, err
		}
		clauses, err := scheduleReceipt(ctx, svc, c, schedule, "Created")
		if err != nil {
			return nil, err
		}
		return append([]string{fmt.Sprintf("Created and published task %q.", task.Title)}, clauses...), nil
	default:
		return nil, parent.ErrConfirmationRejected
	}
}
func scheduleReceipt(ctx context.Context, svc phase3Services, c parent.ServiceContext, schedule parent.Schedule, verb string) ([]string, error) {
	// Read canonical names through the committed-effect IDs and tenant, including
	// archived students when disabling their old schedule. This is read-only;
	// all mutations above still use the ordinary validated Tasks handlers.
	var title, student string
	err := svc.s.DB.QueryRow(ctx, `SELECT r.title,st.display_name FROM task_revisions r JOIN students st ON st.tenant_id=r.tenant_id WHERE r.tenant_id=$1 AND r.id=$2 AND st.id=$3`, c.TenantID, schedule.RevisionID, schedule.StudentID).Scan(&title, &student)
	if err != nil {
		return nil, err
	}
	if !schedule.Enabled {
		return []string{fmt.Sprintf("%s the schedule for %q for %s. Previously assigned work is unchanged.", verb, title, student)}, nil
	}
	zone, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return nil, err
	}
	kind := "one-time"
	if schedule.Kind == "recurrence" {
		kind = "recurring"
	}
	return []string{fmt.Sprintf("%s a %s schedule for %q for %s, starting %s (%s).", verb, kind, title, student, schedule.StartAt.In(zone).Format("2006-01-02 15:04"), schedule.Timezone)}, nil
}

// Receipt frames are system-authored facts, explicitly labelled source=domain.
// The semantic clauses are streamed without invented token pacing; text_end is
// the authoritative replacement on replay, so the UI never appends it twice.
func (s *Server) recordConfirmedReceipt(ctx context.Context, sc scope, conversation, runID string, clauses []string) error {
	text := strings.Join(clauses, " ")
	repo := agent.NewPostgresRepository(s.DB)
	run, err := repo.GetRun(ctx, sc.Tenant, runID)
	if err != nil {
		return err
	}
	if err = repo.AppendMessage(ctx, agent.Message{ID: uuid.NewString(), TenantID: sc.Tenant, ConversationID: conversation, ClientMessageID: "confirmation:" + runID, Role: agent.RoleSystem, Content: text, Sequence: time.Now().UnixNano()}); err != nil {
		return err
	}
	if err = s.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "text_start", RunID: runID, Source: "domain"}); err != nil {
		return err
	}
	for i, clause := range clauses {
		if i > 0 {
			clause = " " + clause
		}
		if err = s.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "text_delta", RunID: runID, Text: clause, Source: "domain"}); err != nil {
			return err
		}
	}
	if err = s.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "text_end", RunID: runID, Text: text, Source: "domain"}); err != nil {
		return err
	}
	if err = repo.TransitionRun(ctx, sc.Tenant, runID, agent.RunSucceeded, run.DurableStep, run.Usage); err != nil {
		return err
	}
	return s.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "terminal", RunID: runID, Status: "completed", Message: "The confirmed parent command completed."})
}

func staleConfirmationMessage(action parent.Action) string {
	switch action.Kind {
	case parent.ToolUpdateSchedule, parent.ActionDisableSchedule:
		return "The schedule changed. Request a fresh preview. No changes from this request were applied."
	default:
		return "The task changed. Request a fresh preview. No changes from this request were applied."
	}
}
func (s *Server) recordStaleConfirmation(ctx context.Context, sc scope, conversation, runID string, action parent.Action) error {
	message := staleConfirmationMessage(action)
	repo := agent.NewPostgresRepository(s.DB)
	run, err := repo.GetRun(ctx, sc.Tenant, runID)
	if err != nil {
		return err
	}
	if err = s.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "error", RunID: runID, Code: "confirmation_stale", Message: message}); err != nil {
		return err
	}
	if err = repo.TransitionRun(ctx, sc.Tenant, runID, agent.RunFailed, run.DurableStep, run.Usage); err != nil {
		return err
	}
	return s.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "terminal", RunID: runID, Code: "confirmation_stale", Status: "failed", Message: message})
}
