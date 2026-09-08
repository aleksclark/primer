package parent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func allParentTools(t *testing.T) Context {
	t.Helper()
	return testContext(t, ToolListStudents, ToolListTasks, ToolGetTask, ToolDraftTask, ToolUpdateTask, ToolPublishTask,
		ToolListSchedules, ToolCreateSchedule, ToolUpdateSchedule, ToolListOccurrences, ToolPreviewAction, ToolConfirmAction)
}

func TestParentToolReadAndMutationServiceDelegation(t *testing.T) {
	students := &fakeStudents{items: []Student{{ID: "student-1", DisplayName: "Alex"}}}
	tasks := &fakeTasks{task: Task{ID: "task-1", TemplateID: "template-1", Version: 2, Title: "Read"}}
	schedules := &fakeSchedules{}
	tools := &Tools{Students: students, Tasks: tasks, Schedules: schedules, Occurrences: fakeOccurrences{}}
	ctx := allParentTools(t)
	if _, err := tools.GetTask(context.Background(), ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.DraftTask(context.Background(), ctx, TaskDraftInput{Title: "Draft"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.UpdateTask(context.Background(), ctx, TaskUpdateInput{TaskID: "task-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.PublishTask(context.Background(), ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ListTasks(context.Background(), ctx, TaskQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ListSchedules(context.Background(), ctx, ScheduleQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ListOccurrences(context.Background(), ctx, OccurrenceQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.UpdateSchedule(context.Background(), ctx, ScheduleUpdateInput{ScheduleID: "schedule-1", ScheduleInput: ScheduleInput{StudentID: "student-1"}}, ""); err != nil {
		t.Fatal(err)
	}
	if len(tasks.seen) < 4 || len(schedules.seen) < 2 {
		t.Fatalf("service delegation incomplete tasks=%d schedules=%d", len(tasks.seen), len(schedules.seen))
	}
}

func TestParentStudentResolutionAndConfirmationActions(t *testing.T) {
	clock := time.Date(2027, 3, 4, 5, 6, 7, 0, time.UTC)
	students := &fakeStudents{items: []Student{{ID: "student-1", DisplayName: "Alex"}}}
	schedules := &fakeSchedules{}
	store := NewMemoryConfirmationStore()
	store.Now = func() time.Time { return clock }
	tools := &Tools{Students: students, Schedules: schedules, Confirmations: store, Now: func() time.Time { return clock }}
	ctx := testContext(t, ToolListStudents, ToolCreateSchedule, ToolUpdateSchedule, ToolPreviewAction, ToolConfirmAction)
	if got, err := tools.ResolveStudent(context.Background(), ctx, "student-1", ""); err != nil || got.ID != "student-1" {
		t.Fatalf("id resolution=%+v err=%v", got, err)
	}
	if _, err := tools.ResolveStudent(context.Background(), ctx, "student-1", "Alex"); !errors.Is(err, ErrClarification) {
		t.Fatalf("both selectors=%v", err)
	}
	if _, err := tools.ResolveStudent(context.Background(), ctx, "", "Unknown"); !errors.Is(err, ErrClarification) {
		t.Fatalf("unknown selector=%v", err)
	}
	if _, err := tools.ResolveStudent(context.Background(), ctx, "", ""); !errors.Is(err, ErrClarification) {
		t.Fatalf("empty selector=%v", err)
	}
	students.items = []Student{{ID: "student-1", DisplayName: "Alex"}, {ID: "student-2", DisplayName: "Alex"}}
	if _, err := tools.ResolveStudent(context.Background(), ctx, "", "Alex"); !errors.Is(err, ErrClarification) {
		t.Fatalf("ambiguous selector=%v", err)
	}
	students.items = []Student{{ID: "student-1", DisplayName: "Alex"}}
	if _, err := tools.CreateSchedule(context.Background(), ctx, ScheduleInput{}, "Alex"); err != nil {
		t.Fatalf("name schedule=%v", err)
	}
	if _, err := tools.UpdateSchedule(context.Background(), ctx, ScheduleUpdateInput{ScheduleID: "schedule-1"}, "Alex"); err != nil {
		t.Fatalf("name update=%v", err)
	}
	preview, err := tools.PreviewDisableSchedule(context.Background(), ctx, "schedule-1")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Handle == "" || preview.Action.Kind != ActionDisableSchedule {
		t.Fatalf("disable preview=%+v", preview)
	}
	if _, err = tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: preview.Handle, Action: preview.Action}); err != nil {
		t.Fatal(err)
	}
	if schedules.disabled != 1 {
		t.Fatalf("disabled=%d", schedules.disabled)
	}
	bulk, err := store.Issue(context.Background(), ConfirmationPreview{TenantID: ctx.TenantID, ActorID: ctx.ActorID, Action: BulkScheduleEditAction([]string{"b", "a"}), ExpiresAt: clock.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: bulk.Handle, Action: bulk.Action}); err != nil {
		t.Fatal(err)
	}
	if schedules.disabled != 2 {
		t.Fatalf("bulk disabled=%d", schedules.disabled)
	}
}

func TestNewToolSetRejectsUnknownAndBlankNames(t *testing.T) {
	if _, err := NewToolSet([]string{" "}); err == nil {
		t.Fatal("blank tool name accepted")
	}
	if _, err := NewToolSet([]string{"not-a-tool"}); err == nil {
		t.Fatal("unknown tool accepted")
	}
}

func TestExecuteConfirmedRejectsUnknownKind(t *testing.T) {
	ctx := testContext(t, ToolConfirmAction)
	tools := &Tools{Confirmations: NewMemoryConfirmationStore()}
	if _, err := tools.executeConfirmed(context.Background(), ctx, ConfirmationPreview{Action: Action{Kind: "unknown", TargetIDs: []string{"x"}}}); err == nil {
		t.Fatal("unknown confirmed action accepted")
	}
}

func TestNormalizeActionAndEmptyHandleFailClosed(t *testing.T) {
	if _, _, err := normalizeAction(Action{Kind: ActionDisableSchedule}); err == nil {
		t.Fatal("empty targets accepted")
	}
	if _, _, err := normalizeAction(Action{Kind: ActionDisableSchedule, TargetIDs: []string{" "}}); err == nil {
		t.Fatal("blank target accepted")
	}
	ctx := testContext(t, ToolConfirmAction)
	tools := &Tools{Confirmations: NewMemoryConfirmationStore()}
	if _, err := tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: "   "}); err == nil {
		t.Fatal("blank handle accepted")
	}
}

