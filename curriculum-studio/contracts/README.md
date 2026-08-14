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
| `openapi/v1/curriculum-studio.yaml` | Hand-authored OpenAPI 3.1 | Browser / public authoring REST |
| `proto/curriculumstudio/v1/*.proto` | Hand-authored protobuf | Primer and other services (gRPC) |

**Do not maintain overlapping DTOs.** Integration payloads from the product
plan (`MaterializationContext`, `MaterializationBundle`, `DomainEvent`) live
only in protobuf. Authoring REST may expose a *subset* needed by the Studio
UI (generic learner profile, run status, lock/edit, export, webhook CRUD)
but must not restate the Primer context/bundle schema.

**Parity rule:** closed enum **wire strings** (statuses, item kinds, event
type names, error codes) must match across OpenAPI, protobuf (suffix after
enum prefix), and DB CHECK constraints. Documented in the foundation
crosswalk.

When a Huma server exists, authoring OpenAPI should be *regenerated from
handler signatures* and this YAML becomes the compatibility baseline, not a
second live source. Until then this file is the authoring contract.

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
`contracts-parity`, `contracts-gates`.

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

Generate language stubs locally (requires network for remote plugins):

```bash
buf generate
```

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
