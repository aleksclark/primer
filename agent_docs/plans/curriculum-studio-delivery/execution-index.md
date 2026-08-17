# Curriculum Studio delivery — execution index

**IB1/I5 status: COMPLETE — reviewed credential-free implementation at code tip `59a3998208ba9ef87dfe0bf4a913eedab3753ef8` (`fix(identity): harden IB1 grant and redirect handling`).** Full specification PASS: 0 Critical/Important (nonblocking minors); full quality/security APPROVED: 0 Critical/Important (one nonblocking minor).

Companion to [`index.md`](./index.md). Implementation orchestrators treat this
file as the wave cursor. Detailed BDD scenarios and E2E IDs live in the four
source plans; this file maps them 1:N to master waves.

**Current status:** IB1-E01..E10 are green with credential-free `httptest`, real-PostgreSQL, and real-process evidence; the official Stytch Go v18.1.0 adapter boundary is qualified. Identity coverage is 84.1% (review reruns 84.3% and 84.1%); OpenAPI/generated-client parity and root/foundation gates are green in a clean environment. Live Stytch credentials/browser proof remains **BLOCKED**; IB1 makes no token/JWT/JWKS implementation claim.
**Post-merge cursor:** **PROCEED to IB2 / I6 only after reviewed IB1 reaches `master`, using [`../stytch-identity-ib0/`](../stytch-identity-ib0/) as the design freeze.** IB2 and later runtime remains unimplemented; live Stytch remains **BLOCKED**.

**Legend**

| Prefix | Track |
| --- | --- |
| `F0` | Shared foundation / root ownership (once) |
| `S*` | Studio platform |
| `C*` | Contracts |
| `D*` | Database / persistence |
| `I*` | Primer Identity |
| `X*` | Explicit cross-cutting integration gates |

**Branch / worktree naming**

```text
impl/<wave-id>-<short-slug>          # preferred per-wave worktree branch
impl/cs-local                        # optional local integration tip (no push)
```

Examples: `impl/F0-root-modules`, `impl/D1-migration-freeze`, `impl/I3-ia-r-residual`,
`impl/S2-authz-boundary`.

---

## 1. Review / merge protocol (two-stage)

Every wave:

1. **Spec review (focused):** another agent checks wave scope vs this index +
   cited detailed phase BDD/E2E IDs; rejects scope bleed and ownership races.
2. **Quality/security review:** anti-cheating audit from the detailed phase;
   fail-closed auth; no secret logs; coverage gates; `git diff --check`.
3. **Local integrate only:** fast-forward or merge into `impl/cs-local` (or
   equivalent). **No push / PR / master** without explicit user authorization.
4. **Stop gates:** if any required command RED, or a BLOCKED external dependency
   is claimed green, **STOP** the dependent waves.

### Blocker semantics

| Marker | Meaning |
| --- | --- |
| **HARD** | Downstream waves must not start |
| **SOFT** | Downstream may proceed on stubs/fakes listed as permitted |
| **BLOCKED** | External dependency (credentials, prod host, user approval); wave may complete credential-free slice only |

---

## 2. Global acceptance command catalog

| ID | Command | When |
| --- | --- | --- |
| G-diff | `git diff --check` | every wave |
| G-likec4 | `npx --yes likec4@1.46.0 validate architecture/curriculum-studio` | architecture-touching waves |
| G-db-py | `cd curriculum-studio/db && python3 -m pytest tests -q` | D* schema waves |
| G-contracts | `cd curriculum-studio/contracts && ./scripts/validate.sh` | C* contract waves |
| G-studio-build | `make studio-build` | S*/F0 after targets exist |
| G-studio-test | `make studio-test` | S*/D* after harness |
| G-studio-cover | `make studio-cover` (**≥85%**) | first Studio cover gate onward |
| G-studio-e2e | `make studio-e2e` / `make studio-e2e-go` | UI/process E2E waves |
| G-studio-mcp | `make studio-mcp-e2e` (official SDK + negatives) | S19 / C12 / X7 |
| G-studio-mcp-ext | `make studio-mcp-external-e2e` (Hermes/mcporter or equiv.) | S19 / C12 / X7 |
| G-identity-test | `make identity-test` | I* after shell |
| G-identity-stytch-broker | `make identity-test-oauth` | I4–I10 adversarial |
| G-identity-e2e | `make identity-e2e` | I* process |
| G-identity-cover | `make identity-cover` (≥80% → raise toward 85%) | I* cover |
| G-lms-test | `make test` / focused LMS packages | I13–I14 product migration |
| G-cover-lms | `make cover` (repo 85%) | when LMS packages change |

Focused package commands from detailed phases are **required in addition** when listed on the wave.

---

## 3. Master wave table

### 3.1 Foundation

