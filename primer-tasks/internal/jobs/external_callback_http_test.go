package jobs

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"primer-tasks/internal/verification"
)

type failingCallbackBody struct{}

func (failingCallbackBody) Read([]byte) (int, error) { return 0, errors.New("body read failed") }
func (failingCallbackBody) Close() error             { return nil }

func TestCallbackHTTPHandlerRejectsBodyReadErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.Body = failingCallbackBody{}
	(&CallbackProcessor{}).HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("body read error status=%d", rec.Code)
	}
}

func TestCallbackHTTPHandlerBindsRequestHeaderBeforeProcessing(t *testing.T) {
	callback := verification.CallbackEnvelope{
		Version: 1, CallbackID: "callback", RequestID: "request", AttemptRef: "attempt",
		VerifierID: "verifier", SchemaVersion: verification.ExternalCallbackSchemaVersion,
		Sequence: 1, RequestDigest: "digest", Type: "progress",
		Progress: &verification.ProgressResult{Code: "checking", Percent: 10},
	}
	body, err := jsonMarshal(callback)
	if err != nil {
		t.Fatal(err)
	}
	handler := (&CallbackProcessor{}).HTTPHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(string(body)))
	req.Header.Set("X-Primer-Request-ID", "different")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("mismatched request id status=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(string(body)))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unconfigured processor status=%d", rec.Code)
	}
}

// Keep this test package independent of an HTTP fixture while still exercising
// the handler's typed-body boundary.
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
