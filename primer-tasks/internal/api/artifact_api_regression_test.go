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
	"os"
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
	reserveStatus := func(in artifactReservationInput) int {
		t.Helper()
		body, _ := json.Marshal(in)
		req := httptest.NewRequest(http.MethodPost, "/student/artifacts/reserve", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		s.reserveArtifact(rec, req, student)
		return rec.Code
	}
	baseReservation := artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "image", Filename: "poem.png", ContentType: "image/png", Size: 9, IdempotencyKey: "invalid-case"}
	for _, tc := range []struct {
		name string
		in   artifactReservationInput
	}{
		{"unsupported kind", func() artifactReservationInput { x := baseReservation; x.Kind = "document"; return x }()},
		{"zero size", func() artifactReservationInput { x := baseReservation; x.Size = 0; return x }()},
		{"long filename", func() artifactReservationInput { x := baseReservation; x.Filename = strings.Repeat("x", 256); return x }()},
		{"too many parts", func() artifactReservationInput { x := baseReservation; x.PartCount = 10001; return x }()},
		{"missing idempotency", func() artifactReservationInput { x := baseReservation; x.IdempotencyKey = ""; return x }()},
	} {
		if code := reserveStatus(tc.in); code != http.StatusBadRequest {
			t.Errorf("%s status=%d", tc.name, code)
		}
	}
	if code := reserveStatus(artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: uuid.NewString(), Kind: "image", Filename: "poem.png", ContentType: "image/png", Size: 9, IdempotencyKey: "missing-requirement"}); code != http.StatusNotFound {
		t.Errorf("missing requirement status=%d", code)
	}
	finalizeStatus := func(in artifactFinalizeInput) int {
		t.Helper()
		body, _ := json.Marshal(in)
		req := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		s.finalizeArtifact(rec, req, student)
		return rec.Code
	}
	if code := finalizeStatus(artifactFinalizeInput{ArtifactID: uuid.NewString(), OccurrenceID: occurrence.String(), RequirementID: requirement.String()}); code != http.StatusBadRequest {
		t.Errorf("missing finalize fields status=%d", code)
	}
	if code := finalizeStatus(artifactFinalizeInput{ArtifactID: "not-a-uuid", OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: "digest"}); code != http.StatusNotFound {
		t.Errorf("invalid finalize id status=%d", code)
	}
	if code := finalizeStatus(artifactFinalizeInput{ArtifactID: uuid.NewString(), OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: "digest"}); code != http.StatusNotFound {
		t.Errorf("unknown finalize artifact status=%d", code)
	}
	if code := finalizeStatus(artifactFinalizeInput{ArtifactID: uuid.NewString(), OccurrenceID: "not-a-uuid", RequirementID: requirement.String(), SHA256: "digest"}); code != http.StatusInternalServerError {
		t.Errorf("invalid finalize occurrence status=%d", code)
	}
	first := reserve("first-upload", 9)
	firstReplay := artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "image", Filename: "poem.png", ContentType: "image/png", Size: 9, IdempotencyKey: first.IdempotencyKey}
	if code := reserveStatus(firstReplay); code != http.StatusOK {
		t.Fatalf("reservation replay status=%d", code)
	}
	if code := reserveStatus(artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "image", Filename: "large.png", ContentType: "image/png", Size: 10001, IdempotencyKey: "too-large"}); code != http.StatusBadRequest {
		t.Fatalf("reservation size limit status=%d", code)
	}
	if code := reserveStatus(artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "video", Filename: "movie.mp4", ContentType: "video/mp4", Size: 9, IdempotencyKey: "wrong-kind"}); code != http.StatusBadRequest {
		t.Fatalf("reservation kind limit status=%d", code)
	}
	unavailable := reserve("unavailable-artifact", 9)
	mustExec(`UPDATE artifacts SET status='rejected' WHERE tenant_id=$1 AND id=$2`, tenant, unavailable.ArtifactID)
	if code := finalizeStatus(artifactFinalizeInput{ArtifactID: unavailable.ArtifactID, OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: "digest", IdempotencyKey: unavailable.IdempotencyKey}); code != http.StatusConflict {
		t.Fatalf("unavailable finalize status=%d", code)
	}
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
	over := reserve("oversized-stream", int64(valid.Len()))
	overRoute := chi.NewRouteContext()
	overRoute.URLParams.Add("id", over.ArtifactID)
	overReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+over.ArtifactID+"/upload", bytes.NewReader(append(valid.Bytes(), 'x'))).WithContext(context.WithValue(ctx, chi.RouteCtxKey, overRoute))
	overRec := httptest.NewRecorder()
	s.uploadArtifact(overRec, overReq, student)
	if overRec.Code != http.StatusBadRequest {
		t.Fatalf("oversized upload status=%d", overRec.Code)
	}
	short := reserve("short-stream", int64(valid.Len()))
	shortRoute := chi.NewRouteContext()
	shortRoute.URLParams.Add("id", short.ArtifactID)
	shortReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+short.ArtifactID+"/upload", bytes.NewReader(valid.Bytes()[:1])).WithContext(context.WithValue(ctx, chi.RouteCtxKey, shortRoute))
	shortRec := httptest.NewRecorder()
	s.uploadArtifact(shortRec, shortReq, student)
	if shortRec.Code != http.StatusBadRequest {
		t.Fatalf("short upload status=%d", shortRec.Code)
	}
	// Finalize and bounded upload are replay-safe, but malformed and multipart
	// routes fail before any object-store substitution can occur.
	replayReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(finalBody))
	replayRec := httptest.NewRecorder()
	s.finalizeArtifact(replayRec, replayReq, student)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("duplicate finalize status=%d body=%s", replayRec.Code, replayRec.Body.String())
	}
	invalidUploadRoute := chi.NewRouteContext()
	invalidUploadRoute.URLParams.Add("id", "not-a-uuid")
	invalidUploadReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/not-a-uuid/upload", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, invalidUploadRoute))
	invalidUploadRec := httptest.NewRecorder()
	s.uploadArtifact(invalidUploadRec, invalidUploadReq, student)
	if invalidUploadRec.Code != http.StatusNotFound {
		t.Fatalf("invalid upload id status=%d", invalidUploadRec.Code)
	}
	finalizedUploadRoute := chi.NewRouteContext()
	finalizedUploadRoute.URLParams.Add("id", replacement.ArtifactID)
	finalizedUploadReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+replacement.ArtifactID+"/upload", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, finalizedUploadRoute))
	finalizedUploadRec := httptest.NewRecorder()
	s.uploadArtifact(finalizedUploadRec, finalizedUploadReq, student)
	if finalizedUploadRec.Code != http.StatusOK || !strings.Contains(finalizedUploadRec.Body.String(), "already_uploaded") {
		t.Fatalf("finalized upload status=%d body=%s", finalizedUploadRec.Code, finalizedUploadRec.Body.String())
	}
	storeFailure := reserve("store-failure", int64(valid.Len()))
	storeFailureRoute := chi.NewRouteContext()
	storeFailureRoute.URLParams.Add("id", storeFailure.ArtifactID)
	storeFailureReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+storeFailure.ArtifactID+"/upload", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, storeFailureRoute))
	originalStore := s.Artifacts
	s.Artifacts = rejectingPutStore{Store: originalStore}
	storeFailureRec := httptest.NewRecorder()
	s.uploadArtifact(storeFailureRec, storeFailureReq, student)
	s.Artifacts = originalStore
	if storeFailureRec.Code != http.StatusBadRequest {
		t.Fatalf("store failure upload status=%d body=%s", storeFailureRec.Code, storeFailureRec.Body.String())
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
	multipartUploadRoute := chi.NewRouteContext()
	multipartUploadRoute.URLParams.Add("id", partReservation.ArtifactID)
	multipartUploadReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+partReservation.ArtifactID+"/upload", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, multipartUploadRoute))
	multipartUploadRec := httptest.NewRecorder()
	s.uploadArtifact(multipartUploadRec, multipartUploadReq, student)
	if multipartUploadRec.Code != http.StatusConflict {
		t.Fatalf("multipart bounded upload status=%d", multipartUploadRec.Code)
	}
	missingPartsBody, _ := json.Marshal(artifactFinalizeInput{ArtifactID: partReservation.ArtifactID, OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: hex.EncodeToString(validSum[:]), IdempotencyKey: partReservation.IdempotencyKey})
	missingPartsReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(missingPartsBody))
	missingPartsRec := httptest.NewRecorder()
	s.finalizeArtifact(missingPartsRec, missingPartsReq, student)
	if missingPartsRec.Code != http.StatusConflict {
		t.Fatalf("missing parts finalize status=%d body=%s", missingPartsRec.Code, missingPartsRec.Body.String())
	}
	invalidPartRoute := chi.NewRouteContext()
	invalidPartRoute.URLParams.Add("id", partReservation.ArtifactID)
	invalidPartRoute.URLParams.Add("part", "bad")
	invalidPartReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+partReservation.ArtifactID+"/parts/bad", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, invalidPartRoute))
	invalidPartRec := httptest.NewRecorder()
	s.uploadArtifactPart(invalidPartRec, invalidPartReq, student)
	if invalidPartRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid part number status=%d", invalidPartRec.Code)
	}
	zeroPartRoute := chi.NewRouteContext()
	zeroPartRoute.URLParams.Add("id", partReservation.ArtifactID)
	zeroPartRoute.URLParams.Add("part", "0")
	zeroPartReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+partReservation.ArtifactID+"/parts/0", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, zeroPartRoute))
	zeroPartRec := httptest.NewRecorder()
	s.uploadArtifactPart(zeroPartRec, zeroPartReq, student)
	if zeroPartRec.Code != http.StatusBadRequest {
		t.Fatalf("zero part number status=%d", zeroPartRec.Code)
	}
	partRoute := chi.NewRouteContext()
	partRoute.URLParams.Add("id", partReservation.ArtifactID)
	partRoute.URLParams.Add("part", "3")
	partReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+partReservation.ArtifactID+"/parts/3", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, partRoute))
	partRec := httptest.NewRecorder()
	s.uploadArtifactPart(partRec, partReq, student)
	if partRec.Code != http.StatusBadRequest {
		t.Fatalf("part bounds status=%d body=%s", partRec.Code, partRec.Body.String())
	}
	emptyPartRoute := chi.NewRouteContext()
	emptyPartRoute.URLParams.Add("id", partReservation.ArtifactID)
	emptyPartRoute.URLParams.Add("part", "1")
	emptyPartReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+partReservation.ArtifactID+"/parts/1", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, emptyPartRoute))
	emptyPartRec := httptest.NewRecorder()
	s.uploadArtifactPart(emptyPartRec, emptyPartReq, student)
	if emptyPartRec.Code != http.StatusBadRequest {
		t.Fatalf("empty part status=%d", emptyPartRec.Code)
	}
	finalizedPartRoute := chi.NewRouteContext()
	finalizedPartRoute.URLParams.Add("id", replacement.ArtifactID)
	finalizedPartRoute.URLParams.Add("part", "1")
	finalizedPartReq := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+replacement.ArtifactID+"/parts/1", bytes.NewReader(valid.Bytes())).WithContext(context.WithValue(ctx, chi.RouteCtxKey, finalizedPartRoute))
	finalizedPartRec := httptest.NewRecorder()
	s.uploadArtifactPart(finalizedPartRec, finalizedPartReq, student)
	if finalizedPartRec.Code != http.StatusNotFound {
		t.Fatalf("finalized part status=%d", finalizedPartRec.Code)
	}
	for _, tc := range []struct {
		part string
		body []byte
	}{
		{"1", valid.Bytes()[:valid.Len()/2]},
		{"2", valid.Bytes()[valid.Len()/2:]},
	} {
		route := chi.NewRouteContext()
		route.URLParams.Add("id", partReservation.ArtifactID)
		route.URLParams.Add("part", tc.part)
		req := httptest.NewRequest(http.MethodPut, "/student/artifacts/"+partReservation.ArtifactID+"/parts/"+tc.part, bytes.NewReader(tc.body)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
		rec := httptest.NewRecorder()
		s.uploadArtifactPart(rec, req, student)
		if rec.Code != http.StatusOK {
			t.Fatalf("part %s status=%d body=%s", tc.part, rec.Code, rec.Body.String())
		}
	}
	if code := reserveStatus(artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "image", Filename: "fourth.png", ContentType: "image/png", Size: int64(valid.Len()), IdempotencyKey: "count-limit"}); code != http.StatusConflict {
		t.Fatalf("count limit status=%d", code)
	}
	// The remaining lifecycle checks use a larger count limit so they do not
	// mask the count-limit assertion above.
	mustExec(`UPDATE verification_requirements SET config=jsonb_set(config,'{maxCount}','10'::jsonb) WHERE tenant_id=$1 AND id=$2`, tenant, requirement)
	missing := reserve("missing-object", int64(valid.Len()))
	missingBody, _ := json.Marshal(artifactFinalizeInput{ArtifactID: missing.ArtifactID, OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: hex.EncodeToString(validSum[:]), IdempotencyKey: missing.IdempotencyKey})
	missingObjectReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(missingBody))
	missingObjectRec := httptest.NewRecorder()
	s.finalizeArtifact(missingObjectRec, missingObjectReq, student)
	if missingObjectRec.Code != http.StatusBadRequest {
		t.Fatalf("missing object finalize status=%d body=%s", missingObjectRec.Code, missingObjectRec.Body.String())
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
	retryRoute := chi.NewRouteContext()
	retryRoute.URLParams.Add("occurrence", occurrence.String())
	retryReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/retry", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, retryRoute))
	retryRec := httptest.NewRecorder()
	s.retryArtifactEvaluation(retryRec, retryReq, student)
	if retryRec.Code != http.StatusConflict {
		t.Fatalf("terminal artifact retry status=%d body=%s", retryRec.Code, retryRec.Body.String())
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
	invalidDerivativeRoute := chi.NewRouteContext()
	invalidDerivativeRoute.URLParams.Add("id", "not-a-uuid")
	invalidDerivativeReq := httptest.NewRequest(http.MethodGet, "/student/artifacts/not-a-uuid/derivative/thumbnail", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, invalidDerivativeRoute))
	invalidDerivativeRec := httptest.NewRecorder()
	s.studentDerivative(invalidDerivativeRec, invalidDerivativeReq, student)
	if invalidDerivativeRec.Code != http.StatusNotFound {
		t.Fatalf("invalid student derivative status=%d", invalidDerivativeRec.Code)
	}
	parentRoute := chi.NewRouteContext()
	parentRoute.URLParams.Add("id", replacement.ArtifactID)
	parentReq := httptest.NewRequest(http.MethodGet, "/parent/artifacts/"+replacement.ArtifactID+"/original", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, parentRoute))
	parentRec := httptest.NewRecorder()
	s.parentOriginal(parentRec, parentReq, scope{Tenant: tenant.String()})
	if parentRec.Code != http.StatusOK {
		t.Fatalf("parent original status=%d", parentRec.Code)
	}
	missingOriginalRoute := chi.NewRouteContext()
	missingOriginalRoute.URLParams.Add("id", uuid.NewString())
	missingOriginalReq := httptest.NewRequest(http.MethodGet, "/parent/artifacts/missing/original", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, missingOriginalRoute))
	missingOriginalRec := httptest.NewRecorder()
	s.parentOriginal(missingOriginalRec, missingOriginalReq, scope{Tenant: tenant.String()})
	if missingOriginalRec.Code != http.StatusNotFound {
		t.Fatalf("missing parent original status=%d", missingOriginalRec.Code)
	}
	invalidOriginalReq := httptest.NewRequest(http.MethodGet, "/parent/artifacts/not-a-uuid/original", nil)
	invalidOriginalRec := httptest.NewRecorder()
	s.parentOriginal(invalidOriginalRec, invalidOriginalReq, scope{Tenant: tenant.String()})
	if invalidOriginalRec.Code != http.StatusNotFound {
		t.Fatalf("invalid parent original status=%d", invalidOriginalRec.Code)
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
	missingDerivativeID := uuid.New()
	mustExec(`INSERT INTO artifact_derivatives(id,tenant_id,artifact_id,derivative_kind,object_key,content_type,byte_size,sha256) VALUES($1,$2,$3,'preview',$4,'image/png',1,'missing')`, missingDerivativeID, tenant, replacementArtifact, "tenants/"+tenant.String()+"/artifacts/"+replacementArtifact.String()+"/missing-preview")
	missingDerivativeRoute := chi.NewRouteContext()
	missingDerivativeRoute.URLParams.Add("id", replacement.ArtifactID)
	missingDerivativeRoute.URLParams.Add("kind", "preview")
	missingDerivativeReq := httptest.NewRequest(http.MethodGet, "/student/artifacts/"+replacement.ArtifactID+"/derivative/preview", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, missingDerivativeRoute))
	missingDerivativeRec := httptest.NewRecorder()
	s.studentDerivative(missingDerivativeRec, missingDerivativeReq, student)
	if missingDerivativeRec.Code != http.StatusNotFound {
		t.Fatalf("missing student derivative status=%d", missingDerivativeRec.Code)
	}
	missingParentObjectRoute := chi.NewRouteContext()
	missingParentObjectRoute.URLParams.Add("occurrence", occurrence.String())
	missingParentObjectRoute.URLParams.Add("id", replacement.ArtifactID)
	missingParentObjectReq := httptest.NewRequest(http.MethodGet, "/occurrences/"+occurrence.String()+"/artifacts/"+replacement.ArtifactID+"/derivative", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, missingParentObjectRoute))
	missingParentObjectRec := httptest.NewRecorder()
	s.parentDerivative(missingParentObjectRec, missingParentObjectReq, scope{Tenant: tenant.String()})
	if missingParentObjectRec.Code != http.StatusNotFound {
		t.Fatalf("missing parent derivative object status=%d", missingParentObjectRec.Code)
	}
	missingParentDerivativeRoute := chi.NewRouteContext()
	missingParentDerivativeRoute.URLParams.Add("occurrence", occurrence.String())
	missingParentDerivativeRoute.URLParams.Add("id", uuid.NewString())
	missingParentDerivativeReq := httptest.NewRequest(http.MethodGet, "/occurrences/"+occurrence.String()+"/artifacts/missing/derivative", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, missingParentDerivativeRoute))
	missingParentDerivativeRec := httptest.NewRecorder()
	s.parentDerivative(missingParentDerivativeRec, missingParentDerivativeReq, scope{Tenant: tenant.String()})
	if missingParentDerivativeRec.Code != http.StatusNotFound {
		t.Fatalf("missing parent derivative status=%d", missingParentDerivativeRec.Code)
	}
	invalidParentDerivativeReq := httptest.NewRequest(http.MethodGet, "/occurrences/"+occurrence.String()+"/artifacts/not-a-uuid/derivative", nil)
	invalidParentDerivativeRec := httptest.NewRecorder()
	s.parentDerivative(invalidParentDerivativeRec, invalidParentDerivativeReq, scope{Tenant: tenant.String()})
	if invalidParentDerivativeRec.Code != http.StatusNotFound {
		t.Fatalf("invalid parent derivative status=%d", invalidParentDerivativeRec.Code)
	}
	mustExec(`UPDATE task_occurrences SET status='awaiting_verification' WHERE tenant_id=$1 AND id=$2`, tenant, occurrence)
	mediaConfig := `{"acceptedKinds":["image","audio","video"],"maxBytes":50000,"maxCount":10,"criteria":[{"id":"shows-work","label":"Shows work","description":"The image shows the poem.","required":true}],"passRule":"all_required","reviewPolicy":"parent_review"}`
	mustExec(`UPDATE verification_requirements SET config=$3 WHERE tenant_id=$1 AND id=$2`, tenant, requirement, mediaConfig)
	audio, err := os.ReadFile("../artifact/testdata/valid.mp3")
	if err != nil {
		t.Fatal(err)
	}
	audioSum := sha256.Sum256(audio)
	audioBody, _ := json.Marshal(artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "audio", Filename: "poem.mp3", ContentType: "audio/mpeg", Size: int64(len(audio)), IdempotencyKey: "audio-upload"})
	audioReserveRec := httptest.NewRecorder()
	s.reserveArtifact(audioReserveRec, httptest.NewRequest(http.MethodPost, "/student/artifacts/reserve", bytes.NewReader(audioBody)), student)
	if audioReserveRec.Code != http.StatusCreated {
		t.Fatalf("audio reserve status=%d body=%s", audioReserveRec.Code, audioReserveRec.Body.String())
	}
	var audioReservation artifactReservationOutput
	if err := json.Unmarshal(audioReserveRec.Body.Bytes(), &audioReservation); err != nil {
		t.Fatal(err)
	}
	audioID := uuid.MustParse(audioReservation.ArtifactID)
	if _, err := store.Put(ctx, uploadKey(tenant, audioID), "audio/mpeg", bytes.NewReader(audio), int64(len(audio))); err != nil {
		t.Fatal(err)
	}
	audioFinalizeBody, _ := json.Marshal(artifactFinalizeInput{ArtifactID: audioReservation.ArtifactID, OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: hex.EncodeToString(audioSum[:]), DurationMS: 1000, IdempotencyKey: audioReservation.IdempotencyKey})
	audioFinalizeRec := httptest.NewRecorder()
	s.finalizeArtifact(audioFinalizeRec, httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(audioFinalizeBody)), student)
	if audioFinalizeRec.Code != http.StatusOK || !strings.Contains(audioFinalizeRec.Body.String(), "preview") {
		t.Fatalf("audio finalize status=%d body=%s", audioFinalizeRec.Code, audioFinalizeRec.Body.String())
	}
	video, err := os.ReadFile("../artifact/testdata/truncated.mp4")
	if err != nil {
		t.Fatal(err)
	}
	videoSum := sha256.Sum256(video)
	videoBody, _ := json.Marshal(artifactReservationInput{OccurrenceID: occurrence.String(), RequirementID: requirement.String(), Kind: "video", Filename: "poem.mp4", ContentType: "video/mp4", Size: int64(len(video)), IdempotencyKey: "video-upload"})
	videoReserveRec := httptest.NewRecorder()
	s.reserveArtifact(videoReserveRec, httptest.NewRequest(http.MethodPost, "/student/artifacts/reserve", bytes.NewReader(videoBody)), student)
	if videoReserveRec.Code != http.StatusCreated {
		t.Fatalf("video reserve status=%d body=%s", videoReserveRec.Code, videoReserveRec.Body.String())
	}
	var videoReservation artifactReservationOutput
	_ = json.Unmarshal(videoReserveRec.Body.Bytes(), &videoReservation)
	videoID := uuid.MustParse(videoReservation.ArtifactID)
	if _, err := store.Put(ctx, uploadKey(tenant, videoID), "video/mp4", bytes.NewReader(video), int64(len(video))); err != nil {
		t.Fatal(err)
	}
	videoFinalizeBody, _ := json.Marshal(artifactFinalizeInput{ArtifactID: videoReservation.ArtifactID, OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: hex.EncodeToString(videoSum[:]), DurationMS: 1000, IdempotencyKey: videoReservation.IdempotencyKey})
	videoFinalizeRec := httptest.NewRecorder()
	s.finalizeArtifact(videoFinalizeRec, httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(videoFinalizeBody)), student)
	if videoFinalizeRec.Code != http.StatusBadRequest {
		t.Fatalf("unverified video duration status=%d body=%s", videoFinalizeRec.Code, videoFinalizeRec.Body.String())
	}
	// Audio is persisted and routed to review rather than entering the image
	// evaluator. The detached worker path still owns that policy transition.
	time.Sleep(1100 * time.Millisecond)
	if err := s.runArtifactStep(ctx); err != nil {
		t.Fatal("audio review worker step:", err)
	}
	noDurationReservation := reserve("completed-occurrence", int64(valid.Len()))
	mustExec(`UPDATE task_occurrences SET status='completed' WHERE tenant_id=$1 AND id=$2`, tenant, occurrence)
	completedOccurrenceBody, _ := json.Marshal(artifactFinalizeInput{ArtifactID: noDurationReservation.ArtifactID, OccurrenceID: occurrence.String(), RequirementID: requirement.String(), SHA256: hex.EncodeToString(audioSum[:]), DurationMS: 1000, IdempotencyKey: noDurationReservation.IdempotencyKey})
	completedOccurrenceRec := httptest.NewRecorder()
	s.finalizeArtifact(completedOccurrenceRec, httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(completedOccurrenceBody)), student)
	if completedOccurrenceRec.Code != http.StatusConflict {
		t.Fatalf("completed occurrence finalize status=%d body=%s", completedOccurrenceRec.Code, completedOccurrenceRec.Body.String())
	}
	mustExec(`UPDATE task_occurrences SET status='awaiting_verification' WHERE tenant_id=$1 AND id=$2`, tenant, occurrence)
	noComposerReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(missingPartsBody))
	s.Artifacts = rejectingPutStore{Store: store}
	noComposerRec := httptest.NewRecorder()
	s.finalizeArtifact(noComposerRec, noComposerReq, student)
	if noComposerRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing composer status=%d body=%s", noComposerRec.Code, noComposerRec.Body.String())
	}
	s.Artifacts = rejectingComposeStore{Store: store}
	composeErrorReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(missingPartsBody))
	composeErrorRec := httptest.NewRecorder()
	s.finalizeArtifact(composeErrorRec, composeErrorReq, student)
	if composeErrorRec.Code != http.StatusBadRequest {
		t.Fatalf("compose error status=%d body=%s", composeErrorRec.Code, composeErrorRec.Body.String())
	}
	s.Artifacts = store
	multipartFinalizeReq := httptest.NewRequest(http.MethodPost, "/student/occurrences/"+occurrence.String()+"/artifacts/finalize", bytes.NewReader(missingPartsBody))
	multipartFinalizeRec := httptest.NewRecorder()
	s.finalizeArtifact(multipartFinalizeRec, multipartFinalizeReq, student)
	if multipartFinalizeRec.Code != http.StatusOK {
		t.Fatalf("multipart finalize status=%d body=%s", multipartFinalizeRec.Code, multipartFinalizeRec.Body.String())
	}
	var multipartOutput artifactOutput
	if err := json.Unmarshal(multipartFinalizeRec.Body.Bytes(), &multipartOutput); err != nil {
		t.Fatal(err)
	}
	var retryJobID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM artifact_rubric_jobs WHERE tenant_id=$1 AND submission_id=$2`, tenant, multipartOutput.SubmissionID).Scan(&retryJobID); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASKS_ARTIFACT_SCRIPTED_FAULT", "always")
	mustExec(`UPDATE artifact_rubric_jobs SET available_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, retryJobID)
	time.Sleep(1100 * time.Millisecond)
	if err := s.runArtifactStep(ctx); err != nil {
		t.Fatal("provider failure worker step:", err)
	}
	var retryStatus, retryError string
	if err := pool.QueryRow(ctx, `SELECT status,last_error FROM artifact_rubric_jobs WHERE tenant_id=$1 AND id=$2`, tenant, retryJobID).Scan(&retryStatus, &retryError); err != nil {
		t.Fatal(err)
	}
	if retryStatus != "queued" || retryError != "provider evaluation failed" {
		t.Fatalf("provider retry status=%s error=%q", retryStatus, retryError)
	}
	revoked := uuid.New()
	revokedUploadRec := httptest.NewRecorder()
	s.uploadArtifact(revokedUploadRec, httptest.NewRequest(http.MethodPut, "/student/artifacts/x/upload", bytes.NewReader(valid.Bytes())), revoked)
	if revokedUploadRec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked upload status=%d", revokedUploadRec.Code)
	}
	revokedPartRec := httptest.NewRecorder()
	s.uploadArtifactPart(revokedPartRec, httptest.NewRequest(http.MethodPut, "/student/artifacts/x/parts/1", bytes.NewReader(valid.Bytes())), revoked)
	if revokedPartRec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked part status=%d", revokedPartRec.Code)
	}
	revokedDerivativeRec := httptest.NewRecorder()
	s.studentDerivative(revokedDerivativeRec, httptest.NewRequest(http.MethodGet, "/student/artifacts/x/derivative/thumbnail", nil), revoked)
	if revokedDerivativeRec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked derivative status=%d", revokedDerivativeRec.Code)
	}
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}
