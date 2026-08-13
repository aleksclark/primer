package validatecmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/validatecmd"
)

func writeMiniActivity(t *testing.T, dir, slug string, withRef bool) {
	t.Helper()
	act := filepath.Join(dir, slug)
	require.NoError(t, os.MkdirAll(act, 0o755))
	body := `
schema_version: "1"
slug: ` + slug + `
title: Mini
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
      - path: home/a.txt
        type: file
        content: hi
  tasks:
    - id: t1
      title: T
      instructions: G
      completion:
        check_id: c1
  checks:
    - id: c1
      kind: file_exists
      params:
        path: home/a.txt
`
	if withRef {
		body += `
reference_solution:
  steps:
    - argv: ["true"]
`
	}
	require.NoError(t, os.WriteFile(filepath.Join(act, "activity.yaml"), []byte(body), 0o644))
}

func TestRunSingleFileQuietAndEmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMiniActivity(t, dir, "mini-ok", false)
	path := filepath.Join(dir, "mini-ok", "activity.yaml")

	var stdout, stderr bytes.Buffer
	results, err := validatecmd.Run(validatecmd.Options{
		SingleFile:  path,
		Stdout:      &stdout,
		Stderr:      &stderr,
		Materialize: true,
		Quiet:       true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].OK)
	// Quiet suppresses OK lines.
	assert.Empty(t, stdout.String())

	// Non-quiet single file prints OK.
	stdout.Reset()
	results, err = validatecmd.Run(validatecmd.Options{
		SingleFile: path,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	require.NoError(t, err)
	assert.True(t, results[0].OK)
	assert.Contains(t, stdout.String(), "OK")
	assert.Contains(t, stdout.String(), "mini-ok")

	// Empty activities dir.
	empty := t.TempDir()
	_, err = validatecmd.Run(validatecmd.Options{ActivitiesDir: empty})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no activity directories")

	// Missing dir.
	_, err = validatecmd.Run(validatecmd.Options{ActivitiesDir: filepath.Join(t.TempDir(), "missing")})
	require.Error(t, err)

	// DefaultActivitiesDir from repo context (may resolve).
	got := validatecmd.DefaultActivitiesDir()
	assert.NotEmpty(t, got)

	// AllOK empty false.
	assert.False(t, validatecmd.AllOK(nil))
	assert.False(t, validatecmd.AllOK([]validatecmd.Result{}))
}

func TestRunSlugMismatchAndMissingActivityFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Directory name differs from slug inside YAML.
	writeMiniActivity(t, dir, "dir-name", false)
	// Overwrite slug inside file.
	p := filepath.Join(dir, "dir-name", "activity.yaml")
	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	// slug is dir-name in writeMiniActivity; change to other-slug
	require.NoError(t, os.WriteFile(p, bytes.ReplaceAll(raw, []byte("slug: dir-name"), []byte("slug: other-slug")), 0o644))

	var stderr bytes.Buffer
	results, err := validatecmd.Run(validatecmd.Options{ActivitiesDir: dir, Stderr: &stderr})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.Contains(t, results[0].Error, "must match directory")
	assert.Contains(t, stderr.String(), "ERR")

	// Missing activity file in subdir.
	dir2 := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir2, "empty-act"), 0o755))
	results, err = validatecmd.Run(validatecmd.Options{ActivitiesDir: dir2, Stderr: &stderr})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.Contains(t, results[0].Error, "no activity.yaml")
}

func TestRunReplayReferenceWithoutMaterialize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMiniActivity(t, dir, "with-ref", true)
	var stderr bytes.Buffer
	results, err := validatecmd.Run(validatecmd.Options{
		ActivitiesDir:   dir,
		ReplayReference: true,
		Materialize:     false,
		Stderr:          &stderr,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.Contains(t, results[0].Error, "reference replay requires materialize")
}

func TestRunInvalidYAMLSingleFile(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "activity.yaml")
	require.NoError(t, os.WriteFile(p, []byte("not: valid: activity: ["), 0o644))
	results, err := validatecmd.Run(validatecmd.Options{SingleFile: p})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.NotEmpty(t, results[0].Error)
}

func TestRunMaterializeFixtureCheckFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	act := filepath.Join(dir, "fail-fix")
	require.NoError(t, os.MkdirAll(act, 0o755))
	// Fixture does not create the path the fixture-stage check requires.
	require.NoError(t, os.WriteFile(filepath.Join(act, "activity.yaml"), []byte(`
schema_version: "1"
slug: fail-fix
title: Fail
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
  tasks:
    - id: t1
      title: T
      instructions: G
      completion:
        check_id: c1
  checks:
    - id: c1
      kind: file_exists
      params:
        path: home/missing.txt
      stages: [fixture]
`), 0o644))

	results, err := validatecmd.Run(validatecmd.Options{
		ActivitiesDir: dir,
		Materialize:   true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.Contains(t, results[0].Error, "fixture-stage")
}
