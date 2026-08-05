package broker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	studentsync "github.com/aleksclark/primer/server/internal/studentclient/sync"
)

func TestProfileSyncStatusAndOfflineSyncWork(t *testing.T) {
	t.Parallel()
	socket, srv, tokenFile := startTestBroker(t, "http://127.0.0.1:1", true)
	ctx := context.Background()
	require.NoError(t, broker.WriteTokenFile(tokenFile, "tok-profile"))
	require.NoError(t, srv.Store().SetDeviceToken(ctx, "tok-profile"))
	require.NoError(t, srv.Store().SetDeviceIdentity(ctx, "dev-1", "stu-1", "ws"))

	// Force offline so sync/profile skip network.
	// Recreate with Offline true.
	_ = srv.Close()
	dir := filepath.Dir(socket)
	socket = filepath.Join(dir, "broker-off.sock")
	srv, err := broker.New(broker.Options{
		SocketPath: socket, DBPath: filepath.Join(dir, "state.db"), TokenFile: tokenFile,
		BaseURL: "http://127.0.0.1:1", WorkspaceRoot: filepath.Join(dir, "ws"),
		SkipPeerCred: true, AllowedGroup: "-", Offline: true, AllowUnsandboxed: true,
	})
	require.NoError(t, err)
	require.NoError(t, broker.ServeBackground(srv, 3*time.Second))
	t.Cleanup(func() { _ = srv.Close() })

	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })

	prof, err := cl.Profile(ctx)
	require.NoError(t, err)
	assert.True(t, prof.Paired)
	assert.Equal(t, "dev-1", prof.DeviceID)
	assert.Equal(t, "stu-1", prof.StudentID)
	assert.NotEmpty(t, prof.SupportedKinds)

	syncRes, err := cl.SyncWork(ctx)
	require.NoError(t, err)
	assert.Equal(t, string(studentsync.StatusOffline), syncRes.Status)

	st, err := cl.Status(ctx, "")
	require.NoError(t, err)
	assert.True(t, st.Health.Paired)

	// Unknown session ops error cleanly.
	_, err = cl.RunCommand(ctx, "missing", "pwd")
	require.Error(t, err)
	_, err = cl.Verify(ctx, "missing")
	require.Error(t, err)
	_, err = cl.Complete(ctx, "missing")
	require.Error(t, err)
	_, err = cl.Pause(ctx, "missing")
	require.Error(t, err)
	_, err = cl.TerminalWrite(ctx, "missing", "x")
	require.Error(t, err)
	_, err = cl.TerminalRead(ctx, "missing")
	require.Error(t, err)
	_, err = cl.TerminalResize(ctx, "missing", 24, 80)
	require.Error(t, err)
	_, err = cl.TypeRune(ctx, "missing", 'a')
	require.Error(t, err)
	_, err = cl.TypeBackspace(ctx, "missing")
	require.Error(t, err)
	_, err = cl.Tutor(ctx, "missing", "help")
	require.Error(t, err)
	_, err = cl.OpenSession(ctx, "")
	require.Error(t, err)
}

func TestSyncWorkOnlinePullsQueue(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{Items: []studentapi.WorkItem{{
			Assignment: domain.StudentAssignment{ID: "asg-1", State: "available", UpdatedAt: time.Now().UTC()},
			Activity:   domain.LearningActivity{Slug: "basic-navigation", Title: "Nav"},
			Revision:   domain.LearningActivityRevision{ID: "rev-1", Content: map[string]any{}},
		}}})
	})
	mux.HandleFunc("GET /student/profile", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.StudentProfile{
			DeviceID: "dev-x", DeviceName: "ws",
			Student: domain.Student{ID: "stu-x", FirstName: "Ada", LastName: "L"},
		})
	})
	httpSrv := httptest.NewServer(mux)
	t.Cleanup(httpSrv.Close)

	socket, srv, tokenFile := startTestBroker(t, httpSrv.URL, true)
	ctx := context.Background()
	require.NoError(t, broker.WriteTokenFile(tokenFile, "tok-sync"))
	require.NoError(t, srv.Store().SetDeviceToken(ctx, "tok-sync"))
	require.NoError(t, srv.Store().SetDeviceIdentity(ctx, "dev-x", "stu-x", "ws"))

	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })

	res, err := cl.SyncWork(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.WorkItems)
	assert.Equal(t, string(studentsync.StatusOnline), res.Status)

	items, err := cl.ListWork(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "basic-navigation", items[0].Activity.Slug)

	prof, err := cl.Profile(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Ada", prof.FirstName)

	assert.Equal(t, studentsync.StatusOnline, broker.SyncStatus("online"))
	assert.Equal(t, studentsync.StatusIdle, broker.SyncStatus(""))
	assert.Equal(t, studentsync.Status("custom"), broker.SyncStatus("custom"))

	// SyncResultFrom maps errors without secrets.
	out := broker.SyncResultFrom(studentsync.Result{Status: studentsync.StatusOffline, Err: assert.AnError, WorkItems: 2})
	assert.Equal(t, 2, out.WorkItems)
	assert.Contains(t, out.Error, "assert.AnError")
}

func TestUnpairedProfile(t *testing.T) {
	t.Parallel()
	socket, _, _ := startTestBroker(t, "http://127.0.0.1:1", true)
	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	prof, err := cl.Profile(context.Background())
	require.NoError(t, err)
	assert.False(t, prof.Paired)
	assert.NotEmpty(t, prof.SupportedKinds)
}

func TestClientStoreAccessors(t *testing.T) {
	t.Parallel()
	socket, srv, _ := startTestBroker(t, "http://127.0.0.1:1", true)
	require.NotNil(t, srv.Store())
	require.NotNil(t, srv.Client())
	_ = socket
	// Seed pending event and ensure health reports it.
	ctx := context.Background()
	csid := uuid.NewString()
	require.NoError(t, srv.Store().SaveSession(ctx, cache.Session{
		ClientSessionID: csid, AssignmentID: uuid.NewString(),
		ActivityRevisionID: uuid.NewString(), State: "started",
		LastAckedSequence: -1, NextSequence: 0,
	}))
	_, err := srv.Store().EnqueueEvent(ctx, csid, contracts.SessionEvent{
		SchemaVersion: "1", EventID: uuid.NewString(), Type: "session.started", Sequence: 0,
	})
	require.NoError(t, err)
	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	h, err := cl.Health(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, h.PendingEvents, 1)
}
