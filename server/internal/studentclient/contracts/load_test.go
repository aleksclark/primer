package contracts_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestLoadCurriculumActivities(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	dir := filepath.Join(root, "curriculum", "activities")
	require.DirExists(t, dir)

	docs, errs := contracts.LoadDocumentsDir(dir)
	require.Empty(t, errs)
	require.GreaterOrEqual(t, len(docs), 3)

	bySlug := map[string]*contracts.ActivityDocument{}
	for _, d := range docs {
		bySlug[d.Slug] = d
	}
	nav := bySlug["basic-navigation"]
	require.NotNil(t, nav)
	assert.Equal(t, contracts.KindTerminal, nav.Kind)
	assert.Equal(t, "PRIMER.DL.6.NAV.1", nav.Standards[0].Code)
	assert.NotEmpty(t, nav.Content.Terminal.Fixtures)
	assert.NotEmpty(t, nav.Content.Tasks)
	assert.NotEmpty(t, nav.Content.Checks)

	files := bySlug["file-organization"]
	require.NotNil(t, files)
	assert.Equal(t, "PRIMER.DL.6.FILES.1", files.Standards[0].Code)

	typingDoc := bySlug["command-typing-basics"]
	require.NotNil(t, typingDoc)
	assert.Equal(t, contracts.KindTyping, typingDoc.Kind)
	assert.Equal(t, "PRIMER.DL.6.TYPE.1", typingDoc.Standards[0].Code)
	for _, s := range typingDoc.Standards {
		assert.Contains(t, s.Code, ".TYPE.")
		assert.NotContains(t, s.Code, ".NAV.")
		assert.NotContains(t, s.Code, ".FILES.")
	}
	require.NotNil(t, typingDoc.Content.Typing)
	assert.NotEmpty(t, typingDoc.Content.Typing.Prompts)
	require.NotEmpty(t, typingDoc.Content.Checks)
	assert.Equal(t, contracts.CheckTypingMetrics, typingDoc.Content.Checks[0].Kind)
}

func TestLoadDocumentJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "activity.json")
	doc := sampleTerminal()
	require.NoError(t, os.WriteFile(path, contracts.MustJSON(doc), 0o644))
	got, err := contracts.LoadDocument(path)
	require.NoError(t, err)
	assert.Equal(t, doc.Slug, got.Slug)
}
