// Package worker implements the primer-agents durable execution worker.
// Workers claim queued runs with FOR UPDATE SKIP LOCKED leases, execute them
// through the canonical MAF runtime adapter, persist attributed events, and
// handle cancellation and startup reconciliation.
//
// No production worker is enabled by default (PRIMER_AGENTS_WORKER_ENABLED).
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	mafagent "github.com/microsoft/agent-framework-go/agent"

	"github.com/aleksclark/primer/agents/internal/appservice"
	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/profile"
	"github.com/aleksclark/primer/agents/internal/repo"
	"github.com/aleksclark/primer/agents/runtime"
)

// Config holds tunable worker parameters. All durations must be positive.
type Config struct {
	// LeaseDuration is how long a claimed run's lease is valid.
	LeaseDuration time.Duration
	// HeartbeatInterval is how often the worker extends the lease.
	HeartbeatInterval time.Duration
	// CancelPollInterval is how often the worker checks for cancel_requested.
	CancelPollInterval time.Duration
	// PollInterval is how often the loop polls for new work when idle.
	PollInterval time.Duration
	// MaxConcurrent is the maximum number of runs executed in parallel.
	MaxConcurrent int
	// ProviderCfg is the MAF provider configuration. A nil/empty provider
	// produces a scripted noop (tests). Production must supply a real config.
	ProviderCfg *mafagent.ProviderConfig
	// AgentFactory is used only by the explicitly opt-in live provider path.
	// Ordinary workers leave it nil and use the deterministic provider config.
	AgentFactory func(profile.Spec) *mafagent.Agent
	// RunTimeout bounds one provider-backed run. Zero preserves test behavior.
	RunTimeout time.Duration
}

// DefaultConfig returns safe defaults for development/test.
func DefaultConfig() Config {
	return Config{
		LeaseDuration:      30 * time.Second,
		HeartbeatInterval:  10 * time.Second,
		CancelPollInterval: 2 * time.Second,
		PollInterval:       500 * time.Millisecond,
		MaxConcurrent:      1,
	}
}

// Worker claims and executes queued runs from the durable store.
type Worker struct {
	pool *pgxpool.Pool
	svc  *appservice.Service
	cfg  Config
	runs *repo.RunRepo
}

// New constructs a Worker backed by pool using cfg.
func New(pool *pgxpool.Pool, svc *appservice.Service, cfg Config) *Worker {
	return &Worker{
		pool: pool,
		svc:  svc,
		cfg:  cfg,
		runs: repo.Runs,
	}
}

// Start reconciles stale runs then polls for new work until ctx is cancelled.
// Each claimed run is executed in a goroutine up to cfg.MaxConcurrent.
func (w *Worker) Start(ctx context.Context) error {
	if err := w.Reconcile(ctx); err != nil {
		slog.Error("worker: reconcile failed", "error", err)
		// Non-fatal: log and continue.
	}

	sem := make(chan struct{}, max(w.cfg.MaxConcurrent, 1))
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			sem <- struct{}{}
			claimed, err := w.claimOne(ctx)
			if err != nil {
				<-sem
				if !errors.Is(err, repo.ErrNotFound) {
					slog.Error("worker: claim error", "error", err)
				}
				continue
			}
			go func(run *domain.Run) {
				defer func() { <-sem }()
				if err := w.execute(ctx, run); err != nil {
					slog.Error("worker: execute error", "run_id", run.ID, "error", err)
				}
			}(claimed)
		}
	}
}

// Reconcile performs startup recovery:
//   - running + provider_started + lease_expired → interrupted
//   - running + NOT provider_started + lease_expired → queued (safe retry)
//   - cancel_requested + no lease or expired + no provider_started → canceled
func (w *Worker) Reconcile(ctx context.Context) error {
	interrupted, err := w.runs.ReconcileInterrupted(ctx, w.pool)
	if err != nil {
		return fmt.Errorf("reconcile interrupted: %w", err)
	}
	if len(interrupted) > 0 {
		slog.Info("worker: reconciled interrupted runs", "count", len(interrupted), "ids", interrupted)
	}
	requeued, err := w.runs.ReconcileNeverStartedInterrupted(ctx, w.pool)
	if err != nil {
		return fmt.Errorf("reconcile never-started: %w", err)
	}
	if len(requeued) > 0 {
		slog.Info("worker: requeued never-started runs", "count", len(requeued), "ids", requeued)
	}
	canceled, err := w.runs.ReconcileStaleCancels(ctx, w.pool)
	if err != nil {
		return fmt.Errorf("reconcile stale cancels: %w", err)
	}
	if len(canceled) > 0 {
		slog.Info("worker: reconciled stale cancels", "count", len(canceled), "ids", canceled)
	}
	return nil
}

