package parent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Tools is the parent command/inspect surface.  It contains service seams, not
// repositories.  A zero-valued Tools is unusable and fails closed.
type Tools struct {
	Students        StudentService
	Tasks           TaskService
	Schedules       ScheduleService
	Occurrences     OccurrenceService
	Confirmations   ConfirmationStore
	Now             func() time.Time
	ConfirmationTTL time.Duration
}

func (t *Tools) now() time.Time {
	if t.Now == nil {
		return time.Now().UTC()
	}
	return t.Now().UTC()
}
func (t *Tools) ttl() time.Duration {
	if t.ConfirmationTTL <= 0 || t.ConfirmationTTL > 15*time.Minute {
		return 5 * time.Minute
	}
	return t.ConfirmationTTL
}
func (t *Tools) read(ctx Context, name string) error {
	if err := ctx.require(name, false); err != nil {
		return err
	}
	return nil
}
func (t *Tools) write(ctx Context, name string) error {
	if err := ctx.require(name, true); err != nil {
		return err
	}
	return nil
}

func (t *Tools) ListStudents(callCtx context.Context, ctx Context, query StudentQuery) ([]Student, error) {
	if err := t.read(ctx, ToolListStudents); err != nil {
		return nil, err
	}
	if t.Students == nil {
		return nil, ErrServiceMissing
	}
	return t.Students.ListStudents(contextOrBackground(callCtx), ctx.serviceContext(), query)
}
func (t *Tools) ListTasks(callCtx context.Context, ctx Context, query TaskQuery) ([]Task, error) {
	if err := t.read(ctx, ToolListTasks); err != nil {
		return nil, err
	}
	if t.Tasks == nil {
		return nil, ErrServiceMissing
	}
	return t.Tasks.ListTasks(contextOrBackground(callCtx), ctx.serviceContext(), query)
}
func (t *Tools) GetTask(callCtx context.Context, ctx Context, id string) (Task, error) {
	if err := t.read(ctx, ToolGetTask); err != nil {
		return Task{}, err
	}
	if t.Tasks == nil {
		return Task{}, ErrServiceMissing
	}
	if strings.TrimSpace(id) == "" {
		return Task{}, ErrInvalidInput
	}
	return t.Tasks.GetTask(contextOrBackground(callCtx), ctx.serviceContext(), strings.TrimSpace(id))
}
func (t *Tools) DraftTask(callCtx context.Context, ctx Context, in TaskDraftInput) (Task, error) {
	if err := t.write(ctx, ToolDraftTask); err != nil {
		return Task{}, err
	}
	if t.Tasks == nil {
		return Task{}, ErrServiceMissing
	}
	return t.Tasks.DraftTask(contextOrBackground(callCtx), ctx.serviceContext(), in)
}
func (t *Tools) UpdateTask(callCtx context.Context, ctx Context, in TaskUpdateInput) (Task, error) {
	if err := t.write(ctx, ToolUpdateTask); err != nil {
		return Task{}, err
	}
	if t.Tasks == nil {
		return Task{}, ErrServiceMissing
	}
	if strings.TrimSpace(in.TaskID) == "" {
		return Task{}, ErrInvalidInput
	}
	return t.Tasks.UpdateTask(contextOrBackground(callCtx), ctx.serviceContext(), in)
}
func (t *Tools) PublishTask(callCtx context.Context, ctx Context, id string) (Task, error) {
	if err := t.write(ctx, ToolPublishTask); err != nil {
		return Task{}, err
	}
	if t.Tasks == nil {
		return Task{}, ErrServiceMissing
	}
	if strings.TrimSpace(id) == "" {
		return Task{}, ErrInvalidInput
	}
	return t.Tasks.PublishTask(contextOrBackground(callCtx), ctx.serviceContext(), strings.TrimSpace(id))
}
func (t *Tools) ListSchedules(callCtx context.Context, ctx Context, query ScheduleQuery) ([]Schedule, error) {
	if err := t.read(ctx, ToolListSchedules); err != nil {
		return nil, err
	}
	if t.Schedules == nil {
		return nil, ErrServiceMissing
	}
	return t.Schedules.ListSchedules(contextOrBackground(callCtx), ctx.serviceContext(), query)
}
func (t *Tools) ListOccurrences(callCtx context.Context, ctx Context, query OccurrenceQuery) ([]Occurrence, error) {
	if err := t.read(ctx, ToolListOccurrences); err != nil {
		return nil, err
	}
	if t.Occurrences == nil {
		return nil, ErrServiceMissing
	}
	return t.Occurrences.ListOccurrences(contextOrBackground(callCtx), ctx.serviceContext(), query)
}

