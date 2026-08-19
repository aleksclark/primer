// Package parent contains the narrow, server-authorized tools exposed to a
// parent's agent.  It deliberately depends on service interfaces rather than
// the phase-2 SQL implementation.  The caller must construct Context from an
// authenticated server session; values in a model request are never used as
// authority.
package parent

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidContext        = errors.New("invalid parent tool context")
	ErrToolUnavailable       = errors.New("parent tool is not available")
	ErrUnknownTool           = errors.New("unknown parent tool")
	ErrClarification         = errors.New("clarification required")
	ErrClarificationRequired = ErrClarification
	ErrInvalidInput          = errors.New("invalid parent tool input")
	ErrServiceMissing        = errors.New("parent tool service is unavailable")
)

// Tool names are intentionally constants.  A model may select only a name in
// this list and only when the server has put that name in the active allowlist.
const (
	ToolListStudents       = "list_students"
	ToolListTasks          = "list_tasks"
	ToolGetTask            = "get_task"
	ToolDraftTask          = "draft_task"
	ToolUpdateTask         = "update_task"
	ToolPublishTask        = "publish_task"
	ToolPreviewAction      = "preview_action"
	ToolConfirmAction      = "confirm_action"
	ToolRetireTask         = "retire_task"
	ToolDisableSchedule    = "disable_schedule"
	ToolBulkScheduleChange = "bulk_schedule_change"
	ToolListSchedules      = "list_schedules"
	ToolCreateSchedule     = "create_schedule"
	ToolUpdateSchedule     = "update_schedule"
	ToolListOccurrences    = "list_occurrences"
)

var knownTools = map[string]struct{}{
	ToolListStudents: {}, ToolListTasks: {}, ToolGetTask: {}, ToolDraftTask: {},
	ToolUpdateTask: {}, ToolPublishTask: {}, ToolPreviewAction: {},
	ToolConfirmAction: {}, ToolRetireTask: {}, ToolDisableSchedule: {},
	ToolBulkScheduleChange: {}, ToolListSchedules: {}, ToolCreateSchedule: {},
	ToolUpdateSchedule: {}, ToolListOccurrences: {},
}

// ToolSet is a server-owned active tool allowlist.  There is no permissive
// default: an empty or unknown allowlist is an error.
type ToolSet struct{ active map[string]struct{} }

func NewToolSet(names []string) (ToolSet, error) {
	if len(names) == 0 {
		return ToolSet{}, fmt.Errorf("%w: empty active tool allowlist", ErrInvalidContext)
	}
	out := ToolSet{active: make(map[string]struct{}, len(names))}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if _, ok := knownTools[name]; !ok {
			return ToolSet{}, fmt.Errorf("%w: %q", ErrUnknownTool, name)
		}
		out.active[name] = struct{}{}
	}
	if len(out.active) == 0 {
		return ToolSet{}, fmt.Errorf("%w: empty active tool allowlist", ErrInvalidContext)
	}
	return out, nil
}

func (s ToolSet) Allows(name string) bool {
	_, ok := s.active[name]
	return ok
}

// Context contains only values derived by the authenticated server and the
// server-issued idempotency key.  It has no tenant, actor, role, or authority
// fields that can be populated by model tool input.
type Context struct {
	TenantID       string
	ActorID        string
	IdempotencyKey string
	Tools          ToolSet
}

// NewContext is the server adapter's construction boundary.  Model request
// structs should never be used as arguments to this function.
func NewContext(tenantID, actorID, idempotencyKey string, activeTools []string) (Context, error) {
	tools, err := NewToolSet(activeTools)
	if err != nil {
		return Context{}, err
	}
	out := Context{TenantID: strings.TrimSpace(tenantID), ActorID: strings.TrimSpace(actorID), IdempotencyKey: strings.TrimSpace(idempotencyKey), Tools: tools}
	if err := out.Validate(true); err != nil {
		return Context{}, err
	}
	return out, nil
}

func (c Context) Validate(mutation bool) error {
	if strings.TrimSpace(c.TenantID) == "" || strings.TrimSpace(c.ActorID) == "" {
		return fmt.Errorf("%w: tenant and actor are required", ErrInvalidContext)
	}
	if len(c.Tools.active) == 0 {
		return fmt.Errorf("%w: active tool allowlist is empty", ErrInvalidContext)
	}
	for name := range c.Tools.active {
		if _, ok := knownTools[name]; !ok {
			return fmt.Errorf("%w: unknown active tool", ErrInvalidContext)
		}
	}
	if mutation && strings.TrimSpace(c.IdempotencyKey) == "" {
		return fmt.Errorf("%w: mutation idempotency key is required", ErrInvalidContext)
	}
	return nil
}

func (c Context) require(name string, mutation bool) error {
	if err := c.Validate(mutation); err != nil {
		return err
	}
	if _, ok := knownTools[name]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
	if !c.Tools.Allows(name) {
		return fmt.Errorf("%w: %s", ErrToolUnavailable, name)
	}
	return nil
}

// ServiceContext is the only authority passed to phase-2 domain services.
// Services should use TenantID/ActorID from this value for all authorization
// and audit decisions, never values from a tool input.
type ServiceContext struct {
	TenantID       string
	ActorID        string
	IdempotencyKey string
}

func (c Context) serviceContext() ServiceContext {
	return ServiceContext{TenantID: c.TenantID, ActorID: c.ActorID, IdempotencyKey: c.IdempotencyKey}
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
