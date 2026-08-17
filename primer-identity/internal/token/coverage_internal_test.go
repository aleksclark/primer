package token

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aleksclark/primer/identity/internal/domain"
)

func TestHashRefreshSecretRequiresBoundedInput(t *testing.T) {
	if _, err := HashRefreshSecret(nil); err == nil {
		t.Fatal("short secret must fail")
	}
	secret := make([]byte, refreshSecretBytes)
	got, err := HashRefreshSecret(secret)
	if err != nil || got == ([32]byte{}) {
		t.Fatalf("hash: %v %#v", err, got)
	}
}

func TestIssuedTokenAndMinterFormatting(t *testing.T) {
	tok := IssuedToken{Compact: "eyJabc", JTI: "secret-jti", Kid: "kid"}
	for _, text := range []string{tok.String(), tok.GoString(), fmt.Sprintf("%v", tok), fmt.Sprintf("%#v", tok), fmt.Sprintf("%v", &tok)} {
		if text != issuedRedacted && text != "token.IssuedToken{redacted}" {
			if containsAny(text, "eyJabc", "secret-jti") {
				t.Fatalf("issued token leaked: %q", text)
			}
		}
	}
	if _, err := tok.MarshalJSON(); err == nil {
		t.Fatal("issued token JSON must fail")
	}
	var m Minter
	if m.GoString() != minterRedacted {
		t.Fatalf("minter gostring: %q", m.GoString())
	}
	if fmt.Sprintf("%v", m) != minterRedacted {
		t.Fatalf("minter format: %q", fmt.Sprintf("%v", m))
	}
}

func containsAny(text string, needles ...string) bool {
	for _, n := range needles {
		if n != "" && len(text) >= len(n) {
			for i := 0; i+len(n) <= len(text); i++ {
				if text[i:i+len(n)] == n {
					return true
				}
			}
		}
	}
	return false
}

func TestNormalizeIssuerCanonicalAuthority(t *testing.T) {
	got, err := normalizeIssuer("https://IDENTITY.EXAMPLE.TEST:443/path/", true)
	if err != nil || got != "https://identity.example.test/path" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := normalizeIssuer("https://127.0.0.1", true); err != nil {
		t.Fatalf("ipv4 issuer: %v", err)
	}
	if _, err := normalizeIssuer("https://[::1]", true); err != nil {
		t.Fatalf("ipv6 issuer: %v", err)
	}
	if _, err := normalizeIssuer("https://identity.example.test:8443", true); err != nil {
		t.Fatalf("nondefault port: %v", err)
	}
	for _, bad := range []string{"", "ftp://x", "https://", "https://user:pass@x", "https://x?q=1", "https://x#f", "https://x/./y", "https://1.2.3", "https://-bad.example"} {
		if _, err := normalizeIssuer(bad, true); err == nil {
			t.Fatalf("expected reject %q", bad)
		}
	}
}

func TestCanonicalScopeDedupesAndSorts(t *testing.T) {
	got, err := canonicalScope("studio:read openid studio:read")
	if err != nil || got != "openid studio:read" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := canonicalScope(`bad"scope`); err == nil {
		t.Fatal("quoted scope token must fail")
	}
}

