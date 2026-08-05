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
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

// TestParentLearningActivityLifecycle covers draft create → list → publish revision →
// assign with reason/priority → list revisions/assignments via parent-learning routes.
func TestParentLearningActivityLifecycle(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "parent-act-lifecycle@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Life", "last_name": "Cycle"})

	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	parentAuth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(repoRoot, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	// Create draft activity via parent API.
	resp = h.Post("/learning-activities", objMap{
		"slug":    "parent-draft-nav",
		"title":   "Parent Draft Nav",
		"summary": "drafted in test",
		"kind":    "terminal",
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	act := decode[domain.LearningActivity](t, resp.Body.Bytes())
	assert.Equal(t, "parent-draft-nav", act.Slug)
	assert.Equal(t, "terminal", act.Kind)
	assert.Equal(t, "draft", act.Status)

	// List activities (and filter bad filter to exercise 400).
	resp = h.Get("/learning-activities?limit=10", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	listed := decode[struct {
		Items []domain.LearningActivity `json:"items"`
	}](t, resp.Body.Bytes())
	found := false
	for _, a := range listed.Items {
		if a.ID == act.ID {
			found = true
		}
	}
	require.True(t, found)

	resp = h.Get("/learning-activities?filter=not-a-filter", parentAuth)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	// Publish revision with standards + default schema version.
	content := objMap{
		"objective":    "Learn parent-published activity",
		"instructions": "Create home/done.txt",
		"terminal": objMap{
			"runtimeProfile": "coreutils-basic",
			"initialCwd":     ".",
			"fixtures": []objMap{
				{"path": "home", "type": "directory"},
			},
		},
		"tasks": []objMap{
			{
				"id": "t1", "title": "Write file", "instructions": "touch home/done.txt",
				"completion": objMap{"checkId": "done"},
			},
		},
		"checks": []objMap{
			{"id": "done", "kind": "file_exists", "params": objMap{"path": "home/done.txt"}},
		},
	}
	resp = h.Post("/learning-activities/"+act.ID+"/revisions", objMap{
		"content": content,
		"standards": []objMap{
			{"code": "PRIMER.DL.6.NAV.1", "role": contracts.StandardRolePrimary},
		},
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	rev := decode[domain.LearningActivityRevision](t, resp.Body.Bytes())
	require.NotEmpty(t, rev.ID)
	assert.Equal(t, act.ID, rev.ActivityID)
	assert.Equal(t, contracts.SchemaVersion, rev.SchemaVersion)

	// Unknown standard code → 422
	resp = h.Post("/learning-activities/"+act.ID+"/revisions", objMap{
		"content":   content,
		"standards": []objMap{{"code": "NO.SUCH.STANDARD", "role": "primary"}},
	}, parentAuth)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code, resp.Body.String())

	// List revisions
	resp = h.Get("/learning-activities/"+act.ID+"/revisions", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	revs := decode[struct {
		Items []domain.LearningActivityRevision `json:"items"`
		Total int                               `json:"totalCount"`
	}](t, resp.Body.Bytes())
	require.GreaterOrEqual(t, revs.Total, 1)
	require.NotEmpty(t, revs.Items)

	// Assign with priority + reason
	prio := 7
	resp = h.Post("/assignments", objMap{
		"studentId":          student.ID,
		"activityRevisionId": rev.ID,
		"priority":           prio,
		"reason":             "lifecycle-test",
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	asg := decode[domain.StudentAssignment](t, resp.Body.Bytes())
	assert.Equal(t, student.ID, asg.StudentID)
	assert.Equal(t, rev.ID, asg.ActivityRevisionID)
	assert.Equal(t, 7, asg.Priority)
	assert.Equal(t, "lifecycle-test", asg.Reason)
	assert.Equal(t, domain.AssignmentAvailable, asg.State)

	// Student assignments list
	resp = h.Get("/students/"+student.ID+"/assignments", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	asgList := decode[struct {
		Items []domain.StudentAssignment `json:"items"`
	}](t, resp.Body.Bytes())
	require.NotEmpty(t, asgList.Items)

	// Global assignments list + bad filter
	resp = h.Get("/assignments?limit=20", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	resp = h.Get("/assignments?filter=bad", parentAuth)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	// Learning sessions list (empty ok) + bad filter
	resp = h.Get("/learning-sessions?limit=5", parentAuth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	resp = h.Get("/learning-sessions?filter=bad", parentAuth)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	// Pairing code for missing student → 404
	resp = h.Post("/pairing-codes", objMap{"studentId": "00000000-0000-0000-0000-000000000099"}, parentAuth)
	assert.Equal(t, http.StatusNotFound, resp.Code)

	// Assign missing student / missing revision
	resp = h.Post("/assignments", objMap{
		"studentId":          "00000000-0000-0000-0000-000000000099",
		"activityRevisionId": rev.ID,
	}, parentAuth)
	assert.Equal(t, http.StatusNotFound, resp.Code)
	resp = h.Post("/assignments", objMap{
		"studentId":          student.ID,
		"activityRevisionId": "00000000-0000-0000-0000-000000000099",
	}, parentAuth)
	assert.Equal(t, http.StatusNotFound, resp.Code)

	// Unauthenticated parent routes reject
	resp = h.Get("/learning-activities")
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
	resp = h.Post("/learning-activities", objMap{"slug": "x", "title": "y", "kind": "terminal"})
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestParentPublishRevisionExplicitSchemaAndDuplicateStandard(t *testing.T) {
	t.Parallel()
	h, q := testutil.API(t)
	ctx := context.Background()

	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "parent-rev-schema@example.com",
		"role":  "parent",
	})
	resp := h.Post("/auth/login", objMap{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code)
	parentAuth := "Authorization: Bearer " + decode[objMap](t, resp.Body.Bytes())["token"].(string)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(repoRoot, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)

	resp = h.Post("/learning-activities", objMap{
		"slug": "schema-act", "title": "Schema", "kind": "terminal",
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	actID := decode[objMap](t, resp.Body.Bytes())["id"].(string)

	content := objMap{
		"objective": "o", "instructions": "i",
		"terminal": objMap{"runtimeProfile": "coreutils-basic", "initialCwd": ".", "fixtures": []objMap{{"path": "home", "type": "directory"}}},
		"tasks":    []objMap{{"id": "t", "title": "T", "instructions": "G", "completion": objMap{"checkId": "c"}}},
		"checks":   []objMap{{"id": "c", "kind": "file_exists", "params": objMap{"path": "home"}}},
	}
	// Duplicate standard codes in one request should still resolve once.
	resp = h.Post("/learning-activities/"+actID+"/revisions", objMap{
		"schemaVersion": "1",
		"content":       content,
		"standards": []objMap{
			{"code": "PRIMER.DL.6.NAV.1", "role": "primary"},
			{"code": "PRIMER.DL.6.NAV.1", "role": "reinforcement"},
		},
	}, parentAuth)
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	rev := decode[domain.LearningActivityRevision](t, resp.Body.Bytes())
	assert.Equal(t, "1", rev.SchemaVersion)
}
