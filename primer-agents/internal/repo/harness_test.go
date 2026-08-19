package repo_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/agents/internal/testutil"
)

// pool returns the shared migrated pool for all repo integration tests.
func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testutil.DB(t)
}
