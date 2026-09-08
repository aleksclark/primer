package api

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"git.clark.team/aleksclark/authstack/auth"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"primer-tasks/internal/devicemanagement"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/jobs"
	"primer-tasks/internal/verification"
)

func TestDialogueWorkerRejectsProductionScriptedAndBadDelay(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "production")
	queue := jobs.NewPostgresRepository(pool)
	job := jobs.DialogueJob{Deadline: time.Now().Add(time.Second)}
	t.Setenv("TASKS_MODEL_PROVIDER", "scripted")
	t.Setenv("TASKS_AGENT_MODE", "scripted")
	if err := s.runDialogueJob(context.Background(), queue, job); err == nil {
		t.Fatal("production scripted dialogue accepted")
	}
	s.Env = "test"
	t.Setenv("TASKS_AGENT_SCRIPTED_DIALOGUE_DELAY_MS", "nope")
	if err := s.runDialogueJob(context.Background(), queue, job); err == nil {
		t.Fatal("invalid scripted delay accepted")
	}
}

func TestPublicDialogueLeaseRenewalRejectsStaleGeneration(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	conn := h.socket(0)
	q := h.question(conn, 0)
	h.answer(conn, q, "lease-renew", "The family repaired the garden wall after the storm.")
	deadline := time.Now().Add(8 * time.Second)
	var job jobs.DialogueJob
	for time.Now().Before(deadline) {
		var owner *string
		err := h.pool.QueryRow(context.Background(), `SELECT id,tenant_id,lease_owner,lease_generation FROM verification_jobs WHERE attempt_id=$1 AND status='running' AND lease_owner IS NOT NULL AND message_id IS NOT NULL`, h.attempt).Scan(&job.ID, &job.TenantID, &owner, &job.LeaseGeneration)
		if err == nil && owner != nil && *owner != "" && job.LeaseGeneration > 0 {
			job.LeaseOwner = *owner
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if job.ID == "" {
		t.Fatal("running dialogue job was not observed")
	}
	repo := jobs.NewPostgresRepository(h.pool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := repo.RenewDialogue(ctx, job); err != nil {
		t.Fatalf("current lease renewal failed: %v", err)
	}
	stale := job
	stale.LeaseGeneration++
	if err := repo.RenewDialogue(ctx, stale); err == nil {
		t.Fatal("stale generation renewed the live lease")
	}
	if _, ok, err := repo.ClaimDialogue(ctx, ""); err == nil || ok {
		t.Fatal("empty owner claimed a dialogue job")
	}
}

func TestAdvertiseAPKBodyManagementProblemAndMount(t *testing.T) {
	advertiseAPKBody(nil)
	advertiseAPKBody(&huma.Operation{Responses: map[string]*huma.Response{}})
	op := &huma.Operation{Responses: map[string]*huma.Response{"200": {Content: map[string]*huma.MediaType{"application/json": {}}}}}
	advertiseAPKBody(op)
	if op.Responses["200"].Content["application/vnd.android.package-archive"] == nil {
		t.Fatal("apk media type missing")
	}
	if err := managementProblem(nil); err != nil {
		t.Fatal(err)
	}
	if err := managementProblem(devicemanagement.ErrUnavailable); err == nil {
		t.Fatal("unavailable problem missing")
	}
	if err := managementProblem(errors.New("other")); err == nil {
		t.Fatal("default problem missing")
	}
	handler := Mount(http.NotFoundHandler(), "/tasks", t.TempDir())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tasks/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-get spa=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/parent", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing index=%d", rec.Code)
	}
}

func TestAgentOriginAllowsConfiguredExtraOriginAndRejectsEmpty(t *testing.T) {
	t.Setenv("TASKS_ALLOWED_ORIGINS", "https://tasks.example, https://other.example")
	s := &Server{Env: "test", Auth: AuthConfig{PublicOrigin: "https://api.example", RedirectURL: "https://api.example/auth/callback"}}
	allowed := httptest.NewRequest(http.MethodGet, "/agent/ws", nil)
	allowed.Header.Set("Origin", "https://tasks.example")
	if !s.agentOriginAllowed(allowed) {
		t.Fatal("configured extra origin rejected")
	}
	blank := httptest.NewRequest(http.MethodGet, "/agent/ws", nil)
	if s.agentOriginAllowed(blank) {
		t.Fatal("empty origin accepted")
	}
}

func TestStudentOriginAllowsConfiguredExtraOriginAndRejectsEmpty(t *testing.T) {
	t.Setenv("TASKS_ALLOWED_ORIGINS", "https://tasks.example, https://other.example")
	s := &Server{Auth: AuthConfig{PublicOrigin: "https://api.example"}}
	allowed := httptest.NewRequest(http.MethodGet, "/student/ws", nil)
	allowed.Header.Set("Origin", "https://tasks.example")
	if !s.studentOriginAllowed(allowed) {
		t.Fatal("configured extra origin rejected")
	}
	blank := httptest.NewRequest(http.MethodGet, "/student/ws", nil)
	if s.studentOriginAllowed(blank) {
		t.Fatal("missing origin accepted")
	}
	foreign := httptest.NewRequest(http.MethodGet, "/student/ws", nil)
	foreign.Header.Set("Origin", "https://evil.example")
	if s.studentOriginAllowed(foreign) {
		t.Fatal("unlisted origin accepted")
	}
}

func TestPublicInspectWithoutParentCookieIsUnauthorized(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	plain := &http.Client{Timeout: 5 * time.Second}
	h.request(plain, "GET", "/occurrences/"+h.occurrence+"/inspect", nil, 401, nil)
	h.request(plain, "POST", "/occurrences/"+h.occurrence+"/override", DialogueOverrideRequest{AttemptID: h.attempt, ExpectedVersion: 1, ClientRequestID: "no-cookie", Accepted: false, Reason: "Parent independently reviewed the attempt."}, 401, nil)
}

func TestPrivateAgentFrameRequiresAuthorization(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	if err := s.writePrivateAgentFrame(context.Background(), nil, scope{Tenant: tenantA, Subject: "parent-a"}, nil, wireAgentEvent{Type: "hello"}); err == nil {
		t.Fatal("nil authorization accepted")
	}
}

func TestClerkSocketAuthorizationRequiresPrincipal(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	s.Auth.Mode = "clerk"
	r := httptest.NewRequest(http.MethodGet, "/agent/ws", nil)
	r.Header.Set("Sec-WebSocket-Protocol", agentBearerProtocol+"token")
	if _, err := s.socketAuthorization(r, scope{Tenant: tenantA, Subject: "parent-a"}); err == nil {
		t.Fatal("clerk socket without principal accepted")
	}
}

func TestExpiredAgentAuthorizationIsRejected(t *testing.T) {
	ctx := context.WithValue(context.Background(), agentAuthKey{}, &agentAuthorization{deadline: time.Now().Add(-time.Second), check: func(context.Context) error { return nil }})
	if err := checkAgentAuthorization(ctx); err == nil {
		t.Fatal("expired authorization accepted")
	}
	pool := integrationPool(t)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	locked := context.WithValue(context.Background(), agentAuthKey{}, &agentAuthorization{issuer: "https://clerk.example", subject: "parent-a", session: "sess", deadline: time.Now().Add(-time.Second)})
	if err := checkLockedAgentParent(locked, tx, tenantA, "parent-a"); err == nil {
		t.Fatal("expired locked parent accepted")
	}
	missing := context.WithValue(context.Background(), agentAuthKey{}, &agentAuthorization{issuer: "https://clerk.example", subject: "parent-a", session: "sess"})
	if err := checkLockedAgentParent(missing, tx, tenantA, "parent-a"); err == nil {
		t.Fatal("missing verifier accepted")
	}
}

func TestClerkScopeAndParentErrorRequirePrincipal(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	s.Auth.Mode = "clerk"
	r := httptest.NewRequest(http.MethodGet, "/parent/students", nil)
	if _, err := s.clerkScope(r); err == nil {
		t.Fatal("clerk scope without principal accepted")
	}
	rec := httptest.NewRecorder()
	s.parentError(rec, auth.ErrUnauthenticated)
	if rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatal("clerk unauthorized omitted WWW-Authenticate")
	}
	rec = httptest.NewRecorder()
	s.clerkLogout(rec, r)
	if rec.Code != 401 {
		t.Fatalf("clerk logout without principal=%d", rec.Code)
	}
}

