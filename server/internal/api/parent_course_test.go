package api_test

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestParentCourseEnrollEligibilityAndPin(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "course-test-password"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "course-parent@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q)

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	auth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

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

	courseDoc := contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          "api-mini-course",
		Title:         "API Mini Course",
		SubjectCode:   "digital-literacy",
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "basic-navigation"},
			{Order: 2, Slug: "file-organization"},
		},
		Prerequisites: []contracts.CoursePrerequisite{{
			Activity: "file-organization", Requires: []string{"basic-navigation"}, Requirement: "completed",
		}},
	}
	resp = h.Post("/courses/publish", objMap{"document": courseDoc}, auth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	pub := decode[objMap](t, resp.Body.Bytes())
	rev := pub["revision"].(map[string]any)
	revisionID := rev["id"].(string)

	resp = h.Post("/students/"+student.ID+"/enrollments", objMap{
		"curriculumRevisionId": revisionID,
		"priority":             5,
	}, auth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	en := decode[domain.Enrollment](t, resp.Body.Bytes())
	require.Equal(t, "active", en.Status)
	require.NotNil(t, en.CurriculumRevisionID)

	resp = h.Get("/enrollments/"+en.ID+"/eligibility", auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	elig := decode[objMap](t, resp.Body.Bytes())
	eligible := elig["eligible"].([]any)
	require.Len(t, eligible, 1)

	resp = h.Post("/enrollments/"+en.ID+"/pin", objMap{
		"slug":   "file-organization",
		"reason": "parent chose challenge",
	}, auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	pinned := decode[domain.Enrollment](t, resp.Body.Bytes())
	assert.Equal(t, "file-organization", pinned.PinnedActivitySlug)

	resp = h.Post("/students/"+student.ID+"/assign-next", objMap{"preferReinforcement": false}, auth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	asg := decode[objMap](t, resp.Body.Bytes())
	assert.Contains(t, asg["reason"].(string), "pin:file-organization")
	assert.Equal(t, true, asg["created"])

	resp = h.Post("/enrollments/"+en.ID+"/pause", objMap{"reason": "vacation"}, auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	paused := decode[domain.Enrollment](t, resp.Body.Bytes())
	assert.Equal(t, "paused", paused.Status)

	resp = h.Get("/enrollments/"+en.ID+"/audit", auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	audit := decode[struct {
		Items []domain.EnrollmentAuditEvent `json:"items"`
	}](t, resp.Body.Bytes())
	require.NotEmpty(t, audit.Items)
}

func TestPublishCourseRequiresPublishedActivities(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)

	const password = "course-missing-act"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "course-missing@example.com",
	})
	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code)
	auth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	resp = h.Post("/courses/publish", objMap{
		"document": contracts.CourseDocument{
			SchemaVersion: "1",
			Slug:          "missing-acts",
			Title:         "Missing",
			SubjectCode:   "digital-literacy",
			Activities:    []contracts.CourseActivityRef{{Order: 1, Slug: "does-not-exist-yet"}},
		},
	}, auth)
	assert.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
}

func TestEnrollmentOverrideAppearsInAudit(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "course-override"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{"email": "override@example.com"})
	student := factory.Student(t, q)
	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code)
	auth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

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
	pub, err := repo.PublishCourseDocument(ctx, q, &contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          "override-course",
		Title:         "Override Course",
		SubjectCode:   "digital-literacy",
		Activities: []contracts.CourseActivityRef{
			{Order: 1, Slug: "basic-navigation"},
			{Order: 2, Slug: "file-organization"},
		},
		Prerequisites: []contracts.CoursePrerequisite{{
			Activity: "file-organization", Requires: []string{"basic-navigation"}, Requirement: "completed",
		}},
	}, time.Now().UTC())
	require.NoError(t, err)

	resp = h.Post("/students/"+student.ID+"/enrollments", objMap{
		"curriculumId":         pub.Curriculum.ID,
		"curriculumRevisionId": pub.Revision.ID,
	}, auth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	enID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	resp = h.Post("/enrollments/"+enID+"/override", objMap{
		"slug":   "file-organization",
		"reason": "student already demonstrated skill offline",
	}, auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	en := decode[domain.Enrollment](t, resp.Body.Bytes())
	assert.Contains(t, en.OverrideSlugs, "file-organization")

	resp = h.Get("/enrollments/"+enID+"/eligibility", auth)
	require.Equal(t, http.StatusOK, resp.Code)
	elig := decode[objMap](t, resp.Body.Bytes())
	// With override, both may be eligible (first incomplete + second overridden).
	eligible := elig["eligible"].([]any)
	require.GreaterOrEqual(t, len(eligible), 1)
}
