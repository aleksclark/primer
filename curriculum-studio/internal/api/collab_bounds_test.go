package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestP17CommentHTTPCodePointBoundAndPageMetadata(t *testing.T) {
	h, _, key, now := newWorkspaceSuite(t)
	pool := testutil.DB(t)
	ws := factory.Workspace(t, pool)
	subject := uuid.New()
	factory.SeedMembership(t, pool, ws.ID, domain.HumanSubjectRef(subject), domain.MembershipRoleAuthor)
	token := mintHuman(t, key, now, subject)
	cur := collabCurriculum(t, h, token, ws.ID.String(), map[string]any{"name": "Comment bounds"})
	rev := collabRevision(t, h, token, cur.ID)
	node := collabNode(t, h, token, rev.ID, "outcome", "Unicode precision", "code")
	path := "/studio/v1/revisions/" + rev.ID + "/comments"
	for _, char := range []string{"x", "界", "🙂"} {
		valid := doJSON(t, h, http.MethodPost, path, map[string]any{"nodeId": node.ID, "body": strings.Repeat(char, domain.MaxCommentLength)}, token)
		require.Equal(t, http.StatusCreated, valid.Code, "valid %s code-point body rejected", char)
		invalid := doJSON(t, h, http.MethodPost, path, map[string]any{"nodeId": node.ID, "body": strings.Repeat(char, domain.MaxCommentLength+1)}, token)
		require.Equal(t, http.StatusUnprocessableEntity, invalid.Code, "oversized %s comment accepted", char)
	}
	for _, test := range []struct {
		query                string
		limit, offset, count int
	}{{"", 25, 0, 3}, {"?limit=1&offset=1", 1, 1, 1}, {"?limit=100&offset=10", 100, 10, 0}} {
		response := doJSON(t, h, http.MethodGet, path+test.query, nil, token)
		require.Equal(t, http.StatusOK, response.Code)
		var page struct {
			Items                     []json.RawMessage
			TotalCount, Limit, Offset int
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
		require.Equal(t, 3, page.TotalCount)
		require.Equal(t, test.limit, page.Limit)
		require.Equal(t, test.offset, page.Offset)
		require.Len(t, page.Items, test.count)
	}
	tooMany := doJSON(t, h, http.MethodGet, path+"?limit=101", nil, token)
	require.Equal(t, http.StatusUnprocessableEntity, tooMany.Code)
}
