package api_test

// Phase 8 coverage additions: exercise Phase 5/6 handler paths via the
// existing stub service + JWKS server helpers defined in api_test.go.
// These are fast unit tests requiring no database.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/api"
	"github.com/aleksclark/primer/agents/internal/authn"
	"github.com/aleksclark/primer/agents/internal/authn/jwttest"
)

// ── helpers reused across handler coverage tests ──────────────────────────────

func p8Handler(t *testing.T) (http.Handler, func() string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	v, err := authn.NewValidator(authn.Options{
		Issuer:  jwttest.DefaultIssuer,
		JWKSURL: jwks.URL,
		Now:     func() time.Time { return now },
	})
	require.NoError(t, err)
	h := api.New(api.Options{
		Validator: v,
		Service:   newStubSvc(),
		Env:       "test",
	})
	mintTok := func() string {
		sub := "identity:" + uuid.NewString()
		c := jwttest.ValidHumanClaims(now, sub)
		c.Scope = jwttest.AllScopes
		return "Bearer " + jwttest.Mint(t, key, c)
	}
	return h, mintTok
}

// ── Phase 5 handler coverage ──────────────────────────────────────────────────

func TestHandlerListEvents(t *testing.T) {
	t.Parallel()
	h, tok := p8Handler(t)
	sameToken := tok() // same principal throughout
	// Create a run first so the stub has it.
	req := httptest.NewRequest(http.MethodPost, "/agents/v1/runs",
		strings.NewReader(`{"profile":"tutor"}`))
	req.Header.Set("Authorization", sameToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "key-ev-list")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var runBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &runBody))
	runID := runBody["id"].(string)

	// List events — should be empty but return 200.
	req2 := httptest.NewRequest(http.MethodGet, "/agents/v1/runs/"+runID+"/events", nil)
	req2.Header.Set("Authorization", sameToken)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
	var evBody map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &evBody))
	assert.NotNil(t, evBody["events"])
}

func TestHandlerCreateGetSession(t *testing.T) {
	t.Parallel()
	h, tok := p8Handler(t)
	sameToken := tok() // same principal for create and get

	// Create session.
	req := httptest.NewRequest(http.MethodPost, "/agents/v1/sessions",
		strings.NewReader(`{"profile":"admin"}`))
	req.Header.Set("Authorization", sameToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var sessBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sessBody))
	sessID := sessBody["id"].(string)
	assert.NotEmpty(t, sessID)
	assert.Equal(t, "admin", sessBody["profile"])

	// Get session — must use the same token (same namespace).
	req2 := httptest.NewRequest(http.MethodGet, "/agents/v1/sessions/"+sessID, nil)
	req2.Header.Set("Authorization", sameToken)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestHandlerGetSessionNotFound(t *testing.T) {
	t.Parallel()
	h, tok := p8Handler(t)
	req := httptest.NewRequest(http.MethodGet, "/agents/v1/sessions/"+uuid.NewString(), nil)
	req.Header.Set("Authorization", tok())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandlerAppendAndListTurns(t *testing.T) {
	t.Parallel()
	h, tok := p8Handler(t)
	sameToken := tok() // same principal throughout

	// Create session.
	req := httptest.NewRequest(http.MethodPost, "/agents/v1/sessions",
		strings.NewReader(`{"profile":"admin"}`))
	req.Header.Set("Authorization", sameToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var sessBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sessBody))
	sessID := sessBody["id"].(string)

	// Append turn.
	turnPayload := `{"idempotencyKey":"turn-1","profile":"admin","expectedRevision":1}`
	reqT := httptest.NewRequest(http.MethodPost, "/agents/v1/sessions/"+sessID+"/turns",
		strings.NewReader(turnPayload))
	reqT.Header.Set("Authorization", sameToken)
	reqT.Header.Set("Content-Type", "application/json")
	recT := httptest.NewRecorder()
	h.ServeHTTP(recT, reqT)
	assert.Equal(t, http.StatusOK, recT.Code)

	// List turns.
	reqL := httptest.NewRequest(http.MethodGet, "/agents/v1/sessions/"+sessID+"/turns", nil)
	reqL.Header.Set("Authorization", sameToken)
	recL := httptest.NewRecorder()
	h.ServeHTTP(recL, reqL)
	assert.Equal(t, http.StatusOK, recL.Code)
}

// ── Phase 6 handler coverage ──────────────────────────────────────────────────

func TestHandlerCreateJob(t *testing.T) {
	t.Parallel()
	h, tok := p8Handler(t)
	body := `{"jobType":"sync"}`
	req := httptest.NewRequest(http.MethodPost, "/agents/v1/jobs",
		strings.NewReader(body))
	req.Header.Set("Authorization", tok())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "job-key-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var run map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &run))
	assert.Equal(t, "queued", run["status"])
}

