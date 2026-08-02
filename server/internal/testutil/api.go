package testutil

import (
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/repo"
)

// Options tunes the test API.
type Options struct {
	// ServiceToken enables ingest authentication; empty leaves it open.
	ServiceToken string
}

// API returns a humatest API wired to the full route set, plus the
// transaction-backed querier used for factories. Statements run inside
// savepoints so deliberate constraint violations don't abort the test
// transaction. All state is rolled back when the test ends.
func API(t *testing.T, opts ...Options) (humatest.TestAPI, repo.Querier) {
	t.Helper()
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	q := NewSavepointQuerier(Tx(t))
	_, testAPI := humatest.New(t)
	api.RegisterRoutes(testAPI, q, api.Options{ServiceToken: o.ServiceToken})
	return testAPI, q
}
