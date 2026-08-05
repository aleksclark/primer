package main_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/broker"
)

// TestBrokerSubcommandRoundTrip starts `primer-student broker` as a subprocess
// and exercises pair/health over the Unix socket without ever seeing the token.
func TestBrokerSubcommandRoundTrip(t *testing.T) {
	bin := buildStudentBin(t)
	dir := t.TempDir()
	socket := filepath.Join(dir, "broker.sock")
	db := filepath.Join(dir, "state.db")
	tokenFile := filepath.Join(dir, "device.token")

	const secret = "e2e-secret-token-never-in-ipc"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/student-devices/pair" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deviceId": "dev-e2e",
				"token":    secret,
				"student":  map[string]any{"id": "stu-e2e", "firstName": "Eve", "lastName": "E"},
				"device":   map[string]any{"id": "dev-e2e", "name": "e2e-ws"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(api.Close)

	cmd := exec.Command(bin, "broker",
		"-socket", socket,
		"-db", db,
		"-token-file", tokenFile,
		"-base-url", api.URL,
		"-skip-peer-cred",
		"-allow-unsandboxed",
		"-socket-group", "-",
	)
	cmd.Env = append(os.Environ(), "PRIMER_BROKER_SKIP_PEERCRED=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	// Wait for socket.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			cl, err := broker.Dial(socket)
			if err == nil {
				_, err = cl.Health(context.Background())
				_ = cl.Close()
				if err == nil {
					break
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })

	ctx := context.Background()
	h, err := cl.Health(ctx)
	require.NoError(t, err)
	require.False(t, h.Paired)

	pair, err := cl.Pair(ctx, "CODE", "e2e-ws")
	require.NoError(t, err)
	require.Equal(t, "dev-e2e", pair.DeviceID)
	raw, _ := json.Marshal(pair)
	require.NotContains(t, string(raw), secret)

	tok, err := os.ReadFile(tokenFile)
	require.NoError(t, err)
	require.Equal(t, secret, string(tok))
	st, err := os.Stat(tokenFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), st.Mode().Perm())

	h2, err := cl.Health(ctx)
	require.NoError(t, err)
	require.True(t, h2.Paired)
	rawH, _ := json.Marshal(h2)
	require.NotContains(t, string(rawH), secret)
}
