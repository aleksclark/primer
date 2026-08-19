package api

import (
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func uuidMust(s string) uuid.UUID { return uuid.MustParse(s) }

func TestPhase2HandlersSurfaceDatabaseFailures(t *testing.T) {
	pool := integrationPool(t)
	pool.Close()
	s := New(pool, "test")
	sc := scope{Tenant: tenantA, Subject: "parent-a"}
	req := func(method, path, body string) *http.Request {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		return r
	}
	run := func(name string, fn func(http.ResponseWriter, *http.Request, scope)) {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			fn(rec, req(http.MethodPost, "/x", `{"title":"x","instructions":"x","requirements":[{"kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`), sc)
			if rec.Code < 400 {
				t.Fatalf("status=%d", rec.Code)
			}
		})
	}
	run("create", s.createTask2)
	run("list", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.listTasks2(w, req(http.MethodGet, "/tasks", ""), sc)
	})
	run("publish", s.publishTask2)
	run("retire", s.retireTask2)
	run("schedule", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.createSchedule2(w, req(http.MethodPost, "/schedules", `{"studentId":"`+tenantA+`","templateId":"`+tenantA+`","revisionId":"`+tenantA+`","kind":"one_off","timezone":"UTC","startAt":"`+time.Now().UTC().Format(time.RFC3339)+`","dueOffsetMinutes":0}`), sc)
	})
	run("materialize", func(w http.ResponseWriter, r *http.Request, sc scope) {
		if e := s.materializeSchedule(r.Context(), sc.Tenant, tenantA, "test"); e == nil {
			t.Fatal("materializer accepted closed DB")
		} else {
			w.WriteHeader(500)
		}
	})
	run("occurrence-list", s.listOccurrences2)
	run("student-list", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.studentOccurrences2(w, r, uuidMust(tenantA))
	})
	run("start", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.startOccurrence2(w, r, uuidMust(tenantA))
	})
	run("decision", s.decideOccurrence2)
	run("retry", s.retryOccurrence2)
	run("status", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.setOccurrenceStatus2(w, r, sc, "canceled")
	})
	run("get", s.parentGetOccurrence2)
	run("student-get", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.studentDetail2(w, r, uuidMust(tenantA))
	})
	run("revise", s.reviseTask2)
	run("schedules", s.listSchedules2)
	run("schedule-update", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.updateSchedule2(w, req(http.MethodPatch, "/schedules/"+tenantA, `{"studentId":"`+tenantA+`","templateId":"`+tenantA+`","revisionId":"`+tenantA+`","kind":"one_off","timezone":"UTC","startAt":"`+time.Now().UTC().Format(time.RFC3339)+`","dueOffsetMinutes":0}`), sc)
	})
	run("schedule-retire", s.retireSchedule2)
}
