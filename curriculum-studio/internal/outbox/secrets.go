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

// SecretStore stores and resolves raw HMAC secrets by their persisted pointer.
// Implementations must keep secret material out of logs and API responses.
type SecretStore interface {
	SecretResolver
	Put(secretRef string, secret []byte)
}

// MemorySecrets is an in-memory store for tests and credential-free CI.
type MemorySecrets struct {
	mu   sync.RWMutex
	byID map[string][]byte
}

// NewMemorySecrets returns an empty in-memory secret store.
func NewMemorySecrets() *MemorySecrets {
	return &MemorySecrets{byID: map[string][]byte{}}
}

// Put stores secret bytes under secretRef. Callers own the mapping.
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
