package contracts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func conformanceDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "testdata", "conformance")
}

func activitySchemaPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	return filepath.Join(root, "content", "technology", "basic_linux", "activity.schema.json")
}

func courseSchemaPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	return filepath.Join(root, "content", "technology", "basic_linux", "course.schema.json")
}

func TestConformanceAccept(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(conformanceDir(t), "accept")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			if strings.HasPrefix(name, "course_") {
				doc, err := contracts.DecodeCourseJSON(data, path)
				require.NoError(t, err)
				require.NoError(t, contracts.ValidateCourseDocument(doc))
				assertJSONSchema(t, courseSchemaPath(t), data, true)
				return
			}

			doc, err := contracts.DecodeActivityJSON(data, path)
			require.NoError(t, err)
			require.NoError(t, contracts.ValidateDocument(doc))
			assertJSONSchema(t, activitySchemaPath(t), data, true)
		})
	}
}

func TestConformanceReject(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(conformanceDir(t), "reject")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			if strings.HasPrefix(name, "course_") {
				_, decErr := contracts.DecodeCourseJSON(data, path)
				valErr := error(nil)
				if decErr == nil {
					doc, _ := contracts.DecodeCourseJSON(data, path)
					valErr = contracts.ValidateCourseDocument(doc)
				}
				assert.True(t, decErr != nil || valErr != nil, "expected Go rejection")
				assertJSONSchema(t, courseSchemaPath(t), data, false)
				return
			}

			doc, decErr := contracts.DecodeActivityJSON(data, path)
			var valErr error
			if decErr == nil {
				valErr = contracts.ValidateDocument(doc)
			}
			assert.True(t, decErr != nil || valErr != nil, "expected Go rejection for %s (dec=%v val=%v)", name, decErr, valErr)

			// Schema path: duplicate keys may be accepted by jsonschema after parse,
			// but unknown fields/kinds/params/version must fail schema.
			if name != "duplicate_key.json" {
				assertJSONSchema(t, activitySchemaPath(t), data, false)
			}
		})
	}
}

func TestStrictDecodeUnknownFieldJSON(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"schemaVersion":"1","slug":"x","title":"t","summary":"s","kind":"terminal","subjectCode":"digital-literacy","standards":[{"code":"PRIMER.DL.6.NAV.1","role":"primary"}],"content":{"objective":"o","instructions":"i","terminal":{"runtimeProfile":"coreutils-basic","fixtures":[]},"tasks":[{"id":"t1","title":"T","instructions":"I","completion":{"checkId":"c1"}}],"checks":[{"id":"c1","kind":"file_exists","params":{"path":"a"}}]},"nope":true}`)
	_, err := contracts.DecodeActivityJSON(raw, "mem.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

func TestStrictDecodeDuplicateYAML(t *testing.T) {
	t.Parallel()
	raw := []byte("schema_version: \"1\"\nschema_version: \"1\"\nslug: x\ntitle: t\nsummary: s\nkind: terminal\nsubject_code: digital-literacy\nstandards:\n  - code: PRIMER.DL.6.NAV.1\n    role: primary\ncontent:\n  objective: o\n  instructions: i\n  terminal:\n    runtime_profile: coreutils-basic\n    fixtures: []\n  tasks:\n    - id: t1\n      title: T\n      instructions: I\n      completion:\n        check_id: c1\n  checks:\n    - id: c1\n      kind: file_exists\n      params:\n        path: a\n")
	_, err := contracts.DecodeActivityYAML(raw, "mem.yaml")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "duplicate")
}

func TestStrictDecodeUnknownSchemaVersion(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.SchemaVersion = "999"
	raw := contracts.MustJSON(doc)
	_, err := contracts.DecodeActivityJSON(raw, "mem.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported schema version")
}

func TestStagesDefaultFinal(t *testing.T) {
	t.Parallel()
	ch := contracts.Check{ID: "c", Kind: contracts.CheckFileExists}
	assert.Equal(t, []string{contracts.StageFinal}, contracts.EffectiveStages(ch))
	assert.False(t, contracts.HasStage(ch, contracts.StageFixture))
	assert.True(t, contracts.HasStage(ch, contracts.StageFinal))
}

func TestMaterializeChecksSkipFinalOutcomes(t *testing.T) {
	t.Parallel()
	checks := []contracts.Check{
		{ID: "fix", Stages: []string{contracts.StageFixture}, Kind: contracts.CheckFileExists, Params: map[string]any{"path": "a"}},
		{ID: "out", Stages: []string{contracts.StageFinal}, Kind: contracts.CheckContentEquals, Params: map[string]any{"path": "a", "value": "x"}},
		{ID: "task", Stages: []string{contracts.StageTask}, Kind: contracts.CheckContentContains, Params: map[string]any{"path": "a", "value": "bad"}},
		{ID: "inv", InvariantAt: []string{contracts.InvariantAtFixture}, Kind: contracts.CheckFileExists, Params: map[string]any{"path": "b"}},
	}
	got := contracts.MaterializeChecks(checks)
	ids := []string{}
	for _, c := range got {
		ids = append(ids, c.ID)
	}
	assert.ElementsMatch(t, []string{"fix", "inv"}, ids)
}

func TestLinuxCourseManifest(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	path := filepath.Join(root, "content", "technology", "basic_linux", "course.json")
	doc, err := contracts.LoadCourseDocument(path)
	require.NoError(t, err)
	assert.Equal(t, "basic-linux", doc.Slug)
	assert.Len(t, doc.Activities, 20)
	assert.NotEmpty(t, doc.Prerequisites)
}

func TestLinuxLesson20LoadsWithStages(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	path := filepath.Join(root, "content", "technology", "basic_linux", "lessons", "20-capstone-verification", "activity.json")
	doc, err := contracts.LoadDocument(path)
	require.NoError(t, err)
	var taskOnly, fixture, final int
	for _, ch := range doc.Content.Checks {
		if contracts.HasStage(ch, contracts.StageTask) && !contracts.HasStage(ch, contracts.StageFinal) {
			taskOnly++
		}
		if contracts.HasStage(ch, contracts.StageFixture) {
			fixture++
		}
		if contracts.HasStage(ch, contracts.StageFinal) {
			final++
		}
	}
	assert.Greater(t, taskOnly, 0, "lesson 20 should have task-scoped initial checks")
	assert.Greater(t, fixture, 0, "lesson 20 should declare fixture-stage invariants")
	assert.Greater(t, final, 0)
	// Materialize checks must not include final-only repaired content checks.
	mat := contracts.MaterializeChecks(doc.Content.Checks)
	for _, ch := range mat {
		assert.NotEqual(t, "readme-correct", ch.ID)
		assert.NotEqual(t, "script-correct", ch.ID)
	}
}

func assertJSONSchema(t *testing.T, schemaPath string, instance []byte, wantValid bool) {
	t.Helper()
	// Prefer python jsonschema when available for Draft 2020-12 conditionals.
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for schema conformance")
	}
	tmp := t.TempDir()
	instPath := filepath.Join(tmp, "instance.json")
	require.NoError(t, os.WriteFile(instPath, instance, 0o644))
	script := `
