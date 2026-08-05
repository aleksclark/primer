package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func TestParentSessionCreateLookupAndDelete(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	ed := factory.EducatorWithPassword(t, q, "pw-ok", factory.Override{
		"email": "parent-sess@example.com",
		"role":  "parent",
	})
	tok, sess, err := repo.CreateParentSession(ctx, q, ed.ID, time.Now().UTC())
	require.NoError(t, err)
	require.NotEmpty(t, tok)
	require.NotNil(t, sess)

	gotSess, gotEd, err := repo.ParentSessionByToken(ctx, q, tok, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, gotSess)
	assert.Equal(t, ed.ID, gotEd.ID)
	assert.Equal(t, sess.ID, gotSess.ID)

	require.NoError(t, repo.DeleteParentSession(ctx, q, tok))
	_, _, err = repo.ParentSessionByToken(ctx, q, tok, time.Now().UTC())
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestListStudentWorkCursorAndAssignments(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()
	student := factory.Student(t, q, factory.Override{"first_name": "Cursor"})
	items, cursor, err := repo.ListStudentWork(ctx, q, student.ID, "", 10)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Empty(t, cursor)

	_, _, err = repo.ListStudentWork(ctx, q, student.ID, "bad-cursor", 10)
	require.Error(t, err)
	var br repo.ErrBadRequest
	require.ErrorAs(t, err, &br)

	_, _, err = repo.ListStudentWork(ctx, q, student.ID, "not-a-time|uuid", 10)
	require.Error(t, err)

	list, err := repo.ListAssignmentsForStudent(ctx, q, student.ID)
	require.NoError(t, err)
	assert.Empty(t, list)
}
