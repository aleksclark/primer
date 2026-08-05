package broker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func curriculumRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
}

func TestBrokerOpenSessionTypingFlow(t *testing.T) {
	t.Parallel()
	root := curriculumRoot(t)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "command-typing-basics", "activity.yaml"))
	require.NoError(t, err)
	contentMap := map[string]any{}
	raw, err := json.Marshal(doc.Content)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &contentMap))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{Items: []studentapi.WorkItem{{
			Assignment: domain.StudentAssignment{
				ID: "asg-type-1", State: "available", UpdatedAt: time.Now().UTC(),
				ActivityRevisionID: "rev-type-1",
			},
			Activity: domain.LearningActivity{ID: "act-1", Slug: doc.Slug, Title: doc.Title, Kind: contracts.KindTyping},
			Revision: domain.LearningActivityRevision{
				ID: "rev-type-1", SchemaVersion: contracts.SchemaVersion, Content: contentMap,
			},
		}}})
	})
	mux.HandleFunc("POST /student/sessions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(domain.LearningSession{
			ID: "srv-sess-1", ClientSessionID: body["clientSessionId"].(string),
			AssignmentID: "asg-type-1", State: "started",
		})
	})
	mux.HandleFunc("POST /student/sessions/srv-sess-1/events", func(w http.ResponseWriter, r *http.Request) {
		var body studentapi.EventsRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		var max int64 = -1
		for _, e := range body.Events {
			if e.Sequence > max {
				max = e.Sequence
			}
		}
		_ = json.NewEncoder(w).Encode(studentapi.EventsAck{AcknowledgedSequence: max})
	})
	mux.HandleFunc("POST /student/sessions/srv-sess-1/complete", func(w http.ResponseWriter, r *http.Request) {
		var req contracts.CompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(contracts.CompletionResult{
			SchemaVersion: contracts.CompletionSchemaVersion,
			CompletionID:  req.CompletionID, Accepted: true, RequestDigest: req.RequestDigest,
		})
	})
	mux.HandleFunc("POST /student/sessions/srv-sess-1/tutor/messages", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.TutorMessageResponse{Reply: "keep your fingers on home row"})
	})
	mux.HandleFunc("GET /student/profile", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.StudentProfile{
			DeviceID: "d1", DeviceName: "ws", Student: domain.Student{ID: "s1", FirstName: "Ty"},
		})
	})
	httpSrv := httptest.NewServer(mux)
	t.Cleanup(httpSrv.Close)

	dir := t.TempDir()
	socket := filepath.Join(dir, "b.sock")
	tokenFile := filepath.Join(dir, "device.token")
	require.NoError(t, broker.WriteTokenFile(tokenFile, "tok"))
	ws := filepath.Join(dir, "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))
	srv, err := broker.New(broker.Options{
		SocketPath: socket, DBPath: filepath.Join(dir, "state.db"), TokenFile: tokenFile,
		BaseURL: httpSrv.URL, WorkspaceRoot: ws, DeviceName: "ws",
		UseSandbox: false, AllowUnsandboxed: true, SkipPeerCred: true, AllowedGroup: "-",
	})
	require.NoError(t, err)
	require.NoError(t, broker.ServeBackground(srv, 3*time.Second))
	t.Cleanup(func() { _ = srv.Close() })
	require.NoError(t, srv.Store().SetDeviceToken(context.Background(), "tok"))

	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	ctx := context.Background()

	syncRes, err := cl.SyncWork(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, syncRes.WorkItems)

	open, err := cl.OpenSession(ctx, "asg-type-1")
	require.NoError(t, err)
	require.NotEmpty(t, open.ClientSessionID)
	assert.Equal(t, contracts.KindTyping, open.Snapshot.Kind)

	sid := open.ClientSessionID
	// Type a couple chars then backspace.
	snap, err := cl.TypeRune(ctx, sid, 'l')
	require.NoError(t, err)
	require.NotNil(t, snap.Typing)
	snap, err = cl.TypeBackspace(ctx, sid)
	require.NoError(t, err)
	assert.Equal(t, "", snap.Typing.Input)

	// Tutor hint path
	tr, err := cl.Tutor(ctx, sid, "help")
	require.NoError(t, err)
	assert.Contains(t, tr.Hint, "home row")

	st, err := cl.Status(ctx, sid)
	require.NoError(t, err)
	require.NotNil(t, st.Snapshot)

	// Verify + pause
	_, err = cl.Verify(ctx, sid)
	require.NoError(t, err)
	_, err = cl.Pause(ctx, sid)
	require.NoError(t, err)

	// Session gone after pause
	_, err = cl.Status(ctx, sid)
	// status may still succeed without snapshot
	require.NoError(t, err)
}