// claimOne attempts to claim exactly one queued run inside a transaction.
// Returns ErrNotFound when the queue is empty.
func (w *Worker) claimOne(ctx context.Context) (*domain.Run, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("claim: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	run, err := w.runs.ClaimNextQueued(ctx, tx, w.cfg.LeaseDuration)
	if err != nil {
		return nil, err // ErrNotFound or DB error
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("claim: commit: %w", err)
	}
	slog.Info("worker: claimed run", "run_id", run.ID, "profile", run.Profile,
		"state_version", run.StateVersion)
	return run, nil
}

// execute runs one claimed run through the MAF runtime to completion.
func (w *Worker) execute(ctx context.Context, run *domain.Run) error {
	// Build a run-scoped cancellable context. This context is owned by the
	// worker, not by any HTTP request; it is cancelled by lease/cancel/error.
	runCtx, cancelCause := context.WithCancelCause(ctx)
	defer cancelCause(nil)
	if w.cfg.RunTimeout > 0 {
		timeoutCtx, cancelTimeout := context.WithTimeout(runCtx, w.cfg.RunTimeout)
		defer cancelTimeout()
		runCtx = timeoutCtx
	}

	// Mark provider_started before any provider call so process-death recovery
	// can correctly classify this run as interrupted rather than re-queuing it.
	if err := w.runs.MarkProviderStarted(ctx, w.pool,
		run.ID, run.OwnerNamespace, run.StateVersion); err != nil {
		// If this fails the run may already be stale; bail without executing.
		return fmt.Errorf("execute: mark provider started: %w", err)
	}
	// StateVersion is now stateVersion+1 after MarkProviderStarted claimed it.
	// We need the new version for heartbeating. Re-fetch.
	refreshed, err := w.svc.GetRun(ctx, run.ID, run.OwnerNamespace)
	if err != nil {
		return fmt.Errorf("execute: refresh run: %w", err)
	}
	stateVersion := refreshed.StateVersion

	// Start heartbeat — extends the lease periodically.
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(w.cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if err := w.runs.ExtendLease(ctx, w.pool,
					run.ID, run.OwnerNamespace, stateVersion, w.cfg.LeaseDuration); err != nil {
					slog.Error("worker: heartbeat failed", "run_id", run.ID, "error", err)
					cancelCause(fmt.Errorf("heartbeat: %w", err))
					return
				}
			}
		}
	}()

	// Start cancel poller — observes durable cancel_requested.
	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		ticker := time.NewTicker(w.cfg.CancelPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				current, err := w.svc.GetRun(ctx, run.ID, run.OwnerNamespace)
				if err != nil {
					slog.Error("worker: cancel poll failed", "run_id", run.ID, "error", err)
					return
				}
				if current.Status == domain.RunStatusCancelRequested {
					slog.Info("worker: observed cancel_requested, cancelling runtime",
						"run_id", run.ID)
					cancelCause(fmt.Errorf("run cancelled by request"))
					return
				}
			}
		}
	}()

	defer func() {
		<-heartbeatDone
		<-cancelDone
	}()

	// Build the durable event writer — events that can't be persisted fail the run.
	dw := &dbEventWriter{
		svc:       w.svc,
		runID:     run.ID,
		namespace: run.OwnerNamespace,
	}
	sink := newDurableSink(dw, cancelCause)

	// Build the MAF agent from the server-owned profile spec.
	termStatus, termErr := w.runMAF(runCtx, run, sink, stateVersion)

	// Terminate the run with its final status + terminal event in one txn.
	fromStatus := domain.RunStatusRunning
	if current, err := w.svc.GetRun(ctx, run.ID, run.OwnerNamespace); err == nil {
		fromStatus = current.Status
		stateVersion = current.StateVersion
	}

	var finalErr *string
	if termErr != nil {
		e := classifyError(termErr)
		finalErr = &e
	}
	_, err = w.svc.TerminateRun(ctx, appservice.TerminateRunCmd{
		RunID:          run.ID,
		OwnerNamespace: run.OwnerNamespace,
		StateVersion:   stateVersion,
		FromStatus:     fromStatus,
		TermStatus:     termStatus,
		ErrorClass:     finalErr,
	})
	if err != nil {
		return fmt.Errorf("execute: terminate: %w", err)
	}
	slog.Info("worker: run complete", "run_id", run.ID, "status", termStatus)
	return nil
}