func TestParentToolsRequireAllowlistedOperations(t *testing.T) {
	ctx := testContext(t, ToolListStudents)
	tools := &Tools{Students: &fakeStudents{}, Tasks: &fakeTasks{}, Schedules: &fakeSchedules{}, Occurrences: fakeOccurrences{}, Confirmations: NewMemoryConfirmationStore()}
	if _, err := tools.ListTasks(context.Background(), ctx, TaskQuery{}); err == nil {
		t.Fatal("list tasks without allowlist accepted")
	}
	if _, err := tools.ListSchedules(context.Background(), ctx, ScheduleQuery{}); err == nil {
		t.Fatal("list schedules without allowlist accepted")
	}
	if _, err := tools.ListOccurrences(context.Background(), ctx, OccurrenceQuery{}); err == nil {
		t.Fatal("list occurrences without allowlist accepted")
	}
	if _, err := tools.PublishTask(context.Background(), ctx, "task"); err == nil {
		t.Fatal("publish without allowlist accepted")
	}
	if _, err := tools.CreateSchedule(context.Background(), ctx, ScheduleInput{}, "Ada"); err == nil {
		t.Fatal("create schedule without allowlist accepted")
	}
	if _, err := tools.UpdateSchedule(context.Background(), ctx, ScheduleUpdateInput{ScheduleID: "s"}, "Ada"); err == nil {
		t.Fatal("update schedule without allowlist accepted")
	}
	if _, err := tools.PreviewAction(context.Background(), ctx, Action{}, ""); err == nil {
		t.Fatal("preview without allowlist accepted")
	}
	if _, err := tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: "h"}); err == nil {
		t.Fatal("confirm without allowlist accepted")
	}
}

