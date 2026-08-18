# Curriculum Studio contracts phased plan

**Outcome:** Curriculum Studio has an implementation-ready, fail-closed contract
lifecycle: protobuf owns Primer/machine integration; Huma-derived OpenAPI owns
browser/public authoring; generated clients are the exclusive consumption path;
generated outputs are never committed; compatibility and conformance gates are
deterministic from a clean checkout.

**Status:** C1–C5 complete; C4 landed in PR #28 at `db587ed`, C5 in PR #31 at `b35164d`. **Current contracts cursor: C6** (Huma boundary DTOs and offline OpenAPI emission).
**Plan directory:** `agent_docs/plans/curriculum-studio-contracts/`
**Branch base:** `66449725337165c2ef00f7c269633696313e0be2`
**Must-cite:** [foundation crosswalk](../curriculum-studio-foundation-crosswalk.md),
[product plan](../primer-curriculum-studio-product-plan.md),
[identity design](../primer-identity-service-design.md),
[`curriculum-studio/contracts/`](../../../curriculum-studio/contracts/),
[`curriculum-studio/db/`](../../../curriculum-studio/db/),
[LikeC4 primer integration](../../../architecture/curriculum-studio/views/primer-integration.c4).

---

## 1. Current-state summary

### Exists (repository evidence)

| Area | Evidence | State |
| --- | --- | --- |
| Product boundary | `agent_docs/plans/primer-curriculum-studio-product-plan.md` | Locked: Studio owns plan/materials; Primer owns learner runtime |
| Ownership freeze | `agent_docs/plans/curriculum-studio-foundation-crosswalk.md` | Locked L1–L6, enum crosswalk, flow matrix |
| Hand OpenAPI baseline | `curriculum-studio/contracts/openapi/v1/curriculum-studio.yaml` (OpenAPI 3.1, `/studio/v1`) | Compatibility baseline; authoring REST surface |
| Hand protobuf | `curriculum-studio/contracts/proto/curriculumstudio/v1/{common,plan,catalog,materialization,events,integration}.proto` | Integration SoT for context/bundle/events + `CurriculumIntegrationService` |
| Buf module | `curriculum-studio/contracts/buf.yaml`, `buf.gen.yaml` | Module `buf.build/primer/curriculum-studio`; Go plugins pinned; `gen/` gitignored |
| Offline validate | `curriculum-studio/contracts/scripts/validate.sh` | `buf lint/format/build`, `protoc` descriptor, OpenAPI structural/schema validate |
| Studio DB enums | `curriculum-studio/db/SCHEMA.md`, migrations `00001`–`00004` | CHECK constraints; wire strings documented |
| LMS Huma pattern | `server/cmd/openapi-gen/main.go`, `Makefile` `openapi`/`client`, `web` `openapi-typescript` + `openapi-fetch` | Signature-derived OpenAPI for LMS/TV **exists**; Studio does not yet |
| Architecture views | `architecture/curriculum-studio/views/primer-integration.c4` | Sync materialize + async events; no shared DB |
| Identity design | `agent_docs/plans/primer-identity-service-design.md` | Bearer JWT `aud=curriculum-studio`, JWKS, scopes, migration `X-Service-Token` |

### Partial

| Area | Gap |
| --- | --- |
| OpenAPI handoff | YAML is live SoT today; Huma handlers/export path do not exist for Studio |
| Generated clients | No Studio TS/Go client packages; LMS pattern not yet mirrored under `curriculum-studio/` |
| gRPC runtime | Protos define service; no server binary, interceptors, or generated-client E2E |
| Enum parity | Documented in crosswalk; no mechanical gate across proto suffix / OpenAPI enum / DB CHECK |
| Compatibility baselines | README names `buf breaking`; no immutable prior artifact pipeline or OpenAPI breaking gate |
| Exclusive consumption | No lint bans for raw fetch/grpc against Studio |
| Conformance harness | No contract conformance E2E against real handlers/gRPC |

### Missing (this plan delivers)

