package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/aleksclark/primer/agents/internal/domain"
)

// ── stub service ──────────────────────────────────────────────────────────────

type stubSvc struct {
	runs     map[string]*domain.Run
	sessions map[string]*domain.Session
	events   map[string][]*domain.RunEvent
}

func newStubSvc() *stubSvc {
	return &stubSvc{
		runs:     make(map[string]*domain.Run),
		sessions: make(map[string]*domain.Session),
		events:   make(map[string][]*domain.RunEvent),
	}
}

func (s *stubSvc) CreateRun(_ context.Context, cmd api.CreateRunCmd) (*domain.Run, error) {
	r := &domain.Run{
		ID:             uuid.NewString(),
		OwnerNamespace: cmd.OwnerNamespace,
		IdempotencyKey: cmd.IdempotencyKey,
		Profile:        cmd.Profile,
		Status:         domain.RunStatusQueued,
		StateVersion:   1,
		CreatedAt:      time.Now(),
	}
	s.runs[r.ID] = r
	return r, nil
}
func (s *stubSvc) GetRun(_ context.Context, id, ns string) (*domain.Run, error) {
	r, ok := s.runs[id]
	if !ok || r.OwnerNamespace != ns {
		return nil, errNotFound
	}
	return r, nil
}
func (s *stubSvc) ListRuns(_ context.Context, ns string, _ int) ([]*domain.Run, error) {
	var out []*domain.Run
	for _, r := range s.runs {
		if r.OwnerNamespace == ns {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *stubSvc) RequestCancel(_ context.Context, id, ns string, _ *string) (*domain.Run, error) {
	r, ok := s.runs[id]
	if !ok || r.OwnerNamespace != ns {
		return nil, errNotFound
	}
	r.Status = domain.RunStatusCancelRequested
	return r, nil
}
func (s *stubSvc) ListEvents(_ context.Context, runID, ns string, afterSeq int64, _ int) ([]*domain.RunEvent, error) {
	r, ok := s.runs[runID]
	if !ok || r.OwnerNamespace != ns {
		return nil, errNotFound
	}
	var out []*domain.RunEvent
	for _, e := range s.events[runID] {
		if e.Sequence > afterSeq {
			out = append(out, e)
		}
	}
	return out, nil
}
func (s *stubSvc) CreateSession(_ context.Context, cmd api.CreateSessionCmd) (*domain.Session, error) {
	sess := &domain.Session{
		ID:             uuid.NewString(),
		OwnerNamespace: cmd.OwnerNamespace,
		Profile:        cmd.Profile,
		Status:         domain.SessionStatusOpen,
		Revision:       1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	s.sessions[sess.ID] = sess
	return sess, nil
}
func (s *stubSvc) GetSession(_ context.Context, id, ns string) (*domain.Session, error) {
	sess, ok := s.sessions[id]
	if !ok || sess.OwnerNamespace != ns {
		return nil, errNotFound
	}
	return sess, nil
}

// errNotFound satisfies errors.Is(err, repo.ErrNotFound).
// We use the real sentinel via the repo package to keep mapping correct.
var errNotFound = fmt.Errorf("not found: %w", repoNotFound())

func repoNotFound() error {
	// Import the real sentinel indirectly to avoid importing repo in test.
	// The mapServiceError in handlers.go uses errors.Is which checks the chain.
	return errNotFoundSentinel{}
}

type errNotFoundSentinel struct{}

func (errNotFoundSentinel) Error() string { return "not found" }
func (errNotFoundSentinel) Is(target error) bool {
	// Match repo.ErrNotFound by value comparison via string.
	return target != nil && target.Error() == "not found"
}

// ── JWKS server ───────────────────────────────────────────────────────────────

func serveJWKS(t *testing.T, keys ...*jwttest.Keypair) *httptest.Server {
	t.Helper()
	doc, _ := json.Marshal(jwttest.JWKSDoc(keys...))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(doc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newValidator(t *testing.T, jwksURL string, now func() time.Time) *authn.Validator {
	t.Helper()
	v, err := authn.NewValidator(authn.Options{
		Issuer:  jwttest.DefaultIssuer,
		JWKSURL: jwksURL,
		Now:     now,
	})
	require.NoError(t, err)
	return v
}

func newHandler(t *testing.T, v api.TokenValidator) http.Handler {
	t.Helper()
	return api.New(api.Options{
		Pool:      nil,
		Validator: v,
		Service:   newStubSvc(),
		Env:       "test",
	})
}

func bearer(tok string) string { return "Bearer " + tok }

func doJSON(t *testing.T, h http.Handler, method, path, authHdr string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if authHdr != "" {
		req.Header.Set("Authorization", authHdr)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ── Health probes need no auth ────────────────────────────────────────────────

func TestHealthzNoAuth(t *testing.T) {
	t.Parallel()
	h := newHandler(t, nil)
	rec := doJSON(t, h, http.MethodGet, "/healthz", "", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestReadyzNoAuth(t *testing.T) {
	t.Parallel()
	h := newHandler(t, nil)
	rec := doJSON(t, h, http.MethodGet, "/readyz", "", nil)
	// Pool is nil so readyz returns 503, but it must not return 401.
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)
}

// ── Authentication negatives ──────────────────────────────────────────────────

func TestAuthRejectsNoToken(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	h := newHandler(t, newValidator(t, srv.URL, func() time.Time { return now }))

	rec := doJSON(t, h, http.MethodGet, "/agents/v1/runs", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthRejectsMissingBearer(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, "identity:"+uuid.NewString()))
	h := newHandler(t, newValidator(t, srv.URL, func() time.Time { return now }))

	// Plain token without "Bearer " prefix.
	rec := doJSON(t, h, http.MethodGet, "/agents/v1/runs", tok, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthRejectsExpiredToken(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	claims := jwttest.ValidHumanClaims(now.Add(-20*time.Minute), "identity:"+uuid.NewString())
	claims.ExpiresAt = now.Add(-5 * time.Minute)
	tok := jwttest.Mint(t, key, claims)
	h := newHandler(t, newValidator(t, srv.URL, func() time.Time { return now }))

	rec := doJSON(t, h, http.MethodGet, "/agents/v1/runs", bearer(tok), nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthRejectsWrongAudience(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	claims := jwttest.ValidHumanClaims(now, "identity:"+uuid.NewString())
	claims.Audience = "primer-lms" // wrong audience
	tok := jwttest.Mint(t, key, claims)
	h := newHandler(t, newValidator(t, srv.URL, func() time.Time { return now }))

	rec := doJSON(t, h, http.MethodGet, "/agents/v1/runs", bearer(tok), nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	other := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	tok := jwttest.Mint(t, other, jwttest.ValidHumanClaims(now, "identity:"+uuid.NewString()))
	h := newHandler(t, newValidator(t, srv.URL, func() time.Time { return now }))

	rec := doJSON(t, h, http.MethodGet, "/agents/v1/runs", bearer(tok), nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthRejectsRawOpaqueBearer(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	h := newHandler(t, newValidator(t, srv.URL, func() time.Time { return now }))

	// Stytch session-token shaped value.
	rec := doJSON(t, h, http.MethodGet, "/agents/v1/runs",
		bearer("stytch_session_token_v1_opaque_string_here"), nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthRejectsInsufficientScope(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	// Token has only sessions:read, not runs:write.
	claims := jwttest.ValidHumanClaims(now, "identity:"+uuid.NewString())
	claims.Scope = authn.ScopeSessionsRead
	tok := jwttest.Mint(t, key, claims)
	h := newHandler(t, newValidator(t, srv.URL, func() time.Time { return now }))

	rec := doJSON(t, h, http.MethodPost, "/agents/v1/runs", bearer(tok), map[string]any{
		"profile": "tutor",
	})
	assert.Equal(t, http.StatusForbidden, rec.Code, "missing runs:write scope must be 403")
}

func TestAuthRejectsUnconfiguredValidator(t *testing.T) {
	t.Parallel()
	// When validator is nil every /agents/v1 route must 401.
	h := api.New(api.Options{Service: newStubSvc()})
	req := httptest.NewRequest(http.MethodGet, "/agents/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer something")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── Authenticated run flow ────────────────────────────────────────────────────

func mint(t *testing.T, key *jwttest.Keypair, now time.Time, sub string) string {
	t.Helper()
	return jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, sub))
}

func TestCreateRunReturnsQueued(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	h := newHandler(t, newValidator(t, srv.URL, func() time.Time { return now }))
	tok := mint(t, key, now, "identity:"+uuid.NewString())

	req := httptest.NewRequest(http.MethodPost, "/agents/v1/runs",
		strings.NewReader(`{"profile":"tutor"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "test-key-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "queued", body["status"])
	assert.NotEmpty(t, body["id"])
	assert.Contains(t, body["ownerNamespace"], jwttest.DefaultClientID)
}

// ── IDOR / ownership isolation ────────────────────────────────────────────────

func TestIDORGetRunReturnNotFound(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	svc := newStubSvc()
	h := api.New(api.Options{Validator: newValidator(t, srv.URL, func() time.Time { return now }), Service: svc})

	subA := "identity:" + uuid.NewString()
	subB := "identity:" + uuid.NewString()
	tokA := mint(t, key, now, subA)
	tokB := mint(t, key, now, subB)

	// A creates a run.
	reqCreate := httptest.NewRequest(http.MethodPost, "/agents/v1/runs",
		strings.NewReader(`{"profile":"tutor"}`))
	reqCreate.Header.Set("Authorization", "Bearer "+tokA)
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Idempotency-Key", "idor-key")
	recCreate := httptest.NewRecorder()
	h.ServeHTTP(recCreate, reqCreate)
	require.Equal(t, http.StatusOK, recCreate.Code)

	var created map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &created))
	runID := created["id"].(string)

	// B tries to read A's run by ID.
	reqGet := httptest.NewRequest(http.MethodGet, "/agents/v1/runs/"+runID, nil)
	reqGet.Header.Set("Authorization", "Bearer "+tokB)
	recGet := httptest.NewRecorder()
	h.ServeHTTP(recGet, reqGet)
	assert.Equal(t, http.StatusNotFound, recGet.Code, "B must not see A's run")
}

func TestIDORCancelRunReturnNotFound(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	svc := newStubSvc()
	h := api.New(api.Options{Validator: newValidator(t, srv.URL, func() time.Time { return now }), Service: svc})

	subA := "identity:" + uuid.NewString()
	subB := "identity:" + uuid.NewString()

	// A creates a run.
	reqCreate := httptest.NewRequest(http.MethodPost, "/agents/v1/runs",
		strings.NewReader(`{"profile":"tutor"}`))
	reqCreate.Header.Set("Authorization", "Bearer "+mint(t, key, now, subA))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Idempotency-Key", "idor-cancel")
	recCreate := httptest.NewRecorder()
	h.ServeHTTP(recCreate, reqCreate)
	require.Equal(t, http.StatusOK, recCreate.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &created))
	runID := created["id"].(string)

	// B tries to cancel A's run.
	reqCancel := httptest.NewRequest(http.MethodPost,
		"/agents/v1/runs/"+runID+"/cancel", strings.NewReader(`{}`))
	reqCancel.Header.Set("Authorization", "Bearer "+mint(t, key, now, subB))
	reqCancel.Header.Set("Content-Type", "application/json")
	recCancel := httptest.NewRecorder()
	h.ServeHTTP(recCancel, reqCancel)
	assert.Equal(t, http.StatusNotFound, recCancel.Code, "B must not cancel A's run")
}

func TestOwnerNamespaceNotInfluencedByBody(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	h := newHandler(t, newValidator(t, srv.URL, func() time.Time { return now }))
	sub := "identity:" + uuid.NewString()
	tok := mint(t, key, now, sub)

	// Body includes a fake owner field — it must be ignored.
	body := `{"profile":"tutor","callerContext":"owner=somebody-else"}`
	req := httptest.NewRequest(http.MethodPost, "/agents/v1/runs", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "ns-override-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ns := resp["ownerNamespace"].(string)
	assert.Contains(t, ns, sub, "namespace must contain the signed subject")
	assert.NotContains(t, ns, "somebody-else", "caller-supplied owner must be ignored")
}

func TestListRunsReturnsOnlyCallerRuns(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	srv := serveJWKS(t, key)
	svc := newStubSvc()
	h := api.New(api.Options{Validator: newValidator(t, srv.URL, func() time.Time { return now }), Service: svc})

	subA := "identity:" + uuid.NewString()
	subB := "identity:" + uuid.NewString()

	for i, sub := range []string{subA, subA, subB} {
		req := httptest.NewRequest(http.MethodPost, "/agents/v1/runs",
			strings.NewReader(`{"profile":"tutor"}`))
		req.Header.Set("Authorization", "Bearer "+mint(t, key, now, sub))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("list-key-%d", i))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// A lists: must see 2 runs.
	reqList := httptest.NewRequest(http.MethodGet, "/agents/v1/runs", nil)
	reqList.Header.Set("Authorization", "Bearer "+mint(t, key, now, subA))
	recList := httptest.NewRecorder()
	h.ServeHTTP(recList, reqList)
	require.Equal(t, http.StatusOK, recList.Code)

	var listBody struct {
		Runs []map[string]any `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(recList.Body.Bytes(), &listBody))
	assert.Len(t, listBody.Runs, 2, "A must see only its own runs")
	for _, r := range listBody.Runs {
		assert.Contains(t, r["ownerNamespace"], subA)
	}
}
