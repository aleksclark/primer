package stytchcache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aleksclark/primer/identity/internal/stytch"
)

const (
	testProject = "project-test"
	testToken   = "opaque-token-never-exposed"
)

var testKey = []byte("01234567890123456789012345678901")

type fakeClient struct {
	calls atomic.Int64
	fn    func(context.Context, string) (stytch.SessionSnapshot, error)
}

func (f *fakeClient) AuthenticateSession(ctx context.Context, token string) (stytch.SessionSnapshot, error) {
	f.calls.Add(1)
	return f.fn(ctx, token)
}
func (f *fakeClient) InvalidateSession(context.Context, string) error { return nil }

func validSnapshot(now time.Time) stytch.SessionSnapshot {
	return stytch.SessionSnapshot{
		ProjectID: testProject, OrganizationID: "org", MemberID: "member",
		Active: true, Eligible: true, ExpiresAt: now.Add(time.Hour), Roles: []string{"reader"},
	}
}

func newTestCache(t *testing.T, client stytch.StytchClient, now *time.Time, extra func(*Config)) *Cache {
	t.Helper()
	cfg := Config{
		Client: client, ProjectID: testProject, HMACKey: testKey,
		PositiveTTL: 15 * time.Second, NegativeTTL: 5 * time.Second,
		PositiveCapacity: 10000, NegativeCapacity: 2000,
		Now: func() time.Time { return *now },
	}
	if extra != nil {
		extra(&cfg)
	}
	cache, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func waitForInFlight(t *testing.T, cache *Cache, want uint64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for cache.Stats().InFlightLoads != want {
		if time.Now().After(deadline) {
			t.Fatalf("in-flight loads=%d, want %d; stats=%+v", cache.Stats().InFlightLoads, want, cache.Stats())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPositiveResultIsCached(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, nil)

	first, err := cache.AuthenticateSession(context.Background(), testToken)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.AuthenticateSession(context.Background(), testToken)
	if err != nil {
		t.Fatal(err)
	}
	if first.MemberID != second.MemberID || client.calls.Load() != 1 {
		t.Fatalf("first=%+v second=%+v calls=%d", first, second, client.calls.Load())
	}
	if got := cache.Stats().PositiveHits; got != 1 {
		t.Fatalf("positive hits=%d", got)
	}
}

func TestPositiveExpiryIsCappedByProvider(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	providerExpiry := now.Add(3 * time.Second)
	client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
		s := validSnapshot(now)
		s.ExpiresAt = providerExpiry
		return s, nil
	}}
	cache := newTestCache(t, client, &now, func(c *Config) { c.PositiveTTL = 15 * time.Second })
	if _, err := cache.AuthenticateSession(context.Background(), testToken); err != nil {
		t.Fatal(err)
	}
	now = now.Add(4 * time.Second)
	if _, err := cache.AuthenticateSession(context.Background(), testToken); !errors.Is(err, ErrDenied) {
		t.Fatalf("err=%v", err)
	}
	if client.calls.Load() != 2 {
		t.Fatalf("calls=%d", client.calls.Load())
	}
}

func TestResultsAndRolesAreDefensiveCopies(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
		s := validSnapshot(now)
		return s, nil
	}}
	cache := newTestCache(t, client, &now, nil)
	got, err := cache.AuthenticateSession(context.Background(), testToken)
	if err != nil {
		t.Fatal(err)
	}
	got.Roles[0] = "mutated"
	got.Roles = append(got.Roles, "another")
	again, err := cache.AuthenticateSession(context.Background(), testToken)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(again.Roles, ",") != "reader" {
		t.Fatalf("roles=%v", again.Roles)
	}
}

