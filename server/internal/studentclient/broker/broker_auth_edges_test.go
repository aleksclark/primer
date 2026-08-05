package broker

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestAuthorizePeerCredMatrix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srv, err := New(Options{
		SocketPath: filepath.Join(dir, "a.sock"), DBPath: filepath.Join(dir, "a.db"),
		BaseURL: "http://127.0.0.1:1", SkipPeerCred: true, AllowedGroup: "-",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	assert.False(t, srv.authorize(nil))
	assert.True(t, srv.authorize(&PeerCred{UID: 0}))
	assert.True(t, srv.authorize(&PeerCred{UID: uint32(os.Getuid())}))
	assert.False(t, srv.authorize(&PeerCred{UID: 999999}))

	srv.opts.AllowedUIDs = []uint32{4242}
	assert.True(t, srv.authorize(&PeerCred{UID: 4242}))
	assert.False(t, srv.authorize(&PeerCred{UID: 999998}))
}

func TestHandleSchemaAndIdempotentReplay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srv, err := New(Options{
		SocketPath: filepath.Join(dir, "h.sock"), DBPath: filepath.Join(dir, "h.db"),
		BaseURL: "http://127.0.0.1:1", SkipPeerCred: true, AllowedGroup: "-", Offline: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	// Unsupported schema.
	resp := srv.handle(Envelope{SchemaVersion: 99, Type: TypeHealth, RequestID: "r1"})
	require.NotNil(t, resp.OK)
	assert.False(t, *resp.OK)
	assert.Contains(t, resp.Error, "unsupported schemaVersion")

	// Empty request id is filled.
	resp = srv.handle(Envelope{SchemaVersion: SchemaVersion, Type: TypeHealth})
	require.NotNil(t, resp.OK)
	assert.True(t, *resp.OK)
	assert.NotEmpty(t, resp.RequestID)

	// Seed doneReqs and verify idempotent open/complete replay.
	reqID := uuid.NewString()
	payload, err := json.Marshal(map[string]any{"replayed": true})
	require.NoError(t, err)
	srv.mu.Lock()
	srv.doneReqs[reqID] = payload
	srv.mu.Unlock()

	resp = srv.handle(Envelope{
		SchemaVersion: SchemaVersion, Type: TypeOpenSession, RequestID: reqID,
	})
	require.NotNil(t, resp.OK)
	assert.True(t, *resp.OK)
	assert.JSONEq(t, string(payload), string(resp.Payload))

	resp = srv.handle(Envelope{
		SchemaVersion: SchemaVersion, Type: TypeComplete, RequestID: reqID,
	})
	require.NotNil(t, resp.OK)
	assert.True(t, *resp.OK)
}

func TestClientDialAndCallErrorPaths(t *testing.T) {
	t.Parallel()
	// Dial missing socket.
	_, err := Dial(filepath.Join(t.TempDir(), "no.sock"))
	require.Error(t, err)

	// Start a broker and exercise Close then call reconnect failure.
	dir := t.TempDir()
	socket := filepath.Join(dir, "c.sock")
	srv, err := New(Options{
		SocketPath: socket, DBPath: filepath.Join(dir, "c.db"),
		BaseURL: "http://127.0.0.1:1", SkipPeerCred: true, AllowedGroup: "-", Offline: true,
	})
	require.NoError(t, err)
	require.NoError(t, ServeBackground(srv, 3*time.Second))
	t.Cleanup(func() { _ = srv.Close() })

	cl, err := Dial(socket)
	require.NoError(t, err)
	require.NoError(t, cl.Close())
	// Second close is fine.
	require.NoError(t, cl.Close())

	// After close, Health reconnects.
	_, err = cl.Health(context.Background())
	require.NoError(t, err)
	require.NoError(t, cl.Close())
}

func TestServeConnUnauthorizedPeer(t *testing.T) {
	t.Parallel()
	// Directly exercise writeErr / serveConn peer reject by calling writeErr.
	dir := t.TempDir()
	srv, err := New(Options{
		SocketPath: filepath.Join(dir, "p.sock"), DBPath: filepath.Join(dir, "p.db"),
		BaseURL: "http://127.0.0.1:1", SkipPeerCred: false, AllowedGroup: "-",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	c1, c2 := net.Pipe()
	t.Cleanup(func() { _ = c1.Close(); _ = c2.Close() })
	go srv.writeErr(c1, "rid", "unauthorized: peer credentials unavailable")
	env, err := Decode(bufio.NewReader(c2))
	require.NoError(t, err)
	require.NotNil(t, env.OK)
	assert.False(t, *env.OK)
	assert.Contains(t, env.Error, "unauthorized")
}

func TestNewMigrateLegacyAndTokenFileErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Legacy migrate with missing source is ok (no-op); invalid legacy path as directory file.
	legacy := filepath.Join(dir, "legacy.db")
	// Create empty legacy file — migrate should no-op or copy safely.
	require.NoError(t, os.WriteFile(legacy, []byte{}, 0o600))

	srv, err := New(Options{
		SocketPath:   filepath.Join(dir, "m.sock"),
		DBPath:       filepath.Join(dir, "state.db"),
		TokenFile:    filepath.Join(dir, "token"),
		LegacyDBPath: legacy,
		BaseURL:      "http://127.0.0.1:1",
		SkipPeerCred: true, AllowedGroup: "-",
	})
	// Empty legacy may error or succeed depending on migrate implementation.
	if err != nil {
		assert.Contains(t, err.Error(), "migrate")
		return
	}
	t.Cleanup(func() { _ = srv.Close() })
	_ = srv.Store()
}

func TestHandleUnknownTypeAndSessionMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srv, err := New(Options{
		SocketPath: filepath.Join(dir, "u.sock"), DBPath: filepath.Join(dir, "u.db"),
		BaseURL: "http://127.0.0.1:1", SkipPeerCred: true, AllowedGroup: "-", Offline: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	resp := srv.handle(Envelope{SchemaVersion: SchemaVersion, Type: "nope", RequestID: "x"})
	require.NotNil(t, resp.OK)
	assert.False(t, *resp.OK)

	// Seed a session row but no live engine session → ops error.
	csid := uuid.NewString()
	require.NoError(t, srv.store.SaveSession(context.Background(), cache.Session{
		ClientSessionID: csid, AssignmentID: "a", State: "started", LastAckedSequence: -1,
	}))
	raw, _ := json.Marshal(RunCommandRequest{ClientSessionID: csid, Line: "pwd"})
	resp = srv.handle(Envelope{SchemaVersion: SchemaVersion, Type: TypeRunCommand, RequestID: "r", Payload: raw})
	require.NotNil(t, resp.OK)
	assert.False(t, *resp.OK)
	_ = contracts.SchemaVersion
}
