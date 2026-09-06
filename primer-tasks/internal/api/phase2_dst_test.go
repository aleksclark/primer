package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"primer-tasks/internal/schedule"
)

func TestPhase2DailyIntervalMaterializesEveryOtherDay(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-interval"), IssuerSecret: []byte("p2-interval")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Every other day","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct {
		ID, TemplateID string
	}
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", `{"studentId":"`+alice+`","templateId":"`+task.TemplateID+`","revisionId":"`+task.ID+`","kind":"recurrence","timezone":"UTC","startAt":"`+start.Format(time.RFC3339)+`","rrule":"FREQ=DAILY;INTERVAL=2;COUNT=3","dueOffsetMinutes":0}`); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	var n, distinct int
	if err := pool.QueryRow(context.Background(), `SELECT count(*), count(DISTINCT (schedule_id, nominal_at)) FROM task_occurrences WHERE student_id=$1`, alice).Scan(&n, &distinct); err != nil {
		t.Fatal(err)
	}
	if n != 3 || distinct != 3 {
		t.Fatalf("interval series total=%d distinct=%d", n, distinct)
	}
	rows, err := pool.Query(context.Background(), `SELECT nominal_at FROM task_occurrences WHERE student_id=$1 ORDER BY nominal_at`, alice)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var prev time.Time
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			t.Fatal(err)
		}
		if !prev.IsZero() && at.Sub(prev) != 48*time.Hour {
			t.Fatalf("interval gap=%s", at.Sub(prev))
		}
		prev = at
	}
}

func TestPhase2HorizonDoesNotMaterializeFarFutureOneOff(t *testing.T) {
	t.Setenv("TASKS_SCHEDULE_HORIZON_DAYS", "1")
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-horizon"), IssuerSecret: []byte("p2-horizon")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Far future","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct {
		ID, TemplateID string
	}
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(10 * 24 * time.Hour).Format(time.RFC3339)
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", `{"studentId":"`+alice+`","templateId":"`+task.TemplateID+`","revisionId":"`+task.ID+`","kind":"one_off","timezone":"UTC","startAt":"`+start+`","dueOffsetMinutes":0}`); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	if err := schedule.NewWorker(pool).Materialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM task_occurrences WHERE student_id=$1`, alice).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("far-future one-off materialized %d rows inside a 1-day horizon", n)
	}
}

func TestPhase2ConcurrentWorkersDoNotDuplicateOccurrences(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-lease"), IssuerSecret: []byte("p2-lease")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Lease uniqueness","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct {
		ID, TemplateID string
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	errors := make(chan error, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- schedule.NewWorker(pool).Materialize(ctx)
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n, distinct int
	if err := pool.QueryRow(context.Background(), `SELECT count(*), count(DISTINCT (schedule_id, nominal_at)) FROM task_occurrences WHERE student_id=$1`, alice).Scan(&n, &distinct); err != nil {
		t.Fatal(err)
	}
	if n != 1 || distinct != 1 {
		t.Fatalf("concurrent workers duplicated occurrences total=%d distinct=%d", n, distinct)
	}
}

func TestPhase2WorkerRunStopsOnCancel(t *testing.T) {
	pool := integrationPool(t)
	_, _ = seedIntegration(t, pool)
	w := schedule.NewWorker(pool)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker Run did not stop after cancel")
	}
}

