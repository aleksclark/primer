package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/verification"
)

// Supplemental stale-queue precondition after an ACTUAL public terminal
// outcome. Recovery may clean the job, never clobber the existing evidence.
func assertTerminalDialogueIgnoresExpiredQueue(t *testing.T, h *publicDialogueHarness, occurrence, attempt string) {
	t.Helper()
	ctx := context.Background()
	const snapshot = `SELECT jsonb_build_object('occurrence',(SELECT to_jsonb(o) FROM task_occurrences o WHERE id=$1),'attempt',(SELECT to_jsonb(a) FROM verification_attempts a WHERE id=$2),'projection',(SELECT to_jsonb(d) FROM dialogue_attempts d WHERE attempt_id=$2),'decisions',(SELECT jsonb_agg(to_jsonb(v) ORDER BY id) FROM verification_decisions v WHERE attempt_id=$2),'overrides',(SELECT jsonb_agg(to_jsonb(v) ORDER BY id) FROM verification_overrides v WHERE attempt_id=$2),'messages',(SELECT jsonb_agg(to_jsonb(v) ORDER BY sequence) FROM verification_messages v WHERE attempt_id=$2),'evaluations',(SELECT jsonb_agg(to_jsonb(v) ORDER BY version) FROM verification_evaluations v WHERE attempt_id=$2),'events',(SELECT jsonb_agg(to_jsonb(v) ORDER BY sequence) FROM verification_events v WHERE attempt_id=$2))::text`
	var before, after, session string
	if err := h.pool.QueryRow(ctx, snapshot, occurrence, attempt).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx, `SELECT session_id FROM verification_jobs WHERE attempt_id=$1 ORDER BY created_at LIMIT 1`, attempt).Scan(&session); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if _, err := h.pool.Exec(ctx, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,session_id,job_key,status,stage,max_attempts,deadline) VALUES($1::uuid,$2,$3,$4,$1::text,'queued','question',3,clock_timestamp()-interval '1 second')`, id, tenantA, attempt, session); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status string
		if err := h.pool.QueryRow(ctx, `SELECT status FROM verification_jobs WHERE id=$1`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale terminal-context job not cleaned")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := h.pool.QueryRow(ctx, snapshot, occurrence, attempt).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("late queue recovery clobbered completed/canceled/overridden evidence")
	}
}

func waitPublicDialogueRunning(t *testing.T, h *publicDialogueHarness) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var running bool
		if err := h.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM verification_jobs WHERE attempt_id=$1 AND stage='evaluation' AND status='running')`, h.attempt).Scan(&running); err != nil {
			t.Fatal(err)
		}
		if running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("actual running dialogue evaluation not observed")
}

