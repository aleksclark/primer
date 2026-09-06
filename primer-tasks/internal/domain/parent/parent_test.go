package parent

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStudents struct {
	items []Student
	seen  []ServiceContext
}

func (f *fakeStudents) ListStudents(_ context.Context, s ServiceContext, _ StudentQuery) ([]Student, error) {
	f.seen = append(f.seen, s)
	return append([]Student(nil), f.items...), nil
}
func (f *fakeStudents) GetStudent(_ context.Context, s ServiceContext, id string) (Student, error) {
	f.seen = append(f.seen, s)
	for _, x := range f.items {
		if x.ID == id {
			return x, nil
		}
	}
	return Student{}, errors.New("not found")
}

type fakeTasks struct {
	task    Task
	retired int
	seen    []ServiceContext
}

func (f *fakeTasks) ListTasks(_ context.Context, s ServiceContext, _ TaskQuery) ([]Task, error) {
	f.seen = append(f.seen, s)
	return []Task{f.task}, nil
}
func (f *fakeTasks) GetTask(_ context.Context, s ServiceContext, _ string) (Task, error) {
	f.seen = append(f.seen, s)
	return f.task, nil
}
func (f *fakeTasks) DraftTask(_ context.Context, s ServiceContext, _ TaskDraftInput) (Task, error) {
	f.seen = append(f.seen, s)
	return f.task, nil
}
func (f *fakeTasks) UpdateTask(_ context.Context, s ServiceContext, _ TaskUpdateInput) (Task, error) {
	f.seen = append(f.seen, s)
	return f.task, nil
}
func (f *fakeTasks) PublishTask(_ context.Context, s ServiceContext, _ string) (Task, error) {
	f.seen = append(f.seen, s)
	return f.task, nil
}
func (f *fakeTasks) RetireTask(_ context.Context, s ServiceContext, id string, version int) (Task, error) {
	f.seen = append(f.seen, s)
	f.retired++
	if id != f.task.ID || version != f.task.Version {
		return Task{}, errors.New("stale")
	}
	return f.task, nil
}

type fakeSchedules struct {
	disabled int
	seen     []ServiceContext
}

func (f *fakeSchedules) ListSchedules(_ context.Context, s ServiceContext, _ ScheduleQuery) ([]Schedule, error) {
	f.seen = append(f.seen, s)
	return nil, nil
}
func (f *fakeSchedules) GetSchedule(_ context.Context, s ServiceContext, id string) (Schedule, error) {
	f.seen = append(f.seen, s)
	return Schedule{ID: id, Version: 3}, nil
}
func (f *fakeSchedules) CreateSchedule(_ context.Context, s ServiceContext, in ScheduleInput) (Schedule, error) {
	f.seen = append(f.seen, s)
	return Schedule{StudentID: in.StudentID}, nil
}
func (f *fakeSchedules) UpdateSchedule(_ context.Context, s ServiceContext, _ ScheduleUpdateInput) (Schedule, error) {
	f.seen = append(f.seen, s)
	return Schedule{}, nil
}
func (f *fakeSchedules) DisableSchedule(_ context.Context, s ServiceContext, _ string, _ int) (Schedule, error) {
	f.seen = append(f.seen, s)
	f.disabled++
	return Schedule{}, nil
}
func (f *fakeSchedules) BulkDisableSchedules(_ context.Context, s ServiceContext, _ []string) ([]Schedule, error) {
	f.seen = append(f.seen, s)
	f.disabled++
	return nil, nil
}

type fakeOccurrences struct{}

func (fakeOccurrences) ListOccurrences(context.Context, ServiceContext, OccurrenceQuery) ([]Occurrence, error) {
	return nil, nil
}

func testContext(t *testing.T, names ...string) Context {
	t.Helper()
	set, err := NewToolSet(names)
	if err != nil {
		t.Fatal(err)
	}
	return Context{TenantID: "tenant-a", ActorID: "parent-a", IdempotencyKey: "message-1", Tools: set}
}