func TestInvalidateAndInvalidateAllRemoveLocalEntries(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, nil)
	if _, err := cache.AuthenticateSession(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.AuthenticateSession(context.Background(), "two"); err != nil {
		t.Fatal(err)
	}
	cache.Invalidate("one")
	if _, err := cache.AuthenticateSession(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	cache.InvalidateAll()
	if _, err := cache.AuthenticateSession(context.Background(), "two"); err != nil {
		t.Fatal(err)
	}
	if got := client.calls.Load(); got != 4 {
		t.Fatalf("calls=%d", got)
	}
}

func TestPositiveAndNegativeCapacityEvictSeparately(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(_ context.Context, token string) (stytch.SessionSnapshot, error) {
		if strings.HasPrefix(token, "bad") {
			return stytch.SessionSnapshot{}, stytch.ErrDefinitive
		}
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, func(c *Config) { c.PositiveCapacity = 1; c.NegativeCapacity = 1 })
	for _, token := range []string{"good-1", "good-2", "bad-1", "bad-2"} {
		_, _ = cache.AuthenticateSession(context.Background(), token)
	}
	if cache.Stats().PositiveEntries != 1 || cache.Stats().NegativeEntries != 1 {
		t.Fatalf("stats=%+v", cache.Stats())
	}
	if _, _ = cache.AuthenticateSession(context.Background(), "good-1"); client.calls.Load() != 5 {
		t.Fatalf("positive eviction calls=%d", client.calls.Load())
	}
	if _, _ = cache.AuthenticateSession(context.Background(), "bad-1"); client.calls.Load() != 6 {
		t.Fatalf("negative eviction calls=%d", client.calls.Load())
	}
	if cache.Stats().Evictions != 4 {
		t.Fatalf("evictions=%d", cache.Stats().Evictions)
	}
}

func TestThousandConcurrentRequestsSingleflightToOneUpstreamCall(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	client := &fakeClient{fn: func(ctx context.Context, _ string) (stytch.SessionSnapshot, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
			return stytch.SessionSnapshot{}, ctx.Err()
		}
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, nil)
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 1000)
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := cache.AuthenticateSession(context.Background(), testToken)
			errs <- err
		}()
	}
	close(start)
	<-entered
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := client.calls.Load(); got != 1 {
		t.Fatalf("calls=%d", got)
	}
}

func TestDefinitiveErrorIsNegativeCached(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
		return stytch.SessionSnapshot{}, stytch.ErrDefinitive
	}}
	cache := newTestCache(t, client, &now, nil)
	for i := 0; i < 2; i++ {
		if _, err := cache.AuthenticateSession(context.Background(), testToken); !errors.Is(err, ErrDenied) {
			t.Fatalf("err=%v", err)
		}
	}
	if client.calls.Load() != 1 || cache.Stats().NegativeHits != 1 {
		t.Fatalf("calls=%d stats=%+v", client.calls.Load(), cache.Stats())
	}
}

func TestTransientErrorsAreNotCached(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
		return stytch.SessionSnapshot{}, errors.New("upstream failure " + testToken)
	}}
	cache := newTestCache(t, client, &now, nil)
	for i := 0; i < 2; i++ {
		if _, err := cache.AuthenticateSession(context.Background(), testToken); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("err=%v", err)
		}
	}
	if client.calls.Load() != 2 || cache.Stats().TransientErrors != 2 {
		t.Fatalf("calls=%d stats=%+v", client.calls.Load(), cache.Stats())
	}
}

func TestContextCancellationIsNotCached(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(ctx context.Context, _ string) (stytch.SessionSnapshot, error) {
		if err := ctx.Err(); err != nil {
			return stytch.SessionSnapshot{}, err
		}
		return stytch.SessionSnapshot{}, errors.New("upstream canceled")
	}}
	cache := newTestCache(t, client, &now, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.AuthenticateSession(ctx, testToken); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if _, err := cache.AuthenticateSession(context.Background(), testToken); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
	if client.calls.Load() != 1 {
		t.Fatalf("calls=%d", client.calls.Load())
	}
}

func TestInvalidateIsBarrierForBlockedLoad(t *testing.T) {
	runInvalidationBarrierTest(t, func(cache *Cache) { cache.Invalidate(testToken) })
}