func TestPublicDialogueOverrideIsAuditedImmutableAndStopsLateWorker(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.stop(false)
	h.delay = "800"
	h.start()
	h.begin()
	conn := h.socket(0)
	q := h.question(conn, 0)
	h.answer(conn, q, "saved-before-override", "The family repaired the garden wall after the storm.")
	waitPublicDialogueRunning(t, h)
	state := h.state()
	originalOccurrence, originalAttempt := h.occurrence, h.attempt
	var before string
	if err := h.pool.QueryRow(context.Background(), `SELECT jsonb_agg(to_jsonb(m) ORDER BY sequence)::text FROM verification_messages m WHERE attempt_id=$1`, originalAttempt).Scan(&before); err != nil {
		t.Fatal(err)
	}
	body := DialogueOverrideRequest{AttemptID: h.attempt, ClientRequestID: "parent-observation", ExpectedVersion: state.Version, Accepted: true, Reason: "Parent independently checked the student's assigned reading."}
	stale := body
	stale.ExpectedVersion--
	h.request(h.parent, "POST", "/occurrences/"+originalOccurrence+"/override", stale, 409, nil)
	var receipt verification.DialogueOverrideReceipt
	h.request(h.parent, "POST", "/occurrences/"+originalOccurrence+"/override", body, 200, &receipt)
	if receipt.OverrideID == "" || receipt.ResultStatus != "completed" || receipt.ResultVersion != state.Version+1 {
		t.Fatal("override acknowledgment did not report actual committed state")
	}
	complete := h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "complete" })
	if complete.DecisionSource != "parent_override" || complete.AcceptedCount != 0 {
		t.Fatal("human override masqueraded as three model evaluations")
	}
	var duplicate verification.DialogueOverrideReceipt
	h.request(h.parent, "POST", "/occurrences/"+originalOccurrence+"/override", body, 200, &duplicate)
	if !reflect.DeepEqual(receipt, duplicate) {
		t.Fatal("idempotent override did not return its original receipt")
	}
	altered := body
	altered.Reason = "A different attempted replacement reason."
	h.request(h.parent, "POST", "/occurrences/"+originalOccurrence+"/override", altered, 409, nil)
	reverse := DialogueOverrideRequest{AttemptID: originalAttempt, ClientRequestID: "unsupported-reversal", ExpectedVersion: receipt.ResultVersion, Accepted: false, Reason: "Attempt to reverse already completed work."}
	var refusal struct {
		Code string `json:"code"`
	}
	h.request(h.parent, "POST", "/occurrences/"+originalOccurrence+"/override", reverse, 409, &refusal)
	if refusal.Code != "completed_reversal_unsupported" {
		t.Fatal("completed reversal did not receive the typed refusal")
	}
	// A following question from the SAME real worker proves the previous
	// in-flight Fantasy call ended; no sleep-to-infer-success or private invoke.
	conn.CloseNow()
	h.followingOccurrence()
	h.begin()
	following := h.socket(0)
	h.question(following, 0)
	var after, status string
	var overrides, evaluations, decisions, completions, audits int
	err := h.pool.QueryRow(context.Background(), `SELECT (SELECT jsonb_agg(to_jsonb(m) ORDER BY sequence)::text FROM verification_messages m WHERE attempt_id=$1),(SELECT status FROM task_occurrences WHERE id=$2),(SELECT count(*) FROM verification_overrides WHERE attempt_id=$1),(SELECT count(*) FROM verification_evaluations WHERE attempt_id=$1),(SELECT count(*) FROM verification_decisions WHERE attempt_id=$1),(SELECT count(*) FROM verification_events WHERE attempt_id=$1 AND kind='complete'),(SELECT count(*) FROM audit_records WHERE entity_id=$2 AND action='verification_override')`, originalAttempt, originalOccurrence).Scan(&after, &status, &overrides, &evaluations, &decisions, &completions, &audits)
	if err != nil || before != after || status != "completed" || overrides != 1 || evaluations != 0 || decisions != 0 || completions != 1 || audits != 1 {
		t.Fatal("late worker/idempotent override rewrote evidence or invented automatic acceptance")
	}
	var inspected DialogueInspect
	h.request(h.parent, "GET", "/occurrences/"+originalOccurrence+"/inspect", nil, 200, &inspected)
	found := false
	for _, entry := range inspected.Entries {
		if entry.Kind == "override" {
			found = entry.Author == "parent" && entry.Text == body.Reason && entry.DecisionID == receipt.OverrideID
		}
	}
	if !found {
		t.Fatal("parent inspect omitted immutable override reason/provenance")
	}
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(h.base)
	jar.SetCookies(u, []*http.Cookie{{Name: "tasks_parent", Value: "parent-b", Path: "/"}})
	foreign := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	h.request(foreign, "GET", "/occurrences/"+originalOccurrence+"/inspect", nil, 404, nil)
	h.request(foreign, "POST", "/occurrences/"+originalOccurrence+"/override", body, 404, nil)
	assertTerminalDialogueIgnoresExpiredQueue(t, h, originalOccurrence, originalAttempt)
}

func TestPublicDialogueAllRequirementsWaitsForRealManualDecision(t *testing.T) {
	h := newPublicDialogueHarnessWithPolicy(t, 1, 6, true)
	h.begin()
	conn := h.socket(0)
	q := h.question(conn, 0)
	for i, answer := range []string{"The family repaired the garden wall after the storm.", "The mortar must dry before the next course of stones.", "Rushing the work would weaken the wall."} {
		h.answer(conn, q, fmt.Sprintf("mixed-%d", i), answer)
		if i < 2 {
			q = h.question(conn, i+1)
		} else {
			h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "answer_evaluation" && e.AcceptedCount == 3 })
		}
	}
	if state := h.state(); state.AcceptedCount != 3 || state.Status != "accepted" || state.OccurrenceStatus != "awaiting_verification" {
		t.Fatal("dialogue bypassed the unsatisfied manual requirement")
	}
	if _, _, _, d, c := h.counts(); d != 1 || c != 0 {
		t.Fatal("requirement acceptance incorrectly completed the occurrence")
	}
	h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/decision", DecisionInput2{Accepted: true, Reason: "Parent observed the separate required work."}, 200, nil)
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "complete" && e.OccurrenceStatus == "completed" })
	if _, _, _, d, c := h.counts(); d != 1 || c != 1 {
		t.Fatal("final manual decision did not produce one dialogue completion event")
	}
}

