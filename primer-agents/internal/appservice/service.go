// Package appservice provides the primer-agents application service:
// transactional lifecycle, idempotent run creation, cancellation, and event
// sequencing over the durable pgx repositories.
//
// No in-memory production state exists here; every operation round-trips to
// PostgreSQL. Workers (Phase 4) call the same service methods.
package appservice

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
)

// Service is the agents application service. All exported methods are
// safe for concurrent use and require no shared mutable process state.
type Service struct {
	pool     *pgxpool.Pool
	sessions *repo.SessionRepo
	runs     *repo.RunRepo
	events   *repo.EventRepo
}

// New constructs a Service backed by pool. The caller retains ownership of the
// pool and must close it; Service never closes the pool.
func New(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:     pool,
		sessions: repo.Sessions,
		runs:     repo.Runs,
		events:   repo.Events,
	}
}

// ─── Session operations ──────────────────────────────────────────────────────

// CreateSessionCmd carries inputs for creating a new session.
type CreateSessionCmd struct {
	OwnerNamespace string
	Profile        string
	CallerContext  *string
	ExpiresAt      *time.Time
}

// CreateSession creates a new open session and returns it.
func (s *Service) CreateSession(ctx context.Context, cmd CreateSessionCmd) (*domain.Session, error) {
	return s.sessions.Create(ctx, s.pool, repo.CreateSessionCmd{
		OwnerNamespace: cmd.OwnerNamespace,
		Profile:        cmd.Profile,
		CallerContext:  cmd.CallerContext,
		ExpiresAt:      cmd.ExpiresAt,
	})
}

// GetSession returns the session with the given id and owner namespace.
func (s *Service) GetSession(ctx context.Context, id, namespace string) (*domain.Session, error) {
	return s.sessions.Get(ctx, s.pool, id, namespace)
}

// CloseSession transitions an open session to closed using a revision CAS.
func (s *Service) CloseSession(ctx context.Context, id, namespace string, revision int64) (*domain.Session, error) {
	return s.sessions.Close(ctx, s.pool, id, namespace, revision)
}

// ─── Run operations ──────────────────────────────────────────────────────────

// CreateRunCmd carries inputs for an idempotent run creation.
type CreateRunCmd struct {
	OwnerNamespace string
	IdempotencyKey string
	Profile        string
	// InputContent is the raw input bytes used only for hash computation; never persisted.
	InputContent []byte
	// InputPreview is a bounded caller-supplied diagnostic summary (≤ 2000 chars, no credential material).
	InputPreview *string
	SessionID    *string
}

// CreateRun creates a run idempotently within the caller's namespace.
//
//   - Same namespace + same key + same input content → returns existing run.
//   - Same namespace + same key + different input content → ErrConflict.
//   - Different namespace + same key → new independent run.
func (s *Service) CreateRun(ctx context.Context, cmd CreateRunCmd) (*domain.Run, error) {
	inputHash := hashContent(cmd.InputContent)
	idempHash := computeIdempotencyHash(cmd.OwnerNamespace, cmd.IdempotencyKey, cmd.Profile, inputHash)

	var inputHashPtr *string
	if len(cmd.InputContent) > 0 {
		h := inputHash
		inputHashPtr = &h
	}

	return s.runs.Create(ctx, s.pool, repo.CreateRunCmd{
		OwnerNamespace:  cmd.OwnerNamespace,
		IdempotencyKey:  cmd.IdempotencyKey,
		IdempotencyHash: idempHash,
		Profile:         cmd.Profile,
		InputHash:       inputHashPtr,
		InputPreview:    cmd.InputPreview,
		SessionID:       cmd.SessionID,
	})
}

// GetRun returns the run with the given id and owner namespace.
func (s *Service) GetRun(ctx context.Context, id, namespace string) (*domain.Run, error) {
	return s.runs.Get(ctx, s.pool, id, namespace, "")
}

// ListRuns returns up to limit recent runs owned by namespace.
func (s *Service) ListRuns(ctx context.Context, namespace string, limit int) ([]*domain.Run, error) {
	return s.runs.ListByOwner(ctx, s.pool, namespace, limit)
}

// RequestCancel requests cancellation of a queued or running run.
// Idempotent: canceling an already-cancel_requested run succeeds and returns
// the existing run unchanged.
func (s *Service) RequestCancel(ctx context.Context, id, namespace string, reasonClass *string) (*domain.Run, error) {
	return s.runs.RequestCancel(ctx, s.pool, repo.RequestCancelCmd{
		RunID:          id,
		OwnerNamespace: namespace,
		ReasonClass:    reasonClass,
	})
}

// ClaimRun transitions a queued run to running using a CAS on stateVersion.
// Intended for Phase 4 workers; exposed here so the state machine can be
// tested without a real worker.
func (s *Service) ClaimRun(ctx context.Context, id, namespace string, stateVersion int64, leaseExpiresAt time.Time) (*domain.Run, error) {
	t := leaseExpiresAt
	return s.runs.Transition(ctx, s.pool, repo.TransitionCmd{
		RunID:           id,
		OwnerNamespace:  namespace,
		ExpectedStatus:  domain.RunStatusQueued,
		ExpectedVersion: stateVersion,
		NewStatus:       domain.RunStatusRunning,
		LeaseExpiresAt:  &t,
	})
}

