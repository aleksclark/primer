# Curriculum Studio delivery — execution index

Companion to [`index.md`](./index.md). Implementation orchestrators treat this
file as the wave cursor. Detailed BDD scenarios and E2E IDs live in the four
source plans; this file maps them 1:N to master waves.

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

Examples: `impl/F0-root-modules`, `impl/D1-migration-freeze`, `impl/I3-jwks`,
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
| G-identity-oauth | `make identity-test-oauth` | I4–I10 adversarial |
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
| **I1** | Identity service shell + isolated DB | Identity | [phase-01](../primer-identity-service/phase-01-service-shell-and-db.md) P1-S*, P1-E* | F0 | **PG1** | `impl/I1-shell-db` | G-identity-test; separate goose table | stop if LMS DSN accepted |
| **I2** | Accounts + external identities + password store | Identity | [phase-02](../primer-identity-service/phase-02-accounts-and-external-identities.md) | I1 | PG2 | `impl/I2-accounts` | G-identity-test | no email auto-link |
| **I3** | Keys, JWKS, access JWT mint/verify | Identity | [phase-03](../primer-identity-service/phase-03-keys-jwks-access-tokens.md) | I2 | PG3 | `impl/I3-jwks` | G-identity-test; JWKS public | **milestone:** Studio production-auth unblocks partially |
| **I4** | OAuth clients + OP code+PKCE | Identity | [phase-04](../primer-identity-service/phase-04-oauth-clients-and-op.md) | I3 | PG4 | `impl/I4-oauth-op` | G-identity-oauth | — |
| **I5** | Sessions, host-only cookies, login CSRF | Identity | [phase-05](../primer-identity-service/phase-05-sessions-cookies-login-csrf.md) | I4 | PG5 | `impl/I5-sessions` | G-identity-oauth; concurrent callback | — |
| **I6** | Google RP via loopback crypto | Identity | [phase-06](../primer-identity-service/phase-06-google-rp-loopback.md) | I5 | PG5b | `impl/I6-google-loopback` | G-identity-oauth; prod rejects test IdP | Live Google **BLOCKED** |
| **I7** | Product BFF confidential client contract | Identity | [phase-07](../primer-identity-service/phase-07-product-bff-contract.md) | I5, I6 | PG6 | `impl/I7-bff-contract` | G-identity-e2e | **milestone:** Studio BFF OIDC |
| **I8** | Service principals + client_credentials | Identity | [phase-08](../primer-identity-service/phase-08-service-principals.md) | I3, I4 | PG6 | `impl/I8-service-principals` | G-identity-test | **milestone:** machine JWT |
| **I9** | Refresh, revoke, logout fail-closed | Identity | [phase-09](../primer-identity-service/phase-09-refresh-revoke-logout.md) | I5, I7 | PG7 | `impl/I9-revoke` | G-identity-oauth | — |
| **I10** | Key rotation + hardened token path | Identity | [phase-10](../primer-identity-service/phase-10-key-rotation-and-hardening.md) | I3, I8, I9 | PG8 | `impl/I10-key-rotation` | G-identity-test | — |
| **I11** | Admin, audit, recovery, privacy | Identity | [phase-11](../primer-identity-service/phase-11-admin-audit-recovery.md) | I2, I9 | PG8 | `impl/I11-admin-audit` | G-identity-test | — |
| **I12** | Studio integration (JWKS consumer contract) | Identity (+ Platform consumer) | [phase-12](../primer-identity-service/phase-12-studio-integration.md); Platform S2 | I3, I7, I8, S2 | PG9 | `impl/I12-studio-integration` | G-identity-e2e + G-studio-test joint | **milestone:** S2 production-auth |
| **I13** | LMS dual-login + service dual-accept (S3–S5) | Identity | [phase-13](../primer-identity-service/phase-13-lms-dual-login-and-service-cutover.md) | I7–I9, I11 | PG10 | `impl/I13-lms-dual` | G-lms-test; empty secret fail-closed | close fail-open **before S7** |
| **I14** | TV admin SSO, S7, ops; live Google BLOCKED | Identity | [phase-14](../primer-identity-service/phase-14-tv-admin-s7-ops-live.md) | I13 | PG11 | `impl/I14-s7-ops` | G-lms-test; device tokens still local | Live Google **BLOCKED** without approval |

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
| **C12** | MCP protocol + tool-schema conformance (official + external client) | Contracts | [phase-12](../curriculum-studio-contracts/phase-12-mcp-protocol-tool-schemas.md); [MCP design](../curriculum-studio-mcp-design.md) | C8+; runtime hard on S19 transport | PG-MCP | `impl/C12-mcp-conformance` | official SDK + external client matrix; no OpenAPI/proto DTO mirror | **HARD** if custom transport invented |

