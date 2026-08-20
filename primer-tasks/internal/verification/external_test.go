package verification

import (
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
