package curriculum_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
)

func sampleBundle() *curriculum.ImportBundle {
	doc := contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "import-nav-demo",
		Title:         "Import Nav Demo",
		Summary:       "tiny",
		Kind:          contracts.KindTerminal,
		SubjectCode:   "digital-literacy",
		Standards: []contracts.StandardRef{
			{Code: "PRIMER.DL.6.NAV.1", Role: contracts.StandardRolePrimary},
		},
		Content: contracts.ActivityContent{
			Objective:    "learn",
			Instructions: "do",
			Terminal: &contracts.TerminalContent{
				RuntimeProfile: contracts.RuntimeCoreutilsBasic,
				Fixtures: []contracts.FixtureEntry{
					{Path: "home", Type: contracts.FixtureDirectory},
					{Path: "home/a.txt", Type: contracts.FixtureFile, Content: "hi\n"},
				},
			},
			Tasks: []contracts.Task{{
				ID: "t1", Title: "T", Instructions: "go",
				Completion: contracts.CheckTree{CheckID: "c1"},
			}},
			Checks: []contracts.Check{{
				ID: "c1", Kind: contracts.CheckFileExists,
				Params: map[string]any{"path": "home/a.txt"},
			}},
			Progression: &contracts.ProgressionPolicy{AllowReset: true, ResumeFromTask: true},
		},
	}
	return &curriculum.ImportBundle{
		SchemaVersion: "1",
		SourceLabel:   "test-bundle",
		Standards: []curriculum.StandardSeed{{
			Code:        "PRIMER.DL.6.NAV.1",
			Source:      "custom",
			SubjectCode: "digital-literacy",
			Domain:      "navigation",
			Description: "nav",
		}},
		Activities: []contracts.ActivityDocument{doc},
		Course: &contracts.CourseDocument{
			SchemaVersion: "1",
			Slug:          "import-demo-course",
			Title:         "Import Demo",
			SubjectCode:   "digital-literacy",
			Activities: []contracts.CourseActivityRef{
				{Order: 1, Slug: "import-nav-demo"},
			},
		},
	}
}

func TestImportPlanAndApplyIdempotent(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	bundle := sampleBundle()
	opts := curriculum.ImportOptions{Now: time.Now().UTC(), ActorID: "", SourceLabel: "unit"}

	plan, err := curriculum.PlanImport(ctx, q, bundle, opts)
	require.NoError(t, err)
	require.True(t, plan.Valid, "errors: %v", plan.Errors)
	require.NotEmpty(t, plan.BundleDigest)
	assert.NotEmpty(t, plan.Actions)
	// Plan is read-only: no activities yet.
	page, err := repo.LearningActivities.List(ctx, q, repo.ListParams{Limit: 10, Filters: map[string]any{"slug": "import-nav-demo"}})
	require.NoError(t, err)
	assert.Equal(t, 0, page.TotalCount)

	manifest1, run1, err := curriculum.ApplyImport(ctx, q, bundle, plan.BundleDigest, opts)
	require.NoError(t, err)
	require.NotNil(t, manifest1)
	require.NotNil(t, run1)
	assert.Equal(t, plan.BundleDigest, manifest1.BundleDigest)
	assert.Empty(t, manifest1.EnrolledStudents)
	assert.Empty(t, manifest1.AssignedStudents)
	assert.False(t, manifest1.IdempotentReplay)

	// Activity published.
	page, err = repo.LearningActivities.List(ctx, q, repo.ListParams{Limit: 10, Filters: map[string]any{"slug": "import-nav-demo"}})
	require.NoError(t, err)
	require.Equal(t, 1, page.TotalCount)

	// Re-apply same digest is idempotent.
	manifest2, run2, err := curriculum.ApplyImport(ctx, q, bundle, plan.BundleDigest, opts)
	require.NoError(t, err)
	assert.True(t, manifest2.IdempotentReplay)
	assert.Equal(t, manifest1.BundleDigest, manifest2.BundleDigest)
	require.NotNil(t, run2)

	// Digest drift rejected.
	_, _, err = curriculum.ApplyImport(ctx, q, bundle, "0"+plan.BundleDigest[1:], opts)
	require.Error(t, err)
}

func TestImportRejectsUnauthorizedNamespace(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	bundle := sampleBundle()
	bundle.Standards[0].Code = "CCSS.MATH.1"
	bundle.Activities[0].Standards[0].Code = "CCSS.MATH.1"

	plan, err := curriculum.PlanImport(context.Background(), q, bundle, curriculum.ImportOptions{})
	require.NoError(t, err)
	require.False(t, plan.Valid)
	assert.NotEmpty(t, plan.Errors)
}

func TestImportRejectsOfficialSourceOverwrite(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	bundle := sampleBundle()
	bundle.Standards[0].Source = "ccss"
	bundle.Standards[0].Code = "PRIMER.DL.6.NAV.1"

	plan, err := curriculum.PlanImport(context.Background(), q, bundle, curriculum.ImportOptions{})
	require.NoError(t, err)
	require.False(t, plan.Valid)
}

func TestBundleDigestStable(t *testing.T) {
	t.Parallel()
	b := sampleBundle()
	d1, err := curriculum.BundleDigest(b)
	require.NoError(t, err)
	d2, err := curriculum.BundleDigest(b)
	require.NoError(t, err)
	assert.Equal(t, d1, d2)
}

func TestBuildBundleFromRepoStandards(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	b, err := curriculum.BuildBundleFromDirs(
		filepath.Join(root, "curriculum", "standards"),
		filepath.Join(root, "curriculum", "activities"),
		"",
		"repo-curriculum",
	)
	require.NoError(t, err)
	require.NotEmpty(t, b.Standards)
	require.NotEmpty(t, b.Activities)
	d, err := curriculum.BundleDigest(b)
	require.NoError(t, err)
	assert.Len(t, d, 64)
}
