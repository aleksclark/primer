package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
)

func TestP17S1CommentPersistsWithAuthor(t *testing.T) {
	t.Parallel()
	handler, _, key, now := newWorkspaceSuite(t)
	author := uuid.New()
	workspace := factory.Workspace(t, testutil.DB(t))
	factory.Membership(t, testutil.DB(t), func(m *domain.WorkspaceMembership) {
		m.WorkspaceID = workspace.ID
		m.SubjectRef = domain.HumanSubjectRef(author)
		m.Role = domain.MembershipRoleAuthor
		m.DisplayName = "Ada Author"
		m.Status = domain.MembershipStatusActive
	})
	token := mintHuman(t, key, now, author)

	created := doJSON(t, handler, http.MethodPost, "/studio/v1/workspaces/"+workspace.ID.String()+"/curricula", map[string]any{
		"name": "Collab comments", "template": "custom",
	}, token)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var curriculum struct{ ID string }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &curriculum))
	rev := doJSON(t, handler, http.MethodPost, "/studio/v1/curricula/"+curriculum.ID+"/revisions", map[string]any{}, token)
	require.Equal(t, http.StatusCreated, rev.Code, rev.Body.String())
	var revision struct{ ID string }
	require.NoError(t, json.Unmarshal(rev.Body.Bytes(), &revision))
	node := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/nodes", map[string]any{
		"kind": "outcome", "title": "Scale drawings",
	}, token)
	require.Equal(t, http.StatusCreated, node.Code, node.Body.String())
	var outcome struct{ ID string }
	require.NoError(t, json.Unmarshal(node.Body.Bytes(), &outcome))

	posted := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/comments", map[string]any{
		"nodeId": outcome.ID, "body": "Tighten the mastery criteria.",
	}, token)
	require.Equal(t, http.StatusCreated, posted.Code, posted.Body.String())
	var comment struct {
		ID         string
		AuthorName string `json:"authorName"`
		AuthorRef  string `json:"authorSubjectRef"`
		Body       string
		NodeID     string `json:"nodeId"`
	}
	require.NoError(t, json.Unmarshal(posted.Body.Bytes(), &comment))
	assert.Equal(t, "Tighten the mastery criteria.", comment.Body)
	assert.Equal(t, "Ada Author", comment.AuthorName)
	assert.Equal(t, domain.HumanSubjectRef(author), comment.AuthorRef)
	assert.Equal(t, outcome.ID, comment.NodeID)

	listed := doJSON(t, handler, http.MethodGet, "/studio/v1/revisions/"+revision.ID+"/comments?nodeId="+outcome.ID, nil, token)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	var page struct {
		Items []struct {
			AuthorName string `json:"authorName"`
			Body       string
		}
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Ada Author", page.Items[0].AuthorName)
	assert.Equal(t, "Tighten the mastery criteria.", page.Items[0].Body)

	reviewer := uuid.New()
	factory.Membership(t, testutil.DB(t), func(m *domain.WorkspaceMembership) {
		m.WorkspaceID = workspace.ID
		m.SubjectRef = domain.HumanSubjectRef(reviewer)
		m.Role = domain.MembershipRoleReviewer
		m.DisplayName = "Ruth Reviewer"
	})
	reviewerToken := mintHuman(t, key, now, reviewer)
	// A second principal reads the committed note, not a response fixture.
	read := doJSON(t, handler, http.MethodGet, "/studio/v1/revisions/"+revision.ID+"/comments", nil, reviewerToken)
	require.Equal(t, http.StatusOK, read.Code, read.Body.String())
	assert.Contains(t, read.Body.String(), "Ada Author")
	path := "/studio/v1/revisions/" + revision.ID + "/approval"
	pending := collabApproval(t, handler, reviewerToken, revision.ID)
	require.Equal(t, "pending", pending.State)
	require.Len(t, pending.ContentFingerprint, 32)
	body := map[string]any{"decision": "approved", "contentFingerprint": pending.ContentFingerprint}
	denied := doJSON(t, handler, http.MethodPost, path, body, token)
	require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
	missing := doJSON(t, handler, http.MethodPost, path, map[string]any{"decision": "approved"}, reviewerToken)
	require.Equal(t, http.StatusUnprocessableEntity, missing.Code, missing.Body.String())
	approved := doJSON(t, handler, http.MethodPost, path, body, reviewerToken)
	require.Equal(t, http.StatusOK, approved.Code, approved.Body.String())
	got := collabApproval(t, handler, token, revision.ID)
	assert.Equal(t, "approved", got.State)
	assert.Equal(t, "Ruth Reviewer", got.ReviewerName)
	assert.Equal(t, domain.HumanSubjectRef(reviewer), got.ReviewerSubjectRef)
	var storedName, storedState string
	require.NoError(t, testutil.DB(t).QueryRow(t.Context(), `SELECT reviewer_display_name,status FROM curriculum_studio.plan_approvals WHERE plan_revision_id=$1`, decodePrefixed(t, revision.ID, "prev_")).Scan(&storedName, &storedState))
	assert.Equal(t, "Ruth Reviewer", storedName)
	assert.Equal(t, "approved", storedState)

	// Mutating the draft invalidates the decision, and a stale browser cannot
	// approve the newer content with its older review fingerprint.
	collabNode(t, handler, token, revision.ID, "outcome", "New mastery target", "new-target")
	fresh := collabApproval(t, handler, reviewerToken, revision.ID)
	require.Equal(t, "pending", fresh.State)
	require.NotEqual(t, pending.ContentFingerprint, fresh.ContentFingerprint)
	stale := doJSON(t, handler, http.MethodPost, path, body, reviewerToken)
	require.Equal(t, http.StatusConflict, stale.Code, stale.Body.String())
	body["contentFingerprint"] = fresh.ContentFingerprint
	body["decision"] = "rejected"
	rejected := doJSON(t, handler, http.MethodPost, path, body, reviewerToken)
	require.Equal(t, http.StatusOK, rejected.Code, rejected.Body.String())
	assert.Equal(t, "rejected", collabApproval(t, handler, token, revision.ID).State)

	second := doJSON(t, handler, http.MethodPost, "/studio/v1/revisions/"+revision.ID+"/comments", map[string]any{"nodeId": outcome.ID, "body": "Please add evidence."}, reviewerToken)
	require.Equal(t, http.StatusCreated, second.Code, second.Body.String())
	paged := doJSON(t, handler, http.MethodGet, "/studio/v1/revisions/"+revision.ID+"/comments?limit=1&offset=1", nil, token)
	require.Equal(t, http.StatusOK, paged.Code, paged.Body.String())
	var comments struct {
		Items      []api.PlanComment
		TotalCount int
	}
	require.NoError(t, json.Unmarshal(paged.Body.Bytes(), &comments))
	assert.Equal(t, 2, comments.TotalCount)
	require.Len(t, comments.Items, 1)
	assert.Equal(t, "Ruth Reviewer", comments.Items[0].AuthorName)
	empty := doJSON(t, handler, http.MethodGet, "/studio/v1/revisions/"+revision.ID+"/comments?limit=1&offset=100", nil, token)
	require.Equal(t, http.StatusOK, empty.Code, empty.Body.String())
	require.NoError(t, json.Unmarshal(empty.Body.Bytes(), &comments))
	assert.Equal(t, 2, comments.TotalCount)
	assert.Empty(t, comments.Items)
}
