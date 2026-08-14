package password_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/password"
)

func TestHashAndVerifyRoundTrip(t *testing.T) {
	t.Parallel()
	phc, err := password.Hash("correct horse battery staple")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(phc, "$argon2id$v=19$"))
	assert.Contains(t, phc, "m=65536,t=3,p=4")
	assert.NotContains(t, phc, "correct horse")

	ok, err := password.Verify(phc, "correct horse battery staple")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = password.Verify(phc, "wrong password")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestVerifyMalformedFailsClosed(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{
		"",
		"bcrypt-not-supported",
		"$argon2i$v=19$m=65536,t=3,p=4$aaaa$bbbb",
		"$argon2id$v=19$m=65536,t=3,p=4",
		"$argon2id$v=19$m=0,t=3,p=4$YQ$YQ",
		"$argon2id$v=99$m=65536,t=3,p=4$YQ$YQ",
	} {
		ok, err := password.Verify(bad, "x")
		assert.False(t, ok, "input %q", bad)
		assert.True(t, errors.Is(err, password.ErrMalformedHash), "input %q: %v", bad, err)
	}
}

func TestHashUsesUniqueSalts(t *testing.T) {
	t.Parallel()
	a, err := password.Hash("same")
	require.NoError(t, err)
	b, err := password.Hash("same")
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

func TestDummyVerifyRuns(t *testing.T) {
	t.Parallel()
	start := time.Now()
	password.DummyVerify()
	// Just ensure it returns; timing is environment-dependent.
	assert.True(t, time.Since(start) >= 0)
}

func TestDefaultParamsDocumented(t *testing.T) {
	t.Parallel()
	p := password.DefaultParams()
	assert.Equal(t, uint32(3), p.Time)
	assert.Equal(t, uint32(64*1024), p.Memory)
	assert.Equal(t, uint8(4), p.Threads)
	assert.Equal(t, uint32(32), p.KeyLen)
	assert.Equal(t, "argon2id", password.AlgorithmArgon2id)
	assert.Equal(t, 1024, password.MaxPasswordBytes)
}

func TestHashRejectsEmptyAndOversizePassword(t *testing.T) {
	t.Parallel()

	_, err := password.Hash("")
	require.Error(t, err)
	assert.True(t, errors.Is(err, password.ErrInvalidPassword), "%v", err)

	giant := strings.Repeat("a", password.MaxPasswordBytes+1)
	start := time.Now()
	_, err = password.Hash(giant)
	elapsed := time.Since(start)
	require.Error(t, err)
	assert.True(t, errors.Is(err, password.ErrInvalidPassword), "%v", err)
	// Must reject before KDF — no Argon2 on giant plaintext.
	assert.Less(t, elapsed, 200*time.Millisecond, "oversize Hash must not run Argon2")

	_, err = password.HashWithParams(giant, password.DefaultParams())
	require.Error(t, err)
	assert.True(t, errors.Is(err, password.ErrInvalidPassword), "%v", err)

	// Boundary: exactly MaxPasswordBytes is accepted (still hashed).
	exact := strings.Repeat("b", password.MaxPasswordBytes)
	phc, err := password.Hash(exact)
	require.NoError(t, err)
	ok, err := password.Verify(phc, exact)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestVerifyRejectsEmptyAndOversizePasswordWithoutKDF(t *testing.T) {
	t.Parallel()

	phc, err := password.Hash("ok-password")
	require.NoError(t, err)

	ok, err := password.Verify(phc, "")
	assert.False(t, ok)
	require.Error(t, err)
	assert.True(t, errors.Is(err, password.ErrInvalidPassword), "%v", err)

	giant := strings.Repeat("x", password.MaxPasswordBytes+1)
	start := time.Now()
	ok, err = password.Verify(phc, giant)
	elapsed := time.Since(start)
	assert.False(t, ok)
	require.Error(t, err)
	assert.True(t, errors.Is(err, password.ErrInvalidPassword), "%v", err)
	assert.Less(t, elapsed, 200*time.Millisecond, "oversize Verify must not run Argon2")
}

func TestHashWithParamsRejectsExcessiveCosts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		p    password.Params
	}{
		{"memory_1gib", password.Params{Time: 3, Memory: 1024 * 1024, Threads: 4, KeyLen: 32}},
		{"memory_256mib", password.Params{Time: 3, Memory: 256 * 1024, Threads: 4, KeyLen: 32}},
		{"time_10", password.Params{Time: 10, Memory: 64 * 1024, Threads: 4, KeyLen: 32}},
		{"threads_16", password.Params{Time: 3, Memory: 64 * 1024, Threads: 16, KeyLen: 32}},
		{"keylen_65", password.Params{Time: 3, Memory: 64 * 1024, Threads: 4, KeyLen: 65}},
		{"keylen_zero", password.Params{Time: 3, Memory: 64 * 1024, Threads: 4, KeyLen: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := password.HashWithParams("password", tc.p)
			require.Error(t, err)
		})
	}

	// Production defaults and modest lower bounds remain allowed.
	_, err := password.HashWithParams("password", password.DefaultParams())
	require.NoError(t, err)
	_, err = password.HashWithParams("password", password.Params{
		Time: 1, Memory: 19 * 1024, Threads: 1, KeyLen: 16,
	})
	require.NoError(t, err)
}

