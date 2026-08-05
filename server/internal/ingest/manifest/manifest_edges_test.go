package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
)

func TestManifestLoadSaveValidateEdges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := manifest.Load(filepath.Join(dir, "missing.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read manifest")

	bad := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("items: [\n"), 0o644))
	_, err = manifest.Load(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")

	// Validate rejects nil, empty id/title, bad class.
	assert.Error(t, (*manifest.Manifest)(nil).Validate())
	assert.Error(t, (&manifest.Manifest{Items: []manifest.Item{{Title: "x", Kind: manifest.KindManual, Class: manifest.ClassMixed}}}).Validate())
	assert.Error(t, (&manifest.Manifest{Items: []manifest.Item{{ID: "a", Kind: manifest.KindManual, Class: manifest.ClassMixed}}}).Validate())
	assert.Error(t, (&manifest.Manifest{Items: []manifest.Item{{ID: "a", Title: "A", Kind: manifest.KindManual, Class: "nope"}}}).Validate())

	// Save validates first.
	err = manifest.Save(filepath.Join(dir, "out.yaml"), &manifest.Manifest{Items: []manifest.Item{{ID: "a", Title: "A", Kind: "bad", Class: manifest.ClassMixed}}})
	require.Error(t, err)

	// Save to unwritable path (file-as-dir parent).
	block := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(block, []byte("x"), 0o644))
	ok := &manifest.Manifest{Items: []manifest.Item{{ID: "a", Title: "A", Kind: manifest.KindManual, Class: manifest.ClassMixed}}}
	err = manifest.Save(filepath.Join(block, "m.yaml"), ok)
	require.Error(t, err)

	// SetProvider missing id.
	require.Error(t, ok.SetProvider("missing", manifest.Provider{TMDB: 1}))
	assert.Nil(t, ok.ByID("missing"))

	// Review load corrupt + save/remove edges.
	rpath := filepath.Join(dir, "review.yaml")
	require.NoError(t, os.WriteFile(rpath, []byte("entries: [\n"), 0o644))
	_, err = manifest.LoadReview(rpath)
	require.Error(t, err)

	// SaveReview on bad path.
	r := &manifest.Review{}
	r.Upsert(manifest.ReviewEntry{ID: "x", Title: "X", Kind: manifest.KindMovie})
	err = manifest.SaveReview(filepath.Join(block, "r.yaml"), r)
	require.Error(t, err)

	// Remove missing is no-op; ByID nil.
	r.Remove("nope")
	assert.Nil(t, r.ByID("nope"))
	assert.NotNil(t, r.ByID("x"))
}
