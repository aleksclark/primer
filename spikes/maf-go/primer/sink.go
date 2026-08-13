package primer

import (
	"context"
	"sync"
	"time"
)

// BoundedSink is a best-effort, non-blocking event sink.
// Dropped events increment Dropped; they never fail the underlying run.
type BoundedSink struct {
	cap     int
	mu      sync.Mutex
	events  []RunEvent
	Dropped int
	// BlockFor simulates a slow consumer on Emit (tests only).
	// When >0, Emit sleeps up to BlockFor but still respects a short internal timeout
	// so the runner is not permanently blocked.
	BlockFor time.Duration
	// OnEmit is an optional hook invoked after a successful enqueue.
	OnEmit func(RunEvent)
}

// NewBoundedSink creates a sink that retains at most capacity events.
func NewBoundedSink(capacity int) *BoundedSink {
	if capacity < 1 {
		capacity = 1
	}
	return &BoundedSink{cap: capacity, events: make([]RunEvent, 0, capacity)}
}

// Emit enqueues e without blocking the caller for long.
func (s *BoundedSink) Emit(ctx context.Context, e RunEvent) {
	if s == nil {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if s.BlockFor > 0 {
		// Bounded wait: never hang the runner on a slow subscriber.
		timer := time.NewTimer(min(s.BlockFor, 25*time.Millisecond))
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) >= s.cap {
		s.Dropped++
		return
	}
	s.events = append(s.events, e)
	if s.OnEmit != nil {
		s.OnEmit(e)
	}
}

// Snapshot returns a copy of retained events.
func (s *BoundedSink) Snapshot() []RunEvent {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RunEvent, len(s.events))
	copy(out, s.events)
	return out
}

// CollectingSink records every event without bound (tests only; not for production).
type CollectingSink struct {
	mu     sync.Mutex
	events []RunEvent
	// OnEmit optional hook after each event (tests: barriers).
	OnEmit func(RunEvent)
}

func (s *CollectingSink) Emit(_ context.Context, e RunEvent) {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	s.mu.Lock()
	s.events = append(s.events, e)
	hook := s.OnEmit
	s.mu.Unlock()
	if hook != nil {
		hook(e)
	}
}

func (s *CollectingSink) Snapshot() []RunEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RunEvent, len(s.events))
	copy(out, s.events)
	return out
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
