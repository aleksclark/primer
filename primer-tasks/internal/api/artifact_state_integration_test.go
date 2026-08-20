package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestArtifactStateAndRetryAreTenantAndStudentScoped(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	studentA, studentB := uuid.New(), uuid.New()
	template, revision, requirement := uuid.New(), uuid.New(), uuid.New()
	schedule, occurrence, attempt, artifactID, submission, job := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	cleanup := []string{
		"DELETE FROM artifact_rubric_events WHERE tenant_id=$1",
		"DELETE FROM artifact_criterion_evaluations WHERE tenant_id=$1",
		"DELETE FROM artifact_rubric_jobs WHERE tenant_id=$1",
		"DELETE FROM artifact_submissions WHERE tenant_id=$1",
		"DELETE FROM artifacts WHERE tenant_id=$1",
		"DELETE FROM verification_decisions WHERE tenant_id=$1",
		"DELETE FROM verification_attempts WHERE tenant_id=$1",
		"DELETE FROM task_occurrences WHERE tenant_id=$1",
		"DELETE FROM verification_requirements WHERE tenant_id=$1",
		"DELETE FROM task_schedules WHERE tenant_id=$1",
		"DELETE FROM task_revisions WHERE tenant_id=$1",
		"DELETE FROM task_templates WHERE tenant_id=$1",
		"DELETE FROM students WHERE tenant_id=$1",
		"DELETE FROM tenants WHERE id=$1",
	}
	t.Cleanup(func() {
		for _, query := range cleanup {
			_, _ = pool.Exec(context.Background(), query, tenantA)
		}
		_, _ = pool.Exec(context.Background(), `DELETE FROM students WHERE tenant_id=$1`, tenantB)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tenants WHERE id=$1`, tenantB)
	})
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO tenants(id,name) VALUES($1,'State A'),($2,'State B')`, tenantA, tenantB)
	exec(`INSERT INTO students(id,tenant_id,display_name) VALUES($1,$3,'Alice'),($2,$4,'Bob')`, studentA, studentB, tenantA, tenantB)
	exec(`INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'State task','published',1)`, template, tenantA)
	exec(`INSERT INTO task_revisions(id,tenant_id,template_id,version,title,status) VALUES($1,$2,$3,1,'State task','published')`, revision, tenantA, template)
	config := `{"acceptedKinds":["image"],"criteria":[{"id":"shows-work","label":"Shows work","description":"The artifact shows work.","required":true}],"passRule":"all_required","reviewPolicy":"parent_review"}`
	exec(`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_artifact_rubric',1,$4,'artifact_upload','fantasy')`, requirement, tenantA, revision, config)
	exec(`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, schedule, tenantA, studentA, template, revision)
	exec(`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification')`, occurrence, tenantA, schedule, studentA, revision)
	exec(`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, attempt, tenantA, occurrence, requirement)
	exec(`INSERT INTO artifacts(id,tenant_id,student_id,object_key,kind,original_name,declared_content_type,byte_size,sha256,status,expires_at) VALUES($1,$2,$3,'state-artifact','image','work.png','image/png',4,'digest','finalized',now()+interval '1 day')`, artifactID, tenantA, studentA)
	exec(`INSERT INTO artifact_submissions(id,tenant_id,student_id,occurrence_id,requirement_id,attempt_id,artifact_id,idempotency_key,status) VALUES($1,$2,$3,$4,$5,$6,$7,'state-submission','review')`, submission, tenantA, studentA, occurrence, requirement, attempt, artifactID)
	exec(`INSERT INTO artifact_rubric_jobs(id,tenant_id,submission_id,status,provider,model,rubric_snapshot) VALUES($1,$2,$3,'review','scripted','fixture',$4)`, job, tenantA, submission, config)
	exec(`INSERT INTO artifact_criterion_evaluations(id,tenant_id,submission_id,criterion_id,required,status,evidence,feedback,provider,model,policy_version) VALUES($1,$2,$3,'shows-work',true,'accepted','evidence','feedback','scripted','fixture','agent_artifact_rubric.v1')`, uuid.New(), tenantA, submission)

	s := NewWithStore(pool, "test", nil)
	// The student protocol replays the durable snapshot for every artifact
	// status and never trusts a browser-provided status.
	for _, stored := range []string{"review", "evaluating", "accepted", "rejected"} {
		exec(`UPDATE artifact_submissions SET status=$2 WHERE id=$1`, submission, stored)
		sub := &studentSubscriber{queue: make(chan wireStudentEvent, 8), done: make(chan struct{})}
		s.studentArtifactSubscribe(ctx, studentIdentity{TenantID: tenantA.String(), StudentID: studentA}, sub, studentCommand{OccurrenceID: occurrence.String(), SubmissionID: submission.String()})
		select {
		case event := <-sub.queue:
			if event.OccurrenceID != occurrence.String() || event.Status != stored {
				t.Fatalf("stored=%s replay=%+v", stored, event)
			}
		default:
			t.Fatalf("stored=%s produced no snapshot", stored)
		}
	}
	missingSub := &studentSubscriber{queue: make(chan wireStudentEvent, 2), done: make(chan struct{})}
	s.studentArtifactSubscribe(ctx, studentIdentity{TenantID: tenantA.String(), StudentID: studentA}, missingSub, studentCommand{OccurrenceID: occurrence.String(), SubmissionID: uuid.NewString()})
	if event := <-missingSub.queue; event.Code != "not_found" {
		t.Fatalf("missing subscription event=%+v", event)
	}
	exec(`UPDATE artifact_submissions SET status='review' WHERE id=$1`, submission)
	exec(`INSERT INTO artifact_rubric_events(tenant_id,job_id,submission_id,sequence,kind,payload) VALUES($1,$2,$3,1,'progress','{"status":"review","message":"safe progress"}')`, tenantA, job, submission)
	replaySub := &studentSubscriber{queue: make(chan wireStudentEvent, 8), done: make(chan struct{})}
	s.studentArtifactSubscribe(ctx, studentIdentity{TenantID: tenantA.String(), StudentID: studentA}, replaySub, studentCommand{OccurrenceID: occurrence.String(), SubmissionID: submission.String()})
	<-replaySub.queue // snapshot
	if event := <-replaySub.queue; event.Sequence != 1 || event.Status != "review" || event.Message != "safe progress" {
		t.Fatalf("durable artifact replay=%+v", event)
	}
	stateRequest := func(occ string) *http.Request {
		rc := chi.NewRouteContext()
		rc.URLParams.Add("occurrence", occ)
		return httptest.NewRequest(http.MethodGet, "/student/occurrences/"+occ+"/artifacts", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rc))
	}
	retryRequest := func() *http.Request {
		rc := chi.NewRouteContext()
		rc.URLParams.Add("occurrence", occurrence.String())
		return httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/retry", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rc))
	}
	decodeState := func(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var state map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		return state
	}

	readyOccurrence := uuid.New()
	exec(`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status) VALUES($1,$2,$3,$4,$5,now()+interval '1 day',now()+interval '1 day 1 hour','pending')`, readyOccurrence, tenantA, schedule, studentA, revision)
	readyRec := httptest.NewRecorder()
	s.studentArtifactState(readyRec, stateRequest(readyOccurrence.String()), studentA)
	if readyRec.Code != http.StatusOK || !strings.Contains(readyRec.Body.String(), `"status":"ready"`) {
		t.Fatalf("ready state status=%d body=%s", readyRec.Code, readyRec.Body.String())
	}
	missingStateRec := httptest.NewRecorder()
	s.studentArtifactState(missingStateRec, stateRequest(uuid.NewString()), studentA)
	if missingStateRec.Code != http.StatusNotFound {
		t.Fatalf("missing student state status=%d body=%s", missingStateRec.Code, missingStateRec.Body.String())
	}

	// A student sees only their occurrence, while a parent in the owning
	// tenant can inspect it without the student filter.
	studentRec := httptest.NewRecorder()
	s.studentArtifactState(studentRec, stateRequest(occurrence.String()), studentA)
	if studentRec.Code != http.StatusOK {
		t.Fatalf("student state status=%d body=%s", studentRec.Code, studentRec.Body.String())
	}
	state := decodeState(t, studentRec)
	if state["status"] != "review" || state["activeSubmissionId"] != submission.String() {
		t.Fatalf("review state=%v", state)
	}
	evaluation, ok := state["evaluation"].(map[string]any)
	if !ok || evaluation["provider"] != "scripted" || len(evaluation["criteria"].([]any)) != 1 {
		t.Fatalf("evaluation=%v", state["evaluation"])
	}
	parentRec := httptest.NewRecorder()
	s.parentArtifactState(parentRec, stateRequest(occurrence.String()), scope{Tenant: tenantA.String()})
	if parentRec.Code != http.StatusOK {
		t.Fatalf("parent state status=%d body=%s", parentRec.Code, parentRec.Body.String())
	}

	// Only failed/review jobs can be retried, and the retry is scoped through
	// the submission's tenant, occurrence, and student in one UPDATE.
	retryRec := httptest.NewRecorder()
	s.retryArtifactEvaluation(retryRec, retryRequest(), studentA)
	if retryRec.Code != http.StatusOK {
		t.Fatalf("review retry status=%d body=%s", retryRec.Code, retryRec.Body.String())
	}
	var jobStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM artifact_rubric_jobs WHERE id=$1`, job).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "queued" {
		t.Fatalf("retry job status=%s", jobStatus)
	}
	conflictRec := httptest.NewRecorder()
	s.retryArtifactEvaluation(conflictRec, retryRequest(), studentA)
	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("non-terminal retry status=%d body=%s", conflictRec.Code, conflictRec.Body.String())
	}

	// Failed jobs take the same authorized retry path; a foreign student gets
	// a generic conflict and cannot change the job.
	exec(`UPDATE artifact_rubric_jobs SET status='failed' WHERE id=$1`, job)
	exec(`UPDATE artifact_submissions SET status='rejected' WHERE id=$1`, submission)
	foreignRec := httptest.NewRecorder()
	s.retryArtifactEvaluation(foreignRec, retryRequest(), studentB)
	if foreignRec.Code != http.StatusConflict {
		t.Fatalf("foreign retry status=%d body=%s", foreignRec.Code, foreignRec.Body.String())
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM artifact_rubric_jobs WHERE id=$1`, job).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "failed" {
		t.Fatalf("foreign retry mutated job=%s", jobStatus)
	}
	if retried, err := s.retryArtifactState(ctx, studentA, occurrence.String()); err != nil || retried.Status != "rejected" {
		t.Fatalf("typed retry state = %#v, err=%v", retried, err)
	}
	if _, err := s.retryArtifactState(ctx, studentA, occurrence.String()); err == nil || err.(interface{ GetStatus() int }).GetStatus() != http.StatusConflict {
		t.Fatalf("non-retryable typed state error=%v", err)
	}

	// Revoked/unknown students receive the same non-disclosing auth result for
	// state and retry rather than an existence oracle.
	revoked := uuid.New()
	revokedState := httptest.NewRecorder()
	s.studentArtifactState(revokedState, stateRequest(occurrence.String()), revoked)
	if revokedState.Code != http.StatusUnauthorized {
		t.Fatalf("revoked state status=%d body=%s", revokedState.Code, revokedState.Body.String())
	}
	revokedRetry := httptest.NewRecorder()
	s.retryArtifactEvaluation(revokedRetry, retryRequest(), revoked)
	if revokedRetry.Code != http.StatusUnauthorized {
		t.Fatalf("revoked retry status=%d body=%s", revokedRetry.Code, revokedRetry.Body.String())
	}
	if _, err := s.retryArtifactState(ctx, revoked, occurrence.String()); err == nil || err.(interface{ GetStatus() int }).GetStatus() != http.StatusUnauthorized {
		t.Fatalf("revoked typed state error=%v", err)
	}

	// A foreign parent tenant cannot inspect the occurrence either.
	foreignParent := httptest.NewRecorder()
	s.parentArtifactState(foreignParent, stateRequest(occurrence.String()), scope{Tenant: tenantB.String()})
	if foreignParent.Code != http.StatusNotFound {
		t.Fatalf("foreign parent status=%d body=%s", foreignParent.Code, foreignParent.Body.String())
	}

	for _, tc := range []struct {
		stored, exposed string
	}{
		{"submitted", "evaluating"},
		{"evaluating", "evaluating"},
		{"rejected", "rejected"},
	} {
		exec(`UPDATE artifact_submissions SET status=$2 WHERE id=$1`, submission, tc.stored)
		mappedRec := httptest.NewRecorder()
		s.studentArtifactState(mappedRec, stateRequest(occurrence.String()), studentA)
		if mappedRec.Code != http.StatusOK {
			t.Fatalf("mapped %s status=%d body=%s", tc.stored, mappedRec.Code, mappedRec.Body.String())
		}
		mapped := decodeState(t, mappedRec)
		if mapped["status"] != tc.exposed {
			t.Fatalf("stored=%s state=%v", tc.stored, mapped)
		}
	}

	// The state mapper exposes terminal acceptance rather than leaking the
	// internal succeeded/accepted vocabulary to the student protocol.
	exec(`UPDATE artifact_submissions SET status='accepted' WHERE id=$1`, submission)
	exec(`UPDATE artifact_rubric_jobs SET status='succeeded',provider='scripted',model='fixture' WHERE id=$1`, job)
	exec(`UPDATE artifact_submissions SET status='accepted' WHERE id=$1`, submission)
	acceptedRec := httptest.NewRecorder()
	s.studentArtifactState(acceptedRec, stateRequest(occurrence.String()), studentA)
	if acceptedRec.Code != http.StatusOK {
		t.Fatalf("accepted state status=%d body=%s", acceptedRec.Code, acceptedRec.Body.String())
	}
	accepted := decodeState(t, acceptedRec)
	if accepted["status"] != "complete" || accepted["evaluation"].(map[string]any)["accepted"] != true {
		t.Fatalf("accepted state=%v", accepted)
	}
}
