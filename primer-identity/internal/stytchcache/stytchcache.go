// Package stytchcache provides bounded, fail-closed session validation caching.
package stytchcache

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"container/list"

	"github.com/aleksclark/primer/identity/internal/stytch"
)

const (
	defaultPositiveTTL        = 15 * time.Second
	defaultNegativeTTL        = 5 * time.Second
	defaultPositiveCapacity   = 10000
	defaultNegativeCapacity   = 2000
	defaultLoadTimeout        = 3 * time.Second
	defaultMaxConcurrentLoads = 128
	maxMaxConcurrentLoads     = 256
	// Stytch session tokens are opaque bearer values. Rejecting anything over
	// this byte limit bounds work and memory before HMAC or load admission.
	maxSessionTokenBytes    = 4096
	maxPositivePayloadBytes = 16 * 1024 * 1024
	maxSnapshotPayloadBytes = 16 * 1024
	maxSnapshotIDLength     = 1024
	maxSnapshotIDRunes      = 255
	maxSnapshotRoleLength   = 512
	maxSnapshotRoleRunes    = 128
	maxAggregateRoleBytes   = 8192
)

var (
	ErrDenied      = errors.New("session denied")
	ErrUnavailable = errors.New("session validation unavailable")
)

// Config contains all cache dependencies and bounds. HMACKey should be an
// independent, process-scoped random secret; it is copied by New.
type Config struct {
	Client    stytch.StytchClient
	ProjectID string
	HMACKey   []byte
	Now       func() time.Time

	PositiveTTL      time.Duration
	NegativeTTL      time.Duration
	LoadTimeout      time.Duration
	PositiveCapacity int
	NegativeCapacity int
	// MaxConcurrentLoads bounds unique provider calls. Waiters for an existing
	// token join its call and do not consume another permit.
	MaxConcurrentLoads int
	// PositivePayloadBudget bounds the aggregate serialized size of positive
	// snapshots. Zero selects the 16 MiB safe default and larger values are
	// rejected rather than silently weakening the bound.
	PositivePayloadBudget int
}

// Stats is a bounded aggregate view of cache activity. It contains no cache
// keys, provider identifiers, or request data.
type Stats struct {
	PositiveHits           uint64
	NegativeHits           uint64
	Misses                 uint64
	Loads                  uint64
	TransientErrors        uint64
	Evictions              uint64
	PositiveEntries        uint64
	NegativeEntries        uint64
	PositiveBytes          uint64
	LoadOverloadRejections uint64
	InFlightLoads          uint64
}

type counters struct {
	positiveHits           atomic.Uint64
	negativeHits           atomic.Uint64
	misses                 atomic.Uint64
	loads                  atomic.Uint64
	transientErrors        atomic.Uint64
	evictions              atomic.Uint64
	loadOverloadRejections atomic.Uint64
	inFlightLoads          atomic.Uint64
}

type entry struct {
	key      string
	expires  time.Time
	snapshot stytch.SessionSnapshot
	bytes    int
}

type lruStore struct {
	mu               sync.Mutex
	capacity         int
	positive         bool
	entries          map[string]*list.Element
	order            *list.List
	entryCount       atomic.Uint64
	payloadBytes     atomic.Uint64
	maintenanceSteps atomic.Uint64
	payloadBudget    int
	evictions        *atomic.Uint64
}

func newLRU(capacity int, positive bool, payloadBudget int, evictions *atomic.Uint64) *lruStore {
	return &lruStore{
		capacity: capacity, positive: positive, payloadBudget: payloadBudget,
		entries: make(map[string]*list.Element), order: list.New(), evictions: evictions,
	}
}

func (s *lruStore) removeElementLocked(el *list.Element, countEviction bool) {
	e := el.Value.(*entry)
	delete(s.entries, e.key)
	s.order.Remove(el)
	s.entryCount.Add(^uint64(0))
	if s.positive && e.bytes > 0 {
		s.payloadBytes.Add(^uint64(e.bytes - 1))
	}
	if countEviction {
		s.evictions.Add(1)
	}
}

