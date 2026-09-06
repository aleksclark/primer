package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/agent/protocol"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
)

func TestPublicAcknowledgementsUseAllP2MutationPaths(t *testing.T) {
	h := newAgentWSTest(t)
	c := h.socket("parent-a")
	conv := h.conversation("parent-a")
	stage := func(name, input string) wireAgentEvent {
		t.Helper()
		submitted := h.run(c, conv, uuid.NewString(), "Prepare "+name)
		model := &targetedToolModel{scriptedParentModel: &scriptedParentModel{}, toolName: name, toolInput: input}
		rt, err := agent.NewFantasyAgent(model, h.s.fantasyTools(tenantA, "parent-a", defaultToolNames(), submitted.RunID), agent.Limits{MaxSteps: 2, MaxTokens: 4096, Deadline: 5 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		// Carry the actual credential admitted by the public socket into the
		// scripted provider driver, just as executeAgentRun does in production.
		h.s.agentHub.mu.Lock()
		authority := h.s.agentHub.credentials[submitted.RunID]
		h.s.agentHub.mu.Unlock()
		if authority == nil {
			t.Fatal("public admission did not retain current authority")
		}
		executionContext := context.WithValue(h.ctx, agentAuthKey{}, authority)
		if _, err = rt.Execute(executionContext, submitted.RunID, "Prepare", func(e protocol.Event) error { return h.s.emitProtocolEvent(executionContext, tenantA, conv, e) }); err != nil {
			for _, tool := range h.s.fantasyTools(tenantA, "parent-a", defaultToolNames(), submitted.RunID) {
				if tool.Info().Name == name {
					schema, _ := json.Marshal(tool.Info().Parameters)
					t.Logf("fixture schema=%s input=%s", schema, input)
				}
			}
			t.Fatal(err)
		}
		return h.until(c, "tool_progress", "awaiting_confirmation")
	}
	confirm := func(p wireAgentEvent) {
		h.send(c, agentCommand{Type: "confirm", RunID: p.RunID, ConfirmationID: p.ConfirmationID})
		if v := h.until(c, "terminal", ""); v.Status != "completed" {
			t.Fatalf("confirm %+v", v)
		}
	}
	preview := stage("draft_task", `{"title":"Careful work","instructions":"Use the normal parent approval rules."}`)
	if h.count(`SELECT count(*) FROM task_revisions WHERE tenant_id=$1`, tenantA) != 0 {
		t.Fatal("unconfirmed draft")
	}
	confirm(preview)
	var taskID, templateID, studentID string
	if err := h.db.QueryRow(h.ctx, `SELECT id,template_id FROM task_revisions WHERE tenant_id=$1`, tenantA).Scan(&taskID, &templateID); err != nil {
		t.Fatal(err)
	}
	if err := h.db.QueryRow(h.ctx, `SELECT id FROM students WHERE tenant_id=$1`, tenantA).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	preview = stage("publish_task", fmt.Sprintf(`{"id":%q}`, taskID))
	if h.count(`SELECT count(*) FROM task_revisions WHERE id=$1 AND status='draft'`, taskID) != 1 {
		t.Fatal("unconfirmed publication")
	}
	confirm(preview)
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	preview = stage("create_schedule", fmt.Sprintf(`{"studentId":%q,"studentName":"","templateId":%q,"revisionId":%q,"kind":"one_off","timezone":"UTC","startAt":%q,"rrule":"","dueOffsetMinutes":0}`, studentID, templateID, taskID, start.Format(time.RFC3339)))
	if h.count(`SELECT count(*) FROM task_schedules WHERE tenant_id=$1`, tenantA) != 0 {
		t.Fatal("unconfirmed schedule")
	}
	confirm(preview)
	var scheduleID string
	if err := h.db.QueryRow(h.ctx, `SELECT id FROM task_schedules WHERE tenant_id=$1`, tenantA).Scan(&scheduleID); err != nil {
		t.Fatal(err)
	}
	update := parent.ScheduleUpdateInput{ScheduleID: scheduleID, ExpectedVersion: 999, ScheduleInput: parent.ScheduleInput{StudentID: studentID, TemplateID: templateID, RevisionID: taskID, Kind: "one_off", Timezone: "UTC", StartAt: start.Add(time.Hour), DueOffsetMinutes: 25}}
	b, _ := json.Marshal(update)
	preview = stage("update_schedule", string(b))
	if !strings.Contains(preview.Summary, "Alice") || !strings.Contains(preview.Summary, "end no end") {
		t.Fatalf("incomplete edit preview: %s", preview.Summary)
	}
	if h.count(`SELECT count(*) FROM task_schedules WHERE id=$1 AND version=1 AND due_offset_minutes=0`, scheduleID) != 1 {
		t.Fatal("unconfirmed edit")
	}
	confirm(preview)
	if h.count(`SELECT count(*) FROM task_schedules WHERE id=$1 AND version=2 AND due_offset_minutes=25`, scheduleID) != 1 {
		t.Fatal("confirmed CAS edit missing")
	}
	// Generic preview_action accepts model JSON. Alternate-case CAS aliases
	// and unknown reasoning fields must not replace the server-read version.
	payload := canonicalActionPayload(update)
	payload["expectedversion"] = 999
	payload["reasoning"] = "hidden provider field"
	generic, _ := json.Marshal(map[string]any{"kind": parent.ToolUpdateSchedule, "targetIds": []string{scheduleID}, "payload": payload, "summary": "not authoritative"})
	preview = stage("preview_action", string(generic))
	if h.count(`SELECT count(*) FROM parent_confirmation_previews WHERE run_id=$1 AND action->'payload'->>'expectedVersion'='2' AND NOT (action->'payload' ? 'reasoning')`, preview.RunID) != 1 {
		t.Fatal("model JSON overrode canonical CAS payload")
	}
	confirm(preview)
	if h.count(`SELECT count(*) FROM task_schedules WHERE id=$1 AND version=3`, scheduleID) != 1 {
		t.Fatal("canonical CAS confirmation failed")
	}
	// Updating a task uses the released editable-template/revision handler, not
	// an in-place update or a second SQL producer. Already-issued content stays.
	var originalSnapshot string
	if err := h.db.QueryRow(h.ctx, `SELECT revision_snapshot::text FROM task_occurrences WHERE schedule_id=$1 ORDER BY nominal_at LIMIT 1`, scheduleID).Scan(&originalSnapshot); err != nil {
		t.Fatal(err)
	}
	preview = stage("update_task", fmt.Sprintf(`{"id":%q,"title":"Careful revised work","instructions":"New instructions for future work."}`, taskID))
	if h.count(`SELECT count(*) FROM task_revisions WHERE template_id=$1`, templateID) != 1 {
		t.Fatal("revision created before confirmation")
	}
	confirm(preview)
	if h.count(`SELECT count(*) FROM task_revisions WHERE template_id=$1 AND version=2 AND status='draft' AND title='Careful revised work'`, templateID) != 1 {
		t.Fatal("confirmed revision missing")
	}
	var preservedSnapshot string
	if err := h.db.QueryRow(h.ctx, `SELECT revision_snapshot::text FROM task_occurrences WHERE schedule_id=$1 ORDER BY nominal_at LIMIT 1`, scheduleID).Scan(&preservedSnapshot); err != nil {
		t.Fatal(err)
	}
	if originalSnapshot != preservedSnapshot {
		t.Fatal("task revision changed issued snapshot")
	}
	// The old revision handle cannot append over a newer draft.
	preview = stage("update_task", fmt.Sprintf(`{"id":%q,"title":"Stale edit","instructions":"Must not apply."}`, taskID))
	h.send(c, agentCommand{Type: "confirm", RunID: preview.RunID, ConfirmationID: preview.ConfirmationID})
	h.until(c, "error", "")
	if h.count(`SELECT count(*) FROM task_revisions WHERE template_id=$1`, templateID) != 2 {
		t.Fatal("stale revision CAS bypassed")
	}
	// The direct ID read does not silently limit a large tenant to its first page.
	if _, err := h.db.Exec(h.ctx, `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) SELECT gen_random_uuid(),$1,$2,$3,$4,'one_off','UTC',now()-interval '2 days' FROM generate_series(1,105)`, tenantA, studentID, templateID, taskID); err != nil {
		t.Fatal(err)
	}
	row, err := (phase3Services{h.s}).GetSchedule(h.ctx, parent.ServiceContext{TenantID: tenantA}, scheduleID)
	if err != nil || row.ID != scheduleID {
		t.Fatalf("ID read beyond 100 rows: %+v %v", row, err)
	}
}

func TestPublicConfirmationFailureRollsBackEveryEffectThenRetries(t *testing.T) {
	h := newAgentWSTest(t)
	h.s.StartAgentWorker(h.ctx)
	conv := h.conversation("parent-a")
	c := h.socket("parent-a")
	h.run(c, conv, "atomic-effects", "Create and schedule a task.")
	p := h.until(c, "tool_progress", "awaiting_confirmation")
	if _, err := h.db.Exec(h.ctx, `CREATE FUNCTION reject_agent_schedule() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected schedule failure'; END $$; CREATE TRIGGER reject_agent_schedule BEFORE INSERT ON task_schedules FOR EACH ROW EXECUTE FUNCTION reject_agent_schedule()`); err != nil {
		t.Fatal(err)
	}
	h.send(c, agentCommand{Type: "confirm", RunID: p.RunID, ConfirmationID: p.ConfirmationID})
	h.until(c, "error", "")
	for _, table := range []string{"task_templates", "task_revisions", "agent_tool_effects"} {
		if h.count(`SELECT count(*) FROM `+table+` WHERE tenant_id=$1`, tenantA) != 0 {
			t.Fatalf("partial commit in %s", table)
		}
	}
	if h.count(`SELECT count(*) FROM parent_confirmation_previews WHERE run_id=$1 AND consumed_at IS NULL`, p.RunID) != 1 {
		t.Fatal("failure consumed handle")
	}
	if _, err := h.db.Exec(h.ctx, `DROP TRIGGER reject_agent_schedule ON task_schedules; DROP FUNCTION reject_agent_schedule()`); err != nil {
		t.Fatal(err)
	}
	h.send(c, agentCommand{Type: "confirm", RunID: p.RunID, ConfirmationID: p.ConfirmationID})
	h.until(c, "terminal", "")
	if h.count(`SELECT count(*) FROM task_schedules WHERE tenant_id=$1`, tenantA) != 1 {
		t.Fatal("retry failed to commit one schedule")
	}
}

func TestAgentExpiredAndUnownedWorkFailsWithoutProviderOrEffect(t *testing.T) {
	h := newAgentWSTest(t)
	conv := h.conversation("parent-a")
	c := h.socket("parent-a")
	submitted := h.run(c, conv, "expiry", "Create and schedule a task.")
	if _, err := h.db.Exec(h.ctx, `UPDATE agent_runs SET deadline=now()-interval '1 second' WHERE id=$1`, submitted.RunID); err != nil {
		t.Fatal(err)
	}
	h.s.reconcileAgentOutcomes(h.ctx)
	terminal := h.until(c, "terminal", "")
	if terminal.Code != "run_expired" || terminal.Status != "failed" {
		t.Fatalf("expiry %+v", terminal)
	}
	if h.count(`SELECT count(*) FROM task_revisions WHERE tenant_id=$1`, tenantA) != 0 {
		t.Fatal("expired work mutated state")
	}
	conv = h.conversation("parent-a")
	submitted = h.run(c, conv, "lease", "Create and schedule a task.")
	set, _ := parent.NewToolSet(defaultToolNames())
	toolCtx := parent.Context{TenantID: tenantA, ActorID: "parent-a", RunID: submitted.RunID, ToolStep: 1, IdempotencyKey: submitted.RunID, Tools: set}
	ctx := context.WithValue(h.ctx, agentLeaseKey{}, jobs.Job{ID: uuid.NewString(), TenantID: tenantA, RunID: submitted.RunID, LeaseOwner: "stale"})
	if _, err := h.s.stageAgentAction(ctx, toolCtx, parent.RetireTaskAction(uuid.NewString(), 1)); err == nil {
		t.Fatal("unowned worker issued a preview")
	}
	h.send(c, agentCommand{Type: "cancel", RunID: submitted.RunID})
	h.until(c, "terminal", "")
	if _, err := h.s.stageAgentAction(h.ctx, toolCtx, parent.RetireTaskAction(uuid.NewString(), 1)); err == nil {
		t.Fatal("canceled run issued a preview")
	}
	if h.count(`SELECT count(*) FROM parent_confirmation_previews WHERE run_id=$1`, submitted.RunID) != 0 {
		t.Fatal("unsafe previews persisted")
	}
	if err := h.s.publishAgent(h.ctx, tenantA, conv, wireAgentEvent{Type: "text_delta", RunID: submitted.RunID, Text: "late model output"}); err == nil {
		t.Fatal("model output appended after terminal cancellation")
	}
	if h.count(`SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND payload->>'text'='late model output'`, submitted.RunID) != 0 {
		t.Fatal("post-cancellation output persisted")
	}
	// Parent membership/session revocation cuts off an already-open socket.
	if _, err := h.db.Exec(h.ctx, `UPDATE bff_sessions SET revoked_at=now() WHERE handle_hash=$1`, hash("parent-a")); err != nil {
		t.Fatal(err)
	}
	ctx2, cancel := context.WithTimeout(h.ctx, 3*time.Second)
	defer cancel()
	_, _, err := c.Read(ctx2)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("revoked socket=%v", err)
	}
}