func TestPublicDialogueAttemptExhaustionAndBoundedParentRetry(t *testing.T) {
	h := newPublicDialogueHarnessWithPolicy(t, 0, 3)
	h.begin()
	firstAttempt := h.attempt
	conn := h.socket(0)
	q := h.question(conn, 0)
	h.answer(conn, q, "first-insufficient", "I do not know.")
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "error" && e.Code == "exhausted" })
	if state := h.state(); state.Status != "exhausted" || state.OccurrenceStatus != "pending" {
		t.Fatal("answer policy exhaustion was not durable")
	}
	h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/retry", nil, 200, nil)
	conn.CloseNow()
	h.begin()
	if h.attempt == firstAttempt {
		t.Fatal("parent retry reused terminal attempt")
	}
	secondAttempt := h.attempt
	conn = h.socket(0)
	q = h.question(conn, 0)
	h.answer(conn, q, "second-insufficient", "I do not know.")
	h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "error" && e.Code == "exhausted" })
	h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/retry", nil, 409, nil)
	var rejected, accepted, attempts int
	if err := h.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM verification_decisions d JOIN verification_attempts a ON a.tenant_id=d.tenant_id AND a.id=d.attempt_id WHERE a.occurrence_id=$1 AND NOT d.accepted),(SELECT count(*) FROM verification_decisions d JOIN verification_attempts a ON a.tenant_id=d.tenant_id AND a.id=d.attempt_id WHERE a.occurrence_id=$1 AND d.accepted),(SELECT count(*) FROM verification_attempts WHERE occurrence_id=$1)`, h.occurrence).Scan(&rejected, &accepted, &attempts); err != nil || rejected != 2 || accepted != 0 || attempts != 2 {
		t.Fatal("retry limit lost prior rejection evidence or created another attempt")
	}
	for _, attempt := range []string{firstAttempt, secondAttempt} {
		var inspect DialogueInspect
		h.request(h.parent, "GET", "/occurrences/"+h.occurrence+"/inspect?attemptId="+attempt, nil, 200, &inspect)
		answers := 0
		for _, entry := range inspect.Entries {
			if entry.Author == "student" {
				answers++
			}
		}
		if answers != 1 || inspect.AttemptTotal != 2 {
			t.Fatal("parent cannot inspect immutable prior attempt history")
		}
	}
}

// Expiry fields are supplemental PostgreSQL preconditions, not accepted
// decisions or a private recovery call. The production process/queue performs
// recovery and public parent retry; the earlier real answer remains evidence.
func TestPublicDialogueExpiredJobsRecoverWithoutStranding(t *testing.T) {
	for _, kind := range []string{"queued_deadline", "running_deadline", "failed_deadline", "running_lease_budget", "queued_budget"} {
		t.Run(kind, func(t *testing.T) {
			h := newPublicDialogueHarness(t)
			h.begin()
			conn := h.socket(0)
			q := h.question(conn, 0)
			h.answer(conn, q, "real-answer-before-expiry", "The family repaired the garden wall after the storm.")
			h.question(conn, 1)
			state := h.state()
			originalAttempt := h.attempt
			conn.CloseNow()
			h.stop(false)
			ctx := context.Background()
			message, job := uuid.NewString(), uuid.NewString()
			var session string
			if err := h.pool.QueryRow(ctx, `SELECT session_id FROM verification_jobs WHERE attempt_id=$1 AND job_key='initial-question'`, h.attempt).Scan(&session); err != nil {
				t.Fatal(err)
			}
			tx, err := h.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, `INSERT INTO verification_messages(id,tenant_id,attempt_id,question_id,policy_version,snapshot_digest,sequence,expected_version,role,content,client_message_id) VALUES($1,$2,$3,$4,$5,$6,2,$7,'student','Expiry precondition answer, not a scored result.','expiry-precondition')`, message, tenantA, h.attempt, state.QuestionID, state.PolicyVersion, state.SnapshotDigest, state.Version); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `UPDATE dialogue_attempts SET version=version+1,next_sequence=3 WHERE tenant_id=$1 AND attempt_id=$2`, tenantA, h.attempt); err != nil {
				t.Fatal(err)
			}
			status := "queued"
			calls := 1
			generation := int64(1)
			var owner any
			var lease any
			deadline := time.Now().UTC().Add(-time.Second)
			if kind == "running_deadline" || kind == "running_lease_budget" {
				status = "running"
				owner = "expired-worker"
				lease = time.Now().UTC().Add(10 * time.Second)
			}
			if kind == "failed_deadline" {
				status = "failed"
			}
			if kind == "running_lease_budget" || kind == "queued_budget" {
				calls = 3
				generation = 3
				deadline = time.Now().UTC().Add(time.Minute)
				if status == "running" {
					lease = time.Now().UTC().Add(-time.Second)
				}
			}
			if _, err = tx.Exec(ctx, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,message_id,session_id,job_key,status,stage,attempts,max_attempts,lease_generation,lease_owner,lease_until,deadline) VALUES($1,$2,$3,$4,$5,'expiry-precondition',$6,'evaluation',$7,3,$8,$9,$10,$11)`, job, tenantA, h.attempt, message, session, status, calls, generation, owner, lease, deadline); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			h.start()
			conn = h.socket(0)
			terminal := h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "error" && e.Status == "exhausted" })
			if terminal.OccurrenceStatus != "pending" || terminal.DecisionID == "" {
				t.Fatal("expiry lacked one parent-retryable durable outcome")
			}
			if kind == "running_lease_budget" && terminal.Code != "lease_exhausted" {
				t.Fatal("lease exhaustion reason is not truthful")
			}
			if kind != "running_lease_budget" && kind != "queued_budget" && terminal.Code != "deadline_exhausted" {
				t.Fatal("deadline exhaustion reason is not truthful")
			}
			var count int
			if err = h.pool.QueryRow(ctx, `SELECT count(*) FROM verification_decisions WHERE attempt_id=$1 AND NOT accepted`, h.attempt).Scan(&count); err != nil || count != 1 {
				t.Fatal("terminal decision missing or duplicated")
			}
			conn.CloseNow()
			h.stop(false)
			h.start()
			conn = h.socket(0)
			h.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "error" && e.Status == "exhausted" })
			h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/retry", nil, 200, nil)
			h.begin()
			if h.attempt == originalAttempt {
				t.Fatal("expired attempt was reused")
			}
			conn.CloseNow()
			conn = h.socket(0)
			h.question(conn, 0)
			// Supplemental stale-writer conformance against the production engine:
			// the original expired generation cannot mutate the new attempt/outcome.
			engine := verification.DialogueEngine{DB: h.pool}
			err = engine.CommitEvaluation(ctx, verification.StudentAuthority{TenantID: tenantA, StudentID: h.studentID, SessionID: session}, h.occurrence, originalAttempt, verification.DialogueLease{JobID: job, Owner: "expired-worker", Generation: generation}, verification.DialogueEvaluation{Accepted: true})
			if !errors.Is(err, verification.ErrDialogueLease) {
				t.Fatal("late expired generation passed the write fence")
			}
			var messages, evaluations, accepted, terminalEvents int
			if err = h.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM verification_messages WHERE attempt_id=$1),(SELECT count(*) FROM verification_evaluations WHERE attempt_id=$1),(SELECT count(*) FROM verification_evaluations WHERE attempt_id=$1 AND accepted),(SELECT count(*) FROM verification_events WHERE attempt_id=$1 AND event_key='job-exhaustion')`, originalAttempt).Scan(&messages, &evaluations, &accepted, &terminalEvents); err != nil || messages != 2 || evaluations != 1 || accepted != 1 || terminalEvents != 1 {
				t.Fatal("recovery/replay/late writer changed preserved evidence")
			}
		})
	}
}

func TestPublicMixedRequirementRetrySelectsActualFailedAttempt(t *testing.T) {
	for _, manualFirst := range []bool{false, true} {
		for _, ambiguous := range []bool{false, true} {
			t.Run(fmt.Sprintf("manual-first-%t-ambiguous-%t", manualFirst, ambiguous), func(t *testing.T) {
				h := newPublicDialogueHarnessWithPolicy(t, 1, 6, true)
				if manualFirst {
					requirements := append([]Requirement(nil), h.task.Requirements...)
					requirements[0], requirements[1] = requirements[1], requirements[0]
					var revised TaskRevision
					h.request(h.parent, "POST", "/tasks/"+h.task.TemplateID+"/revisions", TaskInput2{Title: "Reordered retry task", Instructions: "Read and request the separate checks", Requirements: requirements}, 201, &revised)
					h.request(h.parent, "POST", "/tasks/"+revised.ID+"/publish", nil, 200, nil)
					h.task = revised
					h.followingOccurrence()
				}
				h.begin()
				conn := h.socket(0)
				h.question(conn, 0)
				var inspect DialogueInspect
				h.request(h.parent, "GET", "/occurrences/"+h.occurrence+"/inspect", nil, 200, &inspect)
				var manual, dialogue DialogueAttemptInfo
				for _, attempt := range inspect.Attempts {
					if attempt.Kind == "parent_approval" {
						manual = attempt
					} else if attempt.Kind == "agent_dialogue" {
						dialogue = attempt
					}
				}
				if manual.ID == "" || dialogue.ID == "" || manual.Number != 1 || dialogue.Number != 1 {
					t.Fatal("public tied requirement attempts unavailable")
				}
				h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/decision", DecisionInput2{RequirementID: manual.RequirementID, Accepted: false, Reason: "The separate manual check needs another attempt."}, 200, nil)
				if ambiguous {
					state := h.state()
					h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/override", DialogueOverrideRequest{AttemptID: dialogue.ID, ClientRequestID: "reject-for-specific-retry", ExpectedVersion: state.Version, Accepted: false, Reason: "Parent requests a new dialogue attempt without changing history."}, 200, nil)
					h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/retry", nil, 409, nil)
					h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/retry?requirementId="+uuid.NewString(), nil, 404, nil)
					h.request(h.parent, "POST", "/occurrences/"+h.occurrence+"/retry?requirementId="+manual.RequirementID+"&attemptId="+dialogue.ID, nil, 404, nil)
				}
				path := "/occurrences/" + h.occurrence + "/retry"
				if ambiguous {
					path += "?requirementId=" + manual.RequirementID + "&attemptId=" + manual.ID
				}
				var receipt struct {
					RequirementID     string `json:"requirementId"`
					PreviousAttemptID string `json:"previousAttemptId"`
					AttemptNumber     int    `json:"attemptNumber"`
				}
				h.request(h.parent, "POST", path, nil, 200, &receipt)
				if receipt.RequirementID != manual.RequirementID || receipt.PreviousAttemptID != manual.ID || receipt.AttemptNumber != 2 {
					t.Fatal("retry guessed another requirement from global number/ordinal")
				}
				if ambiguous {
					path = "/occurrences/" + h.occurrence + "/retry?requirementId=" + dialogue.RequirementID + "&attemptId=" + dialogue.ID
					h.request(h.parent, "POST", path, nil, 200, &receipt)
					if receipt.RequirementID != dialogue.RequirementID || receipt.AttemptNumber != 2 {
						t.Fatal("explicit dialogue retry selected another attempt")
					}
					h.request(h.parent, "POST", path, nil, 404, nil)
				}
				var manualCount, dialogueCount int
				if err := h.pool.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE requirement_id=$2),count(*) FILTER(WHERE requirement_id=$3) FROM verification_attempts WHERE occurrence_id=$1`, h.occurrence, manual.RequirementID, dialogue.RequirementID).Scan(&manualCount, &dialogueCount); err != nil || manualCount != 2 || dialogueCount != 1+map[bool]int{true: 1, false: 0}[ambiguous] {
					t.Fatal("retry changed an unselected requirement or duplicated work")
				}
			})
		}
	}
}

