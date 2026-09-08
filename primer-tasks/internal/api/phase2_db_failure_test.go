package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/schedule"
	"primer-tasks/internal/verification"
)

func uuidMust(s string) uuid.UUID { return uuid.MustParse(s) }

func TestPhase2HandlersSurfaceDatabaseFailures(t *testing.T) {
	pool := integrationPool(t)
	pool.Close()
	if err := schedule.NewWorker(pool).Materialize(context.Background()); err == nil {
		t.Fatal("worker accepted closed DB")
	}
	t.Setenv("TASKS_SCHEDULE_HORIZON_DAYS", "365")
	if got := schedule.NewWorker(pool).Horizon; got != 365*24*time.Hour {
		t.Fatalf("horizon=%s", got)
	}
	t.Setenv("TASKS_SCHEDULE_HORIZON_DAYS", "bad")
	if got := schedule.NewWorker(pool).Horizon; got != 45*24*time.Hour {
		t.Fatalf("default horizon=%s", got)
	}
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
	run("submit", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.submitOccurrence2(w, r, uuidMust(tenantA))
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
	run("students", s.listStudents)
	run("create-student", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.createStudent(w, req(http.MethodPost, "/students", `{"displayName":"Ada"}`), sc)
	})
	run("archive-student", s.archiveStudent)
	run("pairing", s.issuePairing)
	run("dialogue-inspect", s.dialogueInspect)
	run("dialogue-override", s.dialogueOverride)
	run("agent-conversation", s.createAgentConversation)
	run("dialogue-start", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.studentDialogueStart(w, req(http.MethodPost, "/student/occurrences/"+tenantA+"/dialogue", `{}`))
	})
	run("dialogue-state", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.studentDialogueState(w, req(http.MethodGet, "/student/occurrences/"+tenantA+"/dialogue", ""))
	})
	run("update-student", s.updateStudent)
	run("student-profile", func(w http.ResponseWriter, r *http.Request, sc scope) {
		s.studentProfile(w, r, uuidMust(tenantA))
	})
	engine := verification.DialogueEngine{DB: pool}
	auth := verification.StudentAuthority{TenantID: tenantA, StudentID: tenantA, SessionID: tenantA}
	ctx := context.Background()
	if _, err := engine.Start(ctx, auth, tenantA, ""); err == nil {
		t.Fatal("start accepted closed DB")
	}
	if _, err := engine.Admit(ctx, auth, tenantA, tenantA, verification.DialogueMessage{}); err == nil {
		t.Fatal("admit accepted closed DB")
	}
	if err := engine.CommitQuestion(ctx, auth, tenantA, tenantA, verification.DialogueLease{JobID: tenantA}, "wall"); err == nil {
		t.Fatal("commit question accepted closed DB")
	}
	if err := engine.CommitEvaluation(ctx, auth, tenantA, tenantA, verification.DialogueLease{JobID: tenantA}, verification.DialogueEvaluation{}); err == nil {
		t.Fatal("commit evaluation accepted closed DB")
	}
	if err := engine.Retry(ctx, auth, tenantA, tenantA, 1); err == nil {
		t.Fatal("retry accepted closed DB")
	}
	if err := engine.Progress(ctx, auth, tenantA, tenantA, verification.DialogueLease{JobID: tenantA}, "thinking"); err == nil {
		t.Fatal("progress accepted closed DB")
	}
	if err := engine.ReconcileDialogueJobs(ctx); err == nil {
		t.Fatal("reconcile accepted closed DB")
	}
	if _, err := verification.ResolveStudentAuthority(ctx, pool, make([]byte, 32)); err == nil {
		t.Fatal("resolve accepted closed DB")
	}
	if err := repo.NewDialogueRepository(pool).PublishRevisionPolicies(ctx, tenantA, tenantA); err == nil {
		t.Fatal("publish policies accepted closed DB")
	}
	if _, err := repo.NewDialogueRepository(pool).RevisionPolicy(ctx, tenantA, tenantA, tenantA); err == nil {
		t.Fatal("revision policy accepted closed DB")
	}
	adapters := phase3Services{s: s}
	scope := parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a", IdempotencyKey: "closed-db"}
	if _, err := adapters.ListStudents(ctx, scope, parent.StudentQuery{}); err == nil {
		t.Fatal("phase3 list students accepted closed DB")
	}
	if _, err := adapters.GetStudent(ctx, scope, tenantA); err == nil {
		t.Fatal("phase3 get student accepted closed DB")
	}
	if _, err := adapters.ListTasks(ctx, scope, parent.TaskQuery{}); err == nil {
		t.Fatal("phase3 list tasks accepted closed DB")
	}
	if _, err := adapters.GetTask(ctx, scope, tenantA); err == nil {
		t.Fatal("phase3 get task accepted closed DB")
	}
	if _, err := adapters.ListSchedules(ctx, scope, parent.ScheduleQuery{}); err == nil {
		t.Fatal("phase3 list schedules accepted closed DB")
	}
	if _, err := adapters.GetSchedule(ctx, scope, tenantA); err == nil {
		t.Fatal("phase3 get schedule accepted closed DB")
	}
	if _, err := adapters.ListOccurrences(ctx, scope, parent.OccurrenceQuery{}); err == nil {
		t.Fatal("phase3 list occurrences accepted closed DB")
	}
	if _, err := adapters.DraftTask(ctx, scope, parent.TaskDraftInput{Title: "x", Instructions: "y"}); err == nil {
		t.Fatal("phase3 draft accepted closed DB")
	}
	if _, _, _, err := engine.WorkerState(ctx, auth, tenantA, tenantA, verification.DialogueLease{JobID: tenantA}); err == nil {
		t.Fatal("worker state accepted closed DB")
	}
	if err := engine.FailDialogueJob(ctx, verification.DialogueJobReference{JobID: tenantA, TenantID: tenantA, Owner: "x", Generation: 1}, "provider_unavailable"); err == nil {
		t.Fatal("fail job accepted closed DB")
	}
	tx, err := pool.Begin(ctx)
	if err == nil {
		_, _ = verification.OverrideDialogue(ctx, tx, tenantA, "parent-a", tenantA, tenantA, "req", "Parent independently reviewed the attempt.", 1, false)
		_ = tx.Rollback(ctx)
	}
}