func (s *lruStore) get(key string, now time.Time) (stytch.SessionSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maintenanceSteps.Add(1)
	el, ok := s.entries[key]
	if !ok {
		return stytch.SessionSnapshot{}, false
	}
	if !el.Value.(*entry).expires.After(now) {
		s.removeElementLocked(el, true)
		return stytch.SessionSnapshot{}, false
	}
	s.order.MoveToFront(el)
	if s.positive {
		return cloneSnapshot(el.Value.(*entry).snapshot), true
	}
	return stytch.SessionSnapshot{}, true
}

func (s *lruStore) put(key string, expires time.Time, snapshot stytch.SessionSnapshot, payloadBytes int, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.positive && (payloadBytes <= 0 || payloadBytes > s.payloadBudget) {
		return false
	}
	if el, ok := s.entries[key]; ok {
		existing := el.Value.(*entry)
		existing.expires = expires
		if s.positive {
			s.payloadBytes.Add(^uint64(existing.bytes - 1))
			existing.snapshot = cloneSnapshot(snapshot)
			existing.bytes = payloadBytes
			s.payloadBytes.Add(uint64(payloadBytes))
		}
		s.order.MoveToFront(el)
		return true
	}
	for len(s.entries) >= s.capacity || (s.positive && s.payloadBytes.Load()+uint64(payloadBytes) > uint64(s.payloadBudget)) {
		s.maintenanceSteps.Add(1)
		s.removeElementLocked(s.order.Back(), true)
	}
	e := &entry{key: key, expires: expires, bytes: payloadBytes}
	if s.positive {
		e.snapshot = cloneSnapshot(snapshot)
	}
	s.entries[key] = s.order.PushFront(e)
	s.entryCount.Add(1)
	if s.positive {
		s.payloadBytes.Add(uint64(payloadBytes))
	}
	return true
}

func (s *lruStore) remove(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if el, ok := s.entries[key]; ok {
		s.removeElementLocked(el, false)
	}
}

func (s *lruStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]*list.Element)
	s.order.Init()
	s.entryCount.Store(0)
	s.payloadBytes.Store(0)
}

type loadResult struct {
	snapshot stytch.SessionSnapshot
	err      error
}

type loadCall struct {
	done   chan struct{}
	result loadResult
}

// Cache validates opaque sessions through the provider and caches only
// validated, bounded snapshots or definitive denials.
type Cache struct {
	client             stytch.StytchClient
	projectID          string
	hmacKey            []byte
	now                func() time.Time
	positiveTTL        time.Duration
	negativeTTL        time.Duration
	loadTimeout        time.Duration
	maxConcurrentLoads int
	positive           *lruStore
	negative           *lruStore
	loadsMu            sync.Mutex
	loads              map[string]*loadCall
	loadPermits        chan struct{}
	stats              counters
	fence              sync.RWMutex
	// generation is a single bounded global fence. Including it in the
	// singleflight key prevents post-invalidation joins without an unbounded
	// per-token epoch map; insertion and the generation check share fence.
	generation atomic.Uint64
}

