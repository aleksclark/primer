package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/aleksclark/trifle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func buildStudentBin(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	cmdDir := filepath.Dir(thisFile)
	bin := filepath.Join(t.TempDir(), "primer-student")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = cmdDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build primer-student: %v\n%s", err, out)
	}
	return bin
}

// directEnv enables legacy direct Store/Client mode for trifle TUI tests
// (no broker process). Production launches use the broker socket.
func directEnv() []string {
	return append(os.Environ(), "PRIMER_STUDENT_DIRECT=1")
}

func TestStudentTUIPairingWhenNoToken(t *testing.T) {
	trifle.SkipOnWindows(t)
	bin := buildStudentBin(t)
	dbPath := filepath.Join(t.TempDir(), "empty.db")

	suite := trifle.NewSuite(t).Use(trifle.TestConfig{
		Program:     bin,
		Env:         directEnv(),
		Args:        []string{"-db", dbPath, "-base-url", "http://127.0.0.1:1", "-offline"},
		Rows:        24,
		Cols:        80,
		StartupWait: 500 * time.Millisecond,
		Timeout:     20 * time.Second,
	})

	suite.Run("shows pairing screen", func(t *testing.T, term *trifle.Terminal) {
		time.Sleep(400 * time.Millisecond)
		h := trifle.NewTestHelper(t, term)
		h.ExpectVisible("pair device", trifle.WithFull())
		h.ExpectVisible("pairing code", trifle.WithFull())

		// Resize should not crash (cols, rows).
		if err := term.Resize(100, 30); err != nil {
			t.Fatalf("resize: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
		h.ExpectVisible("pair device", trifle.WithFull())

		if err := term.Write("q"); err != nil {
			t.Fatalf("quit: %v", err)
		}
		if err := term.WaitWithTimeout(5 * time.Second); err != nil {
			t.Fatalf("process did not exit: %v\noutput:\n%s", err, term.Output())
		}
	})
}

type tuiEnv struct {
	Server       *httptest.Server
	BaseURL      string
	Q            repo.Querier
	DeviceToken  string
	AssignmentID string
	DBPath       string
}

func startTUIEnv(t *testing.T) *tuiEnv {
	t.Helper()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	_, handler := api.New(q, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "tui-" + uuid.NewString()[:8] + "@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Tui", "last_name": "Kid"})
	parentToken := parentLogin(t, srv.URL, ed.Email, password)

	root := repoRoot(t)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	_, err = curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	assignmentID := postJSON[map[string]any](t, srv.URL+"/assignments", map[string]any{
		"studentId":          student.ID,
		"activityRevisionId": rev.ID,
		"priority":           10,
	}, parentToken)["id"].(string)

	code := postJSON[map[string]any](t, srv.URL+"/pairing-codes", map[string]any{
		"studentId": student.ID,
	}, parentToken)["code"].(string)

	pair := postJSON[map[string]any](t, srv.URL+"/student-devices/pair", map[string]any{
		"code":       code,
		"deviceName": "tui-ws",
	}, "")
	deviceToken := pair["token"].(string)

	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := cache.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, store.SetDeviceToken(ctx, deviceToken))
	require.NoError(t, store.SetDeviceIdentity(ctx, pair["deviceId"].(string), student.ID, "tui-ws"))
	_ = store.Close()

	return &tuiEnv{
		Server:       srv,
		BaseURL:      srv.URL,
		Q:            q,
		DeviceToken:  deviceToken,
		AssignmentID: assignmentID,
		DBPath:       dbPath,
	}
}

func parentLogin(t *testing.T, base, email, password string) string {
	t.Helper()
	body := postJSON[map[string]any](t, base+"/auth/login", map[string]any{
		"email": email, "password": password,
	}, "")
	tok, _ := body["token"].(string)
	require.NotEmpty(t, tok)
	return tok
}

func postJSON[T any](t *testing.T, url string, body any, bearer string) T {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300, "POST %s -> %d %s", url, resp.StatusCode, data)
	var out T
	require.NoError(t, json.Unmarshal(data, &out), string(data))
	return out
}

