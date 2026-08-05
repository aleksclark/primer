package worktui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

func withPTYStdio(t *testing.T, feed, readySubstr string, fn func() error) error {
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

	var (
		mu     sync.Mutex
		outBuf bytes.Buffer
	)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				mu.Lock()
				_, _ = outBuf.Write(buf[:n])
				mu.Unlock()
			}
			if rerr != nil {
				return
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() { errCh <- fn() }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		s := outBuf.String()
		mu.Unlock()
		if readySubstr == "" {
			if len(s) > 0 {
				break
			}
		} else if strings.Contains(s, readySubstr) {
			break
		}
		select {
		case err := <-errCh:
			return err
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _ = io.WriteString(ptmx, feed)

	select {
	case err := <-errCh:
		return err
	case <-time.After(5 * time.Second):
		mu.Lock()
		got := outBuf.String()
		mu.Unlock()
		t.Fatalf("timed out waiting for worktui.Run; pty output=%q", got)
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

	err := withPTYStdio(t, "q", "Nav", func() error {
		return Run(Options{BaseURL: srv.URL, DeviceToken: "tok", HTTPClient: srv.Client()})
	})
	require.NoError(t, err)
}