- Authoritative boundary DTO strategy and package layout for Studio service code
- Deterministic offline generation graph (proto + Huma OpenAPI + clients)
- Qualification spikes for hard wire types
- gRPC integration service adapters/harnesses (not full business materializer)
- Auth metadata / JWT scope attachment on both surfaces
- Error, idempotency, pagination semantics implemented at contract adapters
- Domain-event/webhook envelope conformance
- Breaking-change gates + planted red failures + clean-checkout gates
- Exclusive client consumption policy with unauthorized transport bans

---

## 2. Scope boundaries

### In scope

1. Contract ownership, package boundaries, and generation lifecycle under `curriculum-studio/`
2. Protobuf/Buf modules, descriptor images, Go + TS gRPC client packages (build-only outputs)
3. gRPC server registration surface + integration-service **adapters/harnesses** sufficient for contract E2E
4. OpenAPI transition: hand baseline → Huma handler-signature emission → baseline demoted to immutable compatibility artifact
5. Generated TypeScript (authoring REST) and Go (REST + gRPC) client packages; exclusive consumption
6. Auth presentation metadata (Bearer JWT, migration alias, scopes, gRPC metadata)
7. Error model, idempotency keys, pagination envelopes, long-running materialization status semantics
8. Domain-event and webhook envelopes (wire + delivery headers), pull/ack RPCs
9. Shared closed-enum wire-string parity across proto/OpenAPI/DB **without** a second hand DTO catalog
10. Compatibility baselines, breaking-change gates, determinism, no-tracked-generated-outputs, clean-checkout
11. Contract conformance E2E with real generated clients against real handlers/gRPC server
12. Interfaces/stubs that other plans (platform, DB, Identity, LMS) must satisfy — without owning those plans

### Out of scope (drop table)

| Dropped item | Why | Where it lives instead |
| --- | --- | --- |
| Full plan/materialization business agents | Contract plan only | Future Studio domain/platform plan |
| DB schema redesign / new migrations beyond enum-parity fixtures | DB ownership frozen | `curriculum-studio/db/` + DB plan |
| Primer Identity token broker implementation, JWKS hosting, OIDC login | Identity owns authn | `primer-identity-service-design.md` + Identity plan |
| LMS mastery/runtime, student sessions, BFF cookie cutover | LMS owns learner runtime | LMS / Identity migration plans (S1–S7) |
| Studio UI product features | UI consumes generated client only | Studio UI plan |
| Studio→LMS import push adapter | Crosswalk `#uncertainty` / deferred | Deferred adapter decision |
| TV device tokens | TV-owned | Identity design L5 / TV plans |
| Committed generated `*.pb.go`, OpenAPI IR, TS client sources | Policy I4 | Build outputs under gitignored `gen/` / `.tmp/` / `dist/` |
| Second hand DTO catalog mirroring domain types | Violates non-overlap + signature-derived rules | Boundary types on handlers + proto IDL only |
| Restating `MaterializationContext` / full Primer bundle in OpenAPI | Crosswalk L6 | Protobuf only; OpenAPI authoring subset only |
| Cross-DB FKs / shared Postgres | L1 | Never |

---

## 3. Decision record

