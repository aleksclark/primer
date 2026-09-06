package parent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConfirmationRejected = errors.New("confirmation rejected")
	ErrConfirmationExpired  = errors.New("confirmation expired")
	// A changed domain version is irrecoverably stale; expiry/credential
	// refresh alone is not. Preserve the broader expiry classification.
	ErrConfirmationStale    = fmt.Errorf("%w: resource version changed", ErrConfirmationExpired)
	ErrConfirmationReplay   = errors.New("confirmation already used")
	ErrConfirmationForeign  = errors.New("confirmation does not belong to caller")
	ErrConfirmationAltered  = errors.New("confirmation action changed")
	ErrConfirmationNotFound = errors.New("confirmation not found")
)

const (
	ActionCreateTaskSchedule = "create_task_schedule"
	ActionRetireTask         = "retire_task"
	ActionDisableSchedule    = "disable_schedule"
	ActionBulkScheduleEdit   = "bulk_schedule_edit"
)

// Action contains model-selected data only.  Tenant and actor are deliberately
// absent: they are bound from Context when a preview is issued.
type Action struct {
	Kind      string         `json:"kind"`
	TargetIDs []string       `json:"targetIds"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func RetireTaskAction(taskID string, expectedVersion int) Action {
	return Action{Kind: ActionRetireTask, TargetIDs: []string{taskID}, Payload: map[string]any{"expectedVersion": expectedVersion}}
}
func DisableScheduleAction(scheduleID string, expectedVersion int) Action {
	return Action{Kind: ActionDisableSchedule, TargetIDs: []string{scheduleID}, Payload: map[string]any{"expectedVersion": expectedVersion}}
}
func BulkScheduleEditAction(scheduleIDs []string) Action {
	return Action{Kind: ActionBulkScheduleEdit, TargetIDs: scheduleIDs}
}

func isDestructive(kind string) bool {
	return kind == ActionRetireTask || kind == ActionDisableSchedule || kind == ActionBulkScheduleEdit || kind == ToolDraftTask || kind == ToolUpdateTask || kind == ToolPublishTask || kind == ToolCreateSchedule || kind == ToolUpdateSchedule || kind == ActionCreateTaskSchedule
}

func normalizeAction(action Action) (Action, []byte, error) {
	action.Kind = strings.TrimSpace(action.Kind)
	if !isDestructive(action.Kind) {
		return Action{}, nil, fmt.Errorf("%w: action is not destructive", ErrInvalidInput)
	}
	if len(action.TargetIDs) == 0 {
		return Action{}, nil, fmt.Errorf("%w: action has no targets", ErrInvalidInput)
	}
	for i, id := range action.TargetIDs {
		action.TargetIDs[i] = strings.TrimSpace(id)
		if action.TargetIDs[i] == "" {
			return Action{}, nil, fmt.Errorf("%w: action has an empty target", ErrInvalidInput)
		}
	}
	// A bulk operation is a set.  Sorting makes equivalent model encodings
	// produce one digest while still rejecting every actual target change.
	if action.Kind == ActionBulkScheduleEdit {
		sort.Strings(action.TargetIDs)
	}
	if action.Payload == nil {
		action.Payload = map[string]any{}
	}
	b, err := json.Marshal(action)
	if err != nil {
		return Action{}, nil, fmt.Errorf("%w: action is not JSON", ErrInvalidInput)
	}
	return action, b, nil
}

// ActionDigest is the stable digest bound into a confirmation.  JSON object
// keys are sorted by encoding/json; target IDs are normalized above.
func ActionDigest(action Action) (string, error) {
	_, b, err := normalizeAction(action)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write([]byte("primer.tasks.parent.action.v1\x00"))
	_, _ = h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func handleHash(handle string) []byte {
	h := sha256.New()
	_, _ = h.Write([]byte("primer.tasks.parent.confirmation.handle.v1\x00"))
	_, _ = h.Write([]byte(handle))
	return h.Sum(nil)
}

type ConfirmationPreview struct {
	RunID        string     `json:"-"`
	ToolStep     int        `json:"-"`
	ID           string     `json:"id"`
	Handle       string     `json:"handle,omitempty"`
	TenantID     string     `json:"-"`
	ActorID      string     `json:"-"`
	Action       Action     `json:"action"`
	ActionDigest string     `json:"actionDigest"`
	Summary      string     `json:"summary"`
	ExpiresAt    time.Time  `json:"expiresAt"`
	ConsumedAt   *time.Time `json:"consumedAt,omitempty"`
}

type ConfirmationStore interface {
	Issue(context.Context, ConfirmationPreview) (ConfirmationPreview, error)
	Consume(context.Context, string, string, string, string) (ConfirmationPreview, error)
}

func newHandle() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func validatePreview(p ConfirmationPreview, now time.Time) (ConfirmationPreview, error) {
	var err error
	p.Action, _, err = normalizeAction(p.Action)
	if err != nil {
		return ConfirmationPreview{}, err
	}
	if p.TenantID == "" || p.ActorID == "" || p.ExpiresAt.IsZero() || !p.ExpiresAt.After(now) {
		return ConfirmationPreview{}, fmt.Errorf("%w: invalid or expired preview", ErrInvalidInput)
	}
	p.ActionDigest, err = ActionDigest(p.Action)
	if err != nil {
		return ConfirmationPreview{}, err
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Handle == "" {
		p.Handle, err = newHandle()
		if err != nil {
			return ConfirmationPreview{}, err
		}
	}
	return p, nil
}
