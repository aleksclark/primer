package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func confirmedAgentFixture(t *testing.T) (*agentWSTest, *websocket.Conn, string, wireAgentEvent) {
	t.Helper()
	h := newAgentWSTest(t)
	h.s.StartAgentWorker(h.ctx)
	conv := h.conversation("parent-a")
	c := h.socket("parent-a")
	h.run(c, conv, "receipt-proposal", "Create and schedule a task.")
	preview := h.until(c, "tool_progress", "awaiting_confirmation")
	if h.count(`SELECT count(*) FROM task_templates`)+h.count(`SELECT count(*) FROM task_schedules`) != 0 {
		t.Fatal("proposal changed Tasks before confirmation")
	}
	if h.count(`SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND payload->>'source'='domain'`, preview.RunID) != 0 {
		t.Fatal("proposal claimed a confirmed receipt")
	}
	return h, c, conv, preview
}

func TestPublicConfirmationStreamsCommittedCanonicalReceiptAndReplaysOnce(t *testing.T) {
	h, c, conv, preview := confirmedAgentFixture(t)
	var studentID string
	if err := h.db.QueryRow(h.ctx, `SELECT id FROM students WHERE tenant_id=$1`, tenantA).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	// Rename through the public handler after preview. A receipt copied from the
	// proposal would have the obsolete name; actual canonical results must win.
	renamed := requestJSON(t, h.s.Routes(), http.MethodPatch, "/students/"+studentID, "parent-a", `{"displayName":"Alice current name"}`)
	if renamed.Code != 200 {
		t.Fatal("public student rename failed")
	}
	h.send(c, agentCommand{Type: "confirm", RunID: preview.RunID, ConfirmationID: preview.ConfirmationID})
	var final, deltas string
	starts, ends := 0, 0
	for {
		e := h.read(c)
		if e.RunID != preview.RunID {
			continue
		}
		if e.Source == "domain" {
			// The actual public HTTP read must see committed effects by the first text
			// frame. Events and the effect are not separately committed promises.
			req, _ := http.NewRequestWithContext(h.ctx, "GET", h.http.URL+"/tasks", nil)
			req.AddCookie(&http.Cookie{Name: "tasks_parent", Value: "parent-a"})
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			var page TaskPage2
			err = json.NewDecoder(resp.Body).Decode(&page)
			_ = resp.Body.Close()
			if err != nil || resp.StatusCode != 200 || page.TotalCount != 1 || page.Items[0].Status != "published" {
				t.Fatal("receipt arrived before committed public task")
			}
			if h.count(`SELECT count(*) FROM task_schedules`) != 1 {
				t.Fatal("receipt arrived before committed schedule")
			}
			switch e.Type {
			case "text_start":
				starts++
			case "text_delta":
				deltas += e.Text
			case "text_end":
				ends++
				final = e.Text
			}
		}
		if e.Type == "terminal" {
			if e.Status != "completed" {
				t.Fatal("confirmation did not complete")
			}
			break
		}
	}
	if starts != 1 || ends != 1 || deltas != final || !strings.Contains(final, `Created and published task "Scripted parent task"`) || !strings.Contains(final, "Alice current name") || !strings.Contains(final, "one-time schedule") {
		t.Fatalf("incomplete canonical receipt starts=%d ends=%d text=%q", starts, ends, final)
	}
	var stored string
	if err := h.db.QueryRow(h.ctx, `SELECT content FROM agent_messages WHERE conversation_id=$1 AND client_message_id=$2 AND role='system'`, conv, "confirmation:"+preview.RunID).Scan(&stored); err != nil || stored != final {
		t.Fatal("final canonical receipt was not durable/system-authored")
	}
	replay := h.socket("parent-a")
	h.send(replay, agentCommand{Type: "subscribe", ConversationID: conv, Cursor: preview.Cursor})
	var replayText string
	for {
		e := h.read(replay)
		if e.Source == "domain" {
			if e.Type == "text_delta" {
				replayText += e.Text
			}
			if e.Type == "text_end" {
				replayText = e.Text
			}
		}
		if e.Type == "terminal" && e.RunID == preview.RunID {
			break
		}
	}
	if replayText != final {
		t.Fatal("authoritative replay duplicated or lost final text")
	}
	before := h.count(`SELECT count(*) FROM agent_run_events WHERE run_id=$1`, preview.RunID)
	for i := 0; i < 2; i++ {
		h.send(c, agentCommand{Type: "confirm", RunID: preview.RunID, ConfirmationID: preview.ConfirmationID})
	}
	h.send(c, agentCommand{Type: "unknown_receipt_sentinel"})
	h.until(c, "error", "")
	if h.count(`SELECT count(*) FROM agent_run_events WHERE run_id=$1`, preview.RunID) != before || h.count(`SELECT count(*) FROM agent_tool_effects WHERE run_id=$1 AND status='applied'`, preview.RunID) != 3 || h.count(`SELECT count(*) FROM agent_messages WHERE client_message_id=$1`, "confirmation:"+preview.RunID) != 1 {
		t.Fatal("duplicate confirmation duplicated a receipt/effect")
	}
}