### 3.5 Platform track (`S*`)

| Wave | Goal | Owner | Detailed refs | Depends on | PG | Branch | Acceptance | Stop |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **S1** | Studio service shell | Platform | [phase-01](../curriculum-studio-platform/phase-01-service-shell.md) | F0; consumes D1 migrations | PG2 | `impl/S1-service-shell` | G-studio-build; G-studio-test | separate DB only |
| **S2** | Authz boundary (validator-only) | Platform | [phase-02](../curriculum-studio-platform/phase-02-authz-boundary.md) | S1, D3; credential-free via test Identity **or** narrow verifier; **prod-auth** needs I3 | PG3 / soft on I3 | `impl/S2-authz` | G-studio-test; prod rejects test mode | never mint tokens |
| **S3** | Workspaces API | Platform | [phase-03](../curriculum-studio-platform/phase-03-workspaces-api.md) | S2, D3 | PG4c | `impl/S3-workspaces` | G-studio-test | — |
| **S4** | Standards catalog API | Platform | [phase-04](../curriculum-studio-platform/phase-04-standards-catalog.md) | S3, D4, C2 | PG5e | `impl/S4-standards` | G-studio-test | parallel S5 |
| **S5** | Resource catalog API | Platform | [phase-05](../curriculum-studio-platform/phase-05-resource-catalog.md) | S3, D4 | PG5e | `impl/S5-resources` | G-studio-test | parallel S4 |
| **S6** | Plan domain drafts | Platform | [phase-06](../curriculum-studio-platform/phase-06-plan-domain-drafts.md) | S4–S5, D5 | PG6d | `impl/S6-plan-drafts` | G-studio-test | — |
| **S7** | Validation engine | Platform | [phase-07](../curriculum-studio-platform/phase-07-validation-engine.md) | S6, D6 | PG7d | `impl/S7-validation` | G-studio-test | — |
| **S8** | Publish immutability + plan outbox enqueue | Platform | [phase-08](../curriculum-studio-platform/phase-08-publish-immutability.md) | S7, D5, D11 core | PG7d | `impl/S8-publish` | G-studio-test | — |
| **S9** | SPA/BFF shell (house system) | Platform | [phase-09](../curriculum-studio-platform/phase-09-spa-bff-shell.md) | S3; BFF OIDC prefers I7 | PG5e soft / PG9 hard for Identity login | `impl/S9-spa-bff` | G-studio-e2e shell | generated client only |
| **S10** | Planning MVP UI + MD/PDF export | Platform | [phase-10](../curriculum-studio-platform/phase-10-planning-mvp-ui.md) | S8–S9, S5 | PG8d | `impl/S10-planning-mvp` | G-studio-e2e | — |
| **S11** | Materialization domain | Platform | [phase-11](../curriculum-studio-platform/phase-11-materialization-domain.md) | S8, D7, D9 | PG9d | `impl/S11-materialization` | G-studio-test | — |
| **S12** | Agent workflow runner (scripted model) | Platform | [phase-12](../curriculum-studio-platform/phase-12-agent-workflow-runner.md) | S11, D8 | PG10d | `impl/S12-workflow` | G-studio-test resume/kill | live models BLOCKED |
| **S13** | Exports + artifact bytes store | Platform | [phase-13](../curriculum-studio-platform/phase-13-exports-artifacts.md) | S11–S12, D10 | PG10d | `impl/S13-exports` | G-studio-test; object store real | bytes not in PG |
| **S14** | Outbox worker + signed webhooks | Platform | [phase-14](../curriculum-studio-platform/phase-14-outbox-webhooks.md) | S8, S11, D11, C9 | PG11d | `impl/S14-outbox-webhooks` | G-studio-test delivery | — |
| **S15** | Primer integration (gRPC client + service auth) | Platform | [phase-15](../curriculum-studio-platform/phase-15-primer-integration.md) | S12–S14, C5/C11, I8, I12 | PG12 | `impl/S15-primer-integration` | G-studio-test gRPC; no LMS DB | import-push deferred |
| **S16** | Projects integrated | Platform | [phase-16](../curriculum-studio-platform/phase-16-projects-integrated.md) | S12–S13 | PG13 | `impl/S16-projects` | G-studio-test | — |
| **S17** | Collaborative authoring | Platform | [phase-17](../curriculum-studio-platform/phase-17-collaborative-authoring.md) | S10, S16 | PG14 | `impl/S17-collab` | G-studio-e2e | — |
| **S18** | Deploy/ops; live Google/model/deploy BLOCKED | Platform | [phase-18](../curriculum-studio-platform/phase-18-deploy-ops-live-gates.md) | S15–S17 | PG15 | `impl/S18-ops-live` | packaging gates; live **BLOCKED** | needs user approval |
| **S19** | Streamable HTTP MCP `/mcp` tools (authz-filtered; human publish confirm) | Platform | [phase-19](../curriculum-studio-platform/phase-19-streamable-http-mcp.md); [MCP design](../curriculum-studio-mcp-design.md) | S1+S2 for transport qual; S3–S8 + D4–D6/D5/D11 for full tools; I3 (and I7/I8/I12 for prod-auth + MCP client/resource); C12 for conformance claim | PG-MCP | `impl/S19-mcp-endpoint` | G-studio-test; `make studio-mcp-e2e`; Origin/aud/IDOR/idempotency/publish negatives | never mint tokens; never silent publish |

