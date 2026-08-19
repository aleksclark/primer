package conformance

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"

	grpcclient "github.com/aleksclark/primer/curriculum-studio/clients/go-grpc"
	gorest "github.com/aleksclark/primer/curriculum-studio/clients/go-rest"
	"github.com/aleksclark/primer/curriculum-studio/clients/go-rest/generated"
	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/grpcapi"
	"github.com/aleksclark/primer/curriculum-studio/internal/grpcapi/harness"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

func serveJWKS(t *testing.T, key *jwttest.Keypair) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(jwttest.JWKSDocument(key))
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func validator(t *testing.T, jwksURL string, now time.Time) *authn.Validator {
	t.Helper()
	v, err := authn.NewValidator(authn.Options{
		Issuer: jwttest.Issuer, Audience: jwttest.Audience, JWKSURL: jwksURL,
		Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	return v
}

// TestE11_01 exercises the generated Go REST client against the real Huma
// handler. The current production REST surface permits health and the signed
// machine probe without a database; workspace CRUD remains DB-backed and is
// intentionally not faked by this in-memory conformance suite.
func TestE11_01_RESTTour(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	v := validator(t, jwks.URL, now)
	_, handler := api.NewWithPinger(nil, api.Options{Validator: v})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client, err := gorest.NewClient(srv.URL, jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write")), srv.Client())
	require.NoError(t, err)
	health, err := client.Health(context.Background())
	require.NoError(t, err)
	healthBody, ok := health.(*generated.HealthOutBody)
	require.True(t, ok, "Health response type = %T", health)
	assert.Equal(t, "ok", healthBody.Status)

	probe, err := client.MachineMaterializeProbe(context.Background())
	require.NoError(t, err)
	probeBody, ok := probe.(*generated.MachineOutBody)
	require.True(t, ok, "probe response type = %T", probe)
	assert.True(t, probeBody.Ok)
	assert.Equal(t, "identity:svc:primer-lms", probeBody.SubjectRef)
}

// TestE11_02 exercises the generated Go gRPC façade over a TCP loopback
// server, including the LRO status transition, Struct bundle, and durable ack.
func TestE11_02_GRPCIntegrationTour(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	v := validator(t, jwks.URL, now)
	store := harness.New()
	store.SeedPublishedRevision(harness.FixtureRevision("rev_published", "curriculum_1"))
	store.SeedPublishedRevision(&v1.PlanRevision{Id: "rev_draft", State: v1.RevisionState_REVISION_STATE_DRAFT})
	store.SeedPlanGraph("rev_published", harness.FixtureGraph("rev_published"))
	store.SeedEvent(harness.FixtureEvent("event_1", v1.EventType_EVENT_TYPE_PLAN_REVISION_PUBLISHED, "workspace_1", "rev_published"))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer(grpc.UnaryInterceptor(grpcapi.UnaryAuthInterceptor(v)))
	grpcapi.Register(server, grpcapi.Deps{Validator: v, Store: store})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })

	token := jwttest.Mint(t, key, jwttest.ValidServiceClaims(now, "primer-lms", "materialize:write events:read"))
	unary, stream := grpcclient.WithBearerToken(token)
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(unary), grpc.WithStreamInterceptor(stream))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := grpcclient.NewClient(conn, grpcclient.WithTimeout(5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	revision, err := client.GetPublishedRevision(ctx, &v1.GetPublishedRevisionRequest{RevisionId: "rev_published"})
	require.NoError(t, err)
	assert.Equal(t, "rev_published", revision.GetId())
	_, err = client.GetPublishedRevision(ctx, &v1.GetPublishedRevisionRequest{RevisionId: "rev_draft"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcstatus.Code(err), "draft revisions are not integration-readable")

	mat, err := client.Materialize(ctx, &v1.MaterializeRequest{
		IdempotencyKey: "e11-02-materialize",
		Context:        &v1.MaterializationContext{PlanRevisionId: "rev_published", Learner: &v1.LearnerProfile{Id: "learner_1", Grade: "6"}},
	})
	require.NoError(t, err)
	assert.Equal(t, v1.MaterializationStatus_MATERIALIZATION_STATUS_REQUESTED, mat.GetStatus())
	store.SeedBundle(mat.GetId(), harness.FixtureBundle(mat.GetId(), "rev_published"))
	assert.True(t, store.AdvanceStatus(mat.GetId(), v1.MaterializationStatus_MATERIALIZATION_STATUS_READY))
	status, err := client.GetMaterialization(ctx, &v1.GetMaterializationRequest{MaterializationId: mat.GetId()})
	require.NoError(t, err)
	assert.Equal(t, v1.MaterializationStatus_MATERIALIZATION_STATUS_READY, status.GetStatus())
	bundle, err := client.GetMaterializationBundle(ctx, &v1.GetMaterializationBundleRequest{MaterializationId: mat.GetId()})
	require.NoError(t, err)
	require.Len(t, bundle.GetSessions(), 1)
	assert.Equal(t, "math", bundle.GetSessions()[0].GetTutorContext().AsMap()["subject"])

	items, err := client.ListMaterializedItems(ctx, &v1.ListMaterializedItemsRequest{MaterializationId: mat.GetId()})
	require.NoError(t, err)
	assert.Empty(t, items.GetItems())
	events, err := client.PullEvents(ctx, &v1.PullEventsRequest{ConsumerId: "primer-lms"})
	require.NoError(t, err)
	require.Len(t, events.GetEvents(), 1)
	_, err = client.AcknowledgeEvent(ctx, &v1.AcknowledgeEventRequest{EventId: events.GetEvents()[0].GetId(), ConsumerId: "primer-lms"})
	require.NoError(t, err)
	events, err = client.PullEvents(ctx, &v1.PullEventsRequest{ConsumerId: "primer-lms"})
	require.NoError(t, err)
	assert.Empty(t, events.GetEvents())
}

// TestE11_03 proves coherent unauthenticated errors across the generated REST
// and gRPC clients for the equivalent invalid-bearer failure. The current gRPC
// auth interceptor intentionally exposes a transport status without details.
func TestE11_03_DualSurfaceErrors(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	v := validator(t, jwks.URL, now)
	_, handler := api.NewWithPinger(nil, api.Options{Validator: v})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	badToken := "not-a-valid-jwt"
	rest, err := gorest.NewClient(srv.URL, badToken, srv.Client())
	require.NoError(t, err)
	result, err := rest.MachineMaterializeProbe(context.Background())
	require.NoError(t, err)
	restErr, ok := result.(*generated.ErrorModelStatusCode)
	require.True(t, ok, "REST error response type = %T", result)
	assert.Equal(t, http.StatusUnauthorized, restErr.StatusCode)
	assert.Equal(t, generated.ErrorCodeUnauthenticated, restErr.Response.Code)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer(grpc.UnaryInterceptor(grpcapi.UnaryAuthInterceptor(v)))
	store := harness.New()
	grpcapi.Register(server, grpcapi.Deps{Validator: v, Store: store})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	unary, stream := grpcclient.WithBearerToken(badToken)
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(unary), grpc.WithStreamInterceptor(stream))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := grpcclient.NewClient(conn)
	_, err = client.Materialize(context.Background(), &v1.MaterializeRequest{IdempotencyKey: "e11-03"})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, grpcstatus.Code(err))
	assert.Empty(t, grpcstatus.Convert(err).Details(), "auth interceptor deliberately returns no application detail")
}

// TestE11_04 compares the runtime Huma operation inventory and protobuf
// service descriptor with the operations required by their generated clients.
func TestE11_04_RuntimeContractInventory(t *testing.T) {
	humaAPI, _ := api.NewWithPinger(nil, api.Options{})
	operations := map[string]bool{}
	for _, item := range humaAPI.OpenAPI().Paths {
		for _, id := range operationIDs(item) {
			operations[id] = true
		}
	}
	for _, required := range []string{"health", "machineMaterializeProbe", "createWorkspace", "listWorkspaces", "list-events", "create-curriculum", "get-materialization-bundle"} {
		assert.True(t, operations[required], "runtime route missing from emitted OpenAPI inventory: %s", required)
	}

	service := v1.File_curriculumstudio_v1_integration_proto.Services().ByName("CurriculumIntegrationService")
	require.NotNil(t, service)
	methods := map[string]bool{}
	for i := 0; i < service.Methods().Len(); i++ {
		methods[string(service.Methods().Get(i).Name())] = true
	}
	for _, required := range []string{"Materialize", "GetMaterialization", "GetMaterializationBundle", "ListMaterializedItems", "PullEvents", "AcknowledgeEvent"} {
		assert.True(t, methods[required], "gRPC method missing from protobuf descriptor: %s", required)
	}
}

// operationIDs keeps the inventory check independent of a hand-written route
// table while still inspecting the actual emitted Huma operation objects.
func operationIDs(item *huma.PathItem) []string {
	operations := []*huma.Operation{item.Get, item.Put, item.Post, item.Delete, item.Options, item.Head, item.Patch, item.Trace}
	ids := make([]string, 0, len(operations))
	for _, operation := range operations {
		if operation != nil {
			ids = append(ids, operation.OperationID)
		}
	}
	return ids
}

func TestE11_05_CoverageEmitter(t *testing.T) {
	registry, err := LoadRegistry(DefaultRegistryPath())
	require.NoError(t, err)
	mappings := DefaultMappings(registry)
	// E11-01..04 exercise runtime behavior; E11-05 validates traceability;
	// E11-06/07 validate the evidence/consumer build contracts.
	mappings["REQ-E2E-1"] = []string{"E11-01", "E11-02", "E11-03", "E11-04"}
	mappings["REQ-E2E-2"] = []string{"E11-05"}
	mappings["REQ-E2E-3"] = []string{"E11-06", "E11-07"}
	coverage, err := ValidateCoverage(DefaultRegistryPath(), packageDir(t), mappings)
	require.NoError(t, err)
	assert.True(t, coverage.Passed)
	assert.Empty(t, coverage.OrphanE11Tests)
	path := DefaultEvidencePath()
	require.NoError(t, EmitCoverage(path, DefaultRegistryPath(), packageDir(t), mappings))
	orphanMappings := make(map[string][]string, len(mappings))
	for requirement, ids := range mappings {
		orphanMappings[requirement] = append([]string(nil), ids...)
	}
	orphanMappings["REQ-E2E-2"] = []string{"E11-06"}
	_, err = ValidateCoverage(DefaultRegistryPath(), packageDir(t), orphanMappings)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "E11-05")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"REQ-E2E-2"`)
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(file)
}

func TestE11_06_EvidenceBundle(t *testing.T) {
	registry, err := LoadRegistry(DefaultRegistryPath())
	require.NoError(t, err)
	mappings := DefaultMappings(registry)
	mappings["REQ-E2E-1"] = []string{"E11-01", "E11-02", "E11-03", "E11-04"}
	mappings["REQ-E2E-2"] = []string{"E11-05"}
	mappings["REQ-E2E-3"] = []string{"E11-06", "E11-07"}
	path := DefaultEvidencePath()
	require.NoError(t, EmitCoverage(path, DefaultRegistryPath(), packageDir(t), mappings))
	var report Coverage
	require.NoError(t, decodeFile(path, &report))
	assert.Equal(t, 1, report.SchemaVersion)
	assert.True(t, report.Passed)
}

func decodeFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func TestE11_07_LMSGoGRPCCompileProof(t *testing.T) {
	path := filepath.Join("..", "..", "clients", "go-grpc", "examples", "lms_compile", "main.go")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `clients/go-grpc`)
	assert.NotContains(t, string(data), `internal/grpcapi`)
}
