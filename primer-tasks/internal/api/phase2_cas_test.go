package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPhase2CreateScheduleAndRevisionRejectUnsupportedInput(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-input"), IssuerSecret: []byte("p2-input")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Input guard","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct{ ID, TemplateID string }
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", `{"studentId":"`+alice+`","templateId":"`+task.TemplateID+`","revisionId":"`+task.ID+`","kind":"recurrence","timezone":"UTC","startAt":"`+start+`","rrule":"FREQ=MONTHLY","dueOffsetMinutes":0}`); rec.Code != 400 {
		t.Fatalf("monthly create=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.TemplateID+"/revisions", "parent-a", `{"title":"Bad driver","instructions":"x","requirements":[{"id":"r","kind":"agent_dialogue","configVersion":1,"config":{},"interaction":"chat","executor":"fantasy"}]}`); rec.Code != 400 {
		t.Fatalf("unsupported revision=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPatch, "/schedules/"+alice, "parent-a", `{"studentId":"`+alice+`","templateId":"`+task.TemplateID+`","revisionId":"`+task.ID+`","kind":"unsupported","timezone":"UTC","startAt":"`+start+`","dueOffsetMinutes":0}`); rec.Code != 400 {
		t.Fatalf("bad update kind=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPatch, "/schedules/"+alice, "parent-a", `{"studentId":"`+alice+`","templateId":"`+task.TemplateID+`","revisionId":"`+task.ID+`","kind":"recurrence","timezone":"UTC","startAt":"`+start+`","rrule":"FREQ=MONTHLY","dueOffsetMinutes":0}`); rec.Code != 400 {
		t.Fatalf("bad update rrule=%d %s", rec.Code, rec.Body.String())
	}
}

func TestPhase2OccurrenceCollectionFiltersAreServerOwned(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-list"), IssuerSecret: []byte("p2-list")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Collection filter","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct{ ID, TemplateID string }
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", `{"studentId":"`+alice+`","templateId":"`+task.TemplateID+`","revisionId":"`+task.ID+`","kind":"one_off","timezone":"UTC","startAt":"`+start+`","dueOffsetMinutes":0}`); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	pending := requestJSON(t, h, http.MethodGet, "/occurrences?status=pending&limit=20&sort=nominalAt&dir=asc", "parent-a", "")
	if pending.Code != 200 || !strings.Contains(pending.Body.String(), "Collection filter") {
		t.Fatalf("pending filter=%d %s", pending.Code, pending.Body.String())
	}
	completed := requestJSON(t, h, http.MethodGet, "/occurrences?status=completed&limit=20", "parent-a", "")
	if completed.Code != 200 || strings.Contains(completed.Body.String(), "Collection filter") {
		t.Fatalf("completed filter leaked pending work: %s", completed.Body.String())
	}
	tasks := requestJSON(t, h, http.MethodGet, "/tasks?q=Collection&sort=title&dir=asc&limit=5", "parent-a", "")
	if tasks.Code != 200 || !strings.Contains(tasks.Body.String(), "Collection filter") {
		t.Fatalf("task search=%d %s", tasks.Code, tasks.Body.String())
	}
}

func TestPhase2SkipDoesNotCompleteAndRequiresStartThenSubmit(t *testing.T) {
	pool := integrationPool(t)
	alice, bob := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-cas"), IssuerSecret: []byte("p2-cas")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Brush your teeth","instructions":"Use the timer","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
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
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"one_off","timezone":"America/New_York","startAt":"` + start + `","dueOffsetMinutes":0}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	list := requestJSON(t, h, http.MethodGet, "/occurrences?limit=20", "parent-a", "")
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
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/skip", "parent-a", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"excused"`) {
		t.Fatalf("skip=%d %s", rec.Code, rec.Body.String())
	}
	var status string
	if e := pool.QueryRow(context.Background(), `SELECT status FROM task_occurrences WHERE id=$1`, occ.ID).Scan(&status); e != nil || status != "excused" {
		t.Fatalf("skipped status=%q err=%v", status, e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/decision", "parent-a", `{"accepted":true,"reason":"stale after skip"}`); rec.Code != 409 {
		t.Fatalf("stale approve after skip=%d %s", rec.Code, rec.Body.String())
	}
	if e := pool.QueryRow(context.Background(), `SELECT status FROM task_occurrences WHERE id=$1`, occ.ID).Scan(&status); e != nil || status != "excused" {
		t.Fatalf("skip mutated by stale approve: %q", status)
	}
	var decisions int
	if e := pool.QueryRow(context.Background(), `SELECT count(*) FROM verification_decisions d JOIN verification_attempts a ON a.id=d.attempt_id WHERE a.occurrence_id=$1`, occ.ID).Scan(&decisions); e != nil || decisions != 0 {
		t.Fatalf("stale approve persisted %d decisions", decisions)
	}

	second := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Second brushing","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task2 struct {
		ID, TemplateID string
	}
	_ = json.Unmarshal(second.Body.Bytes(), &task2)
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task2.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish second=%d %s", rec.Code, rec.Body.String())
	}
	body2 := `{"studentId":"` + alice + `","templateId":"` + task2.TemplateID + `","revisionId":"` + task2.ID + `","kind":"one_off","timezone":"America/New_York","startAt":"` + start + `","dueOffsetMinutes":0}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body2); rec.Code != 201 {
		t.Fatalf("second schedule=%d %s", rec.Code, rec.Body.String())
	}
	list = requestJSON(t, h, http.MethodGet, "/occurrences?limit=50", "parent-a", "")
	_ = json.Unmarshal(list.Body.Bytes(), &page)
	var live Occurrence2
	for _, item := range page.Items {
		if item.Title == "Second brushing" {
			live = item
		}
	}
	if live.ID == "" {
		t.Fatalf("missing second occurrence: %s", list.Body.String())
	}
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+live.ID+"/submit", dv.Token); rec.Code != 409 {
		t.Fatalf("submit before start=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+live.ID+"/start", dv.Token); rec.Code != 200 {
		t.Fatalf("start=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/occurrences/"+live.ID, "parent-a", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"in_progress"`) {
		t.Fatalf("in_progress=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+live.ID+"/decision", "parent-a", `{"accepted":true,"reason":"too early"}`); rec.Code != 409 {
		t.Fatalf("approve in_progress=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+live.ID+"/submit", dv.Token); rec.Code != 200 {
		t.Fatalf("submit=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+live.ID+"/decision", "parent-b", `{"accepted":true,"reason":"cross"}`); rec.Code != 404 && rec.Code != 409 {
		t.Fatalf("cross-tenant decide=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+live.ID+"/start", dv.Token); rec.Code != 409 {
		t.Fatalf("start after submit=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+live.ID+"/decision", "parent-a", `{"accepted":false,"reason":"try again"}`); rec.Code != 200 {
		t.Fatalf("reject=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+live.ID+"/decision", "parent-a", `{"accepted":false,"reason":"replay reject"}`); rec.Code != 200 {
		t.Fatalf("reject replay=%d %s", rec.Code, rec.Body.String())
	}
	var rejectedStatus string
	if e := pool.QueryRow(context.Background(), `SELECT status FROM task_occurrences WHERE id=$1`, live.ID).Scan(&rejectedStatus); e != nil || rejectedStatus != "pending" {
		t.Fatalf("reject left status=%q err=%v", rejectedStatus, e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+live.ID+"/retry", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("retry=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/occurrences/"+live.ID, "parent-a", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"awaiting_verification"`) {
		t.Fatalf("retried=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+live.ID+"/decision", "parent-a", `{"accepted":true,"reason":"observed"}`); rec.Code != 200 {
		t.Fatalf("approve=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+live.ID+"/skip", "parent-a", ""); rec.Code != 409 {
		t.Fatalf("skip completed=%d %s", rec.Code, rec.Body.String())
	}
	_ = bob
}

func TestPhase2BrowserStartSubmitIdempotency(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-browser"), IssuerSecret: []byte("p2-browser")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Browser lifecycle","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct{ ID, TemplateID string }
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"one_off","timezone":"UTC","startAt":"` + start + `","dueOffsetMinutes":0}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	list := requestJSON(t, h, http.MethodGet, "/occurrences?limit=5", "parent-a", "")
	var page OccurrencePage2
	if e := json.Unmarshal(list.Body.Bytes(), &page); e != nil || len(page.Items) == 0 {
		t.Fatalf("page=%s err=%v", list.Body.String(), e)
	}
	occ := page.Items[0]
	pair := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	code, _ := pairingResponse(t, pair)
	studentPair := requestJSON(t, h, http.MethodPost, "/student/pair", "", `{"code":"`+code+`"}`)
	if studentPair.Code != 200 || len(studentPair.Result().Cookies()) == 0 {
		t.Fatalf("student pair=%d %s", studentPair.Code, studentPair.Body.String())
	}
	cookie := studentPair.Result().Cookies()[0].Value
	req := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(&http.Cookie{Name: "tasks_student", Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	if got := req(http.MethodPost, "/student/occurrences/"+occ.ID+"/submit"); got.Code != 409 {
		t.Fatalf("browser submit before start=%d %s", got.Code, got.Body.String())
	}
	if got := req(http.MethodPost, "/student/occurrences/"+occ.ID+"/start"); got.Code != 200 || !strings.Contains(got.Body.String(), `"status":"in_progress"`) {
		t.Fatalf("browser start=%d %s", got.Code, got.Body.String())
	}
	if got := req(http.MethodPost, "/student/occurrences/"+occ.ID+"/start"); got.Code != 200 || !strings.Contains(got.Body.String(), `"status":"in_progress"`) {
		t.Fatalf("browser start replay=%d %s", got.Code, got.Body.String())
	}
	if got := req(http.MethodPost, "/student/occurrences/"+occ.ID+"/submit"); got.Code != 200 || !strings.Contains(got.Body.String(), `"status":"awaiting_verification"`) {
		t.Fatalf("browser submit=%d %s", got.Code, got.Body.String())
	}
	if got := req(http.MethodPost, "/student/occurrences/"+occ.ID+"/submit"); got.Code != 200 || !strings.Contains(got.Body.String(), `"status":"awaiting_verification"`) {
		t.Fatalf("browser submit replay=%d %s", got.Code, got.Body.String())
	}
	var attempts int
	if e := pool.QueryRow(context.Background(), `SELECT count(*) FROM verification_attempts WHERE occurrence_id=$1`, occ.ID).Scan(&attempts); e != nil || attempts != 1 {
		t.Fatalf("submit replay created %d attempts err=%v", attempts, e)
	}
}

func TestPhase2ConcurrentStartLeavesOneInProgress(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-race"), IssuerSecret: []byte("p2-race")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Race start","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct{ ID, TemplateID string }
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"one_off","timezone":"UTC","startAt":"` + start + `","dueOffsetMinutes":0}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	list := requestJSON(t, h, http.MethodGet, "/occurrences?limit=5", "parent-a", "")
	var page OccurrencePage2
	if e := json.Unmarshal(list.Body.Bytes(), &page); e != nil || len(page.Items) == 0 {
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
	results := make(chan int, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/start", dv.Token).Code
		}()
	}
	wg.Wait()
	close(results)
	var ok, conflict, other int
	for code := range results {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			other++
		}
	}
	if other != 0 || ok == 0 || ok+conflict != 8 {
		t.Fatalf("concurrent start ok=%d conflict=%d other=%d", ok, conflict, other)
	}
	var status string
	if e := pool.QueryRow(context.Background(), `SELECT status FROM task_occurrences WHERE id=$1`, occ.ID).Scan(&status); e != nil || status != "in_progress" {
		t.Fatalf("raced start status=%q err=%v", status, e)
	}
}

func TestPhase2ConcurrentSubmitAndDecisionAreUnique(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-unique"), IssuerSecret: []byte("p2-unique")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Unique CAS","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct{ ID, TemplateID string }
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"one_off","timezone":"UTC","startAt":"` + start + `","dueOffsetMinutes":0}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	list := requestJSON(t, h, http.MethodGet, "/occurrences?limit=5", "parent-a", "")
	var page OccurrencePage2
	if e := json.Unmarshal(list.Body.Bytes(), &page); e != nil || len(page.Items) == 0 {
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
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/start", dv.Token); rec.Code != 200 {
		t.Fatalf("start=%d %s", rec.Code, rec.Body.String())
	}
	submits := make(chan int, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			submits <- requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/submit", dv.Token).Code
		}()
	}
	wg.Wait()
	close(submits)
	var submitOK int
	for code := range submits {
		if code == http.StatusOK {
			submitOK++
		} else if code != http.StatusConflict {
			t.Fatalf("submit status=%d", code)
		}
	}
	if submitOK == 0 {
		t.Fatal("no submit succeeded")
	}
	var attempts int
	var status string
	if e := pool.QueryRow(context.Background(), `SELECT status, (SELECT count(*) FROM verification_attempts a WHERE a.occurrence_id=o.id) FROM task_occurrences o WHERE id=$1`, occ.ID).Scan(&status, &attempts); e != nil || status != "awaiting_verification" || attempts != 1 {
		t.Fatalf("after submit status=%q attempts=%d err=%v", status, attempts, e)
	}
	decisions := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decisions <- requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/decision", "parent-a", `{"accepted":true,"reason":"observed"}`).Code
		}()
	}
	wg.Wait()
	close(decisions)
	var decideOK int
	for code := range decisions {
		if code == http.StatusOK {
			decideOK++
		} else if code != http.StatusConflict {
			t.Fatalf("decide status=%d", code)
		}
	}
	if decideOK == 0 {
		t.Fatal("no decision succeeded")
	}
	var decisionCount int
	if e := pool.QueryRow(context.Background(), `SELECT status, (SELECT count(*) FROM verification_decisions d JOIN verification_attempts a ON a.id=d.attempt_id WHERE a.occurrence_id=o.id) FROM task_occurrences o WHERE id=$1`, occ.ID).Scan(&status, &decisionCount); e != nil || status != "completed" || decisionCount != 1 {
		t.Fatalf("after decide status=%q decisions=%d err=%v", status, decisionCount, e)
	}
}

func TestPhase2SkipInProgressAndCancelAwaiting(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-skip-ip"), IssuerSecret: []byte("p2-skip-ip")})
	h := s.Routes()
	publish := func(title string) (id, template string) {
		create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"`+title+`","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
		var task struct{ ID, TemplateID string }
		if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
			t.Fatal(e)
		}
		if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
			t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
		}
		return task.ID, task.TemplateID
	}
	id1, tmpl1 := publish("Skip in progress")
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", `{"studentId":"`+alice+`","templateId":"`+tmpl1+`","revisionId":"`+id1+`","kind":"one_off","timezone":"UTC","startAt":"`+start+`","dueOffsetMinutes":0}`); rec.Code != 201 {
		t.Fatalf("schedule 1=%d %s", rec.Code, rec.Body.String())
	}
	id2, tmpl2 := publish("Cancel awaiting")
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", `{"studentId":"`+alice+`","templateId":"`+tmpl2+`","revisionId":"`+id2+`","kind":"one_off","timezone":"UTC","startAt":"`+start+`","dueOffsetMinutes":0}`); rec.Code != 201 {
		t.Fatalf("schedule 2=%d %s", rec.Code, rec.Body.String())
	}
	list := requestJSON(t, h, http.MethodGet, "/occurrences?limit=20", "parent-a", "")
	var page OccurrencePage2
	if e := json.Unmarshal(list.Body.Bytes(), &page); e != nil {
		t.Fatal(e)
	}
	var skipOcc, cancelOcc Occurrence2
	for _, item := range page.Items {
		switch item.Title {
		case "Skip in progress":
			skipOcc = item
		case "Cancel awaiting":
			cancelOcc = item
		}
	}
	if skipOcc.ID == "" || cancelOcc.ID == "" {
		t.Fatalf("missing occurrences: %s", list.Body.String())
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
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+skipOcc.ID+"/start", dv.Token); rec.Code != 200 {
		t.Fatalf("start skip target=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+skipOcc.ID+"/skip", "parent-a", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"excused"`) {
		t.Fatalf("skip in_progress=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+skipOcc.ID+"/decision", "parent-a", `{"accepted":true,"reason":"stale"}`); rec.Code != 409 {
		t.Fatalf("approve excused=%d %s", rec.Code, rec.Body.String())
	}
	var skipStatus string
	var skipDecisions int
	if e := pool.QueryRow(context.Background(), `SELECT status, (SELECT count(*) FROM verification_decisions d JOIN verification_attempts a ON a.id=d.attempt_id WHERE a.occurrence_id=o.id) FROM task_occurrences o WHERE id=$1`, skipOcc.ID).Scan(&skipStatus, &skipDecisions); e != nil || skipStatus != "excused" || skipDecisions != 0 {
		t.Fatalf("excused status=%q decisions=%d err=%v", skipStatus, skipDecisions, e)
	}
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+cancelOcc.ID+"/start", dv.Token); rec.Code != 200 {
		t.Fatalf("start cancel target=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+cancelOcc.ID+"/submit", dv.Token); rec.Code != 200 {
		t.Fatalf("submit cancel target=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+cancelOcc.ID+"/cancel", "parent-a", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"canceled"`) {
		t.Fatalf("cancel awaiting=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+cancelOcc.ID+"/decision", "parent-a", `{"accepted":true,"reason":"stale"}`); rec.Code != 409 {
		t.Fatalf("approve canceled=%d %s", rec.Code, rec.Body.String())
	}
}

func TestPhase2SubmitWithoutRequirementIsBlocked(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-noreq"), IssuerSecret: []byte("p2-noreq")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"No requirement","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct{ ID, TemplateID string }
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", `{"studentId":"`+alice+`","templateId":"`+task.TemplateID+`","revisionId":"`+task.ID+`","kind":"one_off","timezone":"UTC","startAt":"`+start+`","dueOffsetMinutes":0}`); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	list := requestJSON(t, h, http.MethodGet, "/occurrences?limit=5", "parent-a", "")
	var page OccurrencePage2
	if e := json.Unmarshal(list.Body.Bytes(), &page); e != nil || len(page.Items) == 0 {
		t.Fatalf("page=%s err=%v", list.Body.String(), e)
	}
	occ := page.Items[0]
	if _, err := pool.Exec(context.Background(), `DELETE FROM verification_requirements WHERE revision_id=$1`, task.ID); err != nil {
		t.Fatal(err)
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
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/start", dv.Token); rec.Code != 200 {
		t.Fatalf("start=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearer(t, h, http.MethodPost, "/device/occurrences/"+occ.ID+"/submit", dv.Token); rec.Code != 409 {
		t.Fatalf("submit without requirement=%d %s", rec.Code, rec.Body.String())
	}
	var status string
	var attempts int
	if e := pool.QueryRow(context.Background(), `SELECT status, (SELECT count(*) FROM verification_attempts a WHERE a.occurrence_id=o.id) FROM task_occurrences o WHERE id=$1`, occ.ID).Scan(&status, &attempts); e != nil || status != "in_progress" || attempts != 0 {
		t.Fatalf("blocked submit left status=%q attempts=%d err=%v", status, attempts, e)
	}
}

func TestPhase2RetryWithoutRejectedAttemptIsConflict(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-retry"), IssuerSecret: []byte("p2-retry")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Retry guard","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct{ ID, TemplateID string }
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"one_off","timezone":"UTC","startAt":"` + start + `","dueOffsetMinutes":0}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	list := requestJSON(t, h, http.MethodGet, "/occurrences?limit=5", "parent-a", "")
	var page OccurrencePage2
	if e := json.Unmarshal(list.Body.Bytes(), &page); e != nil || len(page.Items) == 0 {
		t.Fatalf("page=%s err=%v", list.Body.String(), e)
	}
	occ := page.Items[0]
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/retry", "parent-a", ""); rec.Code != 409 {
		t.Fatalf("retry pending without rejection=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/cancel", "parent-a", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"canceled"`) {
		t.Fatalf("cancel pending=%d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/occurrences/"+occ.ID+"/cancel", "parent-a", ""); rec.Code != 409 {
		t.Fatalf("cancel replay=%d %s", rec.Code, rec.Body.String())
	}
	var status string
	if e := pool.QueryRow(context.Background(), `SELECT status FROM task_occurrences WHERE id=$1`, occ.ID).Scan(&status); e != nil || status != "canceled" {
		t.Fatalf("canceled status=%q err=%v", status, e)
	}
}
