# Phase 13: Certify exact master and hand off

## Goal

Certify the exact protected tip from a fresh clone, settle every PR/check and PR
#66 disposition, and commit a machine-readable handoff for the next roadmap
orchestrator. This phase makes no claim beyond evidence actually run at the
recorded SHA.

## BDD Success Criteria

### Scenario: Fresh clone passes the complete gate matrix

- **Given** a new clone checked out at the candidate `origin/master` SHA with no
  generated outputs or developer caches relied upon
- **When** official bootstrap, tests, coverage, clients/web, Tasks public E2E,
  Android repeats, drift, and Docker image commands run
- **Then** every required credential-free gate passes at its unchanged floor
- **And** git status remains clean after deterministic generation.

### Scenario: Tasks public boundaries work on integrated master

- **Given** real PostgreSQL, object storage, Tasks service/web, and separate
  external verifier fixture on the candidate SHA
- **When** parent/student actors use pairing, schedule/manual approval, parent
  chat confirmation, dialogue, media, and external verification through public
  browser/API/WebSocket boundaries
- **Then** durable tenant-scoped outcomes match P1–P6 contracts
- **And** negative/replay/restart/concurrency/SSRF paths remain fail-closed.

### Scenario: GitHub reports the same certified tip

- **Given** all phase PRs and any accepted PR #66 successor
- **When** `gh` queries merge commits and required checks
- **Then** each merge is reachable from the recorded `origin/master`, no required
  check is pending/cancelled/failing, and the certified SHA equals the handoff
- **And** no direct/force push was used to manufacture lineage.

### Scenario: Handoff is machine-readable and honest

- **Given** the final evidence
- **When** the next orchestrator reads
  `agent_docs/plans/wip-integration/handoff.json`
- **Then** it contains schema version, timestamp, exact master SHA, merged PRs,
  backup/archive refs, command results/coverage totals/image tags, remaining
  roadmap cursors/blockers, and unrun live/physical evidence
- **And** every referenced SHA/ref/path is resolvable.

### Scenario: A failed external prerequisite remains a blocker

- **Given** unavailable emulator, live provider, physical device, registry,
  production environment, or required CI
- **When** certification reaches that item
- **Then** it is recorded as blocked/not run with reason and last attempted
  command
- **And** no fixture, screenshot, old evidence, or local-only success is promoted
  to equivalent proof.

## Implementation Instructions

- Fetch without deleting preservation refs and create a brand-new clone or
  equivalent isolated clean worktree at the candidate master SHA. Record tool
  versions and required dependency availability without recording credentials.
- Run bootstrap/generation before Studio tests exactly as CI documents, then
  verify a second generation and clean tracked status.
- Run full module test/race/coverage matrices, contract/client drift, all web
  builds, Tasks E2E/external E2E, Android full/repeat gates, and production image
  builds. Use managed headless browser for the live Tasks web exploration.
- Verify public Tasks API/client boundaries and generated artifacts are current;
  scan for raw hand-written transports and tracked generated outputs.
- Query all PR merge/check states. If any required check is cancelled/unstable,
  rerun/fix it; do not call certification green.
- Reassess PR #66 final disposition. If merged, certify its exact descendant; if
  superseded/closed, record why and which commit/PR replaces it.
- Write `handoff.json` deterministically (stable keys/format) plus a short human
  summary in the final commit/PR. Validate JSON and all referenced SHAs/refs.
- Merge the handoff/certification PR only after its own checks. Fetch and record
  the final merge SHA; if this necessarily changes the SHA in the JSON, use the
  PR head/tree identifier plus post-merge `integrated_master_sha` update in a
  final tiny reviewed handoff PR, or another non-self-referential scheme that
  remains exact and verifiable.

## End-to-End Test Plan

At minimum, record exact exit status and totals for:

- `make foundation-check test cover`
- supported clean Studio bootstrap, `make studio-test studio-cover`, contract
  generation/conformance/MCP gates, and `make studio-web`
- `make identity-test identity-test-oauth identity-cover identity-openapi`
- `make agents-build agents-vet agents-test agents-race agents-cover
  agents-contracts-check agents-maf-audit`
- `make tasks-test tasks-cover tasks-clients tasks-web tasks-e2e
  tasks-external-e2e` plus required Tasks race suites
- `cd android && ./gradlew test assembleDebug --no-daemon` plus at least 20
  forced `TvViewModelTest` repetitions
- LMS/TV web client generation/build and generated OpenAPI drift checks
- `docker build` for `Dockerfile`, `Dockerfile.tv`, and `Dockerfile.ingest` with
  exact-SHA tags
- `git diff --check`, generated-output scans, link/status validation, and final
  `git status --porcelain`
- managed-headless real-stack browser flows for all integrated Tasks P1–P6
  public surfaces and negative boundaries.

Use real testcontainers/PostgreSQL, MinIO/S3-compatible storage, real local
processes, and separate verifier fixture. Scripted model/loopback identity
fixtures are allowed only for their named credential-free gates.

## Anti-Cheating Audit

- Confirm the clone starts without generated outputs and no prior build artifact
  is copied in.
- Confirm coverage floors/package sets and image Dockerfiles match reviewed
  production paths.
- Confirm browser tests use live network/processes with no first-party route
  interception, fixture HTML, or direct handler calls.
- Confirm persisted-state assertions accompany public status/UI assertions for
  durability and exactly-once claims.
- Confirm GitHub checks are for the exact integrated commit and not stale PR
  heads or cancelled runs.
- Validate handoff JSON values against Git/GitHub/command logs; reject hand-edited
  “pass” entries without evidence.
- Search for hidden skips, ignored failures, broad retries, secret output, or
  claims based on archived screenshots.

## Completion Gate

- [ ] Fresh-clone bootstrap/generation is deterministic and clean.
- [ ] Full tests/races/contracts/clients/web gates pass.
- [ ] Coverage totals meet unchanged LMS 85%, Studio 85%, Identity 80%, Agents
      85%, and Tasks 85% floors.
- [ ] Android focused repeats and full JVM/APK gates pass; connected/physical
      evidence is accurately marked.
- [ ] All three production images build at exact candidate SHA.
- [ ] Managed-headless Tasks P1–P6 public E2E and anti-cheating audit pass.
- [ ] Required CI/PR checks are green and all merge SHAs are reachable.
- [ ] PR #66 has an explicit green-merge or superseded/closed disposition.
- [ ] Working tree is clean and preservation/archive refs resolve.
- [ ] Validated `handoff.json` records exact tip, evidence, next cursors, and
      blockers without live/physical overclaim.
