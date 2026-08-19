package worker_test

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/appservice"
	agentsdb "github.com/aleksclark/primer/agents/internal/db"
	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
	"github.com/aleksclark/primer/agents/internal/testutil"
	"github.com/aleksclark/primer/agents/internal/worker"
	agentruntime "github.com/aleksclark/primer/agents/runtime"
	mafagent "github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newSvc(t *testing.T) *appservice.Service {
	t.Helper()
	return appservice.New(testutil.DB(t))
}

func freshSvc(t *testing.T) *appservice.Service {
	t.Helper()
	url := testutil.URL(t)
	pool, err := agentsdb.Connect(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return appservice.New(pool)
}

func createRun(t *testing.T, svc *appservice.Service, ns, profile string) *domain.Run {
	t.Helper()
	run, err := svc.CreateRun(context.Background(), appservice.CreateRunCmd{
		OwnerNamespace: ns,
		IdempotencyKey: uuid.NewString(),
		Profile:        profile,
	})
	require.NoError(t, err)
	return run
}

func scriptedWorkerCfg(t *testing.T, updates ...*mafagent.ResponseUpdate) worker.Config {
	t.Helper()
	prov := &agentruntime.ScriptedProvider{Updates: updates}
	cfg := worker.DefaultConfig()
	cfg.LeaseDuration = 5 * time.Second
	cfg.HeartbeatInterval = 2 * time.Second
	cfg.CancelPollInterval = 50 * time.Millisecond
	cfg.PollInterval = 20 * time.Millisecond
	cfg.MaxConcurrent = 1
	cfg.RunTimeout = 30 * time.Second
	pc := prov.ProviderConfig()
	cfg.ProviderCfg = &pc
	return cfg
}

// waitForStatus polls until run reaches status or timeout.
func waitForStatus(t *testing.T, svc *appservice.Service, runID, ns string, want domain.RunStatus, timeout time.Duration) *domain.Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r, err := svc.GetRun(context.Background(), runID, ns)
		if err == nil && r.Status == want {
			return r
		}
		time.Sleep(30 * time.Millisecond)
	}
	r, _ := svc.GetRun(context.Background(), runID, ns)
	if r != nil {
		t.Fatalf("timeout waiting for status %q, current %q (run %s)", want, r.Status, runID)
	} else {
		t.Fatalf("timeout waiting for status %q (run %s not found)", want, runID)
	}
	return nil
}

// ── BDD: run executes and produces durable terminal status ────────────────────

func TestWorkerClaimsAndExecutesRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ns := "ns-worker-exec-" + uuid.NewString()[:8]
	svc := newSvc(t)
	run := createRun(t, svc, ns, "tutor")
	assert.Equal(t, domain.RunStatusQueued, run.Status)

	// Worker with a scripted provider that emits one text event.
	cfg := scriptedWorkerCfg(t, agentruntime.TextUpdate("hello from MAF"))
	w := worker.New(testutil.DB(t), svc, cfg)

	workerCtx, workerCancel := context.WithCancel(ctx)
	go func() { _ = w.Start(workerCtx) }()

	got := waitForStatus(t, svc, run.ID, ns, domain.RunStatusSucceeded, 15*time.Second)
	workerCancel()

	assert.Equal(t, domain.RunStatusSucceeded, got.Status)
	assert.NotNil(t, got.ProviderStartedAt, "provider_started_at must be set")
	assert.NotNil(t, got.EndedAt, "ended_at must be set after terminal transition")

	// At least a run.start and run.end event must be persisted.
	evs, err := svc.ListEvents(ctx, run.ID, ns, 0, 100)
	require.NoError(t, err)
	require.NotEmpty(t, evs, "events must be persisted to DB")

	kinds := make(map[string]bool)
	for _, e := range evs {
		kinds[e.Kind] = true
	}
	assert.True(t, kinds[agentruntime.KindStart] || kinds[agentruntime.KindEnd],
		"expected at least run.start or run.end event, got kinds: %v", kinds)
}

// ── BDD: lease exclusivity — two workers can't claim the same run ─────────────

