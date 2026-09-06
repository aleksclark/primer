package artifacts_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/artifacts"
	"github.com/stretchr/testify/require"
)

func TestFSRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := artifacts.NewFsStore(dir)
	require.NoError(t, err)
	defer s.Close()
	data := []byte("# Stored curriculum\n")
	require.NoError(t, s.Put(t.Context(), "exports/plan.md", data, "text/markdown"))
	// Reopen the backend: durability is not an in-memory map.
	other, err := artifacts.NewFsStore(dir)
	require.NoError(t, err)
	defer other.Close()
	r, err := other.Get(t.Context(), "exports/plan.md")
	require.NoError(t, err)
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	require.Equal(t, data, got)
	url, err := s.SignURL(t.Context(), "exports/plan.md", time.Minute)
	require.ErrorIs(t, err, artifacts.ErrSigningUnsupported)
	require.Empty(t, url)
	require.NoError(t, s.Delete(t.Context(), "exports/plan.md"))
	_, err = s.Get(t.Context(), "exports/plan.md")
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, s.Delete(t.Context(), "exports/plan.md"))
}

func TestFSConfinementAndCancellation(t *testing.T) {
	dir := t.TempDir()
	s, err := artifacts.NewFsStore(dir)
	require.NoError(t, err)
	defer s.Close()
	for _, key := range []string{"", "../escape", "/absolute", "a/../../escape", "a\\b", "obj:key"} {
		require.Error(t, s.Put(t.Context(), key, []byte("bad"), "text/plain"))
		_, err := s.Get(t.Context(), key)
		require.Error(t, err)
		require.Error(t, s.Delete(t.Context(), key))
		_, err = s.SignURL(t.Context(), key, time.Minute)
		require.Error(t, err)
	}
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(dir, "escape")))
	require.Error(t, s.Put(t.Context(), "escape/file", []byte("bad"), "text/plain"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, s.Put(ctx, "file", nil, "text/plain"), context.Canceled)
	_, err = s.Get(ctx, "file")
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, s.Delete(ctx, "file"), context.Canceled)
	_, err = s.SignURL(ctx, "file", time.Minute)
	require.ErrorIs(t, err, context.Canceled)
}