func TestHandlerCreateAndListSchedule(t *testing.T) {
	t.Parallel()
	h, tok := p8Handler(t)

	// Create schedule.
	schedPayload := `{"profile":"job","cronExpr":"1m","jobType":"test-job","maxCatchUp":1,"timezone":"UTC"}`
	req := httptest.NewRequest(http.MethodPost, "/agents/v1/schedules",
		strings.NewReader(schedPayload))
	req.Header.Set("Authorization", tok())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var schedBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &schedBody))
	assert.Equal(t, "test-job", schedBody["jobType"])
	assert.Equal(t, true, schedBody["enabled"])

	// List schedules.
	req2 := httptest.NewRequest(http.MethodGet, "/agents/v1/schedules", nil)
	req2.Header.Set("Authorization", tok())
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestHandlerEnableDisableSchedule(t *testing.T) {
	t.Parallel()
	h, tok := p8Handler(t)
	// Enable/disable on a nonexistent ID → 404.
	fakeID := uuid.NewString()
	for _, path := range []string{"/enable", "/disable"} {
		req := httptest.NewRequest(http.MethodPost, "/agents/v1/schedules/"+fakeID+path,
			strings.NewReader("{}"))
		req.Header.Set("Authorization", tok())
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, "path: %s", path)
	}
}

func TestHandlerStudentSessionAndTurn(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	v, err := authn.NewValidator(authn.Options{
		Issuer:  jwttest.DefaultIssuer,
		JWKSURL: jwks.URL,
		Now:     func() time.Time { return now },
	})
	require.NoError(t, err)
	h := api.New(api.Options{Validator: v, Service: newStubSvc(), Env: "test"})

	sub := "identity:" + uuid.NewString()
	claims := jwttest.ValidHumanClaims(now, sub)
	claims.Scope = authn.ScopeStudentSession
	tok := "Bearer " + jwttest.Mint(t, key, claims)

	// Create student session — profile must be student.
	req := httptest.NewRequest(http.MethodPost, "/agents/v1/student/sessions",
		strings.NewReader(`{"opaqueStudentRef":"ref-abc"}`))
	req.Header.Set("Authorization", tok)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var sessBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sessBody))
	assert.Equal(t, "student", sessBody["profile"])
	sessID := sessBody["id"].(string)

	// Append student turn.
	turnPayload := `{"idempotencyKey":"st-turn-1","expectedRevision":1}`
	reqT := httptest.NewRequest(http.MethodPost, "/agents/v1/student/sessions/"+sessID+"/turns",
		strings.NewReader(turnPayload))
	reqT.Header.Set("Authorization", tok)
	reqT.Header.Set("Content-Type", "application/json")
	recT := httptest.NewRecorder()
	h.ServeHTTP(recT, reqT)
	assert.Equal(t, http.StatusOK, recT.Code)
}

func TestHandlerStudentRouteRejectsParentScope(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	v, err := authn.NewValidator(authn.Options{
		Issuer:  jwttest.DefaultIssuer,
		JWKSURL: jwks.URL,
		Now:     func() time.Time { return now },
	})
	require.NoError(t, err)
	h := api.New(api.Options{Validator: v, Service: newStubSvc(), Env: "test"})
	sub := "identity:" + uuid.NewString()
	claims := jwttest.ValidHumanClaims(now, sub)
	// Only runs:write, NOT student:session.
	claims.Scope = authn.ScopeRunsWrite
	tok := "Bearer " + jwttest.Mint(t, key, claims)
	req := httptest.NewRequest(http.MethodPost, "/agents/v1/student/sessions",
		strings.NewReader(`{}`))
	req.Header.Set("Authorization", tok)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandlerJobScopeRequired(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	v, err := authn.NewValidator(authn.Options{
		Issuer:  jwttest.DefaultIssuer,
		JWKSURL: jwks.URL,
		Now:     func() time.Time { return now },
	})
	require.NoError(t, err)
	h := api.New(api.Options{Validator: v, Service: newStubSvc(), Env: "test"})
	sub := "identity:" + uuid.NewString()
	claims := jwttest.ValidHumanClaims(now, sub)
	// Only runs:write, NOT jobs:write.
	claims.Scope = authn.ScopeRunsWrite
	tok := "Bearer " + jwttest.Mint(t, key, claims)

	body, _ := json.Marshal(map[string]any{"jobType": "test"})
	req := httptest.NewRequest(http.MethodPost, "/agents/v1/jobs",
		bytes.NewReader(body))
	req.Header.Set("Authorization", tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "job-scope-test")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// ── Error mapping coverage ────────────────────────────────────────────────────

func TestMapServiceErrorCoverage(t *testing.T) {
	t.Parallel()
	h, tok := p8Handler(t)
	// Get a non-existent run → 404 (ErrNotFound path).
	req := httptest.NewRequest(http.MethodGet, "/agents/v1/runs/"+uuid.NewString(), nil)
	req.Header.Set("Authorization", tok())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
