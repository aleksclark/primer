package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/repo"
)

func TestSessionCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)

	s, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: "ns-sess-create",
		Profile:        "tutor",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, s.ID)
	assert.Equal(t, "ns-sess-create", s.OwnerNamespace)
	assert.Equal(t, "tutor", s.Profile)
	assert.Equal(t, "open", string(s.Status))
	assert.Equal(t, int64(1), s.Revision)
	assert.Nil(t, s.CallerContext)
}

func TestSessionGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)

	created, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: "ns-sess-get",
		Profile:        "admin",
	})
	require.NoError(t, err)

	got, err := repo.Sessions.Get(ctx, p, created.ID, "ns-sess-get")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "admin", got.Profile)
}

func TestSessionGetNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)

	_, err := repo.Sessions.Get(ctx, p, "00000000-0000-0000-0000-000000000000", "ns-miss")
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestSessionGetNamespaceIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)

	s, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: "ns-sess-iso-owner",
		Profile:        "tutor",
	})
	require.NoError(t, err)

	// A different namespace cannot retrieve the session.
	_, err = repo.Sessions.Get(ctx, p, s.ID, "ns-sess-iso-other")
	require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestSessionClose(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)

	s, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: "ns-sess-close",
		Profile:        "tutor",
	})
	require.NoError(t, err)

	closed, err := repo.Sessions.Close(ctx, p, s.ID, "ns-sess-close", s.Revision)
	require.NoError(t, err)
	assert.Equal(t, "closed", string(closed.Status))
	assert.Equal(t, int64(2), closed.Revision)
}

func TestSessionCloseIdempotentOnAlreadyClosed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)

	s, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: "ns-sess-close-idem",
		Profile:        "tutor",
	})
	require.NoError(t, err)

	_, err = repo.Sessions.Close(ctx, p, s.ID, "ns-sess-close-idem", s.Revision)
	require.NoError(t, err)

	// Second close → InvalidTransition (already closed).
	_, err = repo.Sessions.Close(ctx, p, s.ID, "ns-sess-close-idem", 2)
	require.ErrorIs(t, err, repo.ErrInvalidTransition)
}

func TestSessionCloseStaleVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool(t)

	s, err := repo.Sessions.Create(ctx, p, repo.CreateSessionCmd{
		OwnerNamespace: "ns-sess-stale",
		Profile:        "tutor",
	})
	require.NoError(t, err)

	_, err = repo.Sessions.Close(ctx, p, s.ID, "ns-sess-stale", s.Revision+99)
	require.ErrorIs(t, err, repo.ErrStaleVersion)
}