func TestParentToolsFailClosedOnMissingServicesAndInvalidInputs(t *testing.T) {
	ctx := testContext(t, ToolListStudents, ToolListTasks, ToolGetTask, ToolDraftTask, ToolUpdateTask, ToolPublishTask, ToolListSchedules, ToolListOccurrences, ToolCreateSchedule, ToolUpdateSchedule, ToolPreviewAction, ToolConfirmAction)
	empty := &Tools{}
	if _, err := empty.ListTasks(context.Background(), ctx, TaskQuery{}); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("list tasks=%v", err)
	}
	if _, err := empty.GetTask(context.Background(), ctx, "task"); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("get task=%v", err)
	}
	if _, err := empty.DraftTask(context.Background(), ctx, TaskDraftInput{Title: "x"}); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("draft=%v", err)
	}
	if _, err := empty.UpdateTask(context.Background(), ctx, TaskUpdateInput{TaskID: "task"}); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("update=%v", err)
	}
	if _, err := empty.PublishTask(context.Background(), ctx, "task"); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("publish=%v", err)
	}
	if _, err := empty.ListSchedules(context.Background(), ctx, ScheduleQuery{}); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("schedules=%v", err)
	}
	if _, err := empty.ListOccurrences(context.Background(), ctx, OccurrenceQuery{}); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("occurrences=%v", err)
	}
	withTasks := &Tools{Tasks: &fakeTasks{}}
	if _, err := withTasks.GetTask(context.Background(), ctx, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty get=%v", err)
	}
	if _, err := withTasks.UpdateTask(context.Background(), ctx, TaskUpdateInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty update=%v", err)
	}
	if _, err := withTasks.PublishTask(context.Background(), ctx, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty publish=%v", err)
	}
	if _, err := (&Tools{Confirmations: NewMemoryConfirmationStore()}).PreviewAction(context.Background(), ctx, Action{}, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid preview=%v", err)
	}
	if _, err := empty.ResolveStudent(context.Background(), ctx, "", "Ada"); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("resolve=%v", err)
	}
	if _, err := empty.CreateSchedule(context.Background(), ctx, ScheduleInput{}, "Ada"); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("create schedule=%v", err)
	}
	if _, err := empty.UpdateSchedule(context.Background(), ctx, ScheduleUpdateInput{ScheduleID: "s"}, "Ada"); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("update schedule=%v", err)
	}
	if _, err := empty.PreviewRetireTask(context.Background(), ctx, "task"); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("preview retire=%v", err)
	}
	if _, err := empty.PreviewDisableSchedule(context.Background(), ctx, "schedule"); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("preview disable=%v", err)
	}
	if _, err := empty.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: "h"}); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("confirm=%v", err)
	}
	if _, err := (&Tools{Schedules: &fakeSchedules{}}).UpdateSchedule(context.Background(), ctx, ScheduleUpdateInput{}, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty schedule id=%v", err)
	}
}

func TestParentContextAndProviderValidationBranches(t *testing.T) {
	if _, err := NewContext("", "actor", "id", []string{ToolListStudents}); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("empty tenant=%v", err)
	}
	if _, err := NewContext("tenant", "actor", "", []string{ToolDraftTask}); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("empty idempotency=%v", err)
	}
	if err := (Context{TenantID: "t", ActorID: "a", IdempotencyKey: "i"}).Validate(false); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("empty tools=%v", err)
	}
	for _, cfg := range []ProviderConfig{
		{Environment: "bad", Mode: ProviderDisabled, MaxSteps: 1, MaxTokens: 1, MaxDuration: time.Second, MaxConcurrentTenants: 1},
		{Environment: "production", Mode: ProviderBedrock, Primary: ProviderScripted, MaxSteps: 1, MaxTokens: 1, MaxDuration: time.Second, MaxConcurrentTenants: 1, ActiveTools: []string{ToolListStudents}},
		{Environment: "test", Mode: ProviderBedrock, Primary: ProviderBedrock, Fallback: ProviderBedrock, MaxSteps: 1, MaxTokens: 1, MaxDuration: time.Second, MaxConcurrentTenants: 1, ActiveTools: []string{ToolListStudents}},
		{Environment: "test", Mode: ProviderBedrock, Primary: ProviderBedrock, Fallback: ProviderOpenRouter, OpenRouterBaseURL: "not-url", MaxSteps: 1, MaxTokens: 1, MaxDuration: time.Second, MaxConcurrentTenants: 1, ActiveTools: []string{ToolListStudents}},
	} {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid provider accepted: %+v", cfg)
		}
	}
	if (ProviderUnavailable{Mode: ProviderDisabled}).Error() == "" {
		t.Fatal("provider unavailable lost error")
	}
}
