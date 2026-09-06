package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"testing"
	"time"

	"primer-tasks/internal/verification"
)

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
}