### 3.6 Cross-cutting integration gates (`X*`)

| Wave | Goal | Depends on | Acceptance |
| --- | --- | --- | --- |
| **X1** | First Studio cover ≥85% established | S1 (+ enough pkgs) | G-studio-cover |
| **X2** | Credential-free Studio↔loopback Identity joint auth | S2, I3–I6 (or narrow verifier interim) | joint e2e; same middleware |
| **X3** | Production-auth Studio (JWKS + BFF against Identity) | I7, I8, I12, S2, S9 | G-studio-e2e + G-identity-e2e |
| **X4** | Contract conformance + platform handlers aligned | C11, S8+, S15 | C11 matrix + platform E2E |
| **X5** | Fail-open secrets closed pre-S7 | I13 | SharedSecretGuard empty secret fails closed |
| **X6** | Local integration tip green (no remote) | all non-BLOCKED through S15 + I12 | local `impl/cs-local` build/test matrix |
| **X7** | MCP agent surface GA gate (transport + tools + conformance) | S19, C12, I3; publish-confirm path needs I7/I12 as applicable; prefer before over-claiming S15-only machine narrative | official + external clients; MCP-T1–T12 design matrix; no third DB |

---

## 4. Dependency DAG (condensed)

```text
F0
├── PG1 parallel:
│   ├── I1 → I2 → I3 → I4 → I5 → I6 → I7
│   │                      ↘ I8 (from I3/I4)
│   │                      → I9 → I10
│   │                      → I11
│   │                      → I12 (needs S2 + I3/I7/I8)
│   │                      → I13 → I14
│   ├── D1 → D2 → D3 → D4 → D5 → (D6 ∥ D7) → D8 → D9 → D10
│   │                         ↘ D11 ─────────────↗     → D12
│   └── C1 → (C2 ∥ C3) → (C4 ∥ C6*) → C5 → C7 → C8 → C9 → C10 → C11
│
└── S1 (after F0; migrations from D1)
    → S2 (D3; soft I3; hard prod-auth I3/I7/I8/I12)
    → S3 → (S4 ∥ S5) → S6 → S7 → S8
    → S9 (after S3; Identity login hard on I7)
    → S10 (S8+S9)
    → S11 → S12 → S13
    → S14 (S8+S11+D11+C9)
    → S15 (S12–S14+C5/C11+I8+I12)
    → S16 → S17 → S18
    → S19 MCP (after S1/S2+I3 transport; S3–S8/D4–D6 tools; I7/I12 publish-confirm; C12 conformance ∥)
       ↘ X7 (S19+C12)

C12 documents SoT anytime after C8; runtime conformance hard on S19.
```

