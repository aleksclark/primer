package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"primer-tasks/internal/agent/protocol"
)

var ErrSlowSubscriber = errors.New("subscriber queue is full; reconnect from cursor")

// EventLog persists before publishing. The hub is only a transport fan-out;
// it never represents run state and can be discarded on process restart.
type EventLog struct {
	Store Repository
	Hub   *Hub
}

func (l *EventLog) Append(ctx context.Context, tenant string, event protocol.Event) error {
	if event.Protocol != protocol.Version || event.Sequence < 1 || event.RunID == "" {
		return errors.New("invalid event")
	}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err = l.Store.AppendEvent(ctx, RunEvent{RunID: event.RunID, TenantID: tenant, Sequence: event.Sequence, EventType: string(event.Kind), Payload: b, CreatedAt: event.Time}); err != nil {
		return err
	}
	if l.Hub != nil {
		l.Hub.Publish(event)
	}
	return nil
}

type Replay func(context.Context, int64, int) ([]protocol.Event, error)
type Subscription struct {
	C      <-chan protocol.Event
	cancel func()
	closed chan struct{}
}

func (s *Subscription) Close() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

type subscriber struct {
	ch   chan protocol.Event
	stop chan struct{}
	once sync.Once
}

func (s *subscriber) close() { s.once.Do(func() { close(s.stop); close(s.ch) }) }

type Hub struct {
	mu      sync.Mutex
	max     int
	streams map[string]map[*subscriber]struct{}
}

func NewHub(maxQueue int) *Hub {
	if maxQueue < 1 {
		maxQueue = 1
	}
	return &Hub{max: maxQueue, streams: make(map[string]map[*subscriber]struct{})}
}

// Subscribe performs replay before joining live delivery while holding the
// run's lock. This makes the cursor boundary explicit: no event can be lost
// between replay and registration. A slow subscriber is closed and removed;
// the worker continues publishing and the run remains unaffected.
func (h *Hub) Subscribe(ctx context.Context, run string, after int64, replay Replay) (*Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := &subscriber{ch: make(chan protocol.Event, h.max), stop: make(chan struct{})}
	if replay != nil {
		events, err := replay(ctx, after, h.max)
		if err != nil {
			return nil, err
		}
		for _, e := range events {
			if e.Sequence > after {
				s.ch <- e
				after = e.Sequence
			}
		}
	}
	if h.streams[run] == nil {
		h.streams[run] = make(map[*subscriber]struct{})
	}
	h.streams[run][s] = struct{}{}
	sub := &Subscription{C: s.ch, closed: s.stop}
	sub.cancel = func() { h.remove(run, s) }
	return sub, nil
}
func (h *Hub) remove(run string, s *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.streams[run]; set != nil {
		if _, ok := set[s]; ok {
			delete(set, s)
			s.close()
		}
		if len(set) == 0 {
			delete(h.streams, run)
		}
	}
}
func (h *Hub) Publish(e protocol.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.streams[e.RunID] {
		select {
		case s.ch <- e:
		default:
			delete(h.streams[e.RunID], s)
			s.close()
		}
	}
}
func (h *Hub) SubscriberCount(run string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.streams[run])
}