// ResolveStudent is the only name-to-student resolution path used by schedule
// tools.  Zero or multiple exact matches require clarification and never cause
// a mutation.  The list operation is tenant-scoped by the service context.
func (t *Tools) ResolveStudent(callCtx context.Context, ctx Context, id, name string) (Student, error) {
	if err := t.read(ctx, ToolListStudents); err != nil {
		return Student{}, err
	}
	return t.resolveStudent(callCtx, ctx, id, name)
}

func (t *Tools) resolveStudent(callCtx context.Context, ctx Context, id, name string) (Student, error) {
	if t.Students == nil {
		return Student{}, ErrServiceMissing
	}
	id, name = strings.TrimSpace(id), strings.TrimSpace(name)
	if id != "" && name != "" {
		return Student{}, fmt.Errorf("%w: student id and name are both set", ErrClarification)
	}
	if id != "" {
		return t.Students.GetStudent(contextOrBackground(callCtx), ctx.serviceContext(), id)
	}
	if name == "" {
		return Student{}, fmt.Errorf("%w: student is required", ErrClarification)
	}
	students, err := t.Students.ListStudents(contextOrBackground(callCtx), ctx.serviceContext(), StudentQuery{Name: name, Limit: 100})
	if err != nil {
		return Student{}, err
	}
	var matches []Student
	for _, student := range students {
		if strings.EqualFold(strings.TrimSpace(student.DisplayName), name) {
			matches = append(matches, student)
		}
	}
	if len(matches) != 1 {
		return Student{}, fmt.Errorf("%w: choose a student", ErrClarification)
	}
	return matches[0], nil
}

func (t *Tools) CreateSchedule(callCtx context.Context, ctx Context, in ScheduleInput, studentName string) (Schedule, error) {
	if err := t.write(ctx, ToolCreateSchedule); err != nil {
		return Schedule{}, err
	}
	if t.Schedules == nil {
		return Schedule{}, ErrServiceMissing
	}
	student, err := t.resolveStudent(callCtx, ctx, in.StudentID, studentName)
	if err != nil {
		return Schedule{}, err
	}
	in.StudentID = student.ID
	return t.Schedules.CreateSchedule(contextOrBackground(callCtx), ctx.serviceContext(), in)
}
func (t *Tools) UpdateSchedule(callCtx context.Context, ctx Context, in ScheduleUpdateInput, studentName string) (Schedule, error) {
	if err := t.write(ctx, ToolUpdateSchedule); err != nil {
		return Schedule{}, err
	}
	if t.Schedules == nil {
		return Schedule{}, ErrServiceMissing
	}
	if strings.TrimSpace(in.ScheduleID) == "" {
		return Schedule{}, ErrInvalidInput
	}
	student, err := t.resolveStudent(callCtx, ctx, in.StudentID, studentName)
	if err != nil {
		return Schedule{}, err
	}
	in.StudentID = student.ID
	return t.Schedules.UpdateSchedule(contextOrBackground(callCtx), ctx.serviceContext(), in)
}

// PreviewAction issues a durable, actor/tenant/digest-bound handle.  It does
// not execute the action.  The service-specific helpers below additionally
// read the current row to bind its version into the digest.
func (t *Tools) PreviewAction(callCtx context.Context, ctx Context, action Action, summary string) (ConfirmationPreview, error) {
	if err := t.write(ctx, ToolPreviewAction); err != nil {
		return ConfirmationPreview{}, err
	}
	if t.Confirmations == nil {
		return ConfirmationPreview{}, ErrServiceMissing
	}
	action, _, err := normalizeAction(action)
	if err != nil {
		return ConfirmationPreview{}, err
	}
	digest, err := ActionDigest(action)
	if err != nil {
		return ConfirmationPreview{}, err
	}
	p := ConfirmationPreview{
		TenantID: ctx.TenantID, ActorID: ctx.ActorID, Action: action,
		ActionDigest: digest, Summary: strings.TrimSpace(summary),
		ExpiresAt: t.now().Add(t.ttl()),
	}
	return t.Confirmations.Issue(contextOrBackground(callCtx), p)
}
func (t *Tools) PreviewRetireTask(callCtx context.Context, ctx Context, taskID string) (ConfirmationPreview, error) {
	if err := ctx.require(ToolPreviewAction, false); err != nil {
		return ConfirmationPreview{}, err
	}
	if t.Tasks == nil {
		return ConfirmationPreview{}, ErrServiceMissing
	}
	task, err := t.Tasks.GetTask(contextOrBackground(callCtx), ctx.serviceContext(), strings.TrimSpace(taskID))
	if err != nil {
		return ConfirmationPreview{}, err
	}
	return t.PreviewAction(callCtx, ctx, RetireTaskAction(task.ID, task.Version), "Retire task: "+task.Title)
}
func (t *Tools) PreviewDisableSchedule(callCtx context.Context, ctx Context, scheduleID string) (ConfirmationPreview, error) {
	if err := ctx.require(ToolPreviewAction, false); err != nil {
		return ConfirmationPreview{}, err
	}
	if t.Schedules == nil {
		return ConfirmationPreview{}, ErrServiceMissing
	}
	sch, err := t.Schedules.GetSchedule(contextOrBackground(callCtx), ctx.serviceContext(), strings.TrimSpace(scheduleID))
	if err != nil {
		return ConfirmationPreview{}, err
	}
	return t.PreviewAction(callCtx, ctx, DisableScheduleAction(sch.ID, sch.Version), "Disable schedule: "+sch.ID)
}