func TestVerifyRejectsAbsurdPHCBeforeArgon2(t *testing.T) {
	t.Parallel()

	// Pre-encoded dummy salt/hash (raw base64 of 16 and 32 zero bytes).
	salt16 := "AAAAAAAAAAAAAAAAAAAAAA"
	hash32 := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	cases := []struct {
		name string
		phc  string
	}{
		{
			name: "memory_1gib",
			phc:  "$argon2id$v=19$m=1048576,t=3,p=4$" + salt16 + "$" + hash32,
		},
		{
			name: "memory_256mib",
			phc:  "$argon2id$v=19$m=262144,t=3,p=4$" + salt16 + "$" + hash32,
		},
		{
			name: "time_too_high",
			phc:  "$argon2id$v=19$m=65536,t=10,p=4$" + salt16 + "$" + hash32,
		},
		{
			name: "threads_too_high",
			phc:  "$argon2id$v=19$m=65536,t=3,p=16$" + salt16 + "$" + hash32,
		},
		{
			name: "keylen_65",
			// 65 zero bytes as raw std base64 (87 chars) — must reject after decode bounds.
			phc: "$argon2id$v=19$m=65536,t=3,p=4$" + salt16 + "$" +
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		},
		{
			name: "salt_10kib",
			// Oversized salt segment (well above 64 bytes) — reject before KDF.
			phc: "$argon2id$v=19$m=65536,t=3,p=4$" + strings.Repeat("A", 14000) + "$" + hash32,
		},
		{
			name: "trailing_garbage",
			phc:  "$argon2id$v=19$m=65536,t=3,p=4$" + salt16 + "$" + hash32 + "$extra",
		},
		{
			name: "trailing_param_garbage",
			phc:  "$argon2id$v=19$m=65536,t=3,p=4,x=1$" + salt16 + "$" + hash32,
		},
		{
			name: "wrong_param_order",
			phc:  "$argon2id$v=19$t=3,m=65536,p=4$" + salt16 + "$" + hash32,
		},
		{
			name: "version_prefix_garbage",
			phc:  "$argon2id$v=19x$m=65536,t=3,p=4$" + salt16 + "$" + hash32,
		},
		{
			name: "salt_too_short",
			phc:  "$argon2id$v=19$m=65536,t=3,p=4$AAAA$" + hash32, // 3 bytes
		},
		{
			name: "key_too_short",
			phc:  "$argon2id$v=19$m=65536,t=3,p=4$" + salt16 + "$AAAA", // 3 bytes
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			ok, err := password.Verify(tc.phc, "password")
			elapsed := time.Since(start)
			assert.False(t, ok, "phc=%s", tc.phc)
			assert.True(t, errors.Is(err, password.ErrMalformedHash), "phc=%s err=%v", tc.phc, err)
			// Bounded-fast: reject without allocating multi-hundred-MiB Argon2.
			assert.Less(t, elapsed, 500*time.Millisecond, "malformed/absurd PHC must reject quickly")
		})
	}
}

func TestVerifyAcceptsCanonicalModernHash(t *testing.T) {
	t.Parallel()
	phc, err := password.HashWithParams("bound-ok", password.Params{
		Time: 2, Memory: 32 * 1024, Threads: 2, KeyLen: 32,
	})
	require.NoError(t, err)
	ok, err := password.Verify(phc, "bound-ok")
	require.NoError(t, err)
	assert.True(t, ok)
}