func TestPhase2WorkerSkipsCorruptRRULEAndMaterializesLaterSchedules(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-corrupt"), IssuerSecret: []byte("p2-corrupt")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Worker robustness","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct {
		ID, TemplateID string
	}
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute)
	var badID string
	if err := pool.QueryRow(context.Background(), `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local,rrule,due_offset_minutes) VALUES (gen_random_uuid(),$1,$2,$3,$4,'recurrence','UTC',$5,'FREQ=MONTHLY',0) RETURNING id`, tenantA, alice, task.TemplateID, task.ID, start).Scan(&badID); err != nil {
		t.Fatal(err)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", `{"studentId":"`+alice+`","templateId":"`+task.TemplateID+`","revisionId":"`+task.ID+`","kind":"one_off","timezone":"UTC","startAt":"`+start.Format(time.RFC3339)+`","dueOffsetMinutes":0}`); rec.Code != 201 {
		t.Fatalf("good schedule=%d %s", rec.Code, rec.Body.String())
	}
	w := schedule.NewWorker(pool)
	if err := w.Materialize(context.Background()); err != nil {
		t.Fatalf("corrupt schedule blocked worker: %v", err)
	}
	var bad, good int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM task_occurrences WHERE schedule_id=$1`, badID).Scan(&bad); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM task_occurrences WHERE student_id=$1 AND schedule_id<>$2`, alice, badID).Scan(&good); err != nil {
		t.Fatal(err)
	}
	if bad != 0 || good == 0 {
		t.Fatalf("corrupt=%d good=%d", bad, good)
	}
}

func TestPhase2UntilBoundStopsMaterializationAndRestartIsUnique(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-until"), IssuerSecret: []byte("p2-until")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Until bound","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct {
		ID, TemplateID string
	}
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	until := time.Date(2026, 1, 7, 8, 0, 0, 0, time.UTC)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"recurrence","timezone":"UTC","startAt":"` + start.Format(time.RFC3339) + `","endAt":"` + until.Format(time.RFC3339) + `","rrule":"FREQ=DAILY","dueOffsetMinutes":15}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	restart := schedule.NewWorker(pool)
	if err := restart.Materialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n, distinct, offsets int
	if err := pool.QueryRow(context.Background(), `SELECT count(*), count(DISTINCT (schedule_id, nominal_at)), count(*) FILTER (WHERE due_at = nominal_at + interval '15 minutes') FROM task_occurrences WHERE student_id=$1`, alice).Scan(&n, &distinct, &offsets); err != nil {
		t.Fatal(err)
	}
	if n != 3 || distinct != 3 || offsets != 3 {
		t.Fatalf("until series total=%d distinct=%d due-offset=%d", n, distinct, offsets)
	}
}

func TestPhase2WeeklyRecurrenceMaterializesUniqueNominals(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-weekly"), IssuerSecret: []byte("p2-weekly")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Weekly care","instructions":"x","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct {
		ID, TemplateID string
	}
	if e := json.Unmarshal(create.Body.Bytes(), &task); e != nil {
		t.Fatal(e)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatalf("publish=%d %s", rec.Code, rec.Body.String())
	}
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"recurrence","timezone":"UTC","startAt":"` + start.Format(time.RFC3339) + `","rrule":"FREQ=WEEKLY;INTERVAL=2;BYDAY=MO;COUNT=3","dueOffsetMinutes":0}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	worker := schedule.NewWorker(pool)
	if err := worker.Materialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := worker.Materialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n, distinct int
	if err := pool.QueryRow(context.Background(), `SELECT count(*), count(DISTINCT (schedule_id, nominal_at)) FROM task_occurrences WHERE student_id=$1`, alice).Scan(&n, &distinct); err != nil {
		t.Fatal(err)
	}
	if n != 3 || distinct != 3 {
		t.Fatalf("weekly series total=%d distinct=%d", n, distinct)
	}
	var gaps []time.Duration
	rows, err := pool.Query(context.Background(), `SELECT nominal_at FROM task_occurrences WHERE student_id=$1 ORDER BY nominal_at`, alice)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var prev time.Time
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			t.Fatal(err)
		}
		if !prev.IsZero() {
			gaps = append(gaps, at.Sub(prev))
		}
		prev = at
	}
	if len(gaps) != 2 || gaps[0] != 14*24*time.Hour || gaps[1] != 14*24*time.Hour {
		t.Fatalf("weekly gaps=%v", gaps)
	}
}

