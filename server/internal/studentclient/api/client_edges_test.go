package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
)

func TestClientTruncateOnDecodeError(t *testing.T) {
	t.Parallel()
	// Return 200 with invalid JSON longer than 200 chars to hit truncate().
	body := `{"not":"valid-for-profile",` + strings.Repeat("x", 300) + `}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	cl := studentapi.New(srv.URL, "tok")
	_, err := cl.Profile(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
	assert.Contains(t, err.Error(), "…")
}

func TestClientHTTPErrorBodies(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "unauthorized") {
			http.Error(w, "nope", http.StatusUnauthorized)
			return
		}
		http.Error(w, "server broke", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	cl := studentapi.New(srv.URL, "tok")

	// Force path that returns 500 via Profile against generic server.
	_, err := cl.Profile(t.Context())
	require.Error(t, err)
	var he *studentapi.ErrHTTP
	require.ErrorAs(t, err, &he)
	assert.Equal(t, 500, he.StatusCode)

	// Unauthorized
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "revoked", http.StatusUnauthorized)
	}))
	t.Cleanup(srv2.Close)
	cl2 := studentapi.New(srv2.URL, "tok")
	_, err = cl2.Profile(t.Context())
	require.Error(t, err)
	var ue *studentapi.ErrUnauthorized
	require.ErrorAs(t, err, &ue)
}

func TestClientJSONRoundTripHelpers(t *testing.T) {
	t.Parallel()
	// Ensure Work response decode with empty body no-content-ish
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/student/work":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	cl := studentapi.New(srv.URL, "t")
	resp, err := cl.Work(t.Context(), "", 10)
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}
