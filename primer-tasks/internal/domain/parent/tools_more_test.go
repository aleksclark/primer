package parent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestToolsReadWriteSurfaceAndStudentResolutionBranches(t *testing.T) {
	students := &fakeStudents{items: []Student{{ID: "student-1", DisplayName: "Ada"}}}
	tasks := &fakeTasks{task: Task{ID: "task-1", TemplateID: "template-1", Version: 3, Title: "Read", Status: "draft"}}
	schedules := &fakeSchedules{}
	tools := &Tools{Students: students, Tasks: tasks, Schedules: schedules, Occurrences: fakeOccurrences{}}
	ctx := testContext(t, ToolListStudents, ToolListTasks, ToolGetTask, ToolDraftTask, ToolUpdateTask, ToolPublishTask, ToolListSchedules, ToolCreateSchedule, ToolUpdateSchedule, ToolListOccurrences, ToolPreviewAction)

	if _, err := tools.GetTask(nil, ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.GetTask(context.Background(), ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.GetTask(context.Background(), ctx, " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty task=%v", err)
	}
	if _, err := tools.UpdateTask(context.Background(), ctx, TaskUpdateInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty update=%v", err)
	}
	if _, err := tools.UpdateTask(context.Background(), ctx, TaskUpdateInput{TaskID: "task-1", Title: "Changed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.DraftTask(context.Background(), ctx, TaskDraftInput{Title: "Draft"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.PublishTask(context.Background(), ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ListSchedules(context.Background(), ctx, ScheduleQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ListOccurrences(context.Background(), ctx, OccurrenceQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ResolveStudent(context.Background(), ctx, "student-1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ResolveStudent(context.Background(), ctx, "student-1", "Ada"); !errors.Is(err, ErrClarification) {
		t.Fatalf("both student selectors=%v", err)
	}
	if _, err := tools.ResolveStudent(context.Background(), ctx, "", ""); !errors.Is(err, ErrClarification) {
		t.Fatalf("missing student=%v", err)
	}
	if _, err := tools.CreateSchedule(context.Background(), ctx, ScheduleInput{}, ""); !errors.Is(err, ErrClarification) {
		t.Fatalf("missing schedule student=%v", err)
	}
	if _, err := tools.CreateSchedule(context.Background(), ctx, ScheduleInput{}, "Ada"); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.UpdateSchedule(context.Background(), ctx, ScheduleUpdateInput{}, "Ada"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty schedule update=%v", err)
	}
	if _, err := tools.UpdateSchedule(context.Background(), ctx, ScheduleUpdateInput{ScheduleID: "schedule-1"}, "Ada"); err != nil {
		t.Fatal(err)
	}
	if len(students.seen) < 3 || len(tasks.seen) < 4 || len(schedules.seen) < 2 {
		t.Fatalf("service calls students=%d tasks=%d schedules=%d", len(students.seen), len(tasks.seen), len(schedules.seen))
	}

	// Read and write authority are separate from the service implementation.
	readOnly := testContext(t, ToolListStudents)
	if _, err := tools.DraftTask(context.Background(), readOnly, TaskDraftInput{Title: "no"}); !errors.Is(err, ErrToolUnavailable) {
		t.Fatalf("write through read allowlist=%v", err)
	}
	if _, err := tools.ListStudents(context.Background(), Context{}, StudentQuery{}); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("invalid read context=%v", err)
	}
}

func TestToolsDestructivePreviewVariantsAndPayloadTypes(t *testing.T) {
	clock := time.Date(2028, 4, 5, 6, 7, 8, 0, time.UTC)
	store := NewMemoryConfirmationStore()
	store.Now = func() time.Time { return clock }
	tasks := &fakeTasks{task: Task{ID: "task-1", Version: 4, Title: "Read"}}
	schedules := &fakeSchedules{}
	tools := &Tools{Tasks: tasks, Schedules: schedules, Confirmations: store, Now: func() time.Time { return clock }}
	ctx := testContext(t, ToolPreviewAction, ToolConfirmAction, ToolDisableSchedule, ToolBulkScheduleChange)

	preview, err := tools.PreviewDisableSchedule(context.Background(), ctx, "schedule-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: preview.Handle, Action: preview.Action}); err != nil {
		t.Fatal(err)
	}
	if schedules.disabled != 1 {
		t.Fatalf("disabled=%d", schedules.disabled)
	}

	bulk, err := tools.PreviewAction(context.Background(), ctx, BulkScheduleEditAction([]string{"b", "a"}), "Disable two schedules")
	if err != nil {
		t.Fatal(err)
	}
	if len(bulk.Action.TargetIDs) != 2 || bulk.Action.TargetIDs[0] != "a" {
		t.Fatalf("bulk normalization=%+v", bulk.Action)
	}
	if _, err := tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: bulk.Handle, Action: bulk.Action}); err != nil {
		t.Fatal(err)
	}
	if schedules.disabled != 2 {
		t.Fatalf("bulk disabled=%d", schedules.disabled)
	}

	for _, payload := range []map[string]any{
		{"expectedVersion": int(4)}, {"expectedVersion": int64(4)},
		{"expectedVersion": float64(4)}, {"expectedVersion": json.Number("4")},
	} {
		if n, ok := payloadInt(payload, "expectedVersion"); !ok || n != 4 {
			t.Fatalf("payload=%#v n=%d ok=%v", payload, n, ok)
		}
	}
	if _, ok := payloadInt(map[string]any{"expectedVersion": "4"}, "expectedVersion"); ok {
		t.Fatal("string payload accepted")
	}
	if _, err := tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: "missing", Action: RetireTaskAction("task", 1)}); !errors.Is(err, ErrConfirmationNotFound) {
		t.Fatalf("missing confirmation=%v", err)
	}
	if _, err := tools.PreviewAction(context.Background(), ctx, Action{Kind: "read_only", TargetIDs: []string{"x"}}, "bad"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("non-destructive preview=%v", err)
	}
}
