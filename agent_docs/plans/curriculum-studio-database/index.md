# Curriculum Studio database / persistence plan

**Outcome:** Turn the standalone Curriculum Studio SQL schema artifact into
production-grade Go persistence: dedicated DB config and goose migration
lifecycle, pgx repositories and unit-of-work transactions, tenant/workspace
authorization projections, plan-graph and publication consistency, catalogs,
validation reports, learner/context snapshots, materialization and workflow
checkpointing, materialized-item edit/lock/supersession/provenance, exports and
object refs, transactional outbox/webhook leases/idempotency, and
audit/retention/backup/restore with observability and operational gates.

**Database track cursor:** **D7 complete** (learner/class snapshots,
canonical input fingerprints, unique concurrent run idempotency, and status
transitions; merged at `8079020`). The next wave is **D8 — workflow
checkpointing and fencing**. D1–D7 evidence is present on this branch with
real-Postgres repository and migration tests; the full-module generated gRPC
client gate remains a pre-existing Contracts-track blocker.

**Branch / base:** `planning/curriculum-studio-plan-db` @
`66449725337165c2ef00f7c269633696313e0be2`

**Plan directory:** `agent_docs/plans/curriculum-studio-database/`

**Authority order (do not re-open without a decision commit):**

1. [`../primer-curriculum-studio-product-plan.md`](../primer-curriculum-studio-product-plan.md)
2. [`../curriculum-studio-foundation-crosswalk.md`](../curriculum-studio-foundation-crosswalk.md)
3. [`../primer-identity-service-design.md`](../primer-identity-service-design.md)
4. [`../../../architecture/curriculum-studio/`](../../../architecture/curriculum-studio/) (esp. [`views/data-ownership.c4`](../../../architecture/curriculum-studio/views/data-ownership.c4))
5. [`../../../curriculum-studio/db/`](../../../curriculum-studio/db/) + [`../../../curriculum-studio/contracts/`](../../../curriculum-studio/contracts/)

---

## Current-state summary (evidence-backed)

### Exists (design artifact, not production service code)

| Artifact | Evidence |
| --- | --- |
| Standalone service tree | `curriculum-studio/README.md` — separate boundary from LMS/TV |
| Goose SQL migrations 00001–00007 | `curriculum-studio/db/migrations/` (identity/catalogs, plan domain, materialization/integration, invariants, and D4 catalog/resource policies) |
| Schema docs + ERD | `curriculum-studio/db/SCHEMA.md`, `ERD.md` |
| Executable schema tests (Python + Docker/psycopg2) | `curriculum-studio/db/tests/test_schema.py`; command `cd curriculum-studio/db && python3 -m pytest tests -q` |
| Wire contracts (OpenAPI + proto) | `curriculum-studio/contracts/` |
| LikeC4 data ownership (no cross-DB edges) | `architecture/curriculum-studio/views/data-ownership.c4` |
| LMS/TV reference patterns to mirror (not share) | `server/internal/db/db.go` (`Migrator`, embed goose), `server/internal/repo/{querier,tx}.go`, `server/internal/testutil/db.go`, `server/cmd/migrate` |

### Partial

| Item | State |
| --- | --- |
| Goose migrations 00001–00007 | The initial 00001–00004 history is frozen; D4 added forward-only 00005–00007 catalog/resource policy migrations. Studio owns the migrator and checksum policy. |
| Invariants | Catalog scope, ownership, resource policy, and catalog-prerequisite invariants are enforced in SQL and exercised through D4 repositories. Plan-graph invariants still need repository/concurrency coverage in D5. |
| Python schema suite | Proves SQL invariants against real Postgres; does **not** prove plan repositories, leases, outbox workers, or backup drills |

### Missing (this plan owns)

- Workflow stage/attempt checkpointing, retry, reclaim fencing
- Materialized item edit/lock/supersession/provenance/assessment support repos
- Export metadata + object-store refs (bytes out of Postgres)
- Transactional outbox, webhook delivery leases, inbound idempotency keys
- Audit trail, retention, backup/restore/PITR operational gates, metrics/tracing

---

## Scope boundaries

### In scope

- Everything required to make `curriculum_studio` Postgres the durable source of truth for Studio modules via a Go persistence package
- Schema lifecycle: freeze/adopt current 00001–00004, forward-only production policy, controlled down for non-live envs, additive migrations only after freeze
- Repository layer, transactions, concurrency control, test harness, ops runbooks/gates for DB
- Opaque `identity:<uuid>` / `identity:svc:<id>` and LMS `external_ref` handling **as stored projections/snapshots only**

