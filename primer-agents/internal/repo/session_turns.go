package repo

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/agents/internal/domain"
)

const turnCols = `st.id, st.session_id, st.turn_sequence, st.run_id, st.idempotency_key, st.input_preview, st.status, st.created_at`
const turnColsNoAlias = `id, session_id, turn_sequence, run_id, idempotency_key, input_preview, status, created_at`

// SessionTurns is the canonical session-turn repository instance.
var SessionTurns = &SessionTurnRepo{}

// SessionTurnRepo provides persistence operations for agents.session_turns.
type SessionTurnRepo struct{}

// AppendTurnCmd carries inputs for a CAS-protected turn creation.
type AppendTurnCmd struct {
	SessionID        string
	OwnerNamespace   string
	IdempotencyKey   string
	InputPreview     *string
	Profile          string
	ExpectedRevision int64
}

// AppendTurnResult carries the new turn, run, and updated session.
type AppendTurnResult struct {
	Turn    *domain.SessionTurn
	Run     *domain.Run
	Session *domain.Session
}

// AppendTurn atomically advances the session revision and creates a linked run.
//
//   - ExpectedRevision must equal the current session.revision (CAS guard).
//   - Same (session_id, idempotency_key) → idempotent: returns existing turn+run.
//   - Different revision → ErrStaleVersion (conflict for caller to retry).
//   - Wrong namespace → ErrNotFound.
//   - Session not open → ErrInvalidTransition.
func (r *SessionTurnRepo) AppendTurn(ctx context.Context, pool *pgxpool.Pool,
	runs *RunRepo, sessions *SessionRepo, cmd AppendTurnCmd) (*AppendTurnResult, error) {

	var result *AppendTurnResult
	err := WithTx(ctx, pool, func(q Querier) error {
		// Lock the session row to serialise concurrent turn appends.
		const lockSQL = `
			SELECT ` + sessionCols + `
			FROM agents.sessions
			WHERE id = $1 AND owner_namespace = $2
			FOR UPDATE`
		rows, err := q.Query(ctx, lockSQL, cmd.SessionID, cmd.OwnerNamespace)
		if err != nil {
			return fmt.Errorf("lock session: %w", err)
		}
		sess, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.Session])
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("scan locked session: %w", err)
		}
		if sess.Status != domain.SessionStatusOpen {
			return ErrInvalidTransition
		}

		// Idempotency check: same key → return existing turn.
		existing, err := r.getByIdempotencyKey(ctx, q, cmd.SessionID, cmd.IdempotencyKey)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if err == nil && existing.RunID != nil {
			// Already created — fetch the run and return idempotently.
			run, rerr := runs.Get(ctx, q, *existing.RunID, cmd.OwnerNamespace, "")
			if rerr != nil {
				return rerr
			}
			updatedSess := sess
			result = &AppendTurnResult{Turn: existing, Run: run, Session: &updatedSess}
			return nil
		}

		// CAS: expected revision must match current.
		if sess.Revision != cmd.ExpectedRevision {
			return ErrStaleVersion
		}

		newRevision := sess.Revision + 1
		turnSeq := newRevision // turn_sequence == new revision (1-based)

		// Create the linked run first so we have its ID.
		var ihPtr *string
		var inputHash string
		if cmd.InputPreview != nil {
			inputHash = computeTurnInputHash(cmd.SessionID, cmd.IdempotencyKey, *cmd.InputPreview)
			ihPtr = &inputHash
		}
		sessID := cmd.SessionID
		run, err := runs.Create(ctx, q, CreateRunCmd{
			SessionID:       &sessID,
			OwnerNamespace:  cmd.OwnerNamespace,
			IdempotencyKey:  cmd.IdempotencyKey,
			IdempotencyHash: computeTurnIdempHash(cmd.OwnerNamespace, cmd.SessionID, cmd.IdempotencyKey, inputHash),
			Profile:         cmd.Profile,
			InputHash:       ihPtr,
			InputPreview:    cmd.InputPreview,
		})
		if err != nil {
			return fmt.Errorf("create turn run: %w", err)
		}

		// Insert session_turn row with run_id.
		runID := run.ID
		const insertSQL = `
			INSERT INTO agents.session_turns
			    (session_id, turn_sequence, run_id, idempotency_key, input_preview, status)
			VALUES ($1, $2, $3, $4, $5, 'active')
			RETURNING ` + turnColsNoAlias
		trows, err := q.Query(ctx, insertSQL,
			cmd.SessionID, turnSeq, runID, cmd.IdempotencyKey, cmd.InputPreview)
		if err != nil {
			return fmt.Errorf("insert session turn: %w", err)
		}
		turn, err := pgx.CollectExactlyOneRow(trows, pgx.RowToStructByNameLax[domain.SessionTurn])
		if err != nil {
			return fmt.Errorf("scan session turn: %w", err)
		}

		// Advance session revision.
		const advSQL = `
			UPDATE agents.sessions
			SET revision = $3, updated_at = now()
			WHERE id = $1 AND revision = $2
			RETURNING ` + sessionCols
		srows, err := q.Query(ctx, advSQL, cmd.SessionID, sess.Revision, newRevision)
		if err != nil {
			return fmt.Errorf("advance session revision: %w", err)
		}
		updatedSess, err := pgx.CollectExactlyOneRow(srows, pgx.RowToStructByNameLax[domain.Session])
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrStaleVersion // concurrent modification
			}
			return fmt.Errorf("scan updated session: %w", err)
		}

		result = &AppendTurnResult{Turn: &turn, Run: run, Session: &updatedSess}
		return nil
	})
	return result, err
}

