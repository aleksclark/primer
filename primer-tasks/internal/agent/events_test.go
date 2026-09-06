package agent

import (
	"context"
	"testing"

	"primer-tasks/internal/agent/protocol"
)

func TestHubReplaysThenClosesSlowSubscriberWithoutAffectingRun(t *testing.T) {
	h := NewHub(1)
	replay := func(context.Context, int64, int) ([]protocol.Event, error) {
		return []protocol.Event{protocol.TextStart("r", 1)}, nil
	}
	s, err := h.Subscribe(context.Background(), "r", 0, replay)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h.Publish(protocol.TextDelta("r", 2, "one"))
	h.Publish(protocol.TextDelta("r", 3, "two"))
	if h.SubscriberCount("r") != 0 {
		t.Fatal("slow subscriber was allowed to grow")
	}
	if _, ok := <-s.C; !ok {
		t.Fatal("replay event was not retained")
	}
	if _, ok := <-s.C; ok {
		t.Fatal("slow subscriber channel remained open after buffered replay drained")
	}
}

func TestHubCloseIsIdempotentAndCursorIsDurableBoundary(t *testing.T) {
	h := NewHub(2)
	s, err := h.Subscribe(context.Background(), "r", 2, func(_ context.Context, after int64, _ int) ([]protocol.Event, error) {
		if after != 2 {
			t.Fatalf("after=%d", after)
		}
		return []protocol.Event{protocol.TextEnd("r", 3)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.Close()
	h.Publish(protocol.Terminal("r", 4, "succeeded"))
	if h.SubscriberCount("r") != 0 {
		t.Fatal("closed subscription leaked")
	}
}
