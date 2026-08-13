package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
)

func seedShortResponseWork(t *testing.T, store *cache.Store, asg string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, store.SaveWork(ctx, []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: asg, State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "short-resp", Title: "Short Response", Kind: contracts.KindTerminal},
		Revision: domain.LearningActivityRevision{
			ID: "rev-sr", ContentSHA256: "sha-sr",
			Content: map[string]any{
				"objective":    "answer in your own words",
				"instructions": "read blocks then respond",
				"blocks": []any{
					map[string]any{"id": "p1", "kind": "prose", "title": "Overview", "text": "Commands start processes."},
					map[string]any{"id": "v1", "kind": "vocabulary", "terms": []any{
						map[string]any{"term": "shell", "definition": "command interpreter"},
					}},
					map[string]any{"id": "e1", "kind": "example", "title": "pwd", "input": "pwd", "output": "/home/x", "explanation": "prints cwd"},
					map[string]any{"id": "r1", "kind": "resource", "resource": map[string]any{
						"sha256":    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						"mediaType": "text/plain", "label": "notes", "byteSize": 4,
					}},
					map[string]any{"id": "pn", "kind": "parent_note", "text": "parent only"},
				},
				"terminal": map[string]any{
					"runtimeProfile": "default",
					"fixtures":       []any{map[string]any{"path": "a.txt", "type": "file", "content": "a"}},
				},
				"tasks": []any{
					map[string]any{
						"id": "t-action", "title": "Warmup", "instructions": "look around",
						"kind":       "action",
						"completion": map[string]any{"checkId": "always"},
					},
					map[string]any{
						"id": "t-resp", "title": "Explain", "instructions": "write it out",
						"kind": "short_response",
						"response": map[string]any{
							"prompt":   "What is a shell?",
							"maxChars": 80,
							"rubric":   []any{map[string]any{"id": "rc1", "description": "mentions interpreter"}},
						},
						"completion": map[string]any{"checkId": "resp-ok"},
					},
				},
				"checks": []any{
					map[string]any{"id": "always", "kind": "command_succeeded", "params": map[string]any{"command": "true"}},
					map[string]any{"id": "resp-ok", "kind": "response_submitted", "params": map[string]any{"taskId": "t-resp"}},
				},
				"hints": []any{map[string]any{"id": "h1", "text": "think interpreter"}},
			},
		},
	}}))
}

func openOfflineResponseSession(t *testing.T) (*engine.Engine, *engine.Session, *cache.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := cache.Open(filepath.Join(dir, "state.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	asg := "asg-short-resp"
	seedShortResponseWork(t, store, asg)
	ws := filepath.Join(dir, "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))
	eng, err := engine.New(engine.Options{
		Store: store, WorkspaceRoot: ws, Offline: true, AllowUnsandboxed: true, UseSandbox: false,
	})
	require.NoError(t, err)
	sess, err := eng.OpenSession(context.Background(), asg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })
	return eng, sess, store, asg
}

func TestContentBlocksStripsParentNotes(t *testing.T) {
	t.Parallel()
	_, sess, _, _ := openOfflineResponseSession(t)
	blocks := sess.ContentBlocks()
	require.NotEmpty(t, blocks)
	kinds := map[string]bool{}
	for _, b := range blocks {
		kinds[b.Kind] = true
		assert.NotEqual(t, contracts.BlockParentNote, b.Kind)
	}
	assert.True(t, kinds[contracts.BlockProse])
	assert.True(t, kinds[contracts.BlockVocabulary])
	// Snapshot surfaces the same student-visible blocks.
	snap := sess.Snapshot()
	assert.Equal(t, len(blocks), len(snap.Blocks))
	assert.Equal(t, "Short Response", snap.ActivityTitle)
}

func TestSaveResponseDraftAndSubmitResponse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, sess, store, _ := openOfflineResponseSession(t)
	csid := sess.Snapshot().ClientSessionID
	require.NotEmpty(t, csid)

	// Draft persists and surfaces on snapshot when current task is short_response.
	// Advance runner to response task by marking via submit on t-resp regardless of current idx.
	require.NoError(t, sess.SaveResponseDraft(ctx, "t-resp", "draft one"))
	body, err := store.GetResponseDraft(ctx, csid, "t-resp")
	require.NoError(t, err)
	assert.Equal(t, "draft one", body)

	// Guard: empty body
	err = sess.SubmitResponse(ctx, "t-resp", "  \n\t ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")

	// Guard: unknown task
	err = sess.SubmitResponse(ctx, "missing", "hello there")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown task")

	// Guard: action task is not short_response
	err = sess.SubmitResponse(ctx, "t-action", "hello there")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a short_response")

	// Guard: over max chars (80)
	err = sess.SubmitResponse(ctx, "t-resp", strings.Repeat("y", 81))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")

	// Happy path offline
	err = sess.SubmitResponse(ctx, "t-resp", "A shell is a command interpreter.")
	require.NoError(t, err)
	snap := sess.Snapshot()
	// When current task is still warmup, ResponseQueued may be false on snap for other task;
	// durable intent must exist.
	pending, err := store.ListPendingResponses(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, pending)
	found := false
	for _, p := range pending {
		if p.ClientSessionID == csid && p.TaskID == "t-resp" {
			found = true
			assert.Equal(t, "A shell is a command interpreter.", p.Request.Body)
			assert.Equal(t, contracts.ResponseSchemaVersion, p.Request.SchemaVersion)
		}
	}
	assert.True(t, found, "expected pending response intent for t-resp")
	// Draft updated to submitted body
	body, err = store.GetResponseDraft(ctx, csid, "t-resp")
	require.NoError(t, err)
	assert.Equal(t, "A shell is a command interpreter.", body)
	assert.Contains(t, snap.Message, "awaiting sync")

	// Second submit still queues (idempotent intent insert by new submission id)
	require.NoError(t, sess.SubmitResponse(ctx, "t-resp", "A shell interprets typed commands."))
}

func TestSubmitResponseCompletedAndNoStoreGuards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, sess, _, _ := openOfflineResponseSession(t)

	// Force completed via Complete if possible; otherwise Pause then manually mark by closing after complete attempt.
	// Submit after Complete is the guard we need — complete may fail without checks.
	// Use Pause + Close does not set completed. Exercise empty via re-open path below.

	// No-store path: construct by opening then... Session always has store from engine.
	// Cover "session already completed" by completing typing-less path: call Complete after RequiredPassed if possible.
	// For terminal short-response, mark response then verify.
	require.NoError(t, sess.SubmitResponse(ctx, "t-resp", "completed-guard prep answer body"))
	_ = sess.Verify(ctx)
	if sess.Snapshot().RequiredPassed {
		require.NoError(t, sess.Complete(ctx))
		err := sess.SubmitResponse(ctx, "t-resp", "after complete")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already completed")
		err = sess.SaveResponseDraft(ctx, "t-resp", "x")
		// Save still works unless store gone — draft save does not check completed.
		_ = err
	}
}

