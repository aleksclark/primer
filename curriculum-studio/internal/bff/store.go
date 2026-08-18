package bff

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const (
	// PreAuthTTL is the exact 10-minute pre-auth lifetime.
	PreAuthTTL = 10 * time.Minute
	// minVerifierLen is RFC 7636 minimum verifier length.
	minVerifierLen = 43
	// maxVerifierLen is RFC 7636 maximum verifier length.
	maxVerifierLen = 128
	// sessionIDBytes is 256-bit session identifier entropy.
	sessionIDBytes = 32
	// stateBytes is 256-bit opaque state entropy.
	stateBytes = 32
)

// PreAuth is the server-side 10-minute authorization record. The verifier
// never leaves the BFF.
type PreAuth struct {
	Product       Product
	State         string
	CodeVerifier  string
	CodeChallenge string
	ClientID      string
	RedirectURI   string
	Resource      string
	Audience      string
	Issuer        string
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

// Session is server-side token custody keyed by a random opaque cookie.
type Session struct {
	ID           string
	Product      Product
	Subject      string
	ClientID     string
	Audience     string
	AccessToken  string
	RefreshToken string
	Scope        string
	ExpiresAt    time.Time
	CSRF         string
	CreatedAt    time.Time
}

// Store persists pre-auth and session records. Implementations must not log
// access, refresh, or verifier material.
type Store interface {
	PutPreAuth(rec PreAuth) error
	TakePreAuth(state string) (PreAuth, bool)
	GetPreAuth(state string) (PreAuth, bool)
	PreAuthCount() int
	PutSession(sess Session) error
	GetSession(id string) (Session, bool)
	DeleteSession(id string) error
}

// MemoryStore is an in-process store used by tests and development. Production
// wiring must provide a durable Store so a BFF restart does not invalidate or
// orphan browser sessions.
type MemoryStore struct {
	mu       sync.Mutex
	preAuth  map[string]PreAuth
	sessions map[string]Session
}

// NewMemoryStore returns an empty in-process store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		preAuth:  make(map[string]PreAuth),
		sessions: make(map[string]Session),
	}
}

// PutPreAuth stores a pre-auth record keyed by state.
func (s *MemoryStore) PutPreAuth(rec PreAuth) error {
	if rec.State == "" {
		return fmt.Errorf("bff store: pre-auth state is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preAuth[rec.State] = rec
	return nil
}

// TakePreAuth atomically loads and deletes a pre-auth record.
func (s *MemoryStore) TakePreAuth(state string) (PreAuth, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.preAuth[state]
	if !ok {
		return PreAuth{}, false
	}
	delete(s.preAuth, state)
	return rec, true
}

// GetPreAuth returns a pre-auth record without consuming it (tests).
func (s *MemoryStore) GetPreAuth(state string) (PreAuth, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.preAuth[state]
	return rec, ok
}

// PreAuthCount returns the number of live pre-auth records.
func (s *MemoryStore) PreAuthCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.preAuth)
}

// PutSession stores a session by id.
func (s *MemoryStore) PutSession(sess Session) error {
	if sess.ID == "" {
		return fmt.Errorf("bff store: session id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
	return nil
}

// GetSession returns a session by id.
func (s *MemoryStore) GetSession(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

// DeleteSession removes a session.
func (s *MemoryStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func randomURLToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
