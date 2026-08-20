package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
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
	config := `{"acceptedKinds":["image"],"maxBytes":10000,"maxCount":3,"criteria":[{"id":"shows-work","label":"Shows work","description":"The image shows the poem.","required":true}],"passRule":"all_required","reviewPolicy":"parent_review"}`
	mustExec(`INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_artifact_rubric',1,$4,'artifact_upload','fantasy')`, requirement, tenant, revision, config)
	mustExec(`INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, schedule, tenant, student, template, revision)
	mustExec(`INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification')`, occurrence, tenant, schedule, student, revision)
	mustExec(`INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, uuid.New(), tenant, occurrence, requirement)
	store, err := artifactstore.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := NewWithStore(pool, "test", store)
	reserve := func(idem string, size int64) artifactReservationOutput {
		t.Helper()
		body, _ := json.Marshal(artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "image", Filename: "poem.png", ContentType: "image/png", Size: size, IdempotencyKey: idem})
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
		if !strings.HasPrefix(out.UploadURL, "/api/student/artifacts/") || strings.Contains(out.UploadURL, "minio") || strings.Contains(out.UploadURL, "X-Amz") || strings.Contains(out.UploadURL, "tenants/") {
			t.Fatalf("browser upload contract leaked object-store URL: %q", out.UploadURL)
		}
		return out
	}
	first := reserve("first-upload", 9)
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
	var valid bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	if err := png.Encode(&valid, img); err != nil {
		t.Fatal(err)
	}
	validSum := sha256.Sum256(valid.Bytes())
	replacement := reserve("replacement-upload", int64(valid.Len()))
	replacementArtifact := uuid.MustParse(replacement.ArtifactID)
	if _, err := store.Put(ctx, uploadKey(tenant, replacementArtifact), "image/png", bytes.NewReader(valid.Bytes()), int64(valid.Len())); err != nil {
		t.Fatal(err)
	}
	finalBody, _ := json.Marshal(artifactFinalizeInput{ArtifactID: replacement.ArtifactID, OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: hex.EncodeToString(validSum[:]), IdempotencyKey: replacement.IdempotencyKey})
	finalReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(finalBody))
	finalRec := httptest.NewRecorder()
	s.finalizeArtifact(finalRec, finalReq, student)
	if finalRec.Code != http.StatusOK {
		t.Fatalf("valid finalize status=%d body=%s", finalRec.Code, finalRec.Body.String())
	}
	streamed := reserve("streamed-upload", int64(valid.Len()))
	streamRoute := chi.NewRouteContext()
	streamRoute.URLParams.Add("id", streamed.ArtifactID)
	streamReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+streamed.ArtifactID+"/upload", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, streamRoute))
	streamReq.Header.Set("Content-Type", "image/png")
	streamRec := httptest.NewRecorder()
	s.uploadArtifact(streamRec, streamReq, student)
	if streamRec.Code != http.StatusOK {
		t.Fatalf("bounded upload status=%d body=%s", streamRec.Code, streamRec.Body.String())
	}
	partBody, _ := json.Marshal(artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "image", Filename: "parts.png", ContentType: "image/png", Size: int64(valid.Len()), PartCount: 2, IdempotencyKey: "parts-upload"})
	partReserveReq := httptest.NewRequest(http.MethodPost, "/student/artifacts/reserve", bytes.NewReader(partBody))
	partReserveRec := httptest.NewRecorder()
	s.reserveArtifact(partReserveRec, partReserveReq, student)
	if partReserveRec.Code != http.StatusCreated {
		t.Fatalf("part reserve status=%d", partReserveRec.Code)
	}
	var partReservation artifactReservationOutput
	_ = json.Unmarshal(partReserveRec.Body.Bytes(), &partReservation)
	partRoute := chi.NewRouteContext()
	partRoute.URLParams.Add("id", partReservation.ArtifactID)
	partRoute.URLParams.Add("part", "3")
	partReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+partReservation.ArtifactID+"/parts/3", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, partRoute))
	partRec := httptest.NewRecorder()
	s.uploadArtifactPart(partRec, partReq, student)
	if partRec.Code != http.StatusBadRequest {
		t.Fatalf("part bounds status=%d body=%s", partRec.Code, partRec.Body.String())
	}
	workerCtx, workerCancel := context.WithCancel(ctx)
	s.StartArtifactWorker(workerCtx)
	workerCancel()
	t.Setenv("TASKS_AGENT_MODE", "scripted")
	t.Setenv("TASKS_ARTIFACT_SCRIPTED_FIXTURE", "1")
	t.Setenv("TASKS_ARTIFACT_SCRIPTED_DIGEST", hex.EncodeToString(validSum[:]))
	time.Sleep(1100 * time.Millisecond)
	if err := s.runArtifactStep(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.finishArtifactReview(ctx, uuid.NewString(), tenant.String(), uuid.NewString(), "review fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.failArtifactJob(ctx, uuid.NewString(), tenant.String(), uuid.NewString(), "failure fixture"); err != nil {
		t.Fatal(err)
	}
	retryRoute := chi.NewRouteContext()
	retryRoute.URLParams.Add("occurrence", occurrence.String())
	retryReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/retry", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, retryRoute))
	retryRec := httptest.NewRecorder()
	s.retryArtifactEvaluation(retryRec, retryReq, student)
	if retryRec.Code != http.StatusOK {
		t.Fatalf("artifact retry status=%d body=%s", retryRec.Code, retryRec.Body.String())
	}
	stateRoute := chi.NewRouteContext()
	stateRoute.URLParams.Add("occurrence", occurrence.String())
	stateReq := httptest.NewRequest(http.MethodGet, "/student/occurrences/"+occurrence.String()+"/artifacts", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, stateRoute))
	stateRec := httptest.NewRecorder()
	s.studentArtifactState(stateRec, stateReq, student)
	if stateRec.Code != http.StatusOK || !strings.Contains(stateRec.Body.String(), "scripted-fixture") {
		t.Fatalf("artifact state status=%d body=%s", stateRec.Code, stateRec.Body.String())
	}
	derivativeRoute := chi.NewRouteContext()
	derivativeRoute.URLParams.Add("occurrence", occurrence.String())
	derivativeRoute.URLParams.Add("id", replacement.ArtifactID)
	derivativeRoute.URLParams.Add("kind", "thumbnail")
	derivativeReq := httptest.NewRequest(http.MethodGet, "/student/artifacts/"+replacement.ArtifactID+"/derivative/thumbnail", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, derivativeRoute))
	derivativeRec := httptest.NewRecorder()
	s.studentDerivative(derivativeRec, derivativeReq, student)
	if derivativeRec.Code != http.StatusOK {
		t.Fatalf("student derivative status=%d", derivativeRec.Code)
	}
	parentRoute := chi.NewRouteContext()
	parentRoute.URLParams.Add("id", replacement.ArtifactID)
	parentReq := httptest.NewRequest(http.MethodGet, "/parent/artifacts/"+replacement.ArtifactID+"/original", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, parentRoute))
	parentRec := httptest.NewRecorder()
	s.parentOriginal(parentRec, parentReq, scope{Tenant: tenant.String()})
	if parentRec.Code != http.StatusOK {
		t.Fatalf("parent original status=%d", parentRec.Code)
	}
	parentStateRoute := chi.NewRouteContext()
	parentStateRoute.URLParams.Add("occurrence", occurrence.String())
	parentStateReq := httptest.NewRequest(http.MethodGet, "/occurrences/"+occurrence.String()+"/artifacts/inspect", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, parentStateRoute))
	parentStateRec := httptest.NewRecorder()
	s.parentArtifactState(parentStateRec, parentStateReq, scope{Tenant: tenant.String()})
	if parentStateRec.Code != http.StatusOK {
		t.Fatalf("parent artifact state status=%d", parentStateRec.Code)
	}
	parentDerivativeRoute := chi.NewRouteContext()
	parentDerivativeRoute.URLParams.Add("occurrence", occurrence.String())
	parentDerivativeRoute.URLParams.Add("id", replacement.ArtifactID)
	parentDerivativeReq := httptest.NewRequest(http.MethodGet, "/occurrences/"+occurrence.String()+"/artifacts/"+replacement.ArtifactID+"/derivative", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, parentDerivativeRoute))
	parentDerivativeRec := httptest.NewRecorder()
	s.parentDerivative(parentDerivativeRec, parentDerivativeReq, scope{Tenant: tenant.String()})
	if parentDerivativeRec.Code != http.StatusOK {
		t.Fatalf("parent derivative status=%d", parentDerivativeRec.Code)
	}
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}