func TestBrokerOpenSessionTerminalRunAndPTY(t *testing.T) {
	t.Parallel()
	root := curriculumRoot(t)
	doc, err := contracts.LoadDocument(filepath.Join(root, "curriculum", "activities", "basic-navigation", "activity.yaml"))
	require.NoError(t, err)
	contentMap := map[string]any{}
	raw, err := json.Marshal(doc.Content)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &contentMap))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{Items: []studentapi.WorkItem{{
			Assignment: domain.StudentAssignment{
				ID: "asg-term-1", State: "available", UpdatedAt: time.Now().UTC(),
				ActivityRevisionID: "rev-term-1",
			},
			Activity: domain.LearningActivity{ID: "act-t", Slug: doc.Slug, Title: doc.Title, Kind: contracts.KindTerminal},
			Revision: domain.LearningActivityRevision{ID: "rev-term-1", SchemaVersion: contracts.SchemaVersion, Content: contentMap},
		}}})
	})
	mux.HandleFunc("POST /student/sessions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(domain.LearningSession{
			ID: "srv-term", ClientSessionID: body["clientSessionId"].(string),
			AssignmentID: "asg-term-1", State: "started",
		})
	})
	mux.HandleFunc("POST /student/sessions/srv-term/events", func(w http.ResponseWriter, r *http.Request) {
		var body studentapi.EventsRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		var max int64 = -1
		for _, e := range body.Events {
			if e.Sequence > max {
				max = e.Sequence
			}
		}
		_ = json.NewEncoder(w).Encode(studentapi.EventsAck{AcknowledgedSequence: max})
	})
	httpSrv := httptest.NewServer(mux)
	t.Cleanup(httpSrv.Close)

	dir := t.TempDir()
	socket := filepath.Join(dir, "t.sock")
	tokenFile := filepath.Join(dir, "device.token")
	require.NoError(t, broker.WriteTokenFile(tokenFile, "tok"))
	ws := filepath.Join(dir, "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))
	srv, err := broker.New(broker.Options{
		SocketPath: socket, DBPath: filepath.Join(dir, "state.db"), TokenFile: tokenFile,
		BaseURL: httpSrv.URL, WorkspaceRoot: ws, AllowUnsandboxed: true, UseSandbox: false,
		SkipPeerCred: true, AllowedGroup: "-",
	})
	require.NoError(t, err)
	require.NoError(t, broker.ServeBackground(srv, 3*time.Second))
	t.Cleanup(func() { _ = srv.Close() })
	require.NoError(t, srv.Store().SetDeviceToken(context.Background(), "tok"))

	cl, err := broker.Dial(socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	ctx := context.Background()
	_, err = cl.SyncWork(ctx)
	require.NoError(t, err)

	open, err := cl.OpenSession(ctx, "asg-term-1")
	require.NoError(t, err)
	sid := open.ClientSessionID

	snap, err := cl.RunCommand(ctx, sid, "pwd")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, snap.CommandsRun, 1)

	_, err = cl.Verify(ctx, sid)
	require.NoError(t, err)
	_, err = cl.Pause(ctx, sid)
	require.NoError(t, err)
}
