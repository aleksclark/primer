package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"primer-tasks/internal/domain/parent"
	"primer-tasks/internal/schedule"
)

func TestPhase3ServiceAdaptersCoverTaskScheduleAndOccurrencePaths(t *testing.T) {
	pool := integrationPool(t)
	alice, _ := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("phase3-services"), IssuerSecret: []byte("phase3-services")})
	services := phase3Services{s: s}
	ctx := context.Background()
	scope := parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a", IdempotencyKey: "services-test"}

	student, err := services.GetStudent(ctx, scope, alice)
	if err != nil || student.ID != alice {
		t.Fatalf("student=%+v err=%v", student, err)
	}
	if _, err = services.GetStudent(ctx, parent.ServiceContext{TenantID: tenantB}, alice); !errors.Is(err, parent.ErrServiceMissing) {
		t.Fatalf("foreign student=%v", err)
	}
	students, err := services.ListStudents(ctx, scope, parent.StudentQuery{Limit: 1, Offset: -1})
	if err != nil || len(students) != 1 {
		t.Fatalf("student page=%+v err=%v", students, err)
	}

	draft, err := services.DraftTask(ctx, scope, parent.TaskDraftInput{Title: "Service adapter task", Instructions: "Use every service path"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = services.DraftTask(ctx, scope, parent.TaskDraftInput{}); !errors.Is(err, parent.ErrInvalidInput) {
		t.Fatalf("invalid draft=%v", err)
	}
	if _, err = services.UpdateTask(ctx, scope, parent.TaskUpdateInput{TaskID: draft.ID}); err == nil {
		t.Fatal("update task unexpectedly implemented")
	}
	task, err := services.GetTask(ctx, scope, draft.ID)
	if err != nil || task.Status != "draft" {
		t.Fatalf("draft task=%+v err=%v", task, err)
	}
	if _, err = services.GetTask(ctx, parent.ServiceContext{TenantID: tenantB}, draft.ID); !errors.Is(err, parent.ErrServiceMissing) {
		t.Fatalf("foreign task=%v", err)
	}
	if tasks, err := services.ListTasks(ctx, scope, parent.TaskQuery{Query: "Service adapter", Limit: 1, Offset: -1}); err != nil || len(tasks) != 1 {
		t.Fatalf("task page=%+v err=%v", tasks, err)
	}
	published, err := services.PublishTask(ctx, scope, draft.ID)
	if err != nil || published.Status != "published" {
		t.Fatalf("publish=%+v err=%v", published, err)
	}
	if _, err = services.PublishTask(ctx, scope, draft.ID); err == nil {
		t.Fatal("republish unexpectedly succeeded")
	}

	start := time.Now().UTC().Add(90 * time.Minute)
	input := parent.ScheduleInput{StudentID: alice, TemplateID: published.TemplateID, RevisionID: published.ID, Kind: "one_off", Timezone: "UTC", StartAt: start, DueOffsetMinutes: 3}
	sch, err := services.CreateSchedule(ctx, scope, input)
	if err != nil {
		t.Fatal(err)
	}
	if sch.ID == "" || sch.Version != 1 {
		t.Fatalf("created schedule=%+v", sch)
	}
	if got, err := services.GetSchedule(ctx, scope, sch.ID); err != nil || got.ID != sch.ID {
		t.Fatalf("get schedule=%+v err=%v", got, err)
	}
	if schedules, err := services.ListSchedules(ctx, scope, parent.ScheduleQuery{Limit: 10}); err != nil || len(schedules) == 0 {
		t.Fatalf("schedule list=%+v err=%v", schedules, err)
	}
	updated, err := services.UpdateSchedule(ctx, scope, parent.ScheduleUpdateInput{ScheduleID: sch.ID, ExpectedVersion: sch.Version, ScheduleInput: parent.ScheduleInput{StudentID: alice, TemplateID: published.TemplateID, RevisionID: published.ID, Kind: "one_off", Timezone: "America/Chicago", StartAt: start.Add(time.Hour), DueOffsetMinutes: 7}})
	if err != nil || updated.Version != 2 || updated.Timezone != "America/Chicago" {
		t.Fatalf("updated schedule=%+v err=%v", updated, err)
	}

	// Materialization reads the service-owned schedule snapshot and then the
	// adapter exposes the resulting occurrence projection.
	worker := schedule.NewWorker(pool)
	if err := worker.Materialize(ctx); err != nil {
		t.Fatal(err)
	}
	occurrences, err := services.ListOccurrences(ctx, scope, parent.OccurrenceQuery{Limit: 20})
	if err != nil || len(occurrences) == 0 {
		t.Fatalf("occurrences=%+v err=%v", occurrences, err)
	}

	disabled, err := services.DisableSchedule(ctx, scope, sch.ID, updated.Version)
	if err != nil || disabled.Enabled {
		t.Fatalf("disabled=%+v err=%v", disabled, err)
	}
	staleResult, staleErr := services.DisableSchedule(ctx, scope, sch.ID, updated.Version)
	if staleErr == nil || staleResult.ID != "" {
		t.Fatalf("stale disable was accepted: schedule=%+v err=%v", staleResult, staleErr)
	}
	second, err := services.CreateSchedule(ctx, scope, input)
	if err != nil {
		t.Fatal(err)
	}
	bulk, err := services.BulkDisableSchedules(ctx, scope, []string{second.ID})
	if !errors.Is(err, parent.ErrConfirmationRejected) || len(bulk) != 0 {
		t.Fatalf("versionless bulk should fail closed: %+v %v", bulk, err)
	}
	if _, err = services.DisableSchedule(ctx, scope, second.ID, second.Version); err != nil {
		t.Fatal(err)
	}
	if schedules, err := services.ListSchedules(ctx, scope, parent.ScheduleQuery{IncludeDisabled: false, Limit: 20}); err != nil || len(schedules) != 0 {
		t.Fatalf("enabled schedules=%+v err=%v", schedules, err)
	}
	if schedules, err := services.ListSchedules(ctx, scope, parent.ScheduleQuery{IncludeDisabled: true, Limit: 20}); err != nil || len(schedules) < 2 {
		t.Fatalf("all schedules=%+v err=%v", schedules, err)
	}
	if _, err = services.GetSchedule(ctx, parent.ServiceContext{TenantID: tenantB}, sch.ID); !errors.Is(err, parent.ErrServiceMissing) {
		t.Fatalf("foreign schedule=%v", err)
	}
}
