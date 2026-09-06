package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestTaskTemplateViewKeepsHistoryAndPaginatesTemplates(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("usability"), IssuerSecret: []byte("usability")})
	h := s.Routes()
	callTask := func(path, title string) TaskRevision {
		t.Helper()
		rec := requestJSON(t, h, http.MethodPost, path, "parent-a", `{"title":"`+title+`","instructions":"Careful steps for `+title+`","requirements":[{"id":"parent-approval","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]}`)
		if rec.Code != 201 {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		var x TaskRevision
		if err := json.Unmarshal(rec.Body.Bytes(), &x); err != nil {
			t.Fatal(err)
		}
		return x
	}
	first := callTask("/tasks", "Original title")
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+first.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", `{"studentId":"`+alice+`","templateId":"`+first.TemplateID+`","revisionId":"`+first.ID+`","kind":"one_off","timezone":"UTC","startAt":"`+start+`"}`)
	if rec.Code != 201 {
		t.Fatal(rec.Body.String())
	}
	second := callTask("/tasks/"+first.TemplateID+"/revisions", "Revised title")
	third := callTask("/tasks/"+first.TemplateID+"/revisions", "Latest title")
	callTask("/tasks", "Another task")
	list := func(query, parent string) TaskPage2 {
		t.Helper()
		rec := requestJSON(t, h, http.MethodGet, "/tasks"+query, parent, "")
		if rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
		var page TaskPage2
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	page := list("?view=templates&limit=1&sort=title&dir=asc", "parent-a")
	if page.TotalCount != 2 || len(page.Items) != 1 || page.Items[0].Title != "Another task" {
		t.Fatalf("template page: %+v", page)
	}
	page = list("?view=templates&limit=1&offset=1&sort=title&dir=asc", "parent-a")
	if len(page.Items) != 1 || page.Items[0].ID != third.ID || len(page.Items[0].Requirements) != 1 {
		t.Fatalf("latest draft: %+v", page)
	}
	if page := list("?view=templates&q=Original", "parent-a"); page.TotalCount != 0 {
		t.Fatal("search resurrected stale revision")
	}
	page = list("?view=templates&status=published", "parent-a")
	if len(page.Items) != 1 || page.Items[0].ID != first.ID {
		t.Fatalf("published picker: %+v", page)
	}
	if page := list("", "parent-a"); page.TotalCount != 4 {
		t.Fatal("default revision history lost")
	}
	if page := list("?view=templates", "parent-b"); page.TotalCount != 0 {
		t.Fatal("household boundary lost")
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+third.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec = requestJSON(t, h, http.MethodGet, "/schedules", "parent-a", "")
	var schedules SchedulePage2
	if err := json.Unmarshal(rec.Body.Bytes(), &schedules); err != nil {
		t.Fatal(err)
	}
	if len(schedules.Items) != 1 || schedules.Items[0].Title != first.Title || schedules.Items[0].StudentName == "" {
		t.Fatalf("schedule names: %s", rec.Body.String())
	}
	rec = requestJSON(t, h, http.MethodGet, "/occurrences", "parent-a", "")
	var occurrences OccurrencePage2
	if err := json.Unmarshal(rec.Body.Bytes(), &occurrences); err != nil {
		t.Fatal(err)
	}
	if len(occurrences.Items) != 1 || occurrences.Items[0].Title != first.Title || occurrences.Items[0].Instructions != first.Instructions || occurrences.Items[0].StudentName == "" {
		t.Fatalf("snapshot names: %s", rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+second.ID+"/retire", "parent-a", ""); rec.Code != 404 {
		t.Fatal("retire accepted revision ID")
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+first.TemplateID+"/retire", "parent-a", ""); rec.Code != 204 {
		t.Fatal(rec.Body.String())
	}
	page = list("?view=templates&q=Latest", "parent-a")
	if len(page.Items) != 1 || page.Items[0].TemplateStatus != "retired" || page.Items[0].Status != "published" {
		t.Fatal("archive must expose template state without rewriting revision history")
	}
	if page := list("?view=templates&status=published", "parent-a"); page.TotalCount != 0 {
		t.Fatal("archived task offered for scheduling")
	}
}
