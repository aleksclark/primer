package verification

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestExternalSignatureBindsMethodPathTimeDigestAndRequest(t *testing.T) {
	secret := []byte("rotation-secret-v2")
	body := []byte(`{"version":1,"requestId":"request"}`)
	at := time.Unix(1720000000, 123).UTC()
	signature := Sign("POST", "/external/callback", at, "request", body, secret)
	if err := Verify("POST", "/external/callback", at, "request", signature, body, secret, at.Add(time.Second), time.Minute); err != nil {
		t.Fatal(err)
	}
	if wrapped := SignRequest("POST", "/external/callback", at, "request", body, secret); wrapped != signature {
		t.Fatal("request signing wrapper diverged")
	}
	if err := VerifyRequest("POST", "/external/callback", at, "request", signature, body, secret, at, time.Minute); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func() error{
		"method": func() error {
			return Verify("PUT", "/external/callback", at, "request", signature, body, secret, at, time.Minute)
		},
		"path": func() error { return Verify("POST", "/other", at, "request", signature, body, secret, at, time.Minute) },
		"body": func() error {
			return Verify("POST", "/external/callback", at, "request", signature, []byte("red"), secret, at, time.Minute)
		},
		"request": func() error {
			return Verify("POST", "/external/callback", at, "other", signature, body, secret, at, time.Minute)
		},
		"time": func() error {
			return Verify("POST", "/external/callback", at, "request", signature, body, secret, at.Add(2*time.Minute), time.Minute)
		},
	} {
		if err := mutate(); err == nil {
			t.Errorf("%s mutation verified", name)
		}
	}
}

func TestExternalCallbackValidationIsTypedAndDecisionOnly(t *testing.T) {
	accepted := CallbackEnvelope{Version: 1, CallbackID: "callback", RequestID: "request", AttemptRef: "attempt", VerifierID: "verifier", SchemaVersion: ExternalCallbackSchemaVersion, Sequence: 1, RequestDigest: "digest", Type: "accepted", Accepted: &AcceptedResult{Rationale: "safe"}}
	if err := accepted.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []CallbackEnvelope{
		{Version: 1, CallbackID: "callback", RequestID: "request", AttemptRef: "attempt", VerifierID: "verifier", SchemaVersion: ExternalCallbackSchemaVersion, Sequence: 0, RequestDigest: "digest", Type: "accepted", Accepted: &AcceptedResult{}},
		{Version: 1, CallbackID: "callback", RequestID: "request", AttemptRef: "attempt", VerifierID: "verifier", SchemaVersion: ExternalCallbackSchemaVersion, Sequence: 1, RequestDigest: "digest", Type: "progress"},
		{Version: 1, CallbackID: "callback", RequestID: "request", AttemptRef: "attempt", VerifierID: "verifier", SchemaVersion: ExternalCallbackSchemaVersion, Sequence: 1, RequestDigest: "digest", Type: "retryable_error", Error: &ErrorResult{Retryable: false}},
	} {
		if err := invalid.Validate(); err == nil {
			t.Error("invalid callback accepted")
		}
	}
}

func TestExternalConfigEnvelopeAndDecoders(t *testing.T) {
	if err := (ExternalConfig{}).Validate(); err == nil {
		t.Fatal("empty external config accepted")
	}
	config := ExternalConfig{VerifierID: "verifier", Capability: "response", Schema: ExternalCallbackSchemaVersion}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	envelope, err := NewRequestEnvelope("request", "attempt", "requirement", ExternalCallbackSchemaVersion, "idempotency", "/callback", map[string]any{"answer": "opaque"}, time.Now().UTC(), time.Minute, []string{"attempt"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := MarshalRequestEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeRequest(body); err != nil || decoded.RequestID != envelope.RequestID {
		t.Fatalf("request decode=%+v err=%v", decoded, err)
	}
	if _, err := DecodeRequest([]byte(`{"version":1,"expiresAt":"2000-01-01T00:00:00Z"}`)); err == nil {
		t.Fatal("expired malformed request accepted")
	}
	callback := CallbackEnvelope{Version: 1, CallbackID: "callback", RequestID: "request", AttemptRef: "attempt", VerifierID: "verifier", SchemaVersion: ExternalCallbackSchemaVersion, Sequence: 1, RequestDigest: "digest", Type: "progress", Progress: &ProgressResult{Code: "checking", Percent: 10}}
	callbackBody, _ := json.Marshal(callback)
	if decoded, err := DecodeCallback(callbackBody); err != nil || decoded.Type != "progress" {
		t.Fatalf("callback decode=%+v err=%v", decoded, err)
	}
}

type externalDecisionRecorder struct {
	calls    int
	decision Decision
}

func (r *externalDecisionRecorder) CommitDecision(_ context.Context, d Decision) (bool, error) {
	r.calls++
	r.decision = d
	return true, nil
}

func TestExternalCallbackDecisionOnlyBridge(t *testing.T) {
	recorder := &externalDecisionRecorder{}
	binding := ExternalBinding{TenantID: "tenant", AttemptID: "attempt", OccurrenceID: "occurrence", RequirementID: "requirement", VerifierID: "verifier", RequestID: "request", PayloadDigest: "digest"}
	progress := CallbackEnvelope{Version: 1, CallbackID: "progress", RequestID: "request", AttemptRef: "attempt", VerifierID: "verifier", SchemaVersion: ExternalCallbackSchemaVersion, Sequence: 1, RequestDigest: "digest", Type: "progress", Progress: &ProgressResult{Code: "checking", Percent: 40}}
	if decided, err := HandleExternalCallback(context.Background(), recorder, binding, progress); err != nil || decided || recorder.calls != 0 {
		t.Fatalf("progress decided=%v err=%v calls=%d", decided, err, recorder.calls)
	}
	accepted := progress
	accepted.CallbackID = "accepted"
	accepted.Sequence = 2
	accepted.Type = "accepted"
	accepted.Progress = nil
	accepted.Accepted = &AcceptedResult{Rationale: "safe rationale"}
	if decided, err := HandleExternalCallback(context.Background(), recorder, binding, accepted); err != nil || !decided || recorder.calls != 1 || !recorder.decision.Accepted {
		t.Fatalf("accepted decided=%v err=%v decision=%+v", decided, err, recorder.decision)
	}
	mismatch := accepted
	mismatch.CallbackID = "mismatch"
	mismatch.RequestDigest = "wrong"
	if _, err := HandleExternalCallback(context.Background(), recorder, binding, mismatch); err == nil {
		t.Fatal("mismatched callback accepted")
	}
}

func TestExternalEndpointRejectsPrivateAndUnsafeTargets(t *testing.T) {
	for _, endpoint := range []string{"http://example.com", "https://127.0.0.1/verifier", "https://10.0.0.1", "https://[::1]/", "https://user:pass@example.com", "https://example.com?token=secret"} {
		if err := ValidateEndpoint(endpoint); err == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	if err := ValidateEndpoint("https://verifier.example.test/callback"); err != nil {
		t.Fatal(err)
	}
}