| ID | Decision | Rationale |
| --- | --- | --- |
| D1 | **Non-overlap ownership:** OpenAPI owns browser/public authoring REST; protobuf owns Primer/machine integration payloads and `CurriculumIntegrationService`; **MCP owns agent tool schemas** (pinned MCP spec + code-defined JSON Schema) on `/mcp`. | Crosswalk L6–L7; MCP design |
| D2 | **Proto-first exception for gRPC:** `.proto` is SoT for integration RPC/types; generated stubs are build-only; clients are exclusive. | generated-api-client-architecture explicit-IDL exception |
| D3 | **Huma signature-derived OpenAPI for authoring:** once Studio HTTP handlers exist, offline `openapi-gen` (Studio builder) emits OpenAPI; hand YAML becomes **immutable compatibility baseline**, not a live second source. | Matches LMS `server/cmd/openapi-gen`; contracts README handoff clause |
| D4 | **Boundary DTOs live at the edge:** Huma request/response/error structs (authoring) and proto messages (integration) are the only wire models; domain/persistence types map inside adapters — never exported as public DTOs. | Prevents ORM/domain leakage and dual catalogs |
| D5 | **Shared closed enums stay in parity without a second manual DTO source:** one checked **parity fixture** (machine-readable table generated *from* committed sources: proto enum suffixes + OpenAPI enum nodes + DB CHECK extracts) fails CI on drift. Humans edit proto and/or OpenAPI baseline (pre-handoff) and DB migrations only — never a third enum package. Post-Huma handoff, OpenAPI enums come from Go `enum` string types used by handlers; parity fixture compares emitted OpenAPI + proto + DB. | Crosswalk parity rule; avoids DTO cathedral |
| D6 | **No tracked generated outputs:** `curriculum-studio/contracts/gen/**`, `.tmp/**`, client `generated/**`, emitted OpenAPI IR are gitignored; CI fails if tracked. | contracts `.gitignore` + skill I4 |
| D7 | **Separate client packages:** at minimum (1) TS authoring REST client, (2) Go authoring REST client, (3) Go gRPC integration client; optional TS gRPC only if a TS machine caller appears. Façades may inject auth/tracing; may not redefine models. | skill I2 |
| D8 | **Exclusive consumption:** Studio UI, LMS integration adapter, and tests call Studio only via generated clients; raw `fetch`/http/grpc to Studio paths banned except allowlisted harnesses. | skill I3 |
| D9 | **Auth end-state:** `Authorization: Bearer <JWT>` with `aud=curriculum-studio` and required signed public `client_id`; reject `azp` and internal OAuth-client UUIDs. gRPC metadata `authorization: Bearer <JWT>`; `X-Service-Token` migration-only alias. Scopes e.g. `materialize:write`, authoring role scopes as metadata — product authz still local to workspace memberships. | Identity design D4–D7; crosswalk auth table |
| D10 | **Errors:** machine `ErrorCode` wire strings shared; HTTP `application/problem+json` + Huma status; gRPC `google.rpc.Status` + `ErrorDetail`. | common.proto + OpenAPI ErrorModel |
| D11 | **Idempotency:** `Idempotency-Key` header (REST) / `idempotency_key` field (Materialize RPC) required for Materialize, PublishRevision, Export; conflict → `idempotency_key_conflict`. | OpenAPI + integration.proto |
| D12 | **Pagination:** REST `limit`/`offset` + `PageMeta`; gRPC `PageRequest`/`PageResponse` tokens — transport-specific, not domain types. | common.proto comment |
| D13 | **Long-running materialization:** create/materialize returns run resource (`requested`/`running`); bundle fetch fails `failed_precondition` until `ready`; clients poll GetMaterialization / list items. | product plan + protos |
| D14 | **Events:** `DomainEvent` shape + closed `EventType` wire names owned by protobuf; OpenAPI may expose UI event **read** and webhook CRUD without restating Primer-only payload schemas beyond the shared envelope fields needed by UI. | crosswalk flow matrix |
| D15 | **Compatibility baselines:** immutable prior artifacts (buf image + normalized OpenAPI) from release/CI — not hand-edited live schemas. First bootstrap freezes current committed baseline. | skill breaking-change section |
| D16 | **Qualification spikes gate hard types** before mass generation: nullable/optional, timestamp/duration, typed errors, pagination, long-running materialization, `Struct` tutor_context/events. | skill workflow §4 |
| D17 | **Adapters/harnesses only:** this plan may implement contract-facing stubs that satisfy wire semantics (authn parse, enum encode, pagination, idempotency store, fake materialize status machine) without real agent workflows or DB redesign. | task scope |
| D18 | **Module path (frozen):** Go packages under `github.com/aleksclark/primer/curriculum-studio/...` aligning with proto `go_package` `.../curriculum-studio/contracts/gen/go/...`. Root `go.work`/Makefile/CI owned by delivery **F0** only. | integration freeze |
| D19 | **LMS OpenAPI files stay LMS-owned.** Studio never writes `web/openapi.yaml` / `tv-web/openapi.yaml`. | contracts README |
| D20 | **Platform/DB/Identity/LMS interfaces are named contracts between plans**, not ownership transfers (see §7). | task requirement |

