# Studio MCP — third contract surface

This directory is intentionally thin: **all authoritative schema definitions
live in code**, not here. Do not add JSON Schema trees, OpenAPI fragments, or
protobuf messages that mirror MCP tool input/output types.

## Source of truth

| Artefact | Location |
| --- | --- |
| Tool inventory (names, authz classes) | `internal/mcp/authz.go` — `allToolDefs` |
| Input/output JSON Schemas | Derived from Go struct types by the SDK; registered in `internal/mcp/registry.go` via `sdkmcp.AddTool[In, Out]` |
| Protocol pin | `internal/mcp/config.go` — `ProtocolVersion = "2026-07-28"` |
| SDK pin | `go.mod` — `github.com/modelcontextprotocol/go-sdk v1.7.0` |
| Qualification evidence | `internal/mcp/doc.go` (S19 spike PROCEED) |

## Non-overlap rule

MCP tool schemas must **not** be duplicated in:

- `contracts/openapi/` — Huma/authoring REST surface
- `contracts/proto/` — Primer gRPC integration surface

`tools/contract-gates/ownership_scan.py` and `TestE12_01_OwnershipSoT`
enforce this mechanically.

## Conformance

C12 conformance suite: `internal/mcp/conformance/`

```bash
# Run all E12 tests
make studio-mcp-conformance

# Re-emit mcp-coverage.json evidence
make studio-mcp-coverage

# External client (BLOCKED unless mcporter is in PATH)
make studio-mcp-external-e2e

# Full P19+E12 suite
make studio-mcp-e2e
```

Coverage evidence: `tools/contract-gates/evidence/mcp-coverage.json`

## Authentication

Every `/mcp` request must carry:

```
Authorization: Bearer <Primer JWT>
```

- `aud=curriculum-studio`, `iss=<Identity>`, ES256, `kid` in JWKS
- `client_id` must be a registered public string (not a UUID, not `azp`)
- Human principals: `KindHuman`; service principals: `KindService` (no publish tools)

See `contracts/README.md` §Authentication and `internal/authn/` for validation logic.
