package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/api"
)

type okPinger struct{}

func (okPinger) Ping(_ context.Context) error { return nil }

type failPinger struct{}

func (failPinger) Ping(_ context.Context) error { return errors.New("connection refused") }

func TestHealthzAlwaysOK(t *testing.T) {
	t.Parallel()
	h := api.New(api.Options{Pool: nil, Env: "test"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok"`)
}

func TestHealthzOKEvenWithNilPool(t *testing.T) {
	t.Parallel()
	h := api.New(api.Options{Pool: nil})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestReadyzOKWithWorkingPool(t *testing.T) {
	t.Parallel()
	h := api.New(api.Options{Pool: okPinger{}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ready"`)
}

func TestReadyzUnavailableWithNilPool(t *testing.T) {
	t.Parallel()
	h := api.New(api.Options{Pool: nil})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestReadyzUnavailableWhenPingFails(t *testing.T) {
	t.Parallel()
	h := api.New(api.Options{Pool: failPinger{}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "unavailable")
}

func TestRequestIDMiddlewareAssignsID(t *testing.T) {
	t.Parallel()
	h := api.New(api.Options{Pool: okPinger{}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestRequestIDMiddlewareAcceptsValidClientID(t *testing.T) {
	t.Parallel()
	h := api.New(api.Options{Pool: okPinger{}})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "client-id-123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, "client-id-123", rec.Header().Get("X-Request-ID"))
}

func TestRequestIDMiddlewareRejectsInvalidClientID(t *testing.T) {
	t.Parallel()
	h := api.New(api.Options{Pool: okPinger{}})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "bad id with spaces")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// A new UUID should be assigned instead of the bad one.
	rid := rec.Header().Get("X-Request-ID")
	require.NotEmpty(t, rid)
	assert.NotEqual(t, "bad id with spaces", rid)
}
