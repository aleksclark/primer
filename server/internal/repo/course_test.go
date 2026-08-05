package repo_test

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
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestPublishCourseDocumentAndMembership(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
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

	res, err := repo.PublishCourseDocument(ctx, q, &contracts.CourseDocument{
		SchemaVersion:     "1",
		Slug:              "repo-course",
		Title:             "Repo Course",
		SubjectCode:       "digital-literacy",
		ParentDescription: "desc",
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "basic-navigation", Capstone: false},
			{Order: 2, Slug: "file-organization", Capstone: true, Continuity: &contracts.ContinuityPolicy{Mode: contracts.ContinuityFresh}},
		},
		Prerequisites: []contracts.CoursePrerequisite{{
			Activity: "file-organization", Requires: []string{"basic-navigation"}, Requirement: "completed",
		}},
		Gates: []contracts.CourseGate{{
			Activity: "file-organization", Kind: "parent_review", Description: "review",
		}},
		Remediation: []contracts.CourseRemediation{{
			ForActivity: "basic-navigation", BranchSlug: "file-organization", Kind: "remediation",
		}},
	}, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, "repo-course", res.Curriculum.Slug)
	assert.Equal(t, 2, res.Activities)

	acts, err := repo.ListCurriculumActivities(ctx, q, res.Revision.ID)
	require.NoError(t, err)
	require.Len(t, acts, 2)
	assert.Equal(t, 1, acts[0].Position)
	assert.NotNil(t, acts[0].ActivityRevisionID)
	assert.True(t, acts[1].Capstone)
	assert.Equal(t, contracts.ContinuityFresh, acts[1].ContinuityMode)

	prereqs, err := repo.ListCurriculumPrerequisites(ctx, q, res.Revision.ID)
	require.NoError(t, err)
	require.Len(t, prereqs, 1)

	// Second publish increments revision.
	res2, err := repo.PublishCourseDocument(ctx, q, &contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          "repo-course",
		Title:         "Repo Course v2",
		SubjectCode:   "digital-literacy",
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "basic-navigation"},
		},
	}, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, res.Revision.Revision+1, res2.Revision.Revision)
}

func TestEnrollPausePinAudit(t *testing.T) {
	t.Parallel()
	q := testutil.Tx(t)
	ctx := context.Background()
	student := factory.Student(t, q)
	ed := factory.Educator(t, q)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, _, err = curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	pub, err := repo.PublishCourseDocument(ctx, q, &contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          "enroll-course",
		Title:         "Enroll Course",
		SubjectCode:   "digital-literacy",
		Activities:    []contracts.CourseActivityRef{{Order: 1, Slug: "basic-navigation"}},
	}, time.Now().UTC())
	require.NoError(t, err)

	edID := ed.ID
	en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, student.ID, pub.Curriculum.ID, pub.Revision.ID, &edID, 3)
	require.NoError(t, err)
	assert.Equal(t, 3, en.Priority)

	_, err = repo.PinEnrollmentActivity(ctx, q, en.ID, "basic-navigation", "start here", &edID)
	require.NoError(t, err)
	_, err = repo.SetEnrollmentStatus(ctx, q, en.ID, "paused", &edID, "sick day")
	require.NoError(t, err)

	events, err := repo.ListEnrollmentAudit(ctx, q, en.ID, 20)
	require.NoError(t, err)
	actions := map[string]bool{}
	for _, e := range events {
		actions[e.Action] = true
	}
	assert.True(t, actions["enroll"])
	assert.True(t, actions["pin"])
	assert.True(t, actions["pause"])
}