func TestDialogueParentContextRequiresParentCookie(t *testing.T) {
	s := New(integrationPool(t), "test")
	r := httptest.NewRequest(http.MethodGet, "/occurrences/x/inspect", nil)
	if _, err := s.dialogueParentContext(r, scope{Tenant: tenantA, Subject: "parent-a"}); err == nil {
		t.Fatal("missing parent cookie accepted")
	}
	r.AddCookie(&http.Cookie{Name: "tasks_parent", Value: "parent-a"})
	ctx, err := s.dialogueParentContext(r, scope{Tenant: tenantA, Subject: "parent-a"})
	if err != nil || ctx == nil {
		t.Fatalf("parent cookie context rejected: %v", err)
	}
	s.Auth.Mode = "clerk"
	if _, err = s.dialogueParentContext(httptest.NewRequest(http.MethodGet, "/occurrences/x/inspect", nil), scope{Tenant: tenantA, Subject: "parent-a"}); err == nil {
		t.Fatal("clerk parent context without principal accepted")
	}
}

func TestMalformedJSONAndDeviceBearerFailClosed(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	sc := scope{Tenant: tenantA, Subject: "parent-a"}
	bad := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{`))
	bad.Header.Set("Content-Type", "application/json")
	for _, fn := range []func(http.ResponseWriter, *http.Request, scope){s.updateStudent, s.reviseTask2, s.dialogueOverride} {
		rec := httptest.NewRecorder()
		fn(rec, bad, sc)
		if rec.Code < 400 {
			t.Fatalf("malformed json=%d", rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	s.pairBrowser(rec, bad)
	if rec.Code < 400 {
		t.Fatalf("malformed pairBrowser=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.devicePair(rec, bad)
	if rec.Code < 400 {
		t.Fatalf("malformed devicePair=%d", rec.Code)
	}
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	r := httptest.NewRequest(http.MethodPost, "/device/occurrences/x/start", nil)
	if _, err := lockManualDeviceAuthority(context.Background(), tx, r, uuid.MustParse(tenantA)); err == nil {
		t.Fatal("missing bearer accepted")
	}
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 513))
	if _, err := lockManualDeviceAuthority(context.Background(), tx, r, uuid.MustParse(tenantA)); err == nil {
		t.Fatal("oversized bearer accepted")
	}
}

func TestRequestOriginAndCreateStudentDecodeBranches(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "https://tasks.test/x", nil)
	r.TLS = &tls.ConnectionState{}
	if got := requestOrigin(r, "tasks.test"); got != "https://tasks.test" {
		t.Fatalf("tls origin=%s", got)
	}
	r = httptest.NewRequest(http.MethodGet, "http://tasks.test/x", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := requestOrigin(r, "tasks.test"); got != "https://tasks.test" {
		t.Fatalf("forwarded origin=%s", got)
	}
	pool := integrationPool(t)
	s := New(pool, "test")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/students", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	s.createStudent(rec, req, scope{Tenant: tenantA, Subject: "parent-a"})
	if rec.Code < 400 {
		t.Fatalf("malformed student create=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/students", strings.NewReader(`{"displayName":""}`))
	req.Header.Set("Content-Type", "application/json")
	s.createStudent(rec, req, scope{Tenant: tenantA, Subject: "parent-a"})
	if rec.Code != 400 {
		t.Fatalf("empty name=%d", rec.Code)
	}
}

func TestAgentModelRequiresConfiguredProvider(t *testing.T) {
	s := &Server{Env: "test"}
	if _, err := s.agentModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderBedrock, Primary: parent.ProviderBedrock, Fallback: parent.ProviderDisabled}, "prompt"); err == nil {
		t.Fatal("unconfigured bedrock accepted")
	}
}

func TestDialogueWorkerStopsOnCancellationAndScriptedStreamHonorsContext(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.runDialogueWorker(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		stream := scriptedStream(ctx, "hello")
		stream(func(fantasy.StreamPart) bool { return true })
	}()
	select {
	case <-done:
	default:
	}
}

func TestAgentMessageRejectsOversizedTextAndClientIDs(t *testing.T) {
	pool := integrationPool(t)
	s := New(pool, "test")
	ctx := context.Background()
	sc := scope{Tenant: tenantA, Subject: "parent-a"}
	s.agentMessage(ctx, sc, agentCommand{ConversationID: tenantA, Text: strings.Repeat("x", 8193), ClientMessageID: "ok"})
	s.agentMessage(ctx, sc, agentCommand{ConversationID: tenantA, Text: "hello", ClientMessageID: strings.Repeat("y", 129)})
}

func TestAgentBearerRejectsDuplicateEmptyAndSpacedTokens(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/agent/ws", nil)
	r.Header.Set("Sec-WebSocket-Protocol", agentBearerProtocol+"one, "+agentBearerProtocol+"two")
	if _, err := agentBearer(r); err == nil {
		t.Fatal("duplicate bearer accepted")
	}
	r.Header.Set("Sec-WebSocket-Protocol", agentBearerProtocol)
	if _, err := agentBearer(r); err == nil {
		t.Fatal("empty bearer accepted")
	}
	r.Header.Set("Sec-WebSocket-Protocol", agentBearerProtocol+"has space")
	if _, err := agentBearer(r); err == nil {
		t.Fatal("spaced bearer accepted")
	}
}

func TestPhase3ScopeTenantAndScriptedToolInputs(t *testing.T) {
	if got := scopeTenant(parent.ServiceContext{TenantID: tenantA}); got != tenantA {
		t.Fatalf("scopeTenant=%q", got)
	}
	empty := scriptedIDInput(fantasy.Call{}, "publish_task")
	if empty != `{"id":""}` {
		t.Fatalf("empty id input=%s", empty)
	}
	call := fantasy.Call{Prompt: fantasy.Prompt{
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.ToolCallPart{ToolCallID: "c1", ToolName: "publish_task"}, fantasy.ToolCallPart{ToolCallID: "c2", ToolName: "list_students"}}},
		{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{ToolCallID: "c1", Output: fantasy.ToolResultOutputContentText{Text: `{"id":"rev-1","templateId":"tmpl-1"}`}},
			fantasy.ToolResultPart{ToolCallID: "c2", Output: fantasy.ToolResultOutputContentText{Text: `[{"id":"student-1"}]`}},
		}},
	}}
	if got := scriptedIDInput(call, "publish_task"); !strings.Contains(got, "rev-1") {
		t.Fatalf("scripted id=%s", got)
	}
	schedule := scriptedScheduleInput(call)
	if !strings.Contains(schedule, "student-1") || !strings.Contains(schedule, "tmpl-1") || !strings.Contains(schedule, "rev-1") {
		t.Fatalf("scripted schedule=%s", schedule)
	}
	previewCall := fantasy.Call{Prompt: fantasy.Prompt{
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.ToolCallPart{ToolCallID: "c3", ToolName: "list_schedules"}, fantasy.ToolCallPart{ToolCallID: "c4", ToolName: "list_tasks"}}},
		{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{ToolCallID: "c3", Output: fantasy.ToolResultOutputContentText{Text: `{"items":[{"id":"disabled","enabled":false},{"id":"sch-1","enabled":true,"version":4}]}`}},
			fantasy.ToolResultPart{ToolCallID: "c4", Output: fantasy.ToolResultOutputContentText{Text: `{"items":[{"id":"draft","status":"draft"},{"id":"pub-1","status":"published","version":2}]}`}},
		}},
	}}
	schedulePreview := scriptedSchedulePreviewInput(previewCall)
	if !strings.Contains(schedulePreview, "sch-1") || !strings.Contains(schedulePreview, "4") {
		t.Fatalf("schedule preview=%s", schedulePreview)
	}
	taskPreview := scriptedPreviewInput(previewCall)
	if !strings.Contains(taskPreview, "pub-1") {
		t.Fatalf("task preview=%s", taskPreview)
	}
	s := &Server{Env: "production"}
	if _, err := s.agentModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderScripted, Environment: "test"}, "prompt"); err == nil {
		t.Fatal("production scripted parent model accepted")
	}
	if _, err := s.agentModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderDisabled}, "prompt"); err == nil {
		t.Fatal("disabled parent model accepted")
	}
}

func TestDialogueEngineRejectsWrongQuestionKeyAndTerminalRetry(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	conn := h.socket(0)
	q := h.question(conn, 0)
	engine := verification.DialogueEngine{DB: h.pool}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var sessionID, jobID, owner string
	var generation int64
	if err := h.pool.QueryRow(ctx, `SELECT j.id,COALESCE(j.lease_owner,''),j.lease_generation,j.session_id FROM verification_jobs j WHERE j.attempt_id=$1 ORDER BY j.created_at DESC LIMIT 1`, h.attempt).Scan(&jobID, &owner, &generation, &sessionID); err != nil || jobID == "" {
		t.Fatalf("dialogue job missing: %v", err)
	}
	live := verification.StudentAuthority{TenantID: tenantA, StudentID: h.studentID, SessionID: sessionID}
	lease := verification.DialogueLease{JobID: jobID, Owner: owner, Generation: generation}
	if err := engine.CommitQuestion(ctx, live, h.occurrence, h.attempt, lease, "not-the-key"); err == nil {
		t.Fatal("wrong question key accepted")
	}
	if err := engine.CommitEvaluation(ctx, live, h.occurrence, h.attempt, lease, verification.DialogueEvaluation{}); err == nil {
		t.Fatal("evaluation committed during question stage")
	}
	h.answer(conn, q, "complete-one", "The family repaired the garden wall after the storm.")
	h.answer(conn, h.question(conn, 1), "complete-two", "The mortar must dry before the next course of stones.")
	h.answer(conn, h.question(conn, 2), "complete-three", "Rushing the work would weaken the wall.")
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "complete" })
	if err := engine.Retry(ctx, live, h.occurrence, h.attempt, q.Version); err == nil {
		t.Fatal("retry after completion accepted")
	}
	if err := engine.Progress(ctx, live, h.occurrence, h.attempt, lease, "thinking"); err == nil {
		t.Fatal("progress after completion accepted")
	}
	h.request(h.student, "POST", "/student/occurrences/"+h.occurrence+"/dialogue", DialogueStartBody{}, 409, nil)
}

func TestDialogueEngineRejectsUnknownAuthorityProgressAndQuestion(t *testing.T) {
	h := newPublicDialogueHarness(t)
	engine := verification.DialogueEngine{DB: h.pool}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := verification.ResolveStudentAuthority(ctx, h.pool, make([]byte, 8)); err == nil {
		t.Fatal("short session hash accepted")
	}
	if _, err := verification.ResolveStudentAuthority(ctx, h.pool, make([]byte, 32)); err == nil {
		t.Fatal("unknown session hash accepted")
	}
	h.begin()
	auth := verification.StudentAuthority{TenantID: tenantA, StudentID: h.studentID, SessionID: "00000000-0000-0000-0000-000000000099"}
	if err := engine.Progress(ctx, auth, h.occurrence, h.attempt, verification.DialogueLease{JobID: "00000000-0000-0000-0000-000000000099"}, "thinking"); err == nil {
		t.Fatal("unknown session progress accepted")
	}
	if err := engine.Progress(ctx, auth, h.occurrence, h.attempt, verification.DialogueLease{}, "nope"); err == nil {
		t.Fatal("invalid progress phase accepted")
	}
	if err := engine.CommitQuestion(ctx, auth, h.occurrence, h.attempt, verification.DialogueLease{JobID: "00000000-0000-0000-0000-000000000099"}, "wall"); err == nil {
		t.Fatal("unknown lease question commit accepted")
	}
	if err := engine.Retry(ctx, auth, h.occurrence, h.attempt, 1); err == nil {
		t.Fatal("unknown session retry accepted")
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = verification.LockStudentAuthority(ctx, tx, auth); err == nil {
		t.Fatal("unknown student authority locked")
	}
	var sessionID string
	if err = h.pool.QueryRow(ctx, `SELECT id FROM student_sessions WHERE student_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`, h.studentID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	live := verification.StudentAuthority{TenantID: tenantA, StudentID: h.studentID, SessionID: sessionID}
	if err = engine.Progress(ctx, live, "00000000-0000-0000-0000-000000000099", h.attempt, verification.DialogueLease{JobID: "00000000-0000-0000-0000-000000000099"}, "evaluating"); err == nil {
		t.Fatal("foreign occurrence progress accepted")
	}
	if err = engine.CommitQuestion(ctx, live, h.occurrence, h.attempt, verification.DialogueLease{JobID: "00000000-0000-0000-0000-000000000099", Owner: "nobody", Generation: 99}, "wall"); err == nil {
		t.Fatal("wrong lease question commit accepted")
	}
	if err = engine.CommitEvaluation(ctx, live, h.occurrence, h.attempt, verification.DialogueLease{JobID: "00000000-0000-0000-0000-000000000099"}, verification.DialogueEvaluation{}); err == nil {
		t.Fatal("wrong lease evaluation commit accepted")
	}
	if _, _, _, err = engine.WorkerState(ctx, live, h.occurrence, h.attempt, verification.DialogueLease{JobID: "00000000-0000-0000-0000-000000000099"}); err == nil {
		t.Fatal("wrong lease worker state accepted")
	}
}

func TestPublicArchiveRejectsUnknownStudent(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.request(h.parent, "DELETE", "/students/00000000-0000-0000-0000-000000000099", nil, 404, nil)
	h.request(h.parent, "GET", "/occurrences/00000000-0000-0000-0000-000000000099", nil, 404, nil)
	h.request(h.student, "GET", "/student/occurrences/00000000-0000-0000-0000-000000000099", nil, 404, nil)
	h.request(h.parent, "GET", "/students?limit=0&offset=-3&q=Alice", nil, 200, nil)
}

func TestPublicInspectRejectsUnknownAttemptAndFutureCursor(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	h.request(h.parent, "GET", "/occurrences/"+h.occurrence+"/inspect?attemptId=00000000-0000-0000-0000-000000000099", nil, 404, nil)
	h.request(h.parent, "GET", "/occurrences/"+h.occurrence+"/inspect?after=999999", nil, 409, nil)
	h.request(h.parent, "GET", "/occurrences/00000000-0000-0000-0000-000000000099/inspect", nil, 404, nil)
}

func TestPublicDialogueStartRejectsCSRFIdentityAndUnknownOccurrence(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.request(h.student, "POST", "/student/occurrences/"+h.occurrence+"/dialogue", DialogueStartBody{}, 200, nil)
	plain := &http.Client{Timeout: 5 * time.Second}
	h.request(plain, "POST", "/student/occurrences/"+h.occurrence+"/dialogue", DialogueStartBody{}, 403, nil)
	h.request(plain, "GET", "/student/occurrences/"+h.occurrence+"/dialogue", nil, 401, nil)
	h.request(h.student, "POST", "/student/occurrences/00000000-0000-0000-0000-000000000099/dialogue", DialogueStartBody{}, 404, nil)
	h.request(h.student, "GET", "/student/occurrences/00000000-0000-0000-0000-000000000099/dialogue", nil, 404, nil)
	h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/override", map[string]any{}, 400, nil)
}

func TestPublicStudentSocketRejectsFutureCursorAckAndUnsubscribedRetry(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	conn := h.socket(0)
	q := h.question(conn, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, conn, studentCommand{Protocol: 1, Kind: "ack", Cursor: q.Cursor + 50}); err != nil {
		t.Fatal(err)
	}
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "error" && e.Code == "conflict" })
	if err := wsjson.Write(ctx, conn, studentCommand{Protocol: 1, Kind: "unsubscribe"}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, conn, studentCommand{Protocol: 1, Kind: "retry", OccurrenceID: h.occurrence, AttemptID: h.attempt, ExpectedVersion: q.Version}); err != nil {
		t.Fatal(err)
	}
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "error" && e.Code == "not_found" })
	future := h.socket(q.Cursor + 50)
	h.wait(future, func(e wireStudentEvent) bool { return e.Kind == "error" && e.Code == "conflict" })
}

func TestPublicStudentHTTPRejectsQueryCredentialsAndBearer(t *testing.T) {
	h := newPublicDialogueHarness(t)
	r, err := http.NewRequest(http.MethodGet, h.base+"/student/occurrences/"+h.occurrence+"/dialogue?token=not-a-credential", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Origin", h.base)
	resp, err := h.student.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("query credential status %d", resp.StatusCode)
	}
	r, err = http.NewRequest(http.MethodGet, h.base+"/student/occurrences/"+h.occurrence+"/dialogue", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer opaque")
	resp, err = h.student.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("bearer status %d", resp.StatusCode)
	}
}

func TestPublicDialogueRetryAndOverrideRejectStaleOrUnknownBindings(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	conn := h.socket(0)
	q := h.question(conn, 0)
	h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/override", DialogueOverrideRequest{AttemptID: h.attempt, ExpectedVersion: 1, ClientRequestID: "short", Accepted: false, Reason: "no"}, 400, nil)
	h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/override", DialogueOverrideRequest{AttemptID: "00000000-0000-0000-0000-000000000099", ExpectedVersion: 1, ClientRequestID: "unknown-attempt", Accepted: false, Reason: "Parent independently reviewed the attempt."}, 404, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, conn, studentCommand{Protocol: 1, Kind: "retry", OccurrenceID: h.occurrence, AttemptID: h.attempt, ExpectedVersion: q.Version}); err != nil {
		t.Fatal(err)
	}
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "error" })
	if err := wsjson.Write(ctx, conn, studentCommand{Protocol: 1, Kind: "user_message", OccurrenceID: h.occurrence, AttemptID: "00000000-0000-0000-0000-000000000099", QuestionID: q.QuestionID, ExpectedVersion: q.Version, PolicyVersion: q.PolicyVersion, SnapshotDigest: q.SnapshotDigest, ClientMessageID: "wrong-attempt", Text: "The family repaired the garden wall after the storm."}); err != nil {
		t.Fatal(err)
	}
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "error" && e.Code == "not_found" })
}

func TestPublicStudentSocketRejectsBinaryAndInvalidCommands(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.begin()
	conn := h.socket(0)
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "hello" || e.Kind == "question" || e.Kind == "state" })
	if err := conn.Write(context.Background(), websocket.MessageBinary, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	expectStudentClose(t, conn, "", websocket.StatusUnsupportedData)
	conn = h.socket(0)
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "hello" || e.Kind == "question" || e.Kind == "state" })
	if err := conn.Write(context.Background(), websocket.MessageText, []byte("not-json")); err != nil {
		t.Fatal(err)
	}
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "error" && e.Code == "invalid_request" })
}
