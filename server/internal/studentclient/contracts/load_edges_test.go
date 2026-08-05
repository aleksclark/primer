package contracts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestLoadDocumentAndParseErrorPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Missing file.
	_, err := contracts.LoadDocument(filepath.Join(dir, "missing.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read activity")

	// Invalid YAML.
	badYAML := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(badYAML, []byte(":\n  - ["), 0o644))
	_, err = contracts.ParseDocument([]byte(":\n  - ["), badYAML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "yaml")

	// Invalid JSON.
	_, err = contracts.ParseDocument([]byte("{"), "activity.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "json")

	// Valid parse but Validate fails on LoadDocument.
	path := filepath.Join(dir, "activity.yaml")
	require.NoError(t, os.WriteFile(path, []byte("schemaVersion: \"1\"\nslug: x\n"), 0o644))
	_, err = contracts.LoadDocument(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate")
}

func TestLoadDocumentsDirErrorPaths(t *testing.T) {
	t.Parallel()
	// Missing root dir.
	docs, errs := contracts.LoadDocumentsDir(filepath.Join(t.TempDir(), "nope"))
	assert.Nil(t, docs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "read activities dir")

	root := t.TempDir()
	// Non-dir entries skipped.
	require.NoError(t, os.WriteFile(filepath.Join(root, "readme.txt"), []byte("x"), 0o644))

	// Dir without activity file.
	empty := filepath.Join(root, "empty-act")
	require.NoError(t, os.Mkdir(empty, 0o755))

	// Dir with invalid activity.
	bad := filepath.Join(root, "bad-act")
	require.NoError(t, os.Mkdir(bad, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bad, "activity.yaml"), []byte("schemaVersion: \"1\"\n"), 0o644))

	// Dir with slug mismatch (valid document content, wrong slug vs directory).
	mismatch := filepath.Join(root, "dir-name")
	require.NoError(t, os.Mkdir(mismatch, 0o755))
	doc := sampleTerminal()
	doc.Slug = "other-slug"
	require.NoError(t, os.WriteFile(filepath.Join(mismatch, "activity.json"), contracts.MustJSON(doc), 0o644))

	docs, errs = contracts.LoadDocumentsDir(root)
	assert.Empty(t, docs)
	require.GreaterOrEqual(t, len(errs), 3)
	var joined strings.Builder
	for _, e := range errs {
		joined.WriteString(e.Error())
		joined.WriteByte('\n')
	}
	all := joined.String()
	assert.Contains(t, all, "no activity.yaml")
	assert.True(t, strings.Contains(all, "slug") || strings.Contains(all, "validate") || strings.Contains(all, "required"), all)
}

func TestMustJSONPanicsOnBadValue(t *testing.T) {
	t.Parallel()
	defer func() {
		require.NotNil(t, recover())
	}()
	_ = contracts.MustJSON(map[string]any{"ch": make(chan int)})
}
