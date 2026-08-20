package api

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestStudentArtifactSubscriptionRequiresScopedIdentifiers(t *testing.T) {
	sub := &studentSubscriber{queue: make(chan wireStudentEvent, 1), done: make(chan struct{})}
	(&Server{}).studentArtifactSubscribe(context.Background(), studentIdentity{StudentID: uuid.New(), TenantID: "tenant-a"}, sub, studentCommand{})
	select {
	case event := <-sub.queue:
		if event.Code != "invalid_request" || event.Message == "" || event.Retryable {
			t.Fatalf("invalid artifact subscription event=%+v", event)
		}
	default:
		t.Fatal("incomplete artifact subscription did not emit an error")
	}
}

func TestStudentDialogueRequestBoundaries(t *testing.T) {
	if (&Server{}).StudentDialogueHandler() == nil {
		t.Fatal("student dialogue handler missing")
	}
	if !studentQueryCredential(httptest.NewRequest("GET", "/student/ws?token=bad", nil)) {
		t.Fatal("query credential was not detected")
	}
	if studentQueryCredential(httptest.NewRequest("GET", "/student/ws", nil)) {
		t.Fatal("empty query credential was detected")
	}
	for _, tc := range []struct {
		expected, next int64
		conflict       bool
	}{{0, 1, false}, {2, 3, true}, {3, 3, false}} {
		if got := expectedSequenceConflict(tc.expected, tc.next); got != tc.conflict {
			t.Fatalf("expected=%d next=%d got=%v", tc.expected, tc.next, got)
		}
	}
	if stringPtr(nil) != "" || stringPtr(ptr("ok")) != "ok" {
		t.Fatal("string pointer projection failed")
	}
	if _, err := uuid.Parse(uuid.NewString()); err != nil {
		t.Fatal(err)
	}
}

func ptr(value string) *string { return &value }
