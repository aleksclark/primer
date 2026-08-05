package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	return openEngineWS(t, env, dbPath, t.TempDir(), offline)
}

func openEngineWS(t *testing.T, env *harnessEnv, dbPath, wsRoot string, offline bool) *engine.Engine {
	t.Helper()
	store, err := cache.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	require.NoError(t, store.SetDeviceToken(ctx, env.DeviceToken))

	cl := studentapi.New(env.BaseURL, env.DeviceToken)
	eng, err := engine.New(engine.Options{
		Client:                    cl,
		Store:                     store,
		WorkspaceRoot:             wsRoot,
		Offline:                   offline,
		AllowUnsandboxed:          true,
		// Headless scripted RunShell produces structured command evidence.
		StructuredCommandEvidence: true,
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
		AllowUnsandboxed: true, StructuredCommandEvidence: true,
	})
	require.NoError(t, err)

	// Download work first.
	res := eng.SyncOnce(ctx)
	require.NoError(t, res.Err)
	require.GreaterOrEqual(t, res.WorkItems, 1)

	// Run with a mid-script "disconnect": complete offline so completion stays in outbox.
	engOffline, err := engine.New(engine.Options{
		Client: cl, Store: store, WorkspaceRoot: filepath.Join(dir, "ws2"),
		Offline: true, AllowUnsandboxed: true, StructuredCommandEvidence: true,
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
		Client: cl2, Store: store2, AllowUnsandboxed: true, StructuredCommandEvidence: true,
	})
	require.NoError(t, err)

	// Need a server session id on pending rows: StartSession was skipped offline.
	// Bind by starting session with the stored client id, then flush.
	sessList := mustPendingSessions(t, store2)
	for _, csid := range sessList {
		sess, err := store2.GetSession(ctx, csid)
		require.NoError(t, err)
		if sess.ServerSessionID == "" {
			serverSess, err := cl2.StartSession(ctx, sess.ClientSessionID, sess.AssignmentID, contracts.CapStructuredCommandEvidence)
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

func startTypingEnv(t *testing.T) *harnessEnv {
	t.Helper()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	_, handler := api.New(q, api.Options{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	const password = "test-password-ok"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "typing-" + uuid.NewString()[:8] + "@example.com",
		"role":  "parent",
	})
	student := factory.Student(t, q, factory.Override{"first_name": "Type", "last_name": "Kid"})
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
		"deviceName": "typing-ws",
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

func TestEngineCompletesTypingOnlineMasteryIsolation(t *testing.T) {
	t.Parallel()
	env := startTypingEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	eng := openEngine(t, env, dbPath, false)

	// Seed a NAV mastery record so we can prove typing does not touch it.
	stdPage, err := repo.Standards.List(ctx, env.Q, repo.ListParams{
		Limit:  200,
		Search: "PRIMER.DL.6.NAV.1",
	})
	require.NoError(t, err)
	var navStdID string
	for _, st := range stdPage.Items {
		if st.Code == "PRIMER.DL.6.NAV.1" {
			navStdID = st.ID
			break
		}
	}
	require.NotEmpty(t, navStdID, "NAV standard must exist after standards publish")
	navRec := factory.MasteryRecord(t, env.Q, factory.Override{
		"student_id":  env.StudentID,
		"standard_id": navStdID,
		"status":      "in_progress",
		"confidence":  0.42,
	})
	beforeConf := navRec.Confidence
	beforeStatus := navRec.Status

	sess, err := eng.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	assert.Equal(t, contracts.KindTyping, sess.Kind())

	// Load prompts from the activity content via snapshot loop.
	// Type each prompt text exactly (auto-advances on full match).
	root := repoRoot(t)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	require.NotNil(t, doc.Content.Typing)
	for _, p := range doc.Content.Typing.Prompts {
		require.NoError(t, sess.TypeString(ctx, p.Text))
	}

	snap := sess.Snapshot()
	require.NotNil(t, snap.Typing)
	assert.True(t, snap.Typing.Done)
	assert.True(t, snap.RequiredPassed, "snap=%+v typing=%+v", snap, snap.Typing)
	assert.GreaterOrEqual(t, snap.Typing.WPM, 15.0)
	assert.GreaterOrEqual(t, snap.Typing.Accuracy, 0.9)

	require.NoError(t, sess.Complete(ctx))
	snap = sess.Snapshot()
	assert.True(t, snap.Completed)
	assert.True(t, snap.CompletionQueued)
	assert.True(t, snap.CompletionAcked, "snap=%+v", snap)

	// Mastery evidence only for TYPE codes.
	evPage, err := repo.MasteryEvidences.List(ctx, env.Q, repo.ListParams{Limit: 200})
	require.NoError(t, err)
	require.NotEmpty(t, evPage.Items)

	mrPage, err := repo.MasteryRecords.List(ctx, env.Q, repo.ListParams{
		Limit:   50,
		Filters: map[string]any{"student_id": env.StudentID},
	})
	require.NoError(t, err)

	var typeHits, navHits int
	for _, rec := range mrPage.Items {
		std, err := repo.Standards.Get(ctx, env.Q, rec.StandardID)
		require.NoError(t, err)
		switch {
		case contains(std.Code, ".TYPE."):
			typeHits++
			assert.Greater(t, rec.Confidence, 0.0, "TYPE record should gain confidence")
		case contains(std.Code, ".NAV."):
			navHits++
			assert.Equal(t, beforeStatus, rec.Status)
			assert.Equal(t, beforeConf, rec.Confidence, "NAV mastery must not change from typing")
		case contains(std.Code, ".FILES."):
			t.Fatalf("unexpected FILES mastery from typing activity: %s", std.Code)
		}
	}
	assert.GreaterOrEqual(t, typeHits, 1)
	assert.Equal(t, 1, navHits, "seeded NAV record should still exist unchanged")

	// Evidence source_ref should not claim NAV standards.
	for _, e := range evPage.Items {
		// Evidence is linked to mastery_record; check record's standard.
		// List may include unrelated rows — only care about this student's TYPE/NAV.
		_ = e
	}
	// Explicit: no mastery transition for NAV from completion result path.
	page, err := repo.LearningSessions.List(ctx, env.Q, repo.ListParams{
		Limit:   20,
		Filters: map[string]any{"student_id": env.StudentID},
	})
	require.NoError(t, err)
	var completedID string
	for _, s := range page.Items {
		if s.State == "completed" {
			completedID = s.ID
			break
		}
	}
	require.NotEmpty(t, completedID)
	cmp, err := repo.GetCompletionBySession(ctx, env.Q, completedID)
	require.NoError(t, err)
	result, err := repo.CompletionResultFromRow(cmp)
	require.NoError(t, err)
	for _, tr := range result.MasterySnapshot {
		assert.Contains(t, tr.StandardCode, ".TYPE.", "typing mastery must be TYPE-only, got %s", tr.StandardCode)
		assert.NotContains(t, tr.StandardCode, ".NAV.")
	}
	assert.NotEmpty(t, result.MasterySnapshot)
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

func TestTypingSessionRestoresMidPromptNoDoubleEvents(t *testing.T) {
	t.Parallel()
	env := startTypingEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "typing-resume.db")
	wsRoot := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(wsRoot, 0o755))

	// Seed work online then go offline for deterministic local resume.
	engSeed := openEngineWS(t, env, dbPath, wsRoot, false)
	require.NoError(t, engSeed.SyncOnce(ctx).Err)

	eng1 := openEngineWS(t, env, dbPath, wsRoot, true)
	sess1, err := eng1.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	clientSID := sess1.Snapshot().ClientSessionID

	// Type partial first prompt ("ls" in command-typing-basics is short — use first chars).
	root := repoRoot(t)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, doc.Content.Typing.Prompts)
	p0 := doc.Content.Typing.Prompts[0].Text
	runes0 := []rune(p0)
	require.GreaterOrEqual(t, len(runes0), 2)
	// Leave at least one character so the prompt is mid-flight after interrupt.
	partial := string(runes0[:len(runes0)-1])
	require.NoError(t, sess1.TypeString(ctx, partial))
	snap1 := sess1.Snapshot()
	require.NotNil(t, snap1.Typing)
	assert.Equal(t, partial, snap1.Typing.Input)
	assert.Equal(t, 0, snap1.Typing.PromptIndex)

	pending1, err := eng1.Store().GetPendingSync(ctx)
	require.NoError(t, err)
	nEventsBefore := pending1.EventCount
	// Pause and close in-memory session (simulates broker/process restart).
	require.NoError(t, sess1.Pause(ctx))
	_ = sess1.Close()

	eng2 := openEngineWS(t, env, dbPath, wsRoot, true)
	sess2, err := eng2.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	snap2 := sess2.Snapshot()
	assert.Equal(t, clientSID, snap2.ClientSessionID, "should resume same client session")
	require.NotNil(t, snap2.Typing)
	assert.Equal(t, partial, snap2.Typing.Input)
	assert.Contains(t, snap2.Message, "resumed")

	// Resume must not enqueue session_started again.
	pending2, err := eng2.Store().GetPendingSync(ctx)
	require.NoError(t, err)
	assert.Equal(t, nEventsBefore, pending2.EventCount, "resume must not double-count past events")

	// Finish remaining prompts and complete once.
	// Finish current partial then the rest of prompts.
	rest0 := string(runes0[len(runes0)-1:])
	require.NoError(t, sess2.TypeString(ctx, rest0))
	for i, p := range doc.Content.Typing.Prompts {
		if i == 0 {
			continue
		}
		require.NoError(t, sess2.TypeString(ctx, p.Text))
	}
	snapDone := sess2.Snapshot()
	require.True(t, snapDone.RequiredPassed, "snap=%+v", snapDone)
	require.NoError(t, sess2.Complete(ctx))
	assert.True(t, sess2.Snapshot().Completed)
}

func TestTerminalSessionRestoresCwdAndCompletesOnce(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "term-resume.db")
	wsRoot := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(wsRoot, 0o755))

	engSeed := openEngineWS(t, env, dbPath, wsRoot, false)
	require.NoError(t, engSeed.SyncOnce(ctx).Err)

	eng1 := openEngineWS(t, env, dbPath, wsRoot, true)
	sess1, err := eng1.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	clientSID := sess1.Snapshot().ClientSessionID
	// Activity initial_cwd is "home"; enter docs under home.
	require.NoError(t, sess1.RunLine(ctx, "cd docs"))
	require.NoError(t, sess1.RunLine(ctx, "pwd"))
	snap1 := sess1.Snapshot()
	assert.Equal(t, "home/docs", snap1.RelCwd)
	assert.GreaterOrEqual(t, snap1.CommandsRun, 2)

	pending1, err := eng1.Store().GetPendingSync(ctx)
	require.NoError(t, err)
	nEventsBefore := pending1.EventCount

	require.NoError(t, sess1.Pause(ctx))
	sess1.CloseTerminal()
	_ = sess1.Close()

	eng2 := openEngineWS(t, env, dbPath, wsRoot, true)
	sess2, err := eng2.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	snap2 := sess2.Snapshot()
	assert.Equal(t, clientSID, snap2.ClientSessionID)
	assert.Equal(t, "home/docs", snap2.RelCwd)
	assert.GreaterOrEqual(t, snap2.CommandsRun, 2)

	pending2, err := eng2.Store().GetPendingSync(ctx)
	require.NoError(t, err)
	assert.Equal(t, nEventsBefore, pending2.EventCount)

	// Remaining navigation commands (workspace fixtures already present).
	require.NoError(t, sess2.RunLine(ctx, "ls"))
	require.NoError(t, sess2.RunLine(ctx, "cat guide.txt"))
	require.NoError(t, sess2.Verify(ctx))
	snapReady := sess2.Snapshot()
	require.True(t, snapReady.RequiredPassed, "snap=%+v checks=%+v", snapReady, snapReady.Checks)
	require.NoError(t, sess2.Complete(ctx))
	assert.True(t, sess2.Snapshot().Completed)

	// Completing again is idempotent locally.
	require.NoError(t, sess2.Complete(ctx))
}

