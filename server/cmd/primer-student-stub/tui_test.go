package main_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/aleksclark/trifle"
)

func TestStudentWorkTUI(t *testing.T) {
	trifle.SkipOnWindows(t)

	// Fake LMS work endpoint.
	mux := http.NewServeMux()
	mux.HandleFunc("/student/work", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-device-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"assignment": map[string]any{"state": "available"},
					"activity":   map[string]any{"title": "Basic Navigation", "slug": "basic-navigation"},
					"revision":   map[string]any{},
				},
			},
			"cursor": "",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	cmdDir := filepath.Dir(thisFile)
	bin := filepath.Join(t.TempDir(), "primer-student-stub")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = cmdDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build primer-student-stub: %v\n%s", err, out)
	}

	suite := trifle.NewSuite(t).Use(trifle.TestConfig{
		Program:     bin,
		Args:        []string{"-base-url", srv.URL, "-token", "test-device-token"},
		Rows:        24,
		Cols:        80,
		StartupWait: 400 * time.Millisecond,
		Timeout:     15 * time.Second,
	})

	suite.Run("lists work queue from API", func(t *testing.T, term *trifle.Terminal) {
		time.Sleep(300 * time.Millisecond)
		h := trifle.NewTestHelper(t, term)
		h.ExpectVisible("Student work queue", trifle.WithFull())
		h.ExpectVisible("Basic Navigation", trifle.WithFull())
		h.ExpectVisible("basic-navigation", trifle.WithFull())
		if err := term.Write("q"); err != nil {
			t.Fatalf("quit: %v", err)
		}
		if err := term.WaitWithTimeout(5 * time.Second); err != nil {
			t.Fatalf("process did not exit: %v\noutput:\n%s", err, term.Output())
		}
	})
}
