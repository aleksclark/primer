package token

import (
	"context"
	"sync"
	"time"
)

const (
	unknownKidRefreshTimeout  = 2 * time.Second
	unknownKidRefreshCooldown = 5 * time.Second
	unknownKidNegativeTTL     = 5 * time.Second
	unknownKidNegativeMax     = 256
)

type refreshCoordinator struct {
	mu          sync.Mutex
	lastAttempt time.Time
	lastOutcome refreshOutcome
	negatives   map[string]time.Time
	active      *refreshAttempt
	beforeBegin func()
}

type refreshAttempt struct {
	done      chan struct{}
	finalized bool
	outcome   refreshOutcome
	running   bool
}

type refreshDecision struct {
	attempt *refreshAttempt
	launch  bool
	err     error
}

type refreshOutcome uint8

const (
	refreshOutcomeNone refreshOutcome = iota
	refreshOutcomeSuccess
	refreshOutcomeFailure
)

func newRefreshCoordinator() *refreshCoordinator {
	return &refreshCoordinator{negatives: make(map[string]time.Time, 8)}
}

func (c *refreshCoordinator) maybeRefresh(ctx context.Context, clock Clock, kid string, keys PublicKeySource) error {
	if c == nil || keys == nil {
		return denyUnavailable()
	}
	if ctx == nil || ctx.Err() != nil {
		return denyUnavailable()
	}
	if c.beforeBegin != nil {
		c.beforeBegin()
	}
	decision := c.begin(kid, clock.Now().UTC())
	if decision.err != nil {
		return decision.err
	}
	if decision.launch {
		c.launch(decision.attempt, clock, keys)
	}
	return c.await(ctx, decision.attempt)
}

func (c *refreshCoordinator) begin(kid string, now time.Time) refreshDecision {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked(now)
	if c.active != nil {
		if c.active.running {
			if c.active.finalized && c.active.outcome != refreshOutcomeSuccess {
				return refreshDecision{err: denyUnavailable()}
			}
			return refreshDecision{attempt: c.active}
		}
		c.active = nil
	}
	if err := c.suppressedLocked(kid, now); err != nil {
		return refreshDecision{err: err}
	}
	attempt := &refreshAttempt{done: make(chan struct{}), running: true}
	c.active = attempt
	return refreshDecision{attempt: attempt, launch: true}
}

func (c *refreshCoordinator) launch(attempt *refreshAttempt, clock Clock, keys PublicKeySource) {
	refreshCtx, cancel := context.WithTimeout(context.Background(), unknownKidRefreshTimeout)
	go func() {
		timer := time.NewTimer(unknownKidRefreshTimeout)
		defer timer.Stop()
		select {
		case <-timer.C:
			c.failAttempt(attempt, clock.Now().UTC())
		case <-attempt.done:
		}
	}()
	go func() {
		defer cancel()
		err := keys.Refresh(refreshCtx)
		if refreshCtx.Err() != nil {
			err = denyUnavailable()
		}
		c.completeCallback(attempt, err, clock.Now().UTC())
	}()
}

func (c *refreshCoordinator) await(ctx context.Context, attempt *refreshAttempt) error {
	if attempt == nil {
		return denyUnavailable()
	}
	select {
	case <-ctx.Done():
		return denyUnavailable()
	case <-attempt.done:
		c.mu.Lock()
		outcome := attempt.outcome
		c.mu.Unlock()
		if outcome == refreshOutcomeSuccess {
			return nil
		}
		return denyUnavailable()
	}
}

func (c *refreshCoordinator) completeCallback(attempt *refreshAttempt, err error, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.finishAttemptLocked(attempt, err, now)
	attempt.running = false
	if c.active == attempt {
		c.active = nil
	}
}

func (c *refreshCoordinator) finishAttemptLocked(attempt *refreshAttempt, err error, now time.Time) bool {
	outcome := refreshOutcomeFailure
	if err == nil {
		outcome = refreshOutcomeSuccess
	}
	if attempt.finalized {
		return false
	}
	attempt.finalized = true
	attempt.outcome = outcome
	close(attempt.done)
	c.recordAttemptLocked(now, err)
	return true
}

func (c *refreshCoordinator) failAttempt(attempt *refreshAttempt, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !attempt.finalized {
		attempt.finalized = true
		attempt.outcome = refreshOutcomeFailure
		close(attempt.done)
		c.recordAttemptLocked(now, denyUnavailable())
	}
}

func (c *refreshCoordinator) suppressedLocked(kid string, now time.Time) error {
	if !c.lastAttempt.IsZero() && !now.After(c.lastAttempt.Add(unknownKidRefreshCooldown)) {
		if c.lastOutcome == refreshOutcomeFailure {
			return denyUnavailable()
		}
		return denyInvalid()
	}
	if exp, ok := c.negatives[kid]; ok && now.Before(exp) {
		return denyInvalid()
	}
	return nil
}

func (c *refreshCoordinator) recordAttemptLocked(now time.Time, err error) {
	c.lastAttempt = now
	if err != nil {
		c.lastOutcome = refreshOutcomeFailure
	} else {
		c.lastOutcome = refreshOutcomeSuccess
	}
}

func (c *refreshCoordinator) rememberNegative(kid string, now time.Time) {
	if c == nil || kid == "" || len(kid) > 128 {
		return
	}
	now = now.UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked(now)
	if len(c.negatives) >= unknownKidNegativeMax {
		for existing := range c.negatives {
			delete(c.negatives, existing)
			break
		}
	}
	c.negatives[kid] = now.Add(unknownKidNegativeTTL)
}

func (c *refreshCoordinator) evictExpiredLocked(now time.Time) {
	for kid, exp := range c.negatives {
		if !now.Before(exp) {
			delete(c.negatives, kid)
		}
	}
}
