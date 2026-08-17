package token

import (
	"context"
	"sync"

	"github.com/aleksclark/primer/identity/internal/domain"
)

const (
	maxPublishedPublicKeys = 16
	maxKidBindingHistory   = 1024
)

// KeySet is a refreshable public-key cache. Duplicate kids fail closed.
type KeySet struct {
	mu             sync.RWMutex
	keys           map[string]domain.PublicJWK
	bindingHistory map[string]domain.PublicJWK
	next           uint64
	completed      uint64
	hasDone        bool
	load           func(ctx context.Context) ([]domain.PublicJWK, error)
	beforePublish  func()
}

// NewKeySet constructs an empty set that reloads from load on Refresh.
func NewKeySet(load func(ctx context.Context) ([]domain.PublicJWK, error)) (*KeySet, error) {
	if load == nil {
		return nil, denyUnavailable()
	}
	return &KeySet{
		keys:           map[string]domain.PublicJWK{},
		bindingHistory: make(map[string]domain.PublicJWK, maxKidBindingHistory),
		load:           load,
	}, nil
}

// Lookup returns a copied public JWK when the kid is present.
func (s *KeySet) Lookup(ctx context.Context, kid string) (domain.PublicJWK, bool, error) {
	if s == nil {
		return domain.PublicJWK{}, false, denyUnavailable()
	}
	if ctx == nil || ctx.Err() != nil {
		return domain.PublicJWK{}, false, denyUnavailable()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ctx.Err() != nil {
		return domain.PublicJWK{}, false, denyUnavailable()
	}
	jwk, ok := s.keys[kid]
	if !ok {
		return domain.PublicJWK{}, false, nil
	}
	return jwk, true, nil
}

// Refresh replaces the set from the loader after validating every key.
func (s *KeySet) Refresh(ctx context.Context) error {
	if s == nil || s.load == nil {
		return denyUnavailable()
	}
	if ctx == nil || ctx.Err() != nil {
		return denyUnavailable()
	}
	generation := s.startGeneration()
	if ctx.Err() != nil {
		return denyUnavailable()
	}
	loaded, err := s.load(ctx)
	if ctx.Err() != nil {
		return s.finishGeneration(ctx, generation, nil, denyUnavailable())
	}
	if err != nil {
		return s.finishGeneration(ctx, generation, nil, denyUnavailable())
	}
	if len(loaded) == 0 || len(loaded) > maxPublishedPublicKeys {
		return s.finishGeneration(ctx, generation, nil, denyInvalid())
	}
	next := make(map[string]domain.PublicJWK, len(loaded))
	for _, jwk := range loaded {
		if _, err := acceptPublicJWK(jwk.Kid, jwk); err != nil {
			return s.finishGeneration(ctx, generation, nil, denyInvalid())
		}
		if _, dup := next[jwk.Kid]; dup {
			return s.finishGeneration(ctx, generation, nil, denyInvalid())
		}
		next[jwk.Kid] = jwk
	}
	return s.finishGeneration(ctx, generation, next, nil)
}

func (s *KeySet) startGeneration() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	generation := s.next
	s.next++
	return generation
}

func (s *KeySet) finishGeneration(ctx context.Context, generation uint64, next map[string]domain.PublicJWK, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.beforePublish != nil {
		s.beforePublish()
	}
	if ctx == nil || ctx.Err() != nil {
		return denyUnavailable()
	}
	if !s.hasDone || generationAfter(generation, s.completed) {
		s.completed = generation
		s.hasDone = true
		if err != nil {
			return err
		}
		if s.bindingHistory == nil {
			s.bindingHistory = make(map[string]domain.PublicJWK, maxKidBindingHistory)
		}
		additions := make(map[string]domain.PublicJWK)
		for kid, jwk := range next {
			observed, ok := s.bindingHistory[kid]
			if ok {
				if observed != jwk {
					return denyUnavailable()
				}
				continue
			}
			additions[kid] = jwk
		}
		if len(s.bindingHistory)+len(additions) > maxKidBindingHistory {
			return denyUnavailable()
		}
		for kid, jwk := range additions {
			s.bindingHistory[kid] = jwk
		}
		s.keys = next
		return nil
	}
	if err == nil {
		return denyUnavailable()
	}
	return err
}

func generationAfter(candidate, current uint64) bool {
	return candidate != current && int64(candidate-current) > 0
}
