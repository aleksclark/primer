package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestClientOptionsAndErrorTypes(t *testing.T) {
	t.Parallel()
	var sawUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUA = r.Header.Get("User-Agent")
		switch {
		case r.URL.Path == "/student/profile":
			_ = json.NewEncoder(w).Encode(studentapi.StudentProfile{
				DeviceID: "d1", DeviceName: "ws",
				Student: domain.Student{ID: "s1", FirstName: "A"},
			})
		case strings.HasSuffix(r.URL.Path, "/tutor/messages"):
			_ = json.NewEncoder(w).Encode(studentapi.TutorMessageResponse{Reply: "try pwd"})
		case strings.HasSuffix(r.URL.Path, "/complete"):
			_ = json.NewEncoder(w).Encode(contracts.CompletionResult{Accepted: true, CompletionID: "c1"})
		case r.URL.Path == "/boom":
			http.Error(w, strings.Repeat("x", 300), http.StatusTeapot)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	cl := studentapi.New(srv.URL+"/", "tok",
		studentapi.WithHTTPClient(srv.Client()),
		studentapi.WithUserAgent("test-agent/1.0"),
	)
	assert.Equal(t, srv.URL, cl.BaseURL) // trailing slash stripped
	assert.Equal(t, "tok", cl.Token)

	ctx := context.Background()
	prof, err := cl.Profile(ctx)
	require.NoError(t, err)
	assert.Equal(t, "d1", prof.DeviceID)
	assert.Equal(t, "test-agent/1.0", sawUA)

	tm, err := cl.TutorMessage(ctx, "sess-1", "help")
	require.NoError(t, err)
	assert.Equal(t, "try pwd", tm.Reply)

	cr, err := cl.Complete(ctx, "sess-1", contracts.CompletionRequest{CompletionID: "c1"})
	require.NoError(t, err)
	assert.True(t, cr.Accepted)

	// Unauthorized without token
	cl2 := studentapi.New(srv.URL, "")
	_, err = cl2.Profile(ctx)
	var uerr *studentapi.ErrUnauthorized
	require.ErrorAs(t, err, &uerr)
	assert.Contains(t, uerr.Error(), "missing device token")
	assert.Equal(t, "unauthorized", (&studentapi.ErrUnauthorized{}).Error())

	// HTTP error truncates body
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, strings.Repeat("e", 250), http.StatusBadRequest)
	}))
	t.Cleanup(errSrv.Close)
	cl3 := studentapi.New(errSrv.URL, "tok")
	_, err = cl3.Profile(ctx)
	var herr *studentapi.ErrHTTP
	require.ErrorAs(t, err, &herr)
	assert.Equal(t, http.StatusBadRequest, herr.StatusCode)
	assert.Contains(t, herr.Error(), "HTTP 400")
	assert.Contains(t, herr.Error(), "…")
}

// Ensure SetToken works after construction.
func TestClientSetToken(t *testing.T) {
	t.Parallel()
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(studentapi.WorkResponse{})
	}))
	t.Cleanup(srv.Close)
	cl := studentapi.New(srv.URL, "")
	cl.SetToken("abc")
	_, err := cl.Work(context.Background(), "", 10)
	require.NoError(t, err)
	assert.Equal(t, "Bearer abc", auth)
}
