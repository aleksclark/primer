package verification

import (
	"testing"
	"time"
)

func TestExternalConfigRejectsEachRequiredFieldIndividually(t *testing.T) {
	for _, config := range []ExternalConfig{
		{Capability: "response", Schema: ExternalCallbackSchemaVersion},
		{VerifierID: "verifier", Schema: ExternalCallbackSchemaVersion},
		{VerifierID: "verifier", Capability: "response"},
	} {
		if err := config.Validate(); err == nil {
			t.Fatalf("incomplete config accepted: %+v", config)
		}
	}
}

func TestExternalPublicOptionsRejectSensitiveAndNestedValues(t *testing.T) {
	for _, options := range []map[string]any{
		{"endpointUrl": "https://private.example"},
		{"apiToken": "secret"},
		{"headers": map[string]any{"X": "value"}},
		{"not-valid-key!": "value"},
	} {
		if err := ValidatePublicOptions(options); err == nil {
			t.Fatalf("unsafe options accepted: %#v", options)
		}
	}
	if err := ValidatePublicOptions(map[string]any{"mode": "short", "percent": 50.0, "enabled": true}); err != nil {
		t.Fatal(err)
	}
}

func TestExternalCallbackValidationAcceptsAllTypedResults(t *testing.T) {
	base := CallbackEnvelope{
		Version: 1, CallbackID: "callback", RequestID: "request", AttemptRef: "attempt",
		VerifierID: "verifier", SchemaVersion: ExternalCallbackSchemaVersion,
		Sequence: 1, RequestDigest: "digest",
	}
	cases := []CallbackEnvelope{
		{Type: "rejected", Rejected: &RejectedResult{Rationale: "needs revision"}},
		{Type: "retryable_error", Error: &ErrorResult{Code: "busy", Retryable: true}},
		{Type: "terminal_error", Error: &ErrorResult{Code: "unsupported", Retryable: false}},
	}
	for _, tc := range cases {
		t.Run(tc.Type, func(t *testing.T) {
			candidate := base
			candidate.Type, candidate.Rejected, candidate.Error = tc.Type, tc.Rejected, tc.Error
			if err := candidate.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExternalDecodersRejectOversizeAndMalformedBodies(t *testing.T) {
	for _, decode := range []func([]byte) error{
		func(body []byte) error { _, err := DecodeRequest(body); return err },
		func(body []byte) error { _, err := DecodeCallback(body); return err },
	} {
		for _, body := range [][]byte{nil, []byte("{"), make([]byte, (1<<20)+1)} {
			if err := decode(body); err == nil {
				t.Fatal("malformed or oversized protocol body decoded")
			}
		}
	}
}

func TestExternalEnvelopeRejectsDigestAndExpiryMutations(t *testing.T) {
	now := time.Now().UTC()
	envelope, err := NewRequestEnvelope("request", "attempt", "requirement", ExternalCallbackSchemaVersion, "once", "/callback", map[string]string{"answer": "safe"}, now, time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	badDigest := envelope
	badDigest.PayloadDigest = "0"
	if err := badDigest.Validate(now); err == nil {
		t.Fatal("digest mutation accepted")
	}
	if err := envelope.Validate(now.Add(2 * time.Minute)); err == nil {
		t.Fatal("expired envelope accepted")
	}
}
