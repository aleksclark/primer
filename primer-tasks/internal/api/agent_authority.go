package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
)

type agentLeaseKey struct{}

// The row lock serializes cancellation, confirmation and effects. The worker's
// database lease is checked at the actual write boundary, not just on a timer.
func lockAgentAuthority(ctx context.Context, tx pgx.Tx, tenant, actor, runID string) error {
	if err := lockAgentParent(ctx, tx, tenant, actor); err != nil {
		return err
	}
	if err := checkDurableAgentAuthorization(ctx, tx, tenant, actor, runID); err != nil {
		return err
	}
	var status string
	var canceled, member bool
	if err := tx.QueryRow(ctx, `SELECT r.status,r.cancel_requested,EXISTS(SELECT 1 FROM parent_memberships p WHERE p.tenant_id=r.tenant_id AND p.subject_ref=c.actor_id AND p.revoked_at IS NULL AND p.role='admin') FROM agent_runs r JOIN agent_conversations c ON c.tenant_id=r.tenant_id AND c.id=r.conversation_id WHERE r.tenant_id=$1 AND r.id=$2 AND c.actor_id=$3 AND c.status='active' AND r.deadline>now() FOR UPDATE OF r`, tenant, runID, actor).Scan(&status, &canceled, &member); err != nil {
		return parent.ErrInvalidContext
	}
	if canceled || !member || (status != "running" && status != "queued" && status != "awaiting_confirmation") {
		return parent.ErrInvalidContext
	}
	if job, ok := ctx.Value(agentLeaseKey{}).(jobs.Job); ok {
		if job.RunID != runID || job.TenantID != tenant {
			return parent.ErrInvalidContext
		}
		return lockJobLease(ctx, tx, job)
	}
	return nil
}

// Hold the lease row through commit. A takeover/expiry reconciliation cannot
// pass this boundary while an admitted effect or event is still committing.
func lockJobLease(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
	var id string
	if err := tx.QueryRow(ctx, `SELECT id FROM agent_jobs WHERE id=$1 AND run_id=$2 AND tenant_id=$3 AND status='running' AND lease_owner=$4 AND lease_until>clock_timestamp() FOR UPDATE`, job.ID, job.RunID, job.TenantID, job.LeaseOwner).Scan(&id); err != nil {
		return parent.ErrInvalidContext
	}
	return nil
}

func actionTools(kind string) []string {
	switch kind {
	case parent.ActionRetireTask:
		return []string{parent.ToolPublishTask}
	case parent.ActionDisableSchedule:
		return []string{parent.ToolUpdateSchedule}
	case parent.ActionCreateTaskSchedule:
		return []string{parent.ToolDraftTask, parent.ToolPublishTask, parent.ToolCreateSchedule}
	case parent.ToolDraftTask, parent.ToolUpdateTask, parent.ToolPublishTask, parent.ToolCreateSchedule, parent.ToolUpdateSchedule:
		return []string{kind}
	default:
		return nil
	}
}
func allowAction(active []string, kind string) error {
	set, err := parent.NewToolSet(active)
	if err != nil {
		return err
	}
	required := actionTools(kind)
	if len(required) == 0 {
		return parent.ErrToolUnavailable
	}
	required = append(required, parent.ToolPreviewAction, parent.ToolConfirmAction)
	for _, name := range required {
		if !set.Allows(name) {
			return parent.ErrToolUnavailable
		}
	}
	return nil
}

