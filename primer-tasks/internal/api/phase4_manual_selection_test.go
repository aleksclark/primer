package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Reuse the single real process/PG/public-pairing fixture from the original RED.
// All success state below comes from public mutations, never injected decisions.
func exercisePublicManualSelection(h *publicDialogueHarness) {
	t := h.t
	create := func(parent *http.Client, student string, requirements []Requirement) Occurrence2 {
		var task TaskRevision
		h.request(parent, "POST", "/tasks", TaskInput2{Title: "Selected manual work", Instructions: "Finish the selected work", Requirements: requirements}, 201, &task)
		h.request(parent, "POST", "/tasks/"+task.ID+"/publish", nil, 200, nil)
		var schedule Schedule2
		h.request(parent, "POST", "/schedules", ScheduleInput2{StudentID: student, TemplateID: task.TemplateID, RevisionID: task.ID, Kind: "one_off", Timezone: "UTC", StartAt: time.Now().UTC().Add(-time.Minute)}, 201, &schedule)
		var page OccurrencePage2
		h.request(parent, "GET", "/occurrences?limit=100", nil, 200, &page)
		for _, o := range page.Items {
			if o.ScheduleID == schedule.ID {
				return o
			}
		}
		t.Fatal("public selected-work materialization missing")
		return Occurrence2{}
	}
	read := func(o string) Occurrence2 {
		var current Occurrence2
		h.request(h.student, "GET", "/student/occurrences/"+o, nil, 200, &current)
		return current
	}
	post := func(o, requirement, action string, status int) {
		suffix := ""
		if requirement != "" {
			suffix = "?requirementId=" + requirement
		}
		h.request(h.student, "POST", "/student/occurrences/"+o+"/"+action+suffix, nil, status, nil)
	}
	observe := func(o, requirement string, selected, other int) {
		var actualSelected, actualOther, privateRows int
		err := h.pool.QueryRow(context.Background(), `SELECT
   (SELECT count(*) FROM verification_attempts WHERE occurrence_id=$1 AND requirement_id=$2),
   (SELECT count(*) FROM verification_attempts WHERE occurrence_id=$1 AND requirement_id<>$2),
   (SELECT count(*) FROM dialogue_attempts WHERE occurrence_id=$1)
    +(SELECT count(*) FROM dialogue_questions q JOIN verification_attempts a ON a.tenant_id=q.tenant_id AND a.id=q.attempt_id WHERE a.occurrence_id=$1)
    +(SELECT count(*) FROM verification_jobs j JOIN verification_attempts a ON a.tenant_id=j.tenant_id AND a.id=j.attempt_id WHERE a.occurrence_id=$1)
    +(SELECT count(*) FROM verification_messages m JOIN verification_attempts a ON a.tenant_id=m.tenant_id AND a.id=m.attempt_id WHERE a.occurrence_id=$1)
    +(SELECT count(*) FROM verification_evaluations e JOIN verification_attempts a ON a.tenant_id=e.tenant_id AND a.id=e.attempt_id WHERE a.occurrence_id=$1)
    +(SELECT count(*) FROM verification_events e JOIN verification_attempts a ON a.tenant_id=e.tenant_id AND a.id=e.attempt_id WHERE a.occurrence_id=$1)`, o, requirement).Scan(&actualSelected, &actualOther, &privateRows)
		if err != nil {
			t.Fatal(err)
		}
		if actualSelected != selected || actualOther != other || privateRows != 0 {
			t.Fatalf("manual selection observation=(%d,%d,%d), want (%d,%d,0)", actualSelected, actualOther, privateRows, selected, other)
		}
	}
	parallelSubmit := func(o, requirement string) {
		const count = 6
		start := make(chan struct{})
		failures := make(chan error, count)
		var wg sync.WaitGroup
		csrf := h.csrf()
		for range count {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				request, err := http.NewRequest("POST", h.base+"/student/occurrences/"+o+"/submit?requirementId="+requirement, nil)
				if err != nil {
					failures <- err
					return
				}
				request.Header.Set("Origin", h.base)
				request.Header.Set("X-CSRF-Token", csrf)
				response, err := h.student.Do(request)
				if err != nil {
					failures <- err
					return
				}
				response.Body.Close()
				if response.StatusCode != 200 {
					failures <- fmt.Errorf("concurrent selected submit status %d", response.StatusCode)
				}
			}()
		}
		close(start)
		wg.Wait()
		close(failures)
		for err := range failures {
			t.Error(err)
		}
	}
	current := read(h.occurrence)
	manual := current.Verification[1].ID
	dialogue := current.Verification[0].ID
	parallelSubmit(current.ID, manual)
	observe(current.ID, manual, 1, 0)
	h.request(h.parent, "POST", "/occurrences/"+current.ID+"/decision", DecisionInput2{RequirementID: manual, Accepted: true, Reason: "Parent checked the selected manual work."}, 200, nil)
	accepted := read(current.ID)
	if accepted.Status != "awaiting_verification" || accepted.Verification[1].AttemptStatus != "accepted" || accepted.Verification[0].AttemptID != "" {
		t.Fatal("manual decision completed or initialized other required work")
	}
	for _, action := range []string{"start", "submit"} {
		post(current.ID, manual, action, 200)   // accepted selected requirement stays accepted
		post(current.ID, dialogue, action, 409) // validated BEFORE any idempotent 200
		post(current.ID, uuid.NewString(), action, 404)
		post(current.ID, "", action, 409) // mixed means explicit, even with one manual
		for _, query := range []string{"requirementId=", "requirementId=bad", "requirementId=" + manual + "&requirementId=" + dialogue, "unknown=" + manual} {
			h.request(h.student, "POST", "/student/occurrences/"+current.ID+"/"+action+"?"+query, nil, 400, nil)
		}
	}
	observe(current.ID, manual, 1, 0)
	// Reverse order, and another published revision of the same student.
	reversed := create(h.parent, h.studentID, []Requirement{h.task.Requirements[1], h.task.Requirements[0]})
	reverseManual := reversed.Verification[0].ID
	post(reversed.ID, manual, "start", 404)
	post(reversed.ID, reverseManual, "submit", 409)
	post(reversed.ID, reverseManual, "start", 200)
	post(reversed.ID, reverseManual, "start", 200)
	parallelSubmit(reversed.ID, reverseManual)
	observe(reversed.ID, reverseManual, 1, 0)
	h.request(h.parent, "POST", "/occurrences/"+reversed.ID+"/decision", DecisionInput2{RequirementID: reverseManual, Accepted: true, Reason: "Parent checked reverse-order manual work."}, 200, nil)
	if next := read(reversed.ID); next.Status != "awaiting_verification" || next.Verification[1].AttemptID != "" {
		t.Fatal("reverse-order manual work retargeted dialogue")
	}
	// Two supported manual requirements prove that an already-awaiting occurrence
	// must create the *missing selected* attempt, not return a global early 200.
	two := create(h.parent, h.studentID, []Requirement{h.task.Requirements[1], h.task.Requirements[1]})
	first, second := two.Verification[0].ID, two.Verification[1].ID
	post(two.ID, "", "start", 409)
	post(two.ID, first, "start", 200)
	post(two.ID, first, "submit", 200)
	h.request(h.parent, "POST", "/occurrences/"+two.ID+"/decision", DecisionInput2{RequirementID: first, Accepted: true, Reason: "First manual requirement observed."}, 200, nil)
	post(two.ID, second, "start", 200)
	parallelSubmit(two.ID, second)
	var numbers []int
	rows, err := h.pool.Query(context.Background(), `SELECT number FROM verification_attempts WHERE occurrence_id=$1 ORDER BY requirement_id`, two.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n int
		if err = rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(numbers) != 2 || numbers[0] != 1 || numbers[1] != 1 {
		t.Fatalf("attempt numbers are not requirement-scoped: %v", numbers)
	}
	post(two.ID, first, "submit", 200)
	h.request(h.parent, "POST", "/occurrences/"+two.ID+"/decision", DecisionInput2{RequirementID: second, Accepted: true, Reason: "Second manual requirement observed."}, 200, nil)
	if read(two.ID).Status != "completed" {
		t.Fatal("all-manual completion lost")
	}
	for _, action := range []string{"start", "submit"} {
		post(two.ID, first, action, 409)
		post(two.ID, uuid.NewString(), action, 404)
	}
	// Normal single-manual calls without selection remain valid and idempotent.
	single := create(h.parent, h.studentID, []Requirement{h.task.Requirements[1]})
	post(single.ID, "", "start", 200)
	post(single.ID, "", "start", 200)
	post(single.ID, "", "submit", 200)
	post(single.ID, "", "submit", 200)
	observe(single.ID, single.Verification[0].ID, 1, 0)
	h.request(h.parent, "POST", "/occurrences/"+single.ID+"/decision", DecisionInput2{Accepted: false, Reason: "Redo this manual work carefully."}, 200, nil)
	post(single.ID, "", "start", 200)
	post(single.ID, "", "submit", 200)
	observe(single.ID, single.Verification[0].ID, 2, 0)
	// Unsupported interaction is possible through the legacy manual config, but
	// it is not a supported student action. No DB constraint is disabled.
	unsupported := h.task.Requirements[1]
	unsupported.Interaction = "chat"
	wrong := create(h.parent, h.studentID, []Requirement{unsupported})
	post(wrong.ID, wrong.Verification[0].ID, "start", 409)
	post(wrong.ID, wrong.Verification[0].ID, "submit", 409)
	observe(wrong.ID, wrong.Verification[0].ID, 0, 0)
	unknown := h.task.Requirements[1]
	unknown.Kind = "unknown"
	h.request(h.parent, "POST", "/tasks", TaskInput2{Title: "Unsupported kind", Requirements: []Requirement{unknown}}, 400, nil)
	// Real other student and tenant records, not guessed IDs.
	var other Student
	h.request(h.parent, "POST", "/students", map[string]string{"displayName": "Other student"}, 201, &other)
	otherWork := create(h.parent, other.ID, []Requirement{h.task.Requirements[1]})
	parentJar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(h.base)
	parentJar.SetCookies(baseURL, []*http.Cookie{{Name: "tasks_parent", Value: "parent-b", Path: "/"}})
	foreignParent := &http.Client{Jar: parentJar, Timeout: 5 * time.Second}
	var foreign Student
	h.request(foreignParent, "POST", "/students", map[string]string{"displayName": "Foreign household student"}, 201, &foreign)
	foreignWork := create(foreignParent, foreign.ID, []Requirement{h.task.Requirements[1]})
	for _, action := range []string{"start", "submit"} {
		post(otherWork.ID, otherWork.Verification[0].ID, action, 404)
		post(current.ID, foreignWork.Verification[0].ID, action, 404)
		post(foreignWork.ID, foreignWork.Verification[0].ID, action, 404)
	}
	for _, status := range []string{"cancel", "skip"} {
		closed := create(h.parent, h.studentID, []Requirement{h.task.Requirements[1]})
		h.request(h.parent, "POST", "/occurrences/"+closed.ID+"/"+status, nil, 200, nil)
		for _, action := range []string{"start", "submit"} {
			post(closed.ID, closed.Verification[0].ID, action, 409)
		}
		observe(closed.ID, closed.Verification[0].ID, 0, 0)
	}
	// New selected commands preserve cookie-only custody and require CSRF/Origin.
	for _, mode := range []string{"no-csrf", "foreign-origin", "bearer", "csrf-only"} {
		request, _ := http.NewRequest("POST", h.base+"/student/occurrences/"+current.ID+"/submit?requirementId="+manual, nil)
		request.Header.Set("Origin", h.base)
		request.Header.Set("X-CSRF-Token", h.csrf())
		httpClient := h.student
		want := 403
		switch mode {
		case "no-csrf":
			request.Header.Del("X-CSRF-Token")
		case "foreign-origin":
			request.Header.Set("Origin", "https://foreign.invalid")
		case "bearer":
			request.Header.Set("Authorization", "Bearer forbidden")
			want = 401
		case "csrf-only":
			httpClient = &http.Client{Timeout: 5 * time.Second}
			want = 401
		}
		response, err := httpClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("%s status=%d want %d", mode, response.StatusCode, want)
		}
	}
	// Negative expiry precondition only: shorten the owned session, hold occurrence
	// progress, observe the real HTTP request waiting, then let the session expire.
	// No success/evidence row is injected; immutable production guards stay enabled.
	expiring := create(h.parent, h.studentID, []Requirement{h.task.Requirements[1]})
	if _, err = h.pool.Exec(context.Background(), `UPDATE student_sessions SET expires_at=clock_timestamp()+interval '1200 milliseconds' WHERE student_id=$1`, h.studentID); err != nil {
		t.Fatal(err)
	}
	tx, err := h.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), `SELECT id FROM task_occurrences WHERE id=$1 FOR UPDATE`, expiring.ID); err != nil {
		t.Fatal(err)
	}
	responseStatus := make(chan int, 1)
	go func() {
		request, _ := http.NewRequest("POST", h.base+"/student/occurrences/"+expiring.ID+"/start?requirementId="+expiring.Verification[0].ID, nil)
		request.Header.Set("Origin", h.base)
		request.Header.Set("X-CSRF-Token", h.csrf())
		response, e := h.student.Do(request)
		if e != nil {
			responseStatus <- 0
			return
		}
		response.Body.Close()
		responseStatus <- response.StatusCode
	}()
	waiting := false
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		var found bool
		err = h.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT revision_id,status FROM task_occurrences%')`).Scan(&found)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			waiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("selected manual request never reached the occurrence lock")
	}
	if _, err = h.pool.Exec(context.Background(), `SELECT pg_sleep(1.3)`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := <-responseStatus; status != 401 {
		t.Fatalf("expired while blocked status=%d want 401", status)
	}
	observe(expiring.ID, expiring.Verification[0].ID, 0, 0)
	// Revocation has its own fresh, unexpired public pairing.
	jar, _ := cookiejar.New(nil)
	otherBrowser := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	var pairing struct {
		Code string `json:"code"`
	}
	h.request(h.parent, "POST", "/students/"+other.ID+"/pairing", nil, 200, &pairing)
	h.request(otherBrowser, "POST", "/student/pair", map[string]string{"code": pairing.Code}, 200, nil)
	h.request(h.parent, "DELETE", "/students/"+other.ID, nil, 204, nil)
	local := *h
	local.student = otherBrowser
	local.request(otherBrowser, "POST", "/student/occurrences/"+otherWork.ID+"/start?requirementId="+otherWork.Verification[0].ID, nil, 401, nil)
	observe(otherWork.ID, otherWork.Verification[0].ID, 0, 0)
}
