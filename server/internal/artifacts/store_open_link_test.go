package artifacts_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/artifacts"
)

func TestOpenObjectAndLinkSessionArtifact(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := artifacts.NewStore(root)
	require.NoError(t, err)

	payload := []byte("hello-object")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])

	_, n, err := store.PutObject(digest, bytes.NewReader(payload), int64(len(payload)))
	require.NoError(t, err)
	assert.Equal(t, int64(len(payload)), n)

	f, err := store.OpenObject(digest)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	got, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, payload, got)

	// Bad digest.
	_, err = store.OpenObject("zz")
	require.Error(t, err)

	// Link session artifact (hardlink or copy).
	sid := uuid.NewString()
	aid := uuid.NewString()
	rel, err := store.LinkSessionArtifact(sid, aid, digest)
	require.NoError(t, err)
	assert.Contains(t, rel, sid)
	// Idempotent second link.
	rel2, err := store.LinkSessionArtifact(sid, aid, digest)
	require.NoError(t, err)
	assert.Equal(t, rel, rel2)

	// Missing object.
	miss := sha256.Sum256([]byte("nope"))
	_, err = store.LinkSessionArtifact(sid, uuid.NewString(), hex.EncodeToString(miss[:]))
	require.Error(t, err)

	// Invalid ids.
	_, err = store.LinkSessionArtifact("../x", aid, digest)
	require.Error(t, err)

	// ObjectPath edge: empty digest.
	_, err = store.ObjectPath("")
	require.Error(t, err)

	// Ensure linked path exists under root.
	abs := filepath.Join(root, rel)
	st, err := os.Stat(abs)
	require.NoError(t, err)
	assert.False(t, st.IsDir())
}
