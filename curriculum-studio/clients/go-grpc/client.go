// Package grpcclient is the committed Go gRPC façade for Curriculum Studio
// integration callers. Message types come only from generated protobuf stubs.
package grpcclient

import (
	"context"
	"time"

	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Client wraps the generated CurriculumIntegrationService client.
// It does not copy request/response message structs.
type Client struct {
	inner   v1.CurriculumIntegrationServiceClient
	timeout time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithTimeout sets a default per-RPC timeout when the caller deadline is absent.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.timeout = d
	}
}

// NewClient returns a façade over conn. Dial options, interceptors, and
// credentials belong on the connection; this type only wraps RPCs.
func NewClient(conn grpc.ClientConnInterface, opts ...Option) *Client {
	c := &Client{
		inner:   v1.NewCurriculumIntegrationServiceClient(conn),
		timeout: 0,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.timeout <= 0 || ctx.Err() != nil {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.timeout)
}

// WithBearerToken returns unary and stream interceptors that attach
// `authorization: Bearer <token>` metadata. Callers pass these as dial options.
func WithBearerToken(token string) (grpc.UnaryClientInterceptor, grpc.StreamClientInterceptor) {
	value := "Bearer " + token
	unary := func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		return invoker(metadata.AppendToOutgoingContext(ctx, "authorization", value), method, req, reply, cc, opts...)
	}
	stream := func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(metadata.AppendToOutgoingContext(ctx, "authorization", value), desc, cc, method, opts...)
	}
	return unary, stream
}

func (c *Client) GetPublishedRevision(ctx context.Context, in *v1.GetPublishedRevisionRequest, opts ...grpc.CallOption) (*v1.PlanRevision, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.GetPublishedRevision(ctx, in, opts...)
}

func (c *Client) GetPlanGraph(ctx context.Context, in *v1.GetPlanGraphRequest, opts ...grpc.CallOption) (*v1.PlanGraph, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.GetPlanGraph(ctx, in, opts...)
}

func (c *Client) ValidateRevision(ctx context.Context, in *v1.ValidateRevisionRequest, opts ...grpc.CallOption) (*v1.ValidationReport, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.ValidateRevision(ctx, in, opts...)
}

func (c *Client) GetStandard(ctx context.Context, in *v1.GetStandardRequest, opts ...grpc.CallOption) (*v1.Standard, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.GetStandard(ctx, in, opts...)
}

func (c *Client) ListStandards(ctx context.Context, in *v1.ListStandardsRequest, opts ...grpc.CallOption) (*v1.ListStandardsResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.ListStandards(ctx, in, opts...)
}

func (c *Client) GetResource(ctx context.Context, in *v1.GetResourceRequest, opts ...grpc.CallOption) (*v1.Resource, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.GetResource(ctx, in, opts...)
}

func (c *Client) Materialize(ctx context.Context, in *v1.MaterializeRequest, opts ...grpc.CallOption) (*v1.Materialization, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.Materialize(ctx, in, opts...)
}

func (c *Client) GetMaterialization(ctx context.Context, in *v1.GetMaterializationRequest, opts ...grpc.CallOption) (*v1.Materialization, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.GetMaterialization(ctx, in, opts...)
}

func (c *Client) GetMaterializationBundle(ctx context.Context, in *v1.GetMaterializationBundleRequest, opts ...grpc.CallOption) (*v1.MaterializationBundle, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.GetMaterializationBundle(ctx, in, opts...)
}

func (c *Client) ListMaterializedItems(ctx context.Context, in *v1.ListMaterializedItemsRequest, opts ...grpc.CallOption) (*v1.ListMaterializedItemsResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.ListMaterializedItems(ctx, in, opts...)
}

func (c *Client) AcknowledgeEvent(ctx context.Context, in *v1.AcknowledgeEventRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.AcknowledgeEvent(ctx, in, opts...)
}

func (c *Client) PullEvents(ctx context.Context, in *v1.PullEventsRequest, opts ...grpc.CallOption) (*v1.PullEventsResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.inner.PullEvents(ctx, in, opts...)
}
