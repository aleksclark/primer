package repo

import (
	"context"
	"testing"
)

func TestValidateCatalogEndpointRequiresExplicitEgressPolicy(t *testing.T) {
	if err := ValidateCatalogEndpoint(context.Background(), "https://example.com", map[string]any{}); err == nil {
		t.Fatal("endpoint without allowlist accepted")
	}
	if err := ValidateCatalogEndpoint(context.Background(), "https://example.com", map[string]any{"allowlist": []any{"example.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCatalogEndpoint(context.Background(), "http://external-verifier-fixture:8092/v1/verify", map[string]any{"testFixture": true}); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://127.0.0.1:8092", "http://attacker.example/v1", "https://user:pass@verifier.example.test"} {
		if err := ValidateCatalogEndpoint(context.Background(), endpoint, map[string]any{"testFixture": true}); err == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
}

func TestLeaseIntervalDefaults(t *testing.T) {
	if leaseInterval(0) != "1m0s" {
		t.Fatalf("default lease interval=%s", leaseInterval(0))
	}
	if leaseInterval(2) != "2ns" {
		t.Fatalf("lease interval=%s", leaseInterval(2))
	}
}