func TestInvalidateAllIsBarrierForBlockedLoad(t *testing.T) {
	runInvalidationBarrierTest(t, func(cache *Cache) { cache.InvalidateAll() })
}

func TestPerTokenInvalidateDoesNotFenceUnrelatedBlockedLoad(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	const tokenA = "token-a"
	const tokenB = "token-b"
	release := map[string]chan struct{}{
		tokenA: make(chan struct{}),
		tokenB: make(chan struct{}),
	}
	started := make(chan string, 2)
	client := &fakeClient{fn: func(ctx context.Context, token string) (stytch.SessionSnapshot, error) {
		started <- token
		select {
		case <-release[token]:
			return validSnapshot(now), nil
		case <-ctx.Done():
			return stytch.SessionSnapshot{}, ctx.Err()
		}
	}}
	cache := newTestCache(t, client, &now, nil)

	errA := make(chan error, 1)
	errB := make(chan error, 1)
	go func() {
		_, err := cache.AuthenticateSession(context.Background(), tokenA)
		errA <- err
	}()
	go func() {
		_, err := cache.AuthenticateSession(context.Background(), tokenB)
		errB <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("blocked provider loads did not start")
		}
	}

	cache.Invalidate(tokenA)
	close(release[tokenA])
	close(release[tokenB])

	select {
	case err := <-errA:
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("token A after per-token invalidate: %v, want ErrUnavailable", err)
		}
	case <-time.After(time.Second):
		t.Fatal("token A did not finish")
	}
	select {
	case err := <-errB:
		if err != nil {
			t.Fatalf("token B fenced by Invalidate(%s): %v", tokenA, err)
		}
	case <-time.After(time.Second):
		t.Fatal("token B did not finish")
	}
	if _, err := cache.AuthenticateSession(context.Background(), tokenB); err != nil {
		t.Fatalf("token B cache follow-up: %v", err)
	}
	if got := client.calls.Load(); got != 2 {
		t.Fatalf("unrelated token B was not cached: calls=%d", got)
	}
	waitForInFlight(t, cache, 0)
	if stats := cache.Stats(); stats.InFlightLoads != 0 || stats.PositiveEntries != 1 {
		t.Fatalf("stats after isolated invalidate=%+v", stats)
	}
}

func TestPerTokenInvalidateStartsFreshLoadAndStaleDoesNotInsert(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	oldRelease := make(chan struct{})
	newRelease := make(chan struct{})
	started := make(chan int, 2)
	var calls atomic.Int64
	client := &fakeClient{fn: func(_ context.Context, _ string) (stytch.SessionSnapshot, error) {
		call := int(calls.Add(1))
		started <- call
		if call == 1 {
			<-oldRelease
		} else {
			<-newRelease
		}
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, nil)

	oldDone := make(chan error, 1)
	go func() {
		_, err := cache.AuthenticateSession(context.Background(), testToken)
		oldDone <- err
	}()
	select {
	case call := <-started:
		if call != 1 {
			t.Fatalf("first provider call=%d", call)
		}
	case <-time.After(time.Second):
		t.Fatal("old load did not start")
	}

	cache.Invalidate(testToken)

	newDone := make(chan error, 1)
	go func() {
		_, err := cache.AuthenticateSession(context.Background(), testToken)
		newDone <- err
	}()
	select {
	case call := <-started:
		if call != 2 {
			t.Fatalf("post-invalidation provider call=%d", call)
		}
	case <-time.After(time.Second):
		t.Fatal("post-invalidation request joined the stale load")
	}

	close(oldRelease)
	select {
	case err := <-oldDone:
		if err == nil {
			t.Fatal("stale load returned success")
		}
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("stale load err=%v, want ErrUnavailable", err)
		}
	case <-time.After(time.Second):
		t.Fatal("old load did not finish")
	}
	waitForInFlight(t, cache, 1)
	if stats := cache.Stats(); stats.PositiveEntries != 0 {
		t.Fatalf("stale load inserted after invalidate: stats=%+v", stats)
	}

	close(newRelease)
	select {
	case err := <-newDone:
		if err != nil {
			t.Fatalf("fresh load failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fresh load did not finish")
	}
	if _, err := cache.AuthenticateSession(context.Background(), testToken); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("upstream calls=%d, want 2", got)
	}
	if stats := cache.Stats(); stats.InFlightLoads != 0 || stats.PositiveEntries != 1 {
		t.Fatalf("stats after fresh load=%+v", stats)
	}
}

