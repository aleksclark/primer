# Curriculum Studio API contracts

Versioned, design-time contracts for **Curriculum Studio**, a standalone
planning and lesson-production product. Primer LMS consumes Studio through
these contracts; it does not share a database with Studio.

**Service tree location:** `curriculum-studio/contracts/` (co-located with
`curriculum-studio/db/`). See
`agent_docs/plans/curriculum-studio-foundation-crosswalk.md`.

## Contract ownership

Implementation ownership, package map, generated-output policy, and
cross-plan interfaces: [`OWNERS.md`](./OWNERS.md).

One non-duplicative rule:

| Surface | Source of truth | Audience |
| --- | --- | --- |
| Authoring REST | Huma handler signatures via `cmd/openapi-gen` | Browser / public authoring REST |
| `openapi/v1/curriculum-studio.yaml` | Frozen compatibility baseline (C7; not live SoT) | Breaking-change comparison only |
| `proto/curriculumstudio/v1/*.proto` | Hand-authored protobuf | Primer and other services (gRPC) |
| Studio MCP `/mcp` | Pinned official MCP spec + **code-defined tool schemas** beside `internal/mcp` | Curriculum-planning agents |

**Do not maintain overlapping DTOs.** Integration payloads from the product
plan (`MaterializationContext`, `MaterializationBundle`, `DomainEvent`) live
only in protobuf. Authoring REST may expose a *subset* needed by the Studio
UI (generic learner profile, run status, lock/edit, export, webhook CRUD)
but must not restate the Primer context/bundle schema. MCP tool JSON Schemas
must not mirror OpenAPI or protobuf message catalogs; adapters map to the
same application services. See
`agent_docs/plans/curriculum-studio-mcp-design.md` and contracts plan Phase 12.

**Parity rule:** closed enum **wire strings** (statuses, item kinds, event
type names, error codes) must match across OpenAPI, protobuf (suffix after
enum prefix), and DB CHECK constraints. Documented in the foundation
crosswalk.

Authoring OpenAPI is regenerated from Huma handler signatures with
`make contracts-openapi-emit`; the emitted document is the live generation
input. The Huma surface is now live at runtime and this YAML is retained only
as the frozen compatibility baseline. Verify it with
`./scripts/check-baseline.sh`; an intentional baseline refresh must use the
explicit `BASELINE_BOOTSTRAP=1` procedure and commit the digest change.

The C11 conformance matrix runs the generated Go REST and gRPC clients against
loopback Huma/gRPC servers and emits deterministic traceability evidence at
`tools/contract-gates/evidence/conformance-coverage.json`:

```bash
make contracts-conformance
make contracts-ci
```

The matrix covers the generated-client loopback surfaces and the in-memory
integration harness. The following remain explicit handoff blockers rather
than claimed completion: the live Primer Identity token broker/JWKS hosting
and client-credentials issuance, full materialization agent workflows and
artifact storage, the Studio UI BFF cookie integration, LMS production
cutover from static secrets, and the deferred Studio-to-LMS import adapter.
DB-backed authoring CRUD is owned by the Studio platform wave and is not
reimplemented in this contract harness.

LMS `web/openapi.yaml` / `tv-web/openapi.yaml` remain Huma-generated LMS/TV
contracts. They are not Studio contracts.

## Versioning and compatibility

- Package / document version is `v1` (`curriculumstudio.v1`, `/studio/v1`).
- Additive changes inside `v1` are allowed (new optional fields, new RPCs,
  new operations).
- Breaking changes require `v2` (new proto package and new OpenAPI path
  prefix).
- `buf breaking --against <previous-image>` is the proto compatibility gate.
  First baseline is the committed `.proto` set on this branch.

## Authentication

Studio does **not** store passwords or issue login sessions. Primer Identity
issues credentials; Studio authorizes locally.

**End state (required):**

- Humans and services present `Authorization: Bearer <JWT>`.
- JWT is short-lived, single-audience (`aud=curriculum-studio`), verified
  locally via Identity JWKS (`iss`, `exp`, `nbf`, `kid`).
- Human `sub` maps to `workspace_memberships.subject_ref` as
  `identity:<uuid>`. Service principals use `identity:svc:<id>`.
- gRPC metadata uses the same `authorization: Bearer <JWT>` scheme only.

**Migration only (sunset with Identity S7):**

- `X-Service-Token: <JWT>` accepted as a discouraged alias for the same
  access token.
- `X-Service-Token: <legacy static secret>` accepted only while dual-run
  metrics show callers still migrating (same temporary pattern as LMS
  `SharedSecretGuard`).

Studio never treats `X-Service-Token` as a permanent parallel credential
system. See `agent_docs/plans/primer-identity-service-design.md` and the
foundation crosswalk.

## Event delivery envelope (C9)

Webhook deliveries use JSON `DomainEvent` envelope fields from the protobuf
contract and these headers:

- `Content-Type: application/json`
- `X-Curriculum-Studio-Event-Id`
- `X-Curriculum-Studio-Event-Type`
- `X-Curriculum-Studio-Signature` (HMAC signature interface; key material is
  supplied by the platform secrets boundary, not invented by Contracts)

The seven event type wire strings are parity-checked across protobuf, emitted
OpenAPI, and the Studio schema. Missing acknowledgements remain eligible for
at-least-once redelivery; entertainment or Primer-only payload schemas are not
copied into OpenAPI.

## Generated artifact policy

Generated sources are **build outputs and are not committed**:

- `curriculum-studio/contracts/gen/**` (language stubs)
- descriptor images produced by `buf build` / `protoc` under
  `curriculum-studio/contracts/.tmp/`

