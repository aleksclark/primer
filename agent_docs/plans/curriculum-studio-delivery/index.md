# Curriculum Studio — Authoritative Delivery Roadmap

**Outcome:** A single cursor that implementation orchestrators must follow to deliver
Curriculum Studio + Primer Identity as two separate deployables, with no duplicated
ownership, no lowered gates, and realistic interleaving of platform / contracts /
database / identity work.

**Status:** Authoritative after integration synthesis
**Plan directory:** `agent_docs/plans/curriculum-studio-delivery/`
**Branch:** `planning/curriculum-studio-integration`
**Plans are local-only.** Implementation agents may create branches/commits and
integrate into a **local** implementation branch. They must **not** push, open PRs,
or merge `master` without explicit user authorization.

---

## 1. Current-state summary

| Area | State | Evidence |
| --- | --- | --- |
| Product boundary | Locked | [`../primer-curriculum-studio-product-plan.md`](../primer-curriculum-studio-product-plan.md) |
| Foundation crosswalk L1–L6 | Locked + auth table fixed | [`../curriculum-studio-foundation-crosswalk.md`](../curriculum-studio-foundation-crosswalk.md) |
| Identity design D1–D20 | Decision-complete | [`../primer-identity-service-design.md`](../primer-identity-service-design.md) |
| LikeC4 topology | Authored | [`../../../architecture/curriculum-studio/`](../../../architecture/curriculum-studio/) |
| Studio contracts (hand) | Present | `curriculum-studio/contracts/` |
| Studio DB migrations 00001–00004 | Present (design-time) | `curriculum-studio/db/` |
| Platform phased plan | Present (18 phases) | [`../curriculum-studio-platform/`](../curriculum-studio-platform/) |
| Contracts phased plan | Present (11 phases) | [`../curriculum-studio-contracts/`](../curriculum-studio-contracts/) |
| Database phased plan | Present (12 phases) | [`../curriculum-studio-database/`](../curriculum-studio-database/) |
| Identity phased plan | Present (14 phases) | [`../primer-identity-service/`](../primer-identity-service/) |
| Studio Go runtime / SPA | Missing | — |
| Identity Go runtime | Missing | — |

**This roadmap does not implement production code.** It assigns waves, ownership,
dependencies, acceptance commands, review protocol, and stop gates.

---

## 2. Scope boundaries

### In scope

- Master wave graph with stable IDs (`F0`, `S*`, `C*`, `D*`, `I*`, `X*`)
- Exclusive ownership so no two agents implement the same surface
- Integration order, parallel groups, worktree/branch naming
- Acceptance commands, rollback/stop gates, two-stage review
- Traceability from all four detailed-plan phases → master waves
- Implementation-orchestrator prompt contract

### Out of scope

- Production code, live deploy, live Google credentials
- Opening PRs / pushing / merging master without user authorization
- Re-opening crosswalk L1–L6 without a decision commit
- Studio→LMS import push (deferred)
- Student Identity accounts; TV device tokens (TV-owned)

---

## 3. Decision record (integration freeze)

| ID | Decision |
| --- | --- |
| R1 | **Studio module root/path frozen:** `curriculum-studio/` / `github.com/aleksclark/primer/curriculum-studio`. **Identity:** `primer-identity/` / `github.com/aleksclark/primer/identity`. |
| R2 | **Exactly one root owner:** delivery wave **F0** (platform agent) exclusively edits shared root `go.mod`/`go.work`/`Makefile`/CI/dev-compose. DB/contracts/identity agents request root changes via F0; they must not race root files. |
| R3 | Studio is **one deployable modular monolith** with its **own completely separate DB**. Identity is another deployable with its **own DB**. No cross-DB FKs/views/FDW. |
| R4 | Studio Go coverage gate is **≥85%** from the first `studio-cover` (repo `COVER_MIN := 85`). **Never lower.** Identity may start at ≥80% with mandatory adversarial OAuth suite, then raise toward 85% before production-ready — without lowering Studio. |
| R5 | Credential-free Studio auth consumes a **protocol-compatible loopback/test Identity** (preferred) or a narrow test-only verifier on the **same** principal/JWT middleware. Studio is **never** an auth/session issuer. Production fails closed on test providers / `AUTH_MODE=test`. |
| R6 | Ownership (no duplication): enum parity+codegen → **contracts**; repositories/migrations → **database**; Huma handlers + process/BFF/UI/workflow/artifacts/outbox worker → **platform**; gRPC server harness → **contracts** then production wiring → **platform**; Primer client → **platform** using generated gRPC client; Identity OP + LMS/TV migration → **identity**. |
| R7 | Interleave order: module foundation → migration freeze/persistence + contract spikes/parity → authz/workspaces → catalogs/plan/publish → Huma emission/generated clients/UI → materialization/workflows/items/artifacts/outbox → Primer integration. Only genuinely disjoint work runs in parallel. |
| R8 | Identity foundational waves may parallel Studio foundations. Studio **production-auth** phases block on Identity JWKS / BFF / service-principal milestones (I3, I7, I8, I12). Live Google remains **BLOCKED** until explicit credentials/approval. Loopback OIDC is credential-free evidence only. |
| R9 | `X-Service-Token` is **migration-only**; final state Bearer JWT. Existing fail-open empty-secret guards must be closed **before S7**. TV device tokens remain TV-owned. |
| R10 | Studio→LMS import push stays deferred. Primer integration primary path = **generated gRPC client** + webhooks/events. |
| R11 | Plans are local-only. Implementation may commit locally; **no push/PR/master merge** without explicit user authorization. |
| R12 | Foundation crosswalk auth table backticks fixed; detailed plans point here for orchestration. |

