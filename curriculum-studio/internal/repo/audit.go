package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// AuditEvent is a workspace-scoped mutation record. actor_subject_ref is the
// validated principal; no tokens or provider payloads are stored.
type AuditEvent struct {
	ID              uuid.UUID
	WorkspaceID     *uuid.UUID
	ActorSubjectRef string
	Action          string
	EntityKind      string
	EntityID        *uuid.UUID
	Before          json.RawMessage
	After           json.RawMessage
}

// AuditRepo persists curriculum_studio.audit_events.
type AuditRepo struct {
	Q Querier
}

// NewAuditRepo binds to q.
func NewAuditRepo(q Querier) *AuditRepo {
	return &AuditRepo{Q: q}
}

// Insert writes one audit row. actor_subject_ref is canonicalized when present.
func (r *AuditRepo) Insert(ctx context.Context, ev *AuditEvent) (*AuditEvent, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if ev == nil {
		return nil, fmt.Errorf("audit event is required")
	}
	if ev.Action == "" || ev.EntityKind == "" {
		return nil, fmt.Errorf("audit action and entity_kind are required")
	}
	actor := ev.ActorSubjectRef
	if actor != "" {
		canon, err := domain.CanonicalizeSubjectRef(actor)
		if err != nil {
			return nil, err
		}
		actor = canon.String()
	}
	before := ev.Before
	if len(before) == 0 {
		before = json.RawMessage(`{}`)
	}
	after := ev.After
	if len(after) == 0 {
		after = json.RawMessage(`{}`)
	}
	const q = `
INSERT INTO curriculum_studio.audit_events
    (workspace_id, actor_subject_ref, action, entity_kind, entity_id, before, after)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, workspace_id, actor_subject_ref, action, entity_kind, entity_id, before, after`
	out, err := scanAudit(r.Q.QueryRow(ctx, q, ev.WorkspaceID, actor, ev.Action, ev.EntityKind, ev.EntityID, before, after))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

// ListByWorkspace returns recent audit rows for a workspace.
func (r *AuditRepo) ListByWorkspace(ctx context.Context, workspaceID uuid.UUID, limit int) ([]AuditEvent, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil {
		return []AuditEvent{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	const q = `
SELECT id, workspace_id, actor_subject_ref, action, entity_kind, entity_id, before, after
FROM curriculum_studio.audit_events
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2`
	rows, err := r.Q.Query(ctx, q, workspaceID, limit)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		ev, err := scanAuditRow(rows)
		if err != nil {
			return nil, MapError(err)
		}
		out = append(out, *ev)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []AuditEvent{}
	}
	return out, nil
}

func scanAudit(row pgx.Row) (*AuditEvent, error) {
	var ev AuditEvent
	if err := row.Scan(&ev.ID, &ev.WorkspaceID, &ev.ActorSubjectRef, &ev.Action, &ev.EntityKind, &ev.EntityID, &ev.Before, &ev.After); err != nil {
		return nil, err
	}
	return &ev, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAuditRow(row rowScanner) (*AuditEvent, error) {
	var ev AuditEvent
	if err := row.Scan(&ev.ID, &ev.WorkspaceID, &ev.ActorSubjectRef, &ev.Action, &ev.EntityKind, &ev.EntityID, &ev.Before, &ev.After); err != nil {
		return nil, err
	}
	return &ev, nil
}
