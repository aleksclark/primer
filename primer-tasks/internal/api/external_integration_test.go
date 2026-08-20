package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"primer-tasks/internal/jobs"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
)

func externalRouteRequest(method, path, body, id string) *http.Request {
	route := chi.NewRouteContext()
	route.URLParams.Add("id", id)
	return httptest.NewRequest(method, path, bytes.NewBufferString(body)).WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, route))
}

func TestExternalAPIRealPostgresFlowAndAdminRotation(t *testing.T) {
	pool := integrationPool(t)
	f := seedPhase6External(t, pool)
	t.Cleanup(func() { cleanupPhase6External(t, pool, f) })
	s := NewWithStore(pool, "test", nil)
	s.ExternalSecrets = jobs.StaticSecretResolver{"fixture:1": []byte("fixture-secret")}
	ctx := context.Background()
	config := map[string]any{"verifierId": f.verifier.String(), "capability": "response", "schemaVersion": verification.ExternalCallbackSchemaVersion, "options": map[string]any{"mode": "short"}}
	if err := s.validateExternalConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	if _, err := externalConfig(mustJSON(config)); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshotExternalAttempt(ctx, tx, f.tenant.String(), f.attempt.String(), f.requirement.String(), verification.ExternalConfig{VerifierID: f.verifier.String(), Capability: "response", Schema: verification.ExternalCallbackSchemaVersion, Options: map[string]any{"mode": "short"}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	submitRec := httptest.NewRecorder()
	s.submitExternal(submitRec, externalRouteRequest(http.MethodPost, "/student/occurrences/"+f.occurrence.String()+"/external/submit", `{"idempotencyKey":"api-flow-once","publicPayload":{"response":"student answer"}}`, f.occurrence.String()), f.student)
	if submitRec.Code != http.StatusAccepted {
		t.Fatalf("submit status=%d body=%s", submitRec.Code, submitRec.Body)
	}
	var submit ExternalSubmitResult
	if err := json.Unmarshal(submitRec.Body.Bytes(), &submit); err != nil {
		t.Fatal(err)
	}
	if submit.RequestID == "" || submit.Status != "queued" {
		t.Fatalf("submit=%+v", submit)
	}
	duplicateRec := httptest.NewRecorder()
	s.submitExternal(duplicateRec, externalRouteRequest(http.MethodPost, "/student/occurrences/"+f.occurrence.String()+"/external/submit", `{"idempotencyKey":"api-flow-once","publicPayload":{"response":"different retry body"}}`, f.occurrence.String()), f.student)
	var duplicate ExternalSubmitResult
	if duplicateRec.Code != http.StatusAccepted || json.Unmarshal(duplicateRec.Body.Bytes(), &duplicate) != nil || duplicate.RequestID != submit.RequestID {
		t.Fatalf("duplicate status=%d body=%s duplicate=%+v original=%+v", duplicateRec.Code, duplicateRec.Body, duplicate, submit)
	}
	var requestedEvents int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_verifier_events WHERE tenant_id=$1 AND request_id=$2 AND sequence=0`, f.tenant, submit.RequestID).Scan(&requestedEvents); err != nil || requestedEvents != 1 {
		t.Fatalf("requested events=%d err=%v", requestedEvents, err)
	}
	state, err := s.loadExternalState(ctx, f.occurrence.String(), f.student.String(), false)
	if err != nil {
		t.Fatal(err)
	}
	if state.Source != "fixture" || state.SchemaVersion != verification.ExternalCallbackSchemaVersion || state.Capability != "response" {
		t.Fatalf("state=%+v", state)
	}
	if parent, err := s.loadExternalState(ctx, f.occurrence.String(), f.tenant.String(), true); err != nil || parent.AttemptID != state.AttemptID {
		t.Fatalf("parent state=%+v err=%v", parent, err)
	}
	studentState := httptest.NewRecorder()
	s.externalState(studentState, externalRouteRequest(http.MethodGet, "/student/occurrences/"+f.occurrence.String()+"/external", "", f.occurrence.String()), f.student)
	if studentState.Code != http.StatusOK {
		t.Fatalf("student state=%d", studentState.Code)
	}
	parentInspect := httptest.NewRecorder()
	s.inspectExternal(parentInspect, externalRouteRequest(http.MethodGet, "/occurrences/"+f.occurrence.String()+"/external/inspect", "", f.occurrence.String()), scope{Tenant: f.tenant.String(), Subject: "parent", Role: "admin"})
	if parentInspect.Code != http.StatusOK {
		t.Fatalf("parent inspect=%d", parentInspect.Code)
	}
	catalogList := httptest.NewRecorder()
	s.listExternalVerifiers(catalogList, httptest.NewRequest(http.MethodGet, "/admin/verifiers", nil), scope{Tenant: f.tenant.String(), Subject: "parent", Role: "admin"})
	if catalogList.Code != http.StatusOK {
		t.Fatalf("catalog list=%d", catalogList.Code)
	}

	externalRepo := repo.NewExternalRepository(pool)
	claimed, ok, err := externalRepo.Claim(ctx, "coverage-worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if err := externalRepo.Renew(ctx, claimed.ID, claimed.LeaseOwner, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE external_verifier_outbox SET lease_until=now()-interval '1 second' WHERE id=$1`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	if err := externalRepo.RequeueExpired(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	reclaimed, ok, err := externalRepo.Claim(ctx, "coverage-worker-2", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaim ok=%v err=%v", ok, err)
	}
	if err := externalRepo.Finish(ctx, reclaimed, reclaimed.LeaseOwner, "waiting", "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := externalRepo.Finish(ctx, reclaimed, reclaimed.LeaseOwner, "retryable_error", "retry", time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := externalRepo.Finish(ctx, reclaimed, reclaimed.LeaseOwner, "unknown", "", time.Time{}); err == nil {
		t.Fatal("invalid delivery status accepted")
	}
	externalRepo.SecurityFailure(ctx, f.tenant.String(), f.verifier.String(), submit.RequestID, "test_failure")

	var digest, callbackPath string
	if err := pool.QueryRow(ctx, `SELECT payload_digest,callback_path FROM external_verifier_outbox WHERE request_id=$1`, submit.RequestID).Scan(&digest, &callbackPath); err != nil {
		t.Fatal(err)
	}
	callback := verification.CallbackEnvelope{Version: 1, CallbackID: uuid.NewString(), RequestID: submit.RequestID, AttemptRef: state.AttemptID, VerifierID: f.verifier.String(), SchemaVersion: verification.ExternalCallbackSchemaVersion, Sequence: 1, RequestDigest: digest, Type: "progress", Progress: &verification.ProgressResult{Code: "checking", Message: "Checking response", Percent: 50}}
	body, _ := json.Marshal(callback)
	stamp := time.Now().UTC()
	request := httptest.NewRequest(http.MethodPost, callbackPath, bytes.NewReader(body))
	request.Header.Set("X-Primer-Key-ID", "1")
	request.Header.Set("X-Primer-Timestamp", stamp.Format(time.RFC3339Nano))
	request.Header.Set("X-Primer-Request-ID", callback.CallbackID)
	request.Header.Set("X-Primer-Signature", verification.Sign(http.MethodPost, callbackPath, stamp, callback.CallbackID, body, []byte("fixture-secret")))
	route := chi.NewRouteContext()
	route.URLParams.Add("id", f.verifier.String())
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
	badCallback := httptest.NewRequest(http.MethodPost, callbackPath, bytes.NewReader(body))
	badCallback.Header = request.Header.Clone()
	badCallback.Header.Set("X-Primer-Signature", "sha256=00")
	badCallback = badCallback.WithContext(context.WithValue(badCallback.Context(), chi.RouteCtxKey, route))
	badRec := httptest.NewRecorder()
	s.externalCallback(badRec, badCallback)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad callback=%d", badRec.Code)
	}
	callbackRec := httptest.NewRecorder()
	s.externalCallback(callbackRec, request)
	if callbackRec.Code != http.StatusAccepted {
		t.Fatalf("callback=%d body=%s", callbackRec.Code, callbackRec.Body)
	}

	if _, err := pool.Exec(ctx, `UPDATE external_verifier_outbox SET status='dead',expires_at=now()-interval '1 second' WHERE request_id=$1`, submit.RequestID); err != nil {
		t.Fatal(err)
	}
	deadCallbackRec := httptest.NewRecorder()
	s.externalCallback(deadCallbackRec, request)
	if deadCallbackRec.Code != http.StatusUnauthorized {
		t.Fatalf("late dead callback=%d", deadCallbackRec.Code)
	}
	retryRec := httptest.NewRecorder()
	s.retryExternal(retryRec, externalRouteRequest(http.MethodPost, "/occurrences/"+f.occurrence.String()+"/external/retry", "", f.occurrence.String()), scope{Tenant: f.tenant.String(), Role: "admin"})
	if retryRec.Code != http.StatusOK {
		var retryStatus string
		_ = pool.QueryRow(ctx, `SELECT status FROM external_verifier_outbox WHERE request_id=$1`, submit.RequestID).Scan(&retryStatus)
		t.Fatalf("retry=%d body=%s dbstatus=%s", retryRec.Code, retryRec.Body, retryStatus)
	}
	var attempts int
	var expiresAt, envelopeExpiresAt time.Time
	if err := pool.QueryRow(ctx, `SELECT attempts,expires_at,(envelope->>'expiresAt')::timestamptz FROM external_verifier_outbox WHERE request_id=$1`, submit.RequestID).Scan(&attempts, &expiresAt, &envelopeExpiresAt); err != nil || attempts != 0 || !expiresAt.After(time.Now()) || !envelopeExpiresAt.After(time.Now()) {
		t.Fatalf("retry attempts=%d expires=%s envelopeExpires=%s err=%v", attempts, expiresAt, envelopeExpiresAt, err)
	}
	cancelRec := httptest.NewRecorder()
	s.cancelExternal(cancelRec, externalRouteRequest(http.MethodPost, "/occurrences/"+f.occurrence.String()+"/external/cancel", "", f.occurrence.String()), scope{Tenant: f.tenant.String(), Role: "admin"})
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("cancel=%d", cancelRec.Code)
	}
	lateCallbackRec := httptest.NewRecorder()
	s.externalCallback(lateCallbackRec, request)
	if lateCallbackRec.Code != http.StatusUnauthorized {
		t.Fatalf("late canceled callback=%d", lateCallbackRec.Code)
	}
	fallbackRec := httptest.NewRecorder()
	s.fallbackExternal(fallbackRec, externalRouteRequest(http.MethodPost, "/occurrences/"+f.occurrence.String()+"/external/fallback", `{"accepted":true,"reason":"Parent reviewed the safe external result."}`, f.occurrence.String()), scope{Tenant: f.tenant.String(), Role: "admin"})
	if fallbackRec.Code != http.StatusOK {
		t.Fatalf("fallback=%d body=%s", fallbackRec.Code, fallbackRec.Body)
	}
	var occurrenceStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM task_occurrences WHERE id=$1`, f.occurrence).Scan(&occurrenceStatus); err != nil || occurrenceStatus != "awaiting_verification" {
		t.Fatalf("occurrence=%s err=%v", occurrenceStatus, err)
	}
	if strings.Contains(fallbackRec.Body.String(), `"inserted":true`) {
		t.Fatalf("fallback decided canceled delivery: %s", fallbackRec.Body)
	}

	createRec := httptest.NewRecorder()
	s.createExternalVerifier(createRec, httptest.NewRequest(http.MethodPost, "/admin/verifiers", bytes.NewBufferString(`{"name":"rotatable","endpointUrl":"http://external-verifier-fixture:8092/v1/verify","active":true,"schemaVersions":["external_callback.v1"],"capabilities":["response"],"secretRef":"fixture","secretVersion":"1","egressPolicy":{"testFixture":true}}`)), scope{Tenant: f.tenant.String(), Role: "admin"})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("catalog create=%d body=%s", createRec.Code, createRec.Body)
	}
	var created externalVerifierOutput
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rotate := httptest.NewRequest(http.MethodPatch, "/admin/verifiers/"+created.ID.String(), bytes.NewBufferString(`{"active":true,"secretRef":"fixture","secretVersion":"2"}`))
	rotateRoute := chi.NewRouteContext()
	rotateRoute.URLParams.Add("id", created.ID.String())
	rotate = rotate.WithContext(context.WithValue(rotate.Context(), chi.RouteCtxKey, rotateRoute))
	rotateRec := httptest.NewRecorder()
	s.setExternalVerifierActive(rotateRec, rotate, scope{Tenant: f.tenant.String(), Role: "admin"})
	if rotateRec.Code != http.StatusOK {
		t.Fatalf("rotate=%d", rotateRec.Code)
	}
	var version string
	if err := pool.QueryRow(ctx, `SELECT secret_version FROM external_verifier_catalog WHERE id=$1`, created.ID).Scan(&version); err != nil || version != "2" {
		t.Fatalf("version=%s err=%v", version, err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM external_verifier_secret_versions WHERE verifier_id=$1`, created.ID)
	_, _ = pool.Exec(ctx, `DELETE FROM external_verifier_catalog WHERE id=$1`, created.ID)
}

