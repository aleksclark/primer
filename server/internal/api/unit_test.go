package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/repo"
)

func TestNewServesHealthAndSpec(t *testing.T) {
	t.Parallel()
	_, handler := New(nil, Options{CORSOrigins: []string{"http://localhost:5173"}})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	spec, err := http.Get(srv.URL + "/openapi.json")
	require.NoError(t, err)
	defer spec.Body.Close()
	assert.Equal(t, http.StatusOK, spec.StatusCode)
}

func TestCORSMiddleware(t *testing.T) {
	t.Parallel()
	_, handler := New(nil, Options{CORSOrigins: []string{"http://localhost:5173"}})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Preflight from allowed origin
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/students", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "http://localhost:5173", resp.Header.Get("Access-Control-Allow-Origin"))

	// Request from disallowed origin gets no CORS headers
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/health", nil)
	req2.Header.Set("Origin", "http://evil.example")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Empty(t, resp2.Header.Get("Access-Control-Allow-Origin"))

	// Wildcard origin configuration
	_, wildcard := New(nil, Options{CORSOrigins: []string{"*"}})
	srv2 := httptest.NewServer(wildcard)
	defer srv2.Close()
	req3, _ := http.NewRequest(http.MethodGet, srv2.URL+"/health", nil)
	req3.Header.Set("Origin", "http://anything.example")
	resp3, err := http.DefaultClient.Do(req3)
	require.NoError(t, err)
	defer resp3.Body.Close()
	assert.Equal(t, "http://anything.example", resp3.Header.Get("Access-Control-Allow-Origin"))
}

func TestParseFilters(t *testing.T) {
	t.Parallel()

	got, err := ParseFilters([]string{"status:active", "grade:6"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"status": "active", "grade": "6"}, got)

	// Value containing a colon is preserved
	got, err = ParseFilters([]string{"note:a:b"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"note": "a:b"}, got)

	_, err = ParseFilters([]string{"nocolon"})
	assert.Error(t, err)

	_, err = ParseFilters([]string{":empty"})
	assert.Error(t, err)

	got, err = ParseFilters(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStructToValues(t *testing.T) {
	t.Parallel()

	type body struct {
		Name     string  `db:"name"`
		Optional *string `db:"optional"`
		Skipped  string  `db:"-"`
		NoTag    string
	}
	val := "set"
	got := structToValues(body{Name: "x", Optional: &val, Skipped: "no", NoTag: "no"})
	assert.Equal(t, map[string]any{"name": "x", "optional": "set"}, got)

	// Nil pointers are omitted; pointer input is dereferenced
	got = structToValues(&body{Name: "y"})
	assert.Equal(t, map[string]any{"name": "y"}, got)
}

func TestTitleCase(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Assessment Items", titleCase("assessment-items"))
	assert.Equal(t, "Students", titleCase("students"))
	assert.Equal(t, "", titleCase(""))
}

func TestMapError(t *testing.T) {
	t.Parallel()

	// Not found
	err := MapError(repo.ErrNotFound)
	var statusErr huma.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusNotFound, statusErr.GetStatus())

	// Bad request from repo validation
	err = MapError(repo.ErrBadRequest{Msg: "bad"})
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusBadRequest, statusErr.GetStatus())

	// Postgres error codes
	cases := map[string]int{
		"23505": http.StatusConflict,
		"23503": http.StatusUnprocessableEntity,
		"23514": http.StatusUnprocessableEntity,
		"22P02": http.StatusBadRequest,
	}
	for code, want := range cases {
		err = MapError(&pgconn.PgError{Code: code})
		require.ErrorAs(t, err, &statusErr, "code %s", code)
		assert.Equal(t, want, statusErr.GetStatus(), "code %s", code)
	}

	// Unknown errors pass through
	sentinel := errors.New("boom")
	assert.Equal(t, sentinel, MapError(sentinel))

	// Unknown pg codes pass through
	pgErr := &pgconn.PgError{Code: "42P01"}
	assert.Equal(t, error(pgErr), MapError(pgErr))
}

func TestGuardStampsMiddlewareSecurityAndErrors(t *testing.T) {
	t.Parallel()

	called := false
	middleware := func(ctx huma.Context, next func(huma.Context)) {
		called = true
		next(ctx)
	}

	var cfg crudConfig
	Guard(middleware, "someScheme")(&cfg)

	op := cfg.apply(huma.Operation{OperationID: "list-things"})
	require.Len(t, op.Middlewares, 1, "the guard runs before the handler")
	assert.Equal(t, []map[string][]string{{"someScheme": {}}}, op.Security,
		"the scheme is documented on the operation")
	assert.Contains(t, op.Errors, http.StatusUnauthorized,
		"a guarded operation can reject the caller")

	// The recorded middleware is the one supplied.
	op.Middlewares[0](nil, func(huma.Context) {})
	assert.True(t, called)
}

func TestGuardWithoutASchemeStillAppliesMiddleware(t *testing.T) {
	t.Parallel()

	var cfg crudConfig
	Guard(func(ctx huma.Context, next func(huma.Context)) { next(ctx) }, "")(&cfg)

	op := cfg.apply(huma.Operation{})
	assert.Len(t, op.Middlewares, 1)
	assert.Empty(t, op.Security, "an unnamed scheme documents nothing")
}

func TestCRUDConfigWithoutGuardIsUnchanged(t *testing.T) {
	t.Parallel()

	// Resources that opt into no guard must register exactly as before.
	var cfg crudConfig
	SkipCreate()(&cfg)

	op := cfg.apply(huma.Operation{OperationID: "get-thing"})
	assert.Empty(t, op.Middlewares)
	assert.Empty(t, op.Security)
	assert.Empty(t, op.Errors)
	assert.True(t, cfg.skipCreate)
}