func TestPublicFailedOrCanceledConfirmationNeverCommitsSuccessReceipt(t *testing.T) {
	h, c, conv, preview := confirmedAgentFixture(t)
	// Fail the final event write, after receipt/domain work has been attempted.
	// This proves the whole transaction rolls back, not just the visible label.
	_, err := h.db.Exec(h.ctx, `CREATE FUNCTION fail_receipt_terminal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='terminal' AND NEW.payload->>'status'='completed' THEN RAISE EXCEPTION 'injected receipt commit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_receipt_terminal BEFORE INSERT ON agent_run_events FOR EACH ROW EXECUTE FUNCTION fail_receipt_terminal()`)
	if err != nil {
		t.Fatal(err)
	}
	h.send(c, agentCommand{Type: "confirm", RunID: preview.RunID, ConfirmationID: preview.ConfirmationID})
	h.until(c, "error", "")
	for _, q := range []string{`SELECT count(*) FROM task_templates`, `SELECT count(*) FROM task_schedules`, `SELECT count(*) FROM agent_run_events WHERE payload->>'source'='domain'`, `SELECT count(*) FROM agent_messages WHERE role='system'`} {
		if h.count(q) != 0 {
			t.Fatal("failed commit exposed success or partial effect")
		}
	}
	if h.count(`SELECT count(*) FROM parent_confirmation_previews WHERE run_id=$1 AND consumed_at IS NULL`, preview.RunID) != 1 {
		t.Fatal("transient failure incorrectly retired preview")
	}
	if _, err = h.db.Exec(h.ctx, `DROP TRIGGER fail_receipt_terminal ON agent_run_events; DROP FUNCTION fail_receipt_terminal()`); err != nil {
		t.Fatal(err)
	}
	h.send(c, agentCommand{Type: "cancel", RunID: preview.RunID})
	h.until(c, "terminal", "")
	h.send(c, agentCommand{Type: "confirm", RunID: preview.RunID, ConfirmationID: preview.ConfirmationID})
	h.until(c, "error", "")
	if h.count(`SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND payload->>'source'='domain'`, preview.RunID) != 0 {
		t.Fatal("canceled effect claimed a success receipt")
	}
	h.run(c, conv, "fresh-after-failure", "Create and schedule a task.")
	fresh := h.until(c, "tool_progress", "awaiting_confirmation")
	h.send(c, agentCommand{Type: "confirm", RunID: fresh.RunID, ConfirmationID: fresh.ConfirmationID})
	h.until(c, "terminal", "")
	if h.count(`SELECT count(*) FROM task_schedules`) != 1 {
		t.Fatal("fresh proposal failed after rollback/cancel")
	}
}

func TestPublicStaleScheduleRetiresOnlyObsoletePreviewAndFreshProposalSucceeds(t *testing.T) {
	h, c, conv, created := confirmedAgentFixture(t)
	h.send(c, agentCommand{Type: "confirm", RunID: created.RunID, ConfirmationID: created.ConfirmationID})
	h.until(c, "terminal", "")
	h.run(c, conv, "stale-disable", "Disable schedule.")
	stale := h.until(c, "tool_progress", "awaiting_confirmation")
	page := requestJSON(t, h.s.Routes(), "GET", "/schedules", "parent-a", "")
	var schedules SchedulePage2
	if json.Unmarshal(page.Body.Bytes(), &schedules) != nil || len(schedules.Items) != 1 {
		t.Fatal("public schedule missing")
	}
	schedule := schedules.Items[0]
	input := ScheduleInput2{StudentID: schedule.StudentID, TemplateID: schedule.TemplateID, RevisionID: schedule.RevisionID, Kind: schedule.Kind, Timezone: schedule.Timezone, StartAt: schedule.StartAt.Add(time.Hour)}
	body, _ := json.Marshal(input)
	if updated := requestJSON(t, h.s.Routes(), "PATCH", "/schedules/"+schedule.ID, "parent-a", string(body)); updated.Code != 200 {
		t.Fatal("public schedule CAS edit failed")
	}
	h.send(c, agentCommand{Type: "confirm", RunID: stale.RunID, ConfirmationID: stale.ConfirmationID})
	e := h.until(c, "error", "")
	if e.Code != "confirmation_stale" || !strings.Contains(e.Text, "schedule changed") || !strings.Contains(e.Text, "fresh preview") {
		t.Fatalf("stale error not actionable: %+v", e)
	}
	terminal := h.read(c)
	if terminal.Type != "terminal" || terminal.Status != "failed" || terminal.RunID != stale.RunID {
		t.Fatal("stale preview lacks immediate durable terminal state")
	}
	if h.count(`SELECT count(*) FROM parent_confirmation_previews WHERE run_id=$1 AND consumed_at IS NOT NULL`, stale.RunID) != 1 || h.count(`SELECT count(*) FROM task_schedules WHERE id=$1 AND enabled AND version=2`, schedule.ID) != 1 || h.count(`SELECT count(*) FROM agent_tool_effects WHERE run_id=$1`, stale.RunID) != 0 {
		t.Fatal("stale preview was reusable or changed current schedule")
	}
	h.secondActor()
	foreign := h.socket("parent-a2")
	h.send(foreign, agentCommand{Type: "confirm", RunID: stale.RunID, ConfirmationID: stale.ConfirmationID})
	h.send(foreign, agentCommand{Type: "unknown_foreign_sentinel"})
	if e := h.read(foreign); e.Type != "error" || e.Code != "unknown_command" || e.RunID != "" {
		t.Fatal("stale detail leaked to another actor")
	}
	h.run(c, conv, "fresh-disable", "Disable schedule.")
	fresh := h.until(c, "tool_progress", "awaiting_confirmation")
	h.send(c, agentCommand{Type: "confirm", RunID: stale.RunID, ConfirmationID: stale.ConfirmationID})
	h.until(c, "error", "")
	h.send(c, agentCommand{Type: "confirm", RunID: fresh.RunID, ConfirmationID: fresh.ConfirmationID})
	h.until(c, "terminal", "")
	if h.count(`SELECT count(*) FROM task_schedules WHERE id=$1 AND NOT enabled AND version=3`, schedule.ID) != 1 {
		t.Fatal("fresh version-bound proposal did not succeed")
	}
	if h.count(`SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND payload->>'source'='domain'`, stale.RunID) != 0 {
		t.Fatal("stale request claimed success")
	}
}
