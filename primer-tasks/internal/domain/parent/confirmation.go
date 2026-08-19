package parent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConfirmationRejected = errors.New("confirmation rejected")
	ErrConfirmationExpired  = errors.New("confirmation expired")
	// Stale is an alias for the terminal expiry case.
	ErrConfirmationStale    = ErrConfirmationExpired
	ErrConfirmationReplay   = errors.New("confirmation already used")
	ErrConfirmationForeign  = errors.New("confirmation does not belong to caller")
	ErrConfirmationAltered  = errors.New("confirmation action changed")
	ErrConfirmationNotFound = errors.New("confirmation not found")
)

const (
	ActionRetireTask       = "retire_task"
	ActionDisableSchedule  = "disable_schedule"
	ActionBulkScheduleEdit = "bulk_schedule_edit"
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
	return kind == ActionRetireTask || kind == ActionDisableSchedule || kind == ActionBulkScheduleEdit
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

// MemoryConfirmationStore is useful for deterministic tests and local
// qualification.  It has the same terminal states and compare-before-consume
// behavior as SQLConfirmationStore; production wiring must use the SQL store.
type MemoryConfirmationStore struct {
	mu   sync.Mutex
	rows map[string]ConfirmationPreview
	Now  func() time.Time
}

func NewMemoryConfirmationStore() *MemoryConfirmationStore {
	return &MemoryConfirmationStore{rows: make(map[string]ConfirmationPreview), Now: func() time.Time { return time.Now().UTC() }}
}
func (s *MemoryConfirmationStore) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}
func (s *MemoryConfirmationStore) Issue(_ context.Context, p ConfirmationPreview) (ConfirmationPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	p, err = validatePreview(p, s.now())
	if err != nil {
		return ConfirmationPreview{}, err
	}
	if _, exists := s.rows[p.Handle]; exists {
		return ConfirmationPreview{}, fmt.Errorf("%w: duplicate handle", ErrInvalidInput)
	}
	s.rows[p.Handle] = p
	return p, nil
}
func (s *MemoryConfirmationStore) Consume(_ context.Context, handle, tenant, actor, digest string) (ConfirmationPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.rows[handle]
	if !ok {
		return ConfirmationPreview{}, ErrConfirmationNotFound
	}
	now := s.now()
	if p.ConsumedAt != nil {
		return ConfirmationPreview{}, ErrConfirmationReplay
	}
	if !p.ExpiresAt.After(now) {
		return ConfirmationPreview{}, ErrConfirmationExpired
	}
	if subtle.ConstantTimeCompare([]byte(p.TenantID), []byte(tenant)) != 1 || subtle.ConstantTimeCompare([]byte(p.ActorID), []byte(actor)) != 1 {
		return ConfirmationPreview{}, ErrConfirmationForeign
	}
	if subtle.ConstantTimeCompare([]byte(p.ActionDigest), []byte(digest)) != 1 {
		return ConfirmationPreview{}, ErrConfirmationAltered
	}
	p.ConsumedAt = &now
	s.rows[handle] = p
	return p, nil
}
