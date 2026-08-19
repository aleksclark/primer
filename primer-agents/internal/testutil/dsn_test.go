package testutil_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/testutil"
)

func TestTestDatabaseEnvIsAgentsNamespaced(t *testing.T) {
	t.Parallel()
	require.Equal(t, "PRIMER_AGENTS_TEST_DATABASE_URL", testutil.TestDatabaseEnv)
	require.NotEqual(t, "TEST_DATABASE_URL", testutil.TestDatabaseEnv)
	require.NotEqual(t, "DATABASE_URL", testutil.TestDatabaseEnv)
}

func TestResolveTestDatabaseURLIgnoresAmbientBare(t *testing.T) {
	t.Setenv("TEST_DATABASE_URL", "postgres://lms:lms@127.0.0.1:5432/primer_test?sslmode=disable")
	t.Setenv("DATABASE_URL", "postgres://lms:lms@127.0.0.1:5432/primer?sslmode=disable")
	t.Setenv("STUDIO_TEST_DATABASE_URL", "postgres://s:s@127.0.0.1:5432/curriculum_studio_test?sslmode=disable")
	t.Setenv("IDENTITY_TEST_DATABASE_URL", "postgres://id:id@127.0.0.1:5432/primer_identity_test?sslmode=disable")
	require.NoError(t, os.Unsetenv("PRIMER_AGENTS_TEST_DATABASE_URL"))

	url, usedExternal, err := testutil.ResolveTestDatabaseURL()
	require.NoError(t, err)
	require.False(t, usedExternal, "must not inherit any ambient bare database URL")
	require.Empty(t, url)
}

func TestResolveTestDatabaseURLAcceptsAgentsSafeExternal(t *testing.T) {
	safe := "postgres://agents:agents@127.0.0.1:5432/primer_agents_test?sslmode=disable"
	t.Setenv("PRIMER_AGENTS_TEST_DATABASE_URL", safe)
	t.Setenv("DATABASE_URL", "postgres://lms:lms@127.0.0.1:5432/primer_test?sslmode=disable")

	url, usedExternal, err := testutil.ResolveTestDatabaseURL()
	require.NoError(t, err)
	require.True(t, usedExternal)
	require.Equal(t, safe, url)
}

func TestResolveTestDatabaseURLRejectsForbiddenExternal(t *testing.T) {
	cases := []string{
		"postgres://x:x@127.0.0.1:5432/primer?sslmode=disable",
		"postgres://x:x@127.0.0.1:5432/primer_identity?sslmode=disable",
		"postgres://x:x@127.0.0.1:5432/primer_identity_test?sslmode=disable",
		"postgres://x:x@127.0.0.1:5432/primer_tv?sslmode=disable",
		"postgres://x:x@127.0.0.1:5432/curriculum_studio?sslmode=disable",
		"host=127.0.0.1 user=x dbname=primer_test sslmode=disable",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("PRIMER_AGENTS_TEST_DATABASE_URL", dsn)
			_, _, err := testutil.ResolveTestDatabaseURL()
			require.Error(t, err)
			require.True(t,
				strings.Contains(strings.ToLower(err.Error()), "forbidden") ||
					strings.Contains(strings.ToLower(err.Error()), "agents"),
				"err=%v", err)
		})
	}
}

func TestResolveTestDatabaseURLRejectsNonAgentsSafeName(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_TEST_DATABASE_URL", "postgres://x:x@127.0.0.1:5432/mydb?sslmode=disable")
	_, _, err := testutil.ResolveTestDatabaseURL()
	require.Error(t, err)
	require.Contains(t, err.Error(), "agents-safe")
}
