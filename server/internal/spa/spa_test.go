package spa

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerServesIndex(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
}

func TestHandlerFallsBackToIndexForClientRoutes(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(Handler())
	defer srv.Close()

	// Unknown paths (client-side routes) must serve index.html, not 404.
	for _, route := range []string{"/students", "/mastery-records", "/deep/nested/route"} {
		resp, err := http.Get(srv.URL + route)
		require.NoError(t, err, route)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, route)
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/html", route)
	}
}

func TestHandlerServesExistingFiles(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/index.html")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
