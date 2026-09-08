package api

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// Native uses the real generated-client wire route (/device, Bearer), not a
// cookie or dialogue fallback. Public setup and transitions; SQL only observes.
func TestPublicNativeManualRequirementCompatibility(t *testing.T) {
	h := newPublicDialogueHarnessWithPolicy(t, 2, 9, true)
	var pairing struct {
		Code string `json:"code"`
	}
	h.request(h.parent, "POST", "/students/"+h.studentID+"/pairing", nil, 200, &pairing)
	var paired struct {
		Token string `json:"token"`
	}
	h.request(&http.Client{}, "POST", "/device/pair", map[string]string{"code": pairing.Code}, 200, &paired)
	native := &http.Client{Transport: manualBearerTransport{token: paired.Token}, Timeout: 5 * time.Second}
	if paired.Token == "" {
		t.Fatal("public device pairing omitted token")
	}
	create := func(requirements []Requirement) Occurrence2 {
		var task TaskRevision
		h.request(h.parent, "POST", "/tasks", TaskInput2{Title: "Native manual compatibility", Requirements: requirements}, 201, &task)
		h.request(h.parent, "POST", "/tasks/"+task.ID+"/publish", nil, 200, nil)
		var schedule Schedule2
		h.request(h.parent, "POST", "/schedules", ScheduleInput2{StudentID: h.studentID, TemplateID: task.TemplateID, RevisionID: task.ID, Kind: "one_off", Timezone: "UTC", StartAt: time.Now().UTC().Add(-time.Minute)}, 201, &schedule)
		var page OccurrencePage2
		h.request(h.parent, "GET", "/occurrences?limit=100", nil, 200, &page)
		for _, o := range page.Items {
			if o.ScheduleID == schedule.ID {
				return o
			}
		}
		t.Fatal("public materialization missing")
		return Occurrence2{}
	}
	manual := create([]Requirement{h.task.Requirements[1]})
	var detail Occurrence2
	h.request(native, "GET", "/device/occurrences/"+manual.ID, nil, 200, &detail)
	if detail.StudentCapability != "parent_approval" || len(detail.Requirements) != 1 || len(detail.Verification) != 1 || detail.Requirements[0].ID != detail.Verification[0].ID {
		t.Fatal("additive capability/binding lost")
	}
	mutate := func(client *http.Client, route, id, action, suffix string, status int) {
		h.request(client, "POST", route+"/occurrences/"+id+"/"+action+suffix, nil, status, nil)
	}
	for _, action := range []string{"start", "submit"} {
		// Browser succeeds first; native must reuse its selected attempt rather
		// than create a duplicate or drop the typed id/status response.
		mutate(h.student, "/student", manual.ID, action, "?requirementId="+detail.Requirements[0].ID, 200)
		for range 2 {
			var response OccurrenceAction2
			h.request(native, "POST", "/device/occurrences/"+manual.ID+"/"+action, nil, 200, &response)
			if response.ID != manual.ID || response.Status == "" {
				t.Fatal("native action contract lost")
			}
		}
		mutate(native, "/student", manual.ID, action, "", 401)
		mutate(h.student, "/device", manual.ID, action, "", 401)
		mutate(native, "/device", manual.ID, action, "?requirementId="+detail.Requirements[0].ID, 400)
	}
	var count int
	if err := h.pool.QueryRow(context.Background(), `SELECT count(*) FROM verification_attempts WHERE occurrence_id=$1 AND requirement_id=$2`, manual.ID, detail.Requirements[0].ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("bound shared attempt count=%d err=%v", count, err)
	}
	h.request(h.parent, "POST", "/occurrences/"+manual.ID+"/decision", DecisionInput2{Accepted: false, Reason: "Redo carefully"}, 200, nil)
	for _, action := range []string{"start", "submit"} {
		mutate(native, "/device", manual.ID, action, "", 200)
	}
	h.request(h.parent, "POST", "/occurrences/"+manual.ID+"/decision", DecisionInput2{Accepted: true, Reason: "Parent checked native resubmission"}, 200, nil)
	h.request(native, "GET", "/device/occurrences/"+manual.ID, nil, 200, &detail)
	if detail.Status != "completed" || detail.AttemptNumber != 2 || detail.StudentCapability != "parent_approval" {
		t.Fatal("native reject/resubmit/approval regressed")
	}
	for _, action := range []string{"start", "submit"} {
		mutate(native, "/device", manual.ID, action, "", 409)
	}
	// Mixed work must stay unsupported even after browser progress would have
	// triggered the old occurrence-level idempotent 200.
	var mixed Occurrence2
	h.request(native, "GET", "/device/occurrences/"+h.occurrence, nil, 200, &mixed)
	if mixed.StudentCapability != "unsupported" || len(mixed.Verification) != 2 {
		t.Fatal("mixed native capability changed")
	}
	for _, action := range []string{"start", "submit"} {
		mutate(native, "/device", mixed.ID, action, "", 409)
		mutate(h.student, "/student", mixed.ID, action, "?requirementId="+mixed.Verification[1].ID, 200)
		mutate(native, "/device", mixed.ID, action, "", 409)
	}
	// Manual kind alone is not enough: unsupported envelopes cannot inherit
	// the native parent_approval capability or browser manual authority.
	for _, change := range []func(*Requirement){func(r *Requirement) { r.Executor = "other" }, func(r *Requirement) { r.Interaction = "chat" }} {
		req := h.task.Requirements[1]
		change(&req)
		o := create([]Requirement{req})
		for _, action := range []string{"start", "submit"} {
			mutate(native, "/device", o.ID, action, "", 409)
			mutate(h.student, "/student", o.ID, action, "?requirementId="+o.Verification[0].ID, 409)
		}
	}
	invalidVersion := h.task.Requirements[1]
	invalidVersion.ConfigVersion = 2
	h.request(h.parent, "POST", "/tasks", TaskInput2{Title: "Invalid manual version", Requirements: []Requirement{invalidVersion}}, 400, nil)
	// Omitting selection on legacy single-manual browser calls never omits
	// Origin/CSRF. Each rejected request leaves pending work untouched.
	unstarted := create([]Requirement{h.task.Requirements[1]})
	for _, suffix := range []string{"", "?requirementId=" + unstarted.Verification[0].ID} {
		for _, mode := range []string{"missing-csrf", "missing-origin", "foreign-origin", "wrong-csrf"} {
			r, _ := http.NewRequest("POST", h.base+"/student/occurrences/"+unstarted.ID+"/start"+suffix, nil)
			r.Header.Set("Origin", h.base)
			r.Header.Set("X-CSRF-Token", h.csrf())
			switch mode {
			case "missing-csrf":
				r.Header.Del("X-CSRF-Token")
			case "missing-origin":
				r.Header.Del("Origin")
			case "foreign-origin":
				r.Header.Set("Origin", "https://foreign.invalid")
			case "wrong-csrf":
				r.Header.Set("X-CSRF-Token", "wrong")
			}
			response, err := h.student.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 403 {
				t.Fatalf("%s status=%d", mode, response.StatusCode)
			}
		}
	}
	for _, query := range []string{"requirementId=%ZZ", "requirementId=" + unstarted.Verification[0].ID + "&requirementId=%ZZ", "requirementId=" + unstarted.Verification[0].ID + ";ignored=x", "unknown=%ZZ"} {
		h.request(h.student, "POST", "/student/occurrences/"+unstarted.ID+"/start?"+query, nil, 400, nil)
	}
	h.request(native, "GET", "/device/occurrences/"+unstarted.ID, nil, 200, &detail)
	if detail.Status != "pending" || detail.AttemptNumber != 0 {
		t.Fatal("denied cookie action mutated work")
	}
	h.request(h.parent, "DELETE", "/students/"+h.studentID, nil, 204, nil)
	mutate(native, "/device", mixed.ID, "submit", "", 401)
}

type manualBearerTransport struct{ token string }

func (b manualBearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}
