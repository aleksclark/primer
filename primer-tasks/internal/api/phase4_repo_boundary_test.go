package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"primer-tasks/internal/jobs"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
)

func TestPhase4DialogueRepositoryDurableValidationAndReplay(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	tenant, student, template, revision, requirement := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	schedule, occurrence, attempt := uuid.NewString(), uuid.NewString(), uuid.NewString()
	config := `{"sourceRef":"fixture://chapter-4","learningFocus":"Recall source facts","requiredQuestions":1,"rubric":["answers address the distinct question"],"allowedFollowUps":0,"maxAttempts":1,"maxTurns":3,"retentionPolicy":"retain"}`
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO tenants(id,name) VALUES($1,'Repository boundaries')`, []any{tenant}},
		{`INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'Repository student')`, []any{student, tenant}},
		{`INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'Repository dialogue','published',1)`, []any{template, tenant}},
		{`INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions,status) VALUES($1,$2,$3,1,'Repository dialogue','Answer one question','published')`, []any{revision, tenant, template}},
		{`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_dialogue',1,$4,'chat','fantasy')`, []any{requirement, tenant, revision, config}},
		{`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, []any{schedule, tenant, student, template, revision}},
		{`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status,revision_snapshot) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification','{}')`, []any{occurrence, tenant, schedule, student, revision}},
		{`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, []any{attempt, tenant, occurrence, requirement}},
		{`INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,policy_version,config_snapshot,next_sequence) VALUES($1,$2,$3,$4,'dialogue.v1',$5,1)`, []any{tenant, attempt, occurrence, requirement, config}},
	} {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	retryReq, retrySchedule, retryOccurrence, retryAttempt := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	retryConfig := strings.Replace(config, `"maxAttempts":1`, `"maxAttempts":2`, 1)
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,2,'agent_dialogue',1,$4,'chat','fantasy')`, []any{retryReq, tenant, revision, retryConfig}},
		{`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, []any{retrySchedule, tenant, student, template, revision}},
		{`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status,revision_snapshot) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','pending','{}')`, []any{retryOccurrence, tenant, retrySchedule, student, revision}},
		{`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number,status) VALUES($1,$2,$3,$4,1,'rejected')`, []any{retryAttempt, tenant, retryOccurrence, retryReq}},
		{`INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,policy_version,config_snapshot,next_sequence) VALUES($1,$2,$3,$4,'dialogue.v1',$5,1)`, []any{tenant, retryAttempt, retryOccurrence, retryReq, retryConfig}},
	} {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, query := range []string{
			"DELETE FROM verification_jobs WHERE tenant_id=$1", "DELETE FROM verification_evaluations WHERE tenant_id=$1", "DELETE FROM dialogue_questions WHERE tenant_id=$1", "DELETE FROM verification_messages WHERE tenant_id=$1", "DELETE FROM dialogue_attempts WHERE tenant_id=$1", "DELETE FROM verification_attempts WHERE tenant_id=$1", "DELETE FROM task_occurrences WHERE tenant_id=$1", "DELETE FROM verification_requirements WHERE tenant_id=$1", "DELETE FROM task_schedules WHERE tenant_id=$1", "DELETE FROM task_revisions WHERE tenant_id=$1", "DELETE FROM task_templates WHERE tenant_id=$1", "DELETE FROM students WHERE tenant_id=$1", "DELETE FROM tenants WHERE id=$1",
		} {
			_, _ = pool.Exec(context.Background(), query, tenant)
		}
	})

	r := repo.NewDialogueRepository(pool)
	dialogueScope := verification.DialogueContext{TenantID: tenant, StudentID: student, OccurrenceID: occurrence, RequirementID: requirement, AttemptID: attempt, PolicyVersion: "dialogue.v1"}
	if _, _, err := r.AppendMessage(ctx, dialogueScope, uuid.NewString(), "student", "answer", "client"); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct{ role, content, clientID string }{
		"bad role":       {role: "model", content: "answer", clientID: "bad-role"},
		"missing client": {role: "student", content: "answer", clientID: " "},
		"empty content":  {role: "student", content: "", clientID: "empty"},
		"long content":   {role: "student", content: strings.Repeat("x", 12001), clientID: "long"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := r.AppendMessage(ctx, dialogueScope, uuid.NewString(), tc.role, tc.content, tc.clientID); err == nil {
				t.Fatal("invalid message accepted")
			}
		})
	}
	first, inserted, err := r.AppendMessage(ctx, dialogueScope, uuid.NewString(), "student", "answer", "client")
	if err != nil || inserted || first.Content != "answer" {
		t.Fatalf("message replay=%+v inserted=%v err=%v", first, inserted, err)
	}
	question := verification.DialogueQuestion{ID: uuid.NewString(), QuestionKey: "wall", Prompt: "What did the family repair?", Ordinal: 1}
	if _, inserted, err := r.RecordQuestion(ctx, dialogueScope, question); err != nil || !inserted {
		t.Fatalf("question inserted=%v err=%v", inserted, err)
	}
	// Two independent providers may race before either question is visible. The
	// unique natural key returns the committed question to the loser.
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, recordErr := r.RecordQuestion(ctx, dialogueScope, verification.DialogueQuestion{ID: uuid.NewString(), QuestionKey: "follow-up", Prompt: "Name another detail.", Ordinal: 2})
			errCh <- recordErr
		}()
	}
	wg.Wait()
	close(errCh)
	for recordErr := range errCh {
		if recordErr != nil && !errors.Is(recordErr, verification.ErrDuplicateQuestion) {
			t.Fatal(recordErr)
		}
	}
	var questionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dialogue_questions WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt).Scan(&questionCount); err != nil || questionCount != 2 {
		t.Fatalf("question count=%d err=%v", questionCount, err)
	}

	message, inserted, err := r.AppendMessage(ctx, dialogueScope, uuid.NewString(), "student", "The family repaired the wall.", "evaluated")
	if err != nil || !inserted {
		t.Fatalf("evaluation message inserted=%v err=%v", inserted, err)
	}
	evalScope := dialogueScope
	evalScope.MessageID = message.ID
	evaluation := verification.DialogueEvaluation{ID: uuid.NewString(), MessageID: message.ID, Accepted: false, Rationale: "needs a source detail", PolicyVersion: "dialogue.v1", Provider: "fixture", Model: "fixture"}
	recorded, ready, inserted, err := r.RecordAnswerEvaluation(ctx, evalScope, "wall", evaluation)
	if err != nil || !inserted || ready.Accepted || recorded.ID == "" {
		t.Fatalf("recorded=%+v ready=%+v inserted=%v err=%v", recorded, ready, inserted, err)
	}
	replayed, ready, inserted, err := r.RecordAnswerEvaluation(ctx, evalScope, "wall", evaluation)
	if err != nil || inserted || replayed.ID != recorded.ID || ready.Accepted {
		t.Fatalf("replayed=%+v ready=%+v inserted=%v err=%v", replayed, ready, inserted, err)
	}
	if _, _, _, err := r.RecordAnswerEvaluation(ctx, dialogueScope, "missing-question", evaluation); !errors.Is(err, verification.ErrDialogueEvaluation) {
		t.Fatalf("missing question error=%v", err)
	}
	state, err := r.GetDialogueState(ctx, dialogueScope)
	if err != nil || len(state.Messages) != 2 || len(state.Questions) != 2 || len(state.Evaluations) != 1 {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if _, err := r.GetDialogueState(ctx, verification.DialogueContext{TenantID: tenant}); !errors.Is(err, verification.ErrDialogueContext) {
		t.Fatalf("invalid state context error=%v", err)
	}
	// Each child projection is read after the durable attempt row. A failed
	// projection query is surfaced rather than returning a deceptively partial
	// transcript.
	for _, table := range []string{"dialogue_questions", "verification_evaluations", "verification_messages"} {
		hidden := table + "_repo_phase4_hidden"
		if _, err := pool.Exec(ctx, `ALTER TABLE `+table+` RENAME TO `+hidden); err != nil {
			t.Fatal(err)
		}
		if _, err := r.GetDialogueState(ctx, dialogueScope); err == nil {
			t.Fatalf("state read unexpectedly succeeded with %s unavailable", table)
		}
		if _, err := pool.Exec(ctx, `ALTER TABLE `+hidden+` RENAME TO `+table); err != nil {
			t.Fatal(err)
		}
	}

	// A rejected attempt is terminal evidence. A provider replay for a new
	// message must not record a post-terminal evaluation.
	if _, err := pool.Exec(ctx, `UPDATE verification_attempts SET status='rejected' WHERE tenant_id=$1 AND id=$2`, tenant, attempt); err != nil {
		t.Fatal(err)
	}
	terminalMessage, inserted, err := r.AppendMessage(ctx, dialogueScope, uuid.NewString(), "student", "late answer", "late-answer")
	if err != nil || !inserted {
		t.Fatalf("terminal message inserted=%v err=%v", inserted, err)
	}
	terminalScope := dialogueScope
	terminalScope.MessageID = terminalMessage.ID
	if _, _, _, err := r.RecordAnswerEvaluation(ctx, terminalScope, "wall", verification.DialogueEvaluation{ID: uuid.NewString(), MessageID: terminalMessage.ID, Accepted: true, Criteria: []string{"answers address the distinct question"}, Rationale: "late", PolicyVersion: "dialogue.v1"}); !errors.Is(err, verification.ErrDialogueTerminal) {
		t.Fatalf("post-terminal evaluation error=%v", err)
	}

	// A rejected attempt cannot silently create a second dialogue when the
	// parent-authored maxAttempts snapshot is already exhausted.
	if _, err := pool.Exec(ctx, `UPDATE verification_attempts SET status='rejected' WHERE tenant_id=$1 AND id=$2`, tenant, attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE task_occurrences SET status='pending' WHERE tenant_id=$1 AND id=$2`, tenant, occurrence); err != nil {
		t.Fatal(err)
	}
	route := chi.NewRouteContext()
	route.URLParams.Add("id", occurrence)
	retryRequestMain := httptest.NewRequest(http.MethodPost, "/occurrences/"+occurrence+"/retry", bytes.NewBufferString(`{}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	retryRec := httptest.NewRecorder()
	(&Server{DB: pool}).retryOccurrence2(retryRec, retryRequestMain, scope{Tenant: tenant, Subject: "parent"})
	if retryRec.Code != http.StatusConflict || !bytes.Contains(retryRec.Body.Bytes(), []byte("attempt limit")) {
		t.Fatalf("max-attempt retry status=%d body=%s", retryRec.Code, retryRec.Body.String())
	}
	// A rejected attempt within the immutable max-attempt policy creates a new
	// attempt and dialogue projection rather than mutating prior evidence.
	retryRoute2 := chi.NewRouteContext()
	retryRoute2.URLParams.Add("id", retryOccurrence)
	retryRequest2 := httptest.NewRequest(http.MethodPost, "/occurrences/"+retryOccurrence+"/retry", bytes.NewBufferString(`{}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, retryRoute2))
	retryAllowed := httptest.NewRecorder()
	(&Server{DB: pool}).retryOccurrence2(retryAllowed, retryRequest2, scope{Tenant: tenant, Subject: "parent"})
	if retryAllowed.Code != http.StatusOK || !bytes.Contains(retryAllowed.Body.Bytes(), []byte(`"attemptNumber":2`)) {
		t.Fatalf("allowed retry status=%d body=%s", retryAllowed.Code, retryAllowed.Body.String())
	}
	stateRoute := chi.NewRouteContext()
	stateRoute.URLParams.Add("id", occurrence)
	stateRequest := httptest.NewRequest(http.MethodGet, "/student/occurrences/"+occurrence+"/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, stateRoute))
	stateResponse := httptest.NewRecorder()
	(&Server{DB: pool}).studentDialogueState(stateResponse, stateRequest, uuid.MustParse(student))
	if stateResponse.Code != http.StatusOK || !bytes.Contains(stateResponse.Body.Bytes(), []byte(`"status":"error"`)) {
		t.Fatalf("rejected student state status=%d body=%s", stateResponse.Code, stateResponse.Body.String())
	}
	// Starting a fresh occurrence snapshots the requirement inside the same
	// transaction. A malformed parent policy blocks the start rather than
	// creating a partially initialized dialogue attempt.
	badSchedule, badOccurrence := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now()+interval '1 day')`, badSchedule, tenant, student, template, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status,revision_snapshot) VALUES($1,$2,$3,$4,$5,now()+interval '1 day',now()+interval '1 day 1 hour','pending','{}')`, badOccurrence, tenant, badSchedule, student, revision); err != nil {
		t.Fatal(err)
	}
	badRetryRoute := chi.NewRouteContext()
	badRetryRoute.URLParams.Add("id", badOccurrence)
	badRetryRequest := httptest.NewRequest(http.MethodPost, "/occurrences/"+badOccurrence+"/retry", bytes.NewBufferString(`{}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, badRetryRoute))
	badRetryResponse := httptest.NewRecorder()
	(&Server{DB: pool}).retryOccurrence2(badRetryResponse, badRetryRequest, scope{Tenant: tenant, Subject: "parent"})
	if badRetryResponse.Code != http.StatusConflict {
		t.Fatalf("retry without prior attempt status=%d body=%s", badRetryResponse.Code, badRetryResponse.Body.String())
	}
	missingRetryRoute := chi.NewRouteContext()
	missingRetryRoute.URLParams.Add("id", uuid.NewString())
	missingRetryRequest := httptest.NewRequest(http.MethodPost, "/occurrences/missing/retry", bytes.NewBufferString(`{}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, missingRetryRoute))
	missingRetryResponse := httptest.NewRecorder()
	(&Server{DB: pool}).retryOccurrence2(missingRetryResponse, missingRetryRequest, scope{Tenant: tenant, Subject: "parent"})
	if missingRetryResponse.Code != http.StatusNotFound {
		t.Fatalf("retry missing occurrence status=%d body=%s", missingRetryResponse.Code, missingRetryResponse.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE verification_requirements SET config='{}' WHERE tenant_id=$1 AND id=$2`, tenant, requirement); err != nil {
		t.Fatal(err)
	}
	badRoute := chi.NewRouteContext()
	badRoute.URLParams.Add("id", badOccurrence)
	badReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+badOccurrence+"/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, badRoute))
	badRec := httptest.NewRecorder()
	(&Server{DB: pool}).startOccurrence2(badRec, badReq, uuid.MustParse(student))
	if badRec.Code != http.StatusConflict || !bytes.Contains(badRec.Body.Bytes(), []byte("configuration is invalid")) {
		t.Fatalf("invalid start status=%d body=%s", badRec.Code, badRec.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE verification_requirements SET config=$1 WHERE tenant_id=$2 AND id=$3`, config, tenant, requirement); err != nil {
		t.Fatal(err)
	}
	noReqRevision, noReqSchedule, noReqOccurrence := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions,status) VALUES($1,$2,$3,2,'No requirement','No verifier','published')`, noReqRevision, tenant, template); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now()+interval '2 days')`, noReqSchedule, tenant, student, template, noReqRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status,revision_snapshot) VALUES($1,$2,$3,$4,$5,now()+interval '2 days',now()+interval '2 days 1 hour','pending','{}')`, noReqOccurrence, tenant, noReqSchedule, student, noReqRevision); err != nil {
		t.Fatal(err)
	}
	noReqRoute := chi.NewRouteContext()
	noReqRoute.URLParams.Add("id", noReqOccurrence)
	noReqReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+noReqOccurrence+"/dialogue", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, noReqRoute))
	noReqRec := httptest.NewRecorder()
	(&Server{DB: pool}).startOccurrence2(noReqRec, noReqReq, uuid.MustParse(student))
	if noReqRec.Code != http.StatusConflict || !bytes.Contains(noReqRec.Body.Bytes(), []byte("requirement unavailable")) {
		t.Fatalf("missing requirement status=%d body=%s", noReqRec.Code, noReqRec.Body.String())
	}

	// A deleted projection row leaves the Phase 2 retry snapshot unavailable;
	// retry must stop before creating a new verification attempt.
	if _, err := pool.Exec(ctx, `DELETE FROM dialogue_attempts WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt); err != nil {
		t.Fatal(err)
	}
	unavailableRetryReq := httptest.NewRequest(http.MethodPost, "/occurrences/"+occurrence+"/retry", bytes.NewBufferString(`{}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	unavailableRetryRec := httptest.NewRecorder()
	(&Server{DB: pool}).retryOccurrence2(unavailableRetryRec, unavailableRetryReq, scope{Tenant: tenant, Subject: "parent"})
	if unavailableRetryRec.Code != http.StatusConflict || !bytes.Contains(unavailableRetryRec.Body.Bytes(), []byte("snapshot is unavailable")) {
		t.Fatalf("unavailable retry status=%d body=%s", unavailableRetryRec.Code, unavailableRetryRec.Body.String())
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,policy_version,config_snapshot,next_sequence) VALUES($1,$2,$3,$4,'dialogue.v1',$5,1)`, tenant, attempt, occurrence, requirement, config); err != nil {
		t.Fatal(err)
	}

	inspectRequest := func() *http.Request {
		inspectRoute := chi.NewRouteContext()
		inspectRoute.URLParams.Add("id", occurrence)
		return httptest.NewRequest(http.MethodGet, "/occurrences/"+occurrence+"/inspect", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, inspectRoute))
	}
	// The occurrence lookup can succeed while each durable projection query is
	// unavailable. Renaming the table in the real PostgreSQL database models a
	// migration/connection-boundary failure without replacing the repository.
	for _, table := range []string{"verification_messages", "dialogue_questions", "verification_overrides"} {
		hidden := table + "_phase4_hidden"
		if _, err := pool.Exec(ctx, `ALTER TABLE `+table+` RENAME TO `+hidden); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		(&Server{DB: pool}).dialogueInspect(rec, inspectRequest(), scope{Tenant: tenant, Subject: "parent"})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("inspect with %s unavailable status=%d body=%s", table, rec.Code, rec.Body.String())
		}
		if _, err := pool.Exec(ctx, `ALTER TABLE `+hidden+` RENAME TO `+table); err != nil {
			t.Fatal(err)
		}
	}
	// Override auditing is append-only. If the audit sink is unavailable the
	// transaction must fail and leave neither a decision nor an override row.
	if _, err := pool.Exec(ctx, `ALTER TABLE audit_records RENAME TO audit_records_phase4_hidden`); err != nil {
		t.Fatal(err)
	}
	overrideRoute := chi.NewRouteContext()
	overrideRoute.URLParams.Add("id", occurrence)
	overrideReq := httptest.NewRequest(http.MethodPost, "/occurrences/"+occurrence+"/override", bytes.NewBufferString(`{"accepted":false,"reason":"audit outage"}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, overrideRoute))
	overrideRec := httptest.NewRecorder()
	(&Server{DB: pool}).dialogueOverride(overrideRec, overrideReq, scope{Tenant: tenant, Subject: "parent"})
	if overrideRec.Code != http.StatusInternalServerError {
		t.Fatalf("override audit outage status=%d body=%s", overrideRec.Code, overrideRec.Body.String())
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE audit_records_phase4_hidden RENAME TO audit_records`); err != nil {
		t.Fatal(err)
	}
	q := jobs.NewPostgresRepository(pool)
	if err := q.EnqueueDialogue(ctx, jobs.DialogueJob{TenantID: tenant, AttemptID: attempt, MessageID: terminalMessage.ID, MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}
}
