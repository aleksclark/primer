package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/domain/parent"
)

func TestPhase3DomainAdaptersCoverScheduleAndRevisionBoundaries(t *testing.T) {
	pool := integrationPool(t)
	student, _ := seedIntegration(t, pool)
	ctx := context.Background()
	s := New(pool, "test")
	adapters := phase3Services{s: s}
	scope := parent.ServiceContext{TenantID: tenantA, ActorID: "parent-a", IdempotencyKey: "adapter-test"}
	students, err := adapters.ListStudents(ctx, scope, parent.StudentQuery{Limit: 1, Offset: -1})
	if err != nil || len(students) != 1 || students[0].ID != student {
		t.Fatalf("students=%+v err=%v", students, err)
	}
	if _, err = adapters.GetStudent(ctx, scope, uuid.NewString()); !errors.Is(err, parent.ErrServiceMissing) {
		t.Fatalf("missing student=%v", err)
	}
	if _, err = adapters.DraftTask(ctx, scope, parent.TaskDraftInput{}); !errors.Is(err, parent.ErrInvalidInput) {
		t.Fatalf("empty draft=%v", err)
	}
	draft, err := adapters.DraftTask(ctx, scope, parent.TaskDraftInput{Title: "Adapter task", Instructions: "Do it"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapters.UpdateTask(ctx, scope, parent.TaskUpdateInput{TaskID: draft.ID}); err == nil {
		t.Fatal("phase 3 update unexpectedly bypassed revision boundary")
	}
	if _, err = adapters.GetTask(ctx, scope, uuid.NewString()); !errors.Is(err, parent.ErrServiceMissing) {
		t.Fatalf("missing task=%v", err)
	}
	published, err := adapters.PublishTask(ctx, scope, draft.ID)
	if err != nil || published.Status != "published" {
		t.Fatalf("published=%+v err=%v", published, err)
	}
	if _, err = adapters.PublishTask(ctx, scope, draft.ID); err == nil {
		t.Fatal("published task republished")
	}
	if _, err = adapters.ListTasks(ctx, scope, parent.TaskQuery{Limit: 0, Offset: -2}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	sch, err := adapters.CreateSchedule(ctx, scope, parent.ScheduleInput{StudentID: student, TemplateID: published.TemplateID, RevisionID: published.ID, Kind: "one_off", Timezone: "UTC", StartAt: start})
	if err != nil {
		t.Fatal(err)
	}
	if sch.ID == "" {
		t.Fatal("schedule id missing")
	}
	if got, err := adapters.GetSchedule(ctx, scope, sch.ID); err != nil || got.ID != sch.ID {
		t.Fatalf("get schedule=%+v err=%v", got, err)
	}
	if _, err = adapters.UpdateSchedule(ctx, scope, parent.ScheduleUpdateInput{ScheduleID: sch.ID, ExpectedVersion: sch.Version, ScheduleInput: parent.ScheduleInput{StudentID: student, TemplateID: published.TemplateID, RevisionID: published.ID, Kind: "one_off", Timezone: "America/Chicago", StartAt: start, RRULE: ""}}); err != nil {
		t.Fatal(err)
	}
	updated, err := adapters.GetSchedule(ctx, scope, sch.ID)
	if err != nil || updated.Timezone != "America/Chicago" || updated.Version != sch.Version+1 {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if _, err = adapters.DisableSchedule(ctx, scope, sch.ID, sch.Version); err == nil {
		t.Fatal("stale schedule version accepted")
	}
	disabled, err := adapters.DisableSchedule(ctx, scope, sch.ID, updated.Version)
	if err != nil || disabled.Enabled {
		t.Fatalf("disabled=%+v err=%v", disabled, err)
	}
	if _, err = adapters.ListSchedules(ctx, scope, parent.ScheduleQuery{IncludeDisabled: false, Limit: 0, Offset: -1}); err != nil {
		t.Fatal(err)
	}
	if _, err = adapters.ListOccurrences(ctx, scope, parent.OccurrenceQuery{Limit: 0, Offset: -1}); err != nil {
		t.Fatal(err)
	}
	if _, err = adapters.BulkDisableSchedules(ctx, scope, []string{sch.ID}); err == nil {
		t.Fatal("bulk disabled an already disabled schedule")
	}
}