func TestStudentTUIWorkQueueAndComplete(t *testing.T) {
	trifle.SkipOnWindows(t)
	env := startTUIEnv(t)
	bin := buildStudentBin(t)
	ws := filepath.Join(t.TempDir(), "ws")

	suite := trifle.NewSuite(t).Use(trifle.TestConfig{
		Program: bin,
		Env:         directEnv(),
		Args: []string{
			"-db", env.DBPath,
			"-base-url", env.BaseURL,
			"-workspace", ws,
			"-token", env.DeviceToken,
		},
		Rows:        32,
		Cols:        100,
		StartupWait: 800 * time.Millisecond,
		Timeout:     90 * time.Second,
	})

	suite.Run("queue open complete basic-navigation", func(t *testing.T, term *trifle.Terminal) {
		h := trifle.NewTestHelper(t, term)

		// Work queue after sync.
		time.Sleep(800 * time.Millisecond)
		h.ExpectVisible("work queue", trifle.WithFull())
		h.ExpectVisible("Basic Navigation", trifle.WithFull())
		h.ExpectVisible("basic-navigation", trifle.WithFull())

		// Resize should not crash on queue (cols, rows).
		if err := term.Resize(90, 28); err != nil {
			t.Fatalf("resize: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
		h.ExpectVisible("work queue", trifle.WithFull())

		// Open assignment.
		if err := term.Write("\r"); err != nil {
			t.Fatalf("enter: %v", err)
		}
		time.Sleep(1200 * time.Millisecond)
		h.ExpectVisible("Basic Navigation", trifle.WithFull())
		h.ExpectVisible("Checks", trifle.WithFull())

		// basic-navigation fixtures already satisfy filesystem checks; verify then complete.
		if err := term.Write("verify\r"); err != nil {
			t.Fatalf("verify: %v", err)
		}
		time.Sleep(800 * time.Millisecond)
		h.ExpectVisible("REQUIRED PASS", trifle.WithFull())

		if err := term.Write("complete\r"); err != nil {
			t.Fatalf("complete: %v", err)
		}
		time.Sleep(1500 * time.Millisecond)
		h.ExpectVisible("Activity complete", trifle.WithFull())
		// accepted and synced OR awaiting sync
		out := term.Output()
		if !bytesContainsAny(out, "accepted", "awaiting sync", "completed") {
			t.Fatalf("expected completion summary, got:\n%s", out)
		}

		// Return to queue and quit.
		if err := term.Write("\r"); err != nil {
			t.Fatalf("enter summary: %v", err)
		}
		time.Sleep(600 * time.Millisecond)
		h.ExpectVisible("work queue", trifle.WithFull())

		if err := term.Write("q"); err != nil {
			t.Fatalf("quit: %v", err)
		}
		if err := term.WaitWithTimeout(8 * time.Second); err != nil {
			t.Fatalf("process did not exit: %v\noutput:\n%s", err, term.Output())
		}
	})
}

func TestStudentTUICommandRunnerPath(t *testing.T) {
	trifle.SkipOnWindows(t)
	env := startTUIEnv(t)
	bin := buildStudentBin(t)
	ws := filepath.Join(t.TempDir(), "ws-cmd")

	suite := trifle.NewSuite(t).Use(trifle.TestConfig{
		Program: bin,
		Env:         directEnv(),
		Args: []string{
			"-db", env.DBPath,
			"-base-url", env.BaseURL,
			"-workspace", ws,
			"-token", env.DeviceToken,
		},
		Rows:        32,
		Cols:        100,
		StartupWait: 800 * time.Millisecond,
		Timeout:     90 * time.Second,
	})

	suite.Run("runs shell commands in activity", func(t *testing.T, term *trifle.Terminal) {
		h := trifle.NewTestHelper(t, term)
		time.Sleep(800 * time.Millisecond)
		h.ExpectVisible("Basic Navigation", trifle.WithFull())

		if err := term.Write("\r"); err != nil {
			t.Fatalf("open: %v", err)
		}
		time.Sleep(1200 * time.Millisecond)
		h.ExpectVisible("Checks", trifle.WithFull())

		// Real command entry.
		if err := term.Write("pwd\r"); err != nil {
			t.Fatalf("pwd: %v", err)
		}
		time.Sleep(700 * time.Millisecond)
		h.ExpectVisible("commands:", trifle.WithFull())

		if err := term.Write("ls\r"); err != nil {
			t.Fatalf("ls: %v", err)
		}
		time.Sleep(700 * time.Millisecond)
		// welcome.txt should appear in ls output of home.
		h.ExpectVisible("welcome.txt", trifle.WithFull())

		if err := term.Write("verify\r"); err != nil {
			t.Fatalf("verify: %v", err)
		}
		time.Sleep(700 * time.Millisecond)
		h.ExpectVisible("REQUIRED PASS", trifle.WithFull())

		if err := term.Write("complete\r"); err != nil {
			t.Fatalf("complete: %v", err)
		}
		time.Sleep(1500 * time.Millisecond)
		h.ExpectVisible("Activity complete", trifle.WithFull())

		if err := term.Write("q"); err != nil {
			t.Fatalf("quit: %v", err)
		}
		// From summary q returns to queue; quit again.
		time.Sleep(500 * time.Millisecond)
		_ = term.Write("q")
		if err := term.WaitWithTimeout(8 * time.Second); err != nil {
			t.Fatalf("process did not exit: %v\noutput:\n%s", err, term.Output())
		}
	})
}

func bytesContainsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if bytes.Contains([]byte(s), []byte(sub)) {
			return true
		}
	}
	return false
}

