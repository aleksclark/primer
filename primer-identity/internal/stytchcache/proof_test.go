package stytchcache

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/stytch"
)

func TestProofCacheExpiryCapacityAndProviderExpiry(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	cache, err := NewProofCache(ProofConfig{HMACKey: bytes.Repeat([]byte{1}, 32), TTL: 15 * time.Second, Capacity: 1, Now: func() time.Time { return now }})
	require.NoError(t, err)
	one := stytch.SessionSnapshot{ProjectID: "p", OrganizationID: "o", MemberID: "m", ProviderMemberSessionID: "s", Active: true, Eligible: true, ExpiresAt: now.Add(3 * time.Second)}
	require.True(t, cache.Put(one))
	_, ok := cache.Get("p", "o", "m", "s")
	require.True(t, ok)
	now = now.Add(4 * time.Second)
	_, ok = cache.Get("p", "o", "m", "s")
	require.False(t, ok, "provider expiry must defeat any cache TTL")

	two := one
	two.ProviderMemberSessionID = "s2"
	two.ExpiresAt = now.Add(time.Hour)
	three := two
	three.ProviderMemberSessionID = "s3"
	require.True(t, cache.Put(two))
	require.True(t, cache.Put(three))
	require.Equal(t, uint64(1), cache.Stats().Entries)
}

func TestProofCacheRejectsInvalidOrTransientResultsWithoutMutation(t *testing.T) {
	now := time.Now().UTC()
	cache, err := NewProofCache(ProofConfig{HMACKey: bytes.Repeat([]byte{1}, 32), TTL: time.Second, Capacity: 2, Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.False(t, cache.Put(stytch.SessionSnapshot{}))
	require.Equal(t, uint64(0), cache.Stats().Entries)
}

func TestProofCacheCopiesHMACKeyAndNeverExposesRawSessionMaterial(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	key := bytes.Repeat([]byte{9}, 32)
	cache, err := NewProofCache(ProofConfig{HMACKey: key, TTL: 15 * time.Second, Capacity: 4, Now: func() time.Time { return now }})
	require.NoError(t, err)
	key[0] ^= 0xff
	snap := stytch.SessionSnapshot{ProjectID: "p", OrganizationID: "o", MemberID: "m", ProviderMemberSessionID: "raw-session-must-not-appear", Active: true, Eligible: true, ExpiresAt: now.Add(time.Minute)}
	require.True(t, cache.Put(snap))
	got, ok := cache.Get("p", "o", "m", "raw-session-must-not-appear")
	require.True(t, ok)
	require.Equal(t, "raw-session-must-not-appear", got.ProviderMemberSessionID)
	require.NotContains(t, cache.String(), "raw-session")
	require.NotContains(t, fmt.Sprintf("%#v", cache), "raw-session")
}

func TestProofCacheRejectsTTLOverFifteenSecondsAndShortKey(t *testing.T) {
	_, err := NewProofCache(ProofConfig{HMACKey: bytes.Repeat([]byte{1}, 31), TTL: time.Second, Capacity: 1})
	require.Error(t, err)
	_, err = NewProofCache(ProofConfig{HMACKey: bytes.Repeat([]byte{1}, 32), TTL: 16 * time.Second, Capacity: 1})
	require.Error(t, err)
	_, err = NewProofCache(ProofConfig{HMACKey: bytes.Repeat([]byte{1}, 32), TTL: 0, Capacity: 1})
	require.Error(t, err)
	_, err = NewProofCache(ProofConfig{HMACKey: bytes.Repeat([]byte{1}, 32), TTL: time.Second, Capacity: MaxProofCacheCapacity + 1})
	require.Error(t, err)
}

func TestProofCacheDoesNotCacheInactiveOrExpiredAndInvalidatesOneKey(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	cache, err := NewProofCache(ProofConfig{HMACKey: bytes.Repeat([]byte{1}, 32), TTL: 15 * time.Second, Capacity: 4, Now: func() time.Time { return now }})
	require.NoError(t, err)
	inactive := stytch.SessionSnapshot{ProjectID: "p", OrganizationID: "o", MemberID: "m", ProviderMemberSessionID: "s", Active: false, Eligible: true, ExpiresAt: now.Add(time.Minute)}
	require.False(t, cache.Put(inactive))
	expired := inactive
	expired.Active = true
	expired.ExpiresAt = now
	require.False(t, cache.Put(expired))
	one := expired
	one.ExpiresAt = now.Add(time.Minute)
	two := one
	two.ProviderMemberSessionID = "s2"
	require.True(t, cache.Put(one))
	require.True(t, cache.Put(two))
	cache.Invalidate("p", "o", "m", "s")
	_, ok := cache.Get("p", "o", "m", "s")
	require.False(t, ok)
	_, ok = cache.Get("p", "o", "m", "s2")
	require.True(t, ok)
}

func TestProofCacheSingleflightAndRaceDoNotLeakOrRetainStaleSuccess(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	cache, err := NewProofCache(ProofConfig{HMACKey: bytes.Repeat([]byte{3}, 32), TTL: 15 * time.Second, Capacity: 256, Now: func() time.Time { return now }})
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("s-%d", i)
			snap := stytch.SessionSnapshot{ProjectID: "p", OrganizationID: "o", MemberID: "m", ProviderMemberSessionID: id, Active: true, Eligible: true, ExpiresAt: now.Add(time.Minute)}
			if !cache.Put(snap) {
				t.Errorf("put %s failed", id)
				return
			}
			got, ok := cache.Get("p", "o", "m", id)
			if !ok || got.ProviderMemberSessionID != id {
				t.Errorf("get %s failed", id)
			}
		}(i)
	}
	wg.Wait()
	cache.Invalidate("p", "o", "m", "s-1")
	_, ok := cache.Get("p", "o", "m", "s-1")
	require.False(t, ok)
	_, ok = cache.Get("p", "o", "m", "s-2")
	require.True(t, ok)
}
