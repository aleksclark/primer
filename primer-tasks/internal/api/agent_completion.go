package api

import (
	"context"
	"github.com/google/uuid"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/jobs"
	"time"
)

// Finish commits the safe assistant message, run state and terminal frame in
// one transaction. A restart can never see a terminal run without its event.
func (s *Server) finishAgentRun(ctx context.Context, run agent.Run, to agent.RunStatus, status, message, code, text string, usage agent.Usage) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	repo := agent.NewPostgresRepository(tx)
	var current agent.RunStatus
	if err = tx.QueryRow(ctx, `SELECT status FROM agent_runs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, run.TenantID, run.ID).Scan(&current); err != nil {
		return err
	}
	if current.Terminal() {
		return nil
	}
	if job, ok := ctx.Value(agentLeaseKey{}).(jobs.Job); ok {
		if err = lockJobLease(ctx, tx, job); err != nil {
			return err
		}
	}
	if text != "" {
		if err = repo.AppendMessage(ctx, agent.Message{ID: uuid.NewString(), TenantID: run.TenantID, ConversationID: run.ConversationID, ClientMessageID: "assistant:" + run.ID, Role: agent.RoleAssistant, Content: text, Sequence: time.Now().UnixNano()}); err != nil {
			return err
		}
	}
	local := *s
	local.DB, local.agentHub = tx, nil
	// The final text is an authoritative replacement, not another delta. A
	// bounded browser replay can recover a complete final answer even when its
	// earliest token frames are no longer resident in the client window.
	if text != "" {
		if err = local.publishAgent(ctx, run.TenantID, run.ConversationID, wireAgentEvent{Type: "text_end", RunID: run.ID, Text: text}); err != nil {
			return err
		}
	}
	if err = repo.TransitionRun(ctx, run.TenantID, run.ID, to, run.DurableStep, usage); err != nil {
		return err
	}
	if err = local.publishAgent(ctx, run.TenantID, run.ConversationID, wireAgentEvent{Type: "terminal", RunID: run.ID, Status: status, Message: message, Code: code}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Pending confirmations and exhausted jobs must become durable terminal
// outcomes even when the worker died before publishing a terminal frame.
func (s *Server) reconcileAgentOutcomes(ctx context.Context) {
	rows, err := s.DB.Query(ctx, `SELECT r.tenant_id,r.id FROM agent_runs r WHERE r.status IN ('queued','running','awaiting_confirmation','cancel_requested') AND (r.deadline<now() OR EXISTS(SELECT 1 FROM agent_jobs j WHERE j.tenant_id=r.tenant_id AND j.run_id=r.id AND j.status='failed')) LIMIT 100`)
	if err != nil {
		return
	}
	type key struct{ tenant, id string }
	var ids []key
	for rows.Next() {
		var x key
		if rows.Scan(&x.tenant, &x.id) == nil {
			ids = append(ids, x)
		}
	}
	rows.Close()
	for _, x := range ids {
		run, e := agent.NewPostgresRepository(s.DB).GetRun(ctx, x.tenant, x.id)
		if e == nil {
			_ = s.finishAgentRun(ctx, run, agent.RunFailed, "failed", "The run expired or exhausted its retry limit.", "run_expired", "", run.Usage)
		}
	}
}
