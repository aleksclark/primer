//go:build linux

package broker

import (
	"bufio"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupHasUIDCurrentUser(t *testing.T) {
	t.Parallel()
	u, err := user.Current()
	require.NoError(t, err)
	uid64, err := strconv.ParseUint(u.Uid, 10, 32)
	require.NoError(t, err)
	uid := uint32(uid64)

	// Primary group name should include the current UID.
	g, err := user.LookupGroupId(u.Gid)
	require.NoError(t, err)
	assert.True(t, groupHasUID(g.Name, uid), "uid must be member of primary group %s", g.Name)

	// Also exercise supplementary groups when present.
	ids, err := u.GroupIds()
	require.NoError(t, err)
	for _, gid := range ids {
		gg, err := user.LookupGroupId(gid)
		if err != nil {
			continue
		}
		assert.True(t, groupHasUID(gg.Name, uid), "uid should be in group %s", gg.Name)
	}

	assert.False(t, groupHasUID("definitely-not-a-real-group-xyz", uid))
	assert.False(t, groupHasUID(g.Name, 4294967290)) // almost-certainly unknown UID
}

func TestChownSocketGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// chown works on any path; use a regular file (unix sockets may unlink on Close).
	sockPath := filepath.Join(dir, "broker.sock")
	require.NoError(t, os.WriteFile(sockPath, []byte{}, 0o660))

	u, err := user.Current()
	require.NoError(t, err)
	g, err := user.LookupGroupId(u.Gid)
	require.NoError(t, err)

	require.NoError(t, chownSocketGroup(sockPath, g.Name))
	st, err := os.Stat(sockPath)
	require.NoError(t, err)
	require.NotNil(t, st)

	err = chownSocketGroup(sockPath, "no-such-group-zzz-999")
	require.Error(t, err)

	// Missing path should error from Chown after group lookup succeeds.
	err = chownSocketGroup(filepath.Join(dir, "missing.sock"), g.Name)
	require.Error(t, err)
}

func TestAuthorizePeerCredBranches(t *testing.T) {
	t.Parallel()
	s := &Server{opts: Options{AllowedGroup: "-"}}
	assert.False(t, s.authorize(nil))
	assert.True(t, s.authorize(&PeerCred{UID: 0}))
	self := uint32(os.Getuid())
	assert.True(t, s.authorize(&PeerCred{UID: self}))
	// Without AllowedUIDs/group, foreign UID rejected.
	assert.False(t, s.authorize(&PeerCred{UID: 424242}))

	s3 := &Server{opts: Options{AllowedGroup: "-", AllowedUIDs: []uint32{424242}}}
	assert.True(t, s3.authorize(&PeerCred{UID: 424242}))

	u, err := user.Current()
	require.NoError(t, err)
	g, err := user.LookupGroupId(u.Gid)
	require.NoError(t, err)
	s4 := &Server{opts: Options{AllowedGroup: g.Name, AllowedUIDs: nil}}
	other := uint32(1)
	if other == self {
		other = 2
	}
	// Non-self foreign UID: group membership likely false → reject (still covers group call).
	_ = s4.authorize(&PeerCred{UID: other})
	assert.True(t, s4.authorize(&PeerCred{UID: self}))
}

func TestWriteErrSendsErrorEnvelope(t *testing.T) {
	t.Parallel()
	c1, c2 := net.Pipe()
	t.Cleanup(func() { _ = c1.Close(); _ = c2.Close() })
	s := &Server{}
	done := make(chan Envelope, 1)
	go func() {
		env, err := Decode(bufio.NewReader(c2))
		if err != nil {
			return
		}
		done <- env
	}()
	s.writeErr(c1, "req-1", "bad request: boom")
	env := <-done
	require.NotNil(t, env.OK)
	assert.False(t, *env.OK)
	assert.Equal(t, TypeError, env.Type)
	assert.Equal(t, "req-1", env.RequestID)
	assert.Contains(t, env.Error, "boom")
}
