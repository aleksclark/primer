package appservice_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/appservice"
	agentsdb "github.com/aleksclark/primer/agents/internal/db"
	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/repo"
	"github.com/aleksclark/primer/agents/internal/testutil"
)

func newSvc(t *testing.T) (*appservice.Service, *pgxpool.Pool) {
	t.Helper()
	p := testutil.DB(t)
	return appservice.New(p), p
}

// freshPool opens a second independent pool to the same database — simulates restart.
func freshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := testutil.URL(t)
	p, err := agentsdb.Connect(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

// ─── BDD 1: identity and status survive repository/process replacement ────────

func TestRunIdentitySurvivesRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)

	run, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: "ns-restart",
		IdempotencyKey: "key-restart",
		Profile:        "tutor",
		InputContent:   []byte("hello world"),
	})
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusQueued, run.Status)

	// "Restart": open a brand-new pool to the same DB.
	svc2 := appservice.New(freshPool(t))

	got, err := svc2.GetRun(ctx, run.ID, "ns-restart")
	require.NoError(t, err)
	assert.Equal(t, run.ID, got.ID)
	assert.Equal(t, run.OwnerNamespace, got.OwnerNamespace)
	assert.Equal(t, run.Profile, got.Profile)
	assert.Equal(t, run.Status, got.Status)
	assert.Equal(t, run.IdempotencyKey, got.IdempotencyKey)
	assert.Equal(t, run.IdempotencyHash, got.IdempotencyHash)
}

func TestSessionIdentitySurvivesRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)

	sess, err := svc.CreateSession(ctx, appservice.CreateSessionCmd{
		OwnerNamespace: "ns-sess-restart",
		Profile:        "admin",
	})
	require.NoError(t, err)

	svc2 := appservice.New(freshPool(t))
	got, err := svc2.GetSession(ctx, sess.ID, "ns-sess-restart")
	require.NoError(t, err)
	assert.Equal(t, sess.ID, got.ID)
	assert.Equal(t, "admin", got.Profile)
	assert.Equal(t, domain.SessionStatusOpen, got.Status)
}

// ─── BDD 2: idempotent create ─────────────────────────────────────────────────

func TestCreateRunIdempotentSameInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)

	cmd := appservice.CreateRunCmd{
		OwnerNamespace: "ns-idem-svc",
		IdempotencyKey: "key-idem-svc",
		Profile:        "tutor",
		InputContent:   []byte("same input"),
	}
	r1, err := svc.CreateRun(ctx, cmd)
	require.NoError(t, err)

	r2, err := svc.CreateRun(ctx, cmd)
	require.NoError(t, err)
	assert.Equal(t, r1.ID, r2.ID, "same input must return same run")
}

func TestCreateRunConflictDifferentInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)

	_, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: "ns-conflict-svc",
		IdempotencyKey: "key-conflict-svc",
		Profile:        "tutor",
		InputContent:   []byte("input A"),
	})
	require.NoError(t, err)

	_, err = svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: "ns-conflict-svc",
		IdempotencyKey: "key-conflict-svc",
		Profile:        "tutor",
		InputContent:   []byte("input B — materially different"),
	})
	require.ErrorIs(t, err, repo.ErrConflict)
}

func TestCreateRunConcurrentIdempotency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)
	const n = 8
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			r, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
				OwnerNamespace: "ns-conc-svc",
				IdempotencyKey: "key-conc-svc",
				Profile:        "tutor",
				InputContent:   []byte("same"),
			})
			errs[i] = err
			if r != nil {
				ids[i] = r.ID
			}
		}(i)
	}
	wg.Wait()

	var first string
	for i, err := range errs {
		require.NoError(t, err, "goroutine %d", i)
		if first == "" {
			first = ids[i]
		} else {
			assert.Equal(t, first, ids[i])
		}
	}
}

func TestCreateRunNamespaceIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)

	r1, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: "ns-svc-A",
		IdempotencyKey: "shared-key",
		Profile:        "tutor",
		InputContent:   []byte("same"),
	})
	require.NoError(t, err)

	r2, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: "ns-svc-B",
		IdempotencyKey: "shared-key",
		Profile:        "tutor",
		InputContent:   []byte("same"),
	})
	require.NoError(t, err)
	assert.NotEqual(t, r1.ID, r2.ID, "different namespaces must produce independent runs")
}

// ─── BDD 3: lifecycle transitions fail closed ─────────────────────────────────

func TestLifecycleFullPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)
	ns := "ns-lifecycle"

	run, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-lifecycle",
		Profile: "tutor", InputContent: []byte("x"),
	})
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusQueued, run.Status)

	// queued → running
	running, err := svc.ClaimRun(ctx, run.ID, ns, run.StateVersion,
		time.Now().Add(30*time.Second))
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusRunning, running.Status)
	assert.Equal(t, int64(2), running.StateVersion)

	// running → succeeded (with terminal event)
	term, err := svc.TerminateRun(ctx, appservice.TerminateRunCmd{
		RunID: run.ID, OwnerNamespace: ns, StateVersion: running.StateVersion,
		FromStatus: domain.RunStatusRunning, TermStatus: domain.RunStatusSucceeded,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusSucceeded, term.Status)

	// Terminal event was appended.
	evs, err := svc.ListEvents(ctx, run.ID, ns, 0, 100)
	require.NoError(t, err)
	require.NotEmpty(t, evs, "terminal event must be present")
	assert.Equal(t, "run.succeeded", evs[len(evs)-1].Kind)
}