### Out of scope (drop table)

| Dropped item | Why | Where it lives instead |
| --- | --- | --- |
| HTTP/gRPC business handlers (Huma routes, Connect/gRPC services) | Contract + API tracks own transport | contracts plan / future service API plan |
| Auth provider logic, OIDC, JWKS fetch, BFF cookies, session mint | Identity owns authn | `primer-identity-service-design.md` + identity plan |
| OpenAPI/proto ownership and codegen | Contracts track | `curriculum-studio/contracts/` |
| LMS/TV schema changes or shared DB | Forbidden by L1/L2 | LMS/TV own DBs |
| Agent/LLM workflow implementation (generation quality) | Materialization domain/agent plan | product plan agent modules |
| Object-store byte pipelines (PDF renderers, S3 SDK ops) beyond **refs/checksums** | Artifact store track; DB stores refs only | export/artifact plan |
| Primer LMS import-push adapter | Deferred in crosswalk | integration plan |
| UI / SPA | Separate | Studio UI plan |
| In-memory “repositories” as production path | Explicitly prohibited | tests may use real Postgres only |

---

## Global constraints (every phase)

1. **Separate database.** No FKs, views, FDW, dblink, or connection strings pointing at LMS/TV/Identity DBs. Studio may share a Postgres *instance* only with dedicated DB name and goose table `studio_goose_db_version` and schema `curriculum_studio` (see `SCHEMA.md`).
2. **Opaque external refs.** Humans `identity:<uuid>`; services `identity:svc:<id>`; LMS ids only via `integration_identities.external_ref` + snapshots. Never join out of process to Identity/LMS tables.
3. **No credentials in Studio DB.** Memberships are authorization projections only.
4. **SQL invariants remain authoritative** for publish immutability, acyclic prereqs, locked items, assessment supports, etc. Repositories must not disable triggers or use superuser bypass in production paths.
5. **No in-memory substitute** for required durability (runs, stages, outbox, idempotency, audit).
6. **Transactional consistency.** Multi-row domain writes use a single UoW/transaction (e.g. publish revision + outbox event; stage claim + attempt row).
7. **Migration immutability after live.** Before any env is declared live, decide adopt-as-baseline vs squash-once; after live, never rewrite applied migration checksums—only additive forward migrations.
8. **Docs-only plan commit.** This plan does not implement production code.
9. **Mirror LMS patterns, do not couple packages.** Prefer Studio-local copies of `Migrator`/`Querier`/`WithTx`/testcontainer harness under `curriculum-studio/` (exact package layout chosen in Phase 01/02) rather than importing `server/internal/...` into Studio.
10. **Tenant isolation at every query.** Workspace-scoped reads/writes always constrain `workspace_id` (or join-proven tenant) to prevent IDOR.

---

## Decision record

| ID | Decision | Rationale |
| --- | --- | --- |
| D1 | Studio persistence is a **standalone Go module tree** under frozen `curriculum-studio/` module path `github.com/aleksclark/primer/curriculum-studio` (not inside `server/internal`). | Service boundary L1; integration freeze |
| D2 | Use **goose SQL** embedded via `embed.FS`, version table **`studio_goose_db_version`**, Postgres schema **`curriculum_studio`**. | Already specified in `SCHEMA.md` / README; matches LMS `Migrator` pattern |
| D3 | **Adopt current 00001–00004 as the initial immutable baseline** once Phase 01 freeze gate passes; optional one-time squash is allowed **only before** any environment is declared live and only with a recorded checksum inventory. After live: **no rewriting** applied migrations. | Task requirement; protects prod history |
| D4 | Production migrate entrypoint: Studio-owned binary (e.g. `curriculum-studio/cmd/migrate` or `make studio-migrate`) — **not** extending LMS `server/cmd/migrate` as the long-term owner (a temporary dual registration is forbidden once Studio ships). | Avoid LMS binary owning Studio schema |
| D5 | Repository access via **pgx v5** `Querier` + `WithTx` UoW; domain repos accept `Querier` so tests use tx-rollback fixtures. | Proven in `server/internal/repo` |
| D6 | Integration tests use **real PostgreSQL** (testcontainers-go or `TEST_DATABASE_URL`), never sqlite/miniredis/fake SQL. Python schema tests remain as schema-level CI; Go tests own repository behavior. | Anti-cheat / product durability |
| D7 | Workflow worker leasing uses **DB row fencing** (stage status + attempt_number and/or lease token columns via additive migration if missing)—not only process memory. | Restart recovery + stale writer protection |
| D8 | Outbox is the durable event source; webhook deliveries are at-least-once with `UNIQUE (endpoint_id, event_id)` and `idempotency_key`. | Schema already encodes this |
| D9 | Export/object bytes live in artifact object store; DB stores `artifact_ref` + checksum/status only. | Crosswalk artifact ownership |
| D10 | Authorization persistence stores membership projections only; JWT validation is out of scope. Repos expose `GetActiveMembership(workspace_id, subject_ref)` for upper layers. | Identity design §12 |
| D11 | Materialization fingerprint uniqueness is application+index driven: same `(plan_revision_id, input_fingerprint)` may return existing run (idempotent) per policy coded in Phase 07—not silent duplicate side effects. | `idx_studio_mat_runs_fingerprint` |
| D12 | Failure classes listed in the requirement matrix are first-class E2E scenarios, not aspirational bullets. | Plan completeness |
| D13 | Additive schema changes after freeze (e.g. lease columns, retention markers) ship as `00005+` goose files with Up/Down and extend Python+Go tests. | Forward-only after live |
| D14 | Observability: pool stats, migrate version gauge, outbox lag, lease reclaim counters, slow-query logs—exported via existing Primer OTel conventions when Studio service wiring exists; Phase 12 defines DB-layer metrics hooks. | Ops gates |

