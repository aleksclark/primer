# Curriculum Studio foundation crosswalk

**Status:** Authoritative after integration reconciliation
**Branch:** `planning/curriculum-studio-integration`
**Must-cite for:** any later Studio / Identity / Primer integration plan

This document freezes vocabulary, ownership, and flow decisions across the four
foundation artifacts. Future planners cite it; do not re-open locked boundaries
without an explicit decision commit.

## Locked product boundaries

| # | Decision | Owner artifact |
| --- | --- | --- |
| L1 | Curriculum Studio is **one modular independently deployable service** with its **own completely separate PostgreSQL**. No cross-DB FKs, views, FDW, or dblink in either direction. | product plan, architecture, DB |
| L2 | Primer LMS integrates **only through API/events**. LMS owns learner/mastery/session execution. Studio owns plans, materializations, exports. | product plan, architecture, contracts |
| L3 | **Primer Identity** is a separate service with its **own DB**. It owns authentication, provider identities, sessions/clients/keys — **not** product workspace authorization. | identity design |
| L4 | Auth end-state: Google OIDC → Identity OP; stable `provider+sub`; **no email auto-link**; **host-only BFF cookies**; short-lived **single-audience** JWTs + JWKS; service JWT end-state. `X-Service-Token` is **migration-only**. | identity design, contracts |
| L5 | TV **device** tokens remain TV-owned. Human TV admin **may** migrate to Identity. | identity design |
| L6 | OpenAPI and protobuf have a **non-overlapping ownership split** (below). | contracts |

## Artifact map (canonical paths)

| Concern | Canonical path |
| --- | --- |
| Product plan | `agent_docs/plans/primer-curriculum-studio-product-plan.md` |
| This crosswalk | `agent_docs/plans/curriculum-studio-foundation-crosswalk.md` |
| Identity design | `agent_docs/plans/primer-identity-service-design.md` |
| LikeC4 model | `architecture/curriculum-studio/` |
| Studio service root | `curriculum-studio/` |
| Studio DB | `curriculum-studio/db/` |
| Studio contracts | `curriculum-studio/contracts/` |
| OpenAPI (authoring REST) | `curriculum-studio/contracts/openapi/v1/curriculum-studio.yaml` |
| Protobuf (gRPC + integration) | `curriculum-studio/contracts/proto/curriculumstudio/v1/` |

**Boundary move:** top-level `contracts/` was relocated under
`curriculum-studio/contracts/` so the standalone service tree owns its API
surface alongside `db/`.

## Deployable ownership matrix

| Deployable | Own DB | Owns authn? | Owns product authz? | Owns content/plan? | Owns learner runtime? |
| --- | --- | --- | --- | --- | --- |
| Curriculum Studio | `curriculum_studio` Postgres + artifact object store | No (validates JWT/JWKS) | Yes (workspaces/roles) | Yes | No |
| Primer LMS | LMS Postgres | Migrates to Identity | Yes (educators/students) | No (consumes Studio) | Yes |
| Primer Identity | Identity Postgres | Yes | No | No | No |
| Primer TV | TV Postgres | Device tokens local; human admin may use Identity | Yes (catalog/device) | Media catalog | Device playback |

**Explicit absence:** no Studio↔LMS DB edges; no Identity↔product DB edges;
no Studio credential columns.

## Contract ownership split (enforceable)

| Surface | Source of truth | Audience | May define |
| --- | --- | --- | --- |
| OpenAPI `…/curriculum-studio.yaml` | Hand-authored until Huma generates | Browser / Studio UI REST | Authoring CRUD, generic learner profile, run status, lock/edit, export, webhook CRUD, event **read** for UI |
| Protobuf `curriculumstudio.v1` | Hand-authored | Primer and machine callers (gRPC) | `MaterializationContext`, `MaterializationBundle`, `DomainEvent`, integration RPCs, published-revision reads |

**Parity rule:** shared closed enums (statuses, item kinds, event type strings,
error codes) must match **wire string values** across OpenAPI, proto
(after stripping enum prefix), and DB CHECK constraints. Integration payload
**shapes** live only in protobuf — OpenAPI must not restate
`MaterializationContext` / `MaterializationBundle` / full Primer context.

When a Huma server exists, OpenAPI is regenerated from handlers; this YAML is
the compatibility baseline until then.

