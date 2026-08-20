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

	"primer-tasks/internal/jobs"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
)

func TestPhase6ExternalSignedCallbackProcessAndDecision(t *testing.T) {
	pool := integrationPool(t)
	f := seedPhase6External(t, pool)
	seedPhase6ExternalDelivery(t, pool, f)
	t.Cleanup(func() { cleanupPhase6External(t, pool, f) })
	server := NewWithStore(pool, "test", nil)
	server.ExternalSecrets = jobs.StaticSecretResolver{"fixture:1": []byte("secret"), "fixture-new:2": []byte("new-secret")}
	badCallbackRec := httptest.NewRecorder()
	server.externalCallback(badCallbackRec, httptest.NewRequest(http.MethodPost, "/external/verifiers/not-a-uuid/callback", bytes.NewBufferString(`{}`)))
	if badCallbackRec.Code != http.StatusUnauthorized {
		t.Fatalf("malformed callback status=%d", badCallbackRec.Code)
	}
	binding, err := repo.NewExternalRepository(pool).Binding(context.Background(), f.requestID, f.verifier.String(), f.attempt.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.NewVerifierCatalogRepository(pool).RotateSecret(context.Background(), f.verifier, "fixture-new", "2"); err != nil {
		t.Fatal(err)
	}
	path := binding.CallbackPath
	send := func(callback verification.CallbackEnvelope) int {
		t.Helper()
		body, err := json.Marshal(callback)
		if err != nil {
			t.Fatal(err)
		}
		stamp := time.Now().UTC()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("X-Primer-Request-ID", callback.CallbackID)
		req.Header.Set("X-Primer-Key-ID", "1")
		req.Header.Set("X-Primer-Timestamp", stamp.Format(time.RFC3339Nano))
		req.Header.Set("X-Primer-Signature", verification.Sign(http.MethodPost, path, stamp, callback.CallbackID, body, []byte("secret")))
		rec := httptest.NewRecorder()
		server.externalCallback(rec, req)
		return rec.Code
	}
	progress := verification.CallbackEnvelope{Version: 1, CallbackID: "00000000-0000-0000-0000-000000000701", RequestID: f.requestID, AttemptRef: f.attempt.String(), VerifierID: f.verifier.String(), SchemaVersion: verification.ExternalCallbackSchemaVersion, Sequence: 1, RequestDigest: binding.PayloadDigest, Type: "progress", Progress: &verification.ProgressResult{Code: "checking", Percent: 50}}
	if status := send(progress); status != http.StatusAccepted {
		t.Fatalf("progress status=%d", status)
	}
	progressBody, _ := json.Marshal(progress)
	mismatchReq := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(progressBody))
	mismatchReq.Header.Set("X-Primer-Request-ID", "wrong-header")
	mismatchRec := httptest.NewRecorder()
	server.externalCallback(mismatchRec, mismatchReq)
	if mismatchRec.Code != http.StatusUnauthorized {
		t.Fatalf("callback header mismatch status=%d", mismatchRec.Code)
	}
	badSignatureReq := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(progressBody))
	badSignatureReq.Header.Set("X-Primer-Request-ID", progress.CallbackID)
	badSignatureReq.Header.Set("X-Primer-Key-ID", "1")
	badSignatureReq.Header.Set("X-Primer-Timestamp", time.Now().UTC().Format(time.RFC3339Nano))
	badSignatureReq.Header.Set("X-Primer-Signature", "sha256=bad")
	badSignatureRec := httptest.NewRecorder()
	server.externalCallback(badSignatureRec, badSignatureReq)
	if badSignatureRec.Code != http.StatusUnauthorized {
		t.Fatalf("callback signature mismatch status=%d", badSignatureRec.Code)
	}
	processor := &jobs.CallbackProcessor{Outbox: repo.NewExternalRepository(pool), Catalog: repo.NewVerifierCatalogRepository(pool), Secrets: jobs.StaticSecretResolver{"fixture:1": []byte("secret")}, Committer: server, Now: time.Now}
	accepted := progress
	accepted.CallbackID = "00000000-0000-0000-0000-000000000702"
	accepted.Sequence = 3
	accepted.Type = "accepted"
	accepted.Progress = nil
	accepted.Accepted = &verification.AcceptedResult{Rationale: "safe acceptance"}
	handlerAccepted := accepted
	handlerAccepted.CallbackID = "00000000-0000-0000-0000-000000000707"
	handlerAccepted.Sequence = 2
	handlerAcceptedBody, _ := json.Marshal(handlerAccepted)
	handlerAcceptedStamp := time.Now().UTC()
	handlerAcceptedReq := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(handlerAcceptedBody))
	handlerAcceptedReq.Header.Set("X-Primer-Request-ID", handlerAccepted.CallbackID)
	handlerAcceptedReq.Header.Set("X-Primer-Key-ID", "1")
	handlerAcceptedReq.Header.Set("X-Primer-Timestamp", handlerAcceptedStamp.Format(time.RFC3339Nano))
	handlerAcceptedReq.Header.Set("X-Primer-Signature", verification.Sign(http.MethodPost, path, handlerAcceptedStamp, handlerAccepted.CallbackID, handlerAcceptedBody, []byte("secret")))
	handlerAcceptedRec := httptest.NewRecorder()
	processor.HTTPHandler().ServeHTTP(handlerAcceptedRec, handlerAcceptedReq)
	if handlerAcceptedRec.Code != http.StatusOK {
		t.Fatalf("callback processor HTTP accepted status=%d", handlerAcceptedRec.Code)
	}
	if status := send(accepted); status != http.StatusAccepted {
		t.Fatalf("replayed accepted status=%d", status)
	}
	negative := progress
	negative.CallbackID = "00000000-0000-0000-0000-000000000703"
	negative.Sequence = 4
	negativeBody, err := json.Marshal(negative)
	if err != nil {
		t.Fatal(err)
	}
	negativeStamp := time.Now().UTC()
	negativeSignature := verification.Sign(http.MethodPost, path, negativeStamp, negative.CallbackID, negativeBody, []byte("secret"))
	handlerReq := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(negativeBody))
	handlerReq.Header.Set("X-Primer-Request-ID", negative.CallbackID)
	handlerReq.Header.Set("X-Primer-Key-ID", "1")
	handlerReq.Header.Set("X-Primer-Timestamp", negativeStamp.Format(time.RFC3339Nano))
	handlerReq.Header.Set("X-Primer-Signature", negativeSignature)
	handlerRec := httptest.NewRecorder()
	processor.HTTPHandler().ServeHTTP(handlerRec, handlerReq)
	if handlerRec.Code != http.StatusAccepted {
		t.Fatalf("callback processor HTTP progress status=%d", handlerRec.Code)
	}
	for name, mutate := range map[string]func() error{
		"missing key": func() error {
			_, err := processor.Process(context.Background(), http.MethodPost, path, "", negativeStamp.Format(time.RFC3339Nano), negativeSignature, negativeBody)
			return err
		},
		"bad timestamp": func() error {
			_, err := processor.Process(context.Background(), http.MethodPost, path, "1", "not-a-time", negativeSignature, negativeBody)
			return err
		},
		"bad signature": func() error {
			_, err := processor.Process(context.Background(), http.MethodPost, path, "1", negativeStamp.Format(time.RFC3339Nano), "sha256=bad", negativeBody)
			return err
		},
		"wrong path": func() error {
			_, err := processor.Process(context.Background(), http.MethodPost, "/wrong", "1", negativeStamp.Format(time.RFC3339Nano), negativeSignature, negativeBody)
			return err
		},
	} {
		if err := mutate(); err == nil {
			t.Errorf("%s callback accepted", name)
		}
	}
	invalidVerifier := negative
	invalidVerifier.CallbackID = "00000000-0000-0000-0000-000000000704"
	invalidVerifier.VerifierID = "not-a-uuid"
	invalidBody, _ := json.Marshal(invalidVerifier)
	if _, err := processor.Process(context.Background(), http.MethodPost, path, "1", negativeStamp.Format(time.RFC3339Nano), "", invalidBody); err == nil {
		t.Fatal("invalid verifier callback accepted")
	}
	unknownVerifier := negative
	unknownVerifier.CallbackID = "00000000-0000-0000-0000-000000000705"
	unknownVerifier.VerifierID = "00000000-0000-0000-0000-000000000799"
	unknownBody, _ := json.Marshal(unknownVerifier)
	if _, err := processor.Process(context.Background(), http.MethodPost, path, "1", negativeStamp.Format(time.RFC3339Nano), "", unknownBody); err == nil {
		t.Fatal("unknown verifier callback accepted")
	}
	unknownRequest := negative
	unknownRequest.CallbackID = "00000000-0000-0000-0000-000000000706"
	unknownRequest.RequestID = "00000000-0000-0000-0000-000000000798"
	unknownRequestBody, _ := json.Marshal(unknownRequest)
	if _, err := processor.Process(context.Background(), http.MethodPost, path, "1", negativeStamp.Format(time.RFC3339Nano), "", unknownRequestBody); err == nil {
		t.Fatal("unknown request callback accepted")
	}
	for _, kind := range []string{"progress", "rejected", "retryable_error", "terminal_error"} {
		projection := accepted
		projection.Type = kind
		projection.Accepted = nil
		projection.Rejected = &verification.RejectedResult{Rationale: "safe rejection"}
		projection.Progress = &verification.ProgressResult{Code: "checking", Percent: 50}
		projection.Error = &verification.ErrorResult{Code: "temporary", Retryable: kind == "retryable_error"}
		server.publishExternalCallback(context.Background(), projection)
	}
	var decisions, completed int
	var reason string
	if err := pool.QueryRow(context.Background(), `SELECT reason FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2`, f.tenant, f.attempt).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "external verifier accepted result" {
		t.Fatalf("unsafe external rationale persisted: %q", reason)
	}
	var callbackPayload []byte
	if err := pool.QueryRow(context.Background(), `SELECT payload FROM external_verifier_callbacks WHERE tenant_id=$1 AND request_id=$2 AND result_type='accepted'`, f.tenant, f.requestID).Scan(&callbackPayload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(callbackPayload), "safe acceptance") {
		t.Fatalf("raw callback rationale persisted: %s", callbackPayload)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM verification_decisions WHERE tenant_id=$1 AND attempt_id=$2`, f.tenant, f.attempt).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM external_verifier_facts WHERE tenant_id=$1 AND aggregate_id=$2 AND fact_type='occurrence.completed'`, f.tenant, f.occurrence).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 || completed != 1 {
		t.Fatalf("decisions=%d completed facts=%d", decisions, completed)
	}
}
