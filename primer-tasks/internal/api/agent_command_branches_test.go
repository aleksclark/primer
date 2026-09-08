package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"primer-tasks/internal/agent"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
)

func TestValidateSocketEventRejectsNonDomainProvenance(t *testing.T) {
	e := wireAgentEvent{Type: "terminal", Source: "model", ProtocolVersion: agentProtocolVersion, Time: time.Now().UTC()}
	if err := validateSocketEvent(e); err == nil {
		t.Fatal("non-domain provenance accepted")
	}
}

func TestAgentHTTPHelperBranches(t *testing.T) {
	s := &Server{Env: "production", SecureCookie: true}
	req := httptest.NewRequest("GET", "/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: "tasks_csrf", Value: "existing"})
	rec := httptest.NewRecorder()
	if got := s.csrfToken(rec, req); got != "existing" || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("existing csrf got=%q cookies=%v", got, rec.Result().Cookies())
	}
	if maxInt(1, 2) != 2 || maxInt(3, 2) != 3 {
		t.Fatal("maxInt branch incorrect")
	}
}

func TestScheduleReceiptRejectsUnknownIDsAndFinishIgnoresTerminalRuns(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	ctx := context.Background()
	svc := phase3Services{s: s}
	sc := parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a"}
	if _, err := scheduleReceipt(ctx, svc, sc, parent.Schedule{RevisionID: tenantA, StudentID: tenantA, Kind: "recurrence", Timezone: "UTC", StartAt: time.Now().UTC(), Enabled: true}, "Created"); err == nil {
		t.Fatal("unknown schedule receipt accepted")
	}
	if err := s.finishAgentRun(ctx, agent.Run{TenantID: tenantA, ID: tenantA}, agent.RunFailed, "failed", "x", "run_expired", "", agent.Usage{}); err == nil {
		t.Fatal("unknown run finish accepted")
	}
}

func TestApplyAgentActionRejectsUnknownKind(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	if _, err := applyAgentAction(context.Background(), phase3Services{s: s}, parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a"}, parent.Action{Kind: "unknown", TargetIDs: []string{tenantA}}); err == nil {
		t.Fatal("unknown action kind accepted")
	}
}

func TestLockAgentAuthorityRejectsMismatchedJob(t *testing.T) {
	pool := integrationPool(t)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	ctx := context.WithValue(context.Background(), agentLeaseKey{}, jobs.Job{ID: tenantA, RunID: "other", TenantID: tenantA, LeaseOwner: "owner"})
	if err := lockAgentAuthority(ctx, tx, tenantA, "parent-a", tenantA); err == nil {
		t.Fatal("mismatched job lease accepted")
	}
}

func TestApplyAgentActionRejectsMissingTargets(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	if _, err := applyAgentAction(context.Background(), phase3Services{s: s}, parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a"}, parent.Action{Kind: parent.ActionRetireTask}); err == nil {
		t.Fatal("missing receipt target accepted")
	}
}

func TestStageAgentActionRejectsMissingTargetsAndDisabledProvider(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	ctx := parent.Context{TenantID: tenantA, ActorID: "parent-a", RunID: tenantA, ToolStep: 1}
	for _, kind := range []string{parent.ActionDisableSchedule, parent.ActionRetireTask, parent.ToolUpdateSchedule} {
		if _, err := s.stageAgentAction(context.Background(), ctx, parent.Action{Kind: kind}); err == nil {
			t.Fatalf("missing target accepted for %s", kind)
		}
	}
	if _, err := s.stageAgentAction(context.Background(), ctx, parent.Action{Kind: parent.ToolDraftTask, TargetIDs: []string{tenantA}, Payload: map[string]any{"Title": ""}}); err == nil {
		t.Fatal("empty draft title accepted")
	}
	if _, err := s.stageAgentAction(context.Background(), ctx, parent.Action{Kind: parent.ToolUpdateTask, TargetIDs: []string{tenantA}, Payload: map[string]any{"Title": ""}}); err == nil {
		t.Fatal("empty update title accepted")
	}
	t.Setenv("TASKS_MODEL_PROVIDER", "scripted")
	t.Setenv("TASKS_AGENT_MODE", "scripted")
	if err := s.confirmAgentAction(context.Background(), scope{Tenant: tenantA, Subject: "parent-a"}, agentCommand{RunID: tenantA, ConfirmationID: "handle"}); err == nil {
		t.Fatal("unknown confirmation accepted")
	}
	t.Setenv("TASKS_MODEL_PROVIDER", "disabled")
	if _, err := s.stageAgentAction(context.Background(), ctx, parent.Action{Kind: parent.ActionDisableSchedule, TargetIDs: []string{tenantA}}); err == nil {
		t.Fatal("disabled provider accepted")
	}
}

func TestCreateTaskRejectsMalformedJSON(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	sc := scope{Tenant: tenantA, Subject: "parent-a"}
	for _, fn := range []func(http.ResponseWriter, *http.Request, scope){s.createTask2, s.decideOccurrence2, s.createSchedule2, s.updateSchedule2} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{`))
		req.Header.Set("Content-Type", "application/json")
		fn(rec, req, sc)
		if rec.Code < 400 {
			t.Fatalf("malformed json=%d", rec.Code)
		}
	}
}

func TestAgentCommandHandlersFailClosedBeforeDatabaseAccess(t *testing.T) {
	s := &Server{}
	sub := &agentSubscriber{tenant: "tenant-a", conversation: "", queue: make(chan wireAgentEvent, 1), done: make(chan struct{})}
	s.agentSubscribe(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, sub, agentCommand{ConversationID: "not-a-uuid"})
	if sub.conversation != "" {
		t.Fatal("invalid conversation was subscribed")
	}
	s.agentMessage(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, agentCommand{})
	s.agentMessage(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, agentCommand{ConversationID: "not-a-uuid", ClientMessageID: "client", Text: "message"})
	s.agentCancel(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, agentCommand{})
	s.agentConfirm(context.Background(), scope{Tenant: "tenant-a", Subject: "parent-a"}, agentCommand{})
	if agent.NewID() == "" {
		t.Fatal("NewID returned empty id")
	}
	r := httptest.NewRequest(http.MethodGet, "/agent/ws", nil)
	if s.agentOriginAllowed(r) {
		t.Fatal("empty origin accepted")
	}
}
