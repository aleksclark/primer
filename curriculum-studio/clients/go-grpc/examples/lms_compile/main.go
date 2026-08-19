// Package main is a compile-only LMS consumer proof. It imports the committed
// gRPC façade, not Studio server or persistence packages.
package main

import (
	"context"

	grpcclient "github.com/aleksclark/primer/curriculum-studio/clients/go-grpc"
	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
	"google.golang.org/grpc"
)

func main() {
	var conn grpc.ClientConnInterface
	client := grpcclient.NewClient(conn)
	_, _ = client.GetPublishedRevision(context.Background(), &v1.GetPublishedRevisionRequest{
		RevisionId: "revision-from-lms",
	})
}