func (s *Server) stageAgentAction(ctx context.Context, c parent.Context, action parent.Action) (fantasy.ToolResponse, error) {
	if len(action.TargetIDs) != 1 {
		return safeToolJSON(nil, parent.ErrInvalidInput)
	}
	if action.Payload == nil {
		action.Payload = map[string]any{}
	}
	cfg, err := parent.LoadProviderConfig()
	if err != nil {
		return safeToolJSON(nil, err)
	}
	if cfg.Mode == parent.ProviderDisabled {
		return safeToolJSON(nil, parent.ErrToolUnavailable)
	}
	if err = allowAction(cfg.ActiveTools, action.Kind); err != nil {
		return safeToolJSON(nil, err)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return safeToolJSON(nil, err)
	}
	defer tx.Rollback(ctx)
	if err = lockAgentAuthority(ctx, tx, c.TenantID, c.ActorID, c.RunID); err != nil {
		return safeToolJSON(nil, err)
	}
	local := *s
	local.DB = tx
	svc := phase3Services{&local}
	scope := parent.ServiceContext{TenantID: c.TenantID, ActorID: c.ActorID}
	// Bind CAS to a server read. No model-supplied summary, version or tenant can
	// make a preview authorize a different resource version.
	summary := "Confirm " + action.Kind
	switch action.Kind {
	case parent.ActionCreateTaskSchedule:
		var in struct {
			Title, Instructions, StudentID string
			StartAt                        time.Time
		}
		if err = decodeAction(action, &in); err != nil || in.StudentID != action.TargetIDs[0] || strings.TrimSpace(in.Title) == "" || in.StartAt.IsZero() {
			return safeToolJSON(nil, parent.ErrInvalidInput)
		}
		student, e := svc.GetStudent(ctx, scope, in.StudentID)
		if e != nil {
			return safeToolJSON(nil, e)
		}
		action.Payload = map[string]any{"Title": in.Title, "Instructions": in.Instructions, "StudentID": student.ID, "StartAt": in.StartAt}
		summary = fmt.Sprintf("Create and publish %q for %s; schedule once at %s (UTC). Instructions: %s", in.Title, student.DisplayName, in.StartAt.UTC().Format(time.RFC3339), in.Instructions)
	case parent.ToolDraftTask:
		var in parent.TaskDraftInput
		if err = decodeAction(action, &in); err != nil || strings.TrimSpace(in.Title) == "" {
			return safeToolJSON(nil, parent.ErrInvalidInput)
		}
		action.Payload = map[string]any{"Title": in.Title, "Instructions": in.Instructions}
		summary = fmt.Sprintf("Create draft %q. Instructions: %s", in.Title, in.Instructions)
	case parent.ToolUpdateTask:
		var in parent.TaskUpdateInput
		if err = decodeAction(action, &in); err != nil || strings.TrimSpace(in.Title) == "" {
			return safeToolJSON(nil, parent.ErrInvalidInput)
		}
		row, e := svc.GetTask(ctx, scope, action.TargetIDs[0])
		if e != nil {
			return safeToolJSON(nil, e)
		}
		in.TaskID, in.ExpectedVersion, in.Requirements = row.ID, row.Version, row.Requirements
		action.Payload = canonicalActionPayload(in)
		summary = fmt.Sprintf("Create a new draft revision of %q from revision %d, titled %q. Instructions: %s. Verification requirements and issued work remain unchanged.", row.Title, row.Version, in.Title, in.Instructions)
	case parent.ToolPublishTask:
		row, e := svc.GetTask(ctx, scope, action.TargetIDs[0])
		if e != nil {
			return safeToolJSON(nil, e)
		}
		action.Payload = map[string]any{}
		summary = fmt.Sprintf("Publish draft %q, revision %d. Instructions: %s", row.Title, row.Version, row.Instructions)
	case parent.ActionDisableSchedule:
		if len(action.TargetIDs) != 1 {
			return safeToolJSON(nil, parent.ErrInvalidInput)
		}
		row, e := svc.GetSchedule(ctx, scope, action.TargetIDs[0])
		if e != nil {
			return safeToolJSON(nil, e)
		}
		action.Payload = map[string]any{"expectedVersion": row.Version}
		task, e := svc.GetTask(ctx, scope, row.RevisionID)
		if e != nil {
			return safeToolJSON(nil, e)
		}
		summary = fmt.Sprintf("Disable schedule for %q, version %d, starting %s in %s. Issued occurrences remain unchanged.", task.Title, row.Version, row.StartAt.UTC().Format(time.RFC3339), row.Timezone)
	case parent.ActionRetireTask:
		if len(action.TargetIDs) != 1 {
			return safeToolJSON(nil, parent.ErrInvalidInput)
		}
		row, e := svc.GetTask(ctx, scope, action.TargetIDs[0])
		if e != nil {
			return safeToolJSON(nil, e)
		}
		action.Payload = map[string]any{"expectedVersion": row.Version}
		summary = fmt.Sprintf("Retire task %q at revision %d. Issued occurrences remain unchanged.", row.Title, row.Version)
	case parent.ToolCreateSchedule:
		var in parent.ScheduleInput
		if err = decodeAction(action, &in); err != nil {
			return safeToolJSON(nil, err)
		}
		student, e := svc.GetStudent(ctx, scope, in.StudentID)
		if e != nil {
			return safeToolJSON(nil, e)
		}
		task, e := svc.GetTask(ctx, scope, in.RevisionID)
		if e != nil {
			return safeToolJSON(nil, e)
		}
		end := "no end"
		if in.EndAt != nil {
			end = in.EndAt.UTC().Format(time.RFC3339)
		}
		action.Payload = canonicalActionPayload(in)
		summary = fmt.Sprintf("Schedule %q (revision %d) for %s, kind %s, starting %s; end %s; timezone %s; recurrence %q; due offset %d minutes.", task.Title, task.Version, student.DisplayName, in.Kind, in.StartAt.UTC().Format(time.RFC3339), end, in.Timezone, in.RRULE, in.DueOffsetMinutes)
	case parent.ToolUpdateSchedule:
		if len(action.TargetIDs) != 1 {
			return safeToolJSON(nil, parent.ErrInvalidInput)
		}
		row, e := svc.GetSchedule(ctx, scope, action.TargetIDs[0])
		if e != nil {
			return safeToolJSON(nil, e)
		}
		var in parent.ScheduleUpdateInput
		if err = decodeAction(action, &in); err != nil {
			return safeToolJSON(nil, err)
		}
		// Assign after decoding: encoding/json accepts case-insensitive field
		// aliases, so editing one key in an untrusted map is not a CAS fence.
		in.ExpectedVersion = row.Version
		task, e := svc.GetTask(ctx, scope, in.RevisionID)
		if e != nil {
			return safeToolJSON(nil, e)
		}
		student, e := svc.GetStudent(ctx, scope, in.StudentID)
		if e != nil {
			return safeToolJSON(nil, e)
		}
		end := "no end"
		if in.EndAt != nil {
			end = in.EndAt.UTC().Format(time.RFC3339)
		}
		action.Payload = canonicalActionPayload(in)
		summary = fmt.Sprintf("Update schedule for %q (revision %d) assigned to %s, from schedule version %d: kind %s; start %s; end %s; timezone %s; recurrence %q; due offset %d minutes. Issued occurrences remain unchanged.", task.Title, task.Version, student.DisplayName, row.Version, in.Kind, in.StartAt.UTC().Format(time.RFC3339), end, in.Timezone, in.RRULE, in.DueOffsetMinutes)
	}
	if len(summary) > 4096 {
		return safeToolJSON(nil, parent.ErrInvalidInput)
	}
	preview, err := local.parentTools().PreviewAction(ctx, c, action, summary+" No change has been applied.")
	if err != nil {
		return safeToolJSON(nil, err)
	}
	local.agentHub = nil
	if _, err = tx.Exec(ctx, `UPDATE agent_runs SET status='awaiting_confirmation',updated_at=now() WHERE tenant_id=$1 AND id=$2`, c.TenantID, c.RunID); err != nil {
		return safeToolJSON(nil, err)
	}
	var conversation string
	if err = tx.QueryRow(ctx, `SELECT conversation_id FROM agent_runs WHERE tenant_id=$1 AND id=$2`, c.TenantID, c.RunID).Scan(&conversation); err != nil {
		return safeToolJSON(nil, err)
	}
	if err = local.publishAgent(ctx, c.TenantID, conversation, wireAgentEvent{Type: "tool_progress", RunID: c.RunID, Tool: "Prepare change", Phase: "awaiting_confirmation", ConfirmationID: preview.Handle, Summary: preview.Summary, ExpiresAt: preview.ExpiresAt}); err != nil {
		return safeToolJSON(nil, err)
	}
	if err = tx.Commit(ctx); err != nil {
		return safeToolJSON(nil, err)
	}
	return safeToolJSON(preview, nil)
}