## Identifier and subject conventions

| Concept | Canonical form | Notes |
| --- | --- | --- |
| API resource IDs | Opaque prefixed strings (`ws_`, `cur_`, `prev_`, `mat_`, `item_`, …) | Not LMS UUIDs; DB may use UUID PKs mapped at the API edge |
| Human subject_ref | `identity:<uuid>` | UUID is Identity account `sub` |
| Service subject_ref | `identity:svc:<id>` | Service principal id from Identity |
| LMS learner external_ref | opaque LMS id string | Via `integration_identities` only |
| `integration_identities.system` | `primer_lms`, `primer_identity`, `oidc`, `other` | `primer_identity` preferred for Identity-issued subjects; `oidc` retained for generic/external OIDC snapshots |
| Tenant | Studio billing/org root (`tenants`) | Not an Identity tenant; not LMS family id |
| Workspace | Studio authoring boundary (`workspaces`) | Roles live here as projections only |

Identity authenticates; Studio authorizes via `workspace_memberships`.

## Auth presentation and claims

| Caller | End-state header | Migration alias | Validation |
| --- | --- | --- | --- |
| Human (via product BFF) | `Authorization: Bearer <JWT>` | — | Local JWKS; `aud` = product (`curriculum-studio` / `primer-lms`); short TTL |
| Service | `Authorization: Bearer <JWT>` | `X-Service-Token: <JWT>` (discouraged) then legacy static until S7 | Same JWT rules; static path dual-accept then remove |
| gRPC | metadata `authorization: Bearer <JWT>` | no second scheme | Same |

Studio never issues login sessions or stores passwords. Browser sessions are
**host-only BFF cookies** on Studio / LMS / Identity origins — never
`Domain=.example.com`.

## Status and enum crosswalk

Wire strings are lowercase snake_case (OpenAPI/DB). Proto enums use
`PREFIX_VALUE` with the same suffix.

### Materialization run

| Wire | Proto | DB `materialization_runs.status` |
| --- | --- | --- |
| `requested` | `MATERIALIZATION_STATUS_REQUESTED` | `requested` |
| `running` | `…_RUNNING` | `running` |
| `ready` | `…_READY` | `ready` |
| `failed` | `…_FAILED` | `failed` |
| `cancelled` | `…_CANCELLED` | `cancelled` |

### Materialized item kind (canonical closed set)

| Wire | Notes |
| --- | --- |
| `lesson` | |
| `teacher_guide` | |
| `student_instructions` | |
| `practice` | |
| `assignment` | Authoring synonym kept for plan-facing work |
| `discussion_guide` | |
| `worksheet` | |
| `assessment` | |
| `rubric` | |
| `answer_key` | |
| `project_task` | |
| `media_prompt` | |
| `printable_packet` | |
| `session_spec` | Primer-facing session specification item |

### Materialized item lifecycle vs lock

| Concern | Representation |
| --- | --- |
| Lifecycle status | DB/API: `draft`, `ready`, `published`, `superseded` |
| Lock | DB: `locked` boolean + `locked_at` / `locked_by_subject_ref`; API/proto: `ItemLockState` `editable` \| `locked` |
| Rule | Locked body cannot be overwritten by rematerialize; unlock is explicit |

### Plan revision

| Wire | Proto `RevisionState` | DB |
| --- | --- | --- |
| `draft` | `REVISION_STATE_DRAFT` | `draft` |
| `published` | `…_PUBLISHED` | `published` |
| `superseded` | `…_SUPERSEDED` | `superseded` |

### Curriculum durable status

| Layer | Values | Mapping |
| --- | --- | --- |
| DB | `draft`, `active`, `retired` | Storage lifecycle including pre-publish draft and terminal retire |
| API (OpenAPI/proto) | `active`, `archived` | Authoring surface; `archived` ⇔ DB `retired`; create may start `draft` in DB and expose `active` once ready |

### Event type strings

Product plan / outbox / webhooks (dot names):

- `curriculum.created`
- `plan_revision.published`
- `materialization.requested`
- `materialization.ready`
- `materialization.failed`
- `materialized_item.superseded`
- `plan_change.proposed`

Proto `EventType` maps 1:1 (`EVENT_TYPE_CURRICULUM_CREATED` → `curriculum.created`, etc.).

## Flow matrix

