package api

import (
	"testing"
	"time"
)

func TestStudentDialogueHubFiltersAttemptsAndClosesSlowSubscribers(t *testing.T) {
	h := newStudentDialogueHub()
	matching := &studentSubscriber{tenant: "tenant-a", attempt: "attempt-a", queue: make(chan wireStudentEvent, 1), done: make(chan struct{})}
	otherTenant := &studentSubscriber{tenant: "tenant-b", attempt: "attempt-a", queue: make(chan wireStudentEvent, 1), done: make(chan struct{})}
	otherAttempt := &studentSubscriber{tenant: "tenant-a", attempt: "attempt-b", queue: make(chan wireStudentEvent, 1), done: make(chan struct{})}
	h.add(matching)
	h.add(otherTenant)
	h.add(otherAttempt)

	h.publish(wireStudentEvent{Type: "progress", TenantID: "tenant-a", AttemptID: "attempt-a", Sequence: 1})
	select {
	case event := <-matching.queue:
		if event.Type != "progress" {
			t.Fatalf("matching event=%+v", event)
		}
	default:
		t.Fatal("matching subscriber did not receive event")
	}
	for _, sub := range []*studentSubscriber{otherTenant, otherAttempt} {
		select {
		case event := <-sub.queue:
			t.Fatalf("filtered subscriber received %+v", event)
		default:
		}
	}

	// A full queue is a backpressure boundary: the hub closes that subscriber
	// instead of blocking the dialogue worker indefinitely.
	matching.queue <- wireStudentEvent{Type: "already-buffered"}
	h.publish(wireStudentEvent{Type: "progress", TenantID: "tenant-a", AttemptID: "attempt-a", Sequence: 2})
	select {
	case <-matching.done:
	case <-time.After(time.Second):
		t.Fatal("slow subscriber was not closed")
	}
	if matching.enqueue(wireStudentEvent{Type: "after-close"}) {
		t.Fatal("closed subscriber accepted an event")
	}

	h.remove(otherTenant)
	h.remove(otherTenant) // close is intentionally idempotent
}

func TestStudentDialogueSubscriberAddsWireDefaultsAndCancels(t *testing.T) {
	sub := &studentSubscriber{queue: make(chan wireStudentEvent, 2), done: make(chan struct{})}
	cancelled := false
	sub.cancel = func() { cancelled = true }
	studentHub.add(sub)
	t.Cleanup(func() { studentHub.remove(sub) })

	(&Server{}).sendStudentToSubscriber(sub, wireStudentEvent{Type: "state"})
	select {
	case event := <-sub.queue:
		if event.ProtocolVersion != studentProtocolVersion || event.Time.IsZero() {
			t.Fatalf("wire defaults missing: %+v", event)
		}
	default:
		t.Fatal("subscriber did not receive event")
	}
	studentHub.remove(sub)
	if !cancelled {
		t.Fatal("subscriber cancellation callback was not called")
	}
	if (&Server{}).studentDialogueHub() == nil {
		t.Fatal("global student hub disappeared")
	}
}
