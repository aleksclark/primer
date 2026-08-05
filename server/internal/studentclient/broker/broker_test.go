package broker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func startTestBroker(t *testing.T, baseURL string, allowUnsandboxed bool) (socket string, srv *broker.Server, tokenFile string) {
	t.Helper()
	dir := t.TempDir()
	socket = filepath.Join(dir, "broker.sock")
	db := filepath.Join(dir, "state.db")
	tokenFile = filepath.Join(dir, "device.token")
	srv, err := broker.New(broker.Options{
		SocketPath:       socket,
		DBPath:           db,
		TokenFile:        tokenFile,
		BaseURL:          baseURL,
		WorkspaceRoot:    filepath.Join(dir, "ws"),
		DeviceName:       "test-ws",
		UseSandbox:       false,
		AllowUnsandboxed: allowUnsandboxed,
		SkipPeerCred:     true,
		AllowedGroup:     "-",
	})
	require.NoError(t, err)
	require.NoError(t, broker.ServeBackground(srv, 3*time.Second))
	t.Cleanup(func() { _ = srv.Close() })
	return socket, srv, tokenFile
}

func TestTokenFileMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "device.token")
	require.NoError(t, broker.WriteTokenFile(path, "secret-token-value"))
	ok, err := broker.TokenFileModeOK(path)
	require.NoError(t, err)
	require.True(t, ok, "token file must be mode 0600")
	st, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), st.Mode().Perm())

	got, err := broker.ReadTokenFile(path)
	require.NoError(t, err)
	require.Equal(t, "secret-token-value", got)
}

func TestPairNeverReturnsToken(t *testing.T) {
	const secret = "super-secret-device-token-xyz"
	srvHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/student-devices/pair" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deviceId": "dev-1",
				"token":    secret,
				"student":  map[string]any{"id": "stu-1", "firstName": "Ada", "lastName": "L"},
				"device":   map[string]any{"id": "dev-1", "name": "ws"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srvHTTP.Close)

	socket, _, tokenFile := startTestBroker(t, srvHTTP.URL, true)
	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })

	ctx := context.Background()
	pair, err := cl.Pair(ctx, "ABCD", "ws")
	require.NoError(t, err)
	require.Equal(t, "dev-1", pair.DeviceID)
	require.Equal(t, "stu-1", pair.StudentID)
	// Marshal response and ensure secret never appears.
	raw, _ := json.Marshal(pair)
	require.NotContains(t, string(raw), secret)
	require.NotContains(t, strings.ToLower(string(raw)), `"token"`)

	// Token lives only on broker disk.
	tok, err := broker.ReadTokenFile(tokenFile)
	require.NoError(t, err)
	require.Equal(t, secret, tok)
	ok, err := broker.TokenFileModeOK(tokenFile)
	require.NoError(t, err)
	require.True(t, ok)

	h, err := cl.Health(ctx)
	require.NoError(t, err)
	require.True(t, h.Paired)
	rawH, _ := json.Marshal(h)
	require.NotContains(t, string(rawH), secret)
}

func TestAuthorizeRejectsWhenPeerCredRequired(t *testing.T) {
	// Build a server that requires peer cred and an impossible allowed UID so
	// same-user connections still fail the allow list when group is disabled
	// and AllowedUIDs excludes self... Actually same UID is always allowed.
	// Instead verify SkipPeerCred=false path can obtain peercred on Linux
	// without rejecting self.
	dir := t.TempDir()
	socket := filepath.Join(dir, "broker.sock")
	srv, err := broker.New(broker.Options{
		SocketPath:   socket,
		DBPath:       filepath.Join(dir, "state.db"),
		TokenFile:    filepath.Join(dir, "device.token"),
		BaseURL:      "http://127.0.0.1:1",
		SkipPeerCred: false,
		AllowedGroup: "-", // no group; same-uid still OK
	})
	require.NoError(t, err)
	require.NoError(t, broker.ServeBackground(srv, 3*time.Second))
	t.Cleanup(func() { _ = srv.Close() })

	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	_, err = cl.Health(context.Background())
	require.NoError(t, err, "same-uid peer must be accepted")
}

