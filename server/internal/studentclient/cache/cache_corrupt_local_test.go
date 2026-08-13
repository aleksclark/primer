package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// Package-local tests exercise corrupt-row decode paths via direct SQL.
func TestCorruptWorkPayloadDecode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "corrupt-work.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	_, err = s.db.ExecContext(ctx, `INSERT INTO work_items(assignment_id, payload_json, updated_at) VALUES(?,?,?)`,
		"bad-1", "{not-json", time.Now().UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)

	_, err = s.ListWork(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")

	_, err = s.GetWork(ctx, "bad-1")
	require.Error(t, err)

	// CommitWorkSync soft-cancel skips corrupt payload rows without failing.
	_, err = s.db.ExecContext(ctx, `INSERT INTO work_items(assignment_id, payload_json, updated_at) VALUES(?,?,?)`,
		"good-1", mustJSONWork("good-1"), time.Now().UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)
	require.NoError(t, s.SetMeta(ctx, MetaWorkPageSeenIDs, `["good-1"]`))
	require.NoError(t, s.CommitWorkSync(ctx, "c1", studentapi.WorkModeSnapshot))

	// good-1 remains available; bad-1 left as-is (skipped corrupt).
	got, err := s.GetWork(ctx, "good-1")
	require.NoError(t, err)
	assert.Equal(t, "available", got.Assignment.State)
}

func TestCorruptEventPayloadAndCompletionResponse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "corrupt-ev.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	csid := uuid.NewString()
	require.NoError(t, s.SaveSession(ctx, Session{
		ClientSessionID: csid, AssignmentID: "a", State: "started",
		LastAckedSequence: -1, NextSequence: 0, ServerSessionID: "srv",
	}))

	// Insert outbox with valid JSON then list — happy.
	_, err = s.EnqueueEvent(ctx, csid, contracts.SessionEvent{
		EventID: uuid.NewString(), Type: contracts.EventTaskViewed, Sequence: -1,
		Payload: map[string]any{"taskId": "t"},
	})
	require.NoError(t, err)
	evs, err := s.ListPendingEvents(ctx, 10)
	require.NoError(t, err)
	require.NotEmpty(t, evs)

	// Corrupt payload is tolerated (best-effort unmarshal); events still list.
	_, err = s.db.ExecContext(ctx, `UPDATE event_outbox SET payload_json = '{bad' WHERE client_session_id = ?`, csid)
	require.NoError(t, err)
	evs, err = s.ListPendingEvents(ctx, 10)
	require.NoError(t, err)
	require.NotEmpty(t, evs)
	assert.Nil(t, evs[0].Event.Payload)

	// Completion with corrupt request_json fails GetCompletion.
	cid := uuid.NewString()
	require.NoError(t, s.SaveCompletionIntent(ctx, csid, "srv", contracts.CompletionRequest{
		SchemaVersion: "1", CompletionID: cid, RequestDigest: "d", ClientTime: time.Now().UTC(),
	}))
	_, err = s.db.ExecContext(ctx, `UPDATE completion_intents SET request_json = '{nope' WHERE completion_id = ?`, cid)
	require.NoError(t, err)
	_, err = s.GetCompletion(ctx, cid)
	require.Error(t, err)
}

func mustJSONWork(id string) string {
	return `{"assignment":{"id":"` + id + `","state":"available","updatedAt":"2020-01-01T00:00:00Z"},"activity":{"slug":"s"},"revision":{"id":"r","content":{}}}`
}