| Flow | Direction | Transport | Payload SoT | Notes |
| --- | --- | --- | --- | --- |
| Authoring CRUD | UI → Studio API | HTTPS REST OpenAPI | OpenAPI | BFF cookie → access JWT |
| Sync materialize | LMS → Studio | gRPC (primary) / future machine HTTPS | protobuf | Context in, bundle out |
| Domain events | Studio → subscribers (LMS optional) | Webhook POST + outbox | protobuf `DomainEvent` | Studio works without subscribers |
| JWT validate | Studio/LMS → Identity | HTTPS JWKS fetch | Identity | No login hop on API path |
| Interactive login | Browser → Identity (via BFF) | OIDC auth code + PKCE | Identity | Host-only cookies |
| Service credentials | Caller → Identity token endpoint | client_credentials | Identity | Replaces static secrets |
| Artifacts | Studio modules → object store | SDK/API | refs in Studio DB | Bytes not in Postgres |
| LMS import push | Studio → LMS import | **Deferred** | — | Existing LMS import is parent-session; not Phase-3 primary path |

## Architecture decisions after identity freeze

| Element | Decision |
| --- | --- |
| `identity_service` | External decided system (not `#uncertainty`) |
| Authors → Identity | OIDC interactive login (decided) |
| Studio UI → Identity | OIDC authorize / BFF token acquisition (decided) |
| Studio API → Identity | **JWKS only** (not login, not introspection required) |
| LMS API/Web → Identity | Decided migration target; dual-run until S7 |
| Studio→LMS JSON import push | Remains `#uncertainty` / deferred adapter |
| Studio↔LMS DB | **Absent** (positive proof) |
| Identity DB | Own Postgres; not modeled inside Studio system; no product authz tables |

## Artifact / object-store ownership

- Studio **owns** generated lesson/export **bytes** in its artifact object store
  and **metadata/refs** in Studio Postgres (`artifact_ref` / `artifact_uri`).
- A future separate content service for **uploaded source files** remains deferred.
- LMS does not read the Studio object store; it receives bundles/events only.

## Reconciliation conflict decisions (this integration)

| Drift | Resolution |
| --- | --- |
| Top-level `contracts/` vs service boundary | **Move** to `curriculum-studio/contracts/` |
| Contracts dual permanent service auth | End-state Bearer JWT only; `X-Service-Token` migration alias |
| Architecture `#uncertainty` on all auth edges | Clear for Identity-decided edges; keep LMS import-push uncertain |
| DB `system=oidc` vs `identity:<uuid>` | Document `identity:<uuid>` subject_ref; allow `primer_identity` system value; keep `oidc` |
| Item kinds DB vs OpenAPI/proto | Union closed set in DB + OpenAPI + proto |
| Curriculum status draft/retired vs active/archived | DB richer; API maps `archived`↔`retired` |
| Item status vs lock | Keep both dimensions; document mapping |
| go_package path after move | `…/curriculum-studio/contracts/gen/go/…` |

## Deferred (explicit)

- Full Studio service implementation / Huma handlers
- LMS `educators.identity_subject` column and SPA BFF cutover (S1–S7)
- Whether Studio→LMS curriculum **import push** uses existing parent import or new machine route
- Student/child accounts in Identity; passkeys; DPoP; mTLS service clients
- Splitting Studio modules into separately deployed services
- Separate content-ingest service owning source file bytes
- Exact production hostnames and session TTL tuning
- Identity admin UI polish; SCIM

## Validation gates (integration branch)

```bash
# LikeC4
npx --yes likec4@1.46.0 validate architecture/curriculum-studio
npx --yes likec4@1.46.0 build architecture/curriculum-studio -o /tmp/curriculum-studio-site
npx --yes likec4@1.46.0 export json architecture/curriculum-studio -o /tmp/curriculum-studio-model.json

# Contracts
cd curriculum-studio/contracts && ./scripts/validate.sh

# Studio DB
cd curriculum-studio/db && python3 -m pytest tests -q

# Hygiene
git diff --check
# markdown local-link check + clean generated artifacts
```

## Cite order for later plans

1. Product plan (boundary and phases)
2. **This crosswalk** (vocabulary / ownership / flows)
3. Identity design (auth mechanics)
4. LikeC4 (topology)
5. `curriculum-studio/contracts` + `curriculum-studio/db` (wire + storage)
