package testutil

import (
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/artifacts"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tutor"
)

// Options tunes the test API.
type Options struct {
	// ServiceToken enables ingest authentication; empty leaves it open.
	ServiceToken string
	// Tutor overrides the default fake policy stack when non-nil.
	Tutor tutor.Service
	// TutorProviderName is exposed on parent diagnostics.
	TutorProviderName string
	// TutorEnabled gates tutoring when Tutor is set.
	TutorEnabled *bool
	// ArtifactStoreDir configures filesystem artifact storage for tests.
	// When empty, a temporary directory is used automatically.
	ArtifactStoreDir string
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
	apiOpts := api.Options{ServiceToken: o.ServiceToken}
	if o.Tutor != nil {
		apiOpts.Tutor = o.Tutor
		apiOpts.TutorProviderName = o.TutorProviderName
		if o.TutorEnabled != nil {
			apiOpts.TutorEnabled = *o.TutorEnabled
		} else {
			apiOpts.TutorEnabled = true
		}
	}
	dir := o.ArtifactStoreDir
	if dir == "" {
		dir = t.TempDir()
	}
	if store, err := artifacts.NewStore(dir); err == nil {
		apiOpts.ArtifactStore = store
	}
	api.RegisterRoutes(testAPI, q, apiOpts)
	return testAPI, q
}