| Wave | Goal | Owner | Detailed plan refs | Depends on | Parallel group | Branch | Acceptance (min) | Rollback / stop |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **F0** | Shared root: `go.work`, root Makefile stubs (`studio-*`, `identity-*`), CI placeholders, optional dev-compose DB names; scaffold empty module roots if missing | **Platform (sole root owner)** | Platform D1/D11; Identity D1/D18; DB B1 resolved; Contracts D18 | None | **PG0** (solo) | `impl/F0-root-modules` | G-diff; `test -f go.work` or documented deferral; Makefile target names present; no business code | Revert root files only; **HARD** stop if multiple agents edit root |

### 3.2 Identity track (`I*`)

| Wave | Goal | Owner | Detailed refs | Depends on | PG | Branch | Acceptance | Stop |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **I1** | IA foundation / Stytch service shell | Identity | [phase-01](../primer-identity-service/phase-01-service-shell-and-db.md) P1-S*, P1-E* | F0 | **PG1** | `impl/I1-shell-db` | G-identity-test; separate goose table | stop if LMS DSN accepted |
| **I2** | IA foundation / exact tuple mapping | Identity | [phase-02](../primer-identity-service/phase-02-accounts-and-external-identities.md) | I1 | PG2 | `impl/I2-accounts` | G-identity-test | no email auto-link |
| **I3** | IA-R / residual remediation and review | Identity | [phase-03](../primer-identity-service/phase-03-keys-jwks-access-tokens.md) | I2 | PG3 | `impl/I3-ia-r-residual` | G-identity-test; fresh quality/spec review | residual closure only; **not** JWT/JWKS or production auth |
| **I4** | IB0 / reviewed broker, webhook, provisioning design freeze (docs/architecture only) | Identity | [phase-04](../primer-identity-service/phase-04-oauth-clients-and-op.md); [IB0 package](../stytch-identity-ib0/) | I3 | PG4 | `impl/I4-oauth-op` | independent zero-finding exact-tip review after docs links/traceability + LikeC4 1.46 + diff/allowlist gate **PASS** | design complete only; no runtime/live-provider claim |
| **I5** | IB1 / composed Stytch broker exchange — **complete and reviewed** at `59a3998208ba9ef87dfe0bf4a913eedab3753ef8` | Identity | [phase-05](../primer-identity-service/phase-05-sessions-cookies-login-csrf.md) | I4 | PG5 | `impl/I5-sessions` | **Green:** G-identity-stytch-broker; IB1-E01..E10 including IB1-only migration fresh/upgrade/down, sealed state, exactly one unpaginated v18.1.0 `Sessions.Get(OrganizationID,MemberID)`, 1 MiB/256/exact-ID/duplicate behavior, tuple/account and composite association/account FK negatives, authorization-code CAS/unique/index behavior, and no raw provider token; no future-wave tables; 84.1% coverage; OpenAPI/client and root/foundation clean-env gates | Live Stytch credentials/browser proof **BLOCKED**; no token/JWT/JWKS claim |
| **I6** | IB2 / Primer ES256 JWT and JWKS bridge | Identity | [phase-06](../primer-identity-service/phase-06-google-rp-loopback.md) | I5 | PG5b | `impl/IB2-primer-jwks` | G-identity-stytch-broker; IB2-E00..E11 including initial signing-key/refresh-family/current-token migration constraints, copied 400-day issuance evidence with nullable `ON DELETE SET NULL`, required signed public `client_id`, no `azp`/internal UUID, private_key_jwt/RFC7009/sign-before-commit, and Studio MRTR claim binding; prod rejects test IdP | Live Stytch **BLOCKED** |
| **I7** | IB3 / BFF cookie, CSRF and PKCE contract | Identity | [phase-07](../primer-identity-service/phase-07-product-bff-contract.md) | I5, I6 | PG6 | `impl/I7-bff-contract` | G-identity-e2e | **milestone:** Studio BFF OIDC |
| **I8** | IB4 / signed webhook and two-plane revocation | Identity | [phase-08](../primer-identity-service/phase-08-service-principals.md) | I5, I6, **I7** | PG6b | `impl/I8-webhook-revocation` | G-identity-stytch-broker; IB4-E01..E11 including receipt/collision/security-alert/revocation/audit migrations, indexes, leases, delete actions and 400-day retention; signed replay/forgery/dedupe/out-of-order, lease/backoff, four reason-bound collision classes, concurrent one-event/observation/alert, and no authority effect | **milestone:** hard production BFF/MCP revocation gate |
| **I9** | IB5 / Primer-owned service principals and client_credentials | Identity | [phase-09](../primer-identity-service/phase-09-refresh-revoke-logout.md) | I6 | PG7 | `impl/I9-service-principals` | G-identity-e2e; client_credentials class/scope/audience proof | **milestone:** S15 machine JWT |
| **I10** | IB6 / provider-plus-Primer lifecycle | Identity | [phase-10](../primer-identity-service/phase-10-key-rotation-and-hardening.md) | I7, I8 | PG8 | `impl/I10-lifecycle` | G-identity-test; IB6-E01 including refresh rotation/reuse terminal constraints, restart/upgrade durability, and 400-day refresh/grant/revocation/audit retention | — |
| **I11** | IB7 / key rotation and hardening | Identity | [phase-11](../primer-identity-service/phase-11-admin-audit-recovery.md) | I6, I8 | PG8 | `impl/I11-hardening` | G-identity-test; IB7-E01..E04 including the full next/active/retired/destroyed migration/rotation lifecycle, at-most-one active/next, ordered timestamps, and no ciphertext after destroy | — |
| **I12** | IB8 / Studio local-authorization proof | Identity (+ Platform consumer) | [phase-12](../primer-identity-service/phase-12-studio-integration.md); Platform S2 | I6, I8, S2 | PG9 | `impl/I12-studio-integration` | G-identity-e2e + G-studio-test joint | **milestone:** applicable production validator/cutover chain |
| **I13** | IB8 / LMS dual-run local-role cutover | Identity | [phase-13](../primer-identity-service/phase-13-lms-dual-login-and-service-cutover.md) | **I7–I10** | PG10 | `impl/I13-lms-dual` | G-lms-test; empty secret fail-closed | close fail-open **before S7** |
| **I14** | IB8 / TV admin, operations and live Stytch cutover | Identity | [phase-14](../primer-identity-service/phase-14-tv-admin-s7-ops-live.md) | **I8–I11, I13**, approved live credentials/configuration | PG11 | `impl/I14-s7-ops` | G-lms-test; device tokens still local | Live Stytch **BLOCKED** without approval |