func TestLifecycleTerminalCannotBeOverwritten(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)
	ns := "ns-term-no-overwrite"

	run, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-no-ow",
		Profile: "tutor",
	})
	require.NoError(t, err)

	running, err := svc.ClaimRun(ctx, run.ID, ns, run.StateVersion, time.Now().Add(time.Minute))
	require.NoError(t, err)

	_, err = svc.TerminateRun(ctx, appservice.TerminateRunCmd{
		RunID: run.ID, OwnerNamespace: ns, StateVersion: running.StateVersion,
		FromStatus: domain.RunStatusRunning, TermStatus: domain.RunStatusSucceeded,
	})
	require.NoError(t, err)

	// Attempt repeated terminal → must fail.
	_, err = svc.TerminateRun(ctx, appservice.TerminateRunCmd{
		RunID: run.ID, OwnerNamespace: ns, StateVersion: running.StateVersion + 1,
		FromStatus: domain.RunStatusSucceeded, TermStatus: domain.RunStatusFailed,
	})
	require.ErrorIs(t, err, repo.ErrInvalidTransition)
}

func TestLifecycleCancelPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)
	ns := "ns-cancel-path"

	run, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-cp",
		Profile: "tutor",
	})
	require.NoError(t, err)

	// queued → cancel_requested
	rc := "test_reason"
	cr, err := svc.RequestCancel(ctx, run.ID, ns, &rc)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusCancelRequested, cr.Status)

	// cancel_requested → canceled (with terminal event)
	canceled, err := svc.AcknowledgeCancel(ctx, run.ID, ns, cr.StateVersion)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusCanceled, canceled.Status)

	evs, err := svc.ListEvents(ctx, run.ID, ns, 0, 100)
	require.NoError(t, err)
	require.NotEmpty(t, evs)
	assert.Equal(t, "run.canceled", evs[len(evs)-1].Kind)
}

// ─── BDD 4: cancellation survives the initiating request ──────────────────────

func TestCancellationSurvivesContextCancel(t *testing.T) {
	t.Parallel()
	// Use a cancelable context only for the initial cancel operation.
	bgCtx := context.Background()
	svc, _ := newSvc(t)
	ns := "ns-cancel-survive"

	run, err := svc.CreateRun(bgCtx, appservice.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-cs",
		Profile: "tutor",
	})
	require.NoError(t, err)

	// Issue cancel, immediately cancel the context after it returns.
	cancelCtx, cancelFn := context.WithCancel(bgCtx)
	cr, err := svc.RequestCancel(cancelCtx, run.ID, ns, nil)
	require.NoError(t, err)
	cancelFn() // simulates HTTP request context ending

	// New service (restart) — cancel_requested must still be visible.
	svc2 := appservice.New(freshPool(t))
	got, err := svc2.GetRun(bgCtx, run.ID, ns)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusCancelRequested, got.Status,
		"cancel_requested must persist after context cancellation")

	// Repeated cancel is idempotent.
	cr2, err := svc2.RequestCancel(bgCtx, run.ID, ns, nil)
	require.NoError(t, err)
	assert.Equal(t, cr.ID, cr2.ID)
	assert.Equal(t, domain.RunStatusCancelRequested, cr2.Status)

	// Only explicit acknowledgment transitions to canceled.
	canceled, err := svc2.AcknowledgeCancel(bgCtx, run.ID, ns, cr2.StateVersion)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusCanceled, canceled.Status)
}

// ─── BDD 5: events are durable and monotonically replayable ──────────────────

func TestEventsDurableAfterRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)
	ns := "ns-ev-durable"

	run, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-ev-dur",
		Profile: "tutor",
	})
	require.NoError(t, err)

	for i := range 5 {
		p := fmt.Sprintf(`{"i":%d}`, i)
		_, err := svc.AppendEvent(ctx, appservice.AppendEventCmd{
			RunID: run.ID, OwnerNamespace: ns,
			Kind: "test.step", Payload: &p,
		})
		require.NoError(t, err)
	}

	// Restart.
	svc2 := appservice.New(freshPool(t))
	evs, err := svc2.ListEvents(ctx, run.ID, ns, 0, 100)
	require.NoError(t, err)
	require.Len(t, evs, 5)
	for i, ev := range evs {
		assert.Equal(t, int64(i+1), ev.Sequence)
		assert.Equal(t, "test.step", ev.Kind)
	}
}

func TestEventsTerminalConsistencyWithStatus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSvc(t)
	ns := "ns-ev-term-cons"

	run, err := svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: ns, IdempotencyKey: "key-etc",
		Profile: "tutor",
	})
	require.NoError(t, err)

	// Append a regular event before terminal.
	_, err = svc.AppendEvent(ctx, appservice.AppendEventCmd{
		RunID: run.ID, OwnerNamespace: ns, Kind: "run.start",
	})
	require.NoError(t, err)

	running, err := svc.ClaimRun(ctx, run.ID, ns, run.StateVersion, time.Now().Add(time.Minute))
	require.NoError(t, err)

	// TerminateRun commits status + terminal event atomically.
	_, err = svc.TerminateRun(ctx, appservice.TerminateRunCmd{
		RunID: run.ID, OwnerNamespace: ns, StateVersion: running.StateVersion,
		FromStatus: domain.RunStatusRunning, TermStatus: domain.RunStatusSucceeded,
	})
	require.NoError(t, err)

	// After restart, verify terminal status and event agree.
	svc2 := appservice.New(freshPool(t))
	got, err := svc2.GetRun(ctx, run.ID, ns)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusSucceeded, got.Status)

	evs, err := svc2.ListEvents(ctx, run.ID, ns, 0, 100)
	require.NoError(t, err)
	// run.start + run.succeeded
	require.GreaterOrEqual(t, len(evs), 2)
	last := evs[len(evs)-1]
	assert.Equal(t, "run.succeeded", last.Kind,
		"terminal event must match terminal status")
}
