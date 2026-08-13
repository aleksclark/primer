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
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
)

func TestPlanImportReusesExistingDigestAndExternalCourseActivity(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	opts := curriculum.ImportOptions{Now: time.Now().UTC(), SourceLabel: "cov"}

	// First apply creates activity.
	bundle := sampleBundle()
	plan, err := curriculum.PlanImport(ctx, q, bundle, opts)
	require.NoError(t, err)
	require.True(t, plan.Valid, "errors: %v", plan.Errors)
	_, _, err = curriculum.ApplyImport(ctx, q, bundle, plan.BundleDigest, opts)
	require.NoError(t, err)

	// Same content → reuse action via revisionByDigestOptional.
	plan2, err := curriculum.PlanImport(ctx, q, bundle, opts)
	require.NoError(t, err)
	require.True(t, plan2.Valid)
	var sawReuse bool
	for _, a := range plan2.Actions {
		if a.Kind == "activity" && a.Slug == "import-nav-demo" && a.Action == "reuse" {
			sawReuse = true
		}
	}
	assert.True(t, sawReuse, "expected reuse action, got %+v", plan2.Actions)

	// New revision content → create action for new revision.
	bundleNew := sampleBundle()
	bundleNew.Activities[0].Content.Objective = "learn more deeply"
	// Drop course so we only exercise activity planning.
	bundleNew.Course = nil
	plan3, err := curriculum.PlanImport(ctx, q, bundleNew, opts)
	require.NoError(t, err)
	require.True(t, plan3.Valid)
	var sawCreateRev bool
	for _, a := range plan3.Actions {
		if a.Kind == "activity" && a.Action == "create" && a.Detail == "new revision" {
			sawCreateRev = true
		}
	}
	assert.True(t, sawCreateRev, "expected new revision create, got %+v", plan3.Actions)

	// Course referencing already-published activity not present in this bundle
	// exercises latestPublishedActivityRevisionID.
	external := &curriculum.ImportBundle{
		SchemaVersion: "1",
		SourceLabel:   "external-course",
		Standards:     bundle.Standards,
		// no activities — course points at previously published slug
		Course: &contracts.CourseDocument{
			SchemaVersion: "1",
			Slug:          "external-only-course",
			Title:         "External",
			SubjectCode:   "digital-literacy",
			Activities: []contracts.CourseActivityRef{
				{Order: 1, Slug: "import-nav-demo"},
			},
		},
	}
	plan4, err := curriculum.PlanImport(ctx, q, external, opts)
	require.NoError(t, err)
	require.True(t, plan4.Valid, "errors: %v", plan4.Errors)

	// Unknown external slug fails plan.
	missing := &curriculum.ImportBundle{
		SchemaVersion: "1",
		SourceLabel:   "missing",
		Standards:     bundle.Standards,
		Course: &contracts.CourseDocument{
			SchemaVersion: "1",
			Slug:          "missing-course",
			Title:         "Missing",
			SubjectCode:   "digital-literacy",
			Activities: []contracts.CourseActivityRef{
				{Order: 1, Slug: "never-published-slug"},
			},
		},
	}
	plan5, err := curriculum.PlanImport(ctx, q, missing, opts)
	require.NoError(t, err)
	require.False(t, plan5.Valid)
	assert.NotEmpty(t, plan5.Errors)
}

func TestPublishCourseFromPath(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	root := repoRoot(t)

	// Publish standards + both activities needed by course file.
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	for _, slug := range []string{"basic-navigation", "file-organization"} {
		doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", slug, "activity.yaml"))
		require.NoError(t, err)
		_, _, err = curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
		require.NoError(t, err)
	}

	// Write a small course.json into temp dir.
	dir := t.TempDir()
	coursePath := filepath.Join(dir, "course.json")
	courseJSON := `{
  "schemaVersion": "1",
  "slug": "publish-course-path",
  "title": "Publish Course Path",
  "subjectCode": "digital-literacy",
  "parentDescription": "from path",
  "activities": [
    {"order": 1, "slug": "basic-navigation"},
    {"order": 2, "slug": "file-organization", "capstone": true}
  ],
  "remediation": [
    {"forActivity": "basic-navigation", "branchSlug": "file-organization", "kind": "remediation"}
  ]
}`
	require.NoError(t, os.WriteFile(coursePath, []byte(courseJSON), 0o644))

	// Zero time uses Now.
	res, err := curriculum.PublishCourse(ctx, q, coursePath, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "publish-course-path", res.Curriculum.Slug)
	assert.Equal(t, 2, res.Activities)
	assert.NotNil(t, res.Revision)
	assert.NotNil(t, res.Revision.PublishedAt)

	// Missing path errors.
	_, err = curriculum.PublishCourse(ctx, q, filepath.Join(dir, "nope.json"), time.Now().UTC())
	require.Error(t, err)

	// Second publish increments revision via path API.
	res2, err := curriculum.PublishCourse(ctx, q, coursePath, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, res.Revision.Revision+1, res2.Revision.Revision)

	// Remediations persisted for first revision.
	rems, err := repo.ListCurriculumRemediations(ctx, q, res.Revision.ID)
	require.NoError(t, err)
	require.Len(t, rems, 1)
}