### 3.3 Database track (`D*`)

| Wave | Goal | Owner | Detailed refs | Depends on | PG | Branch | Acceptance | Stop |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **D1** | Migration freeze + Studio migrator lifecycle | Database | [phase-01](../curriculum-studio-database/phase-01-migration-freeze-and-lifecycle.md) | F0 | **PG1** | `impl/D1-migration-freeze` | G-db-py; migrate up/down policy | no root Makefile race |
| **D2** | pgx pool, Querier/UoW, testcontainers harness | Database | [phase-02](../curriculum-studio-database/phase-02-persistence-foundation.md) | D1 | PG1 | `impl/D2-persistence-foundation` | G-studio-test (persistence pkgs) | no in-memory prod repos |
| **D3** | Tenant/workspace/membership projections | Database | [phase-03](../curriculum-studio-database/phase-03-tenant-workspace-authz.md) | D2 | PG2 | `impl/D3-authz-projections` | G-studio-test | — |
| **D4** | Catalogs + resources repos | Database | [phase-04](../curriculum-studio-database/phase-04-catalogs-and-resources.md) | D3 | PG3b | `impl/D4-catalogs` | G-studio-test | — |
| **D5** | Plan graph + publish consistency | Database | [phase-05](../curriculum-studio-database/phase-05-plan-graph-and-publication.md) | D4 | PG4b | `impl/D5-plan-graph` | G-studio-test | — |
| **D6** | Validation reports persistence | Database | [phase-06](../curriculum-studio-database/phase-06-validation-reports.md) | D5 | PG5c | `impl/D6-validation-reports` | G-studio-test | parallel D7 OK |
| **D7** | Learner snapshots + mat runs | Database | [phase-07](../curriculum-studio-database/phase-07-learner-snapshots-and-runs.md) | D5 | PG5c | `impl/D7-mat-runs` | G-studio-test | parallel D6 OK |
| **D8** | Workflow checkpointing + fencing | Database | [phase-08](../curriculum-studio-database/phase-08-workflow-checkpointing-and-fencing.md) | D7 | PG6b | `impl/D8-workflow-fencing` | G-studio-test -race | additive 00005 OK |
| **D9** | Materialized items lifecycle | Database | [phase-09](../curriculum-studio-database/phase-09-materialized-items-lifecycle.md) | D8 | PG7b | `impl/D9-items` | G-studio-test | — |
| **D10** | Exports + object refs (no bytes) | Database | [phase-10](../curriculum-studio-database/phase-10-exports-and-object-refs.md) | D9 | PG8b | `impl/D10-export-refs` | G-studio-test | — |
| **D11** | Outbox, webhook leases, idempotency keys | Database | [phase-11](../curriculum-studio-database/phase-11-outbox-webhooks-idempotency.md) | D5 (core); D7–D9 for mat events | PG8b | `impl/D11-outbox` | G-studio-test | — |
| **D12** | Audit, retention, backup drill, DB metrics | Database | [phase-12](../curriculum-studio-database/phase-12-audit-retention-backup-observability.md) | D1–D11 | PG9b | `impl/D12-ops` | G-studio-cover ≥85%; backup drill | — |

### 3.4 Contracts track (`C*`)

