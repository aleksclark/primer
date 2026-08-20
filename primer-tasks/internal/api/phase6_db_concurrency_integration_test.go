package api

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
)

type phase6ExternalFixture struct {
	tenant, student, template, revision, requirement, schedule, occurrence, attempt, verifier uuid.UUID
	requestID                                                                                 string
}

func seedPhase6External(t *testing.T, pool *pgxpool.Pool) phase6ExternalFixture {
	t.Helper()
	f := phase6ExternalFixture{tenant: uuid.New(), student: uuid.New(), template: uuid.New(), revision: uuid.New(), requirement: uuid.New(), schedule: uuid.New(), occurrence: uuid.New(), attempt: uuid.New(), verifier: uuid.New(), requestID: uuid.NewString()}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(context.Background(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO tenants(id,name) VALUES($1,'Phase 6 external')`, f.tenant)
	exec(`INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'External student')`, f.student, f.tenant)
	exec(`INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'External task','published',1)`, f.template, f.tenant)
	exec(`INSERT INTO task_revisions(id,tenant_id,template_id,version,title,status) VALUES($1,$2,$3,1,'External task','published')`, f.revision, f.tenant, f.template)
	config := `{"verifierId":"` + f.verifier.String() + `","capability":"response","schemaVersion":"external_callback.v1","options":{}}`
	exec(`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'external_callback',1,$4,'external','external')`, f.requirement, f.tenant, f.revision, config)
	exec(`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, f.schedule, f.tenant, f.student, f.template, f.revision)
	exec(`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification')`, f.occurrence, f.tenant, f.schedule, f.student, f.revision)
	exec(`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, f.attempt, f.tenant, f.occurrence, f.requirement)
	exec(`INSERT INTO external_verifier_catalog(id,name,endpoint_url,active,schema_versions,capabilities,secret_ref,secret_version,egress_policy) VALUES($1,'fixture','http://external-verifier-fixture:8092/v1/verify',true,'["external_callback.v1"]','["response"]','fixture','1','{"testFixture":true}')`, f.verifier)
	return f
}

func seedPhase6ExternalDelivery(t *testing.T, pool *pgxpool.Pool, f phase6ExternalFixture) {
	t.Helper()
	envelope, err := verification.NewRequestEnvelope(f.requestID, f.attempt.String(), f.requirement.String(), verification.ExternalCallbackSchemaVersion, "external-once", "/external/verifiers/"+f.verifier.String()+"/callback", map[string]any{"response": "answer"}, time.Now().UTC(), time.Hour, []string{"attempt", "requirement"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := verification.MarshalRequestEnvelope(envelope)
	if err := repo.NewExternalRepository(pool).Enqueue(context.Background(), repo.ExternalDelivery{ID: uuid.NewString(), TenantID: f.tenant.String(), AttemptID: f.attempt.String(), RequirementID: f.requirement.String(), VerifierID: f.verifier.String(), RequestID: f.requestID, IdempotencyKey: "external-once", SchemaVersion: envelope.SchemaVersion, CallbackPath: envelope.CallbackPath, Envelope: body, PayloadDigest: envelope.PayloadDigest, MaxAttempts: 5, ExpiresAt: envelope.ExpiresAt}); err != nil {
		t.Fatal(err)
	}
}

func cleanupPhase6External(t *testing.T, pool *pgxpool.Pool, f phase6ExternalFixture) {
	t.Helper()
	for _, query := range []string{
		"DELETE FROM external_verifier_facts WHERE tenant_id=$1", "DELETE FROM external_verifier_events WHERE tenant_id=$1", "DELETE FROM external_verifier_callbacks WHERE tenant_id=$1", "DELETE FROM external_verifier_outbox WHERE tenant_id=$1", "DELETE FROM external_verifier_attempts WHERE tenant_id=$1", "DELETE FROM external_verifier_security_events WHERE tenant_id=$1", "DELETE FROM external_verifier_security_events WHERE verifier_id=$1", "DELETE FROM external_verifier_secret_versions WHERE verifier_id=$1", "DELETE FROM verification_submissions WHERE tenant_id=$1", "DELETE FROM verification_decisions WHERE tenant_id=$1", "DELETE FROM verification_attempts WHERE tenant_id=$1", "DELETE FROM task_occurrences WHERE tenant_id=$1", "DELETE FROM verification_requirements WHERE tenant_id=$1", "DELETE FROM task_schedules WHERE tenant_id=$1", "DELETE FROM task_revisions WHERE tenant_id=$1", "DELETE FROM task_templates WHERE tenant_id=$1", "DELETE FROM external_verifier_catalog WHERE id=$1", "DELETE FROM students WHERE tenant_id=$1", "DELETE FROM tenants WHERE id=$1",
	} {
		if _, err := pool.Exec(context.Background(), query, func() any {
			if query == "DELETE FROM external_verifier_catalog WHERE id=$1" || query == "DELETE FROM external_verifier_secret_versions WHERE verifier_id=$1" || query == "DELETE FROM external_verifier_security_events WHERE verifier_id=$1" {
				return f.verifier
			}
			return f.tenant
		}()); err != nil {
			t.Fatalf("cleanup %s: %v", query, err)
		}
	}
}

func TestPhase6ExternalPostgresLeaseRaceAndStaleFence(t *testing.T) {
	pool := integrationPool(t)
	f := seedPhase6External(t, pool)
	seedPhase6ExternalDelivery(t, pool, f)
	t.Cleanup(func() { cleanupPhase6External(t, pool, f) })
	queue := repo.NewExternalRepository(pool)
	start := make(chan struct{})
	claims := make(chan repo.ExternalDelivery, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			d, ok, err := queue.Claim(context.Background(), "worker-"+uuid.NewString(), time.Minute)
			if err != nil {
				errs <- err
			} else if ok {
				claims <- d
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
	var claimed repo.ExternalDelivery
	count := 0
	for d := range claims {
		claimed = d
		count++
	}
	if count != 1 {
		t.Fatalf("claims=%d, want exactly one", count)
	}
	if err := queue.Finish(context.Background(), claimed, "stale-worker", "accepted", "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	var status, owner string
	if err := pool.QueryRow(context.Background(), `SELECT status,COALESCE(lease_owner,'') FROM external_verifier_outbox WHERE request_id=$1`, f.requestID).Scan(&status, &owner); err != nil {
		t.Fatal(err)
	}
	if status != "running" || owner != claimed.LeaseOwner {
		t.Fatalf("stale finish mutated status=%q owner=%q", status, owner)
	}
	if err := queue.Finish(context.Background(), claimed, claimed.LeaseOwner, "waiting", "", time.Time{}); err != nil {
		t.Fatal(err)
	}
}

func TestPhase6ExternalCallbackReplayHasOneDecisionAndFactSet(t *testing.T) {
	pool := integrationPool(t)
	f := seedPhase6External(t, pool)
	seedPhase6ExternalDelivery(t, pool, f)
	t.Cleanup(func() { cleanupPhase6External(t, pool, f) })
	ctx := context.Background()
	queue := repo.NewExternalRepository(pool)
	binding, err := queue.Binding(ctx, f.requestID, f.verifier.String(), f.attempt.String())
	if err != nil {
		t.Fatal(err)
	}
	callback := verification.CallbackEnvelope{Version: 1, CallbackID: uuid.NewString(), RequestID: f.requestID, AttemptRef: f.attempt.String(), VerifierID: f.verifier.String(), SchemaVersion: verification.ExternalCallbackSchemaVersion, Sequence: 1, RequestDigest: binding.PayloadDigest, Type: "accepted", Accepted: &verification.AcceptedResult{Rationale: "accepted"}}
	body, _ := json.Marshal(callback)
	if inserted, err := queue.RecordCallback(ctx, binding, callback, body); err != nil || inserted {
		t.Fatalf("first callback inserted=%v err=%v", inserted, err)
	}
	if replayed, err := queue.RecordCallback(ctx, binding, callback, body); err != nil || !replayed {
		t.Fatalf("same callback replayed=%v err=%v", replayed, err)
	}
	mutated := append([]byte(nil), body...)
	mutated[len(mutated)-2] = 'x'
	if _, err := queue.RecordCallback(ctx, binding, callback, mutated); err == nil {
		t.Fatal("mutated callback body accepted")
	}
	server := NewWithStore(pool, "test", nil)
	var wg sync.WaitGroup
	committed := make(chan bool, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := server.commitExternalDecision(ctx, verification.Decision{ID: uuid.NewString(), TenantID: f.tenant.String(), AttemptID: f.attempt.String(), OccurrenceID: f.occurrence.String(), Accepted: true, Reason: "accepted", DecidedBy: "external_verifier"})
			committed <- ok
			errs <- err
		}()
	}
	wg.Wait()
	close(committed)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	wins := 0
	for ok := range committed {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("decision winners=%d, want one", wins)
	}
	var decisions, completedFacts, occurrenceCompleted int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2`, f.tenant, f.attempt).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_verifier_facts WHERE tenant_id=$1 AND aggregate_id=$2 AND fact_type='verification.decided'`, f.tenant, f.attempt).Scan(&completedFacts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_verifier_facts WHERE tenant_id=$1 AND aggregate_id=$2 AND fact_type='occurrence.completed'`, f.tenant, f.occurrence).Scan(&occurrenceCompleted); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 || completedFacts != 1 || occurrenceCompleted != 1 {
		t.Fatalf("decisions=%d decidedFacts=%d completedFacts=%d", decisions, completedFacts, occurrenceCompleted)
	}
}
