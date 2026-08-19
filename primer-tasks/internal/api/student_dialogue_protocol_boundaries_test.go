package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"primer-tasks/internal/domain"
	"primer-tasks/internal/verification"
)

// TestStudentDialogueProtocolBoundaries drives the durable protocol adapter
// through its optimistic-concurrency, idempotency, job-gating, and retry
// transitions. It deliberately uses the real PostgreSQL rows that bind a
// student socket to one tenant-scoped dialogue attempt.
func TestStudentDialogueProtocolBoundaries(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	tenant, student, template, revision, requirement := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	schedule, occurrence, attempt := uuid.NewString(), uuid.NewString(), uuid.NewString()
	cleanup := func() {
		for _, query := range []string{
			"DELETE FROM verification_events WHERE tenant_id=$1",
			"DELETE FROM verification_jobs WHERE tenant_id=$1",
			"DELETE FROM verification_evaluations WHERE tenant_id=$1",
			"DELETE FROM dialogue_questions WHERE tenant_id=$1",
			"DELETE FROM verification_messages WHERE tenant_id=$1",
			"DELETE FROM dialogue_attempts WHERE tenant_id=$1",
			"DELETE FROM verification_decisions WHERE tenant_id=$1",
			"DELETE FROM verification_attempts WHERE tenant_id=$1",
			"DELETE FROM task_occurrences WHERE tenant_id=$1",
			"DELETE FROM task_schedules WHERE tenant_id=$1",
			"DELETE FROM verification_requirements WHERE tenant_id=$1",
			"DELETE FROM task_revisions WHERE tenant_id=$1",
			"DELETE FROM task_templates WHERE tenant_id=$1",
			"DELETE FROM students WHERE tenant_id=$1",
			"DELETE FROM tenants WHERE id=$1",
		} {
			_, _ = pool.Exec(context.Background(), query, tenant)
		}
	}
	t.Cleanup(cleanup)
	for _, args := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO tenants(id,name) VALUES($1,'Protocol boundaries')`, []any{tenant}},
		{`INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'Terra')`, []any{student, tenant}},
		{`INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'Dialogue','published',1)`, []any{template, tenant}},
		{`INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions,status) VALUES($1,$2,$3,1,'Dialogue','Answer questions','published')`, []any{revision, tenant, template}},
		{`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_dialogue',1,$4,'chat','fantasy')`, []any{requirement, tenant, revision, `{"sourceRef":"fixture://chapter-4","learningFocus":"Recall source facts","requiredQuestions":3,"rubric":["answers cite the source"],"allowedFollowUps":1,"maxTurns":8,"maxAttempts":2,"retentionPolicy":"retain"}`}},
		{`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, []any{schedule, tenant, student, template, revision}},
		{`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status,revision_snapshot) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification','{}')`, []any{occurrence, tenant, schedule, student, revision}},
		{`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, []any{attempt, tenant, occurrence, requirement}},
		{`INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,policy_version,config_snapshot,accepted_count,turn_count,next_sequence) VALUES($1,$2,$3,$4,'dialogue.v1',$5,0,1,1)`, []any{tenant, attempt, occurrence, requirement, `{"sourceRef":"fixture://chapter-4","learningFocus":"Recall source facts","requiredQuestions":3,"rubric":["answers cite the source"],"allowedFollowUps":1,"maxTurns":8,"maxAttempts":2,"retentionPolicy":"retain"}`}},
	} {
		if _, err := pool.Exec(ctx, args.query, args.args...); err != nil {
			t.Fatal(err)
		}
	}

	s := New(pool, "test")
	identity := studentIdentity{StudentID: uuid.MustParse(student), TenantID: tenant, Source: "bearer", Token: "device"}
	binding, err := s.bindStudentAttempt(ctx, identity, occurrence, attempt)
	if err != nil || binding.AttemptID != attempt || binding.Required != 3 || binding.MaxTurns != 8 {
		t.Fatalf("binding=%+v err=%v", binding, err)
	}
	if _, err := s.bindStudentAttempt(ctx, identity, occurrence, uuid.NewString()); err == nil {
		t.Fatal("foreign attempt was bound to the occurrence")
	}
	if _, err := s.bindStudentAttempt(ctx, identity, "", ""); err == nil {
		t.Fatal("empty attempt selector was accepted")
	}

	// Replay includes one valid durable event and skips malformed payloads; the
	// persisted question suppresses the starter prompt from the source fixture.
	statePayload, _ := json.Marshal(wireStudentEvent{Type: "question", Text: "Persisted question"})
	if _, err := pool.Exec(ctx, `INSERT INTO verification_events(tenant_id,attempt_id,sequence,kind,payload) VALUES($1,$2,1,'question',$3),($1,$2,2,'broken','"not an event"')`, tenant, attempt, statePayload); err != nil {
		t.Fatal(err)
	}
	sub := &studentSubscriber{queue: make(chan wireStudentEvent, 16), done: make(chan struct{})}
	s.studentSubscribe(ctx, identity, sub, studentCommand{Type: "subscribe", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, Cursor: 0})
	if len(sub.queue) != 2 {
		t.Fatalf("replay queue length=%d, want state and valid question", len(sub.queue))
	}
	<-sub.queue // binding state
	replayed := <-sub.queue
	if replayed.Type != "question" || replayed.Text != "Persisted question" || replayed.Sequence != 1 {
		t.Fatalf("replayed event=%+v", replayed)
	}

	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence})
	invalid := <-sub.queue
	// The invalid request is intentionally non-retryable; a client must not be
	// able to turn malformed input into a retry loop.
	if invalid.Code != "invalid_request" || invalid.Retryable {
		t.Fatalf("invalid message event=%+v", invalid)
	}

	first := studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, ClientMessageID: "answer-1", Text: "A durable answer"}
	s.studentMessage(ctx, identity, sub, first)
	var messageCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt).Scan(&messageCount); err != nil || messageCount != 1 {
		t.Fatalf("first message count=%d err=%v", messageCount, err)
	}
	drainStudentEvents(sub)
	// Reusing the client key acknowledges the durable message without creating
	// a second row, even if the replacement text differs.
	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, ClientMessageID: "answer-1", Text: "replayed replacement"})
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt).Scan(&messageCount); err != nil || messageCount != 1 {
		t.Fatalf("idempotent message count=%d err=%v", messageCount, err)
	}
	drainStudentEvents(sub)

	if _, err := pool.Exec(ctx, `DELETE FROM verification_jobs WHERE tenant_id=$1`, tenant); err != nil {
		t.Fatal(err)
	}
	// Two devices racing with one client key converge on one message under the
	// same advisory transaction lock used by the websocket path.
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, _, appendErr := s.appendStudentMessage(ctx, binding, identity, "concurrent-key", "racing answer", 0)
			errCh <- appendErr
		}()
	}
	wg.Wait()
	close(errCh)
	for appendErr := range errCh {
		if appendErr != nil {
			t.Fatal(appendErr)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2 AND client_message_id='concurrent-key'`, tenant, attempt).Scan(&messageCount); err != nil || messageCount != 1 {
		t.Fatalf("concurrent message count=%d err=%v", messageCount, err)
	}

	var messageID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt).Scan(&messageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM verification_jobs WHERE tenant_id=$1`, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,message_id,status,max_attempts) VALUES($1,$2,$3,$4,'queued',1)`, uuid.NewString(), tenant, attempt, messageID); err != nil {
		t.Fatal(err)
	}
	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, ClientMessageID: "answer-while-busy", Text: "blocked"})
	busy := <-sub.queue
	if busy.Code != "conflict" || !busy.Retryable {
		t.Fatalf("busy event=%+v", busy)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM verification_jobs WHERE tenant_id=$1`, tenant); err != nil {
		t.Fatal(err)
	}
	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, ClientMessageID: "stale", Text: "stale", ExpectedSequence: 1})
	stale := <-sub.queue
	if stale.Code != "conflict" || stale.Cursor != 3 || !stale.Retryable {
		t.Fatalf("stale sequence event=%+v", stale)
	}

	// The turn limit is a transactional rejection: it records the decision and
	// exhausts the attempt instead of silently accepting one more answer.
	if _, err := pool.Exec(ctx, `UPDATE dialogue_attempts SET turn_count=8 WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt); err != nil {
		t.Fatal(err)
	}
	s.studentMessage(ctx, identity, sub, studentCommand{Type: "user_message", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, ClientMessageID: "over-limit", Text: "too late"})
	overLimit := <-sub.queue
	if overLimit.Code != "conflict" || !overLimit.Retryable {
		t.Fatalf("over-limit event=%+v", overLimit)
	}
	var attemptStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM verification_attempts WHERE id=$1`, attempt).Scan(&attemptStatus); err != nil || attemptStatus != "exhausted" {
		t.Fatalf("attempt status=%q err=%v", attemptStatus, err)
	}
	// A retry is rejected until a worker has durably marked an evaluation failed.
	s.studentRetry(ctx, identity, sub, studentCommand{Type: "retry", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, AttemptID: attempt})
	noRetry := <-sub.queue
	if noRetry.Code != "conflict" || noRetry.Retryable {
		t.Fatalf("no-retry event=%+v", noRetry)
	}

	retryMessage := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO verification_messages(id,tenant_id,attempt_id,sequence,role,content,client_message_id) VALUES($1,$2,$3,3,'student','retry me','retry-client')`, retryMessage, tenant, attempt); err != nil {
		t.Fatal(err)
	}
	retryJob := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,message_id,status,max_attempts,last_error) VALUES($1,$2,$3,$4,'failed',3,'provider timeout')`, retryJob, tenant, attempt, retryMessage); err != nil {
		t.Fatal(err)
	}
	s.studentRetry(ctx, identity, sub, studentCommand{Type: "retry", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, AttemptID: attempt})
	var retryStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM verification_jobs WHERE id=$1`, retryJob).Scan(&retryStatus); err != nil || retryStatus != "queued" {
		t.Fatalf("retry job status=%q err=%v", retryStatus, err)
	}

	stateRoute := chi.NewRouteContext()
	stateRoute.URLParams.Add("id", occurrence)
	stateRec := httptest.NewRecorder()
	stateReq := httptest.NewRequest(http.MethodGet, "/student/occurrences/"+occurrence+"/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, stateRoute))
	s.studentDialogueState(stateRec, stateReq, uuid.MustParse(student))
	if stateRec.Code != http.StatusOK || !containsAny(stateRec.Body.String(), `"status":"error"`) {
		t.Fatalf("exhausted dialogue state status=%d body=%s", stateRec.Code, stateRec.Body.String())
	}

	// An open attempt with a source outside the immutable allowlist is rejected
	// at subscription rather than receiving a fact from another chapter.
	if _, err := pool.Exec(ctx, `UPDATE verification_attempts SET status='open' WHERE tenant_id=$1 AND id=$2`, tenant, attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE dialogue_attempts SET config_snapshot='{"sourceRef":"fixture://not-available","learningFocus":"Recall source facts","requiredQuestions":3,"rubric":["answers cite the source"],"allowedFollowUps":1,"maxTurns":8,"maxAttempts":2,"retentionPolicy":"retain"}' WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt); err != nil {
		t.Fatal(err)
	}
	unsupportedSub := &studentSubscriber{queue: make(chan wireStudentEvent, 4), done: make(chan struct{})}
	s.studentSubscribe(ctx, identity, unsupportedSub, studentCommand{Type: "subscribe", ProtocolVersion: studentProtocolVersion, OccurrenceID: occurrence, Cursor: 100000})
	if len(unsupportedSub.queue) != 2 {
		t.Fatalf("unsupported source events=%d", len(unsupportedSub.queue))
	}
	<-unsupportedSub.queue // state
	unsupported := <-unsupportedSub.queue
	if unsupported.Code != "unsupported_source" || unsupported.Retryable {
		t.Fatalf("unsupported source event=%+v", unsupported)
	}

	// Inspect keeps agent evidence, evaluations, questions, and parent overrides
	// as distinct timeline records. Include every optional evaluation field so
	// the HTTP projection is checked rather than merely line-touched.
	agentMessageID, questionID := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO verification_messages(id,tenant_id,attempt_id,sequence,role,content,client_message_id) VALUES($1,$2,$3,4,'agent','Follow-up question status','agent-status')`, agentMessageID, tenant, attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dialogue_questions(id,tenant_id,attempt_id,question_key,ordinal,prompt) VALUES($1,$2,$3,'follow-up',1,'What detail supports the answer?')`, questionID, tenant, attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO verification_evaluations(id,tenant_id,attempt_id,question_id,message_id,accepted,rationale,provider,model,policy_version,usage) VALUES($1,$2,$3,$4,$5,false,'needs a source detail','scripted','fixture-model','dialogue.v1','{"inputTokens":4}')`, uuid.NewString(), tenant, attempt, questionID, agentMessageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO verification_overrides(id,tenant_id,occurrence_id,requirement_id,attempt_id,accepted,reason,actor_id) VALUES($1,$2,$3,$4,$5,false,'manual review remains open','parent-a')`, uuid.NewString(), tenant, occurrence, requirement, attempt); err != nil {
		t.Fatal(err)
	}
	inspectRoute := chi.NewRouteContext()
	inspectRoute.URLParams.Add("id", occurrence)
	inspectRec := httptest.NewRecorder()
	inspectReq := httptest.NewRequest(http.MethodGet, "/occurrences/"+occurrence+"/inspect", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, inspectRoute))
	s.dialogueInspect(inspectRec, inspectReq, scope{Tenant: tenant, Subject: "parent-a"})
	if inspectRec.Code != http.StatusOK || !containsAny(inspectRec.Body.String(), "Agent message", "needs a source detail", "fixture-model", "manual review remains open") {
		t.Fatalf("dialogue inspect status=%d body=%s", inspectRec.Code, inspectRec.Body.String())
	}

	// Publishing is also used by a reconnecting worker. Verify the open state
	// fallback and both terminal-status fallbacks, including old states that did
	// not persist TerminalStatus explicitly.
	dialogueScope := verification.DialogueContext{TenantID: tenant, StudentID: student, OccurrenceID: occurrence, RequirementID: requirement, AttemptID: attempt, PolicyVersion: "dialogue.v1"}
	config := domain.DialogueConfig{RequiredQuestions: 3}
	for _, tc := range []struct {
		name          string
		state         verification.DialogueState
		wantStatus    string
		wantEventType string
	}{
		{name: "open", state: verification.DialogueState{Config: config}, wantStatus: "open", wantEventType: "state"},
		{name: "terminal-rejected", state: verification.DialogueState{Config: config, Terminal: true}, wantStatus: "rejected", wantEventType: "complete"},
		{name: "terminal-accepted", state: verification.DialogueState{Config: config, Terminal: true, AcceptedCount: 3}, wantStatus: "accepted", wantEventType: "complete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.publishDialogueResult(ctx, dialogueScope, tc.state); err != nil {
				t.Fatal(err)
			}
			var payload []byte
			if err := pool.QueryRow(ctx, `SELECT payload FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2 ORDER BY sequence DESC LIMIT 1`, tenant, attempt).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			var event wireStudentEvent
			if err := json.Unmarshal(payload, &event); err != nil || event.Type != tc.wantEventType || event.Status != tc.wantStatus {
				t.Fatalf("published event=%+v err=%v", event, err)
			}
		})
	}
}

func drainStudentEvents(sub *studentSubscriber) {
	for len(sub.queue) > 0 {
		<-sub.queue
	}
}
