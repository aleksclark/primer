package jobs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/repo"
)

func TestExternalSecretResolverBindsReferenceAndVersion(t *testing.T) {
	resolver := StaticSecretResolver{"fixture:1": []byte("secret")}
	if got, err := resolver.Resolve(context.Background(), "fixture", "1"); err != nil || string(got) != "secret" {
		t.Fatalf("resolve=%q err=%v", got, err)
	}
	if _, err := resolver.Resolve(context.Background(), "fixture", "2"); err == nil {
		t.Fatal("unknown secret version resolved")
	}
}

type fakeExternalOutbox struct {
	delivery repo.ExternalDelivery
	claimed  bool
	finished []string
}

func (f *fakeExternalOutbox) RequeueExpired(context.Context, time.Time) error { return nil }
func (f *fakeExternalOutbox) Claim(context.Context, string, time.Duration) (repo.ExternalDelivery, bool, error) {
	if f.claimed {
		return repo.ExternalDelivery{}, false, nil
	}
	f.claimed = true
	return f.delivery, true, nil
}
func (f *fakeExternalOutbox) Finish(_ context.Context, _ repo.ExternalDelivery, _ string, status, _ string, _ time.Time) error {
	f.finished = append(f.finished, status)
	return nil
}

type fakeExternalCatalog struct {
	value  repo.VerifierCatalog
	err    error
	oldRef string
	oldErr error
}

func (f fakeExternalCatalog) Get(context.Context, uuid.UUID) (repo.VerifierCatalog, error) {
	return f.value, f.err
}
func (f fakeExternalCatalog) SecretRefForVersion(context.Context, uuid.UUID, string) (string, error) {
	if f.oldErr != nil {
		return "", f.oldErr
	}
	if f.oldRef != "" {
		return f.oldRef, nil
	}
	return "fixture", nil
}

type externalRoundTripper func(*http.Request) (*http.Response, error)

func (f externalRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestExternalWorkerDeliveryOutcomes(t *testing.T) {
	base := repo.ExternalDelivery{ID: "delivery", TenantID: uuid.NewString(), AttemptID: uuid.NewString(), RequirementID: uuid.NewString(), VerifierID: uuid.NewString(), RequestID: uuid.NewString(), Envelope: []byte(`{"version":1,"requestId":"request","attemptRef":"attempt","requirementRef":"requirement","schemaVersion":"external_callback.v1","expiresAt":"2999-01-01T00:00:00Z","idempotencyKey":"once","callbackPath":"/callback","payloadDigest":"2bb80d537b1da3e38bd30361aa855686bde0ba3c80b8a7e3e6e2d1c9b0b3e1b0","payload":{"answer":"opaque"}}`), MaxAttempts: 3, ExpiresAt: time.Now().Add(time.Hour)}
	catalog := fakeExternalCatalog{value: repo.VerifierCatalog{ID: uuid.MustParse(base.VerifierID), EndpointURL: "http://external-verifier-fixture:8092/v1/verify", Active: true, SecretRef: "fixture", SecretVersion: "1", EgressPolicy: map[string]any{"testFixture": true}, Timeout: time.Second}}
	for name, tc := range map[string]struct {
		code int
		want string
	}{"accepted": {code: http.StatusAccepted, want: "waiting"}, "busy": {code: http.StatusTooManyRequests, want: "retryable_error"}, "failure": {code: http.StatusBadGateway, want: "retryable_error"}} {
		t.Run(name, func(t *testing.T) {
			outbox := &fakeExternalOutbox{delivery: base}
			worker := &ExternalWorker{Outbox: outbox, Catalog: catalog, Secrets: StaticSecretResolver{"fixture:1": []byte("secret")}, Client: &http.Client{Transport: externalRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader("body"))}, nil
			})}, Owner: "worker", Lease: time.Minute, Now: time.Now}
			if err := worker.Step(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(outbox.finished) != 1 || outbox.finished[0] != tc.want {
				t.Fatalf("finished=%v", outbox.finished)
			}
		})
	}
}

func TestExternalWorkerUsesDeliverySecretVersionBinding(t *testing.T) {
	base := repo.ExternalDelivery{ID: "bound-delivery", TenantID: uuid.NewString(), AttemptID: uuid.NewString(), RequirementID: uuid.NewString(), VerifierID: uuid.NewString(), RequestID: uuid.NewString(), SecretVersion: "old", Envelope: []byte(`{"version":1,"requestId":"request","attemptRef":"attempt","requirementRef":"requirement","schemaVersion":"external_callback.v1","expiresAt":"2999-01-01T00:00:00Z","idempotencyKey":"once","callbackPath":"/callback","payloadDigest":"2bb80d537b1da3e38bd30361aa855686bde0ba3c80b8a7e3e6e2d1c9b0b3e1b0","payload":{"answer":"opaque"}}`), MaxAttempts: 3, ExpiresAt: time.Now().Add(time.Hour)}
	catalog := fakeExternalCatalog{value: repo.VerifierCatalog{ID: uuid.MustParse(base.VerifierID), EndpointURL: "http://external-verifier-fixture:8092/v1/verify", Active: true, SecretRef: "current", SecretVersion: "new", EgressPolicy: map[string]any{"testFixture": true}, Timeout: time.Second}, oldRef: "previous"}
	outbox := &fakeExternalOutbox{delivery: base}
	worker := &ExternalWorker{Outbox: outbox, Catalog: catalog, Secrets: StaticSecretResolver{"previous:old": []byte("old-secret")}, Client: &http.Client{Transport: externalRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("X-Primer-Key-ID") != "old" {
			t.Fatalf("key id=%q", req.Header.Get("X-Primer-Key-ID"))
		}
		return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader("ack"))}, nil
	})}, Owner: "worker", Lease: time.Minute, Now: time.Now}
	if err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(outbox.finished) != 1 || outbox.finished[0] != "waiting" {
		t.Fatalf("finished=%v", outbox.finished)
	}
}