Commit only:

- `.proto` sources
- OpenAPI source YAML
- `buf.yaml`, `buf.gen.yaml`, Spectral config
- this README and `scripts/validate.sh`

Pin generators in `buf.gen.yaml`. Do not check in `*.pb.go` or TypeScript
clients derived from these files.

## Layout

```text
curriculum-studio/contracts/
  README.md
  OWNERS.md                      # ownership freeze (C1)
  buf.yaml
  buf.gen.yaml
  .gitignore
  proto/curriculumstudio/v1/     # gRPC + shared integration types
  openapi/v1/curriculum-studio.yaml
  lint/.spectral.yaml
  scripts/validate.sh
```

Sibling package map (server edge, clients, gates): see `OWNERS.md` and
`../Makefile` targets `contracts-validate`, `contracts-ownership`,
`contracts-buf-generate`, `clients-go-grpc-build`, `contracts-parity`,
`contracts-gates`, and `contracts-conformance`.

### Current conformance evidence

| Evidence | Command / proof |
| --- | --- |
| E11-01 REST tour | `internal/conformance`, generated Go REST client → loopback Huma |
| E11-02 gRPC tour | `internal/conformance`, generated Go gRPC façade → TCP loopback |
| E11-03 error coherence | generated REST problem response and gRPC unauthenticated status |
| E11-04 inventory | emitted Huma operation and protobuf service descriptors |
| E11-05 traceability | deterministic coverage JSON, registry completeness, orphan rejection |
| E11-06 evidence bundle | coverage artifact validation in the conformance package |
| E11-07 LMS import proof | `clients/go-grpc/examples/lms_compile/main.go`, compiled by `go test ./clients/go-grpc/...` |

The LMS sample intentionally imports only `clients/go-grpc` and protobuf
messages. It does not import `internal/grpcapi`, the Studio server, or the
Studio database; it is an interface compile proof, not a production adapter.

## Closed-enum parity (C2)

Shared closed wire strings are checked mechanically across protobuf enum
suffixes, OpenAPI component enums, and DB CHECK constraints:

```bash
# from curriculum-studio/
make contracts-parity
# or
./contracts/scripts/parity.sh
```

Mapping rules (renames / storage-only only — not a value catalog) live in
`../tools/contract-gates/enum_mappings.yaml`. The extractor never introduces a
third hand-maintained enum SoT.

## Compatibility and policy gates (C10)

The local clean-checkout entry point is `make contracts-ci`. It emits the Huma
spec, generates all ignored clients, checks enum parity, runs protobuf and
OpenAPI breaking gates against committed baselines, rejects unauthorized raw
transport use, runs the full Studio test suite, and verifies no generated
outputs are tracked. `make gates-red-proof` exercises planted failures and
restores the workspace.

Baselines are release artifacts, not live sources:

- `baselines/curriculumstudio.v1.buf.binpb` is checked by
  `scripts/check-buf-breaking.sh`.
- `openapi/v1/curriculum-studio.yaml` is checked by `scripts/check-baseline.sh`
  and `scripts/check-openapi-breaking.sh`.
- Refresh requires a reviewed release change plus
  `BASELINE_BOOTSTRAP=1 scripts/check-baseline.sh` for the OpenAPI digest;
  regenerate the protobuf image explicitly and commit it together.

`tools/contract-gates/raw-transport-allowlist.txt` is narrow and reviewed;
future UI/consumer code must use the generated REST or gRPC façades.

## Validate offline

From `curriculum-studio/contracts/`:

```bash
./scripts/validate.sh
```

This runs `buf lint`, `buf format --diff`, `buf build`, `protoc`
descriptor generation, and OpenAPI 3.1 validation.

Optional Spectral lint (when `@stoplight/spectral-cli` is installed):

```bash
npx --yes @stoplight/spectral-cli@6.15.0 lint \
  openapi/v1/curriculum-studio.yaml \
  --ruleset lint/.spectral.yaml
```

Generate language stubs and REST clients via the production-intended pinned
local paths (no BSR remote plugins — remote execution hit `resource_exhausted`
rate limits and is not reproducible):

```bash
# from curriculum-studio/
make contracts-openapi-emit
make clients-generate
make clients-ts-rest-build
make clients-go-rest-build

# or from curriculum-studio/contracts/ for protobuf only
./scripts/generate.sh
./scripts/generate.sh --twice
```

`./scripts/generate.sh` bootstraps exact pins into the ignored `.tmp` cache,
prepends that private bin to `PATH`, and runs `buf generate` into `gen/go`
(gitignored). Do not invoke ambient host `protoc-gen-go` or BSR remote plugins.

Exact pins (fail closed; no ambient 1.36.5 fallback):

- `protoc-gen-go@v1.36.11` (`google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11`)
- `protoc-gen-go-grpc@v1.5.1` (`google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1`)
- Buf CLI `1.72.x`

C3 qualification (`make contracts-spikes`) bootstraps the private bin, runs the
same committed `buf.gen.yaml` local-plugin config in an isolated temp copy, and
records digests only from that path. Pin drift, missing plugins, or any remote
plugin reference is STOP.

## Primer integration

Primer supplies `curriculumstudio.v1.MaterializationContext` and receives
`curriculumstudio.v1.MaterializationBundle` via
`CurriculumIntegrationService.Materialize` /
`CurriculumIntegrationService.GetMaterializationBundle`.

Domain events (wire strings):

- `curriculum.created`
- `plan_revision.published`
- `materialization.requested`
- `materialization.ready`
- `materialization.failed`
- `materialized_item.superseded`
- `plan_change.proposed`

Studio also works synchronously without event subscribers.