---

## 4. Global constraints (iron rules)

1. Do not reopen crosswalk L1–L6 without an explicit decision commit.
2. Do not implement full domain materialization agents in this plan.
3. Do not commit generated stubs, emitted OpenAPI IR, or client `generated/` trees.
4. Do not add a third hand-maintained DTO/enum catalog.
5. Do not restate Primer `MaterializationContext` / full bundle schemas in OpenAPI.
6. Do not share databases or credentials stores with LMS/Identity.
7. Do not skip qualification spikes; STOP/CONDITIONAL must block dependent phases.
8. Every gate must have a planted red failure proof once.
9. Clean checkout must generate → compile → test without pre-existing `gen/`.
10. Prefer exact repo commands: `curriculum-studio/contracts/./scripts/validate.sh`, Buf 1.72.x pins, Huma v2 pattern from LMS.

---

## 5. Phase overview

| Phase | Goal | Depends on |
| --- | --- | --- |
| [Phase 1: Ownership freeze and package layout](./phase-01-ownership-and-package-layout.md) | Freeze layout, modules, gitignore, build graph skeleton, requirement IDs | None |
| [Phase 2: Closed-enum parity without second DTO source](./phase-02-enum-parity.md) | Mechanical proto/OpenAPI/DB wire-string parity gate | Phase 1 |
| [Phase 3: Hard-type qualification spikes](./phase-03-qualification-spikes.md) | PROCEED/STOP spikes for nullable, time, errors, pagination, LRO, Struct | Phase 1 |
| [Phase 4: Protobuf Buf generation and client packages](./phase-04-protobuf-generation.md) | Deterministic buf generate + Go/TS gRPC client packages | Phase 1–3 |
| [Phase 5: gRPC integration service harness](./phase-05-grpc-integration-harness.md) | Real gRPC server + generated-client calls for integration RPCs | Phase 4 |
| [Phase 6: Authoring boundary DTOs and offline Huma emission](./phase-06-huma-openapi-emission.md) | Huma handlers/DTOs + offline OpenAPI emission matching Studio surface | Phase 2–3 |
| [Phase 7: OpenAPI baseline handoff and REST clients](./phase-07-openapi-handoff-and-rest-clients.md) | Demote hand YAML to baseline; generate TS/Go REST clients; exclusive authoring path | Phase 6 |
| [Phase 8: Auth, errors, idempotency, pagination semantics](./phase-08-auth-errors-idempotency-pagination.md) | JWT/metadata scopes, problem+json/rpc errors, idempotency, page envelopes | Phase 5, 7 |
| [Phase 9: Domain events and webhook envelopes](./phase-09-events-webhooks.md) | Event envelope, pull/ack, webhook CRUD/delivery headers conformance | Phase 5, 8 |
| [Phase 10: Compatibility, exclusive-use, clean-checkout gates](./phase-10-compatibility-and-policy-gates.md) | Breaking gates, raw-transport bans, no-tracked-gen, determinism, planted reds | Phase 4–9 |
| [Phase 11: Full contract conformance E2E matrix](./phase-11-conformance-e2e.md) | End-to-end matrix across REST+gRPC with real clients; plan completion evidence | Phase 10 |
| [Phase 12: MCP protocol, tool schemas, and conformance](./phase-12-mcp-protocol-tool-schemas.md) | Third surface SoT (MCP spec + code tool schemas); official + external Streamable HTTP client matrix | Phase 8+ patterns; platform MCP handler for runtime proof |

Phases 4 and 6 may proceed in parallel after Phase 3 PROCEED. Phase 5 needs Phase 4. Phase 7 needs Phase 6. Phase 8 needs both surfaces. Phase 12 may document SoT/inventory in parallel with platform Phase 19 spike; runtime conformance is hard-gated on `/mcp`.

---

## 6. Testing tiers

