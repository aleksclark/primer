# clients/go-grpc

Generated Go gRPC integration client façade over `contracts/gen/go` stubs.

- Sources: protobuf via `buf generate`
- Build-only output: `generated/` and/or thin wrappers; stubs under
  `contracts/gen/` remain gitignored
- LMS and other machine callers use this package exclusively