---

## Phase overview

| Phase | Goal | Depends on |
| --- | --- | --- |
| [Phase 1: Migration freeze and lifecycle](./phase-01-migration-freeze-and-lifecycle.md) | Freeze/adopt SQL baseline; Studio migrator, config, version table, up/down policy | None |
| [Phase 2: Persistence foundation](./phase-02-persistence-foundation.md) | pgx pool, Querier/UoW, testcontainers harness, repo factory skeleton | Phase 1 |
| [Phase 3: Tenant and workspace authz projections](./phase-03-tenant-workspace-authz.md) | Tenants, workspaces, memberships, integration identity snapshots | Phase 2 |
| [Phase 4: Catalogs and resources](./phase-04-catalogs-and-resources.md) | Frameworks, standards, crosswalks, catalog prereqs, resources | Phase 3 |
| [Phase 5: Plan graph and publication consistency](./phase-05-plan-graph-and-publication.md) | Curricula, revisions, graph writes, publish/supersede, cycle guards | Phase 4 |
| [Phase 6: Validation reports](./phase-06-validation-reports.md) | Persist validation_reports/findings tied to revisions | Phase 5 |
| [Phase 7: Learner snapshots and materialization runs](./phase-07-learner-snapshots-and-runs.md) | Learner profiles, runs, input snapshot + fingerprint idempotency | Phase 5 |
| [Phase 8: Workflow checkpointing and fencing](./phase-08-workflow-checkpointing-and-fencing.md) | Stages/attempts, retry, reclaim, stale lease fencing | Phase 7 |
| [Phase 9: Materialized items lifecycle](./phase-09-materialized-items-lifecycle.md) | Items, edits, locks, supersession, provenance, assessment supports | Phase 8 |
| [Phase 10: Exports and object refs](./phase-10-exports-and-object-refs.md) | Export rows, artifact_ref/checksum, status machine | Phase 9 |
| [Phase 11: Outbox, webhooks, idempotency](./phase-11-outbox-webhooks-idempotency.md) | Transactional outbox, delivery leases, inbound idempotency keys | Phase 5 (events), Phase 7+ for mat events |
| [Phase 12: Audit, retention, backup, observability](./phase-12-audit-retention-backup-observability.md) | Audit_events, retention, PITR/restore drill, metrics, ops Makefile gates | Phases 1–11 |

**Parallelism note:** After Phase 5, Phases 6 and 7 can proceed in parallel. Phase 11’s outbox core can start after Phase 5 but must integrate materialization event types from Phases 7–9 before completion. Phase 12 is last.

---

## Testing tiers