func TestWorkerLeaseExclusivity(t *testing.T) {
	ctx := context.Background()

	ns := "ns-worker-fence-" + uuid.NewString()[:8]
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	run := createRun(t, svc, ns, "tutor")

	// Use two workers pointing at the same DB.
	pc1 := (&agentruntime.ScriptedProvider{Updates: []*mafagent.ResponseUpdate{agentruntime.TextUpdate("w1")}}).ProviderConfig()
	pc2 := (&agentruntime.ScriptedProvider{Updates: []*mafagent.ResponseUpdate{agentruntime.TextUpdate("w2")}}).ProviderConfig()

	cfg1 := worker.DefaultConfig()
	cfg1.LeaseDuration = 10 * time.Second
	cfg1.CancelPollInterval = 50 * time.Millisecond
	cfg1.PollInterval = 20 * time.Millisecond
	cfg1.ProviderCfg = &pc1

	cfg2 := worker.DefaultConfig()
	cfg2.LeaseDuration = 10 * time.Second
	cfg2.CancelPollInterval = 50 * time.Millisecond
	cfg2.PollInterval = 20 * time.Millisecond
	cfg2.ProviderCfg = &pc2

	w1 := worker.New(pool, svc, cfg1)
	w2 := worker.New(pool, svc, cfg2)

	wCtx, wCancel := context.WithTimeout(ctx, 20*time.Second)
	defer wCancel()

	go func() { _ = w1.Start(wCtx) }()
	go func() { _ = w2.Start(wCtx) }()

	// Wait for the run to reach a terminal state.
	got := waitForStatus(t, svc, run.ID, ns, domain.RunStatusSucceeded, 15*time.Second)
	wCancel()

	assert.Equal(t, domain.RunStatusSucceeded, got.Status)

	// Attempt count must be exactly 1 — only one worker should have executed it.
	assert.Equal(t, 1, got.AttemptCount, "exactly one worker must claim and execute the run")
}

// ── BDD: durable cancellation reaches the runtime context ─────────────────────

func TestWorkerCancellationReachesRuntime(t *testing.T) {
	ctx := context.Background()

	ns := "ns-worker-cancel-" + uuid.NewString()[:8]
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	run := createRun(t, svc, ns, "tutor")

	// Provider blocks until its context is cancelled.
	cancelled := make(chan struct{})
	blockingProv := &agentruntime.ScriptedProvider{
		RunFn: func(provCtx context.Context, _ []*message.Message, _ ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				<-provCtx.Done()
				close(cancelled)
				yield(nil, provCtx.Err())
			}
		},
	}
	pc := blockingProv.ProviderConfig()

	cfg := worker.DefaultConfig()
	cfg.LeaseDuration = 30 * time.Second
	cfg.CancelPollInterval = 30 * time.Millisecond
	cfg.PollInterval = 20 * time.Millisecond
	cfg.ProviderCfg = &pc

	w := worker.New(pool, svc, cfg)
	wCtx, wCancel := context.WithTimeout(ctx, 20*time.Second)
	defer wCancel()

	go func() { _ = w.Start(wCtx) }()

	// Wait until the run is running.
	waitForStatus(t, svc, run.ID, ns, domain.RunStatusRunning, 10*time.Second)

	// Commit cancellation via the appservice. This survives the HTTP request.
	reason := "test_cancel"
	_, err := svc.RequestCancel(ctx, run.ID, ns, &reason)
	require.NoError(t, err)

	// The worker's cancel poller must observe cancel_requested and stop the agentruntime.
	select {
	case <-cancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout: provider context was never cancelled")
	}

	// Run must end as canceled.
	got := waitForStatus(t, svc, run.ID, ns, domain.RunStatusCanceled, 10*time.Second)
	wCancel()

	assert.Equal(t, domain.RunStatusCanceled, got.Status)

	// Repeated cancel on an already-terminal run is rejected (not idempotent
	// past terminal). Verify the run is stable at canceled.
	verified, err := svc.GetRun(ctx, got.ID, ns)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusCanceled, verified.Status)
}

// ── BDD: cancellation survives restart ────────────────────────────────────────

func TestCancellationSurvivesNoRunningWorker(t *testing.T) {
	ctx := context.Background()

	ns := "ns-cancel-survive-" + uuid.NewString()[:8]
	svc := newSvc(t)
	run := createRun(t, svc, ns, "tutor")

	// Commit cancel before any worker picks it up.
	_, err := svc.RequestCancel(ctx, run.ID, ns, nil)
	require.NoError(t, err)

	// New service (restart simulation) must still see cancel_requested.
	svc2 := freshSvc(t)
	got, err := svc2.GetRun(ctx, run.ID, ns)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusCancelRequested, got.Status,
		"cancel_requested must survive without a running worker")

	// Worker started after cancellation must acknowledge and terminate.
	pc := (&agentruntime.ScriptedProvider{}).ProviderConfig()
	cfg := worker.DefaultConfig()
	cfg.LeaseDuration = 10 * time.Second
	cfg.CancelPollInterval = 30 * time.Millisecond
	cfg.PollInterval = 20 * time.Millisecond
	cfg.ProviderCfg = &pc

	w := worker.New(testutil.DB(t), svc2, cfg)
	wCtx, wCancel := context.WithTimeout(ctx, 15*time.Second)
	defer wCancel()
	go func() { _ = w.Start(wCtx) }()

	final := waitForStatus(t, svc2, run.ID, ns, domain.RunStatusCanceled, 10*time.Second)
	wCancel()
	assert.Equal(t, domain.RunStatusCanceled, final.Status)
}

// ── BDD: process death — provider-started run becomes interrupted ─────────────

