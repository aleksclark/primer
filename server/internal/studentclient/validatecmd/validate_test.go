package validatecmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/validatecmd"
)

func curriculumActivitiesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// server/internal/studentclient/validatecmd -> repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	dir := filepath.Join(root, "curriculum", "activities")
	st, err := os.Stat(dir)
	require.NoError(t, err)
	require.True(t, st.IsDir())
	return dir
}

func TestRunCurriculumSamples(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	results, err := validatecmd.Run(validatecmd.Options{
		ActivitiesDir: curriculumActivitiesDir(t),
		Stdout:        &stdout,
		Stderr:        &stderr,
		Materialize:   true,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 3)
	assert.True(t, validatecmd.AllOK(results), stderr.String())
	assert.Contains(t, stdout.String(), "basic-navigation")
	assert.Contains(t, stdout.String(), "file-organization")
	assert.Contains(t, stdout.String(), "command-typing-basics")
	assert.Contains(t, stdout.String(), "OK")
}

func TestRunRejectsBadActivity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad-activity")
	require.NoError(t, os.MkdirAll(bad, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bad, "activity.yaml"), []byte(`
schema_version: "1"
slug: bad-activity
title: Bad
kind: terminal
subject_code: digital-literacy
standards:
  - code: PRIMER.DL.6.NAV.1
    role: primary
content:
  objective: x
  instructions: y
  terminal:
    runtime_profile: coreutils-basic
    fixtures:
      - path: ../escape
        type: file
        content: nope
  tasks:
    - id: t1
      title: T
      instructions: I
      completion:
        check_id: c1
  checks:
    - id: c1
      kind: file_exists
      params:
        path: ../escape
`), 0o644))

	var stderr bytes.Buffer
	results, err := validatecmd.Run(validatecmd.Options{
		ActivitiesDir: dir,
		Stderr:        &stderr,
		Materialize:   true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.False(t, validatecmd.AllOK(results))
	assert.Contains(t, results[0].Error, "unsafe")
}