import json, sys
from pathlib import Path
try:
    from jsonschema import Draft202012Validator
except ImportError:
    sys.exit(2)
schema = json.loads(Path(sys.argv[1]).read_text())
inst = json.loads(Path(sys.argv[2]).read_text())
v = Draft202012Validator(schema)
errors = sorted(v.iter_errors(inst), key=lambda e: list(e.path))
if errors:
    for e in errors[:5]:
        loc = ".".join(str(p) for p in e.path) or "<root>"
        print(f"{loc}: {e.message}")
    sys.exit(1)
sys.exit(0)
`
	cmd := exec.Command("python3", "-c", script, schemaPath, instPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 2 {
			t.Skip("jsonschema python package not installed")
		}
		if wantValid {
			t.Fatalf("schema expected valid: %v\n%s", err, out)
		}
		return
	}
	if !wantValid {
		t.Fatalf("schema expected invalid but passed")
	}
}

func TestRejectUnknownCheckParam(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Checks[0].Params["extra"] = "nope"
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown params")
}

func TestCourseRejectsDuplicateOrder(t *testing.T) {
	t.Parallel()
	doc := &contracts.CourseDocument{
		SchemaVersion: contracts.CourseSchemaVersion,
		Slug:          "c",
		Title:         "C",
		SubjectCode:   "digital-literacy",
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "a"},
			{Order: 1, Slug: "b"},
		},
	}
	err := contracts.ValidateCourseDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestCourseRejectsCycle(t *testing.T) {
	t.Parallel()
	doc := &contracts.CourseDocument{
		SchemaVersion: contracts.CourseSchemaVersion,
		Slug:          "c",
		Title:         "C",
		SubjectCode:   "digital-literacy",
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "a"},
			{Order: 2, Slug: "b"},
		},
		Prerequisites: []contracts.CoursePrerequisite{
			{Activity: "a", Requires: []string{"b"}},
			{Activity: "b", Requires: []string{"a"}},
		},
	}
	err := contracts.ValidateCourseDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestAnalyzeDocumentUnused(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID:     "orphan",
		Kind:   contracts.CheckFileExists,
		Params: map[string]any{"path": "home/x"},
		Stages: []string{contracts.StageFinal},
	})
	d := contracts.AnalyzeDocument(doc)
	assert.Contains(t, d.UnusedChecks, "orphan")
	assert.NotEmpty(t, d.Warnings)
}

// Ensure encoding/json is used so testdata round-trips stay available.
var _ = json.Marshal
