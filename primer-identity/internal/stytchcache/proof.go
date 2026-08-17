package stytchcache

import (
	"container/list"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"github.com/aleksclark/primer/identity/internal/stytch"
)

const (
	// DefaultProofCacheCapacity is the process-scoped official proof budget.
	DefaultProofCacheCapacity = 256
	// MaxProofCacheCapacity is the hard maximum accepted by NewProofCache.
	MaxProofCacheCapacity = 1024
)

// ProofConfig configures the small, non-authoritative positive provider proof
// cache. Keys are HMAC digests of the exact tuple/session ID, never tokens.
type ProofConfig struct {
	HMACKey  []byte
	TTL      time.Duration
	Capacity int
	Now      func() time.Time
}

type ProofStats struct{ Entries uint64 }

type proofEntry struct {
	key      string
	expires  time.Time
	snapshot stytch.SessionSnapshot
}

// ProofCache contains only short-lived positive proofs. It deliberately has no
// method to record a negative or transient provider result.
type ProofCache struct {
	key      []byte
	ttl      time.Duration
	capacity int
	now      func() time.Time
	mu       sync.Mutex
	entries  map[string]*list.Element
	order    *list.List
}

func NewProofCache(cfg ProofConfig) (*ProofCache, error) {
	if len(cfg.HMACKey) < 32 || cfg.TTL <= 0 || cfg.TTL > 15*time.Second || cfg.Capacity <= 0 || cfg.Capacity > MaxProofCacheCapacity {
		return nil, errors.New("invalid provider proof cache configuration")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &ProofCache{key: append([]byte(nil), cfg.HMACKey...), ttl: cfg.TTL, capacity: cfg.Capacity, now: cfg.Now, entries: make(map[string]*list.Element), order: list.New()}, nil
}

var _ stytch.SessionProofs = (*ProofCache)(nil)

func (c *ProofCache) digest(project, org, member, session string) string {
	mac := hmac.New(sha256.New, c.key)
	for _, value := range []string{project, org, member, session} {
		var l [4]byte
		l[0] = byte(len(value) >> 24)
		l[1] = byte(len(value) >> 16)
		l[2] = byte(len(value) >> 8)
		l[3] = byte(len(value))
		_, _ = mac.Write(l[:])
		_, _ = mac.Write([]byte(value))
	}
	return string(mac.Sum(nil))
}

// Put records only a valid currently unexpired positive proof. Its lifetime is
// capped by both the configured <=15s TTL and the provider session expiry.
func (c *ProofCache) Put(snapshot stytch.SessionSnapshot) bool {
	now := c.now()
	if c == nil || !snapshot.Active || !snapshot.Eligible || snapshot.ProjectID == "" || snapshot.OrganizationID == "" || snapshot.MemberID == "" || snapshot.ProviderMemberSessionID == "" || !snapshot.ExpiresAt.After(now) {
		return false
	}
	expires := now.Add(c.ttl)
	if snapshot.ExpiresAt.Before(expires) {
		expires = snapshot.ExpiresAt
	}
	key := c.digest(snapshot.ProjectID, snapshot.OrganizationID, snapshot.MemberID, snapshot.ProviderMemberSessionID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[key]; ok {
		existing.Value.(*proofEntry).expires = expires
		existing.Value.(*proofEntry).snapshot = cloneSnapshot(snapshot)
		c.order.MoveToFront(existing)
		return true
	}
	for len(c.entries) >= c.capacity {
		tail := c.order.Back()
		delete(c.entries, tail.Value.(*proofEntry).key)
		c.order.Remove(tail)
	}
	c.entries[key] = c.order.PushFront(&proofEntry{key: key, expires: expires, snapshot: cloneSnapshot(snapshot)})
	return true
}

// Get returns no stale proof: both cache TTL and provider expiry are checked.
func (c *ProofCache) Get(project, org, member, session string) (stytch.SessionSnapshot, bool) {
	if c == nil {
		return stytch.SessionSnapshot{}, false
	}
	key := c.digest(project, org, member, session)
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return stytch.SessionSnapshot{}, false
	}
	e := el.Value.(*proofEntry)
	if !e.expires.After(now) || !e.snapshot.ExpiresAt.After(now) {
		delete(c.entries, key)
		c.order.Remove(el)
		return stytch.SessionSnapshot{}, false
	}
	c.order.MoveToFront(el)
	return cloneSnapshot(e.snapshot), true
}

func (c *ProofCache) Invalidate(project, org, member, session string) {
	if c == nil {
		return
	}
	key := c.digest(project, org, member, session)
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		delete(c.entries, key)
		c.order.Remove(el)
	}
}
func (c *ProofCache) Stats() ProofStats {
	if c == nil {
		return ProofStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return ProofStats{Entries: uint64(len(c.entries))}
}

func (c *ProofCache) String() string { return "stytch provider proof cache" }

func (c *ProofCache) GoString() string { return c.String() }
