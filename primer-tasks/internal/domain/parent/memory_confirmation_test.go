package parent

import (
	"context"
	"crypto/subtle"
	"fmt"
	"sync"
	"time"
)

// Test double only. Production binaries contain only the PostgreSQL store.
type MemoryConfirmationStore struct {
	mu   sync.Mutex
	rows map[string]ConfirmationPreview
	Now  func() time.Time
}

func NewMemoryConfirmationStore() *MemoryConfirmationStore {
	return &MemoryConfirmationStore{rows: make(map[string]ConfirmationPreview), Now: func() time.Time { return time.Now().UTC() }}
}
func (s *MemoryConfirmationStore) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}
func (s *MemoryConfirmationStore) Issue(_ context.Context, p ConfirmationPreview) (ConfirmationPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	p, err = validatePreview(p, s.now())
	if err != nil {
		return ConfirmationPreview{}, err
	}
	if _, exists := s.rows[p.Handle]; exists {
		return ConfirmationPreview{}, fmt.Errorf("%w: duplicate handle", ErrInvalidInput)
	}
	s.rows[p.Handle] = p
	return p, nil
}
func (s *MemoryConfirmationStore) Consume(_ context.Context, handle, tenant, actor, digest string) (ConfirmationPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.rows[handle]
	if !ok {
		return ConfirmationPreview{}, ErrConfirmationNotFound
	}
	now := s.now()
	if p.ConsumedAt != nil {
		return ConfirmationPreview{}, ErrConfirmationReplay
	}
	if !p.ExpiresAt.After(now) {
		return ConfirmationPreview{}, ErrConfirmationExpired
	}
	if subtle.ConstantTimeCompare([]byte(p.TenantID), []byte(tenant)) != 1 || subtle.ConstantTimeCompare([]byte(p.ActorID), []byte(actor)) != 1 {
		return ConfirmationPreview{}, ErrConfirmationForeign
	}
	if subtle.ConstantTimeCompare([]byte(p.ActionDigest), []byte(digest)) != 1 {
		return ConfirmationPreview{}, ErrConfirmationAltered
	}
	p.ConsumedAt = &now
	s.rows[handle] = p
	return p, nil
}
