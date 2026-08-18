package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func TestStandardsCatalogImportListAndIsolation(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)

	body := map[string]any{
		"source":       "tennessee",
		"jurisdiction": "TN",
		"title":        "Grade 6 Mathematics",
		"standards": []map[string]any{
			{"code": "6.NS.A.1", "source": "tennessee", "subject": "math", "grade": "6", "description": "Interpret and compute quotients."},
			{"code": "6.NS.A.2", "source": "tennessee", "subject": "math", "grade": "6", "description": "Fluently divide multi-digit numbers.", "prerequisiteCodes": []string{"6.NS.A.1"}},
		},
	}
	path := "/studio/v1/workspaces/" + workspace.ID.String() + "/standards-catalogs"
	created := doJSON(t, handler, http.MethodPost, path, body, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var catalog struct {
		ID          string `json:"id"`
		Source      string `json:"source"`
		Title       string `json:"title"`
		WorkspaceID string `json:"workspaceId"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &catalog))
	assert.True(t, strings.HasPrefix(catalog.ID, "cat_"))
	assert.Equal(t, "tennessee", catalog.Source)
	assert.Equal(t, "Grade 6 Mathematics", catalog.Title)
	assert.True(t, strings.HasPrefix(catalog.WorkspaceID, "ws_"))

	listed := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces/"+workspace.ID.String()+"/standards-catalogs?limit=1", nil, token)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	var page struct {
		Items      []struct{ ID, Title string } `json:"items"`
		TotalCount int                          `json:"totalCount"`
		Limit      int                          `json:"limit"`
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, 1, page.TotalCount)
	assert.Equal(t, 1, page.Limit)

	standardsPath := "/studio/v1/standards-catalogs/" + catalog.ID + "/standards?q=divide&subject=math&grade=6"
	standards := doJSON(t, handler, http.MethodGet, standardsPath, nil, token)
	require.Equal(t, http.StatusOK, standards.Code, standards.Body.String())
	var standardPage struct {
		Items []struct {
			ID                string   `json:"id"`
			Code              string   `json:"code"`
			Source            string   `json:"source"`
			Description       string   `json:"description"`
			PrerequisiteCodes []string `json:"prerequisiteCodes"`
		} `json:"items"`
		TotalCount int `json:"totalCount"`
	}
	require.NoError(t, json.Unmarshal(standards.Body.Bytes(), &standardPage))
	require.Len(t, standardPage.Items, 1)
	assert.True(t, strings.HasPrefix(standardPage.Items[0].ID, "std_"))
	assert.Equal(t, "6.NS.A.2", standardPage.Items[0].Code)
	assert.Equal(t, "tennessee", standardPage.Items[0].Source)
	assert.Equal(t, 1, standardPage.TotalCount)
	assert.Equal(t, []string{"6.NS.A.1"}, standardPage.Items[0].PrerequisiteCodes)

	got := doJSON(t, handler, http.MethodGet, "/studio/v1/standards-catalogs/"+catalog.ID, nil, token)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())

	// Replaying the same import key is idempotent and does not duplicate rows.
	replay := doJSON(t, handler, http.MethodPost, path, body, token)
	require.Equal(t, http.StatusCreated, replay.Code, replay.Body.String())
	assert.JSONEq(t, created.Body.String(), replay.Body.String())
	var count int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.standard_frameworks WHERE workspace_id = $1`, workspace.ID).Scan(&count))
	assert.Equal(t, 1, count)
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.catalog_standards s JOIN curriculum_studio.standard_frameworks f ON f.id = s.framework_id WHERE f.id = (SELECT id FROM curriculum_studio.standard_frameworks WHERE workspace_id = $1)`, workspace.ID).Scan(&count))
	assert.Equal(t, 2, count)

	// A different subject cannot resolve the workspace-owned catalog.
	foreign := uuid.New()
	foreignToken := mintHuman(t, key, now, foreign)
	denied := doJSON(t, handler, http.MethodGet, "/studio/v1/standards-catalogs/"+catalog.ID, nil, foreignToken)
	assert.Equal(t, http.StatusForbidden, denied.Code)
}

func TestStandardsCatalogImportRollsBackPrerequisiteFailure(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleOwner)
	token := mintHuman(t, key, now, subject)

	body := map[string]any{
		"source": "custom", "title": "Cyclic Catalog",
		"standards": []map[string]any{
			{"code": "A", "source": "custom", "description": "A", "prerequisiteCodes": []string{"B"}},
			{"code": "B", "source": "custom", "description": "B", "prerequisiteCodes": []string{"A"}},
		},
	}
	resp := doJSON(t, handler, http.MethodPost, "/studio/v1/workspaces/"+workspace.ID.String()+"/standards-catalogs", body, token)
	assert.True(t, resp.Code == http.StatusBadRequest || resp.Code == http.StatusConflict, resp.Body.String())
	var count int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM curriculum_studio.standard_frameworks WHERE workspace_id = $1 AND name = 'Cyclic Catalog'`, workspace.ID).Scan(&count))
	assert.Equal(t, 0, count, "failed import must not leave a partial framework")
}
