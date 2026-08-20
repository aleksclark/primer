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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"primer-tasks/internal/artifactstore"
)

// This fixture deliberately uses the public pairing, reservation, upload, and
// finalize routes. The only database writes here are curriculum/test data and
// deadline manipulation; no artifact handler is called directly.

func minioArtifactPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 11), G: uint8(y * 17), B: 91, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func minioStore(t *testing.T) *artifactstore.S3Store {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		if os.Getenv("PRIMER_TASKS_COVERAGE_GATE") == "1" {
			t.Fatalf("Docker is required for combined Tasks/MinIO coverage: %v", err)
		}
		t.Skipf("Docker is unavailable; skipping combined PostgreSQL/MinIO test: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "minio/minio:RELEASE.2024-06-13T22-53-53Z",
			Env:          map[string]string{"MINIO_ROOT_USER": "tasks-integration", "MINIO_ROOT_PASSWORD": "tasks-integration-password"},
			Cmd:          []string{"server", "/data"},
			ExposedPorts: []string{"9000/tcp"},
			WaitingFor:   wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp").WithStartupTimeout(90 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start MinIO: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "9000/tcp")
	if err != nil {
		t.Fatal(err)
	}
	store, err := artifactstore.NewS3(ctx, artifactstore.S3Config{
		Endpoint:       "http://" + host + ":" + port.Port(),
		Region:         "us-east-1",
		Bucket:         "tasks-integration-" + strings.ToLower(strings.ReplaceAll(uuid.NewString(), "-", "")),
		AccessKey:      "tasks-integration",
		SecretKey:      "tasks-integration-password",
		ForcePathStyle: true,
	})
	if err != nil {
		t.Fatalf("create S3Store: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("create MinIO bucket: %v", err)
	}
	return store
}

func publicStudentCookie(t *testing.T, h http.Handler, parent, student string) string {
	t.Helper()
	pair := requestJSON(t, h, http.MethodPost, "/students/"+student+"/pairing", parent, "")
	if pair.Code != http.StatusOK {
		t.Fatalf("issue student pairing = %d: %s", pair.Code, pair.Body.String())
	}
	var pairing struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(pair.Body.Bytes(), &pairing); err != nil || pairing.Code == "" {
		t.Fatalf("decode pairing = %v, body=%s", err, pair.Body.String())
	}
	claim := requestJSON(t, h, http.MethodPost, "/student/pair", "", `{"code":"`+pairing.Code+`"}`)
	if claim.Code != http.StatusOK {
		t.Fatalf("claim student pairing = %d: %s", claim.Code, claim.Body.String())
	}
	for _, cookie := range claim.Result().Cookies() {
		if cookie.Name == "tasks_student" && cookie.Value != "" {
			return cookie.Value
		}
	}
	t.Fatalf("student pairing did not issue tasks_student cookie: %#v", claim.Result().Cookies())
	return ""
}

func minioStudentRequest(t *testing.T, h http.Handler, method, path, cookie string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "tasks_student", Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func minioUpload(t *testing.T, h http.Handler, uploadURL, cookie string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	if !strings.HasPrefix(uploadURL, "/api/student/artifacts/") || strings.Contains(uploadURL, "minio") || strings.Contains(uploadURL, "X-Amz") || strings.Contains(uploadURL, "tenants/") {
		t.Fatalf("reservation leaked object-store upload URL: %q", uploadURL)
	}
	// /api is the public reverse-proxy prefix. The test handler is the Tasks
	// origin itself, so removing only that prefix still exercises the same
	// same-origin binary façade returned to a browser.
	path := strings.TrimPrefix(uploadURL, "/api")
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/png")
	req.AddCookie(&http.Cookie{Name: "tasks_student", Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func minioReservation(t *testing.T, h http.Handler, occurrence, requirement, cookie, idem string, size int) artifactReservationOutput {
	t.Helper()
	body, _ := json.Marshal(ArtifactReservationInput{RequirementID: requirement, Kind: "image", Filename: "evidence.png", ContentType: "image/png", Size: int64(size), IdempotencyKey: idem})
	rec := minioStudentRequest(t, h, http.MethodPost, "/student/occurrences/"+occurrence+"/artifacts/reserve", cookie, body)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("reserve = %d: %s", rec.Code, rec.Body.String())
	}
	var out artifactReservationOutput
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.ArtifactID == "" || out.UploadURL == "" {
		t.Fatalf("incomplete reservation: %#v", out)
	}
	return out
}

func minioFinalize(t *testing.T, h http.Handler, occurrence, requirement, cookie string, reservation artifactReservationOutput, digest string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(ArtifactFinalizeInput{ArtifactID: reservation.ArtifactID, RequirementID: requirement, Digest: digest, SHA256: digest, IdempotencyKey: reservation.IdempotencyKey})
	return minioStudentRequest(t, h, http.MethodPost, "/student/occurrences/"+occurrence+"/artifacts/finalize", cookie, body)
}

func TestTasksArtifactAPIWithPostgresAndMinIO(t *testing.T) {
	pool := integrationPool(t)
	store := minioStore(t)
	ctx := context.Background()

	// Use unique tenants rather than the shared seed tenants so this test is
	// safe with TASKS_TEST_DATABASE_URL as well as with a disposable container.
	tenantA, tenantB := uuid.New(), uuid.New()
	studentA, studentB := uuid.New(), uuid.New()
	parentA, parentB := "minio-parent-a-"+uuid.NewString(), "minio-parent-b-"+uuid.NewString()
	for _, f := range []struct {
		tenant, student uuid.UUID
		parent          string
		name            string
	}{
		{tenantA, studentA, parentA, "MinIO Alice"},
		{tenantB, studentB, parentB, "MinIO Bob"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,$2)`, f.tenant, f.name); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO parent_memberships(tenant_id,subject_ref,role) VALUES($1,$2,'admin')`, f.tenant, f.parent); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO students(id,tenant_id,display_name) VALUES($1,$2,$3)`, f.student, f.tenant, f.name); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO bff_sessions(handle_hash,tenant_id,subject_ref,session_kind,expires_at) VALUES($1,$2,$3,'parent',now()+interval '1 hour')`, hash(f.parent), f.tenant, f.parent); err != nil {
			t.Fatal(err)
		}
	}
	cleanupTenant := func(tenant uuid.UUID) {
		for _, q := range []string{
			`DELETE FROM artifact_criterion_evaluations WHERE tenant_id=$1`,
			`DELETE FROM artifact_rubric_jobs WHERE tenant_id=$1`,
			`DELETE FROM artifact_submissions WHERE tenant_id=$1`,
			`DELETE FROM artifact_upload_parts WHERE tenant_id=$1`,
			`DELETE FROM artifact_upload_reservations WHERE tenant_id=$1`,
			`DELETE FROM artifact_derivatives WHERE tenant_id=$1`,
			`DELETE FROM artifact_scans WHERE tenant_id=$1`,
			`DELETE FROM artifact_retention WHERE tenant_id=$1`,
			`DELETE FROM artifacts WHERE tenant_id=$1`,
			`DELETE FROM verification_decisions WHERE tenant_id=$1`,
			`DELETE FROM verification_attempts WHERE tenant_id=$1`,
			`DELETE FROM task_occurrences WHERE tenant_id=$1`,
			`DELETE FROM verification_requirements WHERE tenant_id=$1`,
			`DELETE FROM task_schedules WHERE tenant_id=$1`,
			`DELETE FROM task_revisions WHERE tenant_id=$1`,
			`DELETE FROM task_templates WHERE tenant_id=$1`,
			`DELETE FROM pairing_codes WHERE tenant_id=$1`,
			`DELETE FROM student_sessions WHERE tenant_id=$1`,
			`DELETE FROM student_devices WHERE tenant_id=$1`,
			`DELETE FROM audit_records WHERE tenant_id=$1`,
			`DELETE FROM bff_sessions WHERE tenant_id=$1`,
			`DELETE FROM parent_memberships WHERE tenant_id=$1`,
			`DELETE FROM students WHERE tenant_id=$1`,
			`DELETE FROM tenants WHERE id=$1`,
		} {
			_, _ = pool.Exec(context.Background(), q, tenant)
		}
	}
	t.Cleanup(func() { cleanupTenant(tenantA); cleanupTenant(tenantB) })

	// Each occurrence has a stable artifact-rubric requirement, but the second
	// Alice occurrence is a distinct authorization scope.
	seedOccurrence := func(tenant, student uuid.UUID, title string) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
		t.Helper()
		templateID, revisionID, requirementID, scheduleID, occurrenceID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
		config := `{"acceptedKinds":["image"],"maxBytes":100000,"maxCount":3,"criteria":[{"id":"shows-work","label":"Shows work","description":"The image shows the work.","required":true}],"passRule":"all_required","reviewPolicy":"parent_review"}`
		if _, err := pool.Exec(ctx, `INSERT INTO task_templates(id,tenant_id,title,status,current_revision) VALUES($1,$2,$3,'published',1)`, templateID, tenant, title); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO task_revisions(id,tenant_id,template_id,version,title,status) VALUES($1,$2,$3,1,$4,'published')`, revisionID, tenant, templateID, title); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO verification_requirements(id,tenant_id,revision_id,ordinal,kind,config_version,config,interaction,executor) VALUES($1,$2,$3,1,'agent_artifact_rubric',1,$4,'artifact_upload','fantasy')`, requirementID, tenant, revisionID, config); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO task_schedules(id,tenant_id,student_id,template_id,revision_id,kind,timezone,start_local) VALUES($1,$2,$3,$4,$5,'one_off','UTC',now())`, scheduleID, tenant, student, templateID, revisionID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO task_occurrences(id,tenant_id,schedule_id,student_id,revision_id,nominal_at,due_at,status) VALUES($1,$2,$3,$4,$5,now(),now()+interval '1 hour','awaiting_verification')`, occurrenceID, tenant, scheduleID, student, revisionID); err != nil {
			t.Fatal(err)
		}
		return templateID, revisionID, requirementID, scheduleID, occurrenceID
	}
	_, _, requirementA, _, occurrenceA := seedOccurrence(tenantA, studentA, "MinIO evidence")
	_, _, _, _, otherOccurrenceA := seedOccurrence(tenantA, studentA, "MinIO second occurrence")
	_, _, requirementB, _, occurrenceB := seedOccurrence(tenantB, studentB, "MinIO Bob evidence")

	s := NewWithStore(pool, "test", store)
	h := s.Routes()
	cookieA := publicStudentCookie(t, h, parentA, studentA.String())
	cookieB := publicStudentCookie(t, h, parentB, studentB.String())
	if profile := minioStudentRequest(t, h, http.MethodGet, "/student/profile", cookieA, nil); profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), studentA.String()) {
		t.Fatalf("public student authentication = %d: %s", profile.Code, profile.Body.String())
	}

	original := minioArtifactPNG(t)
	digest := sha256.Sum256(original)
	digestHex := hex.EncodeToString(digest[:])
	reservation := minioReservation(t, h, occurrenceA.String(), requirementA.String(), cookieA, "minio-upload-once", len(original))
	again := minioReservation(t, h, occurrenceA.String(), requirementA.String(), cookieA, "minio-upload-once", len(original))
	if again.ArtifactID != reservation.ArtifactID || again.ReservationID != reservation.ReservationID {
		t.Fatalf("reserve replay changed reservation: first=%#v replay=%#v", reservation, again)
	}
	if upload := minioUpload(t, h, reservation.UploadURL, cookieA, original); upload.Code != http.StatusOK {
		t.Fatalf("same-origin upload = %d: %s", upload.Code, upload.Body.String())
	}
	finalized := minioFinalize(t, h, occurrenceA.String(), requirementA.String(), cookieA, reservation, digestHex)
	if finalized.Code != http.StatusOK || !strings.Contains(finalized.Body.String(), `"status":"finalized"`) {
		t.Fatalf("finalize = %d: %s", finalized.Code, finalized.Body.String())
	}
	// A finalize replay is answered from durable state and cannot create a
	// second submission or a second derivative.
	if replay := minioFinalize(t, h, occurrenceA.String(), requirementA.String(), cookieA, reservation, digestHex); replay.Code != http.StatusOK {
		t.Fatalf("finalize replay = %d: %s", replay.Code, replay.Body.String())
	}
	var submissions, derivatives int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM artifact_submissions WHERE tenant_id=$1 AND artifact_id=$2`, tenantA, reservation.ArtifactID).Scan(&submissions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM artifact_derivatives WHERE tenant_id=$1 AND artifact_id=$2`, tenantA, reservation.ArtifactID).Scan(&derivatives); err != nil {
		t.Fatal(err)
	}
	if submissions != 1 || derivatives != 1 {
		t.Fatalf("replay created duplicate rows: submissions=%d derivatives=%d", submissions, derivatives)
	}

	// The original bytes are in MinIO, not a PostgreSQL blob. The row contains
	// only the declared size/digest and lifecycle metadata, while the object
	// store returns the exact uploaded bytes.
	originalKey := uploadKey(tenantA, uuid.MustParse(reservation.ArtifactID))
	stored, err := store.Stat(ctx, originalKey)
	if err != nil || stored.Size != int64(len(original)) {
		t.Fatalf("MinIO original stat = %#v, err=%v", stored, err)
	}
	opened, _, err := store.Open(ctx, originalKey)
	if err != nil {
		t.Fatal(err)
	}
	storedBytes, readErr := io.ReadAll(opened)
	_ = opened.Close()
	if readErr != nil || !bytes.Equal(storedBytes, original) {
		t.Fatalf("MinIO original bytes differ: err=%v", readErr)
	}
	var dbSize int64
	var dbDigest string
	if err := pool.QueryRow(ctx, `SELECT byte_size,sha256 FROM artifacts WHERE tenant_id=$1 AND id=$2`, tenantA, reservation.ArtifactID).Scan(&dbSize, &dbDigest); err != nil {
		t.Fatal(err)
	}
	if dbSize != int64(len(original)) || dbDigest != digestHex {
		t.Fatalf("PostgreSQL metadata = size:%d digest:%s", dbSize, dbDigest)
	}
	var blobColumns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='artifacts' AND column_name IN ('content','bytes','data','body','blob')`).Scan(&blobColumns); err != nil {
		t.Fatal(err)
	}
	if blobColumns != 0 {
		t.Fatalf("artifacts table contains a byte payload column: %d", blobColumns)
	}
	var derivativeKey string
	if err := pool.QueryRow(ctx, `SELECT object_key FROM artifact_derivatives WHERE tenant_id=$1 AND artifact_id=$2`, tenantA, reservation.ArtifactID).Scan(&derivativeKey); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Stat(ctx, derivativeKey); err != nil {
		t.Fatalf("MinIO derivative missing: %v", err)
	}

	// Student, tenant, and occurrence boundaries all fail closed and do not
	// reveal whether the foreign artifact exists.
	if rec := minioStudentRequest(t, h, http.MethodPost, "/student/occurrences/"+occurrenceB.String()+"/artifacts/reserve", cookieA, []byte(`{"requirementId":"`+requirementB.String()+`","kind":"image","filename":"foreign.png","contentType":"image/png","size":10,"idempotencyKey":"foreign-occurrence"}`)); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant reserve = %d, want 404", rec.Code)
	}
	if rec := minioStudentRequest(t, h, http.MethodPost, "/student/occurrences/"+occurrenceA.String()+"/artifacts/finalize", cookieB, []byte(`{"artifactId":"`+reservation.ArtifactID+`","requirementId":"`+requirementA.String()+`","digest":"`+digestHex+`","sha256":"`+digestHex+`","idempotencyKey":"foreign-finalize"}`)); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant finalize = %d, want 404", rec.Code)
	}
	if rec := minioStudentRequest(t, h, http.MethodPost, "/student/occurrences/"+otherOccurrenceA.String()+"/artifacts/finalize", cookieA, []byte(`{"artifactId":"`+reservation.ArtifactID+`","requirementId":"`+requirementA.String()+`","digest":"`+digestHex+`","sha256":"`+digestHex+`","idempotencyKey":"wrong-occurrence"}`)); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-occurrence finalize = %d, want 404", rec.Code)
	}
	if rec := minioStudentRequest(t, h, http.MethodGet, "/student/artifacts/"+reservation.ArtifactID+"/derivative/thumbnail", cookieB, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign student derivative = %d, want 404", rec.Code)
	}
	if rec := requestJSON(t, h, http.MethodGet, "/parent/artifacts/"+reservation.ArtifactID+"/original", parentA, ""); rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), original) {
		t.Fatalf("own parent original = %d, bytes=%d", rec.Code, rec.Body.Len())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/parent/artifacts/"+reservation.ArtifactID+"/original", parentB, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign tenant original = %d, want 404", rec.Code)
	}
	if rec := minioStudentRequest(t, h, http.MethodGet, "/student/artifacts/"+reservation.ArtifactID+"/derivative/thumbnail", cookieA, nil); rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("own student derivative = %d, bytes=%d", rec.Code, rec.Body.Len())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/occurrences/"+occurrenceA.String()+"/artifacts/"+reservation.ArtifactID+"/derivative", parentA, ""); rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("own parent derivative = %d, bytes=%d", rec.Code, rec.Body.Len())
	}

	// An uploaded-but-unfinalized reservation expires in both systems: the API
	// refuses late upload and the cleanup replay removes its MinIO object while
	// tombstoning metadata. DeleteObject is deliberately idempotent in S3Store.
	expired := minioReservation(t, h, occurrenceA.String(), requirementA.String(), cookieA, "minio-expired-upload", len(original))
	if rec := minioUpload(t, h, expired.UploadURL, cookieA, original); rec.Code != http.StatusOK {
		t.Fatalf("expired-fixture upload = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE artifacts SET expires_at=now()-interval '1 minute' WHERE id=$1 AND tenant_id=$2`, expired.ArtifactID, tenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE artifact_upload_reservations SET expires_at=now()-interval '1 minute' WHERE artifact_id=$1 AND tenant_id=$2`, expired.ArtifactID, tenantA); err != nil {
		t.Fatal(err)
	}
	if rec := minioUpload(t, h, expired.UploadURL, cookieA, original); rec.Code != http.StatusNotFound {
		t.Fatalf("late upload after expiry = %d, want 404", rec.Code)
	}
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var expiredStatus, reservationStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM artifacts WHERE tenant_id=$1 AND id=$2`, tenantA, expired.ArtifactID).Scan(&expiredStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM artifact_upload_reservations WHERE tenant_id=$1 AND artifact_id=$2`, tenantA, expired.ArtifactID).Scan(&reservationStatus); err != nil {
		t.Fatal(err)
	}
	if expiredStatus != "tombstoned" || reservationStatus != "expired" {
		t.Fatalf("expired lifecycle = artifact:%s reservation:%s", expiredStatus, reservationStatus)
	}
	if _, err := store.Stat(ctx, uploadKey(tenantA, uuid.MustParse(expired.ArtifactID))); err == nil {
		t.Fatal("expired MinIO object still exists")
	}

	// Retention has two deadlines. The original tombstones first, while the
	// derivative remains readable to authorized principals. After the derivative
	// deadline both the object and row disappear, and a cleanup replay cannot
	// resurrect or leak either representation.
	if _, err := pool.Exec(ctx, `UPDATE artifact_retention SET retain_original_until=now()-interval '1 minute',retain_derivatives_until=now()+interval '1 hour' WHERE tenant_id=$1 AND artifact_id=$2`, tenantA, reservation.ArtifactID); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var artifactStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM artifacts WHERE tenant_id=$1 AND id=$2`, tenantA, reservation.ArtifactID).Scan(&artifactStatus); err != nil {
		t.Fatal(err)
	}
	if artifactStatus != "tombstoned" {
		t.Fatalf("original retention status=%s, want tombstoned", artifactStatus)
	}
	if _, err := store.Stat(ctx, originalKey); err == nil {
		t.Fatal("retention cleanup left original MinIO object")
	}
	if rec := requestJSON(t, h, http.MethodGet, "/parent/artifacts/"+reservation.ArtifactID+"/original", parentA, ""); rec.Code != http.StatusNotFound || bytes.Contains(rec.Body.Bytes(), original) {
		t.Fatalf("tombstoned original leaked: status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec := minioStudentRequest(t, h, http.MethodGet, "/student/artifacts/"+reservation.ArtifactID+"/derivative/thumbnail", cookieA, nil); rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("retained derivative = %d, bytes=%d", rec.Code, rec.Body.Len())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/occurrences/"+occurrenceA.String()+"/artifacts/"+reservation.ArtifactID+"/derivative", parentA, ""); rec.Code != http.StatusOK {
		t.Fatalf("retained parent derivative = %d", rec.Code)
	}
	if _, err := pool.Exec(ctx, `UPDATE artifact_retention SET retain_derivatives_until=now()-interval '1 minute' WHERE tenant_id=$1 AND artifact_id=$2`, tenantA, reservation.ArtifactID); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM artifact_derivatives WHERE tenant_id=$1 AND artifact_id=$2`, tenantA, reservation.ArtifactID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("expired derivative rows=%d, want 0", remaining)
	}
	if _, err := store.Stat(ctx, derivativeKey); err == nil {
		t.Fatal("retention cleanup left derivative MinIO object")
	}
	if err := s.CleanupArtifactOrphans(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("cleanup replay: %v", err)
	}
	for _, rec := range []*httptest.ResponseRecorder{
		minioStudentRequest(t, h, http.MethodGet, "/student/artifacts/"+reservation.ArtifactID+"/derivative/thumbnail", cookieA, nil),
		requestJSON(t, h, http.MethodGet, "/parent/artifacts/"+reservation.ArtifactID+"/original", parentA, ""),
		requestJSON(t, h, http.MethodGet, "/occurrences/"+occurrenceA.String()+"/artifacts/"+reservation.ArtifactID+"/derivative", parentA, ""),
	} {
		if rec.Code != http.StatusNotFound {
			t.Fatalf("post-retention endpoint leaked: status=%d body=%q", rec.Code, rec.Body.String())
		}
	}

	// Revocation is checked by the public route on every request; no in-memory
	// artifact authorization survives a durable session revocation.
	if _, err := pool.Exec(ctx, `UPDATE student_sessions SET revoked_at=now() WHERE handle_hash=$1`, hash(cookieA)); err != nil {
		t.Fatal(err)
	}
	if rec := minioStudentRequest(t, h, http.MethodGet, "/student/profile", cookieA, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked student auth = %d, want 401", rec.Code)
	}
	if rec := minioStudentRequest(t, h, http.MethodGet, "/student/artifacts/"+reservation.ArtifactID+"/derivative/thumbnail", cookieA, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked derivative auth = %d, want 401", rec.Code)
	}
}