func TestInvalidateAllFencesAllBlockedLoads(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	const tokenA = "token-a"
	const tokenB = "token-b"
	release := make(chan struct{})
	started := make(chan string, 2)
	client := &fakeClient{fn: func(ctx context.Context, token string) (stytch.SessionSnapshot, error) {
		started <- token
		select {
		case <-release:
			return validSnapshot(now), nil
		case <-ctx.Done():
			return stytch.SessionSnapshot{}, ctx.Err()
		}
	}}
	cache := newTestCache(t, client, &now, nil)

	errA := make(chan error, 1)
	errB := make(chan error, 1)
	go func() {
		_, err := cache.AuthenticateSession(context.Background(), tokenA)
		errA <- err
	}()
	go func() {
		_, err := cache.AuthenticateSession(context.Background(), tokenB)
		errB <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("blocked provider loads did not start")
		}
	}

	cache.InvalidateAll()
	close(release)
	for _, got := range []struct {
		name string
		ch   <-chan error
	}{{"A", errA}, {"B", errB}} {
		select {
		case err := <-got.ch:
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("token %s after InvalidateAll: %v, want ErrUnavailable", got.name, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("token %s did not finish", got.name)
		}
	}
	waitForInFlight(t, cache, 0)
	if stats := cache.Stats(); stats.PositiveEntries != 0 || stats.InFlightLoads != 0 {
		t.Fatalf("stats after InvalidateAll=%+v", stats)
	}
}

