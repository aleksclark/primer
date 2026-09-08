package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"primer-tasks/internal/schedule"
)

func TestPhase2ParentApprovalPublicBoundary(t *testing.T) {
	pool := integrationPool(t)
	alice, bob := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2"), IssuerSecret: []byte("p2")})
	h := s.Routes()
	rec := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Brush your teeth","instructions":"Use the timer","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	if rec.Code != 201 {
		t.Fatalf("create task=%d %s", rec.Code, rec.Body.String())
	}
	var task struct {
		ID         string `json:"id"`
		RevisionID string `json:"revisionId"`
		TemplateID string `json:"templateId"`
	}
	if e := json.Unmarshal(rec.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if task.RevisionID == "" {
		task.RevisionID = task.ID
	}
	if rec = requestJSON(t, h, http.MethodPost, "/tasks/"+task.RevisionID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.RevisionID + `","kind":"one_off","timezone":"America/New_York","startAt":"` + start + `","dueOffsetMinutes":0}`
	if rec = requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	worker := schedule.NewWorker(pool)
	if err := worker.Materialize(context.Background()); err != nil {
		t.Fatalf("worker materialize=%v", err)
	}
	if err := worker.Materialize(context.Background()); err != nil {
		t.Fatalf("worker retry=%v", err)
	}
	worker2 := schedule.NewWorker(pool)
	if err := worker2.Materialize(context.Background()); err != nil {
		t.Fatalf("lease contention should skip, got %v", err)
	}
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	cancelWorker()
	worker.Run(workerCtx)
	list := requestJSON(t, h, http.MethodGet, "/occurrences?limit=20&sort=nominalAt&dir=asc", "parent-a", "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), "Brush your teeth") {
		t.Fatalf("occurrences=%d %s", list.Code, list.Body.String())
	}
	var page OccurrencePage2
	if e := json.Unmarshal(list.Body.Bytes(), &page); e != nil || len(page.Items) != 1 {
		t.Fatalf("page=%s err=%v", list.Body.String(), e)
	}
	occ := page.Items[0]
	pair := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	code, _ := pairingResponse(t, pair)
	device := requestJSON(t, h, http.MethodPost, "/device/pair", "", `{"code":"`+code+`"}`)
	var dv struct {
		Token string `json:"token"`
	}
	if e := json.Unmarshal(device.Body.Bytes(), &dv); e != nil {
		t.Fatal(e)
	}
	if rec = requestBearer(t, h, http.MethodGet, "/device/today", dv.Token); rec.Code != 200 {
		t.Fatalf("device today=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodGet, "/device/occurrences/"+occ.ID, dv.Token); rec.Code != 200 {
		t.Fatalf("device detail=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/start", dv.Token); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"in_progress"`) {
		t.Fatalf("student start=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/decision", "parent-a", `{"accepted":true,"reason":"too early"}`); rec.Code != 409 {
		t.Fatalf("approve before submit=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/submit", dv.Token); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"awaiting_verification"`) {
		t.Fatalf("student submit=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/submit", dv.Token); rec.Code != 200 {
		t.Fatalf("submit replay=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodGet, "/device/occurrences/"+bob, dv.Token); rec.Code != 404 {
		t.Fatalf("foreign device detail=%d", rec.Code)
	}
	if rec = requestBearer(t, h, http.MethodPost, "/device/occurrences/"+bob+"/start", dv.Token); rec.Code != 404 {
		t.Fatalf("foreign device start=%d", rec.Code)
	}
	if rec = requestJSON(t, h, http.MethodPost, "/occurrences/"+bob+"/decision", "parent-a", `{"accepted":true,"reason":"no"}`); rec.Code != 404 {
		t.Fatalf("foreign decision=%d", rec.Code)
	}
	if rec = requestJSON(t, h, http.MethodPost, "/tasks/"+bob+"/revisions", "parent-a", `{"title":"x","instructions":"x","requirements":[{"id":"r","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`); rec.Code != 404 {
		t.Fatalf("foreign revision=%d", rec.Code)
	}
	if rec = requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/decision", "parent-a", `{"accepted":false,"reason":"try again"}`); rec.Code != 200 {
		t.Fatalf("reject=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/retry", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("retry after reject=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/decision", "parent-a", `{"accepted":true,"reason":"observed"}`); rec.Code != 200 {
		t.Fatalf("approve=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestJSON(t, h, http.MethodGet, "/occurrences/"+occ.ID, "parent-a", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"completed"`) {
		t.Fatalf("completed=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/decision", "parent-a", `{"accepted":true,"reason":"replay"}`); rec.Code != 200 {
		t.Fatalf("decision replay=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/start", dv.Token); rec.Code != 409 {
		t.Fatalf("completed device start=%d", rec.Code)
	}
	if rec = requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/submit", dv.Token); rec.Code != 409 {
		t.Fatalf("completed device submit=%d", rec.Code)
	}
	if rec = requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/skip", "parent-a", ""); rec.Code != 409 {
		t.Fatalf("completed skip=%d", rec.Code)
	}
	if rec = requestJSON(t, h, http.MethodGet, "/occurrences", "parent-b", ""); rec.Code != 200 || strings.Contains(rec.Body.String(), occ.ID) {
		t.Fatalf("tenant leak=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodGet, "/device/occurrences/"+occ.ID, dv.Token); rec.Code != 200 {
		t.Fatalf("student own detail=%d", rec.Code)
	}
	_ = bob
}

func TestStudentOccurrenceMetadataAndUnsupportedSubmit(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-cap"), IssuerSecret: []byte("p2-cap")})
	h := s.Routes()
	rec := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Brush your teeth","instructions":"Use the timer","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	if rec.Code != 201 {
		t.Fatalf("create task=%d %s", rec.Code, rec.Body.String())
	}
	var task struct {
		ID         string `json:"id"`
		TemplateID string `json:"templateId"`
	}
	if e := json.Unmarshal(rec.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec = requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"one_off","timezone":"UTC","startAt":"` + start + `","dueOffsetMinutes":0}`
	if rec = requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	if err := schedule.NewWorker(pool).Materialize(context.Background()); err != nil {
		t.Fatalf("materialize=%v", err)
	}
	list := requestJSON(t, h, http.MethodGet, "/occurrences?limit=20", "parent-a", "")
	var page OccurrencePage2
	if e := json.Unmarshal(list.Body.Bytes(), &page); e != nil || len(page.Items) != 1 {
		t.Fatalf("page=%s err=%v", list.Body.String(), e)
	}
	occ := page.Items[0]
	if occ.StudentCapability != "parent_approval" || len(occ.Requirements) != 1 || occ.Requirements[0].Kind != "parent_approval" {
		t.Fatalf("parent list metadata=%+v", occ)
	}
	pair := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	code, _ := pairingResponse(t, pair)
	device := requestJSON(t, h, http.MethodPost, "/device/pair", "", `{"code":"`+code+`"}`)
	var dv struct {
		Token string `json:"token"`
	}
	if e := json.Unmarshal(device.Body.Bytes(), &dv); e != nil {
		t.Fatal(e)
	}
	if rec = requestBearer(t, h, http.MethodGet, "/device/today", dv.Token); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"studentCapability":"parent_approval"`) || strings.Contains(rec.Body.String(), `"sourceText"`) || strings.Contains(rec.Body.String(), `"rubric"`) {
		t.Fatalf("device today metadata=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodGet, "/device/occurrences/"+occ.ID, dv.Token); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"kind":"parent_approval"`) {
		t.Fatalf("device detail metadata=%d %s", rec.Code, rec.Body.String())
	}

	mixed := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Talk then show","instructions":"Dialogue plus parent check","requirements":[{"id":"dialogue","kind":"agent_dialogue","configVersion":1,"config":{"sourceText":"The pupil measured the beam twice before cutting it.","learningFocus":"Use three distinct facts and reasons","requiredQuestions":3,"rubric":["answers the question with a source detail"],"allowedFollowUps":1,"maxAttempts":2,"maxTurns":8,"retentionPolicy":"retain"},"interaction":"chat","executor":"fantasy"},{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	if mixed.Code != 201 {
		t.Fatalf("mixed create=%d %s", mixed.Code, mixed.Body.String())
	}
	var mixedTask struct {
		ID         string `json:"id"`
		TemplateID string `json:"templateId"`
	}
	if e := json.Unmarshal(mixed.Body.Bytes(), &mixedTask); e != nil {
		t.Fatal(e)
	}
	if rec = requestJSON(t, h, http.MethodPost, "/tasks/"+mixedTask.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("mixed publish=%d %s", rec.Code, rec.Body.String())
	}
	mixedBody := `{"studentId":"` + alice + `","templateId":"` + mixedTask.TemplateID + `","revisionId":"` + mixedTask.ID + `","kind":"one_off","timezone":"UTC","startAt":"` + start + `","dueOffsetMinutes":0}`
	if rec = requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", mixedBody); rec.Code != 201 {
		t.Fatalf("mixed schedule=%d %s", rec.Code, rec.Body.String())
	}
	if err := schedule.NewWorker(pool).Materialize(context.Background()); err != nil {
		t.Fatalf("mixed materialize=%v", err)
	}
	listed := requestJSON(t, h, http.MethodGet, "/occurrences?limit=20&sort=nominalAt&dir=asc", "parent-a", "")
	var listedPage OccurrencePage2
	if e := json.Unmarshal(listed.Body.Bytes(), &listedPage); e != nil {
		t.Fatal(e)
	}
	var mixedOcc Occurrence2
	for _, item := range listedPage.Items {
		if item.RevisionID == mixedTask.ID {
			mixedOcc = item
		}
	}
	if mixedOcc.ID == "" || mixedOcc.StudentCapability != "unsupported" || len(mixedOcc.Requirements) != 2 {
		t.Fatalf("mixed metadata=%+v", mixedOcc)
	}
	if rec = requestBearer(t, h, http.MethodGet, "/device/occurrences/"+mixedOcc.ID, dv.Token); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"studentCapability":"unsupported"`) || strings.Contains(rec.Body.String(), `"sourceText"`) || strings.Contains(rec.Body.String(), `"rubric"`) || strings.Contains(rec.Body.String(), `"learningFocus"`) {
		t.Fatalf("mixed device detail=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodPost, "/device/occurrences/"+mixedOcc.ID+"/start", dv.Token); rec.Code != 409 {
		t.Fatalf("mixed start=%d %s", rec.Code, rec.Body.String())
	}
	if rec = requestBearer(t, h, http.MethodPost, "/device/occurrences/"+mixedOcc.ID+"/submit", dv.Token); rec.Code != 409 {
		t.Fatalf("mixed submit=%d %s", rec.Code, rec.Body.String())
	}
}

func TestPhase2CRUDScheduleAndStudentReadPaths(t *testing.T) {
	pool := integrationPool(t)
	alice, bob := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2b"), IssuerSecret: []byte("p2b"), PublicOrigin: "https://example.com"})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Morning care","instructions":"Follow the steps","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	if create.Code != 201 {
		t.Fatalf("create=%d %s", create.Code, create.Body.String())
	}
	var task struct {
		ID         string `json:"id"`
		TemplateID string `json:"templateId"`
	}
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if list := requestJSON(t, h, http.MethodGet, "/tasks?q=Morning&sort=title&dir=asc&limit=1&offset=0", "parent-a", ""); list.Code != 200 {
		t.Fatalf("task list=%d %s", list.Code, list.Body.String())
	}
	revise := requestJSON(t, h, http.MethodPost, "/tasks/"+task.TemplateID+"/revisions", "parent-a", `{"title":"Morning care revised","instructions":"New steps","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	if revise.Code != 201 {
		t.Fatalf("revise=%d %s", revise.Code, revise.Body.String())
	}
	var revision struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(revise.Body.Bytes(), &revision)
	if publish := requestJSON(t, h, http.MethodPost, "/tasks/"+revision.ID+"/publish", "parent-a", ""); publish.Code != 200 {
		t.Fatalf("publish revision=%d %s", publish.Code, publish.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	scheduleBody := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + revision.ID + `","kind":"one_off","timezone":"UTC","startAt":"` + start + `","dueOffsetMinutes":0}`
	schedule := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", scheduleBody)
	if schedule.Code != 201 {
		t.Fatalf("schedule=%d %s", schedule.Code, schedule.Body.String())
	}
	var sch struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(schedule.Body.Bytes(), &sch)
	if got := requestJSON(t, h, http.MethodGet, "/schedules?limit=10&offset=0", "parent-a", ""); got.Code != 200 {
		t.Fatalf("schedule list=%d %s", got.Code, got.Body.String())
	}
	updateBody := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + revision.ID + `","kind":"recurrence","timezone":"UTC","startAt":"` + start + `","rrule":"FREQ=DAILY;COUNT=2","dueOffsetMinutes":5}`
	if got := requestJSON(t, h, http.MethodPatch, "/schedules/"+sch.ID, "parent-a", updateBody); got.Code != 200 {
		t.Fatalf("schedule update=%d %s", got.Code, got.Body.String())
	}
	badUpdate := strings.Replace(updateBody, revision.ID, bob, 1)
	if got := requestJSON(t, h, http.MethodPatch, "/schedules/"+sch.ID, "parent-a", badUpdate); got.Code != 409 {
		t.Fatalf("mismatched schedule revision=%d %s", got.Code, got.Body.String())
	}
	if got := requestJSON(t, h, http.MethodDelete, "/schedules/"+sch.ID, "parent-a", ""); got.Code != 204 {
		t.Fatalf("schedule retire=%d %s", got.Code, got.Body.String())
	}
	if got := requestJSON(t, h, http.MethodDelete, "/schedules/"+sch.ID, "parent-a", ""); got.Code != 404 {
		t.Fatalf("repeat schedule retire=%d", got.Code)
	}
	if got := requestJSON(t, h, http.MethodPatch, "/schedules/"+bob, "parent-a", updateBody); got.Code != 404 {
		t.Fatalf("missing schedule update=%d", got.Code)
	}
	occList := requestJSON(t, h, http.MethodGet, "/occurrences?limit=20&offset=0", "parent-a", "")
	if occList.Code != 200 {
		t.Fatalf("occurrence list=%d %s", occList.Code, occList.Body.String())
	}
	var page OccurrencePage2
	_ = json.Unmarshal(occList.Body.Bytes(), &page)
	if len(page.Items) == 0 {
		t.Fatal("schedule did not materialize an occurrence")
	}
	occ := page.Items[0]
	var oldOccurrence *Occurrence2
	var futureOccurrence *Occurrence2
	for i := range page.Items {
		item := &page.Items[i]
		if item.DueOffsetMinutes == 0 {
			oldOccurrence = item
		}
		if item.DueOffsetMinutes == 5 {
			futureOccurrence = item
		}
	}
	if oldOccurrence == nil || oldOccurrence.ScheduleVersion != 1 || oldOccurrence.Timezone != "UTC" {
		t.Fatalf("old occurrence did not retain its schedule snapshot: %+v", oldOccurrence)
	}
	if futureOccurrence == nil || futureOccurrence.ScheduleVersion != 2 || futureOccurrence.DueOffsetMinutes != 5 {
		t.Fatalf("future occurrence did not use the updated schedule snapshot: %+v", futureOccurrence)
	}
	if _, err := s.DB.Exec(context.Background(), `UPDATE task_revisions SET title='Edited after issue', instructions='Changed after issue' WHERE id=$1`, revision.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(context.Background(), `UPDATE task_schedules SET timezone='America/Chicago', due_offset_minutes=17, version=version+1 WHERE id=$1`, sch.ID); err != nil {
		t.Fatal(err)
	}
	assertHistorical := func(label, body string) {
		if !strings.Contains(body, "Morning care revised") || strings.Contains(body, "Edited after issue") || strings.Contains(body, "Changed after issue") || !strings.Contains(body, `"timezone":"UTC"`) {
			t.Fatalf("%s projection changed after task/schedule edits: %s", label, body)
		}
	}
	updated := requestJSON(t, h, http.MethodGet, "/occurrences/"+oldOccurrence.ID, "parent-a", "")
	if updated.Code != 200 {
		t.Fatalf("parent historical detail=%d %s", updated.Code, updated.Body.String())
	}
	assertHistorical("parent", updated.Body.String())
	devicePair := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	deviceCode, _ := pairingResponse(t, devicePair)
	device := requestJSON(t, h, http.MethodPost, "/device/pair", "", `{"code":"`+deviceCode+`"}`)
	var deviceAuth struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(device.Body.Bytes(), &deviceAuth); err != nil || deviceAuth.Token == "" {
		t.Fatalf("device pair=%d %s", device.Code, device.Body.String())
	}
	deviceDetail := requestBearer(t, h, http.MethodGet, "/device/occurrences/"+oldOccurrence.ID, deviceAuth.Token)
	if deviceDetail.Code != 200 {
		t.Fatalf("device historical detail=%d %s", deviceDetail.Code, deviceDetail.Body.String())
	}
	assertHistorical("device", deviceDetail.Body.String())
	pair := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	code, _ := pairingResponse(t, pair)
	studentPair := requestJSON(t, h, http.MethodPost, "/student/pair", "", `{"code":"`+code+`"}`)
	if studentPair.Code != 200 {
		t.Fatalf("student pair=%d %s", studentPair.Code, studentPair.Body.String())
	}
	studentRequest := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Origin", s.Auth.PublicOrigin)
		for _, cookie := range studentPair.Result().Cookies() {
			req.AddCookie(cookie)
			if cookie.Name == "tasks_csrf" {
				req.Header.Set("X-CSRF-Token", cookie.Value)
			}
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if got := studentRequest(http.MethodGet, "/student/occurrences/"+bob); got.Code != 404 {
		t.Fatalf("foreign browser detail=%d", got.Code)
	}
	if got := studentRequest(http.MethodPost, "/student/occurrences/"+bob+"/start"); got.Code != 404 {
		t.Fatalf("foreign browser start=%d", got.Code)
	}
	if got := studentRequest(http.MethodPost, "/student/occurrences/"+bob+"/submit"); got.Code != 404 {
		t.Fatalf("foreign browser submit=%d", got.Code)
	}
	if got := studentRequest(http.MethodGet, "/student/today"); got.Code != 200 {
		t.Fatalf("student today=%d %s", got.Code, got.Body.String())
	}
	if got := studentRequest(http.MethodGet, "/student/occurrences/"+oldOccurrence.ID); got.Code != 200 {
		t.Fatalf("student historical detail=%d %s", got.Code, got.Body.String())
	} else {
		assertHistorical("student/browser", got.Body.String())
	}
	if got := studentRequest(http.MethodPost, "/student/occurrences/"+occ.ID+"/start"); got.Code != 200 {
		t.Fatalf("student start=%d %s", got.Code, got.Body.String())
	}
	if got := studentRequest(http.MethodPost, "/student/occurrences/"+occ.ID+"/start"); got.Code != 200 {
		t.Fatalf("student start replay=%d %s", got.Code, got.Body.String())
	}
	if got := studentRequest(http.MethodPost, "/student/occurrences/"+occ.ID+"/submit"); got.Code != 200 {
		t.Fatalf("student submit=%d %s", got.Code, got.Body.String())
	}
	if got := studentRequest(http.MethodPost, "/student/occurrences/"+occ.ID+"/submit"); got.Code != 200 {
		t.Fatalf("student submit replay=%d %s", got.Code, got.Body.String())
	}
	if got := requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/skip", "parent-a", ""); got.Code != 200 {
		t.Fatalf("skip=%d %s", got.Code, got.Body.String())
	}
	if got := requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/decision", "parent-a", `{"accepted":true,"reason":"stale"}`); got.Code != 409 {
		t.Fatalf("approve after skip=%d %s", got.Code, got.Body.String())
	}
	if got := requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/retry", "parent-a", ""); got.Code != 409 {
		t.Fatalf("retry after skip=%d %s", got.Code, got.Body.String())
	}
	if got := requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/cancel", "parent-a", ""); got.Code != 409 {
		t.Fatalf("cancel after skip=%d %s", got.Code, got.Body.String())
	}
	if futureOccurrence != nil {
		if got := requestJSON(t, h, http.MethodPost, "/occurrences/"+futureOccurrence.ID+"/cancel", "parent-a", ""); got.Code != 200 {
			t.Fatalf("cancel pending=%d %s", got.Code, got.Body.String())
		}
	}
	if got := requestJSON(t, h, http.MethodPost, "/tasks/"+task.TemplateID+"/retire", "parent-a", ""); got.Code != 204 {
		t.Fatalf("task retire=%d %s", got.Code, got.Body.String())
	}
	if got := requestJSON(t, h, http.MethodPost, "/tasks/"+task.TemplateID+"/retire", "parent-a", ""); got.Code != 404 {
		t.Fatalf("repeat retire=%d", got.Code)
	}
	if got := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); got.Code != 404 {
		t.Fatalf("retired publish=%d", got.Code)
	}
}

func TestPhase2RejectsInvalidSchedulesAndCrossTenantMutations(t *testing.T) {
	pool := integrationPool(t)
	alice, bob := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2c"), IssuerSecret: []byte("p2c")})
	h := s.Routes()
	if got := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Invalid","instructions":"x","requirements":[{"id":"r","kind":"agent_dialogue","configVersion":1,"config":{},"interaction":"chat","executor":"fantasy"}]}`); got.Code != 400 {
		t.Fatalf("invalid requirement=%d", got.Code)
	}
	if got := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{}`); got.Code != 400 {
		t.Fatalf("empty task=%d", got.Code)
	}
	good := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Valid","instructions":"x","requirements":[{"id":"r","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	if good.Code != 201 {
		t.Fatalf("valid task=%d", good.Code)
	}
	var task struct {
		ID         string `json:"id"`
		TemplateID string `json:"templateId"`
	}
	_ = json.Unmarshal(good.Body.Bytes(), &task)
	if got := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); got.Code != 200 {
		t.Fatalf("publish=%d %s", got.Code, got.Body.String())
	}
	if got := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); got.Code != 404 {
		t.Fatalf("republish=%d", got.Code)
	}
	base := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"recurrence","timezone":"Not/IANA","startAt":"` + time.Now().UTC().Format(time.RFC3339) + `","rrule":"FREQ=MONTHLY","dueOffsetMinutes":0}`
	if got := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", base); got.Code != 400 {
		t.Fatalf("bad timezone=%d", got.Code)
	}
	base = `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"unsupported","timezone":"UTC","startAt":"` + time.Now().UTC().Format(time.RFC3339) + `","rrule":"","dueOffsetMinutes":0}`
	if got := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", base); got.Code != 400 {
		t.Fatalf("bad kind=%d", got.Code)
	}
	base = `{"studentId":"` + bob + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"recurrence","timezone":"UTC","startAt":"` + time.Now().UTC().Format(time.RFC3339) + `","rrule":"FREQ=DAILY;COUNT=2","dueOffsetMinutes":0}`
	if got := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", base); got.Code != 409 {
		t.Fatalf("cross tenant schedule=%d", got.Code)
	}
	if got := requestJSON(t, h, http.MethodGet, "/tasks", "parent-b", ""); got.Code != 200 || strings.Contains(got.Body.String(), task.ID) {
		t.Fatalf("cross tenant task list=%d %s", got.Code, got.Body.String())
	}
	fake := alice
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/occurrences/" + fake, 404},
		{http.MethodPost, "/occurrences/" + fake + "/retry", 404},
		{http.MethodPost, "/occurrences/" + fake + "/skip", 404},
		{http.MethodPost, "/student/occurrences/" + fake + "/start", 401},
		{http.MethodPost, "/student/occurrences/" + fake + "/submit", 401},
		{http.MethodPost, "/device/occurrences/" + fake + "/submit", 401},
	} {
		got := requestJSON(t, h, tc.method, tc.path, "parent-a", "")
		if got.Code != tc.want {
			t.Fatalf("negative %s %s=%d want %d", tc.method, tc.path, got.Code, tc.want)
		}
	}
	if got := requestJSON(t, h, http.MethodPost, "/tasks/"+task.TemplateID+"/revisions", "parent-a", `{"title":"","instructions":"","requirements":[]}`); got.Code != 400 {
		t.Fatalf("invalid revision=%d", got.Code)
	}
}
