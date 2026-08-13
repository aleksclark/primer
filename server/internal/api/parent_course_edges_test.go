package api_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func parentAuthHeader(token string) string {
	// Literal Bearer scheme required by BearerToken; keep scheme separate so
	// tooling redaction cannot rewrite the source string to "***".
	return "Authorization: " + "Bearer " + token
}

func TestParentCoursePublishPathListResumeProgress(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "course-path-edges"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "course-path@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q)

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	token := decode[objMap](t, resp.Body.Bytes())["token"].(string)
	auth := parentAuthHeader(token)

	// Empty path → 400
	resp = h.Post("/courses/publish-path", objMap{"path": ""}, auth)
	assert.True(t, resp.Code == http.StatusBadRequest || resp.Code == http.StatusUnprocessableEntity, resp.Body.String())

	// Missing file → 400
	resp = h.Post("/courses/publish-path", objMap{"path": filepath.Join(t.TempDir(), "no-such-course.json")}, auth)
	require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())

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

	// Server-local course.json via absolute path.
	coursePath := filepath.Join(t.TempDir(), "mini-course.json")
	courseJSON := `{
  "schemaVersion": "1",
  "slug": "path-publish-course",
  "title": "Path Publish Course",
  "subjectCode": "digital-literacy",
  "activities": [
    {"order": 1, "slug": "basic-navigation"},
    {"order": 2, "slug": "file-organization"}
  ]
}`
	require.NoError(t, os.WriteFile(coursePath, []byte(courseJSON), 0o644))

	resp = h.Post("/courses/publish-path", objMap{"path": coursePath}, auth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	pub := decode[objMap](t, resp.Body.Bytes())
	curr := pub["curriculum"].(map[string]any)
	rev := pub["revision"].(map[string]any)
	require.Equal(t, "path-publish-course", curr["slug"])
	require.NotEmpty(t, rev["id"])
	assert.Equal(t, float64(2), pub["activities"])

	// List curriculum revisions includes the new one.
	resp = h.Get("/curriculum-revisions?limit=50", auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	page := decode[struct {
		Items []objMap `json:"items"`
		Total int      `json:"totalCount"`
	}](t, resp.Body.Bytes())
	require.GreaterOrEqual(t, page.Total, 1)
	found := false
	for _, it := range page.Items {
		if it["id"] == rev["id"] {
			found = true
			break
		}
	}
	assert.True(t, found, "published revision must appear in list")

	// Enroll by curriculum slug (latest revision).
	resp = h.Post("/students/"+student.ID+"/enrollments", objMap{
		"curriculumSlug": "path-publish-course",
		"priority":       3,
	}, auth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	en := decode[domain.Enrollment](t, resp.Body.Bytes())
	require.Equal(t, "active", en.Status)
	require.NotNil(t, en.CurriculumRevisionID)

	// Pause then resume.
	resp = h.Post("/enrollments/"+en.ID+"/pause", objMap{"reason": "break"}, auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	assert.Equal(t, "paused", decode[domain.Enrollment](t, resp.Body.Bytes()).Status)

	resp = h.Post("/enrollments/"+en.ID+"/resume", objMap{"reason": "back"}, auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	assert.Equal(t, "active", decode[domain.Enrollment](t, resp.Body.Bytes()).Status)

	// Progress alias of eligibility.
	resp = h.Get("/enrollments/"+en.ID+"/progress", auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	prog := decode[objMap](t, resp.Body.Bytes())
	require.Contains(t, prog, "eligible")

	// Enroll missing identifiers → 400
	resp = h.Post("/students/"+student.ID+"/enrollments", objMap{}, auth)
	require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())

	// Unauthenticated list denied.
	resp = h.Get("/curriculum-revisions")
	assert.True(t, resp.Code == http.StatusUnauthorized || resp.Code == http.StatusForbidden, resp.Body.String())
}

func TestParentCourseEnrollByCurriculumIDOnly(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "course-curr-id"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "course-curr-id@example.com",
	})
	student := factory.Student(t, q)
	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code)
	auth := parentAuthHeader(decode[objMap](t, resp.Body.Bytes())["token"].(string))

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

	resp = h.Post("/courses/publish", objMap{
		"document": contracts.CourseDocument{
			SchemaVersion: "1",
			Slug:          "curr-id-course",
			Title:         "Curr ID Course",
			SubjectCode:   "digital-literacy",
			Activities:    []contracts.CourseActivityRef{{Order: 1, Slug: "basic-navigation"}},
		},
	}, auth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	pub := decode[objMap](t, resp.Body.Bytes())
	curriculumID := pub["curriculum"].(map[string]any)["id"].(string)

	// curriculumId alone resolves latest revision.
	resp = h.Post("/students/"+student.ID+"/enrollments", objMap{
		"curriculumId": curriculumID,
	}, auth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	en := decode[domain.Enrollment](t, resp.Body.Bytes())
	require.Equal(t, curriculumID, en.CurriculumID)
	require.NotNil(t, en.CurriculumRevisionID)
}