---

## 4. Source-plan ownership table

| Concern | Owner plan | Master wave prefix | Must not own |
| --- | --- | --- | --- |
| Root `go.work` / root Makefile / CI / dev-compose | Platform via **F0** | `F0` | DB, contracts, identity agents |
| Studio process shell, config, health | Platform | `S1` | — |
| Studio JWT validate + workspace authz + BFF cookie bridge | Platform | `S2` | Token mint / OP |
| Workspaces HTTP API | Platform | `S3` | Persistence schema design |
| Standards/resource HTTP | Platform | `S4`–`S5` | DB catalog SQL design |
| Plan/validate/publish HTTP + domain | Platform | `S6`–`S8` | Contract IDL authorship |
| SPA + BFF shell + planning MVP UI | Platform | `S9`–`S10` | Identity OP |
| Materialization/workflow/export/outbox/Primer adapter | Platform | `S11`–`S15` | Identity DB |
| Projects/collab/ops live | Platform | `S16`–`S18` | Live Google without approval |
| Enum parity, buf gen, spikes, gRPC harness, Huma emission, clients, policy gates | Contracts | `C1`–`C11` | Business repos, SPA product features |
| Goose freeze, pgx repos, UoW, leases, outbox tables, audit/ops DB | Database | `D1`–`D12` | Huma routes, Identity |
| Identity OP, JWKS, Google RP, BFF contract, service principals, LMS/TV migration S0–S7 | Identity | `I1`–`I14` | Studio product authz roles |

Detailed BDD/E2E remain in the four plan directories. This roadmap only sequences and assigns.

---

## 5. Global gates (every wave)

1. Docs-only until implementation waves start; then wave-local code only.
2. Real Postgres (testcontainers) for durability claims; no fake SQL stores.
3. Studio never issues access/refresh tokens or passwords.
4. No cross-DB access; separate goose tables (`studio_goose_db_version`, `identity_goose_db_version`).
5. `make studio-cover` ≥ **85%** once Studio packages exist; never lower the gate.
6. `X-Service-Token` only as migration alias; Bearer JWT end-state.
7. Production rejects test Identity / `STUDIO_AUTH_MODE=test` / empty service secrets (before S7).
8. Generated clients exclusive; no committed `gen/` trees.
9. Local git only unless user authorizes remote.
10. Two-stage review (spec then quality/security) before integrate-to-local-tip.
11. `git diff --check` clean; UTF-8; final newline; no trailing whitespace on touched files.
12. Stop on RED gates; do not skip planted-red proofs.

---

## 6. Document map

| File | Role |
| --- | --- |
| [index.md](./index.md) | This file — decisions, ownership, gates, protocol |
| [execution-index.md](./execution-index.md) | Full wave table, DAG, parallel groups, commands, traceability, prompt contract |

---

## 7. First dispatch set (immediate)

From the final planning tip, dispatch **in order**:

1. **`F0` alone first** (root owner) — create Studio (+ optional Identity) module skeletons, `go.work`, root Makefile/`dev-compose` stubs, CI placeholders. No business logic.
2. Then **parallel group PG1** (disjoint):
   - `I1` Identity service shell + DB
   - `D1`+`D2` migration freeze + persistence foundation
   - `C1`+`C2`+`C3` contracts ownership + enum parity + qualification spikes

Do **not** start `S2` production-auth promotion or LMS S3–S7 until Identity milestones in execution-index are green.

---

## 8. Completion rule

Delivery is complete only when:

1. Every non-BLOCKED master wave Completion Gate is green with real command evidence.
2. Traceability matrix in `execution-index.md` has no missing/orphan detailed phases.
3. Standalone Studio loop works without LMS; Primer path is gRPC+events only.
4. Identity S0–S6 credential-free proofs pass; S7 + live Google explicitly gated/BLOCKED as required.
5. No production claim rests solely on loopback IdP or test auth.
6. Local integration branch is coherent; remote actions still require user auth.

---

## 9. References

- [Product plan](../primer-curriculum-studio-product-plan.md)
- [Foundation crosswalk](../curriculum-studio-foundation-crosswalk.md)
- [Identity design](../primer-identity-service-design.md)
- [Platform plan](../curriculum-studio-platform/)
- [Contracts plan](../curriculum-studio-contracts/)
- [Database plan](../curriculum-studio-database/)
- [Identity plan](../primer-identity-service/)
- [LikeC4](../../../architecture/curriculum-studio/)
- [Execution index](./execution-index.md)
