package broker

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupHasUIDAndChownSocketHelpers(t *testing.T) {
	t.Parallel()
	// Unknown group → false.
	assert.False(t, groupHasUID("definitely-not-a-real-group-xyz", 0))

	// Primary group of current user should contain current uid.
	u, err := user.Current()
	require.NoError(t, err)
	g, err := user.LookupGroupId(u.Gid)
	require.NoError(t, err)
	assert.True(t, groupHasUID(g.Name, uint32(os.Getuid())))

	// chown on a regular file (not live socket) — may fail without privileges but must not panic.
	path := filepath.Join(t.TempDir(), "sockfile")
	require.NoError(t, os.WriteFile(path, []byte{}, 0o666))
	_ = chownSocketGroup(path, g.Name)
	_ = chownSocketGroup(path, "-")
	_ = chownSocketGroup(path, "")
	_ = chownSocketGroup(path, "definitely-not-a-real-group-xyz")
}

func TestAuthorizeWithAllowedGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	u, err := user.Current()
	require.NoError(t, err)
	g, err := user.LookupGroupId(u.Gid)
	require.NoError(t, err)

	srv, err := New(Options{
		SocketPath: filepath.Join(dir, "g.sock"), DBPath: filepath.Join(dir, "g.db"),
		BaseURL: "http://127.0.0.1:1", SkipPeerCred: true, AllowedGroup: g.Name,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	// Force non-self, non-root uid through group path.
	// Same uid short-circuits before group — use different uid that is still in group (hard).
	// At least exercise AllowedGroup != "-" branch with non-matching uid.
	assert.False(t, srv.authorize(&PeerCred{UID: 999991}))
}
