// Package e2e_test exercises CurriculumIntegrationService end-to-end using
// the generated Go gRPC client façade (clients/go-grpc) against a real
// bufconn loopback server backed by the harness store.
//
// Tests are tagged as plain go test (no external dependencies). Coverage:
//
//	E5-01: Materialize + GetMaterialization id/status progression
//	E5-02: GetBundle with precondition error (not ready → ErrorDetail)
//	E5-03: GetBundle with Struct tutor_context (ready → deep-equal fixture)
//	E5-04: Idempotent retry and conflict
//	E5-05: No auth header → unauthenticated; valid JWT → success
//	E5-06: Seeded published revision/graph/validation reads
//	E5-07: PullEvents + AcknowledgeEvent durable ack and EventType enums
package e2e_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	grpcclient "github.com/aleksclark/primer/curriculum-studio/clients/go-grpc"
	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/grpcapi"
	"github.com/aleksclark/primer/curriculum-studio/internal/grpcapi/harness"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

const bufSize = 1 << 20

// serveJWKS starts a test JWKS server for the given keypairs.
func serveJWKS(t *testing.T, keys ...*jwttest.Keypair) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(jwttest.JWKSDocument(keys...))
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// dialWithToken dials a fresh connection to the bufconn listener using
// bearer-token interceptors. This is the correct pattern for auth tests.
func dialWithToken(t *testing.T, lis *bufconn.Listener, token string) *grpcclient.Client {
	t.Helper()
	unary, stream := grpcclient.WithBearerToken(token)
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(unary),
		grpc.WithStreamInterceptor(stream),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return grpcclient.NewClient(conn)
}

// dialNoAuth dials without any bearer token.
func dialNoAuth(t *testing.T, lis *bufconn.Listener) *grpcclient.Client {
	t.Helper()
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return grpcclient.NewClient(conn)
}

// suite2 is a self-contained harness that exposes its bufconn listener so
// callers can create multiple connections with different auth configurations.
type suite2 struct {
	lis   *bufconn.Listener
	store *harness.Store
	gs    *grpc.Server
}

func newSuite2(t *testing.T, jwksURL string) *suite2 {
	t.Helper()
	st := harness.New()
	validator, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwksURL,
		Now:      time.Now,
	})
	require.NoError(t, err)

	lis := bufconn.Listen(bufSize)
	gs := grpc.NewServer(grpc.UnaryInterceptor(grpcapi.UnaryAuthInterceptor(validator)))
	grpcapi.Register(gs, grpcapi.Deps{Validator: validator, Store: st})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})
	return &suite2{lis: lis, store: st, gs: gs}
}

// ctx returns a background context with a reasonable deadline.
func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// ────────────────────────────────────────────────────────────────────────────
// E5-01: Materialize + GetMaterialization id/status progression
// ────────────────────────────────────────────────────────────────────────────