func TestExternalAPIBoundariesFailClosed(t *testing.T) {
	pool := integrationPool(t)
	s := NewWithStore(pool, "test", nil)
	missing := uuid.NewString()
	denied := httptest.NewRecorder()
	s.listExternalVerifiers(denied, httptest.NewRequest(http.MethodGet, "/admin/verifiers", nil), scope{Tenant: uuid.NewString()})
	if denied.Code != http.StatusForbidden {
		t.Fatalf("catalog denied=%d", denied.Code)
	}
	if _, err := externalConfig([]byte("{")); err == nil {
		t.Fatal("malformed external config accepted")
	}
	if containsString([]string{"a"}, "b") {
		t.Fatal("missing capability reported present")
	}
	badSubmit := httptest.NewRecorder()
	s.submitExternal(badSubmit, httptest.NewRequest(http.MethodPost, "/student/occurrences/"+missing+"/external/submit", bytes.NewBufferString(`{"publicPayload":{}}`)), uuid.New())
	if badSubmit.Code != http.StatusBadRequest {
		t.Fatalf("bad submit=%d", badSubmit.Code)
	}
	missingAttempt := httptest.NewRecorder()
	s.submitExternal(missingAttempt, httptest.NewRequest(http.MethodPost, "/student/occurrences/"+missing+"/external/submit", bytes.NewBufferString(`{"idempotencyKey":"missing","publicPayload":{}}`)), uuid.New())
	if missingAttempt.Code != http.StatusNotFound {
		t.Fatalf("missing attempt=%d", missingAttempt.Code)
	}
	longKey := httptest.NewRecorder()
	s.submitExternal(longKey, httptest.NewRequest(http.MethodPost, "/student/occurrences/"+missing+"/external/submit", bytes.NewBufferString(`{"idempotencyKey":"`+strings.Repeat("k", 201)+`","publicPayload":{}}`)), uuid.New())
	if longKey.Code != http.StatusBadRequest {
		t.Fatalf("long idempotency key=%d", longKey.Code)
	}
	largePayload := httptest.NewRecorder()
	s.submitExternal(largePayload, httptest.NewRequest(http.MethodPost, "/student/occurrences/"+missing+"/external/submit", bytes.NewBufferString(`{"idempotencyKey":"large","publicPayload":{"response":"`+strings.Repeat("x", 1<<20)+`"}}`)), uuid.New())
	if largePayload.Code != http.StatusBadRequest {
		t.Fatalf("large payload=%d", largePayload.Code)
	}
	badFallback := httptest.NewRecorder()
	s.fallbackExternal(badFallback, externalRouteRequest(http.MethodPost, "/occurrences/"+missing+"/external/fallback", "{", missing), scope{Tenant: uuid.NewString(), Role: "admin"})
	if badFallback.Code != http.StatusBadRequest {
		t.Fatalf("bad fallback=%d", badFallback.Code)
	}
	badCreate := httptest.NewRecorder()
	s.createExternalVerifier(badCreate, httptest.NewRequest(http.MethodPost, "/admin/verifiers", bytes.NewBufferString(`{"name":"bad","endpointUrl":"http://127.0.0.1:9","active":true,"secretRef":"fixture","secretVersion":"1"}`)), scope{Tenant: uuid.NewString(), Role: "admin"})
	if badCreate.Code != http.StatusBadRequest {
		t.Fatalf("catalog invalid=%d", badCreate.Code)
	}
	badID := httptest.NewRecorder()
	s.setExternalVerifierActive(badID, externalRouteRequest(http.MethodPatch, "/admin/verifiers/not-a-uuid", `{"active":true}`, "not-a-uuid"), scope{Tenant: uuid.NewString(), Role: "admin"})
	if badID.Code != http.StatusNotFound {
		t.Fatalf("bad id=%d", badID.Code)
	}
	student := httptest.NewRecorder()
	s.externalState(student, externalRouteRequest(http.MethodGet, "/student/occurrences/"+missing+"/external", "", missing), uuid.New())
	if student.Code != http.StatusNotFound {
		t.Fatalf("missing student=%d", student.Code)
	}
	archive := httptest.NewRecorder()
	s.archiveStudent(archive, externalRouteRequest(http.MethodDelete, "/students/"+missing, "", missing), scope{Tenant: uuid.NewString(), Role: "admin"})
	if archive.Code != http.StatusNotFound {
		t.Fatalf("missing archive=%d", archive.Code)
	}
	parent := httptest.NewRecorder()
	s.inspectExternal(parent, externalRouteRequest(http.MethodGet, "/occurrences/"+missing+"/external/inspect", "", missing), scope{Tenant: uuid.NewString(), Role: "admin"})
	if parent.Code != http.StatusNotFound {
		t.Fatalf("missing parent=%d", parent.Code)
	}
	retry := httptest.NewRecorder()
	s.retryExternal(retry, externalRouteRequest(http.MethodPost, "/occurrences/"+missing+"/external/retry", "", missing), scope{Tenant: uuid.NewString(), Role: "admin"})
	if retry.Code != http.StatusConflict {
		t.Fatalf("missing retry=%d", retry.Code)
	}
	cancel := httptest.NewRecorder()
	s.cancelExternal(cancel, externalRouteRequest(http.MethodPost, "/occurrences/"+missing+"/external/cancel", "", missing), scope{Tenant: uuid.NewString(), Role: "admin"})
	if cancel.Code != http.StatusNotFound {
		t.Fatalf("missing cancel=%d", cancel.Code)
	}
	fallback := httptest.NewRecorder()
	s.fallbackExternal(fallback, externalRouteRequest(http.MethodPost, "/occurrences/"+missing+"/external/fallback", `{"accepted":true,"reason":"no"}`, missing), scope{Tenant: uuid.NewString(), Role: "admin"})
	if fallback.Code != http.StatusConflict {
		t.Fatalf("missing fallback=%d", fallback.Code)
	}
	noSecret := NewWithStore(pool, "test", nil)
	noSecret.ExternalSecrets = nil
	noSecret.StartExternalWorker(context.Background())
	cb := httptest.NewRecorder()
	noSecret.externalCallback(cb, httptest.NewRequest(http.MethodPost, "/external/verifiers/"+uuid.NewString()+"/callback", bytes.NewBufferString(`{}`)))
	if cb.Code != http.StatusServiceUnavailable {
		t.Fatalf("no secret=%d", cb.Code)
	}
}