| Tier | Command / evidence | Requires |
| --- | --- | --- |
| T0 Static | `git diff --check`; markdown link check; plan ID validator | None |
| T1 Contract offline | `cd curriculum-studio/contracts && ./scripts/validate.sh` | buf, protoc, python3 |
| T2 Enum parity | `go test` / script extracting proto+OpenAPI+DB CHECK | Phase 2 artifacts |
| T3 Spike | Per-shape generate+round-trip fixtures (Phase 3) | generators |
| T4 Generate | `buf generate`; Studio `openapi-gen`; client generate — clean tree | pins |
| T5 Unit/adapter | `go test ./curriculum-studio/...` harness tests | none/networkless |
| T6 gRPC E2E | generated Go client → loopback gRPC server | Phase 5 |
| T7 REST E2E | generated TS/Go client → loopback Huma server | Phase 7–8 |
| T8 Conformance | openapi-diff/buf breaking + runtime behavior vs emitted contract | baselines |
| T9 Policy | tracked-gen scan; raw fetch/grpc ban lint; planted red mutations | CI |
| T10 Clean checkout | fresh worktree/clone: generate → test → clean git status | full toolchain |

Identity/JWKS may use **loopback/fixture JWKS** (credential-free). Live Primer Identity token broker is **BLOCKED** to the Identity plan — do not mark live auth complete on fixture JWKS alone.

---

## 7. Cross-plan interfaces (non-ownership)

| Peer plan | This plan provides | This plan requires | Does not own |
| --- | --- | --- | --- |
| **Platform / Studio service** | Package layout, handler registration hooks, openapi-gen builder slot, gRPC service register API | Process binary, config, observability wiring | Business agents, deploy topology |
| **DB** | Enum wire strings + parity fixture expectations; opaque ID mapping at edge | Stable CHECK sets; migrations for any new closed value | Schema redesign, goose runners beyond fixtures |
| **Identity** | Exact JWT claims consumed (`iss`,`aud=curriculum-studio`,`sub`,`scope`,`kid`,`client_id`), absent `azp`, JWKS fetch interface, migration header alias | JWKS document + client_credentials tokens in end-state | Stytch broker/session, account DB, internal OAuth-client UUID |
| **LMS** | gRPC client package + Materialize/GetBundle/PullEvents contracts; event type strings | LMS adapter using generated client only; learner snapshots as opaque context | Mastery truth, session execution, LMS OpenAPI |
| **Studio UI** | TS REST client package façade | UI imports only client package | Visual design, BFF cookie implementation details (Identity+platform) |

---

## 8. Requirement traceability matrix

Every requirement ID appears in ≥1 phase BDD scenario and ≥1 E2E ID.

