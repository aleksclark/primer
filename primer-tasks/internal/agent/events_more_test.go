package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"primer-tasks/internal/agent/protocol"
)

type eventStore struct {
	events []RunEvent
	err    error
}

func (s *eventStore) CreateConversation(context.Context, Conversation) error { return nil }
func (s *eventStore) GetConversation(context.Context, string, string) (Conversation, error) {
	return Conversation{}, nil
}
func (s *eventStore) AppendUserMessage(context.Context, Message) (Message, bool, error) {
	return Message{}, false, nil
}
func (s *eventStore) AppendMessage(context.Context, Message) error        { return nil }
func (s *eventStore) CreateRun(context.Context, Run) error                { return nil }
func (s *eventStore) GetRun(context.Context, string, string) (Run, error) { return Run{}, nil }
func (s *eventStore) TransitionRun(context.Context, string, string, RunStatus, int, Usage) error {
	return nil
}
func (s *eventStore) RequestCancel(context.Context, string, string) error { return nil }
func (s *eventStore) LeaseRun(context.Context, string, string, time.Duration) (Run, bool, error) {
	return Run{}, false, nil
}
func (s *eventStore) ReconcileExpiredLeases(context.Context, time.Time) error { return nil }
func (s *eventStore) AppendEvent(_ context.Context, event RunEvent) error {
	if s.err != nil {
		return s.err
	}
	s.events = append(s.events, event)
	return nil
}
func (s *eventStore) ReplayEvents(context.Context, string, string, int64, int) ([]RunEvent, error) {
	return nil, nil
}
func (s *eventStore) PutPreview(context.Context, ConfirmationPreview) error { return nil }
func (s *eventStore) ConsumePreview(context.Context, ConfirmationPreview, time.Time) error {
	return nil
}

func TestEventLogPersistsBeforePublishingAndRejectsBadEvents(t *testing.T) {
	store := &eventStore{}
	hub := NewHub(2)
	log := &EventLog{Store: store, Hub: hub}
	sub, err := hub.Subscribe(context.Background(), "run-1", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	e := protocol.TextDelta("run-1", 1, "safe")
	if err := log.Append(context.Background(), "tenant-a", e); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 || store.events[0].EventType != string(protocol.EventTextDelta) {
		t.Fatalf("stored=%+v", store.events)
	}
	select {
	case got := <-sub.C:
		if got.Text != "safe" {
			t.Fatalf("published=%+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("event was not published")
	}
	for _, bad := range []protocol.Event{{RunID: "", Sequence: 1, Protocol: protocol.Version}, {RunID: "run-1", Sequence: 0, Protocol: protocol.Version}, {RunID: "run-1", Sequence: 1, Protocol: protocol.Version + 1}} {
		if err := log.Append(context.Background(), "tenant-a", bad); err == nil {
			t.Fatalf("accepted bad event %+v", bad)
		}
	}
	store.err = errors.New("database unavailable")
	if err := log.Append(context.Background(), "tenant-a", protocol.TextEnd("run-1", 2)); !errors.Is(err, store.err) {
		t.Fatalf("store error=%v", err)
	}
}

func TestHubReplayCursorAndSlowSubscriberAreBounded(t *testing.T) {
	if NewHub(0).max != 1 {
		t.Fatal("hub did not clamp zero queue")
	}
	h := NewHub(2)
	replay := func(_ context.Context, after int64, limit int) ([]protocol.Event, error) {
		if after != 1 || limit != 2 {
			t.Fatalf("replay args after=%d limit=%d", after, limit)
		}
		return []protocol.Event{protocol.TextDelta("run", 2, "two"), protocol.TextDelta("run", 3, "three")}, nil
	}
	sub, err := h.Subscribe(context.Background(), "run", 1, replay)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.SubscriberCount("run"); got != 1 {
		t.Fatalf("count=%d", got)
	}
	for want := int64(2); want <= 3; want++ {
		select {
		case event := <-sub.C:
			if event.Sequence != want {
				t.Fatalf("sequence=%d want %d", event.Sequence, want)
			}
		case <-time.After(time.Second):
			t.Fatal("replay timeout")
		}
	}
	// No receiver is attached now. The queue is bounded and the subscriber is
	// evicted instead of applying backpressure to the publisher.
	h.Publish(protocol.TextDelta("run", 4, "four"))
	h.Publish(protocol.TextDelta("run", 5, "five"))
	h.Publish(protocol.TextDelta("run", 6, "six"))
	if got := h.SubscriberCount("run"); got != 0 {
		t.Fatalf("slow subscriber remained: %d", got)
	}
	sub.Close() // idempotent after the hub evicted it
	if _, err := h.Subscribe(context.Background(), "run", 0, func(context.Context, int64, int) ([]protocol.Event, error) { return nil, errors.New("replay failed") }); err == nil {
		t.Fatal("replay failure swallowed")
	}
}