| Wave | Goal | Owner | Detailed refs | Depends on | PG | Branch | Acceptance | Stop |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **C1** | Ownership freeze + package layout | Contracts | [phase-01](../curriculum-studio-contracts/phase-01-ownership-and-package-layout.md) | F0 | **PG1** | `impl/C1-ownership` | G-contracts; layout docs | no gen committed |
| **C2** | Closed-enum parity gate | Contracts | [phase-02](../curriculum-studio-contracts/phase-02-enum-parity.md) | C1 | PG1 | `impl/C2-enum-parity` | parity script RED on drift | — |
| **C3** | Hard-type qualification spikes | Contracts | [phase-03](../curriculum-studio-contracts/phase-03-qualification-spikes.md) | C1 | PG1 | `impl/C3-spikes` | PROCEED/STOP recorded | **HARD** if STOP |
| **C4** | Protobuf buf generate + gRPC client pkgs | Contracts | [phase-04](../curriculum-studio-contracts/phase-04-protobuf-generation.md) | C1–C3 | PG3 | `impl/C4-protobuf-gen` | clean-tree generate | parallel C6 after C3 |
| **C5** | gRPC integration service harness | Contracts | [phase-05](../curriculum-studio-contracts/phase-05-grpc-integration-harness.md) | C4 | PG4 | `impl/C5-grpc-harness` | generated client E2E | stubs only OK |
| **C6** | Huma boundary DTOs + offline OpenAPI emission | Contracts | [phase-06](../curriculum-studio-contracts/phase-06-huma-openapi-emission.md) | C2–C3; needs S1 handler host hooks | PG3 / after S1 | `impl/C6-huma-emit` | openapi-gen offline | may stub handlers |
| **C7** | OpenAPI baseline handoff + REST clients | Contracts | [phase-07](../curriculum-studio-contracts/phase-07-openapi-handoff-and-rest-clients.md) | C6 | PG5d | `impl/C7-openapi-clients` | TS/Go clients gen | stop dual SoT |
| **C8** | Auth/errors/idempotency/pagination semantics | Contracts | [phase-08](../curriculum-studio-contracts/phase-08-auth-errors-idempotency-pagination.md) | C5, C7 | PG6c | `impl/C8-auth-errors` | REST+gRPC semantics | X-Service-Token alias only |
| **C9** | Events + webhook envelope conformance | Contracts | [phase-09](../curriculum-studio-contracts/phase-09-events-webhooks.md) | C5, C8 | PG7c | `impl/C9-events` | envelope tests | — |
| **C10** | Compatibility + exclusive-use + clean-checkout gates | Contracts | [phase-10](../curriculum-studio-contracts/phase-10-compatibility-and-policy-gates.md) | C4–C9 | PG8c | `impl/C10-policy-gates` | planted reds | — |
| **C11** | Full contract conformance E2E matrix | Contracts | [phase-11](../curriculum-studio-contracts/phase-11-conformance-e2e.md) | C10 | PG9c | `impl/C11-conformance` | matrix green | — |
| **C12** | MCP protocol + tool-schema conformance (official + external client) | Contracts | [phase-12](../curriculum-studio-contracts/phase-12-mcp-protocol-tool-schemas.md); [MCP design](../curriculum-studio-mcp-design.md); [IB0 MCP contract](../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md) | C8+; runtime hard on S19 transport | PG-MCP | `impl/C12-mcp-conformance` | stateless 2026-07-28 clients; RFC 9728 metadata; static registration; required public `client_id` and missing/wrong/`azp`/internal-UUID MRTR negatives; no OpenAPI/proto DTO mirror | **HARD** if custom transport/DCR invented |

### 3.5 Platform track (`S*`)

