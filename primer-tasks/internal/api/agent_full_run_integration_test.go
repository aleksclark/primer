package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"primer-tasks/internal/agent"
	"primer-tasks/internal/jobs"
)

// The donor expected an unconfirmed create. The integrated public contract
// instead requires a durable preview and proves the same three domain effects
// commit exactly once after a freshly authenticated acknowledgement, even
// after worker replacement. Pending proposals themselves confer no authority.
func TestScriptedFantasyDurableRunCreatesAndSchedulesThroughServices(t *testing.T) {
	h := newAgentWSTest(t)
	conv := h.conversation("parent-a")
	c := h.socket("parent-a")
	submitted := h.run(c, conv, "restart-preview", "Create and schedule a task.")
	repository := jobs.NewPostgresRepository(h.db)
	job, ok, err := repository.Claim(h.ctx, "worker-before", time.Minute, 4)
	if err != nil || !ok {
		t.Fatalf("claim %v %v", ok, err)
	}
	if err = h.s.executeAgentRun(h.ctx, job); err != nil {
		t.Fatal(err)
	}
	preview := h.until(c, "tool_progress", "awaiting_confirmation")
	if preview.RunID != submitted.RunID {
		t.Fatal("preview on wrong run")
	}
	run, err := agent.NewPostgresRepository(h.db).GetRun(h.ctx, tenantA, submitted.RunID)
	if err != nil || run.Status != agent.RunAwaitingConfirmation {
		t.Fatalf("pending state %+v %v", run, err)
	}
	if h.count(`SELECT count(*) FROM task_schedules WHERE tenant_id=$1`, tenantA) != 0 {
		t.Fatal("unconfirmed schedule")
	}
	// Simulate a lease whose owner process is gone, preserving the database and
	// websocket. A fresh worker must not rerun the model or duplicate the preview.
	if _, err = h.db.Exec(h.ctx, `UPDATE agent_jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	fresh := New(h.db, "test")
	fresh.StartAgentWorker(h.ctx)
	h.awaitCount(`SELECT count(*) FROM agent_jobs WHERE id=$1 AND status='done'`, 1, job.ID)
	if h.count(`SELECT count(*) FROM parent_confirmation_previews WHERE run_id=$1`, run.ID) != 1 {
		t.Fatal("restart duplicated preview")
	}
	h.send(c, agentCommand{Type: "confirm", RunID: run.ID, ConfirmationID: preview.ConfirmationID})
	terminal := h.until(c, "terminal", "")
	if terminal.Status != "completed" {
		t.Fatalf("terminal %+v", terminal)
	}
	if h.count(`SELECT count(*) FROM task_schedules WHERE tenant_id=$1`, tenantA) != 1 {
		t.Fatal("confirmed schedule missing")
	}
	if h.count(`SELECT count(*) FROM task_revisions WHERE tenant_id=$1 AND status='published'`, tenantA) != 1 {
		t.Fatal("confirmed publication missing")
	}
	// Durable wire and transcript contain no provider reasoning or error bodies.
	rows, err := h.db.Query(context.Background(), `SELECT payload::text FROM agent_run_events WHERE run_id=$1 UNION ALL SELECT content FROM agent_messages WHERE conversation_id=$2`, run.ID, conv)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var text string
		if err = rows.Scan(&text); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, "hidden") || strings.Contains(text, "secret reasoning") {
			t.Fatal("raw reasoning persisted")
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
}
