// Package build holds the mechanics shared by every FactoryBot-style factory:
// sequence numbers for unique values, override merging, and insertion through
// the generic repo layer. Service-specific factories (LMS, TV) supply only
// their defaults and associations.
package build

import (
	"context"
	"maps"
	"sync/atomic"
	"testing"

	"github.com/aleksclark/primer/server/internal/repo"
)

var seq atomic.Int64

// N returns a process-unique sequence number for generating distinct values.
func N() int64 { return seq.Add(1) }

// Override is a column->value map merged over factory defaults.
type Override map[string]any

// Merge layers overrides over a factory's defaults, left to right.
func Merge(defaults map[string]any, overrides []Override) map[string]any {
	out := maps.Clone(defaults)
	if out == nil {
		out = make(map[string]any)
	}
	for _, o := range overrides {
		maps.Copy(out, o)
	}
	return out
}

// Create inserts a row through the repo layer, failing the test on error.
func Create[T any](t *testing.T, q repo.Querier, res *repo.Resource[T], values map[string]any) *T {
	t.Helper()
	item, err := res.Create(context.Background(), q, values)
	if err != nil {
		t.Fatalf("factory create %T: %v", *new(T), err)
	}
	return item
}

// EnsureFK sets column to create() when the caller did not supply it.
func EnsureFK(merged map[string]any, column string, create func() string) {
	if _, ok := merged[column]; !ok {
		merged[column] = create()
	}
}
