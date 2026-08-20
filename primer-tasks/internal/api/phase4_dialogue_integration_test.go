package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"primer-tasks/internal/agent"
	"primer-tasks/internal/jobs"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
)

// TestPhase4ScriptedDialoguePostgresFlow exercises the real persistence and
// Fantasy tool loop rather than inserting accepted evidence. It is deliberately
// one complete chapter fixture: two accepted answers, one insufficient answer,
// follow-up, a third accepted answer, idempotent replay, and one terminal row.
func scopeForParent(tenant, subject string) scope { return scope{Tenant: tenant, Subject: subject} }

func TestPhase4ScriptedDialoguePostgresFlow(t *testing.T) {
	t.Setenv("TASKS_AGENT_MODE", "scripted")
	t.Setenv("TASKS_AGENT_ACTIVE_TOOLS", "list_students")
	pool := integrationPool(t)
	ctx := context.Background()
	tenant, student, template, revision, requirement := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	scheduleID, occurrence, attempt := uuid.NewString(), uuid.NewString(), uuid.NewString()
	cleanup := func() {
		for _, q := range []string{
			"DELETE FROM verification_events WHERE tenant_id=$1", "DELETE FROM verification_jobs WHERE tenant_id=$1", "DELETE FROM verification_evaluations WHERE tenant_id=$1", "DELETE FROM dialogue_questions WHERE tenant_id=$1", "DELETE FROM verification_messages WHERE tenant_id=$1", "DELETE FROM dialogue_attempts WHERE tenant_id=$1", "DELETE FROM verification_decisions WHERE tenant_id=$1", "DELETE FROM verification_attempts WHERE tenant_id=$1", "DELETE FROM task_occurrences WHERE tenant_id=$1", "DELETE FROM verification_requirements WHERE tenant_id=$1", "DELETE FROM task_schedules WHERE tenant_id=$1", "DELETE FROM task_revisions WHERE tenant_id=$1", "DELETE FROM task_templates WHERE tenant_id=$1", "DELETE FROM students WHERE tenant_id=$1", "DELETE FROM tenants WHERE id=$1",
		} {
			_, _ = pool.Exec(context.Background(), q, tenant)
		}
	}
	t.Cleanup(cleanup)
	if _, err := pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,'Phase 4 dialogue')`, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'Terra')`, student, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'Chapter 4','published',1)`, template, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions,status) VALUES($1,$2,$3,1,'Chapter 4','Answer three questions','published')`, revision, tenant, template); err != nil {
		t.Fatal(err)
	}
	config := `{"sourceText":"The family repaired the garden wall after the storm. The mortar must dry before the next course, or rushing will weaken the wall.","learningFocus":"Recall three distinct facts","requiredQuestions":3,"rubric":["answers address the distinct question"],"allowedFollowUps":1,"maxAttempts":2,"maxTurns":8,"retentionPolicy":"retain"}`
	if _, err := pool.Exec(ctx, `INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_dialogue',1,$4,'chat','fantasy')`, requirement, tenant, revision, config); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, scheduleID, tenant, student, template, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status,revision_snapshot) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification',$6)`, occurrence, tenant, scheduleID, student, revision, `{"title":"Chapter 4","instructions":"Answer"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, attempt, tenant, occurrence, requirement); err != nil {
		t.Fatal(err)
	}
	deviceToken := "phase4-flow-device"
	if _, err := pool.Exec(ctx, `INSERT INTO student_devices(id,tenant_id,student_id,token_hash) VALUES($1,$2,$3,$4)`, uuid.NewString(), tenant, student, hash(deviceToken)); err != nil {
		t.Fatal(err)
	}
	r := repo.NewDialogueRepository(pool)
	if err := r.CreateDialogueAttempt(ctx, repo.DialogueAttempt{TenantID: tenant, AttemptID: attempt, OccurrenceID: occurrence, RequirementID: requirement, PolicyVersion: "dialogue.v1", ConfigSnapshot: map[string]any{"sourceText": "The family repaired the garden wall after the storm. The mortar must dry before the next course, or rushing will weaken the wall.", "learningFocus": "Recall three distinct facts", "requiredQuestions": 3, "rubric": []string{"answers address the distinct question"}, "allowedFollowUps": 1, "maxAttempts": 2, "maxTurns": 8, "retentionPolicy": "retain"}, NextSequence: 1}); err != nil {
		t.Fatal(err)
	}
	s := New(pool, "test")
	// Exercise the real student start boundary: it snapshots the published
	// requirement into the occurrence attempt and creates the durable dialogue
	// projection before any answer can be submitted.
	startSchedule, startOccurrence := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now()+interval '1 day')`, startSchedule, tenant, student, template, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status,revision_snapshot) VALUES($1,$2,$3,$4,$5,now()+interval '1 day',now()+interval '1 day 1 hour','pending',$6)`, startOccurrence, tenant, startSchedule, student, revision, `{"title":"Chapter 4"}`); err != nil {
		t.Fatal(err)
	}
	startRoute := chi.NewRouteContext()
	startRoute.URLParams.Add("id", startOccurrence)
	startReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+startOccurrence+"/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, startRoute))
	startRec := httptest.NewRecorder()
	s.studentStart2(startRec, startReq, uuid.MustParse(student))
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", startRec.Code, startRec.Body.String())
	}
	var startCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dialogue_attempts WHERE tenant_id=$1 AND occurrence_id=$2`, tenant, startOccurrence).Scan(&startCount); err != nil || startCount != 1 {
		t.Fatalf("start dialogue attempts=%d err=%v", startCount, err)
	}
	var startAttempt string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM verification_attempts WHERE tenant_id=$1 AND occurrence_id=$2`, tenant, startOccurrence).Scan(&startAttempt); err != nil {
		t.Fatal(err)
	}
	startScope := verification.DialogueContext{TenantID: tenant, StudentID: student, OccurrenceID: startOccurrence, RequirementID: requirement, AttemptID: startAttempt, PolicyVersion: "dialogue.v1"}
	startStateRoute := chi.NewRouteContext()
	startStateRoute.URLParams.Add("id", startOccurrence)
	startStateReq := httptest.NewRequest(http.MethodGet, "/student/occurrences/"+startOccurrence+"/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, startStateRoute))
	startStateRec := httptest.NewRecorder()
	s.studentDialogueState(startStateRec, startStateReq, uuid.MustParse(student))
	if startStateRec.Code != http.StatusOK || !bytes.Contains(startStateRec.Body.Bytes(), []byte("parent-assigned chapter")) {
		t.Fatalf("new dialogue state status=%d body=%s", startStateRec.Code, startStateRec.Body.String())
	}
	wsServer := httptest.NewServer(s.Routes())
	t.Cleanup(wsServer.Close)
	wsHeader := http.Header{"Authorization": {"Bearer " + deviceToken}}
	wsConn, _, err := websocket.Dial(ctx, "ws"+wsServer.URL[len("http"):]+"/student/ws", &websocket.DialOptions{HTTPHeader: wsHeader, Subprotocols: []string{"primer-tasks.student.v1"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wsConn.Close(websocket.StatusNormalClosure, "done") })
	var hello wireStudentEvent
	if err := wsjson.Read(ctx, wsConn, &hello); err != nil || hello.Type != "hello" {
		t.Fatalf("student hello=%+v err=%v", hello, err)
	}
	if err := wsjson.Write(ctx, wsConn, studentCommand{Type: "subscribe", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence}); err != nil {
		t.Fatal(err)
	}
	var stateEvent wireStudentEvent
	if err := wsjson.Read(ctx, wsConn, &stateEvent); err != nil || stateEvent.Type != "state" {
		t.Fatalf("student state=%+v err=%v", stateEvent, err)
	}
	if err := wsjson.Write(ctx, wsConn, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, AttemptID: attempt, ClientMessageID: "socket-answer", Text: "socket answer", ExpectedSequence: 0}); err != nil {
		t.Fatal(err)
	}
	seenAck := false
	for i := 0; i < 4 && !seenAck; i++ {
		var event wireStudentEvent
		if err := wsjson.Read(ctx, wsConn, &event); err != nil {
			t.Fatal(err)
		}
		seenAck = event.Type == "message_ack"
	}
	if !seenAck {
		t.Fatal("student socket did not acknowledge durable answer")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM verification_jobs WHERE tenant_id=$1`, tenant); err != nil {
		t.Fatal(err)
	}
	identity := studentIdentity{StudentID: uuid.MustParse(student), TenantID: tenant, Source: "bearer", Token: "test-token"}
	binding, err := s.bindStudentAttempt(ctx, identity, occurrence, attempt)
	if err != nil {
		t.Fatal(err)
	}
	sub := &studentSubscriber{tenant: tenant, student: student, queue: make(chan wireStudentEvent, 16), done: make(chan struct{})}
	s.studentSubscribe(ctx, identity, sub, studentCommand{Type: "subscribe", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, Cursor: 0})
	if len(sub.queue) == 0 {
		t.Fatal("student subscribe did not return durable state")
	}
	wsMessage, wsSequence, inserted, conflict, err := s.appendStudentMessage(ctx, binding, identity, "ws-answer", "A durable browser answer", 0)
	if err != nil || !inserted || conflict || wsSequence < 1 {
		t.Fatalf("ws append id=%s seq=%d inserted=%v conflict=%v err=%v", wsMessage, wsSequence, inserted, conflict, err)
	}
	if err := s.persistStudentEvent(ctx, binding, wireStudentEvent{Type: "message_ack", MessageID: wsMessage, ClientMessageID: "ws-answer", Text: "A durable browser answer", Role: "student"}); err != nil {
		t.Fatal(err)
	}
	s.studentRetry(ctx, identity, sub, studentCommand{Type: "retry", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, AttemptID: attempt})
	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, AttemptID: attempt})
	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, AttemptID: attempt, ClientMessageID: "socket-answer", Text: "socket answer"})
	if _, err := pool.Exec(ctx, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,message_id,status,max_attempts) VALUES($1,$2,$3,$4,'queued',1)`, uuid.NewString(), tenant, attempt, wsMessage); err != nil {
		t.Fatal(err)
	}
	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, AttemptID: attempt, ClientMessageID: "blocked-by-job", Text: "blocked"})
	if _, err := pool.Exec(ctx, `DELETE FROM verification_jobs WHERE tenant_id=$1`, tenant); err != nil {
		t.Fatal(err)
	}
	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: uuid.NewString(), AttemptID: attempt, ClientMessageID: "foreign", Text: "foreign"})
	answers := []string{"The family repaired the garden wall.", "The mortar must dry before the next course.", "I do not know.", "Rushing the work would weaken the wall."}
	for i, answer := range answers {
		message, inserted, err := r.AppendMessage(ctx, verification.DialogueContext{TenantID: tenant, StudentID: student, OccurrenceID: occurrence, RequirementID: requirement, AttemptID: attempt, PolicyVersion: "dialogue.v1"}, uuid.NewString(), "student", answer, "terra-"+uuid.NewString())
		if err != nil || !inserted {
			t.Fatalf("append %d inserted=%v err=%v", i, inserted, err)
		}
		if err := s.runDialogueJob(ctx, jobs.DialogueJob{TenantID: tenant, AttemptID: attempt, MessageID: message.ID}); err != nil {
			t.Fatalf("dialogue run %d: %v", i, err)
		}
		if i == 1 {
			// The next answer is intentionally accepted after the first two; the
			// separate idempotency assertion below covers provider replay.
			var n int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_evaluations WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt).Scan(&n); err != nil || n != 2 {
				t.Fatalf("evaluation count=%d err=%v", n, err)
			}
		}
	}
	var decisions, completions, evaluations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_evaluations WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt).Scan(&evaluations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_occurrences WHERE tenant_id=$1 AND id=$2 AND status='completed'`, tenant, occurrence).Scan(&completions); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 || evaluations != 4 || completions != 1 {
		t.Fatalf("decisions=%d evaluations=%d completions=%d", decisions, evaluations, completions)
	}
	terminalBinding, err := s.bindStudentAttempt(ctx, identity, occurrence, attempt)
	if err != nil {
		t.Fatal(err)
	}
	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, AttemptID: attempt, ClientMessageID: "after-terminal", Text: "must be rejected"})
	_, _, _, conflict, err = s.appendStudentMessage(ctx, terminalBinding, identity, "stale-sequence", "stale", 1)
	if err != nil || !conflict {
		t.Fatalf("terminal/stale message conflict=%v err=%v", conflict, err)
	}
	// Replaying the terminal provider job returns durable evidence and cannot
	// create another decision or completion transition.
	var finalMessage string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2 ORDER BY sequence DESC LIMIT 1`, tenant, attempt).Scan(&finalMessage); err != nil {
		t.Fatal(err)
	}
	terminalState, err := r.GetDialogueState(ctx, verification.DialogueContext{TenantID: tenant, StudentID: student, OccurrenceID: occurrence, RequirementID: requirement, AttemptID: attempt, PolicyVersion: "dialogue.v1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, prior := range terminalState.Evaluations {
		if prior.MessageID == finalMessage {
			_, ready, inserted, evalErr := r.RecordAnswerEvaluation(ctx, verification.DialogueContext{TenantID: tenant, StudentID: student, OccurrenceID: occurrence, RequirementID: requirement, AttemptID: attempt, PolicyVersion: "dialogue.v1", MessageID: finalMessage}, "rushing", prior)
			if evalErr != nil || inserted || !ready.Accepted {
				t.Fatalf("idempotent evaluation ready=%+v inserted=%v err=%v", ready, inserted, evalErr)
			}
		}
	}
	if err := s.runDialogueJob(ctx, jobs.DialogueJob{TenantID: tenant, AttemptID: attempt, MessageID: finalMessage}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt).Scan(&decisions); err != nil || decisions != 1 {
		t.Fatalf("replay decisions=%d err=%v", decisions, err)
	}
	// A provider outage is durable worker state, not an in-memory error. The
	// disabled provider path must fail the leased job, and a student retry must
	// requeue that exact message rather than create a second answer.
	providerScope := verification.DialogueContext{TenantID: tenant, StudentID: student, OccurrenceID: occurrence, RequirementID: requirement, AttemptID: attempt, PolicyVersion: "dialogue.v1"}
	providerFailureMessage, _, err := r.AppendMessage(ctx, providerScope, uuid.NewString(), "student", "provider failure answer", "provider-failure")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASKS_AGENT_MODE", "disabled")
	if err := s.runDialogueJob(ctx, jobs.DialogueJob{TenantID: tenant, AttemptID: attempt, MessageID: providerFailureMessage.ID}); !errors.Is(err, agent.ErrProviderDisabled) {
		t.Fatalf("disabled provider error=%v", err)
	}
	failureJob := uuid.NewString()
	failureQueue := jobs.NewPostgresRepository(pool)
	if err := failureQueue.EnqueueDialogue(ctx, jobs.DialogueJob{ID: failureJob, TenantID: tenant, AttemptID: attempt, MessageID: providerFailureMessage.ID, MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.claimAndRunDialogue(ctx, failureQueue); err != nil {
		t.Fatalf("failed dialogue job claim returned %v", err)
	}
	var failureStatus, failureError string
	if err := pool.QueryRow(ctx, `SELECT status,last_error FROM verification_jobs WHERE id=$1`, failureJob).Scan(&failureStatus, &failureError); err != nil {
		t.Fatal(err)
	}
	if failureStatus != "failed" || failureError == "" {
		t.Fatalf("provider failure job status=%q error=%q", failureStatus, failureError)
	}
	// A reconnecting student receives the server-owned failed projection, not
	// fallback completion prose. This exercises the durable safe-retry route.
	failureRoute := chi.NewRouteContext()
	failureRoute.URLParams.Add("id", occurrence)
	failureStateReq := httptest.NewRequest(http.MethodGet, "/student/occurrences/"+occurrence+"/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, failureRoute))
	failureStateRec := httptest.NewRecorder()
	s.studentDialogueState(failureStateRec, failureStateReq, uuid.MustParse(student))
	if failureStateRec.Code != http.StatusOK || !bytes.Contains(failureStateRec.Body.Bytes(), []byte(`"status":"error"`)) || !bytes.Contains(failureStateRec.Body.Bytes(), []byte("Your answer is saved")) {
		t.Fatalf("failed dialogue state status=%d body=%s", failureStateRec.Code, failureStateRec.Body.String())
	}
	s.studentRetry(ctx, identity, sub, studentCommand{Type: "retry", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, AttemptID: attempt})
	if err := pool.QueryRow(ctx, `SELECT status FROM verification_jobs WHERE id=$1`, failureJob).Scan(&failureStatus); err != nil || failureStatus != "queued" {
		t.Fatalf("retry status=%q err=%v", failureStatus, err)
	}
	// The remaining worker assertions use the deterministic provider again; the
	// failed job above remains queued as the retry contract requires.
	t.Setenv("TASKS_AGENT_MODE", "scripted")

	// Parent inspect and override use the same tenant-scoped domain operation;
	// override evidence is append-only and does not rewrite the transcript.
	route := chi.NewRouteContext()
	route.URLParams.Add("id", occurrence)
	inspectReq := httptest.NewRequest(http.MethodGet, "/occurrences/"+occurrence+"/inspect", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	inspectRec := httptest.NewRecorder()
	s.dialogueInspect(inspectRec, inspectReq, scopeForParent(tenant, "parent-a"))
	if inspectRec.Code != http.StatusOK || !bytes.Contains(inspectRec.Body.Bytes(), []byte("garden wall")) || !bytes.Contains(inspectRec.Body.Bytes(), []byte(`"provider":"scripted"`)) || !bytes.Contains(inspectRec.Body.Bytes(), []byte(`"policyVersion":"dialogue.v1"`)) {
		t.Fatalf("inspect status=%d body=%s", inspectRec.Code, inspectRec.Body.String())
	}
	var count int
	overrideReq := httptest.NewRequest(http.MethodPost, "/occurrences/"+occurrence+"/override", bytes.NewBufferString(`{"accepted":true,"reason":"Parent reviewed the durable transcript."}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	overrideRec := httptest.NewRecorder()
	s.dialogueOverride(overrideRec, overrideReq, scopeForParent(tenant, "parent-a"))
	if overrideRec.Code != http.StatusOK {
		t.Fatalf("override status=%d body=%s", overrideRec.Code, overrideRec.Body.String())
	}
	overrideReq = httptest.NewRequest(http.MethodPost, "/occurrences/"+occurrence+"/override", bytes.NewBufferString(`{"accepted":true,"reason":"Parent reviewed the durable transcript again."}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	overrideRec = httptest.NewRecorder()
	s.dialogueOverride(overrideRec, overrideReq, scopeForParent(tenant, "parent-a"))
	if overrideRec.Code != http.StatusOK {
		t.Fatalf("second override status=%d body=%s", overrideRec.Code, overrideRec.Body.String())
	}
	// A rejection exercises the opposite state transition while the durable
	// decision remains append-only and the completed occurrence is protected.
	overrideReq = httptest.NewRequest(http.MethodPost, "/occurrences/"+occurrence+"/override", bytes.NewBufferString(`{"accepted":false,"reason":"Parent requires one more specific answer."}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	overrideRec = httptest.NewRecorder()
	s.dialogueOverride(overrideRec, overrideReq, scopeForParent(tenant, "parent-a"))
	if overrideRec.Code != http.StatusOK || !bytes.Contains(overrideRec.Body.Bytes(), []byte(`"status":"pending"`)) {
		t.Fatalf("rejected override status=%d body=%s", overrideRec.Code, overrideRec.Body.String())
	}
	stateReq := httptest.NewRequest(http.MethodGet, "/student/occurrences/"+occurrence+"/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	stateRec := httptest.NewRecorder()
	s.studentDialogueState(stateRec, stateReq, uuid.MustParse(student))
	if stateRec.Code != http.StatusOK || !bytes.Contains(stateRec.Body.Bytes(), []byte("requiredCount")) {
		t.Fatalf("student state status=%d body=%s", stateRec.Code, stateRec.Body.String())
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_overrides WHERE tenant_id=$1 AND occurrence_id=$2`, tenant, occurrence).Scan(&count); err != nil || count != 3 {
		t.Fatalf("override rows=%d err=%v", count, err)
	}

	// Two callers using the same client key observe one durable message.
	scope := verification.DialogueContext{TenantID: tenant, StudentID: student, OccurrenceID: occurrence, RequirementID: requirement, AttemptID: attempt, PolicyVersion: "dialogue.v1"}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = r.AppendMessage(ctx, scope, uuid.NewString(), "student", "duplicate replay", "same-client-key")
		}()
	}
	wg.Wait()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2 AND client_message_id='same-client-key'`, tenant, attempt).Scan(&count); err != nil || count != 1 {
		t.Fatalf("idempotency rows=%d err=%v", count, err)
	}

	// Two provider callbacks for one student message must converge on one
	// evaluation even when both callbacks pass the natural-key lookup before
	// either transaction commits.
	startMessage, inserted, err := r.AppendMessage(ctx, startScope, uuid.NewString(), "student", "The family repaired the wall.", "concurrent-answer")
	if err != nil || !inserted {
		t.Fatalf("concurrent answer inserted=%v err=%v", inserted, err)
	}
	startQuestion, inserted, err := r.RecordQuestion(ctx, startScope, verification.DialogueQuestion{ID: uuid.NewString(), QuestionKey: "concurrency", Prompt: "What did the family repair?", Ordinal: 1})
	if err != nil || !inserted {
		t.Fatalf("concurrent question inserted=%v err=%v", inserted, err)
	}
	callbackScope := startScope
	callbackScope.MessageID = startMessage.ID
	var callbacks sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		callbacks.Add(1)
		go func(n int) {
			defer callbacks.Done()
			_, _, _, callbackErr := r.RecordAnswerEvaluation(ctx, callbackScope, startQuestion.QuestionKey, verification.DialogueEvaluation{
				ID: uuid.NewString(), MessageID: startMessage.ID, QuestionID: startQuestion.ID, Accepted: true,
				Criteria: []string{"answers address the distinct question"}, Rationale: "answer identifies the source fact", PolicyVersion: "dialogue.v1", Provider: "scripted", Model: "fixture",
			})
			errCh <- callbackErr
		}(i)
	}
	callbacks.Wait()
	close(errCh)
	for callbackErr := range errCh {
		if callbackErr != nil {
			t.Fatalf("concurrent evaluation: %v", callbackErr)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_evaluations WHERE tenant_id=$1 AND attempt_id=$2 AND message_id=$3`, tenant, startAttempt, startMessage.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("concurrent evaluation rows=%d err=%v", count, err)
	}

	q := jobs.NewPostgresRepository(pool)
	// The worker claim path is exercised separately from direct execution: an
	// already-evaluated terminal message is safe to replay and still completes
	// the leased job.
	workerJobID := uuid.NewString()
	if err := q.EnqueueDialogue(ctx, jobs.DialogueJob{ID: workerJobID, TenantID: tenant, AttemptID: attempt, MessageID: finalMessage, MaxAttempts: 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.claimAndRunDialogue(ctx, q); err != nil {
		t.Fatalf("claimed terminal dialogue job: %v", err)
	}
	var workerStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM verification_jobs WHERE id=$1`, workerJobID).Scan(&workerStatus); err != nil || workerStatus != "succeeded" {
		t.Fatalf("worker job status=%q err=%v", workerStatus, err)
	}

	queuedMessage, _, err := r.AppendMessage(ctx, scope, uuid.NewString(), "student", "post-terminal durable message", "post-terminal")
	if err != nil {
		t.Fatal(err)
	}
	jobID := uuid.NewString()
	if err := q.EnqueueDialogue(ctx, jobs.DialogueJob{ID: jobID, TenantID: tenant, AttemptID: attempt, MessageID: queuedMessage.ID, MaxAttempts: 2}); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := q.ClaimDialogue(ctx, "owner-a", time.Second)
	if err != nil || !ok || claimed.ID != jobID {
		t.Fatalf("claim=%+v ok=%v err=%v", claimed, ok, err)
	}
	if err := q.CompleteDialogue(ctx, jobID, "owner-a"); err != nil {
		t.Fatal(err)
	}
	if err := q.AppendDialogueEvent(ctx, jobs.DialogueEvent{TenantID: tenant, AttemptID: attempt, Sequence: 1, Kind: "state", Payload: []byte(`{"status":"open"}`)}); err != nil {
		t.Fatal(err)
	}
	events, err := q.ReplayDialogueEvents(ctx, tenant, attempt, 0, 10)
	if err != nil || len(events) < 1 {
		t.Fatalf("replay=%+v err=%v", events, err)
	}
	failedMessage, _, err := r.AppendMessage(ctx, scope, uuid.NewString(), "student", "provider timeout answer", "provider-timeout")
	if err != nil {
		t.Fatal(err)
	}
	failedJob := uuid.NewString()
	if err := q.EnqueueDialogue(ctx, jobs.DialogueJob{ID: failedJob, TenantID: tenant, AttemptID: attempt, MessageID: failedMessage.ID, MaxAttempts: 2}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := q.ClaimDialogue(ctx, "owner-b", time.Second); err != nil || !ok {
		t.Fatalf("failed claim ok=%v err=%v", ok, err)
	}
	if err := q.FailDialogue(ctx, failedJob, "owner-b", errors.New("provider timeout")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE verification_jobs SET status='running', lease_until=now()-interval '1 second' WHERE id=$1`, failedJob); err != nil {
		t.Fatal(err)
	}
	if err := q.RequeueExpiredDialogue(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	cancelMessage, _, err := r.AppendMessage(ctx, startScope, uuid.NewString(), "student", "retry after cancellation", "cancel-answer")
	if err != nil {
		t.Fatal(err)
	}
	cancelJob := uuid.NewString()
	if err := q.EnqueueDialogue(ctx, jobs.DialogueJob{ID: cancelJob, TenantID: tenant, AttemptID: startAttempt, MessageID: cancelMessage.ID, MaxAttempts: 2}); err != nil {
		t.Fatal(err)
	}
	cancelled, ok, err := q.ClaimDialogue(ctx, "owner-cancel", time.Second)
	if err != nil || !ok || cancelled.ID != cancelJob {
		t.Fatalf("cancel claim=%+v ok=%v err=%v", cancelled, ok, err)
	}
	if err := q.FailDialogue(ctx, cancelJob, "owner-cancel", context.Canceled); err != nil {
		t.Fatal(err)
	}
	var lastError string
	if err := pool.QueryRow(ctx, `SELECT last_error FROM verification_jobs WHERE id=$1`, cancelJob).Scan(&lastError); err != nil || lastError != "canceled" {
		t.Fatalf("canceled job last_error=%q err=%v", lastError, err)
	}
	// A terminal rejected attempt projects a safe error state and cannot appear
	// complete merely because the browser reconnects.
	if _, err := pool.Exec(ctx, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,false,'dialogue exhausted','verification_engine')`, uuid.NewString(), tenant, startAttempt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE verification_attempts SET status='rejected' WHERE tenant_id=$1 AND id=$2`, tenant, startAttempt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM verification_jobs WHERE tenant_id=$1 AND attempt_id=$2`, tenant, startAttempt); err != nil {
		t.Fatal(err)
	}
	rejectedStateRec := httptest.NewRecorder()
	s.studentDialogueState(rejectedStateRec, startStateReq, uuid.MustParse(student))
	if rejectedStateRec.Code != http.StatusOK || !bytes.Contains(rejectedStateRec.Body.Bytes(), []byte(`"status":"error"`)) {
		t.Fatalf("rejected terminal state status=%d body=%s", rejectedStateRec.Code, rejectedStateRec.Body.String())
	}
}