func startTypingTUIEnv(t *testing.T) *tuiEnv {
	t.Helper()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	_, handler := api.New(q, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "tui-type-" + uuid.NewString()[:8] + "@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Type", "last_name": "Tui"})
	parentToken := parentLogin(t, srv.URL, ed.Email, password)

	root := repoRoot(t)
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(root, "curriculum", "standards"),
		Now:          time.Now().UTC(),
	})
	require.NoError(t, err)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	_, rev, err := curriculum.PublishDocument(ctx, q, doc, time.Now().UTC())
	require.NoError(t, err)

	assignmentID := postJSON[map[string]any](t, srv.URL+"/assignments", map[string]any{
		"studentId":          student.ID,
		"activityRevisionId": rev.ID,
		"priority":           10,
	}, parentToken)["id"].(string)

	code := postJSON[map[string]any](t, srv.URL+"/pairing-codes", map[string]any{
		"studentId": student.ID,
	}, parentToken)["code"].(string)

	pair := postJSON[map[string]any](t, srv.URL+"/student-devices/pair", map[string]any{
		"code":       code,
		"deviceName": "tui-type-ws",
	}, "")
	deviceToken := pair["token"].(string)

	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := cache.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, store.SetDeviceToken(ctx, deviceToken))
	require.NoError(t, store.SetDeviceIdentity(ctx, pair["deviceId"].(string), student.ID, "tui-type-ws"))
	_ = store.Close()

	return &tuiEnv{
		Server:       srv,
		BaseURL:      srv.URL,
		Q:            q,
		DeviceToken:  deviceToken,
		AssignmentID: assignmentID,
		DBPath:       dbPath,
	}
}

func TestStudentTUITypingFlow(t *testing.T) {
	trifle.SkipOnWindows(t)
	env := startTypingTUIEnv(t)
	bin := buildStudentBin(t)
	ws := filepath.Join(t.TempDir(), "ws-type")

	root := repoRoot(t)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	require.NotNil(t, doc.Content.Typing)

	suite := trifle.NewSuite(t).Use(trifle.TestConfig{
		Program: bin,
		Env:         directEnv(),
		Args: []string{
			"-db", env.DBPath,
			"-base-url", env.BaseURL,
			"-workspace", ws,
			"-token", env.DeviceToken,
		},
		Rows:        32,
		Cols:        100,
		StartupWait: 800 * time.Millisecond,
		Timeout:     120 * time.Second,
	})

	suite.Run("complete typing activity", func(t *testing.T, term *trifle.Terminal) {
		h := trifle.NewTestHelper(t, term)
		time.Sleep(800 * time.Millisecond)
		h.ExpectVisible("work queue", trifle.WithFull())
		h.ExpectVisible("Command Typing Basics", trifle.WithFull())

		if err := term.Write("\r"); err != nil {
			t.Fatalf("open: %v", err)
		}
		time.Sleep(1200 * time.Millisecond)
		h.ExpectVisible("Prompt:", trifle.WithFull())
		h.ExpectVisible("WPM", trifle.WithFull())

		// Auto-type every assigned prompt.
		for _, p := range doc.Content.Typing.Prompts {
			if err := term.Write(p.Text); err != nil {
				t.Fatalf("type %q: %v", p.Text, err)
			}
			time.Sleep(80 * time.Millisecond)
		}
		time.Sleep(600 * time.Millisecond)
		h.ExpectVisible("REQUIRED PASS", trifle.WithFull())

		// Enter completes when thresholds met.
		if err := term.Write("\r"); err != nil {
			t.Fatalf("complete: %v", err)
		}
		time.Sleep(1500 * time.Millisecond)
		h.ExpectVisible("Activity complete", trifle.WithFull())

		if err := term.Write("q"); err != nil {
			t.Fatalf("quit: %v", err)
		}
		time.Sleep(400 * time.Millisecond)
		_ = term.Write("q")
		if err := term.WaitWithTimeout(8 * time.Second); err != nil {
			t.Fatalf("process did not exit: %v\noutput:\n%s", err, term.Output())
		}
	})
}
