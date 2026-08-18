// Package grpcapi implements the CurriculumIntegrationService gRPC server
// for Curriculum Studio. It owns service registration, metadata auth
// interception, and contract adapters/harnesses for integration E2E.
//
// Platform hook: call grpcapi.Register(grpcServer, deps) in the Studio binary.
// The server itself does not implement full agent materialisation workflows;
// it delegates to the harness.Store for contract-test fixtures and is
// designed to accept a real domain port in future.
package grpcapi

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/grpcapi/harness"
)

// authContextKey is the context key for the validated AuthContext.
type authContextKey struct{}

// Deps carries the dependencies injected by the platform at startup.
// This is the Register hook described in the plan. The platform binary
// supplies a real Validator and may replace Store with a DB-backed
// implementation in future.
type Deps struct {
	// Validator validates Bearer JWTs. Required.
	Validator *authn.Validator
	// Store is the harness data store. In production this would be a domain
	// service port; for C5 it is an in-memory fixture store.
	Store *harness.Store
}

// Register wires CurriculumIntegrationService onto s using deps.
// Platform binaries call this once during startup:
//
//	grpcapi.Register(grpc.NewServer(...), grpcapi.Deps{Validator: v, Store: s})
//
// The unary auth interceptor must be installed on the server separately via
// grpc.NewServer(grpc.UnaryInterceptor(grpcapi.UnaryAuthInterceptor(validator))).
func Register(s grpc.ServiceRegistrar, deps Deps) {
	v1.RegisterCurriculumIntegrationServiceServer(s, &server{deps: deps})
}

// UnaryAuthInterceptor returns a gRPC unary server interceptor that validates
// Bearer JWT metadata. It uses the supplied Validator (fixture JWKS for tests).
// Calls without a valid token are rejected with codes.Unauthenticated.
func UnaryAuthInterceptor(validator *authn.Validator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		authCtx, err := extractAndValidate(ctx, validator)
		if err != nil {
			return nil, grpcstatus.Error(codes.Unauthenticated, "missing or invalid bearer token")
		}
		ctx = context.WithValue(ctx, authContextKey{}, authCtx)
		return handler(ctx, req)
	}
}

// extractAndValidate pulls the Authorization header from gRPC metadata and
// validates it through authn.Validator.
func extractAndValidate(ctx context.Context, validator *authn.Validator) (authn.AuthContext, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return authn.AuthContext{}, authn.ErrUnauthorized
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return authn.AuthContext{}, authn.ErrUnauthorized
	}
	raw := vals[0]
	token := strings.TrimPrefix(raw, "Bearer ")
	if token == raw { // no prefix stripped
		return authn.AuthContext{}, authn.ErrUnauthorized
	}
	return validator.Validate(ctx, token)
}

// authContextFrom retrieves the validated AuthContext from ctx.
func authContextFrom(ctx context.Context) (authn.AuthContext, bool) {
	v, ok := ctx.Value(authContextKey{}).(authn.AuthContext)
	return v, ok
}

// --- server ---

// server implements CurriculumIntegrationServiceServer backed by a harness store.
type server struct {
	v1.UnimplementedCurriculumIntegrationServiceServer
	deps Deps
}

// domainErr maps application-level conditions to a gRPC status with a typed
// ErrorDetail attached. Compatible with the google.rpc.Status model because
// our proto-v2 ErrorDetail satisfies the protoiface.MessageV1 interface and
// can be packed via status.WithDetails.
func domainErr(code codes.Code, appCode v1.ErrorCode, msg string) error {
	detail := &v1.ErrorDetail{
		Code:    appCode,
		Message: msg,
	}
	st, err := grpcstatus.New(code, msg).WithDetails(detail)
	if err != nil {
		// Fallback: return a plain status if packing fails (should not happen
		// with proto-v2 generated types).
		return grpcstatus.Error(code, msg)
	}
	return st.Err()
}

// notFound returns a codes.NotFound status with an app ErrorCode.
func notFound(resource, id string) error {
	return domainErr(
		codes.NotFound,
		v1.ErrorCode_ERROR_CODE_NOT_FOUND,
		resource+" not found: "+id,
	)
}

// Materialize starts or resumes a production run. Idempotent on idempotency_key.
func (s *server) Materialize(ctx context.Context, req *v1.MaterializeRequest) (*v1.Materialization, error) {
	if req.GetIdempotencyKey() == "" {
		return nil, domainErr(codes.InvalidArgument, v1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "idempotency_key is required")
	}
	materializationContext := req.GetContext()
	if materializationContext == nil {
		materializationContext = &v1.MaterializationContext{}
	}

	res, err := s.deps.Store.CreateMaterialization(req.GetIdempotencyKey(), materializationContext)
	if err != nil {
		if ce, ok := err.(*harness.ErrConflict); ok {
			return nil, domainErr(
				codes.AlreadyExists,
				v1.ErrorCode_ERROR_CODE_IDEMPOTENCY_KEY_CONFLICT,
				"idempotency key conflict: "+ce.Key,
			)
		}
		return nil, grpcstatus.Error(codes.Internal, "store error")
	}
	return res.Materialization, nil
}

