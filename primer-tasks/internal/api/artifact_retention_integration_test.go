package api

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/artifactstore"
)

func TestArtifactCleanupDeletesExpiredOriginalsAndDerivativesIdempotently(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	tenant, student, template, revision, requirement, schedule, occurrence, attempt, artifactID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	expiredArtifactID, expiredReservationID := uuid.New(), uuid.New()
	cleanup := []string{"DELETE FROM artifact_rubric_events WHERE tenant_id=$1", "DELETE FROM artifact_criterion_evaluations WHERE tenant_id=$1", "DELETE FROM artifact_rubric_jobs WHERE tenant_id=$1", "DELETE FROM artifact_submissions WHERE tenant_id=$1", "DELETE FROM artifact_upload_parts WHERE tenant_id=$1", "DELETE FROM artifact_upload_reservations WHERE tenant_id=$1", "DELETE FROM artifact_derivatives WHERE tenant_id=$1", "DELETE FROM artifact_retention WHERE tenant_id=$1", "DELETE FROM artifacts WHERE tenant_id=$1", "DELETE FROM verification_attempts WHERE tenant_id=$1", "DELETE FROM task_occurrences WHERE tenant_id=$1", "DELETE FROM verification_requirements WHERE tenant_id=$1", "DELETE FROM task_schedules WHERE tenant_id=$1", "DELETE FROM task_revisions WHERE tenant_id=$1", "DELETE FROM task_templates WHERE tenant_id=$1", "DELETE FROM students WHERE tenant_id=$1", "DELETE FROM tenants WHERE id=$1"}
	t.Cleanup(func() {
		for _, q := range cleanup {
			_, _ = pool.Exec(context.Background(), q, tenant)
		}
	})
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO tenants(id,name) VALUES($1,'retention')`, tenant)
	exec(`INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'Retention student')`, student, tenant)
	exec(`INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'Retention','published',1)`, template, tenant)
	exec(`INSERT INTO task_revisions(id,tenant_id,template_id,version,title,status) VALUES($1,$2,$3,1,'Retention','published')`, revision, tenant, template)
	exec(`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_artifact_rubric',1,'{}','artifact_upload','fantasy')`, requirement, tenant, revision)
	exec(`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, schedule, tenant, student, template, revision)
	exec(`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification')`, occurrence, tenant, schedule, student, revision)
	exec(`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, attempt, tenant, occurrence, requirement)
	root := t.TempDir()
	store, err := artifactstore.NewFS(root)
	if err != nil {
		t.Fatal(err)
	}
	s := NewWithStore(pool, "test", store)
	originalKey := uploadKey(tenant, artifactID)
	derivativeKey := "tenants/" + tenant.String() + "/artifacts/" + artifactID.String() + "/derivatives/thumbnail"
	exec(`INSERT INTO artifacts(id,tenant_id,student_id,object_key,kind,original_name,declared_content_type,detected_content_type,byte_size,sha256,status,expires_at) VALUES($1,$2,$3,$4,'image','x.png','image/png','image/png',4,'digest','finalized',now()-interval '2 days')`, artifactID, tenant, student, originalKey)
	exec(`INSERT INTO artifact_derivatives(id,tenant_id,artifact_id,derivative_kind,object_key,content_type,byte_size,sha256) VALUES($1,$2,$3,'thumbnail',$4,'image/png',4,'derivative')`, uuid.New(), tenant, artifactID, derivativeKey)
	exec(`INSERT INTO artifact_retention(tenant_id,artifact_id,retain_original_until,retain_derivatives_until) VALUES($1,$2,now()-interval '2 days',now()+interval '2 days')`, tenant, artifactID)
	if _, err := store.Put(ctx, originalKey, "image/png", bytes.NewReader([]byte("orig")), 4); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(ctx, derivativeKey, "image/png", bytes.NewReader([]byte("deriv")), 5); err != nil {
		t.Fatal(err)
	}
	expiredKey := uploadKey(tenant, expiredArtifactID)
	expiredPartKey := "tenants/" + tenant.String() + "/expired-part"
	exec(`INSERT INTO artifacts(id,tenant_id,student_id,object_key,kind,original_name,declared_content_type,byte_size,status,expires_at) VALUES($1,$2,$3,$4,'image','expired.png','image/png',5,'reserved',now()-interval '1 day')`, expiredArtifactID, tenant, student, expiredKey)
	exec(`INSERT INTO artifact_upload_reservations(id,tenant_id,student_id,artifact_id,occurrence_id,requirement_id,idempotency_key,part_count,expires_at) VALUES($1,$2,$3,$4,$5,$6,'expired-upload',1,now()-interval '1 day')`, expiredReservationID, tenant, student, expiredArtifactID, occurrence, requirement)
	exec(`INSERT INTO artifact_upload_parts(tenant_id,reservation_id,part_number,object_key,byte_size,sha256,uploaded_at) VALUES($1,$2,1,$3,5,'expired',now())`, tenant, expiredReservationID, expiredPartKey)
	if _, err := store.Put(ctx, expiredKey, "image/png", bytes.NewReader([]byte("origx")), 5); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(ctx, expiredPartKey, "image/png", bytes.NewReader([]byte("partx")), 5); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var status string
	noPartArtifactID := uuid.New()
	noPartKey := uploadKey(tenant, noPartArtifactID)
	exec(`INSERT INTO artifacts(id,tenant_id,student_id,object_key,kind,original_name,declared_content_type,byte_size,status,expires_at) VALUES($1,$2,$3,$4,'image','no-part.png','image/png',5,'uploaded',now()-interval '1 day')`, noPartArtifactID, tenant, student, noPartKey)
	if _, err := store.Put(ctx, noPartKey, "image/png", bytes.NewReader([]byte("noprt")), 5); err != nil {
		t.Fatal(err)
	}
	s.Artifacts = rejectingDeleteStore{Store: store}
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err == nil {
		t.Fatal("cleanup ignored expired artifact deletion failure")
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM artifacts WHERE tenant_id=$1 AND id=$2`, tenant, noPartArtifactID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "uploaded" {
		t.Fatalf("failed artifact cleanup changed status=%s", status)
	}
	s.Artifacts = store
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM artifacts WHERE tenant_id=$1 AND id=$2`, tenant, artifactID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "tombstoned" {
		t.Fatalf("status=%s", status)
	}
	var derivatives int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM artifact_derivatives WHERE tenant_id=$1 AND artifact_id=$2`, tenant, artifactID).Scan(&derivatives); err != nil {
		t.Fatal(err)
	}
	if derivatives != 1 {
		t.Fatalf("derivatives=%d before derivative retention expiry", derivatives)
	}
	if _, err := store.Stat(ctx, originalKey); err == nil {
		t.Fatal("expired original still exists")
	}
	if _, err := store.Stat(ctx, expiredKey); err == nil {
		t.Fatal("expired reservation object still exists")
	}
	if _, err := store.Stat(ctx, expiredPartKey); err == nil {
		t.Fatal("expired upload part still exists")
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM artifacts WHERE tenant_id=$1 AND id=$2`, tenant, expiredArtifactID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "tombstoned" {
		t.Fatalf("expired artifact status=%s", status)
	}
	var reservationStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM artifact_upload_reservations WHERE tenant_id=$1 AND id=$2`, tenant, expiredReservationID).Scan(&reservationStatus); err != nil {
		t.Fatal(err)
	}
	if reservationStatus != "expired" {
		t.Fatalf("expired reservation status=%s", reservationStatus)
	}
	if _, err := store.Stat(ctx, derivativeKey); err != nil {
		t.Fatal("derivative should survive original expiry")
	}
	exec(`UPDATE artifact_retention SET retain_derivatives_until=now()-interval '1 second' WHERE tenant_id=$1 AND artifact_id=$2`, tenant, artifactID)
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM artifact_derivatives WHERE tenant_id=$1 AND artifact_id=$2`, tenant, artifactID).Scan(&derivatives); err != nil {
		t.Fatal(err)
	}
	if derivatives != 0 {
		t.Fatalf("derivatives=%d after derivative retention expiry", derivatives)
	}
	if _, err := store.Stat(ctx, derivativeKey); err == nil {
		t.Fatal("expired derivative still exists")
	}
}