| Wave | Goal | Owner | Detailed refs | Depends on | PG | Branch | Acceptance | Stop |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **S1** | Studio service shell | Platform | [phase-01](../curriculum-studio-platform/phase-01-service-shell.md) | F0; consumes D1 migrations | PG2 | `impl/S1-service-shell` | G-studio-build; G-studio-test | separate DB only |
| **S2** | Authz boundary (validator-only) | Platform | [phase-02](../curriculum-studio-platform/phase-02-authz-boundary.md) | S1, D3; credential-free via test Identity **or** narrow verifier; **prod validator/cutover** needs I6+I8 and applicable I12 | PG3 / credential-free independent | `impl/S2-authz` | G-studio-test; prod rejects test mode | never mint tokens |
| **S3** | Workspaces API | Platform | [phase-03](../curriculum-studio-platform/phase-03-workspaces-api.md) | S2, D3 | PG4c | `impl/S3-workspaces` | G-studio-test | — |
| **S4** | Standards catalog API | Platform | [phase-04](../curriculum-studio-platform/phase-04-standards-catalog.md) | S3, D4, C2 | PG5e | `impl/S4-standards` | G-studio-test | parallel S5 |
| **S5** | Resource catalog API | Platform | [phase-05](../curriculum-studio-platform/phase-05-resource-catalog.md) | S3, D4 | PG5e | `impl/S5-resources` | G-studio-test | parallel S4 |
| **S6** | Plan domain drafts | Platform | [phase-06](../curriculum-studio-platform/phase-06-plan-domain-drafts.md) | S4–S5, D5 | PG6d | `impl/S6-plan-drafts` | G-studio-test | — |
| **S7** | Validation engine | Platform | [phase-07](../curriculum-studio-platform/phase-07-validation-engine.md) | S6, D6 | PG7d | `impl/S7-validation` | G-studio-test | — |
| **S8** | Publish immutability + plan outbox enqueue | Platform | [phase-08](../curriculum-studio-platform/phase-08-publish-immutability.md) | S7, D5, D11 core | PG7d | `impl/S8-publish` | G-studio-test | — |
| **S9** | SPA/BFF shell (house system) | Platform | [phase-09](../curriculum-studio-platform/phase-09-spa-bff-shell.md); [IB0 BFF contract](../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md) | S3; credential-free shell may use test Identity; **live BFF** requires I7+I8 | PG5e soft / live BFF hard after I7+I8 | `impl/S9-spa-bff` | G-studio-e2e shell | exact registered redirect/resource/audience; host-only cookie; no browser token |
| **S10** | Planning MVP UI + MD/PDF export | Platform | [phase-10](../curriculum-studio-platform/phase-10-planning-mvp-ui.md) | S8–S9, S5 | PG8d | `impl/S10-planning-mvp` | G-studio-e2e | — |
| **S11** | Materialization domain | Platform | [phase-11](../curriculum-studio-platform/phase-11-materialization-domain.md) | S8, D7, D9 | PG9d | `impl/S11-materialization` | G-studio-test | — |
| **S12** | Agent workflow runner (scripted model) | Platform | [phase-12](../curriculum-studio-platform/phase-12-agent-workflow-runner.md) | S11, D8 | PG10d | `impl/S12-workflow` | G-studio-test resume/kill | live models BLOCKED |
| **S13** | Exports + artifact bytes store | Platform | [phase-13](../curriculum-studio-platform/phase-13-exports-artifacts.md) | S11–S12, D10 | PG10d | `impl/S13-exports` | G-studio-test; object store real | bytes not in PG |
| **S14** | Outbox worker + signed webhooks | Platform | [phase-14](../curriculum-studio-platform/phase-14-outbox-webhooks.md) | S8, S11, D11, C9 | PG11d | `impl/S14-outbox-webhooks` | G-studio-test delivery | — |
| **S15** | Primer integration (gRPC client + service auth) | Platform | [phase-15](../curriculum-studio-platform/phase-15-primer-integration.md) | S12–S14, C5/C11, **I9** machine JWT, I12 applicable service integration | PG12 | `impl/S15-primer-integration` | G-studio-test gRPC; no LMS DB | import-push deferred |
| **S16** | Projects integrated | Platform | [phase-16](../curriculum-studio-platform/phase-16-projects-integrated.md) | S12–S13 | PG13 | `impl/S16-projects` | G-studio-test | — |
| **S17** | Collaborative authoring | Platform | [phase-17](../curriculum-studio-platform/phase-17-collaborative-authoring.md) | S10, S16 | PG14 | `impl/S17-collab` | G-studio-e2e | — |
| **S18** | Deploy/ops; live Stytch/model/deploy BLOCKED | Platform | [phase-18](../curriculum-studio-platform/phase-18-deploy-ops-live-gates.md) | S15–S17 | PG15 | `impl/S18-ops-live` | packaging gates; live **BLOCKED** | needs user approval |
| **S19** | Streamable HTTP MCP `/mcp` protected resource (authz-filtered; human publish confirm) | Platform | [phase-19](../curriculum-studio-platform/phase-19-streamable-http-mcp.md); [MCP design](../curriculum-studio-mcp-design.md); [IB0 MCP contract](../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md) | S1+S2 + credential-free test Identity for transport qual; S3–S8 + D4–D6/D5/D11 for full tools; **I6+I8** for production delegated writes; **I7+applicable I12** for static client registration/publish confirmation; C12 for conformance claim | PG-MCP | `impl/S19-mcp-endpoint` | G-studio-test; `make studio-mcp-e2e`; RFC 9728/resource/aud/raw-Stytch/Origin/IDOR/idempotency/publish negatives; confirmation DB handle binds validated public `client_id`; missing/wrong claim, `azp`, internal UUID denied/audited | no DCR/token mint; service cannot confirm publish |

### 3.6 Cross-cutting integration gates (`X*`)

| Wave | Goal | Depends on | Acceptance |
| --- | --- | --- | --- |
| **X1** | First Studio cover ≥85% established | S1 (+ enough pkgs) | G-studio-cover |
| **X2** | Credential-free Studio↔loopback Identity joint auth | S2 + loopback/test Identity (or narrow verifier) | joint e2e; same middleware; not production auth |
| **X3** | Production-auth Studio (Primer JWT/JWKS + BFF against Identity) | I6, I7, I8, applicable I12, S2, S9 | G-studio-e2e + G-identity-e2e; required public `client_id`, absent `azp`, internal UUID denied |
| **X4** | Contract conformance + platform handlers aligned | C11, S8+, S15 | C11 matrix + platform E2E |
| **X5** | Fail-open secrets closed pre-S7 | I13 | SharedSecretGuard empty secret fails closed |
| **X6** | Local integration tip green (no remote) | all non-BLOCKED through S15 + I12 | local `impl/cs-local` build/test matrix |
| **X7** | MCP agent surface GA gate (transport + protected-resource OAuth + tools + conformance) | S19, C12, I6+I8; static client-registration/publish-confirm path needs I7+applicable I12 | official + external stateless clients; metadata→Identity code+PKCE; raw Stytch rejection; signed public `client_id` issuance/validation and MRTR binding; missing/wrong claim, `azp`, internal UUID denied/audited; human/service publish matrix; no third DB |