// GetMaterialization returns a run by id.
func (s *server) GetMaterialization(ctx context.Context, req *v1.GetMaterializationRequest) (*v1.Materialization, error) {
	mat := s.deps.Store.GetMaterialization(req.GetMaterializationId())
	if mat == nil {
		return nil, notFound("materialization", req.GetMaterializationId())
	}
	return mat, nil
}

// GetMaterializationBundle returns the bundle. Fails with FAILED_PRECONDITION
// until status == READY.
func (s *server) GetMaterializationBundle(ctx context.Context, req *v1.GetMaterializationBundleRequest) (*v1.MaterializationBundle, error) {
	mat := s.deps.Store.GetMaterialization(req.GetMaterializationId())
	if mat == nil {
		return nil, notFound("materialization", req.GetMaterializationId())
	}
	if mat.GetStatus() == v1.MaterializationStatus_MATERIALIZATION_STATUS_FAILED {
		return nil, domainErr(
			codes.FailedPrecondition,
			v1.ErrorCode_ERROR_CODE_MATERIALIZATION_FAILED,
			"materialization failed: "+mat.GetErrorMessage(),
		)
	}
	if mat.GetStatus() != v1.MaterializationStatus_MATERIALIZATION_STATUS_READY {
		return nil, domainErr(
			codes.FailedPrecondition,
			v1.ErrorCode_ERROR_CODE_FAILED_PRECONDITION,
			"materialization not ready: current status "+mat.GetStatus().String(),
		)
	}
	bundle := s.deps.Store.GetBundle(req.GetMaterializationId())
	if bundle == nil {
		return nil, domainErr(codes.Internal, v1.ErrorCode_ERROR_CODE_INTERNAL, "bundle not found for ready materialization")
	}
	return bundle, nil
}

// ListMaterializedItems returns generated items for a run.
func (s *server) ListMaterializedItems(_ context.Context, req *v1.ListMaterializedItemsRequest) (*v1.ListMaterializedItemsResponse, error) {
	if s.deps.Store.GetMaterialization(req.GetMaterializationId()) == nil {
		return nil, notFound("materialization", req.GetMaterializationId())
	}
	return &v1.ListMaterializedItemsResponse{}, nil
}

// GetPublishedRevision returns an immutable published revision.
func (s *server) GetPublishedRevision(_ context.Context, req *v1.GetPublishedRevisionRequest) (*v1.PlanRevision, error) {
	rev := s.deps.Store.GetPublishedRevision(req.GetRevisionId())
	if rev == nil {
		return nil, notFound("published revision", req.GetRevisionId())
	}
	return rev, nil
}

// GetPlanGraph returns the published revision graph.
func (s *server) GetPlanGraph(_ context.Context, req *v1.GetPlanGraphRequest) (*v1.PlanGraph, error) {
	// Require that the revision exists and is published.
	if s.deps.Store.GetPublishedRevision(req.GetRevisionId()) == nil {
		return nil, notFound("published revision", req.GetRevisionId())
	}
	graph := s.deps.Store.GetPlanGraph(req.GetRevisionId())
	if graph == nil {
		return nil, notFound("plan graph", req.GetRevisionId())
	}
	return graph, nil
}

// ValidateRevision runs deterministic invariants against a published revision.
func (s *server) ValidateRevision(_ context.Context, req *v1.ValidateRevisionRequest) (*v1.ValidationReport, error) {
	if s.deps.Store.GetPublishedRevision(req.GetRevisionId()) == nil {
		return nil, notFound("published revision", req.GetRevisionId())
	}
	return s.deps.Store.GetValidationReport(req.GetRevisionId()), nil
}

// GetStandard resolves a standard by code.
func (s *server) GetStandard(_ context.Context, req *v1.GetStandardRequest) (*v1.Standard, error) {
	std := s.deps.Store.GetStandard(req.GetCode())
	if std == nil {
		return nil, notFound("standard", req.GetCode())
	}
	return std, nil
}

// ListStandards pages a catalog. Harness returns empty pages; seeded standards
// support GetStandard only for C5.
func (s *server) ListStandards(_ context.Context, _ *v1.ListStandardsRequest) (*v1.ListStandardsResponse, error) {
	return &v1.ListStandardsResponse{}, nil
}

// GetResource returns a curated resource reference.
func (s *server) GetResource(_ context.Context, req *v1.GetResourceRequest) (*v1.Resource, error) {
	r := s.deps.Store.GetResource(req.GetResourceId())
	if r == nil {
		return nil, notFound("resource", req.GetResourceId())
	}
	return r, nil
}

// PullEvents pages domain events for a consumer.
func (s *server) PullEvents(_ context.Context, req *v1.PullEventsRequest) (*v1.PullEventsResponse, error) {
	var pageSize int32 = 25
	if req.GetPage() != nil && req.GetPage().GetPageSize() > 0 {
		pageSize = req.GetPage().GetPageSize()
	}
	events := s.deps.Store.PullEvents(req.GetConsumerId(), req.GetTypes(), pageSize)
	return &v1.PullEventsResponse{Events: events}, nil
}

// AcknowledgeEvent marks an event as processed for the consumer.
func (s *server) AcknowledgeEvent(_ context.Context, req *v1.AcknowledgeEventRequest) (*emptypb.Empty, error) {
	s.deps.Store.AcknowledgeEvent(req.GetEventId(), req.GetConsumerId())
	return &emptypb.Empty{}, nil
}
