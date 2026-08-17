# clients/go-grpc

Committed Go gRPC **client façade** for `CurriculumIntegrationService`.

LMS and other machine callers import this package only. Message types and the
generated service client come from `contracts/gen/go` (layout A; matches proto
`go_package`). Generated stubs are never committed.

## Generate + build

From `curriculum-studio/`:

```bash
make contracts-buf-generate   # local pinned plugins only
make clients-go-grpc-build    # generate then go test ./clients/go-grpc
```

Or from `curriculum-studio/contracts/`:

```bash
./scripts/bootstrap_local_plugins.sh
./scripts/generate.sh          # once
./scripts/generate.sh --twice  # determinism
```

Pins (fail closed; no BSR remote / no ambient host plugins):

- Buf CLI `1.72.x`
- `protoc-gen-go@v1.36.11`
- `protoc-gen-go-grpc@v1.5.1`

TS gRPC is deferred (C3 qualified Go + openapi-typescript only).

## Façade

```go
conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithUnaryInterceptor(unary), grpc.WithStreamInterceptor(stream))
client := grpcclient.NewClient(conn, grpcclient.WithTimeout(5*time.Second))
unary, stream := grpcclient.WithBearerToken(token)
```

`NewClient` wraps generated RPCs. `WithBearerToken` is a dial-option helper
only. Do not copy protobuf messages in this package.