| Req ID | Requirement | Phase BDD | E2E |
| --- | --- | --- | --- |
| REQ-OWN-1 | Non-overlap OpenAPI vs protobuf ownership | P1-S1, P6-S4, P7-S3, P9-S5 | E1-01, E6-03, E7-04 |
| REQ-OWN-2 | Package boundaries server → emit → client → consumer | P1-S2, P4-S2, P7-S2 | E1-02, E4-02, E7-02 |
| REQ-OWN-3 | No second manual DTO/enum source | P2-S1, P2-S3 | E2-01, E2-03 |
| REQ-OWN-4 | Generated outputs untracked by policy layout | P1-S3, P4-S4 | E1-03, E4-04 |
| REQ-OWN-5 | Cross-plan interface appendix published | P1-S4 | E1-04 |
| REQ-ENUM-1 | Closed enum wire parity proto/OpenAPI/DB | P2-S1, P2-S2 | E2-01, E2-02 |
| REQ-ENUM-2 | Curriculum status API↔DB mapping explicit | P2-S4 | E2-04 |
| REQ-SPIKE-1 | Nullable/optional qualification | P3-S1 | E3-01 |
| REQ-SPIKE-2 | Timestamp/duration qualification | P3-S2 | E3-02 |
| REQ-SPIKE-3 | Typed errors qualification | P3-S3 | E3-03 |
| REQ-SPIKE-4 | Pagination qualification | P3-S4 | E3-04 |
| REQ-SPIKE-5 | Long-running materialization qualification | P3-S5 | E3-05 |
| REQ-SPIKE-6 | Struct tutor_context/events qualification | P3-S6 | E3-06 |
| REQ-SPIKE-7 | Spike outcomes gate later phases | P3-S7 | E3-07 |
| REQ-PROTO-1 | Buf module lint/build/generate deterministic | P4-S1, P4-S3 | E4-01, E4-03 |
| REQ-PROTO-2 | Go gRPC client package from buf generate | P4-S2 | E4-02 |
| REQ-PROTO-3 | Integration service descriptors include all RPCs | P4-S5 | E4-05 |
| REQ-GRPC-1 | Integration service RPCs via generated client | P5-S1, P5-S2, P5-S3, P5-S4 | E5-01, E5-02, E5-03, E5-04 |
| REQ-GRPC-2 | gRPC auth metadata required | P5-S5 | E5-05 |
| REQ-GRPC-3 | PullEvents/AcknowledgeEvent harness | P5-S6 | E5-06, E5-07 |
| REQ-OPEN-1 | Offline Huma OpenAPI emission (no listener) | P6-S1, P6-S2 | E6-01, E6-02 |
| REQ-OPEN-2 | Hand YAML demoted to compatibility baseline | P7-S1, P7-S6 | E7-01 |
| REQ-OPEN-3 | TS + Go REST clients generated, exclusive use | P7-S2, P7-S3, P7-S4 | E7-02, E7-05 |
| REQ-OPEN-4 | Boundary DTOs explicit; emission vs baseline gap tracked | P6-S3, P6-S5 | E6-04, E6-05 |
| REQ-OPEN-5 | Generated clients call real handlers success and typed errors | P7-S5 | E7-03, E7-06 |
| REQ-AUTH-1 | Bearer JWT aud + required public `client_id`/absent `azp` + gRPC metadata | P8-S1, P8-S2 | E8-01, E8-02 |
| REQ-AUTH-2 | Scope metadata + migration X-Service-Token alias | P8-S3, P8-S4 | E8-03, E8-04, E8-05 |
| REQ-ERR-1 | Shared ErrorCode + problem+json / rpc Status | P8-S5 | E8-06 |
| REQ-IDEM-1 | Idempotency key semantics Materialize/Publish/Export | P8-S6 | E8-07 |
| REQ-PAGE-1 | REST limit/offset and gRPC page tokens | P8-S7 | E8-08 |
| REQ-EVT-1 | DomainEvent envelope + event type strings | P9-S1, P9-S2 | E9-01, E9-02 |
| REQ-EVT-2 | Webhook CRUD + delivery headers | P9-S3 | E9-03 |
| REQ-EVT-3 | PullEvents / AcknowledgeEvent | P9-S4 | E9-04 |
| REQ-EVT-4 | UI event read via generated REST client | P9-S5 | E9-05 |
| REQ-COMPAT-1 | buf breaking + OpenAPI breaking vs immutable baseline | P10-S1, P10-S2 | E10-01, E10-02 |
| REQ-POL-1 | No tracked generated outputs | P10-S3 | E10-03 |
| REQ-POL-2 | Unauthorized raw fetch/grpc banned | P10-S4 | E10-04 |
| REQ-POL-3 | Deterministic double-generate | P10-S5 | E10-05 |
| REQ-POL-4 | Planted red failures prove gates | P10-S6 | E10-06 |
| REQ-POL-5 | Clean-checkout gate | P10-S7 | E10-07 |
| REQ-E2E-1 | Full REST+gRPC conformance matrix | P11-S1, P11-S2, P11-S3, P11-S4 | E11-01, E11-02, E11-03, E11-04 |
| REQ-E2E-2 | Traceability coverage JSON complete | P11-S5 | E11-05 |
| REQ-E2E-3 | Plan completion evidence + LMS client compile sample | P11-S6 | E11-06, E11-07 |
| REQ-MCP-1 | MCP third-surface SoT (spec + code schemas; no OpenAPI/proto DTO mirror) | P12-S1 | E12-01 |
| REQ-MCP-2 | Frozen tool inventory + deterministic authz-filtered list | P12-S2 | E12-02 |
| REQ-MCP-3 | Official Go SDK Streamable HTTP conformance tour | P12-S3 | E12-03 |
| REQ-MCP-4 | External Streamable HTTP client interoperability | P12-S4 | E12-04 |
| REQ-MCP-5 | Protocol version / Origin / header negatives | P12-S5 | E12-05 |
| REQ-MCP-6 | Wrong audience, invalid/missing public `client_id`, `azp`/internal UUID, and IDOR handle fail closed | P12-S6 | E12-06 |
| REQ-MCP-7 | Idempotent patch, concurrent conflict, and publish confirmation bound to signed public `client_id` | P12-S7 | E12-07, E12-08 |
| REQ-MCP-8 | Disconnect/cancel semantics | P12-S8 | E12-09 |
| REQ-MCP-9 | REQ-MCP coverage matrix complete | P12-S9 | E12-10 |

