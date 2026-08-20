package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"primer-tasks/internal/artifactstore"
)

func TestMalformedArtifactFinalizeReleasesRetrySlot(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	tenant, student, template, revision, requirement, schedule, occurrence := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	cleanupQueries := []string{
		"DELETE FROM artifact_criterion_evaluations WHERE tenant_id=$1", "DELETE FROM artifact_rubric_jobs WHERE tenant_id=$1", "DELETE FROM artifact_submissions WHERE tenant_id=$1", "DELETE FROM artifact_upload_parts WHERE tenant_id=$1", "DELETE FROM artifact_upload_reservations WHERE tenant_id=$1", "DELETE FROM artifact_derivatives WHERE tenant_id=$1", "DELETE FROM artifact_scans WHERE tenant_id=$1", "DELETE FROM artifact_retention WHERE tenant_id=$1", "DELETE FROM artifacts WHERE tenant_id=$1", "DELETE FROM verification_decisions WHERE tenant_id=$1", "DELETE FROM verification_attempts WHERE tenant_id=$1", "DELETE FROM task_occurrences WHERE tenant_id=$1", "DELETE FROM verification_requirements WHERE tenant_id=$1", "DELETE FROM task_schedules WHERE tenant_id=$1", "DELETE FROM task_revisions WHERE tenant_id=$1", "DELETE FROM task_templates WHERE tenant_id=$1", "DELETE FROM students WHERE tenant_id=$1", "DELETE FROM tenants WHERE id=$1",
	}
	t.Cleanup(func() {
		for _, query := range cleanupQueries {
			_, _ = pool.Exec(context.Background(), query, tenant)
		}
	})
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO tenants(id,name) VALUES($1,'artifact retry')`, tenant)
	mustExec(`INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,'Student')`, student, tenant)
	mustExec(`INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,'Poem','published',1)`, template, tenant)
	mustExec(`INSERT INTO task_revisions(id,tenant_id,template_id,version,title,status) VALUES($1,$2,$3,1,'Poem','published')`, revision, tenant, template)
	config := `{"acceptedKinds":["image"],"maxBytes":10000,"criteria":[{"id":"shows-work","label":"Shows work","description":"The image shows the poem.","required":true}],"passRule":"all_required","reviewPolicy":"parent_review"}`
	mustExec(`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_artifact_rubric',1,$4,'artifact_upload','fantasy')`, requirement, tenant, revision, config)
	mustExec(`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, schedule, tenant, student, template, revision)
	mustExec(`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification')`, occurrence, tenant, schedule, student, revision)
	mustExec(`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, uuid.New(), tenant, occurrence, requirement)
	store, err := artifactstore.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := NewWithStore(pool, "test", store)
	reserve := func(idem string) artifactReservationOutput {
		t.Helper()
		body, _ := json.Marshal(artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "image", Filename: "poem.png", ContentType: "image/png", Size: 9, IdempotencyKey: idem})
		req := httptest.NewRequest(http.MethodPost, "/student/artifacts/reserve", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		s.reserveArtifact(rec, req, student)
		if rec.Code != http.StatusCreated {
			t.Fatalf("reserve status=%d body=%s", rec.Code, rec.Body.String())
		}
		var out artifactReservationOutput
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(out.UploadURL, "/student/artifacts/") || strings.Contains(out.UploadURL, "minio") || strings.Contains(out.UploadURL, "X-Amz") || strings.Contains(out.UploadURL, "tenants/") {
			t.Fatalf("browser upload contract leaked object-store URL: %q", out.UploadURL)
		}
		return out
	}
	first := reserve("first-upload")
	bad := []byte("not image")
	sum := sha256.Sum256(bad)
	if _, err := store.Put(ctx, uploadKey(tenant, uuid.MustParse(first.ArtifactID)), "image/png", bytes.NewReader(bad), int64(len(bad))); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(artifactFinalizeInput{ArtifactID: first.ArtifactID, OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: hex.EncodeToString(sum[:]), IdempotencyKey: first.IdempotencyKey})
	req := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.finalizeArtifact(rec, req, student)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed finalize status=%d body=%s", rec.Code, rec.Body.String())
	}
	var reservationStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM artifact_upload_reservations WHERE tenant_id=$1 AND artifact_id=$2`, tenant, first.ArtifactID).Scan(&reservationStatus); err != nil {
		t.Fatal(err)
	}
	if reservationStatus != "canceled" {
		t.Fatalf("reservation status=%s, want canceled", reservationStatus)
	}
	_ = reserve("replacement-upload")
}