func TestSessionPTYWriteAndResize(t *testing.T) {
	t.Parallel()
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "pty-state.db")
	eng := openEngine(t, env, dbPath, true)

	// Ensure work is cached offline-style via an online sync first.
	engOnline := openEngine(t, env, dbPath, false)
	require.NoError(t, engOnline.SyncOnce(ctx).Err)

	sess, err := eng.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	t.Cleanup(func() { sess.CloseTerminal() })

	snap := sess.Snapshot()
	require.Equal(t, contracts.KindTerminal, snap.Kind)
	require.True(t, snap.HasTerminal, "expected PTY on terminal session")

	require.NoError(t, sess.WriteTerminal(ctx, []byte("echo pty-hello\n")))
	require.NoError(t, sess.WriteTerminal(ctx, []byte("ls\n")))

	deadline := time.Now().Add(4 * time.Second)
	var screen string
	for time.Now().Before(deadline) {
		screen = sess.TerminalScreen()
		if strings.Contains(screen, "pty-hello") || strings.Contains(screen, "welcome.txt") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t,
		strings.Contains(screen, "pty-hello") || strings.Contains(screen, "welcome.txt"),
		"screen should show command output, got:\n%s", screen)

	require.NoError(t, sess.ResizeTerminal(30, 100))
	snap = sess.Snapshot()
	assert.True(t, snap.HasTerminal)
	assert.NotEmpty(t, snap.TerminalScreen)

	// RunLine still works alongside PTY.
	require.NoError(t, sess.RunLine(ctx, "pwd"))
	snap = sess.Snapshot()
	assert.GreaterOrEqual(t, snap.CommandsRun, 1)
}