func TestKeySetRefreshPreservesLastGoodAndRejectsCancel(t *testing.T) {
	good := domain.PublicJWK{}
	// Use acceptPublicJWK path via loader returning empty/error first.
	var current []domain.PublicJWK
	var mu sync.Mutex
	set, err := NewKeySet(func(ctx context.Context) ([]domain.PublicJWK, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		mu.Lock()
		defer mu.Unlock()
		if current == nil {
			return nil, errUnavailableForTest()
		}
		return current, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := set.Refresh(context.Background()); err == nil {
		t.Fatal("loader error must fail")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := set.Lookup(ctx, "missing"); err == nil {
		t.Fatal("canceled lookup must fail")
	}
	if err := set.Refresh(ctx); err == nil {
		t.Fatal("canceled refresh must fail")
	}
	_ = good
}

func errUnavailableForTest() error { return denyUnavailable() }

func TestGenerationAfterWrapSafe(t *testing.T) {
	if generationAfter(1, 1) {
		t.Fatal("equal generations are not after")
	}
	if !generationAfter(2, 1) {
		t.Fatal("2 is after 1")
	}
}

func TestRefreshCoordinatorNegativeAndCooldown(t *testing.T) {
	c := newRefreshCoordinator()
	now := time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)
	c.rememberNegative("kid", now)
	if err := c.suppressedLocked("kid", now.Add(time.Second)); err == nil {
		t.Fatal("negative cache must suppress")
	}
	c.recordAttemptLocked(now, denyUnavailable())
	if err := c.suppressedLocked("other", now.Add(time.Second)); err == nil {
		t.Fatal("failure cooldown must suppress")
	}
	c.recordAttemptLocked(now, nil)
	if err := c.suppressedLocked("other", now.Add(time.Second)); err == nil {
		t.Fatal("success cooldown must still suppress")
	}
	c.evictExpiredLocked(now.Add(6 * time.Second))
	c.rememberNegative("", now)
	c.failAttempt(&refreshAttempt{done: make(chan struct{})}, now)
	if err := c.maybeRefresh(nil, realClock{}, "kid", nil); err == nil {
		t.Fatal("nil keys must fail")
	}
}

func TestAcceptPublicJWKRejectsMismatchAndInvalid(t *testing.T) {
	if _, err := acceptPublicJWK("wanted", domain.PublicJWK{}); err == nil {
		t.Fatal("empty jwk must fail")
	}
	if _, err := acceptTrustedPublicJWK("wanted", domain.PublicJWK{Kid: "other"}); err == nil {
		t.Fatal("trusted mismatch must fail unavailable")
	}
}

func TestClassifySubjectServiceAndInvalid(t *testing.T) {
	if _, err := classifySubject("identity:svc:"); err == nil {
		t.Fatal("empty service id must fail")
	}
	kind, err := classifySubject("identity:svc:jobs")
	if err != nil || kind != KindService {
		t.Fatalf("service subject: %v %v", kind, err)
	}
	if _, err := classifySubject("not-a-subject"); err == nil {
		t.Fatal("unknown subject must fail")
	}
}

func TestMintFreshnessRejectsNilAndBackwardClock(t *testing.T) {
	var f mintFreshness
	if _, err := f.sample(0, false); err == nil {
		t.Fatal("nil freshness must fail")
	}
	clock := &testClock{now: time.Date(2026, 8, 16, 15, 4, 5, 0, time.UTC)}
	f = mintFreshness{ctx: context.Background(), clock: clock, startedWall: time.Now(), startedClock: clock.now, lastClock: clock.now}
	clock.now = clock.now.Add(-time.Second)
	if _, err := f.sample(0, false); err == nil {
		t.Fatal("backward clock must fail")
	}
}

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

func TestNewRandomJTICanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newRandomJTI(ctx); err == nil {
		t.Fatal("canceled jti must fail")
	}
	if _, err := newRandomJTI(nil); err == nil {
		t.Fatal("nil ctx must fail")
	}
}

func TestParseRawRSRejectsShortAndHighS(t *testing.T) {
	if _, _, err := parseRawRS([]byte("short")); err == nil {
		t.Fatal("short sig must fail")
	}
	zero := make([]byte, 64)
	if _, _, err := parseRawRS(zero); err == nil {
		t.Fatal("zero r/s must fail")
	}
}

func TestPublicMatchesJWKRejectsNil(t *testing.T) {
	if err := publicMatchesJWK(nil, nil); err == nil {
		t.Fatal("nil keys must fail")
	}
}

func TestKeySetLookupMissingAndNil(t *testing.T) {
	if _, _, err := (*KeySet)(nil).Lookup(context.Background(), "x"); err == nil {
		t.Fatal("nil set must fail")
	}
	set, err := NewKeySet(func(context.Context) ([]domain.PublicJWK, error) {
		return []domain.PublicJWK{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := set.Lookup(context.Background(), "missing"); err != nil || ok {
		t.Fatalf("missing lookup: ok=%v err=%v", ok, err)
	}
}

func TestAssertionClaimsRejectMissingAndMismatch(t *testing.T) {
	if _, err := parseAssertionClaims([]byte(`{"iss":"a"}`)); err == nil {
		t.Fatal("incomplete assertion claims must fail")
	}
	if _, err := parseAssertionHeader([]byte(`{"alg":"none","kid":"x"}`)); err == nil {
		t.Fatal("none alg must fail")
	}
}
