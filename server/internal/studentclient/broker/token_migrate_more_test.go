package broker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
)

func TestWriteTokenFileEmptyPath(t *testing.T) {
	t.Parallel()
	require.Error(t, broker.WriteTokenFile("", "tok"))
}

func TestReadTokenFileEmptyAndMissing(t *testing.T) {
	t.Parallel()
	tok, err := broker.ReadTokenFile("")
	require.NoError(t, err)
	assert.Empty(t, tok)

	tok, err = broker.ReadTokenFile(filepath.Join(t.TempDir(), "nope.token"))
	require.NoError(t, err)
	assert.Empty(t, tok)
}

func TestTokenFileModeOKMissingAndBadMode(t *testing.T) {
	t.Parallel()
	ok, err := broker.TokenFileModeOK(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	assert.False(t, ok)

	path := filepath.Join(t.TempDir(), "device.token")
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0o644))
	ok, err = broker.TokenFileModeOK(path)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, broker.WriteTokenFile(path, "secret2"))
	ok, err = broker.TokenFileModeOK(path)
	require.NoError(t, err)
	assert.True(t, ok)
	got, err := broker.ReadTokenFile(path)
	require.NoError(t, err)
	assert.Equal(t, "secret2", got)
}

func TestMigrateLegacyStateNoopsAndCopy(t *testing.T) {
	t.Parallel()
	// Empty paths no-op
	require.NoError(t, broker.MigrateLegacyState("", "x", ""))
	require.NoError(t, broker.MigrateLegacyState("x", "", ""))

	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy", "state.db")
	dest := filepath.Join(dir, "broker", "state.db")
	tokenFile := filepath.Join(dir, "broker", "device.token")

	// Same path no-op
	require.NoError(t, broker.MigrateLegacyState(legacy, legacy, tokenFile))

	// Missing legacy no-op
	require.NoError(t, broker.MigrateLegacyState(legacy, dest, tokenFile))

	// Create legacy DB with token
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o755))
	store, err := cache.Open(legacy)
	require.NoError(t, err)
	require.NoError(t, store.SetDeviceToken(context.Background(), "legacy-tok"))
	require.NoError(t, store.Close())

	// Dest already exists → no-op
	require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
	require.NoError(t, os.WriteFile(dest, []byte("x"), 0o600))
	require.NoError(t, broker.MigrateLegacyState(legacy, dest, tokenFile))
	require.NoError(t, os.Remove(dest))

	// Real migrate copies + extracts token
	require.NoError(t, broker.MigrateLegacyState(legacy, dest, tokenFile))
	_, err = os.Stat(dest)
	require.NoError(t, err)
	// legacy renamed to bak
	_, err = os.Stat(legacy + ".pre-broker.bak")
	require.NoError(t, err)
	tok, err := broker.ReadTokenFile(tokenFile)
	require.NoError(t, err)
	assert.Equal(t, "legacy-tok", tok)

	// Dest exists now → second migrate no-op
	require.NoError(t, broker.MigrateLegacyState(legacy+".pre-broker.bak", dest, tokenFile))
}

func TestBrokerNewLoadsTokenFromDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	tokenFile := filepath.Join(dir, "device.token")
	socket := filepath.Join(dir, "b.sock")

	store, err := cache.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, store.SetDeviceToken(context.Background(), "from-db"))
	require.NoError(t, store.Close())

	srv, err := broker.New(broker.Options{
		SocketPath: socket, DBPath: dbPath, TokenFile: tokenFile,
		BaseURL: "http://127.0.0.1:1", WorkspaceRoot: filepath.Join(dir, "ws"),
		SkipPeerCred: true, AllowedGroup: "-", AllowUnsandboxed: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	// Token file written from DB
	tok, err := broker.ReadTokenFile(tokenFile)
	require.NoError(t, err)
	assert.Equal(t, "from-db", tok)
}