// New constructs a validation cache. Zero TTL and capacity values select the
// safe defaults; negative values and non-positive explicit TTLs are rejected.
func New(cfg Config) (*Cache, error) {
	if cfg.Client == nil {
		return nil, errors.New("stytch cache client is required")
	}
	if strings.TrimSpace(cfg.ProjectID) == "" {
		return nil, errors.New("stytch cache project id is required")
	}
	if len(cfg.HMACKey) < 32 {
		return nil, errors.New("stytch cache hmac key must be at least 32 bytes")
	}
	if cfg.PositiveTTL == 0 {
		cfg.PositiveTTL = defaultPositiveTTL
	} else if cfg.PositiveTTL <= 0 || cfg.PositiveTTL > defaultPositiveTTL {
		return nil, errors.New("stytch cache positive ttl must be positive")
	}
	if cfg.NegativeTTL == 0 {
		cfg.NegativeTTL = defaultNegativeTTL
	} else if cfg.NegativeTTL <= 0 || cfg.NegativeTTL > defaultNegativeTTL {
		return nil, errors.New("stytch cache negative ttl must be positive")
	}
	if cfg.LoadTimeout == 0 {
		cfg.LoadTimeout = defaultLoadTimeout
	} else if cfg.LoadTimeout <= 0 || cfg.LoadTimeout > defaultLoadTimeout {
		return nil, errors.New("stytch cache load timeout must be positive and at most three seconds")
	}
	if cfg.PositiveCapacity == 0 {
		cfg.PositiveCapacity = defaultPositiveCapacity
	} else if cfg.PositiveCapacity <= 0 || cfg.PositiveCapacity > defaultPositiveCapacity {
		return nil, errors.New("stytch cache positive capacity must not be negative")
	}
	if cfg.NegativeCapacity == 0 {
		cfg.NegativeCapacity = defaultNegativeCapacity
	} else if cfg.NegativeCapacity <= 0 || cfg.NegativeCapacity > defaultNegativeCapacity {
		return nil, errors.New("stytch cache negative capacity must not be negative")
	}
	if cfg.MaxConcurrentLoads == 0 {
		cfg.MaxConcurrentLoads = defaultMaxConcurrentLoads
	} else if cfg.MaxConcurrentLoads <= 0 || cfg.MaxConcurrentLoads > maxMaxConcurrentLoads {
		return nil, errors.New("stytch cache max concurrent loads is out of bounds")
	}
	if cfg.PositivePayloadBudget == 0 {
		cfg.PositivePayloadBudget = maxPositivePayloadBytes
	} else if cfg.PositivePayloadBudget <= 0 || cfg.PositivePayloadBudget > maxPositivePayloadBytes {
		return nil, errors.New("stytch cache positive payload budget is out of bounds")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	key := append([]byte(nil), cfg.HMACKey...)
	c := &Cache{
		client: cfg.Client, projectID: cfg.ProjectID, hmacKey: key, now: cfg.Now,
		positiveTTL: cfg.PositiveTTL, negativeTTL: cfg.NegativeTTL, loadTimeout: cfg.LoadTimeout,
		maxConcurrentLoads: cfg.MaxConcurrentLoads,
		loads:              make(map[string]*loadCall), loadPermits: make(chan struct{}, cfg.MaxConcurrentLoads),
	}
	c.positive = newLRU(cfg.PositiveCapacity, true, cfg.PositivePayloadBudget, &c.stats.evictions)
	c.negative = newLRU(cfg.NegativeCapacity, false, 0, &c.stats.evictions)
	return c, nil
}

// AuthenticateSession returns a defensive copy of a validated provider
// snapshot. Provider failures other than definitive denial are never cached.
func (c *Cache) AuthenticateSession(ctx context.Context, token string) (stytch.SessionSnapshot, error) {
	if len(token) == 0 || len(token) > maxSessionTokenBytes {
		return stytch.SessionSnapshot{}, ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return stytch.SessionSnapshot{}, err
	}
	key := c.digest(token)
	c.fence.RLock()
	now := c.now()
	if snapshot, ok := c.positive.get(key, now); ok {
		c.fence.RUnlock()
		c.stats.positiveHits.Add(1)
		return snapshot, nil
	}
	if _, ok := c.negative.get(key, now); ok {
		c.fence.RUnlock()
		c.stats.negativeHits.Add(1)
		return stytch.SessionSnapshot{}, ErrDenied
	}
	c.fence.RUnlock()
	c.stats.misses.Add(1)

	// Misses briefly serialize only the admission/recheck path. Cache hits do
	// not take this mutex, so unrelated read sections remain concurrent.
	c.loadsMu.Lock()
	c.fence.RLock()
	now = c.now()
	if snapshot, ok := c.positive.get(key, now); ok {
		c.fence.RUnlock()
		c.loadsMu.Unlock()
		c.stats.positiveHits.Add(1)
		return snapshot, nil
	}
	if _, ok := c.negative.get(key, now); ok {
		c.fence.RUnlock()
		c.loadsMu.Unlock()
		c.stats.negativeHits.Add(1)
		return stytch.SessionSnapshot{}, ErrDenied
	}
	generation := c.generation.Load()
	call, admitted := c.joinOrStartLoadLocked(token, key, generation)
	c.fence.RUnlock()
	c.loadsMu.Unlock()
	if !admitted {
		return stytch.SessionSnapshot{}, ErrUnavailable
	}
	select {
	case <-ctx.Done():
		return stytch.SessionSnapshot{}, ctx.Err()
	case <-call.done:
		loaded := call.result
		c.fence.RLock()
		fresh := c.generation.Load() == generation
		c.fence.RUnlock()
		if !fresh {
			return stytch.SessionSnapshot{}, ErrUnavailable
		}
		if loaded.err != nil {
			return stytch.SessionSnapshot{}, loaded.err
		}
		return cloneSnapshot(loaded.snapshot), nil
	}
}

// joinOrStartLoadLocked deduplicates by HMAC key and generation. It is called
// with loadsMu and the invalidation read fence held, so an invalidation cannot
// advance the generation between lookup and registration.
func (c *Cache) joinOrStartLoadLocked(token, key string, generation uint64) (*loadCall, bool) {
	flight := flightKey(key, generation)
	if call, ok := c.loads[flight]; ok {
		return call, true
	}
	select {
	case c.loadPermits <- struct{}{}:
	default:
		c.stats.loadOverloadRejections.Add(1)
		return nil, false
	}
	call := &loadCall{done: make(chan struct{})}
	c.loads[flight] = call
	c.stats.loads.Add(1)
	c.stats.inFlightLoads.Add(1)
	go c.runLoad(token, key, generation, flight, call)
	return call, true
}

func (c *Cache) runLoad(token, key string, generation uint64, flight string, call *loadCall) {
	loaded := c.load(token, key, generation)
	c.loadsMu.Lock()
	if current, ok := c.loads[flight]; ok && current == call {
		delete(c.loads, flight)
	}
	<-c.loadPermits
	c.stats.inFlightLoads.Add(^uint64(0))
	c.loadsMu.Unlock()
	call.result = loaded
	close(call.done)
}

func (c *Cache) load(token, key string, generation uint64) loadResult {
	providerCtx, cancel := context.WithTimeout(context.Background(), c.loadTimeout)
	defer cancel()
	snapshot, err := c.client.AuthenticateSession(providerCtx, token)
	if err != nil {
		if ctxErr := providerCtx.Err(); ctxErr != nil {
			c.stats.transientErrors.Add(1)
			return loadResult{err: ErrUnavailable}
		}
		if errors.Is(err, stytch.ErrDefinitive) {
			c.cacheNegative(key, generation)
			return loadResult{err: ErrDenied}
		}
		c.stats.transientErrors.Add(1)
		return loadResult{err: ErrUnavailable}
	}
	if ctxErr := providerCtx.Err(); ctxErr != nil {
		c.stats.transientErrors.Add(1)
		return loadResult{err: ErrUnavailable}
	}
	payloadBytes, safe := validateSnapshotPayload(snapshot)
	if !safe {
		c.stats.transientErrors.Add(1)
		return loadResult{err: ErrUnavailable}
	}
	now := c.now()
	if snapshot.ProjectID != c.projectID || !snapshot.Active || !snapshot.Eligible ||
		snapshot.OrganizationID == "" || snapshot.MemberID == "" || !snapshot.ExpiresAt.After(now) {
		c.cacheNegative(key, generation)
		return loadResult{err: ErrDenied}
	}
	expires := now.Add(c.positiveTTL)
	if snapshot.ExpiresAt.Before(expires) {
		expires = snapshot.ExpiresAt
	}
	c.fence.Lock()
	defer c.fence.Unlock()
	if c.generation.Load() != generation {
		return loadResult{err: ErrUnavailable}
	}
	c.positive.put(key, expires, snapshot, payloadBytes, now)
	return loadResult{snapshot: cloneSnapshot(snapshot)}
}

func (c *Cache) cacheNegative(key string, generation uint64) {
	c.fence.Lock()
	defer c.fence.Unlock()
	now := c.now()
	if c.generation.Load() == generation {
		c.negative.put(key, now.Add(c.negativeTTL), stytch.SessionSnapshot{}, 0, now)
	}
}

// Invalidate removes one locally cached validation. It never revokes the
// provider session.
func (c *Cache) Invalidate(token string) {
	c.fence.Lock()
	defer c.fence.Unlock()
	key := c.digest(token)
	c.generation.Add(1)
	c.positive.remove(key)
	c.negative.remove(key)
}

// InvalidateAll clears both bounded stores without invoking the provider.
func (c *Cache) InvalidateAll() {
	c.fence.Lock()
	defer c.fence.Unlock()
	c.generation.Add(1)
	c.positive.clear()
	c.negative.clear()
}

// Stats returns aggregate counters and current bounded entry counts.
func (c *Cache) Stats() Stats {
	return Stats{
		PositiveHits: c.stats.positiveHits.Load(), NegativeHits: c.stats.negativeHits.Load(),
		Misses: c.stats.misses.Load(), Loads: c.stats.loads.Load(),
		TransientErrors: c.stats.transientErrors.Load(), Evictions: c.stats.evictions.Load(),
		PositiveEntries: c.positive.entryCount.Load(), NegativeEntries: c.negative.entryCount.Load(),
		PositiveBytes:          c.positive.payloadBytes.Load(),
		LoadOverloadRejections: c.stats.loadOverloadRejections.Load(),
		InFlightLoads:          c.stats.inFlightLoads.Load(),
	}
}

func flightKey(key string, generation uint64) string {
	return key + "\x00" + strconv.FormatUint(generation, 10)
}

func validateSnapshotPayload(snapshot stytch.SessionSnapshot) (int, bool) {
	if !validSnapshotText(snapshot.ProjectID, maxSnapshotIDRunes, maxSnapshotIDLength, false) ||
		!validSnapshotText(snapshot.OrganizationID, maxSnapshotIDRunes, maxSnapshotIDLength, true) ||
		!validSnapshotText(snapshot.MemberID, maxSnapshotIDRunes, maxSnapshotIDLength, true) ||
		len(snapshot.Roles) > stytch.MaxSessionRoles {
		return 0, false
	}
	roleBytes := 0
	for _, role := range snapshot.Roles {
		if !validSnapshotText(role, maxSnapshotRoleRunes, maxSnapshotRoleLength, true) {
			return 0, false
		}
		roleBytes += len(role)
		if roleBytes > maxAggregateRoleBytes {
			return 0, false
		}
	}
	size := snapshotPayloadSize(snapshot)
	return size, size > 0 && size <= maxSnapshotPayloadBytes
}

func validSnapshotText(value string, maxRunes, maxBytes int, allowEmpty bool) bool {
	if (!allowEmpty && value == "") || !utf8.ValidString(value) || len(value) > maxBytes || utf8.RuneCountInString(value) > maxRunes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func snapshotPayloadSize(snapshot stytch.SessionSnapshot) int {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return 0
	}
	return len(payload)
}

func (c *Cache) digest(token string) string {
	mac := hmac.New(sha256.New, c.hmacKey)
	_, _ = mac.Write([]byte(c.projectID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(token))
	return string(mac.Sum(nil))
}

func cloneSnapshot(snapshot stytch.SessionSnapshot) stytch.SessionSnapshot {
	snapshot.Roles = append([]string(nil), snapshot.Roles...)
	return snapshot
}

// String deliberately avoids exposing internal fields, cache keys, or the
// secret key through formatting and diagnostics.
func (c *Cache) String() string { return "stytch session validation cache" }

// GoString protects Go-syntax formatting (%#v) from exposing internal cache
// state as well.
func (c *Cache) GoString() string { return c.String() }