// runMAF executes one run through the MAF runtime and returns the terminal
// status and any error. This is the only place MAF is invoked.
func (w *Worker) runMAF(ctx context.Context, run *domain.Run, sink agentruntime.EventSink, _ int64) (domain.RunStatus, error) {
	spec, err := profile.Build(profile.Name(run.Profile))
	if err != nil {
		return domain.RunStatusFailed, fmt.Errorf("unknown profile %q: %w", run.Profile, err)
	}

	provCfg := w.providerConfig()
	var mafAgent *mafagent.Agent
	if w.cfg.AgentFactory != nil {
		mafAgent = w.cfg.AgentFactory(spec)
	} else {
		mafAgent = profile.BuildAgent(spec, provCfg)
	}
	runner := agentruntime.NewRunner(spec.AgentSpec, mafAgent, sink)

	input := ""
	if run.InputPreview != nil {
		input = *run.InputPreview
	}

	if err := runner.Run(ctx, input); err != nil {
		if ctx.Err() != nil {
			cause := context.Cause(ctx)
			if cause != nil && isCancelRequest(cause) {
				return domain.RunStatusCanceled, cause
			}
			return domain.RunStatusInterrupted, ctx.Err()
		}
		return domain.RunStatusFailed, err
	}
	return domain.RunStatusSucceeded, nil
}

func (w *Worker) providerConfig() mafagent.ProviderConfig {
	if w.cfg.ProviderCfg != nil {
		return *w.cfg.ProviderCfg
	}
	// Fallback: deterministic noop for tests (no network, no credentials).
	prov := &agentruntime.ScriptedProvider{
		Name:    "noop",
		Updates: nil,
	}
	return prov.ProviderConfig()
}

func isCancelRequest(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "run cancelled by request"
}

func classifyError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	return "execution_error"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Durable event writer ───────────────────────────────────────────────────────

// dbEventWriter persists runtime events to the agents database via the
// appservice. If a write fails the DurableEventSink cancels the run context.
type dbEventWriter struct {
	svc       *appservice.Service
	runID     string
	namespace string
}

func (w *dbEventWriter) Write(ctx context.Context, e agentruntime.RunEvent) error {
	payload := safePayload(e)
	_, err := w.svc.AppendEvent(ctx, appservice.AppendEventCmd{
		RunID:          w.runID,
		OwnerNamespace: w.namespace,
		Kind:           e.Kind,
		AgentID:        nilStr(e.AgentID),
		AgentType:      nilStr(e.AgentType),
		AgentDepth:     nilInt(e.Depth),
		RootRunID:      nilStr(e.RootRunID),
		ParentRunID:    nilStr(e.ParentRunID),
		Payload:        payload,
	})
	return err
}

// safePayload builds a bounded event payload from safe diagnostic fields only.
// Raw model text, tool arguments, and response content are excluded.
func safePayload(e agentruntime.RunEvent) *string {
	switch e.Kind {
	case agentruntime.KindStart:
		// input preview is already bounded and caller-supplied; safe to persist
		if e.Text != "" {
			s := boundedStr(e.Text, 2000)
			return &s
		}
	case agentruntime.KindError, agentruntime.KindEnd:
		if e.Err != "" {
			s := boundedStr(e.Err, 512)
			return &s
		}
	case agentruntime.KindToolStart, agentruntime.KindToolEnd:
		if e.ToolName != "" {
			s := e.ToolName
			return &s
		}
	}
	return nil
}

func boundedStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nilInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// pgxTxQuerier adapts pgx.Tx to repo.Querier (already satisfied by pgx.Tx).
var _ repo.Querier = (pgx.Tx)(nil)

// ── durableSink ───────────────────────────────────────────────────────────────

// durableSink wraps a dbEventWriter as an agentruntime.EventSink.
// On any write error it calls cancelCause so the runner's context is cancelled.
type durableSink struct {
	w      *dbEventWriter
	cancel context.CancelCauseFunc
}

func newDurableSink(w *dbEventWriter, cancel context.CancelCauseFunc) *durableSink {
	return &durableSink{w: w, cancel: cancel}
}

func (s *durableSink) Emit(ctx context.Context, e agentruntime.RunEvent) {
	if err := s.w.Write(ctx, e); err != nil {
		if s.cancel != nil {
			s.cancel(fmt.Errorf("durable event write: %w", err))
		}
	}
}