func TestE5_01_MaterializeAndGetProgression(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	suite := newSuite2(t, jwks.URL)
	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))
	client := dialWithToken(t, suite.lis, tok)

	c, cancel := ctx()
	defer cancel()

	// Materialize returns a run id with status REQUESTED or RUNNING.
	mat, err := client.Materialize(c, &v1.MaterializeRequest{
		IdempotencyKey: "e5-01-key-1",
		Context: &v1.MaterializationContext{
			PlanRevisionId: "prev_fixture_1",
			Learner:        &v1.LearnerProfile{Id: "learner_1", Grade: "6"},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, mat.GetId(), "materialization id must be set")
	assert.Equal(t, "prev_fixture_1", mat.GetPlanRevisionId())
	assert.Equal(t, v1.MaterializationStatus_MATERIALIZATION_STATUS_REQUESTED, mat.GetStatus())

	// GetMaterialization returns the same id.
	got, err := client.GetMaterialization(c, &v1.GetMaterializationRequest{
		MaterializationId: mat.GetId(),
	})
	require.NoError(t, err)
	assert.Equal(t, mat.GetId(), got.GetId())
	assert.Equal(t, v1.MaterializationStatus_MATERIALIZATION_STATUS_REQUESTED, got.GetStatus())

	// Advance status to RUNNING then READY via store.
	suite.store.AdvanceStatus(mat.GetId(), v1.MaterializationStatus_MATERIALIZATION_STATUS_RUNNING)
	got2, err := client.GetMaterialization(c, &v1.GetMaterializationRequest{MaterializationId: mat.GetId()})
	require.NoError(t, err)
	assert.Equal(t, v1.MaterializationStatus_MATERIALIZATION_STATUS_RUNNING, got2.GetStatus())
}

// ────────────────────────────────────────────────────────────────────────────
// E5-02: GetBundle precondition while not ready; ErrorDetail extracted
// ────────────────────────────────────────────────────────────────────────────

func TestE5_02_GetBundleNotReadyPrecondition(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	suite := newSuite2(t, jwks.URL)
	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))
	client := dialWithToken(t, suite.lis, tok)

	c, cancel := ctx()
	defer cancel()

	mat, err := client.Materialize(c, &v1.MaterializeRequest{
		IdempotencyKey: "e5-02-key",
		Context:        &v1.MaterializationContext{PlanRevisionId: "prev_1"},
	})
	require.NoError(t, err)

	// Status is REQUESTED → GetBundle must fail_precondition.
	_, err = client.GetMaterializationBundle(c, &v1.GetMaterializationBundleRequest{
		MaterializationId: mat.GetId(),
	})
	require.Error(t, err)

	st := grpcstatus.Convert(err)
	assert.Equal(t, codes.FailedPrecondition, st.Code())

	// Extract our custom ErrorDetail from the status details.
	var found *v1.ErrorDetail
	for _, d := range st.Details() {
		if ed, ok := d.(*v1.ErrorDetail); ok {
			found = ed
			break
		}
	}
	require.NotNil(t, found, "ErrorDetail must be present in status details")
	assert.Equal(t, v1.ErrorCode_ERROR_CODE_FAILED_PRECONDITION, found.GetCode())
	assert.NotEmpty(t, found.GetMessage())
}

// ────────────────────────────────────────────────────────────────────────────
// E5-03: GetBundle after READY returns Struct tutor_context deep equal fixture
// ────────────────────────────────────────────────────────────────────────────

func TestE5_03_GetBundleReadyWithTutorContext(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	suite := newSuite2(t, jwks.URL)
	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))
	client := dialWithToken(t, suite.lis, tok)

	c, cancel := ctx()
	defer cancel()

	mat, err := client.Materialize(c, &v1.MaterializeRequest{
		IdempotencyKey: "e5-03-key",
		Context:        &v1.MaterializationContext{PlanRevisionId: "prev_2"},
	})
	require.NoError(t, err)

	// Seed a ready bundle with tutor_context Struct.
	fixtureBundle := harness.FixtureBundle(mat.GetId(), "prev_2")
	suite.store.SeedBundle(mat.GetId(), fixtureBundle)
	suite.store.AdvanceStatus(mat.GetId(), v1.MaterializationStatus_MATERIALIZATION_STATUS_READY)

	bundle, err := client.GetMaterializationBundle(c, &v1.GetMaterializationBundleRequest{
		MaterializationId: mat.GetId(),
	})
	require.NoError(t, err)
	assert.Equal(t, mat.GetId(), bundle.GetId())
	assert.Equal(t, "prev_2", bundle.GetPlanRevisionId())
	require.Len(t, bundle.GetSessions(), 1)

	sess := bundle.GetSessions()[0]
	assert.Equal(t, "sess_1", sess.GetId())
	require.NotNil(t, sess.GetTutorContext(), "tutor_context Struct must be present")

	// Verify Struct round-trip: extract fields and compare to fixture.
	tc := sess.GetTutorContext().AsMap()
	assert.Equal(t, "math", tc["subject"])
	assert.Equal(t, "1.0", tc["version"])
}

// ────────────────────────────────────────────────────────────────────────────
// E5-04: Idempotent retry returns same id; conflict yields IDEMPOTENCY_KEY_CONFLICT
// ────────────────────────────────────────────────────────────────────────────

