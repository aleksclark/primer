package contracts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestParseDocumentExtensionFallbackAndLoadEdges(t *testing.T) {
	t.Parallel()

	// Encode a known-good document as JSON and YAML via MustJSON + Decode roundtrip.
	base := sampleTerminal()
	base.Slug = "parse-edge"
	jsonBody := contracts.MustJSON(base)

	doc, err := contracts.ParseDocument(jsonBody, "a.json")
	require.NoError(t, err)
	assert.Equal(t, "parse-edge", doc.Slug)

	// Empty extension defaults to YAML — write real YAML using snake_case tags via file from curriculum sample is heavy;
	// instead use DecodeActivityYAML path only when content is YAML. For ParseDocument(""), YAML is tried first.
	// Provide minimal valid YAML with snake_case field names.
	yamlBody := []byte(`
schema_version: "1"
slug: parse-edge-yaml
title: Parse Edge
kind: terminal
subject_code: digital-literacy
standards:
  - code: PRIMER.DL.6.NAV.1
    role: primary
content:
  objective: o
  instructions: i
  terminal:
    runtime_profile: coreutils-basic
    fixtures:
      - path: home
        type: directory
  checks:
    - id: exists
      kind: file_exists
      params:
        path: home
  tasks:
    - id: t1
      title: T
      instructions: G
      completion:
        check_id: exists
`)
	doc, err = contracts.ParseDocument(yamlBody, "activity.yaml")
	require.NoError(t, err)
	assert.Equal(t, "parse-edge-yaml", doc.Slug)

	doc, err = contracts.ParseDocument(yamlBody, "activity")
	require.NoError(t, err)
	assert.Equal(t, "parse-edge-yaml", doc.Slug)

	// Unknown extension still parses YAML body.
	doc, err = contracts.ParseDocument(yamlBody, "x.toml")
	require.NoError(t, err)
	assert.Equal(t, "parse-edge-yaml", doc.Slug)

	// LoadDocument missing
	_, err = contracts.LoadDocument(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)

	// LoadDocument invalid content
	p := filepath.Join(t.TempDir(), "activity.yaml")
	require.NoError(t, os.WriteFile(p, []byte("slug: x\n"), 0o644))
	_, err = contracts.LoadDocument(p)
	require.Error(t, err)

	// LoadCourseDocument missing + invalid
	_, err = contracts.LoadCourseDocument(filepath.Join(t.TempDir(), "c.json"))
	require.Error(t, err)
	cp := filepath.Join(t.TempDir(), "course.json")
	require.NoError(t, os.WriteFile(cp, []byte(`{"schemaVersion":"1"}`), 0o644))
	_, err = contracts.LoadCourseDocument(cp)
	require.Error(t, err)

	// LoadDocumentsDir missing root
	_, errs := contracts.LoadDocumentsDir(filepath.Join(t.TempDir(), "nope"))
	require.NotEmpty(t, errs)

	// Empty dir → no docs, no errs
	docs, errs := contracts.LoadDocumentsDir(t.TempDir())
	assert.Empty(t, docs)
	assert.Empty(t, errs)

	// MustJSON panics on bad value
	require.Panics(t, func() { contracts.MustJSON(make(chan int)) })
	b := contracts.MustJSON(map[string]any{"a": 1})
	assert.Contains(t, string(b), `"a"`)
}