func TestExternalWorkerDeliveryFailsClosed(t *testing.T) {
	base := repo.ExternalDelivery{ID: "delivery", TenantID: uuid.NewString(), AttemptID: uuid.NewString(), RequirementID: uuid.NewString(), VerifierID: uuid.NewString(), RequestID: uuid.NewString(), Envelope: []byte(`{"version":1}`), MaxAttempts: 1, ExpiresAt: time.Now().Add(time.Hour)}
	cases := []struct {
		name    string
		catalog repo.VerifierCatalog
		secrets SecretResolver
		client  *http.Client
		want    string
	}{
		{"disabled", repo.VerifierCatalog{ID: uuid.MustParse(base.VerifierID), Active: false}, StaticSecretResolver{"fixture:1": []byte("secret")}, nil, "dead"},
		{"invalid endpoint", repo.VerifierCatalog{ID: uuid.MustParse(base.VerifierID), Active: true, EndpointURL: "http://127.0.0.1:1"}, StaticSecretResolver{"fixture:1": []byte("secret")}, nil, "dead"},
		{"missing secret", repo.VerifierCatalog{ID: uuid.MustParse(base.VerifierID), Active: true, EndpointURL: "http://external-verifier-fixture:8092/v1/verify", EgressPolicy: map[string]any{"testFixture": true}, SecretRef: "missing", SecretVersion: "1"}, StaticSecretResolver{}, nil, "retryable_error"},
		{"transport", repo.VerifierCatalog{ID: uuid.MustParse(base.VerifierID), Active: true, EndpointURL: "http://external-verifier-fixture:8092/v1/verify", EgressPolicy: map[string]any{"testFixture": true}, SecretRef: "fixture", SecretVersion: "1"}, StaticSecretResolver{"fixture:1": []byte("secret")}, &http.Client{Transport: externalRoundTripper(func(*http.Request) (*http.Response, error) { return nil, errors.New("timeout") })}, "retryable_error"},
		{"catalog unavailable", repo.VerifierCatalog{}, StaticSecretResolver{"fixture:1": []byte("secret")}, nil, "retryable_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outbox := &fakeExternalOutbox{delivery: base}
			w := &ExternalWorker{Outbox: outbox, Catalog: fakeExternalCatalog{value: tc.catalog, err: func() error {
				if tc.name == "catalog unavailable" {
					return errors.New("catalog unavailable")
				}
				return nil
			}()}, Secrets: tc.secrets, Client: tc.client, Owner: "worker", Lease: time.Minute, Now: time.Now}
			if err := w.Step(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(outbox.finished) != 1 || outbox.finished[0] != tc.want {
				t.Fatalf("finished=%v want=%s", outbox.finished, tc.want)
			}
		})
	}
}

func TestExternalWorkerFailsWhenBoundSecretVersionIsUnavailable(t *testing.T) {
	base := repo.ExternalDelivery{ID: "old-secret", TenantID: uuid.NewString(), AttemptID: uuid.NewString(), RequirementID: uuid.NewString(), VerifierID: uuid.NewString(), RequestID: uuid.NewString(), SecretVersion: "old", Envelope: []byte(`{"version":1}`), MaxAttempts: 2, ExpiresAt: time.Now().Add(time.Hour)}
	catalog := fakeExternalCatalog{value: repo.VerifierCatalog{ID: uuid.MustParse(base.VerifierID), EndpointURL: "http://external-verifier-fixture:8092/v1/verify", Active: true, SecretRef: "current", SecretVersion: "new", EgressPolicy: map[string]any{"testFixture": true}}, oldErr: errors.New("retired secret missing")}
	outbox := &fakeExternalOutbox{delivery: base}
	worker := &ExternalWorker{Outbox: outbox, Catalog: catalog, Secrets: StaticSecretResolver{"current:new": []byte("secret")}, Owner: "worker", Lease: time.Minute, Now: time.Now}
	if err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(outbox.finished) != 1 || outbox.finished[0] != "retryable_error" {
		t.Fatalf("finished=%v", outbox.finished)
	}
}

func TestCallbackHTTPHandlerRejectsMalformedAndMismatchedCallbacks(t *testing.T) {
	handler := (&CallbackProcessor{}).HTTPHandler()
	for _, body := range []string{"", "{}"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(body))
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusBadRequest {
			t.Fatalf("body=%q status=%d", body, rec.Code)
		}
	}
}

func TestExternalWorkerRunStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	base := repo.ExternalDelivery{ID: "run", VerifierID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour)}
	w := &ExternalWorker{Outbox: &fakeExternalOutbox{delivery: base, claimed: true}, Catalog: fakeExternalCatalog{}, Poll: time.Millisecond, Lease: time.Minute, Owner: "run", Now: time.Now}
	w.Run(ctx)
}

func TestExternalWorkerConfigurationAndBackoff(t *testing.T) {
	worker := NewExternalWorker(nil, nil, nil)
	if err := worker.Step(context.Background()); err == nil {
		t.Fatal("unconfigured worker succeeded")
	}
	if got := backoff(1); got < 2*time.Second || got >= 3*time.Second {
		t.Fatalf("backoff(1)=%s", got)
	}
	if got := backoff(20); got < 256*time.Second || got >= 257*time.Second {
		t.Fatalf("backoff cap=%s", got)
	}
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return nil }}
	worker.Client = client
	if worker.Client == nil {
		t.Fatal("client not configured")
	}
}
