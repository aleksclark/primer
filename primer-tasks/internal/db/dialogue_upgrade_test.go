package db

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"primer-tasks/internal/domain"
)

// This is a schema/authority checkpoint, NOT public dialogue acceptance.
// Synthetic prerequisite/evidence inserts here exercise real PostgreSQL
// constraints; later E2E must use public pairing/WS and the actual Fantasy worker.
func TestDialogueMigrationsPreserveReleasedP3AndFenceEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("primer_tasks_p4_cp1"), tcpostgres.WithUsername("tasks"), tcpostgres.WithPassword("tasks"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatalf("owned PostgreSQL is required, no skipped proof: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Errorf("owned PostgreSQL cleanup: %v", err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM tasks_schema_migrations`).Scan(&n); err != nil || n != 11 {
		t.Fatalf("fresh migration count %d: %v", n, err)
	}

	t.Run("released-nine-upgrade", func(t *testing.T) {
		if _, err = pool.Exec(ctx, `CREATE SCHEMA p4_upgrade`); err != nil {
			t.Fatal(err)
		}
		cfg := pool.Config().Copy()
		cfg.ConnConfig.RuntimeParams["search_path"] = "p4_upgrade"
		upgrade, e := pgxpool.NewWithConfig(ctx, cfg)
		if e != nil {
			t.Fatal(e)
		}
		defer upgrade.Close()
		execDialogueSQL(t, ctx, upgrade, `CREATE TABLE tasks_schema_migrations(version text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now())`)
		entries, e := migrations.ReadDir("migrations")
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			if entry.Name() > "00009_parent_agent_authority.sql" {
				continue
			}
			b, e := migrations.ReadFile("migrations/" + entry.Name())
			if e != nil {
				t.Fatal(e)
			}
			execDialogueSQL(t, ctx, upgrade, string(b))
			execDialogueSQL(t, ctx, upgrade, `INSERT INTO tasks_schema_migrations(version) VALUES($1)`, entry.Name())
		}
		seedDialogueLegacy(t, ctx, upgrade)
		before := dialogueLegacyDigests(t, ctx, upgrade)
		if e = Migrate(ctx, upgrade); e != nil {
			t.Fatal(e)
		}
		if e = Migrate(ctx, upgrade); e != nil {
			t.Fatal(e)
		}
		after := dialogueLegacyDigests(t, ctx, upgrade)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("upgrade changed canonical identity/student/manual/P3 history")
		}
		if e = upgrade.QueryRow(ctx, `SELECT count(*) FROM tasks_schema_migrations`).Scan(&n); e != nil || n != 11 {
			t.Fatalf("upgrade migration count %d: %v", n, e)
		}
	})

	f := seedDialogueLegacy(t, ctx, pool)
	insertDialogueProjection(t, ctx, pool, f)
	question := uuid.NewString()
	execDialogueSQL(t, ctx, pool, `INSERT INTO dialogue_questions(id,tenant_id,attempt_id,question_key,ordinal,version,prompt) VALUES($1,$2,$3,'wall',1,2,'What did the family repair?')`, question, f.tenant, f.attempt)
	message := uuid.NewString()
	execDialogueSQL(t, ctx, pool, `INSERT INTO verification_messages(id,tenant_id,attempt_id,question_id,policy_version,snapshot_digest,sequence,expected_version,role,content,client_message_id) VALUES($1,$2,$3,$4,'dialogue.v1',$5,1,2,'student','They repaired the wall.','message-one')`, message, f.tenant, f.attempt, question, f.snapshot.Digest)
	job := uuid.NewString()
	execDialogueSQL(t, ctx, pool, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,message_id,session_id,job_key,max_attempts,deadline) VALUES($1,$2,$3,$4,$5,'answer-one',3,now()+interval '10 minutes')`, job, f.tenant, f.attempt, message, f.session)
	evaluation := uuid.NewString()
	execDialogueSQL(t, ctx, pool, `INSERT INTO verification_evaluations(id,tenant_id,attempt_id,question_id,message_id,job_id,policy_version,snapshot_digest,version,accepted,rationale,provider,model,criteria) VALUES($1,$2,$3,$4,$5,$6,'dialogue.v1',$7,4,true,'Answer addresses the current question using the assigned source.','scripted','fixture','["source detail"]')`, evaluation, f.tenant, f.attempt, question, message, job, f.snapshot.Digest)
	override := uuid.NewString()
	execDialogueSQL(t, ctx, pool, `INSERT INTO verification_overrides(id,tenant_id,occurrence_id,requirement_id,attempt_id,client_request_id,expected_version,accepted,reason,actor_id) VALUES($1,$2,$3,$4,$5,'override-one',4,true,'Parent observed the work.','local-parent')`, override, f.tenant, f.occurrence, f.requirement, f.attempt)
	execDialogueSQL(t, ctx, pool, `INSERT INTO verification_events(tenant_id,attempt_id,sequence,event_key,kind,payload) VALUES($1,$2,1,'message-one','message_ack','{}')`, f.tenant, f.attempt)
	decision := uuid.NewString()
	execDialogueSQL(t, ctx, pool, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,false,'synthetic constraint fixture, not E2E','verification_engine')`, decision, f.tenant, f.attempt)

	t.Run("immutable-retained-evidence", func(t *testing.T) {
		for _, table := range []string{"dialogue_revision_policies", "dialogue_questions", "verification_messages", "verification_evaluations", "verification_overrides", "verification_events", "verification_decisions"} {
			for _, query := range []string{fmt.Sprintf(`UPDATE %s SET tenant_id=tenant_id WHERE tenant_id=$1`, table), fmt.Sprintf(`DELETE FROM %s WHERE tenant_id=$1`, table)} {
				expectDialogueSQLReject(t, ctx, pool, "23514", query, f.tenant)
			}
		}
		expectDialogueSQLReject(t, ctx, pool, "23514", `DELETE FROM dialogue_attempts WHERE tenant_id=$1`, f.tenant)
		expectDialogueSQLReject(t, ctx, pool, "23514", `UPDATE dialogue_attempts SET config_snapshot=jsonb_set(config_snapshot,'{config,retentionPolicy}','"redact"') WHERE tenant_id=$1`, f.tenant)
		expectDialogueSQLReject(t, ctx, pool, "23514", `UPDATE dialogue_attempts SET student_id=$2 WHERE tenant_id=$1`, f.tenant, f.otherStudent)
		// The mutable state version remains distinct from immutable evidence.
		execDialogueSQL(t, ctx, pool, `UPDATE dialogue_attempts SET version=4,next_sequence=2 WHERE tenant_id=$1`, f.tenant)
		expectDialogueSQLReject(t, ctx, pool, "23514", `UPDATE dialogue_attempts SET version=1 WHERE tenant_id=$1`, f.tenant)
	})
	t.Run("message-question-policy-bindings", func(t *testing.T) {
		other := f
		other.attempt = uuid.NewString()
		other.occurrence = uuid.NewString()
		execDialogueSQL(t, ctx, pool, `INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at) VALUES($1,$2,$3,$4,$5,now()+interval '1 day',now()+interval '1 day')`, other.occurrence, f.tenant, f.schedule, f.student, f.revision)
		execDialogueSQL(t, ctx, pool, `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, other.attempt, f.tenant, other.occurrence, f.requirement)
		insertDialogueProjection(t, ctx, pool, other)
		foreignQuestion := uuid.NewString()
		execDialogueSQL(t, ctx, pool, `INSERT INTO dialogue_questions(id,tenant_id,attempt_id,question_key,ordinal,version,prompt) VALUES($1,$2,$3,'other',1,2,'A foreign question?')`, foreignQuestion, f.tenant, other.attempt)
		messageSQL := `INSERT INTO verification_messages(id,tenant_id,attempt_id,question_id,policy_version,snapshot_digest,sequence,expected_version,role,content,client_message_id) VALUES($1,$2,$3,$4,$5,$6,2,4,'student','answer','second')`
		expectDialogueSQLReject(t, ctx, pool, "23503", messageSQL, uuid.NewString(), f.tenant, f.attempt, foreignQuestion, "dialogue.v1", f.snapshot.Digest)
		expectDialogueSQLReject(t, ctx, pool, "23503", messageSQL, uuid.NewString(), f.tenant, f.attempt, question, "dialogue.v2", f.snapshot.Digest)
		expectDialogueSQLReject(t, ctx, pool, "23503", messageSQL, uuid.NewString(), f.tenant, f.attempt, question, "dialogue.v1", strings.Repeat("0", 64))
		expectDialogueSQLReject(t, ctx, pool, "23505", `INSERT INTO dialogue_questions(id,tenant_id,attempt_id,question_key,ordinal,version,prompt) VALUES($1,$2,$3,'renamed',2,5,' WHAT DID THE FAMILY REPAIR? ')`, uuid.NewString(), f.tenant, f.attempt)
		expectDialogueSQLReject(t, ctx, pool, "23514", `INSERT INTO dialogue_questions(id,tenant_id,attempt_id,question_key,ordinal,version,prompt) VALUES($1,$2,$3,'fourth',4,11,'Fourth question?')`, uuid.NewString(), f.tenant, f.attempt)
		// Use a new unevaluated message so the FK, rather than an earlier unique
		// constraint, is the rejecting boundary. These are synthetic SQL rows,
		// never a public/Fantasy execution fixture.
		crossMessage, crossJob := uuid.NewString(), uuid.NewString()
		execDialogueSQL(t, ctx, pool, messageSQL, crossMessage, f.tenant, f.attempt, question, "dialogue.v1", f.snapshot.Digest)
		execDialogueSQL(t, ctx, pool, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,message_id,session_id,job_key,status,stage,max_attempts,deadline) VALUES($1,$2,$3,$4,$5,'binding-probe','succeeded','done',3,now()+interval '1 minute')`, crossJob, f.tenant, f.attempt, crossMessage, f.session)
		// A real same-tenant but different attempt/question cannot be attached to
		// this message just because the independent IDs all exist.
		expectDialogueSQLReject(t, ctx, pool, "23503", `INSERT INTO verification_evaluations(id,tenant_id,attempt_id,question_id,message_id,job_id,policy_version,snapshot_digest,version,accepted,rationale,provider,model,criteria) VALUES($1,$2,$3,$4,$5,$6,'dialogue.v1',$7,5,false,'Add a specific detail or reason from the assigned source.','scripted','fixture','[]')`, uuid.NewString(), f.tenant, f.attempt, foreignQuestion, crossMessage, crossJob, f.snapshot.Digest)
		expectDialogueSQLReject(t, ctx, pool, "23505", `INSERT INTO verification_evaluations(id,tenant_id,attempt_id,question_id,message_id,job_id,policy_version,snapshot_digest,version,accepted,rationale,provider,model,criteria) VALUES($1,$2,$3,$4,$5,$6,'dialogue.v1',$7,5,true,'Answer addresses the current question using the assigned source.','scripted','fixture','["source detail"]')`, uuid.NewString(), f.tenant, f.attempt, question, crossMessage, crossJob, f.snapshot.Digest)
		expectDialogueSQLReject(t, ctx, pool, "23514", `INSERT INTO verification_overrides(id,tenant_id,occurrence_id,requirement_id,attempt_id,client_request_id,expected_version,accepted,reason,actor_id) VALUES($1,$2,$3,$4,$5,'foreign',4,true,'Must fail','local-parent')`, uuid.NewString(), f.tenant, other.occurrence, f.requirement, f.attempt)
		// Session from another student cannot authorize a job in this attempt.
		expectDialogueSQLReject(t, ctx, pool, "23514", `INSERT INTO verification_jobs(id,tenant_id,attempt_id,session_id,job_key,max_attempts,deadline) VALUES($1,$2,$3,$4,'foreign-session',3,now()+interval '1 minute')`, uuid.NewString(), f.tenant, other.attempt, f.otherSession)
	})
	t.Run("job-admission-is-bound-and-monotonic", func(t *testing.T) {
		expectDialogueSQLReject(t, ctx, pool, "23505", `INSERT INTO verification_jobs(id,tenant_id,attempt_id,session_id,job_key,max_attempts,deadline) VALUES($1,$2,$3,$4,'concurrent-active',3,now()+interval '1 minute')`, uuid.NewString(), f.tenant, f.attempt, f.session)
		expectDialogueSQLReject(t, ctx, pool, "23514", `UPDATE verification_jobs SET lease_owner='partial' WHERE id=$1`, job)
		execDialogueSQL(t, ctx, pool, `UPDATE verification_jobs SET status='running',attempts=2,lease_generation=1,lease_owner='synthetic-lease',lease_until=now()+interval '30 seconds' WHERE id=$1`, job)
		expectDialogueSQLReject(t, ctx, pool, "23514", `UPDATE verification_jobs SET attempts=1 WHERE id=$1`, job)
		expectDialogueSQLReject(t, ctx, pool, "23514", `UPDATE verification_jobs SET lease_generation=0 WHERE id=$1`, job)
		expectDialogueSQLReject(t, ctx, pool, "23514", `UPDATE verification_jobs SET session_id=$2 WHERE id=$1`, job, f.otherSession)
		execDialogueSQL(t, ctx, pool, `UPDATE verification_jobs SET status='failed',lease_owner=NULL,lease_until=NULL,last_error='provider_unavailable' WHERE id=$1`, job)
		expectDialogueSQLReject(t, ctx, pool, "23514", `UPDATE verification_jobs SET attempts=0 WHERE id=$1`, job)
	})
	t.Run("idempotency-and-criteria", func(t *testing.T) {
		expectDialogueSQLReject(t, ctx, pool, "23505", `INSERT INTO verification_messages(id,tenant_id,attempt_id,question_id,policy_version,snapshot_digest,sequence,expected_version,role,content,client_message_id) VALUES($1,$2,$3,$4,'dialogue.v1',$5,2,4,'student','replacement text','message-one')`, uuid.NewString(), f.tenant, f.attempt, question, f.snapshot.Digest)
		for _, criteria := range []string{`["unapproved"]`, `["source detail","source detail"]`, `[7]`} {
			expectDialogueSQLReject(t, ctx, pool, "23514", `INSERT INTO verification_evaluations(id,tenant_id,attempt_id,question_id,message_id,job_id,policy_version,snapshot_digest,version,accepted,rationale,provider,model,criteria) VALUES($1,$2,$3,$4,$5,$6,'dialogue.v1',$7,5,true,'Answer addresses the current question using the assigned source.','scripted','fixture',$8)`, uuid.NewString(), f.tenant, f.attempt, question, message, job, f.snapshot.Digest, criteria)
		}
		expectDialogueSQLReject(t, ctx, pool, "23505", `INSERT INTO verification_evaluations(id,tenant_id,attempt_id,question_id,message_id,job_id,policy_version,snapshot_digest,version,accepted,rationale,provider,model,criteria) VALUES($1,$2,$3,$4,$5,$6,'dialogue.v1',$7,5,true,'Answer addresses the current question using the assigned source.','scripted','fixture','["source detail"]')`, uuid.NewString(), f.tenant, f.attempt, question, message, job, f.snapshot.Digest)
		execDialogueSQL(t, ctx, pool, `INSERT INTO verification_events(tenant_id,attempt_id,sequence,event_key,kind,payload) VALUES($1,$2,2,'completion','complete','{}')`, f.tenant, f.attempt)
		expectDialogueSQLReject(t, ctx, pool, "23505", `INSERT INTO verification_events(tenant_id,attempt_id,sequence,event_key,kind,payload) VALUES($1,$2,3,'another-completion','complete','{}')`, f.tenant, f.attempt)
		// A partial schema proof never implies the task was completed.
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM task_occurrences WHERE id=$1`, f.occurrence).Scan(&status); err != nil || status != "pending" {
			t.Fatal("schema inserted a fake completion")
		}
	})
}

type dialogueFixture struct {
	tenant, student, otherStudent, session, otherSession, revision, requirement, schedule, occurrence, attempt string
	snapshot                                                                                                   domain.DialogueSnapshot
}

func seedDialogueLegacy(t *testing.T, ctx context.Context, pool *pgxpool.Pool) dialogueFixture {
	t.Helper()
	f := dialogueFixture{tenant: uuid.NewString(), student: uuid.NewString(), otherStudent: uuid.NewString(), session: uuid.NewString(), otherSession: uuid.NewString(), revision: uuid.NewString(), requirement: uuid.NewString(), schedule: uuid.NewString(), occurrence: uuid.NewString(), attempt: uuid.NewString()}
	config := domain.DialogueConfig{SourceRef: "fixture://chapter-4", LearningFocus: "Three source facts", RequiredQuestions: 3, Rubric: []string{"source detail"}, AllowedFollowUps: 1, MaxAttempts: 2, MaxTurns: 8, RetentionPolicy: "retain"}
	var err error
	f.snapshot, err = domain.NewDialogueSnapshot(f.revision, f.requirement, 1, config)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domain.SnapshotDialogueConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	envelope, _ := json.Marshal([]domain.VerificationRequirement{{Kind: "agent_dialogue", ConfigVersion: 1, Interaction: "chat", Executor: "fantasy", Config: canonical}})
	template, manualReq, manualAttempt := uuid.NewString(), uuid.NewString(), uuid.NewString()
	execDialogueSQL(t, ctx, pool, `INSERT INTO tenants(id,name) VALUES($1,'P4 owned synthetic household')`, f.tenant)
	execDialogueSQL(t, ctx, pool, `INSERT INTO parent_memberships(tenant_id,subject_ref,role) VALUES($1,'local-parent','admin')`, f.tenant)
	execDialogueSQL(t, ctx, pool, `INSERT INTO parent_identities(issuer,subject,tenant_id,subject_ref) VALUES('https://fixture.invalid','fixture-parent',$1,'local-parent')`, f.tenant)
	execDialogueSQL(t, ctx, pool, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES($1,$2,'local-parent','parent',now()+interval '1 hour')`, []byte("synthetic-parent-hash"), f.tenant)
	for _, student := range []string{f.student, f.otherStudent} {
		execDialogueSQL(t, ctx, pool, `INSERT INTO students(id,tenant_id,display_name) VALUES($1::uuid,$2,$1::text)`, student, f.tenant)
	}
	for _, pair := range [][2]string{{f.session, f.student}, {f.otherSession, f.otherStudent}} {
		execDialogueSQL(t, ctx, pool, `INSERT INTO student_sessions(id,tenant_id,student_id,handle_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, pair[0], f.tenant, pair[1], []byte(pair[0]))
	}
	execDialogueSQL(t, ctx, pool, `INSERT INTO student_devices(id,tenant_id,student_id,token_hash) VALUES($1,$2,$3,$4)`, uuid.NewString(), f.tenant, f.student, []byte("synthetic-device-hash"))
	execDialogueSQL(t, ctx, pool, `INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'Original issued work','published',1)`, template, f.tenant)
	execDialogueSQL(t, ctx, pool, `INSERT INTO task_revisions(id,tenant_id,template_id,version,title,instructions,status) VALUES($1,$2,$3,1,'Original issued work','Read the assigned source','published')`, f.revision, f.tenant, template)
	execDialogueSQL(t, ctx, pool, `INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,0,'agent_dialogue',1,$4,'chat','fantasy')`, f.requirement, f.tenant, f.revision, envelope)
	execDialogueSQL(t, ctx, pool, `INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'parent_approval',1,'[{"kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}]','parent_action','human')`, manualReq, f.tenant, f.revision)
	execDialogueSQL(t, ctx, pool, `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, f.schedule, f.tenant, f.student, template, f.revision)
	execDialogueSQL(t, ctx, pool, `INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,revision_snapshot) VALUES($1,$2,$3,$4,$5,now(),now(),'{}')`, f.occurrence, f.tenant, f.schedule, f.student, f.revision)
	for _, pair := range [][2]string{{f.attempt, f.requirement}, {manualAttempt, manualReq}} {
		execDialogueSQL(t, ctx, pool, `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, pair[0], f.tenant, f.occurrence, pair[1])
	}
	execDialogueSQL(t, ctx, pool, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,false,'Retained manual rejection','local-parent')`, uuid.NewString(), f.tenant, manualAttempt)
	conversation, message, run := uuid.NewString(), uuid.NewString(), uuid.NewString()
	execDialogueSQL(t, ctx, pool, `INSERT INTO agent_conversations(id,tenant_id,actor_id,status,policy_version) VALUES($1,$2,'local-parent','active','parent.v1')`, conversation, f.tenant)
	execDialogueSQL(t, ctx, pool, `INSERT INTO agent_messages(id,tenant_id,conversation_id,client_message_id,role,content,sequence) VALUES($1,$2,$3,'preserved-command','user','Preserved original parent command',1)`, message, f.tenant, conversation)
	execDialogueSQL(t, ctx, pool, `INSERT INTO agent_runs(id,tenant_id,conversation_id,user_message_id,status,max_steps,max_tokens,deadline) VALUES($1,$2,$3,$4,'awaiting_confirmation',8,1024,now()+interval '5 minutes')`, run, f.tenant, conversation, message)
	execDialogueSQL(t, ctx, pool, `INSERT INTO agent_jobs(id,tenant_id,run_id,kind,status,max_attempts) VALUES($1,$2,$3,'agent_run','queued',3)`, uuid.NewString(), f.tenant, run)
	execDialogueSQL(t, ctx, pool, `INSERT INTO agent_run_parent_authority(tenant_id,run_id,issuer,subject,session_id) VALUES($1,$2,'https://fixture.invalid','fixture-parent','opaque-provider-session')`, f.tenant, run)
	execDialogueSQL(t, ctx, pool, `INSERT INTO agent_tool_effects(tenant_id,run_id,step,tool_name,action_digest,status,result) VALUES($1,$2,1,'draft_task',$3,'applied','{"title":"Preserved original"}')`, f.tenant, run, strings.Repeat("a", 64))
	execDialogueSQL(t, ctx, pool, `INSERT INTO parent_confirmation_previews(id,tenant_id,actor_id,handle_hash,action_kind,action_digest,action,summary,expires_at,run_id,tool_step) VALUES($1,$2,'local-parent',$3,'draft_task',$4,'{}','Preserved preview',now()+interval '5 minutes',$5,2)`, uuid.NewString(), f.tenant, []byte("synthetic-handle-hash"), make([]byte, 32), run)
	execDialogueSQL(t, ctx, pool, `INSERT INTO agent_run_events(run_id,tenant_id,sequence,event_type,payload) VALUES($1,$2,1,'tool_progress','{"label":"Prepare change"}')`, run, f.tenant)
	return f
}

