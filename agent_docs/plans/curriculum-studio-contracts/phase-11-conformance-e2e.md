# Phase 11: Full contract conformance E2E matrix

## Goal

Close the plan with a single conformance matrix proving REST authoring and gRPC
integration contracts together: real generated clients, real loopback servers,
auth, errors, LRO materialization, events/webhooks, parity, and policy gates —
with requirement traceability evidence complete.

**Depends on:** Phase 10
**Duration guess:** 3–5 days

## BDD Success Criteria

#### Scenario: P11-S1 — Authoring REST conformance tour

- **Given** loopback Huma server + fixture JWT (human subject with workspace
  membership)
- **When** generated TS (and Go) clients execute a scripted tour: health →
  create workspace → create curriculum → create revision → write graph nodes →
  validate → publish (idempotent) → authoring materialize → lock item → export →
  webhook CRUD → list events
- **Then** each step asserts typed responses and persisted harness state
- **And** unauthorized second subject cannot mutate foreign workspace

#### Scenario: P11-S2 — Integration gRPC conformance tour

- **Given** same harness backend as REST (shared store) + service JWT with
  materialize scope
- **When** generated Go gRPC client: GetPublishedRevision → Materialize → poll
  GetMaterialization → GetMaterializationBundle → ListMaterializedItems →
  PullEvents → AcknowledgeEvent
- **Then** bundle Struct fields match fixture keys; event ack sticks
- **And** draft revision ids are not readable via integration API

#### Scenario: P11-S3 — Cross-surface enum and error coherence

- **Given** the same failure injected (locked item overwrite, immutable
  revision edit)
- **When** triggered via REST and via gRPC equivalent where applicable
- **Then** ErrorCode wire strings match
- **And** enum parity gate remains green on emitted OpenAPI + proto + DB

#### Scenario: P11-S4 — Conformance vs emitted contract

- **Given** emitted OpenAPI + buf image
- **When** runtime route/RPC inventory is compared to contract
- **Then** no unregistered runtime routes; no missing critical operations
- **And** optional tools (schemathesis/grpcurl scripts) run smoke subset

#### Scenario: P11-S5 — Traceability matrix completion

- **Given** index §8 requirement IDs
- **When** conformance runner prints coverage JSON
- **Then** every REQ-* maps to ≥1 passed scenario/E2E id
- **And** orphan tests without REQ ids fail the coverage check

#### Scenario: P11-S6 — Plan completion evidence bundle

- **Given** all phases complete
- **When** `make contracts-conformance` and `make contracts-ci` run on clean
  checkout
- **Then** evidence bundle path is written (logs + REPORT links)
- **And** residual blockers list only live Identity OP and real materialization
  agents (explicitly out of scope)

## Implementation Instructions

1. **Conformance harness package**
   `curriculum-studio/internal/conformance/`:
   - shared testcontainer-optional; default in-memory harness OK
   - boots Huma + gRPC on localhost ports
   - seeds identity fixture JWKS + memberships
   - runs tours as subtests with stable names matching E2E IDs

2. **Shared store** between REST and gRPC so publish then materialize crosses
   surfaces.

3. **Coverage emitter** writes
   `tools/contract-gates/evidence/conformance-coverage.json`.

4. **Matrix table in README** linking commands:

   ```bash
   make contracts-ci
   make contracts-conformance
   ```

5. **Do not** mark LMS production adapter done; provide a sample LMS-side test
   that only builds against `clients/go-grpc` as interface proof.

6. **Final docs:** update `curriculum-studio/contracts/README.md` current-state
   from “hand OpenAPI live” to “Huma live + baseline frozen” reflecting actual
   end state.

### Focused verification

- full conformance green thrice (flake hunt)
- clean checkout once more
- traceability validator script

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E11-01 | conformance harness | REST tour | all steps pass; tenancy denial works |
| E11-02 | same harness | gRPC tour | bundle+events pass |
| E11-03 | dual surface errors | locked/immutable | ErrorCode match |
| E11-04 | inventory diff | runtime vs contract | no missing critical ops |
| E11-05 | coverage JSON | REQ-* set | 100% mapped + no orphans |
| E11-06 | clean worktree | contracts-ci + conformance | green; evidence bundle |
| E11-07 | sample LMS import | compile against go-grpc client | success without server source import |

## Anti-Cheating Audit

- Tours that skip auth or use server internals
- Separate non-shared stores faking cross-surface consistency
- Coverage JSON hand-written without running tests
- Claiming Identity/live materialization complete
- Flaky sleeps without status poll correctness
- Conformance allowlisted for raw transport and then used as only client path

## Completion Gate

- [ ] P11-S1…P11-S6 pass
- [ ] E11-01…E11-07 evidence
- [ ] Traceability 100%
- [ ] Clean-checkout green
- [ ] Residual out-of-scope blockers explicitly listed
- [ ] Index completion rule satisfied
- [ ] contracts README reflects handoff reality

## Dependencies and rollback

- **Depends on:** Phase 10 (and transitively all prior)
- **Unblocks:** Studio UI plan, LMS integration adapter plan, platform binary plan
- **Rollback:** none for plan docs; implementation rollback per prior phases

## Residual blockers (expected open)

1. Live Primer Identity OP / real JWKS hosting and client_credentials issuance
2. Full materialization agent workflows and artifact object store production wiring
3. Studio UI BFF cookie implementation (Identity + platform)
4. LMS production cutover off static secrets (Identity S1–S7)
5. Deferred Studio→LMS import push adapter decision