---

## 4. Dependency DAG (condensed)

```text
F0
├── PG1 parallel:
│   ├── I1 → I2 → I3 → I4 → I5 → I6 → I7 → I8 (webhook/revocation hard dependency)
│   │                                → I9 (service principals) → I10
│   │                                → I11
│   │                                → I12 (needs S2 + I6/I8)
│   │                      → I13 → I14
│   ├── D1 → D2 → D3 → D4 → D5 → (D6 ∥ D7) → D8 → D9 → D10
│   │                         ↘ D11 ─────────────↗     → D12
│   └── C1 → (C2 ∥ C3) → (C4 ∥ C6*) → C5 → C7 → C8 → C9 → C10 → C11
│
└── S1 (after F0; migrations from D1)
    → S2 (D3; credential-free test path independent; hard prod validator/cutover I6+I8+applicable I12)
    → S3 → (S4 ∥ S5) → S6 → S7 → S8
    → S9 (after S3; live BFF hard on I7+I8)
    → S10 (S8+S9)
    → S11 → S12 → S13
    → S14 (S8+S11+D11+C9)
    → S15 (S12–S14+C5/C11+I9 machine JWT+applicable I12)
    → S16 → S17 → S18
    → S19 MCP (credential-free after S1/S2; S3–S8/D4–D6 tools; I6+I8 delegated writes; I7+applicable I12 registration/publish-confirm; C12 conformance ∥)
       ↘ X7 (S19+C12)

C12 documents SoT anytime after C8; runtime conformance hard on S19.
```

`C6*` needs S1 openapi-gen host hooks (SOFT: contracts may ship emission against harness handlers first).

**MCP sequencing (stable; do not renumber F0/PG1/PG2 history):**