func insertDialogueProjection(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f dialogueFixture) {
	t.Helper()
	snapshot, err := json.Marshal(f.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	execDialogueSQL(t, ctx, pool, `INSERT INTO dialogue_revision_policies(tenant_id,revision_id,requirement_id,snapshot_digest,snapshot) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,requirement_id) DO NOTHING`, f.tenant, f.revision, f.requirement, f.snapshot.Digest, snapshot)
	execDialogueSQL(t, ctx, pool, `INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,student_id,revision_id,policy_version,snapshot_digest,config_snapshot) VALUES($1,$2,$3,$4,$5,$6,'dialogue.v1',$7,$8)`, f.tenant, f.attempt, f.occurrence, f.requirement, f.student, f.revision, f.snapshot.Digest, snapshot)
}
func execDialogueSQL(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatalf("synthetic SQL setup: %v", err)
	}
}
func expectDialogueSQLReject(t *testing.T, ctx context.Context, pool *pgxpool.Pool, code, query string, args ...any) {
	t.Helper()
	_, err := pool.Exec(ctx, query, args...)
	pgerr, ok := err.(*pgconn.PgError)
	if !ok || pgerr.Code != code {
		t.Fatalf("expected SQLSTATE %s, got %v", code, err)
	}
}
func dialogueLegacyDigests(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range []string{"tenants", "parent_memberships", "parent_identities", "parent_session_revocations", "bff_sessions", "students", "student_sessions", "student_devices", "task_templates", "task_revisions", "verification_requirements", "task_schedules", "task_occurrences", "verification_attempts", "verification_decisions", "agent_conversations", "agent_messages", "agent_runs", "agent_jobs", "agent_run_parent_authority", "agent_tool_effects", "parent_confirmation_previews", "agent_run_events", "tasks_schema_migrations"} {
		where := ""
		if table == "tasks_schema_migrations" {
			where = " WHERE version<='00009_parent_agent_authority.sql'"
		}
		var raw string
		if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM %s t%s`, table, where)).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		out[table] = fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
	}
	return out
}