func TestE5_04_IdempotentRetryAndConflict(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	suite := newSuite2(t, jwks.URL)
	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))
	client := dialWithToken(t, suite.lis, tok)

	c, cancel := ctx()
	defer cancel()

	const idemKey = "e5-04-idem-key"
	const revID = "prev_idem"

	// First call.
	mat1, err := client.Materialize(c, &v1.MaterializeRequest{
		IdempotencyKey: idemKey,
		Context:        &v1.MaterializationContext{PlanRevisionId: revID},
	})
	require.NoError(t, err)

	// Retry with same key + same revision → same id.
	mat2, err := client.Materialize(c, &v1.MaterializeRequest{
		IdempotencyKey: idemKey,
		Context:        &v1.MaterializationContext{PlanRevisionId: revID},
	})
	require.NoError(t, err)
	assert.Equal(t, mat1.GetId(), mat2.GetId(), "idempotent retry must return same materialization id")

	// Conflict: same key, same revision but a different learner context.
	_, err = client.Materialize(c, &v1.MaterializeRequest{
		IdempotencyKey: idemKey,
		Context: &v1.MaterializationContext{
			PlanRevisionId: revID,
			Learner:        &v1.LearnerProfile{Id: "different-learner"},
		},
	})
	require.Error(t, err)
	assert.Equal(t, codes.AlreadyExists, grpcstatus.Code(err))

	// Conflict: same key, different revision.
	_, err = client.Materialize(c, &v1.MaterializeRequest{
		IdempotencyKey: idemKey,
		Context:        &v1.MaterializationContext{PlanRevisionId: "prev_different"},
	})
	require.Error(t, err)
	st := grpcstatus.Convert(err)
	assert.Equal(t, codes.AlreadyExists, st.Code())

	// ErrorDetail code must be IDEMPOTENCY_KEY_CONFLICT.
	var found *v1.ErrorDetail
	for _, d := range st.Details() {
		if ed, ok := d.(*v1.ErrorDetail); ok {
			found = ed
			break
		}
	}
	require.NotNil(t, found, "ErrorDetail must be present")
	assert.Equal(t, v1.ErrorCode_ERROR_CODE_IDEMPOTENCY_KEY_CONFLICT, found.GetCode())
}

// ────────────────────────────────────────────────────────────────────────────
// E5-05: Auth — missing token → unauthenticated; valid JWT → success
// ────────────────────────────────────────────────────────────────────────────

func TestE5_05_AuthMetadataRequired(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	suite := newSuite2(t, jwks.URL)

	c, cancel := ctx()
	defer cancel()

	// No auth → unauthenticated.
	noAuthClient := dialNoAuth(t, suite.lis)
	_, err := noAuthClient.Materialize(c, &v1.MaterializeRequest{
		IdempotencyKey: "e5-05-noauth",
		Context:        &v1.MaterializationContext{PlanRevisionId: "prev_x"},
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, grpcstatus.Code(err))

	// Valid token with aud=curriculum-studio → success.
	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))
	authClient := dialWithToken(t, suite.lis, tok)
	mat, err := authClient.Materialize(c, &v1.MaterializeRequest{
		IdempotencyKey: "e5-05-auth",
		Context:        &v1.MaterializationContext{PlanRevisionId: "prev_x"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, mat.GetId())
}

// ────────────────────────────────────────────────────────────────────────────
// E5-06: Seeded published revision / graph / validation reads
// ────────────────────────────────────────────────────────────────────────────

func TestE5_06_PublishedRevisionGraphValidation(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	suite := newSuite2(t, jwks.URL)
	tok := jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, uuid.NewString()))
	client := dialWithToken(t, suite.lis, tok)

	const revID = "prev_pub_1"
	const currID = "cur_1"

	// Seed fixture data.
	suite.store.SeedPublishedRevision(harness.FixtureRevision(revID, currID))
	suite.store.SeedPlanGraph(revID, harness.FixtureGraph(revID))

	c, cancel := ctx()
	defer cancel()

	// GetPublishedRevision returns the immutable revision.
	rev, err := client.GetPublishedRevision(c, &v1.GetPublishedRevisionRequest{RevisionId: revID})
	require.NoError(t, err)
	assert.Equal(t, revID, rev.GetId())
	assert.Equal(t, v1.RevisionState_REVISION_STATE_PUBLISHED, rev.GetState())
	assert.Equal(t, currID, rev.GetCurriculumId())

	// GetPlanGraph returns the graph for the published revision.
	graph, err := client.GetPlanGraph(c, &v1.GetPlanGraphRequest{RevisionId: revID})
	require.NoError(t, err)
	assert.Equal(t, revID, graph.GetRevisionId())
	require.NotEmpty(t, graph.GetNodes())

	// ValidateRevision returns a validation report (empty findings for fixture).
	report, err := client.ValidateRevision(c, &v1.ValidateRevisionRequest{RevisionId: revID})
	require.NoError(t, err)
	assert.Equal(t, revID, report.GetRevisionId())

	// Draft/missing revision → not found.
	_, err = client.GetPublishedRevision(c, &v1.GetPublishedRevisionRequest{RevisionId: "prev_draft_not_seeded"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcstatus.Code(err))
}

