package testutil

import (
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/repo"
)

// API returns a humatest API wired to the full route set, plus the
// transaction-backed querier used for factories. Statements run inside
// savepoints so deliberate constraint violations don't abort the test
// transaction. All state is rolled back when the test ends.
func API(t *testing.T) (humatest.TestAPI, repo.Querier) {
	t.Helper()
	q := NewSavepointQuerier(Tx(t))
	_, testAPI := humatest.New(t)
	api.RegisterRoutes(testAPI, q)
	return testAPI, q
}