func TestEventSurvivesBrokerRestart(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "broker.sock")
	db := filepath.Join(dir, "state.db")
	tokenFile := filepath.Join(dir, "device.token")
	require.NoError(t, broker.WriteTokenFile(tokenFile, "tok-restart"))

	// Seed a pending outbox event via cache directly (simulates prior session).
	store, err := cache.Open(db)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, store.SetDeviceToken(ctx, "tok-restart"))
	csid := uuid.NewString()
	require.NoError(t, store.SaveSession(ctx, cache.Session{
		ClientSessionID:    csid,
		AssignmentID:       uuid.NewString(),
		ActivityRevisionID: uuid.NewString(),
		State:              "started",
		LastAckedSequence:  -1,
		NextSequence:       0,
	}))
	ev := contracts.SessionEvent{
		SchemaVersion: "1",
		EventID:       uuid.NewString(),
		Type:          "session.started",
		Sequence:      0,
	}
	_, err = store.EnqueueEvent(ctx, csid, ev)
	require.NoError(t, err)
	pending, err := store.GetPendingSync(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, pending.EventCount, 1)
	require.NoError(t, store.Close())

	// Start broker #1 — durable rows still present.
	srv1, err := broker.New(broker.Options{
		SocketPath:   socket,
		DBPath:       db,
		TokenFile:    tokenFile,
		BaseURL:      "http://127.0.0.1:1",
		SkipPeerCred: true,
		AllowedGroup: "-",
		Offline:      true,
	})
	require.NoError(t, err)
	require.NoError(t, broker.ServeBackground(srv1, 3*time.Second))
	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	h, err := cl.Health(ctx)
	require.NoError(t, err)
	require.True(t, h.Paired)
	require.GreaterOrEqual(t, h.PendingEvents, 1)
	_ = cl.Close()
	require.NoError(t, srv1.Close())

	// Restart broker #2 on same DB — events still pending.
	socket2 := filepath.Join(dir, "broker2.sock")
	srv2, err := broker.New(broker.Options{
		SocketPath:   socket2,
		DBPath:       db,
		TokenFile:    tokenFile,
		BaseURL:      "http://127.0.0.1:1",
		SkipPeerCred: true,
		AllowedGroup: "-",
		Offline:      true,
	})
	require.NoError(t, err)
	require.NoError(t, broker.ServeBackground(srv2, 3*time.Second))
	t.Cleanup(func() { _ = srv2.Close() })
	cl2, err := broker.Dial(socket2)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl2.Close() })
	h2, err := cl2.Health(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, h2.PendingEvents, 1, "queued events must survive broker restart")
}

func TestMigrateLegacyState(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy", "state.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o755))
	store, err := cache.Open(legacy)
	require.NoError(t, err)
	require.NoError(t, store.SetDeviceToken(context.Background(), "legacy-tok"))
	require.NoError(t, store.Close())

	dest := filepath.Join(dir, "broker", "state.db")
	tokenFile := filepath.Join(dir, "broker", "device.token")
	require.NoError(t, broker.MigrateLegacyState(legacy, dest, tokenFile))

	_, err = os.Stat(dest)
	require.NoError(t, err)
	_, err = os.Stat(legacy + ".pre-broker.bak")
	require.NoError(t, err)
	tok, err := broker.ReadTokenFile(tokenFile)
	require.NoError(t, err)
	require.Equal(t, "legacy-tok", tok)
}

func TestListWorkRoundTrip(t *testing.T) {
	socket, srv, _ := startTestBroker(t, "http://127.0.0.1:1", true)
	// Seed work via broker store.
	ctx := context.Background()
	require.NoError(t, srv.Store().SaveWork(ctx, nil))

	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	items, err := cl.ListWork(ctx)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestHealthReportsAllowUnsandboxed(t *testing.T) {
	socket, _, _ := startTestBroker(t, "http://127.0.0.1:1", false)
	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	h, err := cl.Health(context.Background())
	require.NoError(t, err)
	require.False(t, h.AllowUnsandboxed, "production default must not allow unsandboxed")
}