// ────────────────────────────────────────────────────────────────────────────
// E5-07: PullEvents + AcknowledgeEvent durable ack and EventType enums
// ────────────────────────────────────────────────────────────────────────────

func TestE5_07_PullEventsAndAck(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	suite := newSuite2(t, jwks.URL)
	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "events:read"))
	client := dialWithToken(t, suite.lis, tok)

	const consumerID = "identity:svc:primer-lms"
	const wsID = "ws_1"
	const revID = "prev_ev_1"

	// Seed two events of different types via the closed EventType enum.
	ev1 := harness.FixtureEvent("evt_1", v1.EventType_EVENT_TYPE_PLAN_REVISION_PUBLISHED, wsID, revID)
	ev2 := harness.FixtureEvent("evt_2", v1.EventType_EVENT_TYPE_MATERIALIZATION_READY, wsID, "")
	suite.store.SeedEvent(ev1)
	suite.store.SeedEvent(ev2)

	c, cancel := ctx()
	defer cancel()

	// PullEvents returns both unacked events.
	resp, err := client.PullEvents(c, &v1.PullEventsRequest{ConsumerId: consumerID})
	require.NoError(t, err)
	assert.Len(t, resp.GetEvents(), 2)

	// Verify EventType enum values match the closed wire mapping.
	types := make(map[v1.EventType]bool)
	for _, ev := range resp.GetEvents() {
		types[ev.GetType()] = true
	}
	assert.True(t, types[v1.EventType_EVENT_TYPE_PLAN_REVISION_PUBLISHED])
	assert.True(t, types[v1.EventType_EVENT_TYPE_MATERIALIZATION_READY])

	// Acknowledge event 1.
	_, err = client.AcknowledgeEvent(c, &v1.AcknowledgeEventRequest{
		EventId:    "evt_1",
		ConsumerId: consumerID,
	})
	require.NoError(t, err)

	// Ack is durable in the harness store.
	assert.True(t, suite.store.IsAcknowledged("evt_1", consumerID))
	assert.False(t, suite.store.IsAcknowledged("evt_2", consumerID))

	// PullEvents after ack returns only unacked event.
	resp2, err := client.PullEvents(c, &v1.PullEventsRequest{ConsumerId: consumerID})
	require.NoError(t, err)
	require.Len(t, resp2.GetEvents(), 1)
	assert.Equal(t, "evt_2", resp2.GetEvents()[0].GetId())

	// Type filter: pull only PLAN_REVISION_PUBLISHED events — already acked, so 0.
	resp3, err := client.PullEvents(c, &v1.PullEventsRequest{
		ConsumerId: consumerID,
		Types:      []v1.EventType{v1.EventType_EVENT_TYPE_PLAN_REVISION_PUBLISHED},
	})
	require.NoError(t, err)
	assert.Empty(t, resp3.GetEvents())
}

// ────────────────────────────────────────────────────────────────────────────
// Additional: missing materialization → NOT_FOUND; register uses generated descriptor
// ────────────────────────────────────────────────────────────────────────────

func TestE5_GetMaterializationNotFound(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	suite := newSuite2(t, jwks.URL)
	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))
	client := dialWithToken(t, suite.lis, tok)

	c, cancel := ctx()
	defer cancel()
	_, err := client.GetMaterialization(c, &v1.GetMaterializationRequest{MaterializationId: "mat_missing"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcstatus.Code(err))
}

func TestE5_MaterializeRequiresIdempotencyKey(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	suite := newSuite2(t, jwks.URL)
	tok := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write"))
	client := dialWithToken(t, suite.lis, tok)

	c, cancel := ctx()
	defer cancel()
	_, err := client.Materialize(c, &v1.MaterializeRequest{
		Context: &v1.MaterializationContext{PlanRevisionId: "prev_x"},
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
}