// AcknowledgeCancel transitions a cancel_requested run to canceled and appends
// a terminal event in the same transaction.
func (s *Service) AcknowledgeCancel(ctx context.Context, id, namespace string, stateVersion int64) (*domain.Run, error) {
	return s.terminateWithEvent(ctx, id, namespace,
		domain.RunStatusCancelRequested, stateVersion,
		domain.RunStatusCanceled, "canceled", nil, "run.canceled")
}

// TerminateRunCmd carries inputs for a terminal run transition.
type TerminateRunCmd struct {
	RunID          string
	OwnerNamespace string
	StateVersion   int64
	// FromStatus must be running or cancel_requested.
	FromStatus  domain.RunStatus
	TermStatus  domain.RunStatus // succeeded | failed | interrupted
	ResultClass *string
	ErrorClass  *string
}

// TerminateRun transitions a running run to a terminal status and appends a
// terminal event in the same transaction, guaranteeing status/event atomicity.
func (s *Service) TerminateRun(ctx context.Context, cmd TerminateRunCmd) (*domain.Run, error) {
	kind := "run." + string(cmd.TermStatus)
	return s.terminateWithEvent(ctx, cmd.RunID, cmd.OwnerNamespace,
		cmd.FromStatus, cmd.StateVersion,
		cmd.TermStatus, string(cmd.TermStatus), cmd.ResultClass, kind)
}

// terminateWithEvent is the shared helper that does a CAS status transition and
// appends a terminal event atomically. Both writes commit or both roll back.
func (s *Service) terminateWithEvent(
	ctx context.Context,
	id, namespace string,
	fromStatus domain.RunStatus,
	stateVersion int64,
	toStatus domain.RunStatus,
	resultClass string,
	errorClass *string,
	eventKind string,
) (*domain.Run, error) {
	var updated *domain.Run
	err := withTx(ctx, s.pool, func(tx pgx.Tx) error {
		rc := resultClass
		run, err := s.runs.Transition(ctx, tx, repo.TransitionCmd{
			RunID:           id,
			OwnerNamespace:  namespace,
			ExpectedStatus:  fromStatus,
			ExpectedVersion: stateVersion,
			NewStatus:       toStatus,
			ResultClass:     &rc,
			ErrorClass:      errorClass,
		})
		if err != nil {
			return err
		}
		updated = run
		_, err = s.events.Append(ctx, tx, repo.AppendEventCmd{
			RunID:          id,
			OwnerNamespace: namespace,
			Kind:           eventKind,
		})
		return err
	})
	return updated, err
}

// ─── Event operations ─────────────────────────────────────────────────────────

// AppendEventCmd carries inputs for appending a single event to a run.
type AppendEventCmd struct {
	RunID          string
	OwnerNamespace string
	RootRunID      *string
	ParentRunID    *string
	AgentID        *string
	AgentType      *string
	AgentDepth     *int
	Kind           string
	Payload        *string
}

// AppendEvent appends a sequenced event to the run's log. Ownership is checked
// before the sequence increment; the caller must hold a transaction if combining
// with a concurrent status update.
func (s *Service) AppendEvent(ctx context.Context, cmd AppendEventCmd) (*domain.RunEvent, error) {
	var ev *domain.RunEvent
	err := withTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		ev, err = s.events.Append(ctx, tx, repo.AppendEventCmd{
			RunID:          cmd.RunID,
			OwnerNamespace: cmd.OwnerNamespace,
			RootRunID:      cmd.RootRunID,
			ParentRunID:    cmd.ParentRunID,
			AgentID:        cmd.AgentID,
			AgentType:      cmd.AgentType,
			AgentDepth:     cmd.AgentDepth,
			Kind:           cmd.Kind,
			Payload:        cmd.Payload,
		})
		return err
	})
	return ev, err
}

// ListEvents returns up to limit events for runID owned by namespace with
// sequence > afterSeq, ordered by sequence ASC (cursor-based replay).
func (s *Service) ListEvents(ctx context.Context, runID, namespace string, afterSeq int64, limit int) ([]*domain.RunEvent, error) {
	return s.events.List(ctx, s.pool, runID, namespace, afterSeq, limit)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// withTx begins a transaction on pool, runs fn, and commits. Rolls back on
// any error. Uses pgx.Tx (not repo.Querier) so the repos' Querier interface is
// satisfied by the concrete pgx.Tx.
func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// hashContent returns the lower-hex SHA-256 of b, or 64 zeros for empty input.
func hashContent(b []byte) string {
	if len(b) == 0 {
		return fmt.Sprintf("%064x", 0)
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)
}

// computeIdempotencyHash returns the lower-hex SHA-256 of the canonical
// idempotency scope string: namespace|idempotencyKey|profile|inputHash.
// This binds every significant input dimension and is different for any other
// namespace, key, profile, or input content.
func computeIdempotencyHash(namespace, idempotencyKey, profile, inputHash string) string {
	scope := namespace + "|" + idempotencyKey + "|" + profile + "|" + inputHash
	sum := sha256.Sum256([]byte(scope))
	return fmt.Sprintf("%x", sum)
}