// ListTurns returns session turns in ascending turn_sequence order.
func (r *SessionTurnRepo) ListTurns(ctx context.Context, q Querier, sessionID, namespace string, limit int) ([]*domain.SessionTurn, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const sql = `
		SELECT ` + turnCols + `
		FROM agents.session_turns st
		JOIN agents.sessions s ON s.id = st.session_id
		WHERE st.session_id = $1 AND s.owner_namespace = $2
		ORDER BY st.turn_sequence ASC
		LIMIT $3`
	rows, err := q.Query(ctx, sql, sessionID, namespace, limit)
	if err != nil {
		return nil, fmt.Errorf("list turns: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.SessionTurn])
	if err != nil {
		return nil, fmt.Errorf("scan turns: %w", err)
	}
	out := make([]*domain.SessionTurn, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out, nil
}

func (r *SessionTurnRepo) getByIdempotencyKey(ctx context.Context, q Querier, sessionID, key string) (*domain.SessionTurn, error) {
	const sql = `SELECT ` + turnColsNoAlias + ` FROM agents.session_turns
		WHERE session_id = $1 AND idempotency_key = $2`
	rows, err := q.Query(ctx, sql, sessionID, key)
	if err != nil {
		return nil, fmt.Errorf("get turn by key: %w", err)
	}
	t, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.SessionTurn])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan turn: %w", err)
	}
	return &t, nil
}

// computeTurnInputHash returns a deterministic hash of the turn's identity.
func computeTurnInputHash(sessionID, idempKey, preview string) string {
	sum := sha256.Sum256([]byte(sessionID + "|" + idempKey + "|" + preview))
	return fmt.Sprintf("%x", sum)
}

func computeTurnIdempHash(namespace, sessionID, idempKey, inputHash string) string {
	scope := namespace + "|" + sessionID + "|" + idempKey + "|" + inputHash
	sum := sha256.Sum256([]byte(scope))
	return fmt.Sprintf("%x", sum)
}

func init() {}

// ListRunEventsByCursor pages run events after afterSeq for replay streaming.
func ListRunEventsByCursor(ctx context.Context, q Querier, runID, namespace string, afterSeq int64, limit int) ([]*domain.RunEvent, error) {
	return Events.List(ctx, q, runID, namespace, afterSeq, limit)
}

// GetRunWithOwnership fetches a run and verifies namespace. Returns ErrNotFound on mismatch.
func GetRunWithOwnership(ctx context.Context, q Querier, runID, namespace string) (*domain.Run, error) {
	return Runs.Get(ctx, q, runID, namespace, "")
}

// SubscribeRunEvents returns a channel that receives a wake-up signal after new
// events are appended using PostgreSQL LISTEN/NOTIFY. The channel is closed when
// ctx is done. The caller should combine with polling for correctness.
func SubscribeRunEvents(ctx context.Context, pool *pgxpool.Pool, runID string) (<-chan struct{}, error) {
	ch := make(chan struct{}, 8)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire listen conn: %w", err)
	}

	channel := "agents_run_" + stripDashes(runID)
	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		conn.Release()
		return nil, fmt.Errorf("listen: %w", err)
	}

	go func() {
		defer close(ch)
		defer conn.Release()
		for {
			_, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				return // ctx done or connection error
			}
			select {
			case ch <- struct{}{}:
			default: // drop wake-up if buffer full; subscriber polls anyway
			}
		}
	}()
	return ch, nil
}

func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		if c != '-' {
			out = append(out, c)
		}
	}
	return string(out)
}

// waitForNewEvents blocks until a new event is available (via notify or poll
// timeout). Returns immediately if notifyCh fires; otherwise waits pollInterval.
func WaitForNewEvents(ctx context.Context, notifyCh <-chan struct{}, pollInterval time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case _, ok := <-notifyCh:
		return ok
	case <-time.After(pollInterval):
		return true
	}
}