| Tier | Command / harness | Requires | Proves |
| --- | --- | --- | --- |
| T0 Static schema docs | `cd curriculum-studio/db && python3 -m pytest tests/test_schema.py -q -k 'migrations_are_goose or schema_stays or no_credential or docs_cover'` | none | migration pairing, namespace hygiene |
| T1 Python schema E2E | `cd curriculum-studio/db && python3 -m pytest tests -q` | Docker or `TEST_DATABASE_URL` | SQL invariants on real Postgres |
| T2 Go unit (pure) | `cd curriculum-studio && go test ./internal/… -count=1 -short` | Go toolchain | parsers, fingerprint hash, ref formatting |
| T3 Go Postgres integration | `cd curriculum-studio && go test ./internal/persistence/... -count=1` | Docker testcontainers or `TEST_DATABASE_URL` | repositories, UoW, races |
| T4 Migrate CLI | `go run ./curriculum-studio/cmd/migrate up` / `down` against disposable DB | Postgres | lifecycle |
| T5 Ops drill | documented restore/PITR script dry-run in Phase 12 | backup fixtures | recovery |
| T6 Lint/race | `go test -race` on persistence packages; `git diff --check` | — | concurrency + hygiene |

Exact package paths may settle in Phase 01/02 as `curriculum-studio/internal/db` + `…/internal/repo` (recommended) or equivalent; phases name the intended layout and require the Makefile targets `studio-test`, `studio-migrate` (names stabilized in Phase 01).

---

## Requirement traceability matrix

Each requirement maps to BDD scenario IDs and E2E test IDs. Implementers must keep this table green.

| Req ID | Requirement | Phase | BDD scenarios | E2E tests |
| --- | --- | --- | --- | --- |
| REQ-MIG-01 | Dedicated Studio migrator + `studio_goose_db_version` | 1 | P1-S1, P1-S2 | P1-E1, P1-E2 |
| REQ-MIG-02 | Freeze/adopt 00001–00004; no casual rewrite after live | 1 | P1-S3, P1-S4 | P1-E3, P1-E4 |
| REQ-MIG-03 | Up/down policy for non-live vs forbidden destructive prod down | 1 | P1-S5 | P1-E5 |
| REQ-CFG-01 | Dedicated DB config/DSN isolation from LMS/TV | 1 | P1-S6 | P1-E6 |
| REQ-FOUND-01 | pgx pool connect/ping + Querier/WithTx UoW | 2 | P2-S1, P2-S2 | P2-E1, P2-E2 |
| REQ-FOUND-02 | Testcontainers harness + tx rollback fixtures | 2 | P2-S3 | P2-E3 |
| REQ-FOUND-03 | Repository factory wiring | 2 | P2-S4 | P2-E4 |
| REQ-FOUND-04 | No in-memory production repository substitute | 2 | P2-S5 | P2-E5 |
| REQ-AUTHZ-01 | Tenant/workspace CRUD isolation | 3 | P3-S1, P3-S2 | P3-E1, P3-E2 |
| REQ-AUTHZ-02 | Membership projections `identity:<uuid>` / service refs | 3 | P3-S3, P3-S4 | P3-E3, P3-E4 |
| REQ-AUTHZ-03 | Integration identity snapshots opaque; no cross-DB | 3 | P3-S5 | P3-E5 |
| REQ-AUTHZ-04 | Tenant/IDOR denied at repo queries | 3 | P3-S6 | P3-E6 |
| REQ-CAT-01 | Standards frameworks/catalog/crosswalks/prereq DAG | 4 | P4-S1, P4-S2, P4-S3 | P4-E1, P4-E2, P4-E3 |
| REQ-CAT-02 | Resources metadata + artifact_ref only | 4 | P4-S4 | P4-E4 |
| REQ-PLAN-01 | Draft plan graph write/read within revision | 5 | P5-S1 | P5-E1 |
| REQ-PLAN-02 | Immutable publish; only published→superseded | 5 | P5-S2, P5-S3 | P5-E2, P5-E3 |
| REQ-PLAN-03 | Prerequisite-cycle contention rejected | 5 | P5-S4 | P5-E4 |
| REQ-PLAN-04 | Same-revision prereq + child lock on published | 5 | P5-S5 | P5-E5 |
| REQ-VAL-01 | Validation reports/findings persistence | 6 | P6-S1, P6-S2 | P6-E1, P6-E2 |
| REQ-MAT-01 | Learner/class profile snapshots | 7 | P7-S1 | P7-E1 |
| REQ-MAT-02 | Materialization run complete input_snapshot + fingerprint | 7 | P7-S2 | P7-E2 |
| REQ-MAT-03 | Fingerprint idempotency / no duplicate side effects | 7 | P7-S3 | P7-E3 |
| REQ-WF-01 | Stage checkpoint persist + resume | 8 | P8-S1, P8-S2 | P8-E1, P8-E2 |
| REQ-WF-02 | Retry/reclaim fencing; stale lease writer loses | 8 | P8-S3, P8-S4 | P8-E3, P8-E4 |
| REQ-WF-03 | Restart recovery mid-stage | 8 | P8-S5 | P8-E5 |
| REQ-ITEM-01 | Item create with plan node provenance | 9 | P9-S1 | P9-E1 |
| REQ-ITEM-02 | Locked-item overwrite rejected; unlock explicit | 9 | P9-S2, P9-S3 | P9-E2, P9-E3 |
| REQ-ITEM-03 | Edit history + supersession chain | 9 | P9-S4, P9-S5 | P9-E4, P9-E5 |
| REQ-ITEM-04 | Assessment publish requires rubric/answer_key support | 9 | P9-S6 | P9-E6 |
| REQ-EXP-01 | Export status + object ref/checksum | 10 | P10-S1, P10-S2 | P10-E1, P10-E2 |
| REQ-OUT-01 | Transactional outbox with domain write | 11 | P11-S1 | P11-E1 |
| REQ-OUT-02 | Webhook at-least-once + idempotent delivery keys | 11 | P11-S2, P11-S3 | P11-E2, P11-E3 |
| REQ-OUT-03 | Inbound idempotency_keys scope uniqueness | 11 | P11-S4 | P11-E4 |
| REQ-AUD-01 | Audit events for mutating actions | 12 | P12-S1 | P12-E1 |
| REQ-OPS-01 | Retention policy jobs without breaking FK integrity | 12 | P12-S2 | P12-E2 |
| REQ-OPS-02 | Backup/restore + PITR drill | 12 | P12-S3 | P12-E3 |
| REQ-OPS-03 | Observability metrics/gates in Makefile CI | 12 | P12-S4 | P12-E4 |
| REQ-PRIV-01 | External-ref privacy (no credential/PII dumping beyond snapshot policy) | 3,12 | P3-S5, P12-S5 | P3-E5, P12-E5 |

