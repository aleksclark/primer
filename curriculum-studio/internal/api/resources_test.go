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

func TestResourceCatalogCRUDListFilterAndIsolation(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	path := "/studio/v1/workspaces/" + workspace.ID.String() + "/resources"

	book := doJSON(t, handler, http.MethodPost, path, map[string]any{
		"kind": "book", "title": "Arithmetic for Builders", "creators": []string{"A. Author", "B. Editor"},
		"locator": "https://example.com/arithmetic", "notes": "Use chapters 1-3.",
	}, token)
	require.Equal(t, http.StatusCreated, book.Code, book.Body.String())
	var created struct {
		ID, WorkspaceID, Kind, Title, Locator, Notes string
		Creators                                     []string
	}
	require.NoError(t, json.Unmarshal(book.Body.Bytes(), &created))
	assert.True(t, strings.HasPrefix(created.ID, "res_"))
	assert.Equal(t, "book", created.Kind)
	assert.Equal(t, "Arithmetic for Builders", created.Title)
	assert.Equal(t, []string{"A. Author", "B. Editor"}, created.Creators)
	assert.Equal(t, "Use chapters 1-3.", created.Notes)
	assert.True(t, strings.HasPrefix(created.WorkspaceID, "ws_"))

	video := doJSON(t, handler, http.MethodPost, path, map[string]any{
		"kind": "video", "title": "Fractions in Motion", "locator": "obj:media/fractions-v1",
	}, token)
	require.Equal(t, http.StatusCreated, video.Code, video.Body.String())

	list := doJSON(t, handler, http.MethodGet, path+"?kind=book&q=Arithmetic&limit=1", nil, token)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	var page struct {
		Items             []map[string]any `json:"items"`
		TotalCount, Limit int
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, 1, page.TotalCount)
	assert.Equal(t, "Arithmetic for Builders", page.Items[0]["title"])

	get := doJSON(t, handler, http.MethodGet, "/studio/v1/resources/"+created.ID, nil, token)
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())

	updated := doJSON(t, handler, http.MethodPatch, "/studio/v1/resources/"+created.ID, map[string]any{
		"kind": "document", "title": "Arithmetic Workbook", "creators": []string{"A. Author"}, "notes": "Updated",
	}, token)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Contains(t, updated.Body.String(), "Arithmetic Workbook")

	deleted := doJSON(t, handler, http.MethodDelete, "/studio/v1/resources/"+created.ID, nil, token)
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())
	gone := doJSON(t, handler, http.MethodGet, "/studio/v1/resources/"+created.ID, nil, token)
	assert.Equal(t, http.StatusNotFound, gone.Code)

	foreignSubject := uuid.New()
	foreignToken := mintHuman(t, key, now, foreignSubject)
	foreign := doJSON(t, handler, http.MethodGet, "/studio/v1/resources/"+created.ID, nil, foreignToken)
	assert.Equal(t, http.StatusForbidden, foreign.Code)
}

func TestResourceCatalogRejectsBlobAndBadReferences(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)
	subject := uuid.New()
	workspace := factory.Workspace(t, pool)
	factory.SeedMembership(t, pool, workspace.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleOwner)
	token := mintHuman(t, key, now, subject)
	path := "/studio/v1/workspaces/" + workspace.ID.String() + "/resources"

	badKind := doJSON(t, handler, http.MethodPost, path, map[string]any{"kind": "other", "title": "Nope"}, token)
	assert.Equal(t, http.StatusBadRequest, badKind.Code)
	badURL := doJSON(t, handler, http.MethodPost, path, map[string]any{"kind": "book", "title": "Bad", "locator": "file:///tmp/book.pdf"}, token)
	assert.Equal(t, http.StatusBadRequest, badURL.Code)

	largeNotes := strings.Repeat("x", 20*1024)
	tooLarge := doJSON(t, handler, http.MethodPost, path, map[string]any{"kind": "book", "title": "Too Large", "notes": largeNotes}, token)
	assert.Equal(t, http.StatusBadRequest, tooLarge.Code)

	// API payloads contain metadata references only. There is no content/blob
	// column, and a valid artifact locator is persisted as artifact_ref.
	valid := doJSON(t, handler, http.MethodPost, path, map[string]any{"kind": "document", "title": "Reference Only", "locator": "urn:primer:artifact:doc-1"}, token)
	require.Equal(t, http.StatusCreated, valid.Code, valid.Body.String())
	var response struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(valid.Body.Bytes(), &response))
	id := strings.TrimPrefix(response.ID, "res_")
	id = id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	var artifactRef string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT artifact_ref FROM curriculum_studio.resources WHERE id = $1`, id).Scan(&artifactRef))
	assert.Equal(t, "urn:primer:artifact:doc-1", artifactRef)
}