func TestPhase2DSTGapMaterializationSkipsMissingWallTime(t *testing.T) {
	t.Setenv("TASKS_SCHEDULE_HORIZON_DAYS", "400")
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-gap"), IssuerSecret: []byte("p2-gap")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"DST gap","instructions":"Skip 02:30","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
	var task struct {
		ID, TemplateID string
	}
	_ = json.Unmarshal(create.Body.Bytes(), &task)
	_ = requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", "")
	loc, _ := time.LoadLocation("America/New_York")
	start := time.Date(2026, 3, 7, 2, 30, 0, 0, loc)
	until := time.Date(2026, 3, 11, 12, 0, 0, 0, loc)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"recurrence","timezone":"America/New_York","startAt":"` + start.Format(time.RFC3339) + `","endAt":"` + until.Format(time.RFC3339) + `","rrule":"FREQ=DAILY","dueOffsetMinutes":0}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	var gap, total, distinct int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM task_occurrences WHERE student_id=$1 AND timezone('America/New_York', nominal_at)::date = DATE '2026-03-08'`, alice).Scan(&gap); err != nil {
		t.Fatal(err)
	}
	if gap != 0 {
		t.Fatalf("gap day invented %d occurrences", gap)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*), count(DISTINCT (schedule_id, nominal_at)) FROM task_occurrences WHERE student_id=$1`, alice).Scan(&total, &distinct); err != nil {
		t.Fatal(err)
	}
	if total != 4 || distinct != 4 {
		t.Fatalf("gap series total=%d distinct=%d", total, distinct)
	}
	rows, err := pool.Query(context.Background(), `SELECT timezone('America/New_York', nominal_at) FROM task_occurrences WHERE student_id=$1 ORDER BY nominal_at`, alice)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var local time.Time
		if err := rows.Scan(&local); err != nil {
			t.Fatal(err)
		}
		if local.Hour() != 2 || local.Minute() != 30 {
			t.Fatalf("gap series local wall moved: %s", local)
		}
		if local.Month() == time.March && local.Day() == 8 {
			t.Fatalf("persisted gap instant %s", local)
		}
	}
}

func TestPhase2DSTFoldMaterializationIsUniqueAcrossRestart(t *testing.T) {
	t.Setenv("TASKS_SCHEDULE_HORIZON_DAYS", "400")
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("p2-dst"), IssuerSecret: []byte("p2-dst")})
	h := s.Routes()
	create := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"DST fold","instructions":"Keep local 01:30","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
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
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 31, 1, 30, 0, 0, loc)
	until := time.Date(2026, 11, 3, 12, 0, 0, 0, loc)
	body := `{"studentId":"` + alice + `","templateId":"` + task.TemplateID + `","revisionId":"` + task.ID + `","kind":"recurrence","timezone":"America/New_York","startAt":"` + start.Format(time.RFC3339) + `","endAt":"` + until.Format(time.RFC3339) + `","rrule":"FREQ=DAILY","dueOffsetMinutes":0}`
	if rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", body); rec.Code != 201 {
		t.Fatalf("schedule=%d %s", rec.Code, rec.Body.String())
	}
	worker := schedule.NewWorker(pool)
	if err := worker.Materialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := worker.Materialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	restart := schedule.NewWorker(pool)
	if err := restart.Materialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n, distinct, fold int
	if err := pool.QueryRow(context.Background(), `SELECT count(*), count(DISTINCT (schedule_id, nominal_at)) FROM task_occurrences WHERE student_id=$1`, alice).Scan(&n, &distinct); err != nil {
		t.Fatal(err)
	}
	if n != 5 || distinct != 5 {
		t.Fatalf("fold series total=%d distinct=%d", n, distinct)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM task_occurrences WHERE student_id=$1 AND timezone('America/New_York', nominal_at)::date = DATE '2026-11-01'`, alice).Scan(&fold); err != nil {
		t.Fatal(err)
	}
	if fold != 2 {
		t.Fatalf("fold day occurrences=%d", fold)
	}
}
