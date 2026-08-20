package repo

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestVerifierSecretRotationRejectsIncompleteReferencesBeforeDatabaseAccess(t *testing.T) {
	r := NewVerifierCatalogRepository(nil)
	for _, ref := range []string{"", "fixture"} {
		if err := r.RotateSecret(context.Background(), uuid.New(), ref, ""); err == nil {
			t.Fatalf("incomplete rotation accepted ref=%q", ref)
		}
	}
	if err := r.RotateSecret(context.Background(), uuid.New(), "", "1"); err == nil {
		t.Fatal("empty secret reference accepted")
	}
}
