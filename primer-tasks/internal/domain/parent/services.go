package parent

import (
	"context"
	"time"
)

// These are service seams, not repositories.  The phase-2 service adapters own
// tenant predicates, validation, transactions, and idempotency.  Parent tools
// never receive a database handle and cannot bypass those rules.
type Student struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Task struct {
	ID           string           `json:"id"`
	TemplateID   string           `json:"templateId"`
	Version      int              `json:"version"`
	Title        string           `json:"title"`
	Instructions string           `json:"instructions"`
	Status       string           `json:"status"`
	Requirements []map[string]any `json:"requirements,omitempty"`
}

type Schedule struct {
	ID               string     `json:"id"`
	StudentID        string     `json:"studentId"`
	TemplateID       string     `json:"templateId"`
	RevisionID       string     `json:"revisionId"`
	Kind             string     `json:"kind"`
	Timezone         string     `json:"timezone"`
	StartAt          time.Time  `json:"startAt"`
	EndAt            *time.Time `json:"endAt,omitempty"`
	RRULE            string     `json:"rrule,omitempty"`
	DueOffsetMinutes int        `json:"dueOffsetMinutes"`
	Enabled          bool       `json:"enabled"`
	Version          int        `json:"version"`
}

type Occurrence struct {
	ID         string    `json:"id"`
	StudentID  string    `json:"studentId"`
	ScheduleID string    `json:"scheduleId"`
	RevisionID string    `json:"revisionId"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	NominalAt  time.Time `json:"nominalAt"`
	DueAt      time.Time `json:"dueAt"`
}

type StudentQuery struct {
	Name   string
	Limit  int
	Offset int
}
type TaskQuery struct {
	Query  string
	Limit  int
	Offset int
}
type ScheduleQuery struct {
	IncludeDisabled bool
	Limit           int
	Offset          int
}
type OccurrenceQuery struct {
	Status string
	Limit  int
	Offset int
}

type TaskDraftInput struct {
	Title        string
	Instructions string
	Requirements []map[string]any
}
type TaskUpdateInput struct {
	ExpectedVersion int
	TaskID          string
	Title           string
	Instructions    string
	Requirements    []map[string]any
}
type ScheduleInput struct {
	StudentID        string     `json:"studentId"`
	TemplateID       string     `json:"templateId"`
	RevisionID       string     `json:"revisionId"`
	Kind             string     `json:"kind"`
	Timezone         string     `json:"timezone"`
	StartAt          time.Time  `json:"startAt"`
	EndAt            *time.Time `json:"endAt,omitempty"`
	RRULE            string     `json:"rrule"`
	DueOffsetMinutes int        `json:"dueOffsetMinutes"`
}
type ScheduleUpdateInput struct {
	ExpectedVersion int    `json:"expectedVersion,omitempty"`
	ScheduleID      string `json:"scheduleId"`
	ScheduleInput
}

type StudentService interface {
	ListStudents(context.Context, ServiceContext, StudentQuery) ([]Student, error)
	GetStudent(context.Context, ServiceContext, string) (Student, error)
}
type TaskService interface {
	ListTasks(context.Context, ServiceContext, TaskQuery) ([]Task, error)
	GetTask(context.Context, ServiceContext, string) (Task, error)
	DraftTask(context.Context, ServiceContext, TaskDraftInput) (Task, error)
	UpdateTask(context.Context, ServiceContext, TaskUpdateInput) (Task, error)
	PublishTask(context.Context, ServiceContext, string) (Task, error)
	RetireTask(context.Context, ServiceContext, string, int) (Task, error)
}
type ScheduleService interface {
	ListSchedules(context.Context, ServiceContext, ScheduleQuery) ([]Schedule, error)
	GetSchedule(context.Context, ServiceContext, string) (Schedule, error)
	CreateSchedule(context.Context, ServiceContext, ScheduleInput) (Schedule, error)
	UpdateSchedule(context.Context, ServiceContext, ScheduleUpdateInput) (Schedule, error)
	DisableSchedule(context.Context, ServiceContext, string, int) (Schedule, error)
	BulkDisableSchedules(context.Context, ServiceContext, []string) ([]Schedule, error)
}
type OccurrenceService interface {
	ListOccurrences(context.Context, ServiceContext, OccurrenceQuery) ([]Occurrence, error)
}