`C6*` needs S1 openapi-gen host hooks (SOFT: contracts may ship emission against harness handlers first).

**MCP sequencing (stable; do not renumber F0/PG1/PG2 history):**

1. Transport qualification: after **S1** + **I3** (JWKS) — spike SDK `v1.7.0` Streamable HTTP stateless handler.
2. Read tools: after **S4–S8** domain readiness and **D4–D6** persistence (workspaces/catalogs/plan/validate as needed).
3. Mutation tools: after **S6–S8**, **D5**, **C8** (idempotency/error semantics).
4. Publish confirmation: after Identity/BFF milestones (**I7**, **I12**) + **S8** publish path.
5. Final conformance (**C12**/**X7**): before claiming MCP GA; coordinate relative to **S15** so Primer gRPC is not described as the only machine/agent path once MCP is in scope.

---

## 5. Parallel groups (dispatch units)

| Group | Waves | Notes |
| --- | --- | --- |
| **PG0** | F0 | Solo — root owner |
| **PG1** | I1, D1→D2, C1→C2→C3 | **First parallel dispatch after F0** |
| **PG2** | I2, D3, S1 | After PG1 foundations |
| **PG3** | I3, C4, S2(soft), D4 prep | JWKS milestone |
| **PG4** | I4, C5, S3 | — |
| **PG5** | I5–I6, S4∥S5, C7 path, D5 | catalogs parallel |
| **PG6** | I7∥I8, S6, D6∥D7, C8 | BFF + service principals |
| **PG7** | I9, S7–S8, D8, C9 | publish + revoke |
| **PG8** | I10–I11, S9–S10, D9–D11, C10 | MVP UI + persistence depth |
| **PG9** | I12, X3, D12, C11, S11 | Studio↔Identity joint |
| **PG10+** | S12–S18, I13–I14, X4–X6 | materialize → Primer → migrations |
| **PG-MCP** | S19, C12, X7 | After S1/I3 for spike; full tools after S8/D5; publish-confirm after I7/I12; do not reorder PG0–PG2 history |

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
| 3 Keys/JWKS | I3, X2 |
| 4 OAuth OP | I4 |
| 5 Sessions/CSRF | I5 |
| 6 Google loopback | I6 |
| 7 BFF contract | I7, X3 |
| 8 Service principals | I8 |
| 9 Refresh/revoke | I9 |
| 10 Key rotation | I10 |
| 11 Admin/audit | I11 |
| 12 Studio integration | I12, X3 |
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
| Identity OP / JWKS / Google / LMS cutover / MCP client+resource registration | **I*** |

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

**Step 1 (PG1) — dispatch immediately after F0 green:**

| Agent | Wave | Worktree branch |
| --- | --- | --- |
| Identity | I1 | `impl/I1-shell-db` |
| Database | D1 then D2 (same agent sequential) | `impl/D1-migration-freeze` → `impl/D2-persistence-foundation` |
| Contracts | C1 then C2 then C3 | `impl/C1-ownership` → `impl/C2-enum-parity` → `impl/C3-spikes` |

Do not start S2 production-auth, I13, or S15 until their hard dependencies clear.

---

## 10. Rollback / stop gate summary

| Condition | Action |
| --- | --- |
| Two agents edit root Makefile/go.work | STOP; revert non-F0; F0 re-owns |
| studio-cover proposed <85% | REJECT change |
| Studio mints tokens / OP endpoints | REJECT; move to Identity |
| C3 spike STOP | Block C4+ and dependent S15 |
| Empty service secret still fail-open | Block I14/S7 cutover (X5) |
| Live Google claimed without approval | BLOCKED — loopback only |
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
