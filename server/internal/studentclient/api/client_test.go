package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestClientPairAndWork(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /student-devices/pair", func(w http.ResponseWriter, r *http.Request) {
		var body studentapi.PairRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "ABCD", body.Code)
		_ = json.NewEncoder(w).Encode(studentapi.PairResponse{
			DeviceID: "dev-1",
			Token:    "tok-1",
			Student:  domain.Student{ID: "stu-1", FirstName: "Ada"},
			Device:   domain.StudentDevice{ID: "dev-1", Name: "ws"},
		})
	})
	mux.HandleFunc("GET /student/work", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok-1", r.Header.Get("Authorization"))
		assert.Equal(t, "tok-1", r.Header.Get("X-Device-Token"))
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{
			Items: []studentapi.WorkItem{{
				Assignment: domain.StudentAssignment{ID: "asg-1", State: "available"},
				Activity:   domain.LearningActivity{Slug: "basic-navigation", Title: "Basic Navigation"},
				Revision:   domain.LearningActivityRevision{ID: "rev-1", ContentSHA256: "abc"},
			}},
		})
	})
	mux.HandleFunc("GET /student/profile", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			http.Error(w, "nope", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(studentapi.StudentProfile{
			DeviceID: "dev-1", DeviceName: "ws",
			Student: domain.Student{ID: "stu-1", FirstName: "Ada"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cl := studentapi.New(srv.URL, "")
	pair, err := cl.Pair(context.Background(), "ABCD", "ws")
	require.NoError(t, err)
	assert.Equal(t, "tok-1", pair.Token)
	assert.Equal(t, "tok-1", cl.Token)

	prof, err := cl.Profile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Ada", prof.Student.FirstName)

	work, err := cl.Work(context.Background(), "", 10)
	require.NoError(t, err)
	require.Len(t, work.Items, 1)
	assert.Equal(t, "basic-navigation", work.Items[0].Activity.Slug)
}

func TestClientUnauthorized(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "revoked", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	cl := studentapi.New(srv.URL, "dead")
	_, err := cl.Profile(context.Background())
	require.Error(t, err)
	var uerr *studentapi.ErrUnauthorized
	require.ErrorAs(t, err, &uerr)
}

func TestClientSessionLifecycle(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /student/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(domain.LearningSession{
			ID: "sess-1", AssignmentID: "asg-1", ClientSessionID: "c1", State: "started",
		})
	})
	mux.HandleFunc("POST /student/sessions/sess-1/events", func(w http.ResponseWriter, r *http.Request) {
		var body studentapi.EventsRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body.Events, 1)
		_ = json.NewEncoder(w).Encode(studentapi.EventsAck{AcknowledgedSequence: body.Events[0].Sequence})
	})
	mux.HandleFunc("POST /student/sessions/sess-1/artifacts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(domain.LearningSessionArtifact{ID: "row-1", ArtifactID: "art-1"})
	})
	mux.HandleFunc("POST /student/sessions/sess-1/complete", func(w http.ResponseWriter, r *http.Request) {
		var req contracts.CompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		_ = json.NewEncoder(w).Encode(contracts.CompletionResult{
			SchemaVersion: contracts.CompletionSchemaVersion,
			CompletionID:  req.CompletionID,
			Accepted:      true,
			RequestDigest: req.RequestDigest,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cl := studentapi.New(srv.URL, "tok")
	sess, err := cl.StartSession(context.Background(), "c1", "asg-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", sess.ID)

	ack, err := cl.PostEvents(context.Background(), sess.ID, []contracts.SessionEvent{{
		SchemaVersion: contracts.EventSchemaVersion,
		EventID:       "e1",
		Type:          contracts.EventSessionStarted,
		Sequence:      0,
		ClientTime:    time.Now().UTC(),
	}})
	require.NoError(t, err)
	assert.Equal(t, int64(0), ack.AcknowledgedSequence)

	_, err = cl.PostArtifact(context.Background(), sess.ID, contracts.ArtifactMeta{
		SchemaVersion: contracts.ArtifactSchemaVersion,
		ArtifactID:    "art-1",
		Filename:      "a.txt",
		MediaType:     "text/plain",
		ByteSize:      1,
		SHA256:        "aa",
		CreatedAt:     time.Now().UTC(),
	})
	require.NoError(t, err)

	res, err := cl.Complete(context.Background(), sess.ID, contracts.CompletionRequest{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  "cmp-1",
		RequestDigest: "d1",
		ClientTime:    time.Now().UTC(),
	})
	require.NoError(t, err)
	assert.True(t, res.Accepted)
}