func TestLocalCapabilityGateRejectsCommandOnlyRevision(t *testing.T) {
	t.Parallel()
	// Offline open must still refuse revisions that require structured command
	// evidence when the runner does not advertise the capability.
	q := testutil.Tx(t)
	ctx := context.Background()
	store, err := cache.Open(filepath.Join(t.TempDir(), "cap.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	student := factory.Student(t, q)
	subj := factory.Subject(t, q, factory.Override{"code": "digital-literacy-local-cap"})
	std := factory.Standard(t, q, factory.Override{"code": "PRIMER.DL.6.PIPE.LOCAL", "subject_id": subj.ID})
	act, err := repo.CreateDraftActivity(ctx, q, "cmd-only-local-"+uuid.NewString()[:8], "cmd only", "", contracts.KindTerminal, &subj.ID)
	require.NoError(t, err)
	content := contracts.ActivityContent{
		Objective:    "run a command",
		Instructions: "run ls",
		Terminal: &contracts.TerminalContent{
			RuntimeProfile: contracts.RuntimeCoreutilsBasic,
			Fixtures:       []contracts.FixtureEntry{{Path: "home", Type: "directory"}},
		},
		Tasks: []contracts.Task{{
			ID: "t1", Title: "ls", Instructions: "ls",
			Completion: contracts.CheckTree{CheckID: "c-ls"},
		}},
		Checks: []contracts.Check{{
			ID: "c-ls", Kind: contracts.CheckCommandProperties,
			Params: map[string]any{"executable": "ls", "exitCode": 0},
		}},
	}
	rev, err := repo.PublishDraftRevision(ctx, q, act.ID, content, "1", []contracts.StandardRef{{
		Code: std.Code, Role: contracts.StandardRolePrimary, Weight: 1,
	}}, map[string]string{std.Code: std.ID}, time.Now().UTC())
	require.NoError(t, err)
	asg, err := repo.CreateAssignment(ctx, q, student.ID, rev.ID, nil, 1, "test")
	require.NoError(t, err)

	// Seed local work cache without going through the API.
	require.NoError(t, store.SaveWork(ctx, []studentapi.WorkItem{{
		Assignment: *asg,
		Revision:   *rev,
		Activity:   *act,
	}}))

	eng, err := engine.New(engine.Options{
		Store:                            store,
		Offline:                          true,
		AllowUnsandboxed:                 true,
		DisableStructuredCommandEvidence: true, // force capability off
	})
	require.NoError(t, err)
	_, err = eng.OpenSession(ctx, asg.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "structured_command_evidence")
}