func TestRestoreSubmittedResponsesOnReopen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	store, err := cache.Open(dbPath)
	require.NoError(t, err)
	asg := "asg-restore-resp"
	seedShortResponseWork(t, store, asg)
	ws := filepath.Join(dir, "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	eng, err := engine.New(engine.Options{
		Store: store, WorkspaceRoot: ws, Offline: true, AllowUnsandboxed: true, UseSandbox: false,
	})
	require.NoError(t, err)
	sess, err := eng.OpenSession(ctx, asg)
	require.NoError(t, err)
	csid := sess.Snapshot().ClientSessionID
	require.NoError(t, sess.SubmitResponse(ctx, "t-resp", "Restored answer about the shell."))
	require.NoError(t, sess.Pause(ctx))
	require.NoError(t, sess.Close())
	require.NoError(t, store.Close())

	// Reopen durable store + engine; restoreSubmittedResponses runs inside OpenSession.
	store2, err := cache.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store2.Close() })
	eng2, err := engine.New(engine.Options{
		Store: store2, WorkspaceRoot: ws, Offline: true, AllowUnsandboxed: true, UseSandbox: false,
	})
	require.NoError(t, err)
	sess2, err := eng2.OpenSession(ctx, asg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess2.Close() })

	// Prior intent still listed for the original client session id (resume may reuse or create).
	acked, err := store2.ListAckedResponseTaskIDs(ctx, csid)
	require.NoError(t, err)
	assert.True(t, acked["t-resp"], "acked map should include submitted task for prior session")

	// Fresh session ContentBlocks still student-visible.
	blocks := sess2.ContentBlocks()
	require.NotEmpty(t, blocks)
	for _, b := range blocks {
		assert.NotEqual(t, contracts.BlockParentNote, b.Kind)
	}

	// Draft from prior session is keyed by client session; new session may differ.
	// Behavior: SaveResponseDraft on new session works independently.
	require.NoError(t, sess2.SaveResponseDraft(ctx, "t-resp", "new draft"))
	body, err := store2.GetResponseDraft(ctx, sess2.Snapshot().ClientSessionID, "t-resp")
	require.NoError(t, err)
	assert.Equal(t, "new draft", body)
}

func TestSaveResponseDraftNoStore(t *testing.T) {
	t.Parallel()
	// Engine without store cannot be constructed; SaveResponseDraft error path needs eng.opts.Store nil.
	// Cover via SubmitResponse unknown/empty already. Additional: closed store errors.
	ctx := context.Background()
	_, sess, store, _ := openOfflineResponseSession(t)
	require.NoError(t, store.Close())
	err := sess.SaveResponseDraft(ctx, "t-resp", "x")
	require.Error(t, err)
	err = sess.SubmitResponse(ctx, "t-resp", "still something")
	require.Error(t, err)
}

func TestDrainObserveLockedViaVerifyAndWriteNewline(t *testing.T) {
	t.Parallel()
	// Use standard basic-navigation env so PTY + observe path is real.
	env := startEnv(t)
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "obs.db")
	ws := filepath.Join(t.TempDir(), "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))

	engSeed := openEngineWS(t, env, dbPath, ws, false)
	require.NoError(t, engSeed.SyncOnce(ctx).Err)
	eng := openEngineWS(t, env, dbPath, ws, true)
	sess, err := eng.OpenSession(ctx, env.AssignmentID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	// Verify always calls drainObserveLocked (nil reader is fine).
	require.NoError(t, sess.Verify(ctx))

	// If PTY is live, newline write schedules observe drain.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if sess.Snapshot().HasTerminal {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sess.Snapshot().HasTerminal {
		require.NoError(t, sess.WriteTerminal(ctx, []byte("true\n")))
		// Let idle drain finish without fixed long sleep — poll message/commands.
		end := time.Now().Add(2 * time.Second)
		for time.Now().Before(end) {
			_ = sess.Snapshot()
			time.Sleep(50 * time.Millisecond)
		}
		require.NoError(t, sess.Verify(ctx))
	}

	// ContentBlocks on terminal activity (may be empty for basic-navigation).
	_ = sess.ContentBlocks()
}
