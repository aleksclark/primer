package api

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/agent/protocol"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
)

func TestDurableToolEffectLedgerReplaysAndRejectsChangedMutation(t *testing.T) {
	pool := integrationPool(t)
	seedIntegration(t, pool)
	ctx := context.Background()
	runID, conversationID, messageID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM agent_run_events WHERE tenant_id=$1`, tenantA)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_tool_effects WHERE tenant_id=$1`, tenantA)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_runs WHERE tenant_id=$1`, tenantA)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_messages WHERE tenant_id=$1`, tenantA)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_conversations WHERE tenant_id=$1`, tenantA)
		_, _ = pool.Exec(ctx, `DELETE FROM task_schedules WHERE tenant_id=$1 AND student_id IN (SELECT id FROM students WHERE tenant_id=$1 AND display_name='ledger student')`, tenantA)
		_, _ = pool.Exec(ctx, `DELETE FROM students WHERE tenant_id=$1 AND display_name='ledger student'`, tenantA)
		_, _ = pool.Exec(ctx, `DELETE FROM task_templates WHERE tenant_id=$1 AND title LIKE 'ledger-test-%'`, tenantA)
	})
	store := agent.NewPostgresRepository(pool)
	if err := store.CreateConversation(ctx, agent.Conversation{ID: conversationID, TenantID: tenantA, ActorID: "parent-a", Status: agent.ConversationActive, PolicyVersion: "p", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendUserMessage(ctx, agent.Message{ID: messageID, TenantID: tenantA, ConversationID: conversationID, ClientMessageID: "ledger-test", Role: agent.RoleUser, Content: "create", Sequence: 1, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, agent.Run{ID: runID, TenantID: tenantA, ConversationID: conversationID, UserMessageID: messageID, Status: agent.RunRunning, MaxSteps: 4, MaxTokens: 100, Deadline: now.Add(time.Hour), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := phase3Services{&Server{DB: pool}}
	if replay, err := svc.beginToolEffect(ctx, parent.ServiceContext{}, parent.ToolDraftTask, map[string]string{"title": "unit"}); err != nil || replay != nil {
		t.Fatalf("non-durable ledger=%v %v", replay, err)
	}
	if replay, err := svc.beginToolEffect(ctx, parent.ServiceContext{RunID: "test-run"}, parent.ToolDraftTask, map[string]string{"title": "unit"}); err != nil || replay != nil {
		t.Fatalf("invalid durable ledger=%v %v", replay, err)
	}
	if err := svc.completeToolEffect(ctx, parent.ServiceContext{RunID: "test-run"}, parent.ToolDraftTask, nil, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	toolCtx := parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a", IdempotencyKey: runID, RunID: runID, ToolStep: 1}
	first, err := svc.DraftTask(ctx, toolCtx, parent.TaskDraftInput{Title: "ledger-test-first", Instructions: "once"})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.DraftTask(ctx, toolCtx, parent.TaskDraftInput{Title: "ledger-test-first", Instructions: "once"})
	if err != nil || replay.ID != first.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	if _, err = svc.DraftTask(ctx, toolCtx, parent.TaskDraftInput{Title: "ledger-test-changed", Instructions: "must fail closed"}); !errors.Is(err, errToolEffectInProgress) {
		t.Fatalf("changed replay err=%v", err)
	}
	var effects, templates int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM agent_tool_effects WHERE run_id=$1`, runID).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM task_templates WHERE tenant_id=$1 AND title LIKE 'ledger-test-%'`, tenantA).Scan(&templates); err != nil {
		t.Fatal(err)
	}
	if effects != 1 || templates != 1 {
		t.Fatalf("ledger effects=%d templates=%d", effects, templates)
	}
	server := &Server{DB: pool, agentHub: newAgentHub()}
	if err := server.executeAgentRun(ctx, jobs.Job{TenantID: tenantA, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM agent_runs WHERE id=$1`, runID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	var terminals int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND event_type='terminal'`, runID).Scan(&terminals); err != nil {
		t.Fatal(err)
	}
	if status != string(agent.RunFailed) || terminals != 1 {
		t.Fatalf("durable boundary status=%s terminals=%d", status, terminals)
	}
	// A second mutation boundary has the same replay and changed-input rules.
	studentID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'ledger student')`, studentID, tenantA); err != nil {
		t.Fatal(err)
	}
	toolCtx.ToolStep = 2
	published, err := svc.PublishTask(ctx, toolCtx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	replayedPublished, err := svc.PublishTask(ctx, toolCtx, first.ID)
	if err != nil || replayedPublished.ID != published.ID {
		t.Fatalf("publish replay=%+v err=%v", replayedPublished, err)
	}
	toolCtx.ToolStep = 3
	scheduleInput := parent.ScheduleInput{StudentID: studentID, TemplateID: published.TemplateID, RevisionID: published.ID, Kind: "one_off", Timezone: "UTC", StartAt: now.Add(time.Hour)}
	schedule, err := svc.CreateSchedule(ctx, toolCtx, scheduleInput)
	if err != nil {
		t.Fatal(err)
	}
	replayedSchedule, err := svc.CreateSchedule(ctx, toolCtx, scheduleInput)
	if err != nil || replayedSchedule.ID != schedule.ID {
		t.Fatalf("schedule replay=%+v err=%v", replayedSchedule, err)
	}
	scheduleInput.StartAt = now.Add(2 * time.Hour)
	if _, err = svc.CreateSchedule(ctx, toolCtx, scheduleInput); !errors.Is(err, errToolEffectInProgress) {
		t.Fatalf("changed schedule replay err=%v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM task_schedules WHERE tenant_id=$1 AND student_id=$2`, tenantA, studentID).Scan(&templates); err != nil {
		t.Fatal(err)
	}
	if templates != 1 {
		t.Fatalf("schedule effects=%d", templates)
	}
	toolCtx.ToolStep = 4
	reservedDigest := effectDigest(parent.ToolDraftTask, map[string]string{"title": "reserved"})
	if _, err = pool.Exec(ctx, `INSERT INTO agent_tool_effects(tenant_id,run_id,step,tool_name,action_digest,status) VALUES($1,$2,$3,$4,$5,'reserved')`, tenantA, runID, toolCtx.ToolStep, parent.ToolDraftTask, reservedDigest); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.DraftTask(ctx, toolCtx, parent.TaskDraftInput{Title: "reserved", Instructions: "safe"}); !errors.Is(err, errToolEffectInProgress) {
		t.Fatalf("reserved replay err=%v", err)
	}
}

func TestPostgresAgentRepositoryTransitionsLeasesReplayRestartAndConfirm(t *testing.T) {
	pool := integrationPool(t)
	seedIntegration(t, pool)
	ctx := context.Background()
	tenant := tenantA
	conversationID, messageID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	cleanupAgent := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM agent_run_events WHERE tenant_id=$1`, tenant)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_jobs WHERE tenant_id=$1`, tenant)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_confirmation_previews WHERE tenant_id=$1`, tenant)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_runs WHERE tenant_id=$1`, tenant)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_messages WHERE tenant_id=$1`, tenant)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_conversations WHERE tenant_id=$1`, tenant)
	}
	t.Cleanup(cleanupAgent)
	store := agent.NewPostgresRepository(pool)
	if err := store.CreateConversation(ctx, agent.Conversation{ID: conversationID, TenantID: tenant, ActorID: "parent-a", Status: agent.ConversationActive, PolicyVersion: "parent.v1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.GetConversation(ctx, tenant, conversationID)
	if err != nil || conversation.ActorID != "parent-a" {
		t.Fatalf("conversation=%+v err=%v", conversation, err)
	}
	if _, err = store.GetConversation(ctx, tenantB, conversationID); !errors.Is(err, agent.ErrNotFound) {
		t.Fatalf("foreign conversation=%v", err)
	}
	if err = store.AppendMessage(ctx, agent.Message{ID: uuid.NewString(), TenantID: tenant, ConversationID: conversationID, Role: agent.RoleUser, Content: "wrong API", Sequence: 99}); err == nil {
		t.Fatal("user message bypassed idempotent append")
	}
	if err = store.AppendMessage(ctx, agent.Message{ID: uuid.NewString(), TenantID: tenant, ConversationID: conversationID, Role: agent.RoleAssistant, Content: "assistant", Sequence: 2}); err != nil {
		t.Fatal(err)
	}
	message, inserted, err := store.AppendUserMessage(ctx, agent.Message{ID: messageID, TenantID: tenant, ConversationID: conversationID, ClientMessageID: "client-retry", Content: "list students", Sequence: 1, CreatedAt: now})
	if err != nil || !inserted || message.ID != messageID {
		t.Fatalf("first message=%+v inserted=%v err=%v", message, inserted, err)
	}
	duplicate, inserted, err := store.AppendUserMessage(ctx, agent.Message{ID: uuid.NewString(), TenantID: tenant, ConversationID: conversationID, ClientMessageID: "client-retry", Content: "different text", Sequence: 9})
	if err != nil || inserted || duplicate.ID != messageID || duplicate.Content != "list students" {
		t.Fatalf("idempotent retry=%+v inserted=%v err=%v", duplicate, inserted, err)
	}
	if err := store.CreateRun(ctx, agent.Run{ID: uuid.NewString(), TenantID: tenant, ConversationID: conversationID, UserMessageID: messageID, Status: agent.RunQueued, MaxSteps: 0, MaxTokens: 1, Deadline: now.Add(time.Hour)}); err == nil {
		t.Fatal("invalid run limits accepted by repository")
	}
	run := agent.Run{ID: runID, TenantID: tenant, ConversationID: conversationID, UserMessageID: messageID, Status: agent.RunQueued, MaxSteps: 4, MaxTokens: 100, Deadline: now.Add(time.Hour), Provenance: agent.Provenance{Provider: "scripted", Model: "test", PolicyVersion: "parent.v1", PromptDigest: "digest"}, CreatedAt: now}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetRun(ctx, tenantB, runID); !errors.Is(err, agent.ErrNotFound) {
		t.Fatalf("foreign run lookup=%v", err)
	}
	if err := store.TransitionRun(ctx, tenant, runID, agent.RunRunning, 1, agent.Usage{InputTokens: 2}); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestCancel(ctx, tenant, runID); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRun(ctx, tenant, runID)
	if err != nil || got.Status != agent.RunRunning || !got.CancelRequested || got.DurableStep != 1 {
		t.Fatalf("cancel request=%+v err=%v", got, err)
	}
	if err := store.TransitionRun(ctx, tenant, runID, agent.RunCanceled, 2, agent.Usage{TotalTokens: 3}); err != nil {
		t.Fatal(err)
	}
	if err := store.TransitionRun(ctx, tenant, runID, agent.RunSucceeded, 3, agent.Usage{}); !errors.Is(err, agent.ErrAlreadyTerminal) {
		t.Fatalf("terminal transition=%v", err)
	}

	// A leased run can be picked up by a new worker after the old process is
	// considered dead. The conditional claim is also the retry/restart boundary.
	restartID := uuid.NewString()
	if err := store.CreateRun(ctx, agent.Run{ID: restartID, TenantID: tenant, ConversationID: conversationID, UserMessageID: messageID, Status: agent.RunQueued, MaxSteps: 2, MaxTokens: 20, Deadline: now.Add(time.Hour), Provenance: agent.Provenance{Provider: "scripted", Model: "test", PolicyVersion: "p", PromptDigest: "d"}, CreatedAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	leased, ok, err := store.LeaseRun(ctx, tenant, "worker-one", time.Minute)
	if err != nil || !ok || leased.ID != restartID || leased.Attempt != 1 {
		t.Fatalf("first lease=%+v ok=%v err=%v", leased, ok, err)
	}
	if _, ok, err := store.LeaseRun(ctx, tenant, "worker-two", time.Minute); err != nil || ok {
		t.Fatalf("active lease was stolen ok=%v err=%v", ok, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET lease_until=now()-interval '1 second' WHERE id=$1`, restartID); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileExpiredLeases(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	reclaimed, ok, err := store.LeaseRun(ctx, tenant, "worker-two", time.Minute)
	if err != nil || !ok || reclaimed.ID != restartID || reclaimed.Attempt != 2 {
		t.Fatalf("reclaimed lease=%+v ok=%v err=%v", reclaimed, ok, err)
	}
	if err := store.AppendEvent(ctx, agent.RunEvent{RunID: runID, TenantID: tenant, Sequence: 1, EventType: string(protocol.EventTextStart), Payload: agent.JSON(protocol.TextStart(runID, 1)), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	// Sequence conflict is deliberately idempotent for a replaying publisher.
	if err := store.AppendEvent(ctx, agent.RunEvent{RunID: runID, TenantID: tenant, Sequence: 1, EventType: "different", Payload: []byte(`{"different":true}`), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(ctx, agent.RunEvent{RunID: runID, TenantID: tenant, Sequence: 2, EventType: string(protocol.EventTerminal), Payload: agent.JSON(protocol.Terminal(runID, 2, "failed")), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReplayEvents(ctx, tenant, conversationID, 0, 10)
	if err != nil || len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("replay=%+v err=%v", events, err)
	}
	cursor, err := store.ReplayEvents(ctx, tenant, conversationID, 1, 10)
	if err != nil || len(cursor) != 1 || cursor[0].Sequence != 2 {
		t.Fatalf("cursor replay=%+v err=%v", cursor, err)
	}
	foreign, err := store.ReplayEvents(ctx, tenantB, conversationID, 0, 10)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign replay=%+v err=%v", foreign, err)
	}

	preview := agent.ConfirmationPreview{ID: uuid.NewString(), TenantID: tenant, ActorID: "parent-a", Action: "retire_task", ActionDigest: strings.Repeat("a", 64), Payload: agent.JSON(map[string]string{"id": "task"}), ExpiresAt: now.Add(5 * time.Minute)}
	if err := store.PutPreview(ctx, preview); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumePreview(ctx, preview, now); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumePreview(ctx, preview, now); !errors.Is(err, agent.ErrPreviewExpired) {
		t.Fatalf("preview replay=%v", err)
	}
	if err := store.PutPreview(ctx, agent.ConfirmationPreview{ID: uuid.NewString(), TenantID: tenant, ActorID: "parent-a", Action: "retire_task", ActionDigest: strings.Repeat("b", 64), Payload: []byte(`{}`), ExpiresAt: now.Add(5 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresJobsClaimCompleteFailAndRequeue(t *testing.T) {
	pool := integrationPool(t)
	seedIntegration(t, pool)
	ctx := context.Background()
	tenant := tenantA
	conversationID, messageID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM agent_jobs WHERE tenant_id=$1`, tenant)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_runs WHERE tenant_id=$1`, tenant)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_messages WHERE tenant_id=$1`, tenant)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_conversations WHERE tenant_id=$1`, tenant)
	})
	store := agent.NewPostgresRepository(pool)
	if err := store.CreateConversation(ctx, agent.Conversation{ID: conversationID, TenantID: tenant, ActorID: "parent-a", Status: agent.ConversationActive, PolicyVersion: "p", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendUserMessage(ctx, agent.Message{ID: messageID, TenantID: tenant, ConversationID: conversationID, ClientMessageID: "job-message", Role: agent.RoleUser, Content: "run", Sequence: 1, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, agent.Run{ID: runID, TenantID: tenant, ConversationID: conversationID, UserMessageID: messageID, Status: agent.RunQueued, MaxSteps: 1, MaxTokens: 10, Deadline: now.Add(time.Hour), Provenance: agent.Provenance{Provider: "scripted", Model: "test", PolicyVersion: "p", PromptDigest: "d"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	jobsStore := jobs.NewPostgresRepository(pool)
	if err := jobsStore.Enqueue(ctx, jobs.Job{ID: uuid.NewString(), TenantID: tenant, RunID: runID, MaxAttempts: 2, AvailableAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := jobsStore.Enqueue(ctx, jobs.Job{ID: uuid.NewString(), TenantID: tenant, RunID: runID, MaxAttempts: 0, AvailableAt: now}); err == nil {
		t.Fatal("invalid max attempts accepted")
	}
	claimed, ok, err := jobsStore.Claim(ctx, "worker-a", time.Minute, 4)
	if err != nil || !ok || claimed.Attempts != 1 || claimed.Status != jobs.Running || claimed.LeaseOwner != "worker-a" {
		t.Fatalf("claim=%+v ok=%v err=%v", claimed, ok, err)
	}
	if err := jobsStore.Renew(ctx, claimed.ID, "worker-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := jobsStore.Renew(ctx, claimed.ID, "wrong-owner", time.Minute); err == nil {
		t.Fatal("wrong owner renewed job")
	}
	if err := jobsStore.Complete(ctx, claimed.ID, "wrong-owner"); err != nil {
		t.Fatal(err)
	}
	if err := jobsStore.Fail(ctx, claimed.ID, "worker-a", errors.New("provider secret must not persist")); err != nil {
		t.Fatal(err)
	}
	var status, lastError string
	if err := pool.QueryRow(ctx, `SELECT status,last_error FROM agent_jobs WHERE id=$1`, claimed.ID).Scan(&status, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != string(jobs.Queued) || lastError != "job_failed" || strings.Contains(lastError, "secret") {
		t.Fatalf("failed job status=%s error=%s", status, lastError)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_jobs SET available_at=now() WHERE id=$1`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err = jobsStore.Claim(ctx, "worker-b", time.Minute, 4)
	if err != nil || !ok || claimed.Attempts != 2 {
		t.Fatalf("retry claim=%+v ok=%v err=%v", claimed, ok, err)
	}
	if err := jobsStore.Fail(ctx, claimed.ID, "worker-b", nil); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM agent_jobs WHERE id=$1`, claimed.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(jobs.Failed) {
		t.Fatalf("exhausted job status=%s", status)
	}

	// A leased job from a dead worker is requeued while attempts remain.
	run2, msg2, conv2 := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if err := store.CreateConversation(ctx, agent.Conversation{ID: conv2, TenantID: tenant, ActorID: "parent-a", Status: agent.ConversationActive, PolicyVersion: "p", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendUserMessage(ctx, agent.Message{ID: msg2, TenantID: tenant, ConversationID: conv2, ClientMessageID: "job-message-2", Content: "run", Sequence: 1, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, agent.Run{ID: run2, TenantID: tenant, ConversationID: conv2, UserMessageID: msg2, Status: agent.RunQueued, MaxSteps: 1, MaxTokens: 10, Deadline: now.Add(time.Hour), Provenance: agent.Provenance{Provider: "scripted", Model: "test", PolicyVersion: "p", PromptDigest: "d"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	job2 := uuid.NewString()
	if err := jobsStore.Enqueue(ctx, jobs.Job{ID: job2, TenantID: tenant, RunID: run2, MaxAttempts: 3, AvailableAt: now}); err != nil {
		t.Fatal(err)
	}
	claimed2, ok, err := jobsStore.Claim(ctx, "dead-worker", time.Minute, 4)
	if err != nil || !ok || claimed2.ID != job2 {
		t.Fatalf("second claim=%+v ok=%v err=%v", claimed2, ok, err)
	}
	if err := pool.QueryRow(ctx, `UPDATE agent_jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, job2).Scan(); err == nil {
		t.Fatal("unexpected row scan from update")
	}
	if err := jobsStore.RequeueExpired(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var owner *string
	if err := pool.QueryRow(ctx, `SELECT status,lease_owner FROM agent_jobs WHERE id=$1`, job2).Scan(&status, &owner); err != nil {
		t.Fatal(err)
	}
	if status != string(jobs.Queued) || owner != nil {
		t.Fatalf("requeued status=%s", status)
	}
}

func TestParentToolsUseTenantScopedDomainServicesAndSQLConfirmation(t *testing.T) {
	pool := integrationPool(t)
	seedIntegration(t, pool)
	ctx := context.Background()
	tools := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p3-tools"), IssuerSecret: []byte("p3-tools")}).parentTools()
	a, err := parent.NewContext(tenantA, "parent-a", "idempotent-message", []string{parent.ToolListStudents, parent.ToolListTasks, parent.ToolDraftTask, parent.ToolPublishTask, parent.ToolCreateSchedule, parent.ToolListSchedules, parent.ToolPreviewAction, parent.ToolConfirmAction})
	if err != nil {
		t.Fatal(err)
	}
	students, err := tools.ListStudents(ctx, a, parent.StudentQuery{Limit: 20})
	if err != nil || len(students) == 0 || students[0].DisplayName == "Bob" {
		t.Fatalf("tenant students=%+v err=%v", students, err)
	}
	if tasks, err := tools.ListTasks(ctx, a, parent.TaskQuery{Query: "tenant-b-only", Limit: 20}); err != nil || len(tasks) != 0 {
		t.Fatalf("cross-tenant task query=%+v err=%v", tasks, err)
	}
	draft, err := tools.DraftTask(ctx, a, parent.TaskDraftInput{Title: "Agent integration task", Instructions: "Use the ordinary service"})
	if err != nil {
		t.Fatal(err)
	}
	published, err := tools.PublishTask(ctx, a, draft.ID)
	if err != nil || published.Status != "published" {
		t.Fatalf("publish=%+v err=%v", published, err)
	}
	preview, err := tools.PreviewRetireTask(ctx, a, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	foreign := a
	foreign.ActorID = "parent-b"
	if _, err := tools.ConfirmAction(ctx, foreign, parent.ConfirmActionInput{Handle: preview.Handle, Action: preview.Action}); !errors.Is(err, parent.ErrConfirmationForeign) {
		t.Fatalf("foreign confirmation=%v", err)
	}
	if _, err := tools.ConfirmAction(ctx, a, parent.ConfirmActionInput{Handle: preview.Handle, Action: parent.RetireTaskAction(draft.ID, draft.Version+1)}); !errors.Is(err, parent.ErrConfirmationAltered) {
		t.Fatalf("altered confirmation=%v", err)
	}
	if _, err := tools.ConfirmAction(ctx, a, parent.ConfirmActionInput{Handle: preview.Handle, Action: preview.Action}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ConfirmAction(ctx, a, parent.ConfirmActionInput{Handle: preview.Handle, Action: preview.Action}); !errors.Is(err, parent.ErrConfirmationReplay) {
		t.Fatalf("replayed confirmation=%v", err)
	}

	staleDraft, err := tools.DraftTask(ctx, a, parent.TaskDraftInput{Title: "Stale confirmation task", Instructions: "version changes after preview"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tools.PublishTask(ctx, a, staleDraft.ID); err != nil {
		t.Fatal(err)
	}
	stalePreview, err := tools.PreviewRetireTask(ctx, a, staleDraft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE task_revisions SET version=version+1 WHERE id=$1`, staleDraft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tools.ConfirmAction(ctx, a, parent.ConfirmActionInput{Handle: stalePreview.Handle, Action: stalePreview.Action}); err == nil {
		t.Fatal("stale task revision was retired")
	}
	var taskStatus string
	if err = pool.QueryRow(ctx, `SELECT status FROM task_templates WHERE id=$1 AND tenant_id=$2`, staleDraft.TemplateID, tenantA).Scan(&taskStatus); err != nil || taskStatus != "published" {
		t.Fatalf("stale confirmation changed task status=%q err=%v", taskStatus, err)
	}
	expiredPreview, err := tools.PreviewAction(ctx, a, parent.RetireTaskAction(staleDraft.ID, staleDraft.Version+1), "expire this preview")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE parent_confirmation_previews SET created_at=now()-interval '2 seconds', expires_at=now()-interval '1 second' WHERE id=$1`, expiredPreview.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tools.ConfirmAction(ctx, a, parent.ConfirmActionInput{Handle: expiredPreview.Handle, Action: expiredPreview.Action}); !errors.Is(err, parent.ErrConfirmationExpired) {
		t.Fatalf("expired confirmation=%v", err)
	}
	corruptPreview, err := tools.PreviewAction(ctx, a, parent.RetireTaskAction(staleDraft.ID, staleDraft.Version+1), "Corrupt this preview")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE parent_confirmation_previews SET action='[]'::jsonb WHERE id=$1`, corruptPreview.ID); err != nil {
		t.Fatal(err)
	}
	corruptStore := &parent.SQLConfirmationStore{DB: pool}
	if _, err = corruptStore.Consume(ctx, corruptPreview.Handle, tenantA, "parent-a", corruptPreview.ActionDigest); !errors.Is(err, parent.ErrConfirmationRejected) {
		t.Fatalf("corrupt confirmation=%v", err)
	}

	// The service, not the tool input, supplies the student tenant boundary.
	if _, err := tools.CreateSchedule(ctx, a, parent.ScheduleInput{TemplateID: published.TemplateID, RevisionID: published.ID, Kind: "one_off", Timezone: "UTC", StartAt: time.Now().UTC().Add(time.Hour)}, "Bob"); !errors.Is(err, parent.ErrClarification) {
		t.Fatalf("foreign name resolution=%v", err)
	}
}

func TestAgentConfigurationProviderFailuresAreExplicit(t *testing.T) {
	for _, key := range []string{"TASKS_ENV", "TASKS_AGENT_MODE", "TASKS_MODEL_PROVIDER", "TASKS_AGENT_ACTIVE_TOOLS", "TASKS_AGENT_MAX_SECONDS"} {
		t.Setenv(key, "")
	}
	if _, err := os.Stat("."); err != nil {
		t.Fatal(err)
	}
	cfg, err := parent.LoadProviderConfig()
	if err != nil || cfg.Mode != parent.ProviderDisabled || cfg.Availability() == nil {
		t.Fatalf("disabled config=%+v err=%v", cfg, err)
	}
}