func TestPublicDialogueCancellationFencesLateEvaluation(t *testing.T) {
	h := newPublicDialogueHarness(t)
	h.stop(false)
	h.delay = "800"
	h.start()
	h.begin()
	conn := h.socket(0)
	q := h.question(conn, 0)
	h.answer(conn, q, "saved-before-cancel", "The family repaired the garden wall after the storm.")
	waitPublicDialogueRunning(t, h)
	occurrence, attempt := h.occurrence, h.attempt
	h.request(h.parent, "POST", "/occurrences/"+occurrence+"/cancel", nil, 200, nil)
	state := h.state()
	if state.OccurrenceStatus != "canceled" {
		t.Fatal("student did not observe actual cancellation")
	}
	conn.CloseNow()
	h.followingOccurrence()
	h.begin()
	following := h.socket(0)
	h.question(following, 0)
	var messages, evaluations, decisions, completions int
	if err := h.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM verification_messages WHERE attempt_id=$1),(SELECT count(*) FROM verification_evaluations WHERE attempt_id=$1),(SELECT count(*) FROM verification_decisions WHERE attempt_id=$1),(SELECT count(*) FROM verification_events WHERE attempt_id=$1 AND kind='complete')`, attempt).Scan(&messages, &evaluations, &decisions, &completions); err != nil || messages != 1 || evaluations != 0 || decisions != 0 || completions != 0 {
		t.Fatal("canceled worker accepted late evidence or lost the saved answer")
	}
	assertTerminalDialogueIgnoresExpiredQueue(t, h, occurrence, attempt)
}
