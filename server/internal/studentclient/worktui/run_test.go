package worktui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

func withPTYStdio(t *testing.T, feed string, fn func() error) error {
	t.Helper()
	ptmx, tty, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = tty.Close()
	})
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin = tty
	os.Stdout = tty
	defer func() {
		os.Stdin = oldIn
		os.Stdout = oldOut
	}()
	errCh := make(chan error, 1)
	go func() { errCh <- fn() }()
	time.Sleep(250 * time.Millisecond)
	_, _ = io.WriteString(ptmx, feed)
	select {
	case err := <-errCh:
		return err
	case <-time.After(8 * time.Second):
		t.Fatal("timed out waiting for worktui.Run")
		return nil
	}
}

func TestRunQuitsAfterLoad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"assignment": map[string]any{"state": "available"},
					"activity":   map[string]any{"title": "Nav", "slug": "basic-navigation"},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	err := withPTYStdio(t, "q", func() error {
		return Run(Options{BaseURL: srv.URL, DeviceToken: "tok", HTTPClient: srv.Client()})
	})
	require.NoError(t, err)
}
