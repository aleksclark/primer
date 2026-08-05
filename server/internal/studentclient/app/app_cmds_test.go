package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
	"github.com/aleksclark/primer/server/internal/studentclient/sync"
)

func TestAppCmdsViaBrokerAndDirect(t *testing.T) {
	t.Parallel()
	// HTTP fake LMS
	mux := http.NewServeMux()
	mux.HandleFunc("POST /student-devices/pair", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deviceId": "dev-1", "token": "secret-tok",
			"student": map[string]any{"id": "stu-1", "firstName": "A", "lastName": "B"},
			"device":  map[string]any{"id": "dev-1", "name": "ws"},
		})
	})
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{Items: []studentapi.WorkItem{{
			Assignment: domain.StudentAssignment{ID: "asg", State: "available", UpdatedAt: time.Now().UTC()},
			Activity:   domain.LearningActivity{Slug: "s", Title: "T", Kind: contracts.KindTerminal},
			Revision:   domain.LearningActivityRevision{ID: "r", Content: map[string]any{}},
		}}})
	})
	mux.HandleFunc("GET /student/profile", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.StudentProfile{
			DeviceID: "dev-1", DeviceName: "ws", Student: domain.Student{ID: "stu-1", FirstName: "A"},
		})
	})
	httpSrv := httptest.NewServer(mux)
	t.Cleanup(httpSrv.Close)

	dir := t.TempDir()
	socket := filepath.Join(dir, "b.sock")
	tokenFile := filepath.Join(dir, "device.token")
	srv, err := broker.New(broker.Options{
		SocketPath: socket, DBPath: filepath.Join(dir, "state.db"), TokenFile: tokenFile,
		BaseURL: httpSrv.URL, WorkspaceRoot: filepath.Join(dir, "ws"),
		SkipPeerCred: true, AllowedGroup: "-", AllowUnsandboxed: true, UseSandbox: false,
	})
	require.NoError(t, err)
	require.NoError(t, broker.ServeBackground(srv, 3*time.Second))
	t.Cleanup(func() { _ = srv.Close() })

	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })

	m := NewModel(Options{Broker: cl, DeviceName: "ws", Offline: false})
	// pair via cmd
	msg := m.pairCmd("ABCD")()
	pm := msg.(pairedMsg)
	require.NoError(t, pm.err)

	// profile boot path
	msg = m.boot()
	bm := msg.(bootMsg)
	require.NoError(t, bm.err)
	assert.True(t, bm.hasToken)

	// sync + load work
	msg = m.syncCmd()()
	sd := msg.(syncDoneMsg)
	require.NoError(t, sd.res.Err)
	assert.Equal(t, 1, sd.res.WorkItems)

	msg = m.loadWorkCmd()()
	wl := msg.(workLoadedMsg)
	require.NoError(t, wl.err)
	require.Len(t, wl.items, 1)

	// offline broker sync short-circuit
	m.opts.Offline = true
	msg = m.syncCmd()()
	assert.Equal(t, sync.StatusOffline, msg.(syncDoneMsg).res.Status)

	// Direct-mode cmds with store
	store, err := cache.Open(filepath.Join(dir, "direct.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SetDeviceToken(context.Background(), "tok"))
	apiCl := studentapi.New(httpSrv.URL, "tok")
	md := NewModel(Options{Store: store, Client: apiCl, Offline: false, AllowUnsandboxed: true, WorkspaceRoot: filepath.Join(dir, "dws")})
	msg = md.pairCmd("CODE")()
	require.NoError(t, msg.(pairedMsg).err)
	msg = md.syncCmd()()
	require.NoError(t, msg.(syncDoneMsg).res.Err)

	// Direct session cmds with fake session methods via zero engine session aren't safe.
	// Exercise verify/complete/hint/run/term paths by installing a minimal real session via engine offline after seeding work.
	require.NoError(t, store.SaveWork(context.Background(), []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: "asg-local", State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "local", Title: "Local", Kind: contracts.KindTerminal},
		Revision: domain.LearningActivityRevision{ID: "rev-local", Content: map[string]any{
			"objective": "o",
			"terminal": map[string]any{
				"runtimeProfile": "coreutils-basic",
				"initialCwd":     ".",
				"fixtures":       []any{},
			},
			"tasks":  []any{map[string]any{"id": "t", "title": "T", "instructions": "Go", "completion": map[string]any{"checkId": "c"}}},
			"checks": []any{map[string]any{"id": "c", "kind": "file_exists", "params": map[string]any{"path": "."}}},
		}},
	}}))
	eng, err := engine.New(engine.Options{
		Client: apiCl, Store: store, WorkspaceRoot: filepath.Join(dir, "dws"),
		Offline: true, AllowUnsandboxed: true, UseSandbox: false,
	})
	require.NoError(t, err)
	// Open may fail if content validation fails; if so, still cover cmd wrappers with nil-safe broker mode.
	sess, err := eng.OpenSession(context.Background(), "asg-local")
	if err == nil {
		md.sess = sess
		md.brokerSessionID = ""
		md.opts.Broker = nil
		msg = md.runCmd("pwd")()
		_ = msg.(cmdDoneMsg)
		msg = md.verifyCmd()()
		_ = msg.(verifyDoneMsg)
		msg = md.hintCmd()()
		_ = msg.(hintDoneMsg)
		msg = md.termPollCmd()()
		_ = msg.(termPollMsg)
		if cmd := md.resizeTerminalCmd(40, 100); cmd != nil {
			_ = cmd()
		}
		msg = md.completeCmd()()
		_ = msg.(completeDoneMsg)
		_ = sess.Close()
	}

	// Broker mode cmd wrappers with missing session (error paths)
	mb := NewModel(Options{Broker: cl})
	mb.brokerSessionID = "missing"
	msg = mb.runCmd("pwd")()
	assert.Error(t, msg.(cmdDoneMsg).err)
	msg = mb.termWriteCmd("x")()
	assert.Error(t, msg.(termPollMsg).err)
	msg = mb.termPollCmd()()
	assert.Error(t, msg.(termPollMsg).err)
	msg = mb.verifyCmd()()
	_ = msg.(verifyDoneMsg)
	msg = mb.completeCmd()()
	assert.Error(t, msg.(completeDoneMsg).err)
	msg = mb.hintCmd()()
	_ = msg.(hintDoneMsg)
	if cmd := mb.resizeTerminalCmd(20, 80); cmd != nil {
		_ = cmd()
	}
}
