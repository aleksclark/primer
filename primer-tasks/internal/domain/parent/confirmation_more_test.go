package parent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConfirmationValidationAndSQLFailClosedBranches(t *testing.T) {
	if _, err := (&SQLConfirmationStore{}).Issue(context.Background(), ConfirmationPreview{}); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("nil SQL issue=%v", err)
	}
	if _, err := (&SQLConfirmationStore{}).Consume(context.Background(), "handle", "tenant", "actor", "digest"); !errors.Is(err, ErrServiceMissing) {
		t.Fatalf("nil SQL consume=%v", err)
	}
	for _, value := range []string{"", "short", "not-hex-digest-with-length-that-is-not-valid"} {
		if got := decodeDigest(value); len(got) != 31 {
			t.Fatalf("invalid digest %q length=%d", value, len(got))
		}
	}
	if got := decodeDigest("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); len(got) != 32 {
		t.Fatalf("valid digest length=%d", len(got))
	}
	now := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	valid := ConfirmationPreview{TenantID: "tenant", ActorID: "actor", Action: RetireTaskAction("task", 1), ExpiresAt: now.Add(time.Minute)}
	if _, err := validatePreview(valid, now); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []ConfirmationPreview{
		{TenantID: "", ActorID: "actor", Action: valid.Action, ExpiresAt: valid.ExpiresAt},
		{TenantID: "tenant", ActorID: "", Action: valid.Action, ExpiresAt: valid.ExpiresAt},
		{TenantID: "tenant", ActorID: "actor", Action: valid.Action, ExpiresAt: now},
		{TenantID: "tenant", ActorID: "actor", Action: valid.Action, ExpiresAt: now.Add(-time.Minute)},
		{TenantID: "tenant", ActorID: "actor", Action: Action{Kind: "unknown", TargetIDs: []string{"task"}}, ExpiresAt: valid.ExpiresAt},
	} {
		if _, err := validatePreview(bad, now); err == nil {
			t.Fatalf("invalid preview accepted: %+v", bad)
		}
	}
	memory := NewMemoryConfirmationStore()
	memory.Now = func() time.Time { return now }
	if _, err := memory.Issue(context.Background(), ConfirmationPreview{TenantID: "tenant", ActorID: "actor", Action: valid.Action, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
}
