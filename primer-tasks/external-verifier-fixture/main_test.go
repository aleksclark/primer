package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func signedRequest(t *testing.T, secret []byte, path string, envelope requestEnvelope) *http.Request {
	t.Helper()
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	r := httptest.NewRequest(http.MethodPost, "http://fixture.test"+path, bytes.NewReader(body))
	r.Header.Set(signatureHeader, sign(secret, http.MethodPost, path, envelope.RequestID, body, at))
	r.Header.Set(timestampHeader, at.Format(time.RFC3339Nano))
	r.Header.Set(requestIDHeader, envelope.RequestID)
	r.Header.Set("Content-Type", "application/json")
	return r
}
func envelope(callbackPath string) requestEnvelope {
	payload := json.RawMessage(`{"answer":"opaque"}`)
	return requestEnvelope{Version: 1, RequestID: "00000000-0000-0000-0000-000000000001", AttemptRef: "00000000-0000-0000-0000-000000000002", RequirementRef: "00000000-0000-0000-0000-000000000003", SchemaVersion: "external_callback.v1", ExpiresAt: time.Now().Add(time.Hour).UTC(), IdempotencyKey: "idem-1", CallbackPath: callbackPath, PayloadDigest: digest(payload), Payload: payload}
}

func TestFixtureSignsProgressAndMakesRequestIdempotentAcrossRestart(t *testing.T) {
	var mu sync.Mutex
	var callbacks [][]byte
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		callbacks = append(callbacks, b)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer callback.Close()
	t.Setenv("FIXTURE_CALLBACK_BASE_URL", callback.URL)
	t.Setenv("FIXTURE_EGRESS_MODE", "test")
	path := filepath.Join(t.TempDir(), "ledger.json")
	l, err := openLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(l, "test-secret")
	e := envelope("/callback")
	rec := httptest.NewRecorder()
	f.verifyRequest(rec, signedRequest(t, f.secret, "/v1/verify", e))
	if rec.Code != 202 {
		t.Fatalf("first status = %d, body=%s", rec.Code, rec.Body)
	}
	mu.Lock()
	got := len(callbacks)
	mu.Unlock()
	if got != 3 {
		t.Fatalf("callbacks = %d, want 3", got)
	}
	restartedLedger, err := openLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	restarted := newFixture(restartedLedger, "test-secret")
	rec = httptest.NewRecorder()
	restarted.verifyRequest(rec, signedRequest(t, restarted.secret, "/v1/verify", e))
	if rec.Code != 202 || !bytes.Contains(rec.Body.Bytes(), []byte(`"idempotent":true`)) {
		t.Fatalf("replay = %d %s", rec.Code, rec.Body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(callbacks) != got {
		t.Fatalf("replay delivered %d new callbacks", len(callbacks)-got)
	}
}
func TestFixtureRejectsForgedAndConflictingRequests(t *testing.T) {
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer callback.Close()
	t.Setenv("FIXTURE_CALLBACK_BASE_URL", callback.URL)
	t.Setenv("FIXTURE_EGRESS_MODE", "test")
	l, err := openLedger(filepath.Join(t.TempDir(), "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(l, "test-secret")
	e := envelope("/")
	r := signedRequest(t, []byte("wrong"), "/v1/verify", e)
	rec := httptest.NewRecorder()
	f.verifyRequest(rec, r)
	if rec.Code != 401 {
		t.Fatalf("forged status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	f.verifyRequest(rec, signedRequest(t, f.secret, "/v1/verify", e))
	if rec.Code != 202 {
		t.Fatalf("valid status = %d", rec.Code)
	}
	conflict := e
	conflict.RequestID = "00000000-0000-0000-0000-000000000099"
	rec = httptest.NewRecorder()
	f.verifyRequest(rec, signedRequest(t, f.secret, "/v1/verify", conflict))
	if rec.Code != 409 {
		t.Fatalf("idempotency conflict = %d", rec.Code)
	}
}
func TestFixtureFaultsDuplicateAndOutOfOrderCallbacks(t *testing.T) {
	var mu sync.Mutex
	var sequences []int64
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c callbackEnvelope
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			t.Errorf("callback JSON: %v", err)
		}
		mu.Lock()
		sequences = append(sequences, c.Sequence)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer callback.Close()
	t.Setenv("FIXTURE_CALLBACK_BASE_URL", callback.URL)
	t.Setenv("FIXTURE_EGRESS_MODE", "test")
	l, err := openLedger(filepath.Join(t.TempDir(), "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(l, "test-secret")
	f.setFaults([]string{"duplicate_callbacks", "out_of_order"})
	e := envelope("/callback")
	e.RequestID = "00000000-0000-0000-0000-000000000011"
	e.IdempotencyKey = "faulted"
	rec := httptest.NewRecorder()
	f.verifyRequest(rec, signedRequest(t, f.secret, "/v1/verify", e))
	if rec.Code != 202 {
		t.Fatalf("fault status = %d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []int64{3, 3, 1, 1, 2, 2}
	if len(sequences) != len(want) {
		t.Fatalf("sequences = %v, want %v", sequences, want)
	}
	for i := range want {
		if sequences[i] != want[i] {
			t.Fatalf("sequences = %v, want %v", sequences, want)
		}
	}
}
func TestFixtureCorruptCallbackIsActuallyCorrupt(t *testing.T) {
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if json.Valid(body) {
			t.Errorf("corrupt callback remained valid JSON: %s", body)
		}
		w.WriteHeader(400)
	}))
	defer callback.Close()
	t.Setenv("FIXTURE_CALLBACK_BASE_URL", callback.URL)
	t.Setenv("FIXTURE_EGRESS_MODE", "test")
	l, err := openLedger(filepath.Join(t.TempDir(), "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(l, "test-secret")
	f.setFaults([]string{"corrupt_callback"})
	e := envelope("/")
	e.RequestID = "00000000-0000-0000-0000-000000000012"
	e.IdempotencyKey = "corrupt"
	rec := httptest.NewRecorder()
	f.verifyRequest(rec, signedRequest(t, f.secret, "/v1/verify", e))
	if rec.Code != 202 {
		t.Fatalf("corrupt delivery should not fail receipt: %d", rec.Code)
	}
	stored, ok := f.ledger.findRequest(e.IdempotencyKey)
	if !ok || stored.Final {
		t.Fatalf("corrupt callback marked request final: %+v", stored)
	}
}
