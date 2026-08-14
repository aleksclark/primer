package testutil_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/testutil"
)

func TestResolveTestDatabaseURLIgnoresAmbientBareTest(t *testing.T) {
	// Adversarial: LMS/root bare TEST_DATABASE_URL / DATABASE_URL must never be inherited.
	t.Setenv("TEST_DATABASE_URL", "postgres://lms:p@127.0.0.1:5432/primer_test?sslmode=disable")
	t.Setenv("DATABASE_URL", "postgres://lms:p@127.0.0.1:5432/primer?sslmode=disable")
	t.Setenv("STUDIO_TEST_DATABASE_URL", "postgres://studio:p@127.0.0.1:5432/curriculum_studio_test?sslmode=disable")
	require.NoError(t, os.Unsetenv("IDENTITY_TEST_DATABASE_URL"))

	url, usedExternal, err := testutil.ResolveTestDatabaseURL()
	require.NoError(t, err)
	require.False(t, usedExternal, "must not inherit ambient bare TEST_DATABASE_URL")
	require.Empty(t, url, "unset IDENTITY_TEST_DATABASE_URL means testcontainer path")
}

func TestResolveTestDatabaseURLAcceptsIdentitySafeExternal(t *testing.T) {
	safe := "postgres://identity:p@127.0.0.1:5432/primer_identity_test?sslmode=disable"
	t.Setenv("IDENTITY_TEST_DATABASE_URL", safe)
	t.Setenv("TEST_DATABASE_URL", "postgres://lms:p@127.0.0.1:5432/primer_test?sslmode=disable")

	url, usedExternal, err := testutil.ResolveTestDatabaseURL()
	require.NoError(t, err)
	require.True(t, usedExternal)
	require.Equal(t, safe, url)
}

func TestResolveTestDatabaseURLRejectsForbiddenExternal(t *testing.T) {
	cases := []string{
		"postgres://x:p@127.0.0.1:5432/primer?sslmode=disable",
		"postgres://x:p@127.0.0.1:5432/curriculum_studio?sslmode=disable",
		"postgres://x:p@127.0.0.1:5432/curriculum_studio_test?sslmode=disable",
		"postgres://x:p@127.0.0.1:5432/primer_tv?sslmode=disable",
		"postgres://x:p@127.0.0.1:5432/primer_tv_test?sslmode=disable",
		"postgres://x:p@127.0.0.1:5432/tv_test?sslmode=disable",
		"host=127.0.0.1 user=x dbname=primer_test sslmode=disable",
		// Non-forbidden but not Identity-safe for the harness override path.
		"postgres://x:p@127.0.0.1:5432/identity_scratch_42?sslmode=disable",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("IDENTITY_TEST_DATABASE_URL", dsn)
			_, _, err := testutil.ResolveTestDatabaseURL()
			require.Error(t, err)
			msg := strings.ToLower(err.Error())
			require.True(t,
				strings.Contains(msg, "forbidden") ||
					strings.Contains(msg, "identity") ||
					strings.Contains(msg, "safe"),
				"err=%v", err)
		})
	}
}

func TestTestDatabaseEnvNameIsIdentityNamespaced(t *testing.T) {
	t.Parallel()
	require.Equal(t, "IDENTITY_TEST_DATABASE_URL", testutil.TestDatabaseEnv)
	require.NotEqual(t, "TEST_DATABASE_URL", testutil.TestDatabaseEnv)
}