---

## Cross-plan dependencies

| Dependency | Direction | Notes |
| --- | --- | --- |
| Foundation crosswalk L1–L7 | inbound | Locked boundaries (incl. MCP third surface L7) |
| Identity design (subject_ref, no product authz in Identity) | inbound | Memberships only |
| Contracts enums/event strings | inbound parity | DB CHECK ↔ wire strings; this plan does not own contracts |
| MCP design / platform Phase 19 | peer | MCP maps audit/idempotency/optimistic concurrency onto existing tables; the mandatory one-use confirmation record must persist the signed public `client_id` binding, so this track owns the narrow additive migration unless an existing row shape demonstrably enforces every binding |
| Future Studio API/handlers plan | outbound | Consumes repositories from this plan |
| Future agent/materializer plan | outbound | Consumes workflow checkpoint APIs |
| LMS/TV migrate binaries | reference-only | Pattern mirror; no shared version table |
| Artifact object store plan | peer | Bytes vs refs split (D9) |

---

## MCP persistence mapping (no phase renumber)

Curriculum Studio MCP does **not** introduce a third database. Prefer existing tables:

| MCP concern | Tables / phases | Notes |
| --- | --- | --- |
| Workspace authz | tenants, workspaces, workspace_memberships (Phase 3) | Every tool + opaque handle |
| Draft/graph/publish | plan_revisions, graph tables (Phase 5) | Optimistic concurrency on existing version columns |
| Validation findings | validation_reports/findings (Phase 6) | Same as REST |
| Idempotent tool mutations | `idempotency_keys` (Phase 11) | scope e.g. `mcp:<tool>` |
| Audit | `audit_events` (Phase 12) | tool name, subject_ref, workspace_id, opaque ids |
| Publish step-up evidence | Mandatory persisted confirmation record + audit | Bind human subject, signed public `client_id` string, workspace, canonical draft digest, literal confirm tool, issue/consume IDs and five-minute expiry; missing/wrong claim fails closed; never store `azp` or an Identity internal OAuth-client UUID. Label additive `0000N` ownership = this DB track; platform/MCP must not invent ad-hoc SQL |

**MCP confirmation rule:** use existing rows only if their schema and transactional CAS demonstrably enforce every binding above. Otherwise open one narrow additive goose file here with Up/Down + Python/Go tests; do not duplicate broader Identity SQL and never create it from contracts OpenAPI/proto.