func TestAllowlistAndServerScopeFailClosed(t *testing.T) {
	if _, err := NewToolSet(nil); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("empty allowlist: %v", err)
	}
	if _, err := NewToolSet([]string{"made_up"}); !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("unknown allowlist: %v", err)
	}
	ctx := Context{TenantID: "tenant-a", ActorID: "parent-a", IdempotencyKey: "x"}
	if _, err := (&Tools{}).ListTasks(context.Background(), ctx, TaskQuery{}); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("empty context: %v", err)
	}

	students := &fakeStudents{items: []Student{{ID: "a", DisplayName: "Alex"}}}
	tools := &Tools{Students: students}
	ctx = testContext(t, ToolListStudents)
	ctx.TenantID = "tenant-from-session"
	got, err := tools.ListStudents(context.Background(), ctx, StudentQuery{Name: "tenant-b"})
	if err != nil || len(got) != 1 {
		t.Fatalf("list: %#v %v", got, err)
	}
	if students.seen[0].TenantID != "tenant-from-session" || students.seen[0].ActorID != "parent-a" {
		t.Fatalf("model scope reached service: %#v", students.seen[0])
	}
}

func TestAmbiguousStudentDoesNotMutateSchedule(t *testing.T) {
	students := &fakeStudents{items: []Student{{ID: "a", DisplayName: "Alex"}, {ID: "b", DisplayName: "alex"}}}
	schedules := &fakeSchedules{}
	tools := &Tools{Students: students, Schedules: schedules}
	ctx := testContext(t, ToolCreateSchedule)
	_, err := tools.CreateSchedule(context.Background(), ctx, ScheduleInput{Timezone: "UTC"}, "Alex")
	if !errors.Is(err, ErrClarification) {
		t.Fatalf("error=%v", err)
	}
	if len(schedules.seen) != 0 {
		t.Fatal("ambiguous resolution mutated schedule service")
	}
}

func TestConfirmationIsDigestBoundActorTenantScopedAndSingleUse(t *testing.T) {
	clock := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	store := NewMemoryConfirmationStore()
	store.Now = func() time.Time { return clock }
	tasks := &fakeTasks{task: Task{ID: "task-1", Title: "Read", Version: 7}}
	tools := &Tools{Tasks: tasks, Confirmations: store, Now: func() time.Time { return clock }}
	ctx := testContext(t, ToolPreviewAction, ToolConfirmAction)
	preview, err := tools.PreviewRetireTask(context.Background(), ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Handle == "" || preview.ActionDigest == "" || !preview.ExpiresAt.Equal(clock.Add(5*time.Minute)) {
		t.Fatalf("preview=%+v", preview)
	}
	if tasks.retired != 0 {
		t.Fatal("preview executed task")
	}

	altered := RetireTaskAction("task-1", 8)
	if _, err = tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: preview.Handle, Action: altered}); !errors.Is(err, ErrConfirmationAltered) {
		t.Fatalf("altered=%v", err)
	}
	foreign := ctx
	foreign.ActorID = "parent-b"
	if _, err = tools.ConfirmAction(context.Background(), foreign, ConfirmActionInput{Handle: preview.Handle, Action: preview.Action}); !errors.Is(err, ErrConfirmationForeign) {
		t.Fatalf("foreign=%v", err)
	}
	if _, err = tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: preview.Handle, Action: preview.Action}); err != nil {
		t.Fatal(err)
	}
	if tasks.retired != 1 {
		t.Fatalf("retire calls=%d", tasks.retired)
	}
	if _, err = tools.ConfirmAction(context.Background(), ctx, ConfirmActionInput{Handle: preview.Handle, Action: preview.Action}); !errors.Is(err, ErrConfirmationReplay) {
		t.Fatalf("replay=%v", err)
	}
}

func TestConfirmationExpiry(t *testing.T) {
	clock := time.Now().UTC()
	store := NewMemoryConfirmationStore()
	store.Now = func() time.Time { return clock }
	p, err := store.Issue(context.Background(), ConfirmationPreview{TenantID: "t", ActorID: "a", Action: RetireTaskAction("x", 1), ExpiresAt: clock.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Second)
	if _, err = store.Consume(context.Background(), p.Handle, "t", "a", p.ActionDigest); !errors.Is(err, ErrConfirmationExpired) {
		t.Fatalf("expired=%v", err)
	}
}

func TestProviderModesAndProductionScriptedRejection(t *testing.T) {
	for _, key := range []string{"TASKS_ENV", "TASKS_AGENT_MODE", "TASKS_MODEL_PROVIDER", "TASKS_AGENT_ACTIVE_TOOLS"} {
		t.Setenv(key, "")
	}
	c, err := LoadProviderConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ProviderDisabled || c.Availability() == nil {
		t.Fatalf("disabled config=%+v", c)
	}
	c = ProviderConfig{Environment: "production", Mode: ProviderScripted, MaxSteps: 1, MaxTokens: 1, MaxDuration: time.Second}
	if err := c.Validate(); err == nil {
		t.Fatal("production accepted scripted")
	}
}
