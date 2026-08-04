package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/repo"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
}

type harnessEnv struct {
	Server       *httptest.Server
	BaseURL      string
	Q            repo.Querier
	DeviceToken  string
	AssignmentID string
	StudentID    string
}

func startEnv(t *testing.T) *harnessEnv {
	t.Helper()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	_, handler := api.New(q, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "engine-" + uuid.NewString()[:8] + "@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Engine", "last_name": "Kid"})

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
		"deviceName": "engine-ws",
	}, "")
	deviceToken := pair["token"].(string)

	return &harnessEnv{
		Server:       srv,
		BaseURL:      srv.URL,
		Q:            q,
		DeviceToken:  deviceToken,
		AssignmentID: assignmentID,
		StudentID:    student.ID,
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

func openEngine(t *testing.T, env *harnessEnv, dbPath string, offline bool) *engine.Engine {
	t.Helper()
	store, err := cache.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	require.NoError(t, store.SetDeviceToken(ctx, env.DeviceToken))

	cl := studentapi.New(env.BaseURL, env.DeviceToken)
	eng, err := engine.New(engine.Options{
		Client:           cl,
		Store:            store,
		WorkspaceRoot:    t.TempDir(),
		Offline:          offline,
		AllowUnsandboxed: true,
	})
	require.NoError(t, err)
	return eng
}

func TestEngineCompletesBasicNavigationOnline(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	eng := openEngine(t, env, dbPath, false)

	err := eng.RunAssignment(ctx, env.AssignmentID, engine.BasicNavigationScript())
	require.NoError(t, err)

	st := eng.Status()
	assert.True(t, st.RequiredPassed)
	assert.True(t, st.CompletionQueued)
	assert.True(t, st.CompletionAcked, "status=%+v", st)
	assert.GreaterOrEqual(t, st.CommandsRun, 3)
	assert.Equal(t, "basic-navigation", st.ActivitySlug)

	// Server has exactly one completion.
	assertServerOneCompletion(t, env)
}

func TestEngineDisconnectRestartResume(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")

	// Phase A: pull work + start session + run half, queue events, no completion yet.
	store, err := cache.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, store.SetDeviceToken(ctx, env.DeviceToken))
	cl := studentapi.New(env.BaseURL, env.DeviceToken)
	eng, err := engine.New(engine.Options{
		Client: cl, Store: store, WorkspaceRoot: filepath.Join(dir, "ws"),
		AllowUnsandboxed: true,
	})
	require.NoError(t, err)

	// Download work first.
	res := eng.SyncOnce(ctx)
	require.NoError(t, res.Err)
	require.GreaterOrEqual(t, res.WorkItems, 1)

	// Run with a mid-script "disconnect": complete offline so completion stays in outbox.
	engOffline, err := engine.New(engine.Options{
		Client: cl, Store: store, WorkspaceRoot: filepath.Join(dir, "ws2"),
		Offline: true, AllowUnsandboxed: true,
	})
	require.NoError(t, err)
	err = engOffline.RunAssignment(ctx, env.AssignmentID, engine.BasicNavigationScript())
	require.NoError(t, err)
	st := engOffline.Status()
	assert.True(t, st.RequiredPassed)
	assert.True(t, st.CompletionQueued)
	assert.False(t, st.CompletionAcked)

	pending, err := store.GetPendingSync(ctx)
	require.NoError(t, err)
	require.Greater(t, pending.EventCount+pending.CompleteCount, 0, "expected durable pending sync")
	require.NoError(t, store.Close())

	// Phase B: reopen cache, online sync flushes everything once.
	store2, err := cache.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store2.Close() })
	cl2 := studentapi.New(env.BaseURL, env.DeviceToken)
	eng2, err := engine.New(engine.Options{
		Client: cl2, Store: store2, AllowUnsandboxed: true,
	})
	require.NoError(t, err)

	// Need a server session id on pending rows: StartSession was skipped offline.
	// Bind by starting session with the stored client id, then flush.
	sessList := mustPendingSessions(t, store2)
	for _, csid := range sessList {
		sess, err := store2.GetSession(ctx, csid)
		require.NoError(t, err)
		if sess.ServerSessionID == "" {
			serverSess, err := cl2.StartSession(ctx, sess.ClientSessionID, sess.AssignmentID)
			require.NoError(t, err)
			require.NoError(t, store2.BindServerSession(ctx, sess.ClientSessionID, serverSess.ID))
		}
	}

	require.NoError(t, eng2.ResumeAndSync(ctx))
	pending2, err := store2.GetPendingSync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, pending2.EventCount)
	assert.Equal(t, 0, pending2.CompleteCount)

	// Flush again (idempotent) — still one completion on server.
	require.NoError(t, eng2.ResumeAndSync(ctx))
	assertServerOneCompletion(t, env)
}