func TestInvalidateCachedTokenLeavesOtherCacheHit(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, nil)
	if _, err := cache.AuthenticateSession(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.AuthenticateSession(context.Background(), "two"); err != nil {
		t.Fatal(err)
	}
	cache.Invalidate("one")
	if _, err := cache.AuthenticateSession(context.Background(), "two"); err != nil {
		t.Fatal(err)
	}
	if got := client.calls.Load(); got != 2 {
		t.Fatalf("cached token B reloaded after Invalidate(A): calls=%d", got)
	}
	if _, err := cache.AuthenticateSession(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if got := client.calls.Load(); got != 3 {
		t.Fatalf("invalidated token A was not reloaded: calls=%d", got)
	}
	if stats := cache.Stats(); stats.PositiveHits != 1 || stats.InFlightLoads != 0 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestMixedTokenInvalidationRace(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	const tokens = 32
	release := make(chan struct{})
	var started atomic.Int64
	entered := make(chan struct{}, tokens)
	client := &fakeClient{fn: func(ctx context.Context, token string) (stytch.SessionSnapshot, error) {
		if n := started.Add(1); n <= tokens {
			entered <- struct{}{}
		}
		select {
		case <-release:
			s := validSnapshot(now)
			s.MemberID = "member-" + token
			return s, nil
		case <-ctx.Done():
			return stytch.SessionSnapshot{}, ctx.Err()
		}
	}}
	cache := newTestCache(t, client, &now, func(c *Config) { c.MaxConcurrentLoads = 64 })

	blockedErrs := make([]chan error, tokens)
	for i := 0; i < tokens; i++ {
		blockedErrs[i] = make(chan error, 1)
		go func(i int) {
			_, err := cache.AuthenticateSession(context.Background(), fmt.Sprintf("race-%d", i))
			blockedErrs[i] <- err
		}(i)
	}
	for i := 0; i < tokens; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("blocked mixed-token loads did not start")
		}
	}

	for i := 0; i < tokens; i += 2 {
		cache.Invalidate(fmt.Sprintf("race-%d", i))
	}
	close(release)
	for i := 0; i < tokens; i++ {
		select {
		case err := <-blockedErrs[i]:
			if i%2 == 0 {
				if !errors.Is(err, ErrUnavailable) {
					t.Fatalf("even token %d err=%v, want ErrUnavailable", i, err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("odd token %d fenced by unrelated invalidates: %v", i, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("token %d did not finish", i)
		}
	}

	var wg sync.WaitGroup
	const mixed = 128
	start := make(chan struct{})
	for i := 0; i < mixed; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			token := fmt.Sprintf("race-%d", i%tokens)
			switch i % 5 {
			case 0:
				cache.Invalidate(token)
			case 1:
				if i%20 == 1 {
					cache.InvalidateAll()
				} else {
					cache.Invalidate(fmt.Sprintf("race-%d", (i+1)%tokens))
				}
			default:
				_, _ = cache.AuthenticateSession(context.Background(), token)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	waitForInFlight(t, cache, 0)
	if stats := cache.Stats(); stats.InFlightLoads != 0 {
		t.Fatalf("final stats=%+v", stats)
	}
}

func runInvalidationBarrierTest(t *testing.T, invalidate func(*Cache)) {
	t.Helper()
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	oldRelease := make(chan struct{})
	newRelease := make(chan struct{})
	started := make(chan int, 2)
	var calls atomic.Int64
	client := &fakeClient{fn: func(_ context.Context, _ string) (stytch.SessionSnapshot, error) {
		call := int(calls.Add(1))
		started <- call
		if call == 1 {
			<-oldRelease
		} else {
			<-newRelease
		}
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, nil)

	oldDone := make(chan error, 1)
	go func() {
		_, err := cache.AuthenticateSession(context.Background(), testToken)
		oldDone <- err
	}()
	select {
	case call := <-started:
		if call != 1 {
			t.Fatalf("first provider call=%d", call)
		}
	case <-time.After(time.Second):
		t.Fatal("old load did not start")
	}

	invalidate(cache)

	newDone := make(chan error, 1)
	go func() {
		_, err := cache.AuthenticateSession(context.Background(), testToken)
		newDone <- err
	}()
	select {
	case call := <-started:
		if call != 2 {
			t.Fatalf("post-invalidation provider call=%d", call)
		}
	case <-time.After(time.Second):
		t.Fatal("post-invalidation request joined the blocked old load")
	}

	close(oldRelease)
	select {
	case err := <-oldDone:
		if err == nil {
			t.Fatal("pre-invalidation load returned stale success")
		}
	case <-time.After(time.Second):
		t.Fatal("old load did not finish")
	}
	close(newRelease)
	select {
	case err := <-newDone:
		if err != nil {
			t.Fatalf("post-invalidation load failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("post-invalidation load did not finish")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("upstream calls=%d, want 2", got)
	}
}

func TestSharedLoadUsesIndependentProviderContext(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	entered := make(chan struct{})
	release := make(chan struct{})
	client := &fakeClient{fn: func(_ context.Context, _ string) (stytch.SessionSnapshot, error) {
		close(entered)
		<-release
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, nil)

	starterCtx, cancelStarter := context.WithCancel(context.Background())
	starterDone := make(chan error, 1)
	go func() {
		_, err := cache.AuthenticateSession(starterCtx, testToken)
		starterDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("shared provider load did not start")
	}

	waiterDone := make(chan error, 1)
	go func() {
		_, err := cache.AuthenticateSession(context.Background(), testToken)
		waiterDone <- err
	}()
	cancelStarter()
	select {
	case err := <-starterDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("starter error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("starter did not honor its context")
	}

	close(release)
	select {
	case err := <-waiterDone:
		if err != nil {
			t.Fatalf("remaining waiter failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("remaining waiter did not receive shared result")
	}
	if got := client.calls.Load(); got != 1 {
		t.Fatalf("upstream calls=%d, want 1", got)
	}
}

func TestDistinctLoadsRejectImmediatelyAtConcurrentLoadLimit(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	const (
		maxLoads = 2
		requests = 6
	)
	release := make(chan struct{})
	started := make(chan struct{}, maxLoads)
	var active atomic.Int64
	var maxActive atomic.Int64
	client := &fakeClient{fn: func(ctx context.Context, _ string) (stytch.SessionSnapshot, error) {
		current := active.Add(1)
		for {
			old := maxActive.Load()
			if current <= old || maxActive.CompareAndSwap(old, current) {
				break
			}
		}
		started <- struct{}{}
		defer active.Add(-1)
		select {
		case <-release:
			return validSnapshot(now), nil
		case <-ctx.Done():
			return stytch.SessionSnapshot{}, ctx.Err()
		}
	}}
	cache := newTestCache(t, client, &now, func(c *Config) { c.MaxConcurrentLoads = maxLoads })

	start := make(chan struct{})
	results := make(chan error, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := cache.AuthenticateSession(context.Background(), fmt.Sprintf("distinct-%d", i))
			results <- err
		}(i)
	}
	close(start)
	for i := 0; i < maxLoads; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("admitted provider load did not start")
		}
	}

	var rejected int
	for i := 0; i < requests-maxLoads; i++ {
		select {
		case err := <-results:
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("excess request error=%v, want ErrUnavailable", err)
			}
			rejected++
		case <-time.After(time.Second):
			t.Fatal("excess request was not rejected immediately")
		}
	}
	if rejected != requests-maxLoads || client.calls.Load() != maxLoads {
		t.Fatalf("rejected=%d calls=%d, want rejected=%d calls=%d", rejected, client.calls.Load(), requests-maxLoads, maxLoads)
	}
	if got := cache.Stats().InFlightLoads; got != maxLoads {
		t.Fatalf("in-flight=%d, want %d", got, maxLoads)
	}
	if got := cache.Stats().LoadOverloadRejections; got != requests-maxLoads {
		t.Fatalf("overload rejections=%d, want %d", got, requests-maxLoads)
	}
	if got := maxActive.Load(); got > maxLoads {
		t.Fatalf("max provider concurrency=%d, want <=%d", got, maxLoads)
	}

	close(release)
	wg.Wait()
	close(results)
	var successes int
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("admitted request error=%v", err)
		}
	}
	if successes != maxLoads || cache.Stats().InFlightLoads != 0 {
		t.Fatalf("successes=%d final stats=%+v", successes, cache.Stats())
	}
}

func TestOversizeAndUnboundedSnapshotsAreNotCached(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*stytch.SessionSnapshot)
	}{
		{"oversize payload", func(s *stytch.SessionSnapshot) {
			s.Roles = make([]string, stytch.MaxSessionRoles)
			for i := range s.Roles {
				s.Roles[i] = strings.Repeat("r", maxSnapshotRoleLength)
			}
		}},
		{"too many roles", func(s *stytch.SessionSnapshot) {
			s.Roles = make([]string, stytch.MaxSessionRoles+1)
		}},
		{"oversize member id", func(s *stytch.SessionSnapshot) {
			s.MemberID = strings.Repeat("m", maxSnapshotIDLength+1)
		}},
		{"control character in organization id", func(s *stytch.SessionSnapshot) {
			s.OrganizationID = "org\nadmin"
		}},
		{"invalid utf-8 role", func(s *stytch.SessionSnapshot) {
			s.Roles = []string{string([]byte{0xff})}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
			client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
				s := validSnapshot(now)
				tc.mutate(&s)
				return s, nil
			}}
			cache := newTestCache(t, client, &now, nil)
			for i := 0; i < 2; i++ {
				if _, err := cache.AuthenticateSession(context.Background(), testToken); !errors.Is(err, ErrUnavailable) {
					t.Fatalf("err=%v", err)
				}
			}
			if got := client.calls.Load(); got != 2 {
				t.Fatalf("unsafe snapshot was cached, calls=%d", got)
			}
		})
	}
}

func TestPositivePayloadBudgetEvictsBeforeInsert(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(_ context.Context, token string) (stytch.SessionSnapshot, error) {
		s := validSnapshot(now)
		s.Roles = []string{strings.Repeat("r", 100)}
		return s, nil
	}}
	probe := validSnapshot(now)
	probe.Roles = []string{strings.Repeat("a", 100)}
	budget := snapshotPayloadSize(probe) * 2
	cache := newTestCache(t, client, &now, func(c *Config) {
		c.PositivePayloadBudget = budget
	})

	for _, token := range []string{"one", "two", "three"} {
		if _, err := cache.AuthenticateSession(context.Background(), token); err != nil {
			t.Fatalf("token %q: %v", token, err)
		}
	}
	stats := cache.Stats()
	if stats.PositiveEntries != 2 {
		t.Fatalf("positive entries=%d, want 2", stats.PositiveEntries)
	}
	if stats.PositiveBytes > uint64(budget) {
		t.Fatalf("positive bytes=%d, budget=%d", stats.PositiveBytes, budget)
	}
	if stats.Evictions != 1 {
		t.Fatalf("evictions=%d, want 1", stats.Evictions)
	}
}

func TestFullCacheRandomMissDoesBoundedMaintenance(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	const capacity = 128
	var evictions atomic.Uint64
	store := newLRU(capacity, false, 0, &evictions)
	for i := 0; i < capacity; i++ {
		if !store.put(fmt.Sprintf("expired-%d", i), now.Add(-time.Second), stytch.SessionSnapshot{}, 0, now) {
			t.Fatalf("put expired entry %d failed", i)
		}
	}
	store.maintenanceSteps.Store(0)
	if _, ok := store.get("random-miss", now); ok {
		t.Fatal("random miss unexpectedly hit")
	}
	if got := store.maintenanceSteps.Load(); got != 1 {
		t.Fatalf("maintenance steps=%d, want one requested-entry check", got)
	}
	if got := store.entryCount.Load(); got != capacity {
		t.Fatalf("random miss scanned/removed entries: count=%d, want %d", got, capacity)
	}
}

func TestInvalidSnapshotsAreNegativeCached(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*stytch.SessionSnapshot)
	}{
		{"inactive", func(s *stytch.SessionSnapshot) { s.Active = false }},
		{"ineligible", func(s *stytch.SessionSnapshot) { s.Eligible = false }},
		{"wrong project", func(s *stytch.SessionSnapshot) { s.ProjectID = "other" }},
		{"missing organization", func(s *stytch.SessionSnapshot) { s.OrganizationID = "" }},
		{"missing member", func(s *stytch.SessionSnapshot) { s.MemberID = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
			client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
				s := validSnapshot(now)
				tc.mutate(&s)
				return s, nil
			}}
			cache := newTestCache(t, client, &now, nil)
			for i := 0; i < 2; i++ {
				if _, err := cache.AuthenticateSession(context.Background(), testToken); !errors.Is(err, ErrDenied) {
					t.Fatalf("err=%v", err)
				}
			}
			if client.calls.Load() != 1 {
				t.Fatalf("calls=%d", client.calls.Load())
			}
		})
	}
}