func (s *Server) confirmAgentAction(ctx context.Context, sc scope, cmd agentCommand) error {
	cfg, err := parent.LoadProviderConfig()
	if err != nil || cfg.Mode == parent.ProviderDisabled {
		return parent.ErrToolUnavailable
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var conversation, status string
	var canceled bool
	if err = tx.QueryRow(ctx, `SELECT r.conversation_id,r.status,r.cancel_requested FROM agent_runs r JOIN agent_conversations c ON c.tenant_id=r.tenant_id AND c.id=r.conversation_id WHERE r.tenant_id=$1 AND r.id=$2 AND c.actor_id=$3 FOR UPDATE OF r`, sc.Tenant, cmd.RunID, sc.Subject).Scan(&conversation, &status, &canceled); err != nil {
		return parent.ErrConfirmationForeign
	}
	if cmd.ConversationID != "" && cmd.ConversationID != conversation {
		return parent.ErrConfirmationForeign
	}
	var payload []byte
	var digest string
	var step int
	var consumed *time.Time
	if err = tx.QueryRow(ctx, `SELECT action,encode(action_digest,'hex'),tool_step,consumed_at FROM parent_confirmation_previews WHERE handle_hash=$1 AND tenant_id=$2 AND actor_id=$3 AND run_id=$4 FOR UPDATE`, parentHandleHash(cmd.ConfirmationID), sc.Tenant, sc.Subject, cmd.RunID).Scan(&payload, &digest, &step, &consumed); err != nil {
		return parent.ErrConfirmationForeign
	}
	var action parent.Action
	if json.Unmarshal(payload, &action) != nil {
		return parent.ErrConfirmationAltered
	}
	if err = allowAction(cfg.ActiveTools, action.Kind); err != nil {
		return err
	}
	if err = lockAgentParent(ctx, tx, sc.Tenant, sc.Subject); err != nil {
		return err
	}
	if err = checkDurableAgentAuthorization(ctx, tx, sc.Tenant, sc.Subject, cmd.RunID); err != nil {
		return err
	}
	// Identical acknowledgements observe the same committed result. They do not
	// consume a fresh key or append another event.
	if consumed != nil && status == "succeeded" && !canceled {
		return nil
	}
	if err = lockAgentAuthority(ctx, tx, sc.Tenant, sc.Subject, cmd.RunID); err != nil {
		return err
	}
	if status != "awaiting_confirmation" {
		return parent.ErrConfirmationRejected
	}
	local := *s
	local.DB, local.agentHub = tx, nil
	if _, err = (&parent.SQLConfirmationStore{DB: tx}).Consume(ctx, cmd.ConfirmationID, sc.Tenant, sc.Subject, digest); err != nil {
		return err
	}
	svc := phase3Services{&local}
	c := parent.ServiceContext{TenantID: sc.Tenant, ActorID: sc.Subject, RunID: cmd.RunID, ToolStep: step, IdempotencyKey: cmd.RunID}
	err = applyAgentAction(ctx, svc, c, action)
	if err != nil {
		return err
	}
	if err = local.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "tool_progress", RunID: cmd.RunID, Tool: "Confirm change", Phase: "completed", Summary: "Confirmed change applied through the Tasks service."}); err != nil {
		return err
	}
	if err = agent.NewPostgresRepository(tx).TransitionRun(ctx, sc.Tenant, cmd.RunID, agent.RunSucceeded, step, agent.Usage{}); err != nil {
		return err
	}
	if err = local.publishAgent(ctx, sc.Tenant, conversation, wireAgentEvent{Type: "terminal", RunID: cmd.RunID, Status: "completed", Message: "The confirmed parent command completed."}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Only typed domain values, never the raw model payload, are persisted.
func canonicalActionPayload(value any) map[string]any {
	b, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func decodeAction(action parent.Action, out any) error {
	b, err := json.Marshal(action.Payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
func applyAgentAction(ctx context.Context, svc phase3Services, c parent.ServiceContext, a parent.Action) error {
	if len(a.TargetIDs) != 1 {
		return parent.ErrConfirmationRejected
	}
	var err error
	switch a.Kind {
	case parent.ActionRetireTask, parent.ActionDisableSchedule:
		var v struct {
			ExpectedVersion int `json:"expectedVersion"`
		}
		if err = decodeAction(a, &v); err != nil {
			return err
		}
		if a.Kind == parent.ActionRetireTask {
			_, err = svc.RetireTask(ctx, c, a.TargetIDs[0], v.ExpectedVersion)
		} else {
			_, err = svc.DisableSchedule(ctx, c, a.TargetIDs[0], v.ExpectedVersion)
		}
	case parent.ToolDraftTask:
		var in parent.TaskDraftInput
		if err = decodeAction(a, &in); err == nil {
			_, err = svc.DraftTask(ctx, c, in)
		}
	case parent.ToolUpdateTask:
		var in parent.TaskUpdateInput
		if err = decodeAction(a, &in); err == nil {
			in.TaskID = a.TargetIDs[0]
			_, err = svc.UpdateTask(ctx, c, in)
		}
	case parent.ToolPublishTask:
		_, err = svc.PublishTask(ctx, c, a.TargetIDs[0])
	case parent.ToolCreateSchedule:
		var in parent.ScheduleInput
		if err = decodeAction(a, &in); err == nil {
			_, err = svc.CreateSchedule(ctx, c, in)
		}
	case parent.ToolUpdateSchedule:
		var in parent.ScheduleUpdateInput
		if err = decodeAction(a, &in); err == nil {
			in.ScheduleID = a.TargetIDs[0]
			_, err = svc.UpdateSchedule(ctx, c, in)
		}
	case parent.ActionCreateTaskSchedule:
		var in struct {
			Title, Instructions, StudentID string
			StartAt                        time.Time
		}
		if err = decodeAction(a, &in); err != nil {
			return err
		}
		task, e := svc.DraftTask(ctx, c, parent.TaskDraftInput{Title: in.Title, Instructions: in.Instructions})
		if e != nil {
			return e
		}
		c.ToolStep++
		task, e = svc.PublishTask(ctx, c, task.ID)
		if e != nil {
			return e
		}
		c.ToolStep++
		_, err = svc.CreateSchedule(ctx, c, parent.ScheduleInput{StudentID: in.StudentID, TemplateID: task.TemplateID, RevisionID: task.ID, Kind: "one_off", Timezone: "UTC", StartAt: in.StartAt})
	default:
		err = errors.New("unsupported action")
	}
	return err
}
