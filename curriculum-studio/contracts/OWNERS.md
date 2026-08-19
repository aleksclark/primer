# Curriculum Studio contracts — implementation ownership

Frozen ownership for wire contracts and package boundaries. This document is
normative for waves C1+; it does not reopen foundation crosswalk L1–L6.

## Non-overlap rule (REQ-OWN-1)

| Surface | Source of truth | Audience | Must not own |
| --- | --- | --- | --- |
| `openapi/v1/curriculum-studio.yaml` (→ Huma emission post C6/C7) | Browser / public **authoring REST** | Studio UI and public authoring clients | Primer `MaterializationContext`, full `MaterializationBundle`, machine-only integration RPCs |
| `proto/curriculumstudio/v1/*.proto` | Primer / **machine integration** | LMS adapter, service callers, gRPC | Draft graph editing UI DTOs; LMS OpenAPI files |
| `internal/mcp` + pinned MCP spec `2026-07-28` | **MCP tool schemas** (code-defined; C12) | Curriculum-planning agents via `/mcp` Streamable HTTP | OpenAPI or protobuf mirrors of MCP tool input/output types; any schema that duplicates a tool DTO in the other two surfaces |

**Mechanical enforcement:** `tools/contract-gates/ownership_scan.py` fails if
OpenAPI `components.schemas` introduces forbidden integration schema names
(`MaterializationContext`, full Primer bundle field trees beyond the thin
authoring subset already present).

## Package map (REQ-OWN-2)

```text
curriculum-studio/
  contracts/                 # IDL + baseline + offline validate (this tree)
  db/                        # DB-owned schema (parity consumer only)
  cmd/openapi-gen/           # offline OpenAPI emitter (Phase 6)
  cmd/studio-api/            # future process binary hook (platform owns fullness)
  internal/api/              # Huma authoring edge
  internal/grpcapi/          # gRPC integration edge
  internal/boundary/         # shared wire helpers (enum map, ids) — NOT a DTO catalog
  internal/authn/            # JWT/JWKS validate adapter interface
  clients/ts-rest/           # generated TS authoring REST client package
  clients/go-rest/           # generated Go authoring REST client package
  clients/go-grpc/           # generated Go gRPC integration client package
  tools/contract-gates/      # parity, ownership, tracked-gen, spikes
```

**Dependency direction:** `internal/*` and `cmd/*` must never import
`clients/*`. Clients consume generated stubs only; server packages own handlers.

Go module path (frozen, D18):
`github.com/aleksclark/primer/curriculum-studio`

Proto `go_package` remains:
`github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1;curriculumstudiov1`

Root `go.work` / root Makefile / CI are owned by delivery wave **F0** only.

## Generated output policy (REQ-OWN-4 / D6)

Never commit:

- `curriculum-studio/contracts/gen/**`
- `curriculum-studio/contracts/.tmp/**` (includes pinned local plugin cache)
- `curriculum-studio/clients/**/generated/**`
- `curriculum-studio/**/openapi.emitted.yaml`
- `*.pb.go`, `*_grpc.pb.go`, descriptor binaries

Enforced by contracts `.gitignore`, monorepo ignore notes, and
`tools/contract-gates/check_no_tracked_generated.sh`.

## Generator path (C3/C4)

Production and C3 qualification use **local** Buf plugins only (not BSR remote):

- Config: `contracts/buf.gen.yaml` (`local: protoc-gen-go`, `local: protoc-gen-go-grpc`)
- Bootstrap: `contracts/scripts/bootstrap_local_plugins.sh`
  installs exact Go module pins into ignored `.tmp/pinned-plugins/` and fails
  closed on missing/mismatched versions (no ambient fallback).
- Pins: `protoc-gen-go@v1.36.11`, `protoc-gen-go-grpc@v1.5.1`, Buf CLI `1.72.x`.
- C4 production generate: `contracts/scripts/generate.sh` (and
  `make contracts-buf-generate` / `make clients-go-grpc-build`).
  Output: gitignored `contracts/gen/go`. Client façade:
  `clients/go-grpc` (no copied messages).

Remote BSR plugins are non-reproducible under rate limits (`resource_exhausted`)
and must not re-enter the ordinary generate path.

## Cross-plan interfaces (REQ-OWN-5)

These are **named contracts between plans**, not ownership transfers.

### Identity (does not own OP / sessions / Google)

Studio consumes:

- JWT claims: `iss`, `aud=curriculum-studio`, `sub`, `scope`, `kid`, `exp`, `nbf`
- JWKS document fetch interface (URL + key set)
- End-state header: `Authorization: Bearer <JWT>`
- Migration-only alias: `X-Service-Token` (sunset with Identity S7)
- gRPC metadata: `authorization: Bearer <JWT>` only

Studio never mints tokens, stores passwords, or hosts login UI.

### Database (does not own schema redesign)

Studio contracts require:

- Stable CHECK closed sets for shared wire enums
- Documented API↔DB maps (e.g. curricula `archived` ⇔ `retired`)
- Opaque prefixed resource ids at the edge (not LMS UUIDs)

Contracts may add parity fixtures only; migrations remain DB-owned.

### LMS (does not own mastery / sessions)

Studio provides:

- gRPC client package + Materialize / GetBundle / PullEvents contracts
- Event type wire strings

LMS must call Studio only via generated clients (no raw fetch/grpc to Studio).

### Platform / Studio service binary

Contracts provide:

- Package layout and handler registration hooks
- Offline `openapi-gen` builder slot
- gRPC service register API surface

Platform owns process binary, config, observability, and deploy topology.
Business materialization agents are out of scope for the contracts plan.

## Requirement ID registry seed

See `tools/contract-gates/requirements.txt`. Full coverage is validated in C11.

## Citations

- `agent_docs/plans/curriculum-studio-foundation-crosswalk.md` (L1–L6, enum crosswalk)
- `agent_docs/plans/primer-curriculum-studio-product-plan.md`
- `agent_docs/plans/primer-identity-service-design.md`
- `agent_docs/plans/curriculum-studio-contracts/` (phases 1–11)
- `agent_docs/plans/curriculum-studio-delivery/execution-index.md` (C1–C11)