1. Transport qualification: after **S1/S2** with credential-free test Identity/narrow verifier — spike SDK `v1.7.0` Streamable HTTP stateless handler; this does not prove production auth.
2. Read tools: after **S4–S8** domain readiness and **D4–D6** persistence (workspaces/catalogs/plan/validate as needed).
3. Mutation tools: after **S6–S8**, **D5**, **C8** (idempotency/error semantics).
4. Production delegated writes: after **I6/IB2 + I8/IB4**. Client registration/BFF mediation/publish confirmation additionally require **I7/IB3 + applicable I12/IB8** and the **S8** publish path.
5. Final conformance (**C12**/**X7**): before claiming MCP GA; coordinate relative to **S15** so Primer gRPC is not described as the only machine/agent path once MCP is in scope.

---

## 5. Parallel groups (dispatch units)

| Group | Waves | Notes |
| --- | --- | --- |
| **PG0** | F0 | Solo — root owner |
| **PG1** | I1, D1→D2, C1→C2→C3 | **First parallel dispatch after F0** |
| **PG2** | I2, D3, S1 | After PG1 foundations |
| **PG3** | I3, C4, S2(credential-free), D4 prep | IA-R residual remediation; no JWKS milestone |
| **PG4** | I4, C5, S3 | — |
| **PG5** | I5–I6, S4∥S5, C7 path, D5 | catalogs parallel |
| **PG6** | I7, S6, D6∥D7, C8 | BFF first; I8 must wait for I7 |
| **PG6b** | I8 | signed webhook/two-plane revocation after I7 |
| **PG7** | I9, S7–S8, D8, C9 | publish + service principals |
| **PG8** | I10–I11, S9–S10, D9–D11, C10 | MVP UI + persistence depth |
| **PG9** | I12, X3, D12, C11, S11 | Studio↔Identity joint |
| **PG10+** | S12–S18, I13–I14, X4–X6 | materialize → Primer → migrations |
| **PG-MCP** | S19, C12, X7 | Credential-free spike after S1/S2; full tools after S8/D5; delegated writes after I6+I8; registration/publish-confirm after I7+applicable I12; do not reorder PG0–PG2 history |

---

## 6. Traceability: detailed phases → master waves

### Platform → waves

| Platform phase | Wave(s) |
| --- | --- |
| 1 Service shell | S1 (+ F0) |
| 2 Authz boundary | S2, X2, X3 |
| 3 Workspaces | S3 |
| 4 Standards | S4 |
| 5 Resources | S5 |
| 6 Plan drafts | S6 |
| 7 Validation | S7 |
| 8 Publish | S8 |
| 9 SPA/BFF | S9 |
| 10 Planning MVP UI | S10 |
| 11 Materialization | S11 |
| 12 Workflow runner | S12 |
| 13 Exports/artifacts | S13 |
| 14 Outbox/webhooks | S14 |
| 15 Primer integration | S15 |
| 16 Projects | S16 |
| 17 Collab | S17 |
| 18 Deploy/live | S18 |
| 19 Streamable HTTP MCP | S19, X7 |

### Contracts → waves

| Contracts phase | Wave(s) |
| --- | --- |
| 1 Ownership/layout | C1 |
| 2 Enum parity | C2 |
| 3 Spikes | C3 |
| 4 Protobuf gen | C4 |
| 5 gRPC harness | C5 |
| 6 Huma emission | C6 |
| 7 OpenAPI handoff/clients | C7 |
| 8 Auth/errors/idem/page | C8 |
| 9 Events/webhooks | C9 |
| 10 Policy gates | C10 |
| 11 Conformance E2E | C11, X4 |
| 12 MCP protocol/tool schemas | C12, X7 |

### Database → waves

| DB phase | Wave(s) |
| --- | --- |
| 1 Migration freeze | D1 |
| 2 Persistence foundation | D2 |
| 3 Tenant/workspace authz | D3 |
| 4 Catalogs/resources | D4 |
| 5 Plan graph/publish | D5 |
| 6 Validation reports | D6 |
| 7 Learner/runs | D7 |
| 8 Workflow fencing | D8 |
| 9 Items lifecycle | D9 |
| 10 Export refs | D10 |
| 11 Outbox/webhooks/idem | D11 |
| 12 Audit/ops | D12, X1 |

### Identity → waves

| Identity phase | Wave(s) |
| --- | --- |
| 1 Shell/DB | I1 |
| 2 Accounts | I2 |
| 3 IA-R residual remediation and review | I3 |
| 4 IB0 broker/webhook/provisioning freeze | I4 |
| 5 IB1 composed Stytch broker exchange | I5 |
| 6 IB2 Primer ES256 JWT/JWKS bridge | I6, X3 |
| 7 IB3 BFF cookie/CSRF/PKCE contract | I7, X3 |
| 8 IB4 signed webhook/two-plane revocation | I8, X3 |
| 9 IB5 Primer-owned service principals | I9 |
| 10 IB6 provider-plus-Primer lifecycle | I10 |
| 11 IB7 key rotation and hardening | I11 |
| 12 IB8 Studio integration | I12, X3 |
| 13 LMS dual-login | I13, X5 |
| 14 TV/S7/ops/live | I14 |

**Orphan check:** every phase file in the four plans appears exactly once above.
**Missing check:** every master wave cites ≥1 detailed phase (F0/X* are meta and cite decisions).

---

## 7. Ownership matrix (implementation surfaces)

| Surface | Single owner wave track |
| --- | --- |
| Root go.work / Makefile / CI / compose | **F0** only |
| Studio `go.mod` module path | F0 + S1 (create); frozen path |
| Identity `go.mod` | F0 + I1 |
| Enum parity fixture / codegen | **C\*** |
| Goose migrations / repos / UoW | **D\*** |
| Huma handlers / domain services | **S\*** |
| BFF/UI SPA | **S9–S10, S17** |
| Outbox persistence schema | **D11** |
| Outbox worker process | **S14** |
| Artifact refs in DB | **D10** |
| Artifact bytes / object store | **S13** |
| gRPC server harness | **C5** |
| gRPC production wiring + Primer adapter | **S15** |
| Generated gRPC/REST clients | **C4, C7** |
| MCP tool schemas + MCP conformance harness | **C12** |
| MCP `/mcp` adapter + tool wiring | **S19** |
| Primer Identity token broker / JWKS / Stytch B2B broker / LMS cutover / MCP client+resource registration | **I*** |

---

## 8. Implementation-orchestrator prompt contract

Copy this when dispatching a wave agent:

```text
You are implementing Curriculum Studio delivery wave <WAVE_ID> only.

Workspace: <impl worktree path>
Branch: impl/<WAVE_ID>-<slug>
Base: local integration tip (impl/cs-local or planning tip) — do not push/PR/merge master.

Authority order:
1. agent_docs/plans/curriculum-studio-delivery/index.md + execution-index.md
2. Cited detailed plan phase file(s) for this wave
3. Foundation crosswalk L1–L6 (do not reopen)
4. Identity design for auth mechanics

Rules:
- Own only surfaces listed for this wave. If you need root Makefile/go.work/CI, STOP and request F0.
- Studio module: curriculum-studio / github.com/aleksclark/primer/curriculum-studio
- Identity module: primer-identity / github.com/aleksclark/primer/identity
- Studio is one modular monolith + own DB; Identity separate deployable + own DB.
- Studio never issues auth sessions/tokens; validate JWT/JWKS only.
- studio-cover ≥85% once applicable; never lower gates.
- X-Service-Token migration-only; Bearer JWT end-state.
- No Studio→LMS import push; Primer via generated gRPC + events.
- Plans local-only; no push/PR/master without explicit user authorization.
- Run listed acceptance commands; leave evidence in the PR description (local).
- Two-stage review required before integrate-to-local-tip.
- On RED or ownership conflict: STOP and report blocker.

Deliver: code + tests for this wave's BDD/E2E IDs only; update nothing outside ownership.
```

---

## 9. First parallel implementation dispatch (from planning tip)

**Step 0:** F0 (`impl/F0-root-modules`) — root owner only.

**Historical Step 1 (PG1) — dispatched after F0 green; do not redispatch:**

| Agent | Wave | Worktree branch |
| --- | --- | --- |
| Identity | I1 | `impl/I1-shell-db` |
| Database | D1 then D2 (same agent sequential) | `impl/D1-migration-freeze` → `impl/D2-persistence-foundation` |
| Contracts | C1 then C2 then C3 | `impl/C1-ownership` → `impl/C2-enum-parity` → `impl/C3-spikes` |

Do not start S2 production validator/cutover, I13, or S15 until their hard dependencies clear; S2 credential-free verifier work remains separate. S15 needs I9 machine JWT, not I8.

---

## 10. Rollback / stop gate summary

| Condition | Action |
| --- | --- |
| Two agents edit root Makefile/go.work | STOP; revert non-F0; F0 re-owns |
| studio-cover proposed <85% | REJECT change |
| Studio mints tokens / OP endpoints | REJECT; move to Identity |
| C3 spike STOP | Block C4+ and dependent S15 |
| Empty service secret still fail-open | Block I14/S7 cutover (X5) |
| Live Stytch claimed without approval | BLOCKED — loopback only |
| Push/PR without user auth | STOP remote actions |
| Contract dual SoT after handoff | STOP C7; keep baseline |

---

## 11. Validation checklist for this roadmap commit

- [x] Decision record R1–R12
- [x] Source-plan ownership table
- [x] Global gates
- [x] Wave table with stable IDs
- [x] Dependency DAG + parallel groups
- [x] Per-wave branch naming
- [x] Plan section/scenario references (via phase links)
- [x] Acceptance commands
- [x] Review/merge protocol + blocker semantics
- [x] Full phase→wave traceability (no orphans)
- [x] Orchestrator prompt contract
- [x] First dispatch set identified


## Stytch delivery supersession

**Human-facing Identity phase labels:** Phase 3 = **IA-R residual remediation and review**; Phase 6 = **IB2 Primer ES256 JWT/JWKS bridge**; Phase 8 = **IB4 signed webhook and two-plane revocation**; Phase 9 = **IB5 Primer-owned service principals**; Phase 10 = **IB6 provider-plus-Primer lifecycle**; Phase 11 = **IB7 key rotation and hardening**. Historical phase filenames remain for link stability only and are non-authoritative.

Historical F0/PG1/PG2 are complete history; do not redispatch I1/I2. The original IA foundation remains at `87d5c215134825edb410266a62c15534e1e9ecea`; IA-R is reviewed at code tip `8623ee639bd64d40f819d3079c52ed567b65b27b` with specification PASS: 0 Critical/Important; quality/security APPROVED: 0 Critical/Important. **IB0 independently passed design review at `4bfd6d5c03412d134a635c32279ef37b1c4e0d9e` with 0 Critical/Important/Minor. IB1 is complete and reviewed at code tip `59a3998208ba9ef87dfe0bf4a913eedab3753ef8`: full specification PASS 0 Critical/Important (nonblocking minors), full quality/security APPROVED 0 Critical/Important (one nonblocking minor), IB1-E01..E10 credential-free/`httptest`/real-PostgreSQL/process evidence green, official adapter qualified, Identity coverage 84.1% (review reruns 84.3%/84.1%), and OpenAPI/client parity plus root/foundation gates green clean-env.** The post-merge cursor is **IB2/I6 only after reviewed IB1 reaches `master`**. The reviewed ordering remains **IA-R** remediation/review → **IB0** broker/webhook/provisioning contract freeze → **IB1** composed Stytch callback/code issuance → **IB2** public code consumption and Primer ES256/JWKS → **IB3** BFF cookies/CSRF/PKCE → **IB4** signed webhook/cache+grant revocation → **IB5** local service principals → **IB6** lifecycle → **IB7** hardening → **IB8** Studio/LMS/TV/live cutover; IB1 makes no token/JWT/JWKS implementation or live-provider claim, and live Stytch credentials/browser proof remains **BLOCKED**.

S2 production validator/cutover waits for IB2+IB4 and applicable IB8 integration, and rejects raw Stytch tokens/roles/tuples. S9 live BFF waits for IB3+IB4; S15 uses only IB5-issued local service-principal machine JWTs; S19/X7 accept only Primer JWTs and need IB2+IB4 for delegated human writes, plus IB3+applicable IB8 for registration/publish confirmation. X2 is credential-free downstream Primer-token evidence; X3 is IB2+IB3+IB4 plus applicable IB8/S2/S9; X7 is IB2+IB4 plus S19/C12. `G-identity-stytch-broker` covers tuple mapping, Primer-only bridge, provider-outage fail-closed/no-negative-cache, and signed webhook replay/forgery/dedupe/out-of-order proof.