func TestReconcileInterruptedRunAfterLeaseExpiry(t *testing.T) {
	ctx := context.Background()

	ns := "ns-interrupted-" + uuid.NewString()[:8]
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	run := createRun(t, svc, ns, "tutor")

	// Simulate a dead worker: manually claim the run, set provider_started_at,
	// and expire the lease. This represents a process that died mid-execution.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	claimed, err := repo.Runs.ClaimNextQueued(ctx, tx, 1*time.Hour)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	assert.Equal(t, run.ID, claimed.ID)

	// Mark provider started so reconcile knows it can't safely re-queue.
	err = repo.Runs.MarkProviderStarted(ctx, pool, run.ID, ns, claimed.StateVersion)
	require.NoError(t, err)

	// Expire the lease by setting it to the past directly.
	_, err = pool.Exec(ctx,
		`UPDATE agents.runs SET lease_expires_at = now() - interval '1 second' WHERE id = $1`, run.ID)
	require.NoError(t, err)

	// New worker starts and reconciles: expired + provider_started → interrupted.
	pc := (&agentruntime.ScriptedProvider{}).ProviderConfig()
	cfg := worker.DefaultConfig()
	cfg.LeaseDuration = 10 * time.Second
	cfg.PollInterval = 50 * time.Millisecond
	cfg.ProviderCfg = &pc

	w := worker.New(pool, svc, cfg)
	require.NoError(t, w.Reconcile(ctx))

	got, err := svc.GetRun(ctx, run.ID, ns)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusInterrupted, got.Status,
		"provider-started expired run must become interrupted, not re-queued or succeeded")
	assert.Equal(t, run.ID, got.ID, "run ID must be preserved after interruption")
}

// ── BDD: never-started claim requeued safely ──────────────────────────────────

func TestReconcileRequeuesNeverStartedExpiredClaim(t *testing.T) {
	ctx := context.Background()

	ns := "ns-requeue-" + uuid.NewString()[:8]
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	run := createRun(t, svc, ns, "tutor")

	// Claim without marking provider_started (worker died before provider call).
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	claimed, err := repo.Runs.ClaimNextQueued(ctx, tx, 1*time.Hour)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	require.Equal(t, run.ID, claimed.ID)

	// Expire the lease without setting provider_started.
	_, err = pool.Exec(ctx,
		`UPDATE agents.runs SET lease_expires_at = now() - interval '1 second' WHERE id = $1`, run.ID)
	require.NoError(t, err)

	// Reconcile: running + no provider_started + expired → queued.
	pc := (&agentruntime.ScriptedProvider{}).ProviderConfig()
	cfg := worker.DefaultConfig()
	cfg.ProviderCfg = &pc

	w := worker.New(pool, svc, cfg)
	require.NoError(t, w.Reconcile(ctx))

	got, err := svc.GetRun(ctx, run.ID, ns)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusQueued, got.Status,
		"never-started expired run must return to queued for safe retry")
}

// ── BDD: student profile denies child delegation ─────────────────────────────

func TestStudentProfileDeniesChildren(t *testing.T) {
	t.Parallel()
	// Build the student spec directly to assert the invariant without
	// a full worker+postgres round-trip.
	studentSpec := agentruntime.AgentSpec{
		Type:        "student",
		MaxChildren: 0,
	}
	runner := agentruntime.NewRunner(studentSpec, nil, &agentruntime.CollectingSink{})

	prov := &agentruntime.ScriptedProvider{Updates: []*mafagent.ResponseUpdate{agentruntime.TextUpdate("ok")}}
	childAgent := agentruntime.NewScriptedAgent(mafagent.Config{ID: "c", Name: "C"}, prov)

	_, _, err := runner.StartChild(context.Background(), agentruntime.ChildSpec{
		Type:  "child",
		Tools: nil,
	}, childAgent)
	require.Error(t, err, "student profile must deny child delegation")
	assert.Contains(t, err.Error(), "max_children=0",
		"denial must reference the policy invariant")
}

// ── BDD: concurrent claim fence — exactly one winner per run ──────────────────

func TestConcurrentClaimFence(t *testing.T) {
	ctx := context.Background()
	// Use a fresh pool/svc so runs from prior tests don't pollute the race.
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	ns := "ns-fence-" + uuid.NewString()[:8]
	created := createRun(t, svc, ns, "tutor")

	// Ten goroutines race to claim a single queued run.
	const goroutines = 10
	var winners int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				return
			}
			defer tx.Rollback(ctx) //nolint:errcheck
			claimed, err := repo.Runs.ClaimNextQueued(ctx, tx, 30*time.Second)
			if err != nil {
				return
			}
			// Only count claims of OUR specific run (namespace-matched).
			if claimed.ID != created.ID {
				return // some other test's stale run; ignore
			}
			if err2 := tx.Commit(ctx); err2 == nil {
				atomic.AddInt64(&winners, 1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), winners, "FOR UPDATE SKIP LOCKED must allow exactly one winner")
}
