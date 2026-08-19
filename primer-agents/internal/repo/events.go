package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/agents/internal/domain"
)

const (
	// eventCols is used in INSERT...RETURNING (no table alias needed).
	eventCols = `
id, run_id, sequence, schema_version, root_run_id, parent_run_id,
agent_id, agent_type, agent_depth, kind, payload, created_at`

	// eventColsAliased is used in JOIN queries where both tables have an id column.
	eventColsAliased = `
e.id, e.run_id, e.sequence, e.schema_version, e.root_run_id, e.parent_run_id,
e.agent_id, e.agent_type, e.agent_depth, e.kind, e.payload, e.created_at`
)

// Events is the canonical event repository instance.
var Events = &EventRepo{}

// EventRepo provides persistence operations for agents.run_events.
type EventRepo struct{}

// AppendEventCmd carries the inputs for appending a single event to a run.
type AppendEventCmd struct {
	RunID          string
	OwnerNamespace string
	RootRunID      *string
	ParentRunID    *string
	AgentID        *string
	AgentType      *string
	AgentDepth     *int
	Kind           string
	// Payload is stored as-is; callers must ensure octet_length <= 65535.
	Payload *string
}

// Append atomically allocates the next per-run sequence number and inserts the
// event. The caller must hold a transaction that also owns any concurrent run
// status update (e.g. terminal transitions) to guarantee event/status atomicity.
//
// Ownership is enforced: the run must exist and belong to OwnerNamespace.
func (r *EventRepo) Append(ctx context.Context, q Querier, cmd AppendEventCmd) (*domain.RunEvent, error) {
	// Atomically claim the next sequence by incrementing the run's counter.
	// The WHERE on owner_namespace enforces ownership before any write.
	const claimSQL = `
		UPDATE agents.runs
		SET next_event_seq = next_event_seq + 1
		WHERE id = $1 AND owner_namespace = $2
		RETURNING next_event_seq - 1 AS seq`

	var seq int64
	if err := q.QueryRow(ctx, claimSQL, cmd.RunID, cmd.OwnerNamespace).Scan(&seq); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("claim event sequence: %w", err)
	}

	const insertSQL = `
		INSERT INTO agents.run_events
		    (run_id, sequence, root_run_id, parent_run_id,
		     agent_id, agent_type, agent_depth, kind, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + eventCols

	rows, err := q.Query(ctx, insertSQL,
		cmd.RunID, seq,
		cmd.RootRunID, cmd.ParentRunID,
		cmd.AgentID, cmd.AgentType, cmd.AgentDepth,
		cmd.Kind, cmd.Payload,
	)
	if err != nil {
		return nil, fmt.Errorf("insert event: %w", err)
	}
	ev, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.RunEvent])
	if err != nil {
		return nil, fmt.Errorf("scan event: %w", err)
	}
	return &ev, nil
}

// List returns up to limit events for runID owned by namespace with sequence >
// afterSeq, ordered by sequence ASC. Provides cursor-based replay.
func (r *EventRepo) List(ctx context.Context, q Querier, runID, namespace string, afterSeq int64, limit int) ([]*domain.RunEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const sql = `
		SELECT ` + eventColsAliased + `
		FROM agents.run_events e
		JOIN agents.runs r ON r.id = e.run_id
		WHERE e.run_id = $1
		  AND r.owner_namespace = $2
		  AND e.sequence > $3
		ORDER BY e.sequence ASC
		LIMIT $4`

	rows, err := q.Query(ctx, sql, runID, namespace, afterSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.RunEvent])
	if err != nil {
		return nil, fmt.Errorf("scan events: %w", err)
	}
	out := make([]*domain.RunEvent, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out, nil
}
