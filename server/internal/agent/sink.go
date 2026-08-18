package agent

import (
	"context"
	"sync"
	"time"
)

// BoundedSink is a best-effort, bounded event sink. Dropped events are counted
// but never fail the underlying run. Safe for concurrent use.
type BoundedSink struct {
	cap    int
	mu     sync.Mutex
	events []RunEvent
	// Dropped counts events not enqueued (capacity or test BlockFor timeout).
	Dropped int
	// BlockFor simulates a slow consumer on Emit (tests only).
	// Emit sleeps up to min(BlockFor, 25ms) then drops if still full.
	BlockFor time.Duration
	// OnEmit is an optional hook invoked after a successful enqueue (tests).
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
		wait := s.BlockFor
		if wait > 25*time.Millisecond {
			wait = 25 * time.Millisecond
		}
		timer := time.NewTimer(wait)
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

// CollectingSink records every event without bound. For tests only.
type CollectingSink struct {
	mu     sync.Mutex
	events []RunEvent
	// OnEmit is an optional barrier hook (tests).
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

// Snapshot returns a copy of all collected events.
func (s *CollectingSink) Snapshot() []RunEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RunEvent, len(s.events))
	copy(out, s.events)
	return out
}
