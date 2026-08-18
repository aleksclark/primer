package grpcclient_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	grpcclient "github.com/aleksclark/primer/curriculum-studio/clients/go-grpc"
	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

const bufSize = 1024 * 1024

type stubIntegrationServer struct {
	v1.UnimplementedCurriculumIntegrationServiceServer
	lastAuth string
}

func (s *stubIntegrationServer) Materialize(ctx context.Context, req *v1.MaterializeRequest) (*v1.Materialization, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		vals := md.Get("authorization")
		if len(vals) > 0 {
			s.lastAuth = vals[0]
		}
	}
	if req.GetIdempotencyKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key required")
	}
	return &v1.Materialization{Id: "mat_test_1", PlanRevisionId: req.GetContext().GetPlanRevisionId()}, nil
}

func (s *stubIntegrationServer) GetMaterialization(context.Context, *v1.GetMaterializationRequest) (*v1.Materialization, error) {
	return &v1.Materialization{Id: "mat_test_1"}, nil
}

func (s *stubIntegrationServer) GetMaterializationBundle(context.Context, *v1.GetMaterializationBundleRequest) (*v1.MaterializationBundle, error) {
	return &v1.MaterializationBundle{}, nil
}

func (s *stubIntegrationServer) GetPublishedRevision(context.Context, *v1.GetPublishedRevisionRequest) (*v1.PlanRevision, error) {
	return &v1.PlanRevision{}, nil
}

func (s *stubIntegrationServer) GetPlanGraph(context.Context, *v1.GetPlanGraphRequest) (*v1.PlanGraph, error) {
	return &v1.PlanGraph{}, nil
}

func (s *stubIntegrationServer) ValidateRevision(context.Context, *v1.ValidateRevisionRequest) (*v1.ValidationReport, error) {
	return &v1.ValidationReport{}, nil
}

func (s *stubIntegrationServer) GetStandard(context.Context, *v1.GetStandardRequest) (*v1.Standard, error) {
	return &v1.Standard{}, nil
}

func (s *stubIntegrationServer) ListStandards(context.Context, *v1.ListStandardsRequest) (*v1.ListStandardsResponse, error) {
	return &v1.ListStandardsResponse{}, nil
}

func (s *stubIntegrationServer) GetResource(context.Context, *v1.GetResourceRequest) (*v1.Resource, error) {
	return &v1.Resource{}, nil
}

func (s *stubIntegrationServer) ListMaterializedItems(context.Context, *v1.ListMaterializedItemsRequest) (*v1.ListMaterializedItemsResponse, error) {
	return &v1.ListMaterializedItemsResponse{}, nil
}

func (s *stubIntegrationServer) PullEvents(context.Context, *v1.PullEventsRequest) (*v1.PullEventsResponse, error) {
	return &v1.PullEventsResponse{}, nil
}

func (s *stubIntegrationServer) AcknowledgeEvent(context.Context, *v1.AcknowledgeEventRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func dialStub(t *testing.T, srv *stubIntegrationServer, extra ...grpc.DialOption) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	gs := grpc.NewServer()
	v1.RegisterCurriculumIntegrationServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})
	opts := []grpc.DialOption{
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	opts = append(opts, extra...)
	conn, err := grpc.NewClient("passthrough:///bufnet", opts...)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestP4S2NewClientWrapsGeneratedServiceWithoutCopiedMessages(t *testing.T) {
	srv := &stubIntegrationServer{}
	conn := dialStub(t, srv)
	client := grpcclient.NewClient(conn)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := client.Materialize(ctx, &v1.MaterializeRequest{
		IdempotencyKey: "idem-1",
		Context:        &v1.MaterializationContext{PlanRevisionId: "rev_1"},
	})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if got.GetId() != "mat_test_1" {
		t.Fatalf("unexpected id %q", got.GetId())
	}
	gotPkg := reflect.TypeOf(got)
	if gotPkg.Kind() == reflect.Pointer {
		gotPkg = gotPkg.Elem()
	}
	wantPkg := reflect.TypeOf(v1.Materialization{})
	if gotPkg.PkgPath() != wantPkg.PkgPath() {
		t.Fatalf("Materialize returned copied type %s, want generated package %s", gotPkg.PkgPath(), wantPkg.PkgPath())
	}
}

func TestP4S2WithBearerTokenInjectsAuthorizationMetadata(t *testing.T) {
	srv := &stubIntegrationServer{}
	unary, stream := grpcclient.WithBearerToken("tok-abc")
	conn := dialStub(t, srv, grpc.WithUnaryInterceptor(unary), grpc.WithStreamInterceptor(stream))
	client := grpcclient.NewClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Materialize(ctx, &v1.MaterializeRequest{IdempotencyKey: "k"}); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if srv.lastAuth != "Bearer tok-abc" {
		t.Fatalf("authorization metadata = %q, want Bearer tok-abc", srv.lastAuth)
	}
}

func TestP4S2FacadeDoesNotDefineMessageStructs(t *testing.T) {
	dir := filepath.Join(studioRoot(t), "clients", "go-grpc")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			_, ok = ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			name := ts.Name.Name
			switch name {
			case "Client":
				return true
			default:
				t.Errorf("%s defines extra struct %s (façade must not copy messages)", e.Name(), name)
			}
			return true
		})
	}
}

func TestP4S5GeneratedClientExposesAllIntegrationRPCs(t *testing.T) {
	required := []string{
		"Materialize",
		"GetMaterialization",
		"GetMaterializationBundle",
		"GetPublishedRevision",
		"GetPlanGraph",
		"ValidateRevision",
		"GetStandard",
		"ListStandards",
		"GetResource",
		"ListMaterializedItems",
		"PullEvents",
		"AcknowledgeEvent",
	}
	clientType := reflect.TypeOf((*grpcclient.Client)(nil))
	var missing []string
	for _, name := range required {
		if _, ok := clientType.MethodByName(name); !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("client façade missing RPCs: %v", missing)
	}

	genIface := reflect.TypeOf((*v1.CurriculumIntegrationServiceClient)(nil)).Elem()
	for _, name := range required {
		if _, ok := genIface.MethodByName(name); !ok {
			t.Errorf("generated service client missing %s", name)
		}
	}
}
