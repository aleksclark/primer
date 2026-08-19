# Curriculum Studio MCP adapter

`internal/mcp` is the source of truth for the third Curriculum Studio contract
surface: tool names, authorization classes, and Go-defined input/output schemas.
It is intentionally separate from Huma/OpenAPI and protobuf generation.

The endpoint is mounted at `POST /mcp` by `cmd/studio-server` and uses the
pinned official Go SDK (`github.com/modelcontextprotocol/go-sdk v1.7.0`) in
stateless Streamable HTTP mode. Requests are authenticated with the existing
Studio JWT validator; the adapter never mints tokens, performs DCR, or accepts
raw Stytch credentials.

The SDK qualification evidence and protocol pin are recorded in `doc.go`.
The initial read tools use injected Studio services. Tools whose owning domain
service is not yet available remain visible in the frozen inventory but fail
closed with an MCP tool error rather than returning fabricated state.

Run the local protocol tests with:

```sh
make -C curriculum-studio studio-mcp-e2e
```