func TestEngineOnlineThenIdempotentRerunSync(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	eng := openEngine(t, env, dbPath, false)
	require.NoError(t, eng.RunAssignment(ctx, env.AssignmentID, engine.BasicNavigationScript()))
	require.NoError(t, eng.ResumeAndSync(ctx))
	require.NoError(t, eng.ResumeAndSync(ctx))
	assertServerOneCompletion(t, env)
}

func assertServerOneCompletion(t *testing.T, env *harnessEnv) {
	t.Helper()
	ctx := context.Background()
	// Count completions via sessions for this student.
	page, err := repo.LearningSessions.List(ctx, env.Q, repo.ListParams{
		Limit:   50,
		Filters: map[string]any{"student_id": env.StudentID},
	})
	require.NoError(t, err)
	require.NotEmpty(t, page.Items)
	var completed int
	var sessionIDs []string
	for _, s := range page.Items {
		if s.State == "completed" {
			completed++
			sessionIDs = append(sessionIDs, s.ID)
		}
	}
	require.Equal(t, 1, completed, "expected exactly one completed session")

	cmp, err := repo.GetCompletionBySession(ctx, env.Q, sessionIDs[0])
	require.NoError(t, err)
	require.NotEmpty(t, cmp.CompletionID)

	// Mastery evidence once.
	evPage, err := repo.MasteryEvidences.List(ctx, env.Q, repo.ListParams{Limit: 100})
	require.NoError(t, err)
	var n int
	for _, e := range evPage.Items {
		if e.SourceRef != "" && (contains(e.SourceRef, sessionIDs[0]) || contains(e.SourceRef, cmp.CompletionID)) {
			n++
		}
	}
	assert.Greater(t, n, 0, "expected mastery evidence for session")

	mrPage, err := repo.MasteryRecords.List(ctx, env.Q, repo.ListParams{
		Limit:   50,
		Filters: map[string]any{"student_id": env.StudentID},
	})
	require.NoError(t, err)
	require.NotEmpty(t, mrPage.Items)
}

func mustPendingSessions(t *testing.T, store *cache.Store) []string {
	t.Helper()
	pending, err := store.GetPendingSync(context.Background())
	require.NoError(t, err)
	seen := map[string]bool{}
	var ids []string
	for _, e := range pending.Events {
		if !seen[e.ClientSessionID] {
			seen[e.ClientSessionID] = true
			ids = append(ids, e.ClientSessionID)
		}
	}
	for _, c := range pending.Completions {
		if !seen[c.ClientSessionID] {
			seen[c.ClientSessionID] = true
			ids = append(ids, c.ClientSessionID)
		}
	}
	require.NotEmpty(t, ids)
	return ids
}

func contains(s, sub string) bool {
	return len(sub) > 0 && (s == sub || len(s) >= len(sub) && (stringIndex(s, sub) >= 0))
}

func stringIndex(s, sub string) int {
	return bytes.Index([]byte(s), []byte(sub))
}
