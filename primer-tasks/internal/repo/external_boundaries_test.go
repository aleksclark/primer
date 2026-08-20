package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"primer-tasks/internal/verification"
)

func TestExternalRepositoryRejectsInvalidDeliveriesBeforeDatabaseAccess(t *testing.T) {
	r := NewExternalRepository(nil)
	if err := r.Enqueue(context.Background(), ExternalDelivery{}); err == nil {
		t.Fatal("nil database accepted")
	}
	if err := (&ExternalRepository{}).Enqueue(context.Background(), ExternalDelivery{}); err == nil {
		t.Fatal("unconfigured repository accepted")
	}
	if interval := leaseInterval(0); interval != time.Minute.String() {
		t.Fatalf("zero lease interval=%q", interval)
	}
	if interval := leaseInterval(-time.Second); interval != time.Minute.String() {
		t.Fatalf("negative lease interval=%q", interval)
	}
}

func TestSafeCallbackPayloadDropsExternalProse(t *testing.T) {
	payload := safeCallbackPayload(verification.CallbackEnvelope{Version: 1, SchemaVersion: "external_callback.v1", Sequence: 2, Type: "accepted", Accepted: &verification.AcceptedResult{Rationale: "private verifier reasoning"}})
	if _, ok := payload["rationale"]; ok || payload["type"] != "accepted" {
		t.Fatalf("unsafe callback projection=%v", payload)
	}
}

func TestFixtureCatalogEndpointPolicyRemainsExplicit(t *testing.T) {
	policy := map[string]any{"testFixture": true}
	if err := ValidateCatalogEndpoint(context.Background(), "http://external-verifier-fixture:8092/v1/verify", policy); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://other:8092/v1/verify", "http://fixture:8092/v1/verify?secret=red", "not a url"} {
		if err := ValidateCatalogEndpoint(context.Background(), endpoint, policy); err == nil {
			t.Fatalf("unsafe fixture endpoint accepted: %s", endpoint)
		}
	}
}

func TestVerifierCatalogRepositoryRequiresDatabase(t *testing.T) {
	r := NewVerifierCatalogRepository(nil)
	if err := r.Create(context.Background(), VerifierCatalog{}); err == nil {
		t.Fatal("nil catalog database accepted")
	}
	if _, err := r.Get(context.Background(), uuid.Nil); err == nil {
		t.Fatal("nil catalog lookup accepted")
	}
}
