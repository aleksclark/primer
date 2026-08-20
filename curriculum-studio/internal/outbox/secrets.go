package outbox

import (
	"context"
	"fmt"
	"sync"
)

// SecretResolver maps a secret_ref pointer to HMAC bytes. The raw secret
// never lives on webhook_endpoints and must not be logged.
type SecretResolver interface {
	Resolve(ctx context.Context, secretRef string) ([]byte, error)
}

// MemorySecrets is an in-memory resolver for tests and credential-free CI.
type MemorySecrets struct {
	mu   sync.RWMutex
	byID map[string][]byte
}

// NewMemorySecrets returns an empty in-memory secret store.
func NewMemorySecrets() *MemorySecrets {
	return &MemorySecrets{byID: map[string][]byte{}}
}

// Put stores secret bytes under secretRef. Tests own the mapping.
func (s *MemorySecrets) Put(secretRef string, secret []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(secret))
	copy(cp, secret)
	s.byID[secretRef] = cp
}

// Resolve returns a copy of the secret for secretRef.
func (s *MemorySecrets) Resolve(_ context.Context, secretRef string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	secret, ok := s.byID[secretRef]
	if !ok {
		return nil, fmt.Errorf("unknown secret_ref")
	}
	cp := make([]byte, len(secret))
	copy(cp, secret)
	return cp, nil
}
