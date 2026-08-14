package testutil_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
)

func TestResolveTestDatabaseURLIgnoresAmbientBareTEST(t *testing.T) {
	// Adversarial: LMS/root TEST_DATABASE_URL must never be inherited.
	t.Setenv("TEST_DATABASE_URL", "postgres://lms:lms@127.0.0.1:5432/primer_test?sslmode=disable")
	t.Setenv("DATABASE_URL", "postgres://lms:lms@127.0.0.1:5432/primer?sslmode=disable")
	t.Setenv("IDENTITY_TEST_DATABASE_URL", "postgres://id:id@127.0.0.1:5432/primer_identity_test?sslmode=disable")
	require.NoError(t, os.Unsetenv("STUDIO_TEST_DATABASE_URL"))

	url, usedExternal, err := testutil.ResolveTestDatabaseURL()
	require.NoError(t, err)
	require.False(t, usedExternal, "must not inherit ambient bare TEST_DATABASE_URL")
	require.Empty(t, url, "unset STUDIO_TEST_DATABASE_URL means testcontainer path")
}

func TestResolveTestDatabaseURLAcceptsStudioSafeExternal(t *testing.T) {
	safe := "postgres://studio:studio@127.0.0.1:5432/curriculum_studio_test?sslmode=disable"
	t.Setenv("STUDIO_TEST_DATABASE_URL", safe)
	// Poison ambient vars — still must use Studio-namespaced only.
	t.Setenv("TEST_DATABASE_URL", "postgres://lms:lms@127.0.0.1:5432/primer_test?sslmode=disable")

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
		"host=127.0.0.1 user=x dbname=primer_test sslmode=disable",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("STUDIO_TEST_DATABASE_URL", dsn)
			_, _, err := testutil.ResolveTestDatabaseURL()
			require.Error(t, err)
			require.True(t,
				strings.Contains(strings.ToLower(err.Error()), "forbidden") ||
					strings.Contains(strings.ToLower(err.Error()), "studio"),
				"err=%v", err)
		})
	}
}

func TestTestDatabaseEnvNameIsStudioNamespaced(t *testing.T) {
	t.Parallel()
	require.Equal(t, "STUDIO_TEST_DATABASE_URL", testutil.TestDatabaseEnv)
	require.NotEqual(t, "TEST_DATABASE_URL", testutil.TestDatabaseEnv)
}
