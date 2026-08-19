package phase6_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/api"
	"github.com/aleksclark/primer/agents/internal/appservice"
	"github.com/aleksclark/primer/agents/internal/authn"
	"github.com/aleksclark/primer/agents/internal/authn/jwttest"
	agentsdb "github.com/aleksclark/primer/agents/internal/db"
	"github.com/aleksclark/primer/agents/internal/domain"
	"github.com/aleksclark/primer/agents/internal/profile"
	"github.com/aleksclark/primer/agents/internal/repo"
	"github.com/aleksclark/primer/agents/internal/testutil"
	agentruntime "github.com/aleksclark/primer/agents/runtime"
	mafagent "github.com/microsoft/agent-framework-go/agent"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testutil.DB(t)
}

func newSvc(t *testing.T) *appservice.Service {
	t.Helper()
	return appservice.New(testutil.DB(t))
}

func serveJWKS(t *testing.T, keys ...*jwttest.Keypair) *httptest.Server {
	t.Helper()
	doc, _ := json.Marshal(jwttest.JWKSDoc(keys...))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(doc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newHandler(t *testing.T, key *jwttest.Keypair, jwksURL string) http.Handler {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	v, err := authn.NewValidator(authn.Options{
		Issuer:  jwttest.DefaultIssuer,
		JWKSURL: jwksURL,
		Now:     func() time.Time { return now },
	})
	require.NoError(t, err)
	svc := newSvc(t)
	return api.New(api.Options{
		Pool:      testutil.DB(t),
		Validator: v,
		Service:   &appSvcAdapter{svc: svc},
		Env:       "test",
	})
}

func mintToken(t *testing.T, key *jwttest.Keypair, now time.Time, sub, scope string) string {
	t.Helper()
	claims := jwttest.ValidHumanClaims(now, sub)
	claims.Scope = scope
	return jwttest.Mint(t, key, claims)
}

// ── BDD: Student profile is always server-selected ───────────────────────────

func TestStudentProfileServerSelected(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	h := newHandler(t, key, srv.URL)
	sub := "identity:" + uuid.NewString()
	tok := mintToken(t, key, now, sub, authn.ScopeStudentSession)

	req := httptest.NewRequest(http.MethodPost, "/agents/v1/student/sessions",
		strings.NewReader(`{"opaqueStudentRef":"student-ref-abc"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"student session create must succeed with correct scope")
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "student", body["profile"],
		"student route must always return profile=student")
}

// ── BDD: Student route fails closed without student scope ────────────────────

func TestStudentRouteFailsClosedWithoutScope(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	h := newHandler(t, key, srv.URL)
	sub := "identity:" + uuid.NewString()
	// Uses runs:write scope — NOT student:session.
	tok := mintToken(t, key, now, sub, authn.ScopeRunsWrite)

	req := httptest.NewRequest(http.MethodPost, "/agents/v1/student/sessions",
		strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code,
		"student route must be 403 without agents:student:session scope")
}

// ── BDD: Student profile invariants are code-level ───────────────────────────

func TestStudentProfileHardInvariants(t *testing.T) {
	spec, err := profile.Build(profile.Student)
	require.NoError(t, err)

	// These must be zero — immutable code-level invariants.
	assert.Equal(t, 0, spec.AgentSpec.MaxChildren,
		"student MaxChildren must be 0")
	assert.Equal(t, 0, spec.AgentSpec.MaxDepth,
		"student MaxDepth must be 0")
	assert.Equal(t, 0, spec.AgentSpec.MaxTotalChildren,
		"student MaxTotalChildren must be 0")
	assert.Nil(t, spec.AgentSpec.Tools,
		"student tool grant must be nil")

	require.NoError(t, profile.ValidateStudentSpec(spec),
		"ValidateStudentSpec must pass on the server-built student spec")
}

// ── BDD: Student MAF StartChild is denied at the engine level ─────────────────

func TestStudentStartChildDenied(t *testing.T) {
	spec, err := profile.Build(profile.Student)
	require.NoError(t, err)

	// Build a real runner with the student spec.
	prov := &agentruntime.ScriptedProvider{Updates: []*mafagent.ResponseUpdate{
		agentruntime.TextUpdate("hello student"),
	}}
	parentAgent := agentruntime.NewScriptedAgent(mafagent.Config{ID: "p", Name: "P"}, prov)
	runner := agentruntime.NewRunner(spec.AgentSpec, parentAgent, &agentruntime.CollectingSink{})

	childProv := &agentruntime.ScriptedProvider{Updates: []*mafagent.ResponseUpdate{agentruntime.TextUpdate("child")}}
	childAgent := agentruntime.NewScriptedAgent(mafagent.Config{ID: "c", Name: "C"}, childProv)

	// StartChild must be denied — MaxChildren=0.
	_, _, err = runner.StartChild(context.Background(), agentruntime.ChildSpec{
		Type: "child",
	}, childAgent)
	require.Error(t, err, "student profile must deny child delegation")
	assert.Contains(t, err.Error(), "max_children=0",
		"denial must identify the student policy invariant")
}

// ── BDD: Student authority does not leak into parent/admin namespace ──────────

func TestStudentParentNamespaceIsolation(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	parentNS := "human:identity:abc-parent/admin-bff"
	studentNS := "human:identity:abc-student/student-bff"

	// Create a parent session.
	parentSess, err := svc.CreateSession(ctx, appservice.CreateSessionCmd{
		OwnerNamespace: parentNS,
		Profile:        "admin",
	})
	require.NoError(t, err)

	// Create a student session.
	studentSess, err := svc.CreateStudentSession(ctx, appservice.CreateStudentSessionCmd{
		OwnerNamespace: studentNS,
	})
	require.NoError(t, err)
	assert.Equal(t, "student", studentSess.Profile)

	// Student namespace cannot access parent session.
	_, err = svc.GetSession(ctx, parentSess.ID, studentNS)
	require.Error(t, err, "student namespace must not retrieve parent session")

	// Parent namespace cannot access student session.
	_, err = svc.GetSession(ctx, studentSess.ID, parentNS)
	require.Error(t, err, "parent namespace must not retrieve student session")
}

// ── BDD: On-demand job uses durable run lifecycle ────────────────────────────

func TestJobDurableLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	ns := "ns-job-" + uuid.NewString()[:8]

	run, err := svc.CreateJob(ctx, appservice.CreateJobCmd{
		OwnerNamespace: ns,
		IdempotencyKey: "job-1",
		JobType:        "sync-content",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusQueued, run.Status)
	assert.Equal(t, "job", run.Profile, "job run must always have profile=job")

	// Idempotent: same key → same run.
	run2, err := svc.CreateJob(ctx, appservice.CreateJobCmd{
		OwnerNamespace: ns,
		IdempotencyKey: "job-1",
		JobType:        "sync-content",
	})
	require.NoError(t, err)
	assert.Equal(t, run.ID, run2.ID, "same idempotency key must return same run")

	// Status survives repo replacement.
	svc2 := appservice.New(freshPool(t))
	got, err := svc2.GetRun(ctx, run.ID, ns)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusQueued, got.Status)
	assert.Equal(t, "job", got.Profile)
}

func freshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := testutil.URL(t)
	p, err := agentsdb.Connect(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

// ── BDD: Schedule firing is idempotent — UNIQUE(schedule_id, due_at) ──────────

func TestScheduleFiringIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	ns := "ns-sched-idem-" + uuid.NewString()[:8]

	// Create schedule with next_due_at in the past.
	dueAt := time.Now().Add(-time.Second).UTC().Truncate(time.Millisecond)
	sched, err := svc.CreateSchedule(ctx, appservice.CreateScheduleCmd{
		OwnerNamespace: ns,
		Profile:        "job",
		JobType:        "sync",
		CronExpr:       "1m",
		Timezone:       "UTC",
		MaxCatchUp:     1,
		NextDueAt:      &dueAt,
	})
	require.NoError(t, err)
	_ = sched

	// Two concurrent scheduler instances try to fire the same schedule.
	var wins atomic.Int64
	var wg sync.WaitGroup
	wg.Add(5)
	for range 5 {
		go func() {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				return
			}
			defer tx.Rollback(ctx) //nolint:errcheck
			result, err := repo.Schedules.ClaimNextDue(ctx, tx, repo.Runs, 30*time.Second)
			if err == nil && result != nil {
				if err2 := tx.Commit(ctx); err2 == nil {
					wins.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), wins.Load(),
		"exactly one scheduler must claim and fire a given due_at")

	// Verify exactly one run exists for this schedule's namespace.
	runs, err := svc.ListRuns(ctx, ns, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, len(runs), "exactly one run must exist for one firing")
	assert.Equal(t, "job", runs[0].Profile)
}

// ── BDD: Schedule cross-namespace IDOR denied ─────────────────────────────────

func TestScheduleCrossNamespaceDenied(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	ownerNS := "ns-sched-own-" + uuid.NewString()[:8]
	otherNS := "ns-sched-other-" + uuid.NewString()[:8]

	sched, err := svc.CreateSchedule(ctx, appservice.CreateScheduleCmd{
		OwnerNamespace: ownerNS,
		Profile:        "job",
		JobType:        "sync",
		CronExpr:       "1h",
	})
	require.NoError(t, err)

	_, err = svc.GetSchedule(ctx, sched.ID, otherNS)
	require.Error(t, err, "wrong namespace must not retrieve schedule")

	_, err = svc.SetScheduleEnabled(ctx, sched.ID, otherNS, false)
	require.Error(t, err, "wrong namespace must not disable schedule")
}

// ── BDD: Schedule next_due_at advances after firing ──────────────────────────

func TestScheduleNextDueAdvances(t *testing.T) {
	ctx := context.Background()
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	ns := "ns-sched-adv-" + uuid.NewString()[:8]

	dueAt := time.Now().Add(-time.Second).UTC()
	_, err := svc.CreateSchedule(ctx, appservice.CreateScheduleCmd{
		OwnerNamespace: ns,
		Profile:        "job",
		JobType:        "advance-test",
		CronExpr:       "1m",
		Timezone:       "UTC",
		NextDueAt:      &dueAt,
	})
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx) //nolint:errcheck

	result, err := repo.Schedules.ClaimNextDue(ctx, tx, repo.Runs, 30*time.Second)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	// next_due_at must be advanced by one cron period (1 minute).
	require.NotNil(t, result.Schedule.NextDueAt)
	assert.True(t, result.Schedule.NextDueAt.After(dueAt),
		"next_due_at must advance after firing")
}

// ── appSvcAdapter for API tests ───────────────────────────────────────────────

type appSvcAdapter struct{ svc *appservice.Service }

func (a *appSvcAdapter) CreateRun(ctx context.Context, cmd api.CreateRunCmd) (*domain.Run, error) {
	return a.svc.CreateRun(ctx, appservice.CreateRunCmd{
		OwnerNamespace: cmd.OwnerNamespace, IdempotencyKey: cmd.IdempotencyKey,
		Profile: cmd.Profile, InputContent: cmd.InputContent, InputPreview: cmd.InputPreview,
	})
}
func (a *appSvcAdapter) GetRun(ctx context.Context, id, ns string) (*domain.Run, error) {
	return a.svc.GetRun(ctx, id, ns)
}
func (a *appSvcAdapter) ListRuns(ctx context.Context, ns string, limit int) ([]*domain.Run, error) {
	return a.svc.ListRuns(ctx, ns, limit)
}
func (a *appSvcAdapter) RequestCancel(ctx context.Context, id, ns string, rc *string) (*domain.Run, error) {
	return a.svc.RequestCancel(ctx, id, ns, rc)
}
func (a *appSvcAdapter) ListEvents(ctx context.Context, runID, ns string, after int64, limit int) ([]*domain.RunEvent, error) {
	return a.svc.ListEvents(ctx, runID, ns, after, limit)
}
func (a *appSvcAdapter) CreateSession(ctx context.Context, cmd api.CreateSessionCmd) (*domain.Session, error) {
	return a.svc.CreateSession(ctx, appservice.CreateSessionCmd{
		OwnerNamespace: cmd.OwnerNamespace, Profile: cmd.Profile,
	})
}
func (a *appSvcAdapter) GetSession(ctx context.Context, id, ns string) (*domain.Session, error) {
	return a.svc.GetSession(ctx, id, ns)
}
func (a *appSvcAdapter) AppendTurn(ctx context.Context, cmd api.AppendTurnCmd) (*repo.AppendTurnResult, error) {
	return a.svc.AppendTurn(ctx, appservice.AppendTurnCmd{
		SessionID: cmd.SessionID, OwnerNamespace: cmd.OwnerNamespace,
		IdempotencyKey: cmd.IdempotencyKey, Profile: "admin",
		ExpectedRevision: cmd.ExpectedRevision,
	})
}
func (a *appSvcAdapter) ListTurns(ctx context.Context, id, ns string, limit int) ([]*domain.SessionTurn, error) {
	return a.svc.ListTurns(ctx, id, ns, limit)
}
func (a *appSvcAdapter) CreateJob(ctx context.Context, cmd api.CreateJobCmd) (*domain.Run, error) {
	return a.svc.CreateJob(ctx, appservice.CreateJobCmd{
		OwnerNamespace: cmd.OwnerNamespace, IdempotencyKey: cmd.IdempotencyKey,
		JobType: cmd.JobType, InputPreview: cmd.InputPreview,
	})
}
func (a *appSvcAdapter) CreateSchedule(ctx context.Context, cmd api.CreateScheduleCmd) (*domain.Schedule, error) {
	return a.svc.CreateSchedule(ctx, appservice.CreateScheduleCmd{
		OwnerNamespace: cmd.OwnerNamespace, Profile: cmd.Profile,
		JobType: cmd.JobType, CronExpr: cmd.CronExpr,
	})
}
func (a *appSvcAdapter) GetSchedule(ctx context.Context, id, ns string) (*domain.Schedule, error) {
	return a.svc.GetSchedule(ctx, id, ns)
}
func (a *appSvcAdapter) ListSchedules(ctx context.Context, ns string, limit int) ([]*domain.Schedule, error) {
	return a.svc.ListSchedules(ctx, ns, limit)
}
func (a *appSvcAdapter) SetScheduleEnabled(ctx context.Context, id, ns string, e bool) (*domain.Schedule, error) {
	return a.svc.SetScheduleEnabled(ctx, id, ns, e)
}
func (a *appSvcAdapter) CreateStudentSession(ctx context.Context, cmd api.CreateStudentSessionCmd) (*domain.Session, error) {
	return a.svc.CreateStudentSession(ctx, appservice.CreateStudentSessionCmd{
		OwnerNamespace: cmd.OwnerNamespace,
	})
}
func (a *appSvcAdapter) AppendStudentTurn(ctx context.Context, cmd api.AppendStudentTurnCmd) (*repo.AppendTurnResult, error) {
	return a.svc.AppendStudentTurn(ctx, appservice.AppendStudentTurnCmd{
		SessionID: cmd.SessionID, OwnerNamespace: cmd.OwnerNamespace,
		IdempotencyKey: cmd.IdempotencyKey, ExpectedRevision: cmd.ExpectedRevision,
	})
}