func TestNoSensitiveValuesInErrorsStatsOrFormattedCache(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(context.Context, string) (stytch.SessionSnapshot, error) {
		return stytch.SessionSnapshot{}, errors.New("bad " + testToken)
	}}
	cache := newTestCache(t, client, &now, nil)
	_, err := cache.AuthenticateSession(context.Background(), testToken)
	digest := cache.digest(testToken)
	for _, output := range []string{err.Error(), fmt.Sprintf("%+v", cache.Stats()), fmt.Sprintf("%+v", cache), fmt.Sprintf("%#v", cache), cache.String()} {
		if strings.Contains(output, testToken) || strings.Contains(output, digest) {
			t.Fatalf("sensitive output: %q", output)
		}
	}
}

func TestSessionTokenBoundsAreCheckedBeforeAdmission(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	var seen string
	client := &fakeClient{fn: func(_ context.Context, token string) (stytch.SessionSnapshot, error) {
		seen = token
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, nil)
	tooLong := strings.Repeat("x", maxSessionTokenBytes+1)

	for _, token := range []string{"", tooLong} {
		if _, err := cache.AuthenticateSession(context.Background(), token); !errors.Is(err, ErrDenied) {
			t.Fatalf("token length %d: err=%v, want ErrDenied", len(token), err)
		}
	}
	if got := client.calls.Load(); got != 0 {
		t.Fatalf("invalid tokens reached provider: calls=%d", got)
	}
	if seen != "" {
		t.Fatalf("provider observed rejected token %q", seen)
	}
	if stats := cache.Stats(); stats.Misses != 0 || stats.Loads != 0 {
		t.Fatalf("rejected tokens entered cache admission: stats=%+v", stats)
	}
}

func TestSessionTokenByteLimitAcceptsBoundaryAndRejectsNextByte(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	client := &fakeClient{fn: func(_ context.Context, _ string) (stytch.SessionSnapshot, error) {
		return validSnapshot(now), nil
	}}
	cache := newTestCache(t, client, &now, nil)
	boundary := strings.Repeat("é", maxSessionTokenBytes/2) // exactly 4096 UTF-8 bytes
	if len(boundary) != maxSessionTokenBytes {
		t.Fatalf("boundary bytes=%d, want %d", len(boundary), maxSessionTokenBytes)
	}
	if _, err := cache.AuthenticateSession(context.Background(), boundary); err != nil {
		t.Fatalf("boundary token rejected: %v", err)
	}
	if _, err := cache.AuthenticateSession(context.Background(), boundary+"x"); !errors.Is(err, ErrDenied) {
		t.Fatalf("over-boundary token err=%v, want ErrDenied", err)
	}
	if got := client.calls.Load(); got != 1 {
		t.Fatalf("provider calls=%d, want 1", got)
	}
}

func TestConstructorValidation(t *testing.T) {
	valid := Config{Client: &fakeClient{}, ProjectID: testProject, HMACKey: testKey}
	cases := []struct {
		name string
		cfg  Config
	}{
		{"nil client", Config{ProjectID: testProject, HMACKey: testKey}},
		{"empty project", Config{Client: valid.Client, HMACKey: testKey}},
		{"short key", Config{Client: valid.Client, ProjectID: testProject, HMACKey: []byte("short")}},
		{"negative positive ttl", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, PositiveTTL: -time.Second}},
		{"positive ttl over hard maximum", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, PositiveTTL: 15*time.Second + time.Nanosecond}},
		{"negative negative ttl", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, NegativeTTL: -time.Second}},
		{"negative ttl over hard maximum", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, NegativeTTL: 5*time.Second + time.Nanosecond}},
		{"negative capacity", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, PositiveCapacity: -1}},
		{"positive capacity over hard maximum", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, PositiveCapacity: 10001}},
		{"negative capacity over hard maximum", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, NegativeCapacity: 2001}},
		{"load timeout must be positive", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, LoadTimeout: -time.Nanosecond}},
		{"load timeout over hard maximum", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, LoadTimeout: 3*time.Second + time.Nanosecond}},
		{"payload budget over hard maximum", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, PositivePayloadBudget: maxPositivePayloadBytes + 1}},
		{"concurrent loads over hard maximum", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, MaxConcurrentLoads: maxMaxConcurrentLoads + 1}},
		{"concurrent loads must be positive", Config{Client: valid.Client, ProjectID: testProject, HMACKey: testKey, MaxConcurrentLoads: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("expected constructor error")
			}
		})
	}
	if _, err := New(valid); err != nil {
		t.Fatal(err)
	}
	defaultCache, err := New(valid)
	if err != nil {
		t.Fatal(err)
	}
	if defaultCache.maxConcurrentLoads != defaultMaxConcurrentLoads {
		t.Fatalf("default max concurrent loads=%d, want %d", defaultCache.maxConcurrentLoads, defaultMaxConcurrentLoads)
	}
}
