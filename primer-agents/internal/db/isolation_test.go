package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentsdb "github.com/aleksclark/primer/agents/internal/db"
)

func TestForbiddenDBNamesContainsAllForeignServices(t *testing.T) {
	t.Parallel()
	required := []string{
		"primer", "primer_test",
		"primer_tv", "primer_tv_test", "tv", "tv_test",
		"primer_identity", "primer_identity_test",
		"curriculum_studio", "curriculum_studio_test",
		"studio",
	}
	for _, name := range required {
		assert.True(t, agentsdb.IsForbiddenDBName(name), "expected %q to be forbidden", name)
	}
}

func TestAllowedAgentsDBNamesAreNotForbidden(t *testing.T) {
	t.Parallel()
	for _, name := range agentsdb.AllowedAgentsDBNames {
		assert.False(t, agentsdb.IsForbiddenDBName(name), "allowed name %q must not be forbidden", name)
	}
}

func TestIsForbiddenDBNameCaseInsensitive(t *testing.T) {
	t.Parallel()
	assert.True(t, agentsdb.IsForbiddenDBName("PRIMER"))
	assert.True(t, agentsdb.IsForbiddenDBName("Primer_TV"))
	assert.True(t, agentsdb.IsForbiddenDBName("CURRICULUM_STUDIO"))
	assert.False(t, agentsdb.IsForbiddenDBName("PRIMER_AGENTS"))
	assert.False(t, agentsdb.IsForbiddenDBName("primer_agents_test"))
}

func TestValidateDatabaseURLRejectsForbiddenNames(t *testing.T) {
	t.Parallel()
	cases := []string{
		"postgres://u:x@localhost:5432/primer?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_test?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_tv?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_tv_test?sslmode=disable",
		"postgres://u:x@localhost:5432/tv?sslmode=disable",
		"postgres://u:x@localhost:5432/tv_test?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_identity?sslmode=disable",
		"postgres://u:x@localhost:5432/primer_identity_test?sslmode=disable",
		"postgres://u:x@localhost:5432/curriculum_studio?sslmode=disable",
		"postgres://u:x@localhost:5432/curriculum_studio_test?sslmode=disable",
		"postgres://u:x@localhost:5432/studio?sslmode=disable",
		"host=localhost user=u password=p dbname=primer sslmode=disable",
		"host=localhost user=u dbname=primer_tv sslmode=disable",
		"host=localhost user=u dbname=CURRICULUM_STUDIO",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Parallel()
			err := agentsdb.ValidateDatabaseURL(dsn)
			require.Error(t, err)
			msg := strings.ToLower(err.Error())
			assert.True(t,
				strings.Contains(msg, "forbidden") || strings.Contains(msg, "database"),
				"expected forbidden/database rejection, got %v", err)
		})
	}
}

func TestValidateDatabaseURLAcceptsAgentsNames(t *testing.T) {
	t.Parallel()
	cases := []string{
		"postgres://agents:x@localhost:5432/primer_agents?sslmode=disable",
		"postgresql://agents:x@db:5432/primer_agents",
		"host=localhost user=agents password=p dbname=primer_agents sslmode=disable",
		"postgres://agents:x@localhost:5432/primer_agents_test?sslmode=disable",
		"host=localhost dbname=PRIMER_AGENTS",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Parallel()
			err := agentsdb.ValidateDatabaseURL(dsn)
			require.NoError(t, err)
		})
	}
}

func TestValidateDatabaseURLRejectsEmpty(t *testing.T) {
	t.Parallel()
	require.Error(t, agentsdb.ValidateDatabaseURL(""))
	require.Error(t, agentsdb.ValidateDatabaseURL("   "))
}

func TestValidateDatabaseURLRejectsMissingDBName(t *testing.T) {
	t.Parallel()
	// These DSNs have no database name (empty path or missing dbname keyword).
	// The double-slash form resolves to a real name so is not in this list.
	cases := []string{
		"postgres://u:x@localhost:5432/",
		"host=localhost user=u password=p sslmode=disable",
	}
	for _, dsn := range cases {
		t.Run(dsn, func(t *testing.T) {
			t.Parallel()
			err := agentsdb.ValidateDatabaseURL(dsn)
			require.Error(t, err)
		})
	}
}

func TestIsAgentsSafeTestDBName(t *testing.T) {
	t.Parallel()
	assert.True(t, agentsdb.IsAgentsSafeTestDBName("primer_agents"))
	assert.True(t, agentsdb.IsAgentsSafeTestDBName("primer_agents_test"))
	assert.True(t, agentsdb.IsAgentsSafeTestDBName("PRIMER_AGENTS"))
	assert.False(t, agentsdb.IsAgentsSafeTestDBName("primer"))
	assert.False(t, agentsdb.IsAgentsSafeTestDBName("primer_identity"))
	assert.False(t, agentsdb.IsAgentsSafeTestDBName("curriculum_studio"))
	assert.False(t, agentsdb.IsAgentsSafeTestDBName(""))
}

func TestMigratorCurrentVersion(t *testing.T) {
	t.Parallel()
	// CurrentVersion on an agent-safe DSN against a real DB
	// (not testcontainer — just verify parse/call path with a real URL from
	// the shared harness).
	// Use a stub URL to verify DSN validation fires first.
	_, err := agentsdb.Agents.CurrentVersion(context.Background(),
		"postgres://agents:x@localhost:1/primer_agents?connect_timeout=0")
	require.Error(t, err, "CurrentVersion on unreachable DB must return error")
	// Verify it's a DB error, not a DSN isolation error.
	assert.NotContains(t, strings.ToLower(err.Error()), "forbidden")
}
