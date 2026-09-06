package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

// This fixture is also checked against the actual browser preset compiler.
// These are not fabricated UI counts: each output goes through the public
// schedule endpoint and the production PostgreSQL materializer.
func TestBrowserPresetsMaterializeExpectedCounts(t *testing.T) {
	t.Setenv("TASKS_SCHEDULE_HORIZON_DAYS", "730")
	data, err := os.ReadFile("../../web/src/schedule-preset-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name     string `json:"name"`
		Settings struct {
			Timezone string `json:"timezone"`
		} `json:"settings"`
		Expected    []ScheduleInput2 `json:"expected"`
		Occurrences int              `json:"occurrences"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("presets"), IssuerSecret: []byte("presets")})
	h := s.Routes()
	rec := requestJSON(t, h, http.MethodPost, "/tasks", "parent-a", `{"title":"Preset task","instructions":"Check carefully"}`)
	var task TaskRevision
	if rec.Code != 201 {
		t.Fatal(rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/tasks/"+task.ID+"/publish", "parent-a", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			total := 0
			for _, input := range fixture.Expected {
				input.StudentID = alice
				input.TemplateID = task.TemplateID
				input.RevisionID = task.ID
				input.Timezone = fixture.Settings.Timezone
				body, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				rec := requestJSON(t, h, http.MethodPost, "/schedules", "parent-a", string(body))
				if rec.Code != 201 {
					t.Fatal(rec.Body.String())
				}
				var saved Schedule2
				if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
					t.Fatal(err)
				}
				var count int
				if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM task_occurrences WHERE schedule_id=$1`, saved.ID).Scan(&count); err != nil {
					t.Fatal(err)
				}
				total += count
			}
			if total != fixture.Occurrences {
				t.Fatalf("materialized %d; want %d", total, fixture.Occurrences)
			}
		})
	}
}
