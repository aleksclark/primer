package broker

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidationAndDefaults(t *testing.T) {
	t.Parallel()
	_, err := New(Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "socket path")

	dir := t.TempDir()
	_, err = New(Options{SocketPath: filepath.Join(dir, "s.sock")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db path")

	// Defaults: token file beside DB, AllowedGroup students, workspace root.
	srv, err := New(Options{
		SocketPath:   filepath.Join(dir, "b.sock"),
		DBPath:       filepath.Join(dir, "state.db"),
		BaseURL:      "http://127.0.0.1:1",
		SkipPeerCred: true,
		// TokenFile empty → default name
		// AllowedGroup empty → students
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })
	assert.Equal(t, "students", srv.opts.AllowedGroup)
	assert.NotEmpty(t, srv.opts.TokenFile)
	assert.NotEmpty(t, srv.opts.WorkspaceRoot)
	assert.Equal(t, "workstation", srv.opts.DeviceName)
}

func TestContainsTokenLeak(t *testing.T) {
	t.Parallel()
	assert.True(t, containsTokenLeak([]byte(`{"token":"x"}`)))
	assert.True(t, containsTokenLeak([]byte(`{"deviceToken":"x"}`)))
	assert.True(t, containsTokenLeak([]byte(`{"device_token":"x"}`)))
	assert.False(t, containsTokenLeak([]byte(`{"ok":true}`)))
}