---

## 9. Completion rule

The plan is complete only when:

1. All phases 1–12 Completion Gates are checked with real command evidence (Phase 12 runtime rows may share evidence with platform Phase 19).
2. Traceability matrix rows are green (BDD + E2E IDs observed).
3. Clean-checkout T10 passes with empty `gen/` at start and clean git status at end.
4. No tracked files under generated paths; raw Studio transport ban lint is green.
5. Compatibility baselines exist as immutable artifacts with breaking gates red on planted breaks.
6. Hand OpenAPI is not a live second source (handoff complete or explicitly still baseline-only with Huma emission matching it within tolerance and consumers on generated clients).
7. Cross-plan interface appendix is published and no ownership bleed into DB/Identity/LMS business plans.
8. Live Primer Identity token broker and full materialization agents remain explicitly **not claimed**.
9. MCP surface does not duplicate OpenAPI/proto DTOs; official + external client proofs exist or are explicitly BLOCKED with named dependency.

---

## 9b. Delivery orchestration

Wave order and exclusive ownership of codegen vs handlers:
[`../curriculum-studio-delivery/`](../curriculum-studio-delivery/).
This plan owns enum parity, protobuf/OpenAPI lifecycle, contract harnesses, and **MCP tool-schema/conformance** — not business repos or SPA.
MCP design: [`../curriculum-studio-mcp-design.md`](../curriculum-studio-mcp-design.md).

## 10. Rollback posture

- Each phase keeps the repository buildable; prefer feature flags only for migration alias headers, not for dual contract sources.
- If Huma emission diverges from baseline incompatibly, **stop handoff** (Phase 7), keep baseline authoritative, open a spike fix — do not dual-write fields in both forever.
- Compatibility baseline corruption → restore from last green CI artifact; never silently skip breaking checks.
- MCP conformance failures must not be “fixed” by weakening REST/gRPC gates.


## Stytch-backed identity reconciliation (authoritative)

Stytch B2B is upstream human authentication/session authority. Primer Identity is its sole SDK/API client and downstream Primer token broker: it validates opaque Stytch sessions, maps exact `(project_id, organization_id, member_id)` to a local account, and issues only short-lived single-audience Primer JWTs/JWKS. Studio, LMS, TV, MCP, product APIs, and browser JS never receive, store, forward, log, or validate a Stytch session token, SessionJWT, tuple, or role.

Stytch organization/member roles are eligibility hints only. Studio workspace membership, LMS educator roles, and TV device authentication remain local systems of record; a Stytch organization does not create a Studio tenant/workspace. Provisioning is explicit invite/admin only, and distinct cross-org tuples stay distinct personas without email merge/linking. IA is library-only and unapproved; production auth waits for IA-R and IB1–IB4, with signed webhook/cache+grant revocation as a hard BFF/MCP gate.

Required E2Es: mapped tuple to local membership; valid no-membership token denied; same-email cross-org isolation; Stytch admin-like role denied without local role; raw Stytch bearer rejected; outage fails closed/no negative cache; signed webhook forgery/replay/dedupe/out-of-order; explicit revocation bound; LMS local-role dual run; and no token/provider payload in audit logs.
