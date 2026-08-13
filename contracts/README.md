# Curriculum Studio API contracts

Versioned, design-first contracts for **Curriculum Studio**, a standalone
planning and lesson-production product. Primer LMS consumes Studio through
these contracts; it does not share a database with Studio.

## Contract ownership

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

Studio does **not** store passwords or issue login sessions.

- Human callers present a bearer token issued by a separate future
  auth/identity service (`Authorization: Bearer`).
- Machine callers present a scoped service credential as
  `X-Service-Token` or as a bearer token (same presentation rule as the
  existing LMS `SharedSecretGuard`).
- Tokens carry workspace and scope claims. Studio authorizes; identity
  authenticates.

## Generated artifact policy

Generated sources are **build outputs and are not committed**:

- `contracts/gen/**` (language stubs)
- descriptor images produced by `buf build` / `protoc` under `contracts/.tmp/`

Commit only:

- `.proto` sources
- OpenAPI source YAML
- `buf.yaml`, `buf.gen.yaml`, Spectral config
- this README and `scripts/validate.sh`

Pin generators in `buf.gen.yaml`. Do not check in `*.pb.go` or TypeScript
clients derived from these files.

## Layout

```text
contracts/
  README.md
  buf.yaml
  buf.gen.yaml
  .gitignore
  proto/curriculumstudio/v1/     # gRPC + shared integration types
  openapi/v1/curriculum-studio.yaml
  lint/.spectral.yaml
  scripts/validate.sh
```

## Validate offline

From `contracts/`:

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

Domain events:

- `curriculum.created`
- `plan_revision.published`
- `materialization.requested`
- `materialization.ready`
- `materialization.failed`
- `materialized_item.superseded`
- `plan_change.proposed`

Studio also works synchronously without event subscribers.
