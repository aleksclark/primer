package repo_test

import (
	"strings"
	"testing"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestP17CommentRepoAndDatabaseLengthBound(t *testing.T) {
	pool := testutil.DB(t)
	ctx := t.Context()
	ws, _, revision := planFixture(t, pool)
	comments := repo.NewCommentRepo(pool)
	for _, char := range []string{"x", "界", "🙂"} {
		t.Run(char, func(t *testing.T) {
			body := strings.Repeat(char, domain.MaxCommentLength)
			in := &domain.PlanComment{WorkspaceID: ws.ID, PlanRevisionID: revision.ID, NodeID: "out_bound", AuthorSubjectRef: "identity:test", Body: body}
			saved, err := comments.Create(ctx, in)
			require.NoError(t, err)
			require.Equal(t, body, saved.Body)
			in.Body = body + char
			_, err = comments.Create(ctx, in)
			require.ErrorIs(t, err, repo.ErrCheckViolation)
			// Bypassing HTTP/repo validation still reaches the database CHECK.
			_, err = pool.Exec(ctx, `INSERT INTO curriculum_studio.plan_comments(workspace_id,plan_revision_id,node_id,author_subject_ref,body) VALUES($1,$2,'out_bound','identity:test',$3)`, ws.ID, revision.ID, in.Body)
			require.ErrorIs(t, repo.MapError(err), repo.ErrCheckViolation)
			var characters, bytes int
			require.NoError(t, pool.QueryRow(ctx, `SELECT char_length(body),octet_length(body) FROM curriculum_studio.plan_comments WHERE id=$1`, saved.ID).Scan(&characters, &bytes))
			require.Equal(t, domain.MaxCommentLength, characters)
			require.Equal(t, len(body), bytes)
		})
	}
	var total int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.plan_comments WHERE plan_revision_id=$1`, revision.ID).Scan(&total))
	require.Equal(t, 3, total)
	for _, test := range []struct{ limit, offset, wantLimit, wantOffset int }{{0, -1, 25, 0}, {101, 0, 100, 0}, {1, 2, 1, 2}} {
		limit, offset := repo.CommentPageBounds(test.limit, test.offset)
		require.Equal(t, test.wantLimit, limit)
		require.Equal(t, test.wantOffset, offset)
		items, count, err := comments.ListPage(ctx, ws.ID, revision.ID, "", test.limit, test.offset)
		require.NoError(t, err)
		require.Equal(t, 3, count)
		require.Len(t, items, min(limit, 3-offset))
	}
}