---

## Completion rule (full plan)

The database/persistence track is complete only when:

1. All phases 1–12 have every BDD scenario passing with **real Postgres** evidence (T1 and/or T3 as specified).
2. Anti-cheating audits for each phase find no in-memory stand-ins, no trigger-disabling, no cross-DB access, no rewritten live migrations.
3. `studio-migrate up` on empty DB yields schema equal to inventory in `SCHEMA.md` (+ any additive 00005+).
4. Makefile gates `studio-test` / `studio-migrate` / documented restore drill are green locally.
5. Traceability matrix rows are each backed by named automated tests.
6. No production handler/auth/contract work is required to call this track done—but repositories are usable by those tracks without further schema redesign.

---

## Open blockers / assumptions

| ID | Item | Impact |
| --- | --- | --- |
| B1 | Exact Go module path for Studio | **RESOLVED (integration):** `curriculum-studio/go.mod` module `github.com/aleksclark/primer/curriculum-studio`. Identity is separate `primer-identity/` / `github.com/aleksclark/primer/identity`. Root `go.work`/Makefile/CI owned by delivery wave **F0** only. |
| B2 | Whether workflow lease columns need additive migration 00005 (current schema has stage status but no explicit `lease_owner`/`lease_expires_at`) | Phase 8 may add 00005; allowed under D13 |
| B3 | Production backup tooling host (pgBackRest vs managed PITR) not chosen | Phase 12 defines interface + drill against disposable Postgres; prod binder later |
| B4 | Object store vendor (S3/MinIO) | Phase 10 only requires ref string conventions + integrity fields |
| B5 | Sibling API/identity implementation plans may land in parallel | Do not block DB track; keep interfaces stable |

---

## Delivery orchestration

Wave order and cross-plan ownership: [`../curriculum-studio-delivery/`](../curriculum-studio-delivery/).
This plan owns repositories/migrations only — not Huma handlers, SPA, or Identity.

## Document map

- [phase-01-migration-freeze-and-lifecycle.md](./phase-01-migration-freeze-and-lifecycle.md)
- [phase-02-persistence-foundation.md](./phase-02-persistence-foundation.md)
- [phase-03-tenant-workspace-authz.md](./phase-03-tenant-workspace-authz.md)
- [phase-04-catalogs-and-resources.md](./phase-04-catalogs-and-resources.md)
- [phase-05-plan-graph-and-publication.md](./phase-05-plan-graph-and-publication.md)
- [phase-06-validation-reports.md](./phase-06-validation-reports.md)
- [phase-07-learner-snapshots-and-runs.md](./phase-07-learner-snapshots-and-runs.md)
- [phase-08-workflow-checkpointing-and-fencing.md](./phase-08-workflow-checkpointing-and-fencing.md)
- [phase-09-materialized-items-lifecycle.md](./phase-09-materialized-items-lifecycle.md)
- [phase-10-exports-and-object-refs.md](./phase-10-exports-and-object-refs.md)
- [phase-11-outbox-webhooks-idempotency.md](./phase-11-outbox-webhooks-idempotency.md)
- [phase-12-audit-retention-backup-observability.md](./phase-12-audit-retention-backup-observability.md)


## Stytch-backed identity reconciliation (authoritative)

Stytch B2B is upstream human authentication/session authority. Primer Identity is its sole SDK/API client and downstream Primer token broker: it validates opaque Stytch sessions, maps exact `(project_id, organization_id, member_id)` to a local account, and issues only short-lived single-audience Primer JWTs/JWKS. Studio, LMS, TV, MCP, product APIs, and browser JS never receive, store, forward, log, or validate a Stytch session token, SessionJWT, tuple, or role.

Stytch organization/member roles are eligibility hints only. Studio workspace membership, LMS educator roles, and TV device authentication remain local systems of record; a Stytch organization does not create a Studio tenant/workspace. Provisioning is explicit invite/admin only, and distinct cross-org tuples stay distinct personas without email merge/linking. IA is library-only and unapproved; production auth waits for IA-R and IB1–IB4, with signed webhook/cache+grant revocation as a hard BFF/MCP gate.

Required E2Es: mapped tuple to local membership; valid no-membership token denied; same-email cross-org isolation; Stytch admin-like role denied without local role; raw Stytch bearer rejected; outage fails closed/no negative cache; signed webhook forgery/replay/dedupe/out-of-order; explicit revocation bound; LMS local-role dual run; and no token/provider payload in audit logs.
