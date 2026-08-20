package api

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"primer-tasks/internal/verification"
)

// TestArtifactLeaseReclaimFencesEveryWorkerMutation plants the failure mode
// that a long provider call can expose: worker A loses its lease, worker B
// reclaims the job, and A resumes. Every worker-owned mutation must reject A;
// B must still be able to finish the same job.
func TestArtifactLeaseReclaimFencesEveryWorkerMutation(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	tenant, student, template, revision, requirement := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
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
			_, _ = pool.Exec(context.Background(), query, tenant)
		}
	})
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}

	exec(`INSERT INTO tenants(id,name) VALUES($1,'lease fencing')`, tenant)
	exec(`INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'Lease student')`, student, tenant)
	exec(`INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'Lease task','published',1)`, template, tenant)
	exec(`INSERT INTO task_revisions(id,tenant_id,template_id,version,title,status) VALUES($1,$2,$3,1,'Lease task','published')`, revision, tenant, template)
	rubricJSON := `{"acceptedKinds":["image"],"criteria":[{"id":"shows-work","label":"Shows work","description":"The image shows the work.","required":true}],"passRule":"all_required","reviewPolicy":"parent_review"}`
	exec(`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_artifact_rubric',1,$4,'artifact_upload','fantasy')`, requirement, tenant, revision, rubricJSON)
	exec(`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, schedule, tenant, student, template, revision)
	exec(`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification')`, occurrence, tenant, schedule, student, revision)
	exec(`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, attempt, tenant, occurrence, requirement)
	exec(`INSERT INTO artifacts(id,tenant_id,student_id,object_key,kind,original_name,declared_content_type,byte_size,expires_at) VALUES($1,$2,$3,$4,'image','poem.png','image/png',4,now()+interval '1 day')`, artifactID, tenant, student, "lease-artifact")
	exec(`INSERT INTO artifact_submissions(id,tenant_id,student_id,occurrence_id,requirement_id,attempt_id,artifact_id,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,'lease-submission')`, submission, tenant, student, occurrence, requirement, attempt, artifactID)
	exec(`INSERT INTO artifact_rubric_jobs(id,tenant_id,submission_id,rubric_snapshot,status,available_at,lease_owner,lease_until) VALUES($1,$2,$3,$4,'running',now()-interval '1 minute','old-worker',now()-interval '1 second')`, job, tenant, submission, rubricJSON)

	// Reclaim exactly as the worker does, then claim as the replacement worker.
	exec(`UPDATE artifact_rubric_jobs SET status='queued',lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE status='running' AND lease_until<now()`)
	var claimed string
	if err := pool.QueryRow(ctx, `UPDATE artifact_rubric_jobs SET status='running',lease_owner='new-worker',lease_until=now()+interval '5 minutes',updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='queued' AND available_at<=now() RETURNING id`, tenant, job).Scan(&claimed); err != nil {
		t.Fatal("replacement worker did not reclaim expired job:", err)
	}
	if claimed != job.String() {
		t.Fatalf("claimed job=%s, want %s", claimed, job)
	}

	s := NewWithStore(pool, "test", nil)
	oldCtx := withArtifactLease(ctx, "old-worker", job.String())
	newCtx := withArtifactLease(ctx, "new-worker", job.String())
	rubric := verification.ArtifactRubric{
		AcceptedKinds: []string{"image"},
		Criteria:      []verification.ArtifactCriterion{{ID: "shows-work", Label: "Shows work", Description: "The image shows the work.", Required: true}},
		PassRule:      "all_required",
		ReviewPolicy:  "parent_review",
	}
	decision := verification.Decision{ID: uuid.NewString(), TenantID: tenant.String(), AttemptID: attempt.String(), OccurrenceID: occurrence.String(), Accepted: true, Reason: "accepted", DecidedBy: "verification_engine"}
	oldMutations := []struct {
		name string
		call func() error
	}{
		{"progress", func() error {
			return s.appendArtifactProgress(oldCtx, tenant.String(), job.String(), submission.String(), "old-progress", map[string]any{"worker": "old"})
		}},
		{"criterion progress", func() error {
			_, _, err := (artifactRubricBackend{server: s}).RecordArtifactCriterion(oldCtx, verification.ArtifactContext{TenantID: tenant.String(), JobID: job.String(), SubmissionID: submission.String(), Rubric: rubric, Provider: "old", Model: "old", PolicyVersion: verification.ArtifactRubricPolicyVersion}, verification.ArtifactCriterionResult{CriterionID: "shows-work", Required: true, Status: "accepted", Evidence: "old evidence"})
			return err
		}},
		{"retry", func() error {
			return s.retryOrResolveArtifact(oldCtx, job.String(), tenant.String(), submission.String(), rubric, "old retry")
		}},
		{"policy", func() error {
			return s.resolveArtifactPolicy(oldCtx, job.String(), tenant.String(), submission.String(), rubric, "old policy")
		}},
		{"compatibility review", func() error {
			return s.finishArtifactReview(oldCtx, job.String(), tenant.String(), submission.String(), "old review")
		}},
		{"failure policy", func() error {
			return s.failArtifactJob(oldCtx, job.String(), tenant.String(), submission.String(), "old failure")
		}},
		{"completion", func() error {
			return s.finishArtifactDecision(oldCtx, job.String(), tenant.String(), submission.String(), "old", "old", true, true)
		}},
		{"decision", func() error {
			_, err := s.CommitDecision(oldCtx, decision)
			return err
		}},
	}
	for _, mutation := range oldMutations {
		t.Run(mutation.name, func(t *testing.T) {
			if err := mutation.call(); err == nil || !errors.Is(err, errArtifactLeaseLost) {
				t.Fatalf("old worker error=%v, want lease loss", err)
			}
		})
	}

	var jobStatus, owner, submissionStatus, attemptStatus, occurrenceStatus string
	var events, criteria, decisions int
	if err := pool.QueryRow(ctx, `SELECT status,COALESCE(lease_owner,''), (SELECT status FROM artifact_submissions WHERE id=$2), (SELECT status FROM verification_attempts WHERE id=$3), (SELECT status FROM task_occurrences WHERE id=$4), (SELECT count(*) FROM artifact_rubric_events WHERE job_id=$1), (SELECT count(*) FROM artifact_criterion_evaluations WHERE submission_id=$2), (SELECT count(*) FROM verification_decisions WHERE attempt_id=$3) FROM artifact_rubric_jobs WHERE id=$1`, job, submission, attempt, occurrence).Scan(&jobStatus, &owner, &submissionStatus, &attemptStatus, &occurrenceStatus, &events, &criteria, &decisions); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "running" || owner != "new-worker" || submissionStatus != "submitted" || attemptStatus != "open" || occurrenceStatus != "awaiting_verification" || events != 0 || criteria != 0 || decisions != 0 {
		t.Fatalf("stale worker mutated state: job=%s owner=%s submission=%s attempt=%s occurrence=%s events=%d criteria=%d decisions=%d", jobStatus, owner, submissionStatus, attemptStatus, occurrenceStatus, events, criteria, decisions)
	}

	// Exercise the live policy and compatibility terminal paths while the
	// replacement lease is active. Each path leaves a durable event and then
	// the test explicitly reopens the fixture for the next worker operation.
	if err := s.resolveArtifactPolicy(newCtx, job.String(), tenant.String(), submission.String(), rubric, "parent review"); err != nil {
		t.Fatal("replacement policy:", err)
	}
	exec(`UPDATE artifact_submissions SET status='submitted' WHERE id=$1`, submission)
	exec(`UPDATE artifact_rubric_jobs SET status='running',lease_owner='new-worker',lease_until=now()+interval '5 minutes',rubric_snapshot=$2 WHERE id=$1`, job, rubricJSON)
	if err := s.finishArtifactReview(newCtx, job.String(), tenant.String(), submission.String(), "manual review"); err != nil {
		t.Fatal("replacement compatibility review:", err)
	}
	exec(`UPDATE artifact_submissions SET status='submitted' WHERE id=$1`, submission)
	exec(`UPDATE artifact_rubric_jobs SET status='running',lease_owner='new-worker',lease_until=now()+interval '5 minutes',rubric_snapshot=$2,attempts=0 WHERE id=$1`, job, rubricJSON)
	rejectRubric := rubric
	rejectRubric.ReviewPolicy = "reject"
	if err := s.resolveArtifactPolicy(newCtx, job.String(), tenant.String(), submission.String(), rejectRubric, "required criterion rejected"); err != nil {
		t.Fatal("replacement reject policy:", err)
	}
	exec(`UPDATE artifact_submissions SET status='submitted' WHERE id=$1`, submission)
	exec(`UPDATE artifact_rubric_jobs SET status='running',lease_owner='new-worker',lease_until=now()+interval '5 minutes',rubric_snapshot=$2,attempts=2,max_attempts=2 WHERE id=$1`, job, rubricJSON)
	if err := s.retryOrResolveArtifact(newCtx, job.String(), tenant.String(), submission.String(), rubric, "attempts exhausted"); err != nil {
		t.Fatal("replacement exhausted policy:", err)
	}
	exec(`UPDATE artifact_submissions SET status='submitted' WHERE id=$1`, submission)
	exec(`UPDATE artifact_rubric_jobs SET status='running',lease_owner='new-worker',lease_until=now()+interval '5 minutes',rubric_snapshot='{}'::jsonb,attempts=0,max_attempts=2 WHERE id=$1`, job)
	if err := s.failArtifactJob(newCtx, job.String(), tenant.String(), submission.String(), "invalid snapshot"); err != nil {
		t.Fatal("replacement failure:", err)
	}
	exec(`UPDATE artifact_submissions SET status='submitted' WHERE id=$1`, submission)
	exec(`UPDATE artifact_rubric_jobs SET status='running',lease_owner='new-worker',lease_until=now()+interval '5 minutes',rubric_snapshot=$2 WHERE id=$1`, job, rubricJSON)
	if err := s.retryOrResolveArtifact(newCtx, job.String(), tenant.String(), submission.String(), rubric, "provider retry"); err != nil {
		t.Fatal("replacement retry:", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM artifact_rubric_jobs WHERE id=$1`, job).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "queued" {
		t.Fatalf("retry status=%s, want queued", jobStatus)
	}
	exec(`UPDATE artifact_rubric_jobs SET status='running',lease_owner='new-worker-2',lease_until=now()+interval '5 minutes' WHERE id=$1`, job)
	newCtx = withArtifactLease(ctx, "new-worker-2", job.String())

	// The replacement worker is not merely allowed to read the lease: it can
	// record progress, commit the decision, and atomically complete the job.
	if _, _, err := (artifactRubricBackend{server: s}).RecordArtifactCriterion(newCtx, verification.ArtifactContext{TenantID: tenant.String(), JobID: job.String(), SubmissionID: submission.String(), Rubric: rubric, Provider: "new", Model: "new", PolicyVersion: verification.ArtifactRubricPolicyVersion}, verification.ArtifactCriterionResult{CriterionID: "shows-work", Required: true, Status: "accepted", Evidence: "new evidence"}); err != nil {
		t.Fatal("replacement criterion:", err)
	}
	inserted, err := s.CommitDecision(newCtx, decision)
	if err != nil || !inserted {
		t.Fatalf("replacement decision inserted=%v err=%v", inserted, err)
	}
	if err := s.finishArtifactDecision(newCtx, job.String(), tenant.String(), submission.String(), "new", "new", true, true); err != nil {
		t.Fatal("replacement completion:", err)
	}
	if err := pool.QueryRow(ctx, `SELECT (SELECT status FROM artifact_rubric_jobs WHERE id=$1), (SELECT status FROM artifact_submissions WHERE id=$2), (SELECT status FROM verification_attempts WHERE id=$3), (SELECT status FROM task_occurrences WHERE id=$4), (SELECT count(*) FROM artifact_rubric_events WHERE job_id=$1), (SELECT count(*) FROM artifact_criterion_evaluations WHERE submission_id=$2), (SELECT count(*) FROM verification_decisions WHERE attempt_id=$3)`, job, submission, attempt, occurrence).Scan(&jobStatus, &submissionStatus, &attemptStatus, &occurrenceStatus, &events, &criteria, &decisions); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "succeeded" || submissionStatus != "accepted" || attemptStatus != "accepted" || occurrenceStatus != "completed" || events != 8 || criteria != 1 || decisions != 1 {
		t.Fatalf("replacement state job=%s submission=%s attempt=%s occurrence=%s events=%d criteria=%d decisions=%d", jobStatus, submissionStatus, attemptStatus, occurrenceStatus, events, criteria, decisions)
	}
}
