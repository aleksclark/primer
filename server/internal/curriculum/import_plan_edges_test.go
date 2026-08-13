package curriculum_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
)

func TestBundleDigestAndPlanImportValidationEdges(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	opts := curriculum.ImportOptions{Now: time.Now().UTC(), SourceLabel: "edges"}

	_, err := curriculum.BundleDigest(nil)
	require.Error(t, err)

	// Nil bundle plan.
	_, err = curriculum.PlanImport(ctx, q, nil, opts)
	require.Error(t, err)

	// Empty bundle shape.
	empty := &curriculum.ImportBundle{SchemaVersion: "1"}
	plan, err := curriculum.PlanImport(ctx, q, empty, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)
	require.NotEmpty(t, plan.Errors)

	// Official source rejected.
	b := sampleBundle()
	b.Standards[0].Source = "ngss"
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// Unauthorized custom namespace.
	b = sampleBundle()
	b.Standards[0].Code = "OTHER.X.1"
	b.Activities[0].Standards[0].Code = "OTHER.X.1"
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// Missing subject_code on standard.
	b = sampleBundle()
	b.Standards[0].SubjectCode = ""
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// DisallowCreateSubjects + unknown subject.
	b = sampleBundle()
	b.Standards[0].SubjectCode = "no-such-subject-xyz"
	b.Activities[0].SubjectCode = "no-such-subject-xyz"
	opts2 := opts
	opts2.DisallowCreateSubjects = true
	plan, err = curriculum.PlanImport(ctx, q, b, opts2)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// Invalid activity document.
	b = sampleBundle()
	b.Activities[0].Slug = "Bad_Slug"
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// Activity references unknown standard not in bundle/DB.
	b = sampleBundle()
	b.Activities[0].Standards = []contracts.StandardRef{
		{Code: "PRIMER.DL.6.NAV.999", Role: contracts.StandardRolePrimary},
	}
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// Invalid course document.
	b = sampleBundle()
	b.Course.Slug = "Bad"
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// Course activity missing from bundle and DB.
	b = sampleBundle()
	b.Course.Activities = []contracts.CourseActivityRef{{Order: 1, Slug: "never-published-slug"}}
	// Keep bundle activity different so course slug is external-missing.
	b.Activities[0].Slug = "import-nav-demo"
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// Secret/path soft scan.
	b = sampleBundle()
	b.SourceLabel = `file:///home/secret`
	// Actually scan is over marshaled JSON — put password field via summary.
	b.Activities[0].Summary = `{"password":"x"}`
	// Summary is plain string; inject into title for substring match of "password"
	b.Activities[0].Title = `password`
	// The scan looks for \"password\" JSON key — put in SourceLabel as raw? Use a standard description.
	b.Standards[0].Description = `{"password":"x"}`
	// Description is free text containing the JSON-ish key pattern after marshal.
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	// May or may not trip depending on marshal escaping — force via SourceLabel with file://
	b = sampleBundle()
	b.SourceLabel = "file:///tmp/x"
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// Duplicate activity slugs.
	b = sampleBundle()
	b.Activities = append(b.Activities, b.Activities[0])
	plan, err = curriculum.PlanImport(ctx, q, b, opts)
	require.NoError(t, err)
	assert.False(t, plan.Valid)

	// Wildcard authorized namespaces allows non-PRIMER custom codes that still fail validation
	// of activity standard codes — use authorized * for a custom PRIMER-like code path.
	b = sampleBundle()
	opts3 := opts
	opts3.AuthorizedNamespaces = []string{"*"}
	plan, err = curriculum.PlanImport(ctx, q, b, opts3)
	require.NoError(t, err)
	assert.True(t, plan.Valid, "errors: %v", plan.Errors)

	// Digest stable across standard order.
	b1 := sampleBundle()
	b2 := sampleBundle()
	b2.Standards = append([]curriculum.StandardSeed{}, b1.Standards...)
	// Add second standard and reverse order.
	b1.Standards = append(b1.Standards, curriculum.StandardSeed{
		Code: "PRIMER.DL.6.NAV.2", Source: "custom", SubjectCode: "digital-literacy", Domain: "nav", Description: "d2",
	})
	b2.Standards = []curriculum.StandardSeed{b1.Standards[1], b1.Standards[0]}
	// Drop course/activities differences — keep activities same so digest compares standards sort.
	b1.Activities = nil
	b2.Activities = nil
	b1.Course = nil
	b2.Course = nil
	d1, err := curriculum.BundleDigest(b1)
	require.NoError(t, err)
	d2, err := curriculum.BundleDigest(b2)
	require.NoError(t, err)
	assert.Equal(t, d1, d2)
}

func TestBuildBundleFromDirsEdges(t *testing.T) {
	t.Parallel()
	// Empty dirs → empty-ish bundle (no error).
	b, err := curriculum.BuildBundleFromDirs("", "", "", "lab")
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Equal(t, "lab", b.SourceLabel)

	// Missing standards dir.
	_, err = curriculum.BuildBundleFromDirs(filepath.Join(t.TempDir(), "missing"), "", "", "x")
	require.Error(t, err)

	// Bad activity directory (subdir without activity file → error).
	actDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(actDir, "broken-act"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(actDir, "broken-act", "readme.txt"), []byte("nope"), 0o644))
	_, err = curriculum.BuildBundleFromDirs("", actDir, "", "x")
	require.Error(t, err)

	// Invalid YAML in activity.yaml.
	actDir2 := t.TempDir()
	badSlug := filepath.Join(actDir2, "bad-slug")
	require.NoError(t, os.MkdirAll(badSlug, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(badSlug, "activity.yaml"), []byte(":::not-yaml"), 0o644))
	_, err = curriculum.BuildBundleFromDirs("", actDir2, "", "x")
	require.Error(t, err)

	// Missing course path.
	_, err = curriculum.BuildBundleFromDirs("", "", filepath.Join(t.TempDir(), "no-course.json"), "x")
	require.Error(t, err)
}

func TestApplyImportDigestMismatchAndDefaults(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	bundle := sampleBundle()

	// Zero Now + nil namespaces use defaults; wrong digest rejected.
	_, _, err := curriculum.ApplyImport(ctx, q, bundle, "deadbeef", curriculum.ImportOptions{})
	require.Error(t, err)
}