// ConfirmAction compares the caller's exact action to the durable digest, then
// dispatches the action stored in that durable row.  It never dispatches the
// caller's possibly altered action and never accepts a conversational "yes".
type ConfirmationResult struct {
	Preview   ConfirmationPreview `json:"preview"`
	Task      *Task               `json:"task,omitempty"`
	Schedule  *Schedule           `json:"schedule,omitempty"`
	Schedules []Schedule          `json:"schedules,omitempty"`
}
type ConfirmActionInput struct {
	Handle string
	Action Action
}

func (t *Tools) ConfirmAction(callCtx context.Context, ctx Context, in ConfirmActionInput) (ConfirmationResult, error) {
	if err := t.write(ctx, ToolConfirmAction); err != nil {
		return ConfirmationResult{}, err
	}
	if t.Confirmations == nil {
		return ConfirmationResult{}, ErrServiceMissing
	}
	if strings.TrimSpace(in.Handle) == "" {
		return ConfirmationResult{}, ErrConfirmationRejected
	}
	digest, err := ActionDigest(in.Action)
	if err != nil {
		return ConfirmationResult{}, ErrConfirmationAltered
	}
	p, err := t.Confirmations.Consume(contextOrBackground(callCtx), in.Handle, ctx.TenantID, ctx.ActorID, digest)
	if err != nil {
		return ConfirmationResult{}, err
	}
	return t.executeConfirmed(callCtx, ctx, p)
}
func (t *Tools) executeConfirmed(callCtx context.Context, ctx Context, p ConfirmationPreview) (ConfirmationResult, error) {
	result := ConfirmationResult{Preview: p}
	scope := ctx.serviceContext()
	switch p.Action.Kind {
	case ActionRetireTask:
		if t.Tasks == nil {
			return ConfirmationResult{}, ErrServiceMissing
		}
		version, ok := payloadInt(p.Action.Payload, "expectedVersion")
		if !ok || len(p.Action.TargetIDs) != 1 {
			return ConfirmationResult{}, ErrConfirmationRejected
		}
		task, err := t.Tasks.RetireTask(contextOrBackground(callCtx), scope, p.Action.TargetIDs[0], version)
		if err != nil {
			return ConfirmationResult{}, err
		}
		result.Task = &task
	case ActionDisableSchedule:
		if t.Schedules == nil {
			return ConfirmationResult{}, ErrServiceMissing
		}
		version, ok := payloadInt(p.Action.Payload, "expectedVersion")
		if !ok || len(p.Action.TargetIDs) != 1 {
			return ConfirmationResult{}, ErrConfirmationRejected
		}
		sch, err := t.Schedules.DisableSchedule(contextOrBackground(callCtx), scope, p.Action.TargetIDs[0], version)
		if err != nil {
			return ConfirmationResult{}, err
		}
		result.Schedule = &sch
	case ActionBulkScheduleEdit:
		if t.Schedules == nil {
			return ConfirmationResult{}, ErrServiceMissing
		}
		schedules, err := t.Schedules.BulkDisableSchedules(contextOrBackground(callCtx), scope, append([]string(nil), p.Action.TargetIDs...))
		if err != nil {
			return ConfirmationResult{}, err
		}
		result.Schedules = schedules
	default:
		return ConfirmationResult{}, ErrConfirmationRejected
	}
	return result, nil
}

func payloadInt(payload map[string]any, key string) (int, bool) {
	v, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), int64(int(n)) == n
	case float64:
		return int(n), n == float64(int(n))
	case json.Number:
		i, err := strconv.Atoi(string(n))
		return i, err == nil
	default:
		return 0, false
	}
}
