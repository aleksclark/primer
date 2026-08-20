package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"primer-tasks/internal/jobs"
)

// phase6DialogueFixture is deliberately made only from tables in the current
// production migrations. Phase 6's external_callback driver is not present in
// this checkout, so these tests exercise the existing durable verifier queue,
// decision boundary, and replay facts rather than inventing that boundary.
type phase6DialogueFixture struct {
	tenant, student, template, revision, requirement uuid.UUID
	schedule, occurrence, attempt                    uuid.UUID
	messages                                         []uuid.UUID
}

func phase6Fixture(t *testing.T, pool *pgxpool.Pool, messageCount int) phase6DialogueFixture {
	t.Helper()
	f := phase6DialogueFixture{
		tenant: uuid.New(), student: uuid.New(), template: uuid.New(), revision: uuid.New(), requirement: uuid.New(),
		schedule: uuid.New(), occurrence: uuid.New(), attempt: uuid.New(),
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(context.Background(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO tenants(id,name) VALUES($1,'Phase 6 database concurrency')`, f.tenant)
	exec(`INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'Concurrency student')`, f.student, f.tenant)
	exec(`INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'Concurrency task','published',1)`, f.template, f.tenant)
	exec(`INSERT INTO task_revisions(id,tenant_id,template_id,version,title,status) VALUES($1,$2,$3,1,'Concurrency task','published')`, f.revision, f.tenant, f.template)
	exec(`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_dialogue',1,'{}','chat','fantasy')`, f.requirement, f.tenant, f.revision)
	exec(`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, f.schedule, f.tenant, f.student, f.template, f.revision)
	exec(`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification')`, f.occurrence, f.tenant, f.schedule, f.student, f.revision)
	exec(`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, f.attempt, f.tenant, f.occurrence, f.requirement)
	for i := 0; i < messageCount; i++ {
		message := uuid.New()
		f.messages = append(f.messages, message)
		exec(`INSERT INTO verification_messages(id,tenant_id,attempt_id,sequence,role,content,client_message_id) VALUES($1,$2,$3,$4,'student',$5,$6)`, message, f.tenant, f.attempt, i+1, "durable answer", "phase6-message-"+uuid.NewString())
	}
	return f
}

func cleanupPhase6Fixture(t *testing.T, pool *pgxpool.Pool, f phase6DialogueFixture) {
	t.Helper()
	for _, query := range []string{
		"DELETE FROM audit_records WHERE tenant_id=$1",
		"DELETE FROM verification_events WHERE tenant_id=$1",
		"DELETE FROM verification_jobs WHERE tenant_id=$1",
		"DELETE FROM verification_overrides WHERE tenant_id=$1",
		"DELETE FROM verification_evaluations WHERE tenant_id=$1",
		"DELETE FROM dialogue_questions WHERE tenant_id=$1",
		"DELETE FROM verification_messages WHERE tenant_id=$1",
		"DELETE FROM dialogue_attempts WHERE tenant_id=$1",
		"DELETE FROM verification_decisions WHERE tenant_id=$1",
		"DELETE FROM verification_attempts WHERE tenant_id=$1",
		"DELETE FROM task_occurrences WHERE tenant_id=$1",
		"DELETE FROM verification_requirements WHERE tenant_id=$1",
		"DELETE FROM task_schedules WHERE tenant_id=$1",
		"DELETE FROM task_revisions WHERE tenant_id=$1",
		"DELETE FROM task_templates WHERE tenant_id=$1",
		"DELETE FROM students WHERE tenant_id=$1",
		"DELETE FROM tenants WHERE id=$1",
	} {
		if _, err := pool.Exec(context.Background(), query, f.tenant); err != nil {
			t.Fatalf("cleanup %s: %v", query, err)
		}
	}
}

func TestPhase6PostgresDialogueClaimRaceIsExactlyOnceAndStaleCompletionIsFenced(t *testing.T) {
	pool := integrationPool(t)
	f := phase6Fixture(t, pool, 4)
	t.Cleanup(func() { cleanupPhase6Fixture(t, pool, f) })
	ctx := context.Background()
	queue := jobs.NewPostgresRepository(pool)
	for i, message := range f.messages {
		if err := queue.EnqueueDialogue(ctx, jobs.DialogueJob{ID: uuid.NewString(), TenantID: f.tenant.String(), AttemptID: f.attempt.String(), MessageID: message.String(), MaxAttempts: 3}); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			// Make the first row eligible immediately; the production enqueue
			// default is also now when no future time is provided.
			if _, err := pool.Exec(ctx, `UPDATE verification_jobs SET available_at=now() WHERE tenant_id=$1`, f.tenant); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Every caller uses the actual PostgresRepository ClaimDialogue method.
	// Its CTE uses FOR UPDATE SKIP LOCKED, so a row can be returned to only one
	// concurrent worker even when all workers start at the same instant.
	start := make(chan struct{})
	claims := make(chan jobs.DialogueJob, len(f.messages))
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			claimed, ok, err := queue.ClaimDialogue(ctx, "phase6-worker-"+uuid.NewString(), time.Minute)
			if err != nil {
				errs <- err
				return
			}
			if ok {
				claims <- claimed
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	claimedIDs := make([]string, 0, len(claims))
	owners := make(map[string]string)
	var first jobs.DialogueJob
	for claimed := range claims {
		claimedIDs = append(claimedIDs, claimed.ID)
		if _, duplicate := owners[claimed.ID]; duplicate {
			t.Fatalf("job %s was claimed by multiple workers", claimed.ID)
		}
		owners[claimed.ID] = claimed.LeaseOwner
		if first.ID == "" {
			first = claimed
		}
	}
	sort.Strings(claimedIDs)
	if len(claimedIDs) != len(f.messages) {
		t.Fatalf("claimed %d jobs, want %d (ids=%v)", len(claimedIDs), len(f.messages), claimedIDs)
	}
	if len(owners) != len(f.messages) {
		t.Fatalf("unique owners=%d, want %d", len(owners), len(f.messages))
	}

	// The repository methods deliberately return no error for an update that
	// affects zero rows. State, rather than an in-memory acknowledgement, is the
	// fencing contract: an old owner cannot complete the replacement's row.
	if err := queue.CompleteDialogue(ctx, first.ID, "stale-after-claim"); err != nil {
		t.Fatal(err)
	}
	var status, owner string
	if err := pool.QueryRow(ctx, `SELECT status,lease_owner FROM verification_jobs WHERE id=$1`, first.ID).Scan(&status, &owner); err != nil {
		t.Fatal(err)
	}
	if status != string(jobs.Running) || owner != first.LeaseOwner {
		t.Fatalf("stale completion changed job: status=%q owner=%q want running/%q", status, owner, first.LeaseOwner)
	}
	if err := queue.CompleteDialogue(ctx, first.ID, first.LeaseOwner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM verification_jobs WHERE id=$1`, first.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		t.Fatalf("valid completion status=%q", status)
	}
}

func TestPhase6PostgresDialogueReclaimBackoffDeadLetterAndRestart(t *testing.T) {
	pool := integrationPool(t)
	f := phase6Fixture(t, pool, 1)
	t.Cleanup(func() { cleanupPhase6Fixture(t, pool, f) })
	ctx := context.Background()
	queue := jobs.NewPostgresRepository(pool)
	jobID := uuid.NewString()
	if err := queue.EnqueueDialogue(ctx, jobs.DialogueJob{ID: jobID, TenantID: f.tenant.String(), AttemptID: f.attempt.String(), MessageID: f.messages[0].String(), MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}

	first, ok, err := queue.ClaimDialogue(ctx, "process-before-restart", time.Minute)
	if err != nil || !ok {
		t.Fatalf("initial claim=%+v ok=%v err=%v", first, ok, err)
	}
	// A process restart leaves the lease row behind. Reconcile is the durable
	// recovery boundary; forcing the timestamp expired models that passage of
	// time without making the test sleep for a production lease duration.
	if _, err := pool.Exec(ctx, `UPDATE verification_jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	restartedQueue := jobs.NewPostgresRepository(pool)
	if err := restartedQueue.RequeueExpiredDialogue(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	second, ok, err := restartedQueue.ClaimDialogue(ctx, "process-after-restart", time.Minute)
	if err != nil || !ok || second.ID != jobID || second.Attempts != 2 {
		t.Fatalf("reclaimed=%+v ok=%v err=%v", second, ok, err)
	}

	if err := restartedQueue.FailDialogue(ctx, jobID, second.LeaseOwner, context.DeadlineExceeded); err != nil {
		t.Fatal(err)
	}
	var status, lastError string
	var available time.Time
	if err := pool.QueryRow(ctx, `SELECT status,last_error,available_at FROM verification_jobs WHERE id=$1`, jobID).Scan(&status, &lastError, &available); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || lastError != "dialogue_job_failed" || !available.After(time.Now()) {
		t.Fatalf("retry state status=%q error=%q available=%s", status, lastError, available)
	}
	if _, ok, err := restartedQueue.ClaimDialogue(ctx, "too-early", time.Minute); err != nil || ok {
		t.Fatalf("backoff claim ok=%v err=%v, want no claim", ok, err)
	}

	// Move only the production availability field forward, then claim the
	// retry as a fresh worker. This is the same durable action a scheduler waits
	// for after the one-second bounded backoff.
	if _, err := pool.Exec(ctx, `UPDATE verification_jobs SET available_at=now() WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	third, ok, err := restartedQueue.ClaimDialogue(ctx, "retry-worker", time.Minute)
	if err != nil || !ok || third.Attempts != 3 {
		t.Fatalf("second retry claim=%+v ok=%v err=%v", third, ok, err)
	}
	if err := restartedQueue.FailDialogue(ctx, jobID, third.LeaseOwner, context.Canceled); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status,last_error FROM verification_jobs WHERE id=$1`, jobID).Scan(&status, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || lastError != "canceled" {
		t.Fatalf("exhausted cancellation status=%q error=%q", status, lastError)
	}
}

func TestPhase6PostgresVersionedProgressReplayAndDecisionReplayAreTransactional(t *testing.T) {
	pool := integrationPool(t)
	f := phase6Fixture(t, pool, 1)
	t.Cleanup(func() { cleanupPhase6Fixture(t, pool, f) })
	ctx := context.Background()
	queue := jobs.NewPostgresRepository(pool)

	// Competing workers may replay the same durable fact. The natural key is
	// (tenant, attempt, sequence), and the repository must retain one immutable
	// version rather than append duplicate progress.
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- queue.AppendDialogueEvent(ctx, jobs.DialogueEvent{
				TenantID: f.tenant.String(), AttemptID: f.attempt.String(), Sequence: 1,
				Kind: "progress", Payload: []byte(`{"phase":"same-version"}`),
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2 AND sequence=1`, f.tenant, f.attempt).Scan(&count); err != nil || count != 1 {
		t.Fatalf("versioned fact count=%d err=%v", count, err)
	}

	for sequence := int64(2); sequence <= 3; sequence++ {
		if err := queue.AppendDialogueEvent(ctx, jobs.DialogueEvent{TenantID: f.tenant.String(), AttemptID: f.attempt.String(), Sequence: sequence, Kind: "progress", Payload: []byte(`{"phase":"next"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	replayed, err := queue.ReplayDialogueEvents(ctx, f.tenant.String(), f.attempt.String(), 0, 10)
	if err != nil || len(replayed) != 3 {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	for i, event := range replayed {
		if event.Sequence != int64(i+1) {
			t.Fatalf("replay sequence[%d]=%d", i, event.Sequence)
		}
	}
	cursor, err := queue.ReplayDialogueEvents(ctx, f.tenant.String(), f.attempt.String(), 1, 10)
	if err != nil || len(cursor) != 2 || cursor[0].Sequence != 2 {
		t.Fatalf("cursor replay=%+v err=%v", cursor, err)
	}

	// The generic decision transaction is the closest existing production
	// contract to an external callback result. It locks the attempt, inserts
	// one unique immutable decision, and projects completion transactionally.
	if _, err := pool.Exec(ctx, `UPDATE task_occurrences SET status='awaiting_verification' WHERE id=$1`, f.occurrence); err != nil {
		t.Fatal(err)
	}
	s := NewWithStore(pool, "test", nil)
	start := make(chan struct{})
	codes := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			route := chi.NewRouteContext()
			route.URLParams.Add("id", f.occurrence.String())
			req := httptest.NewRequest(http.MethodPost, "/occurrences/"+f.occurrence.String()+"/override", bytes.NewBufferString(`{"accepted":true,"reason":"replayed durable callback result"}`)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
			rec := httptest.NewRecorder()
			s.dialogueOverride(rec, req, scope{Tenant: f.tenant.String(), Subject: "phase6-parent"})
			codes <- rec.Code
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Fatalf("concurrent decision status=%d", code)
		}
	}
	var decisions, overrides, audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2`, f.tenant, f.attempt).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_overrides WHERE tenant_id=$1 AND attempt_id=$2`, f.tenant, f.attempt).Scan(&overrides); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_records WHERE tenant_id=$1 AND entity_id=$2 AND action='verification_override'`, f.tenant, f.occurrence).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	var attemptStatus, occurrenceStatus string
	if err := pool.QueryRow(ctx, `SELECT a.status,o.status FROM verification_attempts a JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=$2 WHERE a.tenant_id=$1 AND a.id=$3`, f.tenant, f.occurrence, f.attempt).Scan(&attemptStatus, &occurrenceStatus); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 || overrides != 8 || audits != 8 || attemptStatus != "accepted" || occurrenceStatus != "completed" {
		t.Fatalf("decision effects decisions=%d overrides=%d audits=%d attempt=%q occurrence=%q", decisions, overrides, audits, attemptStatus, occurrenceStatus)
	}
}
