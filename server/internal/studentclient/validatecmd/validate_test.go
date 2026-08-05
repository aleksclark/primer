package validatecmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
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

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
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

func TestRepairActivityMaterializeUsesStages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	act := filepath.Join(dir, "repair-lab")
	require.NoError(t, os.MkdirAll(act, 0o755))
	// Fixture has broken content; final check wants fixed content; task check
	// proves the broken initial content while the task is active.
	yaml := `
schema_version: "1"
slug: repair-lab
title: Repair Lab
summary: fix a file
kind: terminal
subject_code: digital-literacy
standards:
  - code: PRIMER.DL.6.FILES.1
    role: primary
content:
  objective: repair
  instructions: fix the file
  terminal:
    runtime_profile: coreutils-basic
    fixtures:
      - path: note.txt
        type: file
        content: "broken\n"
  tasks:
    - id: inspect
      title: Inspect
      instructions: See broken content
      completion:
        check_id: initial-broken
    - id: fix
      title: Fix
      instructions: Write fixed
      prerequisites: [inspect]
      completion:
        check_id: final-fixed
  checks:
    - id: initial-broken
      kind: content_equals
      stages: [task]
      params:
        path: note.txt
        value: "broken\n"
    - id: note-exists
      kind: file_exists
      stages: [fixture, final]
      invariant_at: [fixture, final]
      params:
        path: note.txt
    - id: final-fixed
      kind: content_equals
      stages: [final]
      params:
        path: note.txt
        value: "fixed\n"
`
	require.NoError(t, os.WriteFile(filepath.Join(act, "activity.yaml"), []byte(yaml), 0o644))

	var stdout, stderr bytes.Buffer
	results, err := validatecmd.Run(validatecmd.Options{
		ActivitiesDir: dir,
		Stdout:        &stdout,
		Stderr:        &stderr,
		Materialize:   true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].OK, stderr.String())
	assert.Contains(t, results[0].StageSummary, "fixture=")
}

func TestReferenceReplay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	act := filepath.Join(dir, "replay-lab")
	require.NoError(t, os.MkdirAll(act, 0o755))
	yaml := `
schema_version: "1"
slug: replay-lab
title: Replay Lab
summary: s
kind: terminal
subject_code: digital-literacy
standards:
  - code: PRIMER.DL.6.FILES.1
    role: primary
content:
  objective: o
  instructions: i
  terminal:
    runtime_profile: coreutils-basic
    fixtures:
      - path: out.txt
        type: file
        content: "start\n"
  tasks:
    - id: t1
      title: T
      instructions: I
      completion:
        check_id: done
  checks:
    - id: start
      kind: content_equals
      stages: [fixture]
      params:
        path: out.txt
        value: "start\n"
    - id: done
      kind: content_equals
      stages: [final]
      params:
        path: out.txt
        value: "done\n"
reference_solution:
  deterministic: true
  steps:
    - argv: ["sh", "-c", "printf 'done\\n' > out.txt"]
`
	require.NoError(t, os.WriteFile(filepath.Join(act, "activity.yaml"), []byte(yaml), 0o644))

	var stderr bytes.Buffer
	results, err := validatecmd.Run(validatecmd.Options{
		ActivitiesDir:   dir,
		Stderr:          &stderr,
		Materialize:     true,
		ReplayReference: true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	if !results[0].OK {
		// bwrap may be restricted in some environments.
		if assert.Contains(t, results[0].Error, "bwrap") || assert.Contains(t, results[0].Error, "bubblewrap") {
			t.Skip(results[0].Error)
		}
		t.Fatalf("replay failed: %s", results[0].Error)
	}
}

func TestLesson20MaterializeWithoutWorkaround(t *testing.T) {
	t.Parallel()
	path := filepath.Join(repoRoot(t), "content", "technology", "basic_linux", "lessons", "20-capstone-verification", "activity.json")
	require.FileExists(t, path)

	// Sanity: document declares task-only imperfect checks.
	doc, err := contracts.LoadDocument(path)
	require.NoError(t, err)
	foundTask := false
	for _, ch := range doc.Content.Checks {
		if ch.ID == "initial-script-imperfect" {
			assert.True(t, contracts.HasStage(ch, contracts.StageTask))
			assert.False(t, contracts.HasStage(ch, contracts.StageFixture))
			foundTask = true
		}
	}
	require.True(t, foundTask)

	var stderr bytes.Buffer
	results, err := validatecmd.Run(validatecmd.Options{
		SingleFile:  path,
		Stderr:      &stderr,
		Materialize: true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].OK, stderr.String()+results[0].Error)
}
