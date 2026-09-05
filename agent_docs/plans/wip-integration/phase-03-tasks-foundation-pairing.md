# Phase 03: Integrate Tasks foundation and pairing

## Goal

Land the donor Phase 1 snapshot (`62795d53`) as the first reviewable Primer
Tasks implementation: standalone service/database, tenant-scoped parent and
student pairing, generated clients, web shell, Android pairing baseline, and
default host Make/non-Docker development proof. An opt-in Stacklane-compatible
Compose vector is additive only and must still be tested because P1 introduces
it. This establishes the service boundary needed by every later Tasks phase.

## BDD Success Criteria

### Scenario: A parent pairs web and Android clients to one student

- **Given** two real tenants in the standalone Tasks stack and an authenticated
  parent in tenant A
- **When** the parent creates a student and issues a one-use pairing QR through
  the public API/UI, then fresh web and Android clients consume it
- **Then** each client is bound only to that student and tenant
- **And** the token is stored by the platform-appropriate secure boundary.

### Scenario: Pairing replay and cross-tenant access fail closed

- **Given** a consumed, expired, revoked, malformed, or tenant-B pairing payload
- **When** a client presents it to tenant A through the public endpoint
- **Then** the server rejects it without issuing credentials or leaking student
  metadata
- **And** the denial is diagnosable without logging bearer/QR secrets.

### Scenario: Generated contracts are the client boundary

- **Given** a clean checkout without generated Tasks clients
- **When** the official Tasks generation targets run
- **Then** web/Kotlin clients compile against the emitted Huma contract
- **And** no hand-written duplicate DTO transport or tracked generated output is
  introduced.

### Scenario: The local stack is isolated and reloadable

- **Given** two independent worktree stacks on the default host Make/non-Docker path
- **When** each starts independently and one source file is changed
- **Then** health/endpoints resolve to the correct stack and only that stack
  reloads
- **And** stop/cleanup does not affect the other stack.
- **Given** P1 also introduces the additive opt-in Compose/Stacklane vector
- **When** each worktree starts that vector through `primer-tasks/scripts/dev`
- **Then** Compose isolation/hot-reload proofs pass without making Compose the
  default host command.

## Implementation Instructions

- Start from the merged Phase 02 master. Materialize the non-plan tree delta
  from donor plan baseline `ec36e559` to P1 endpoint `62795d53`; do not merge the
  old 38-behind branch or replay its intermediate final-plan edits.
- For additive `primer-tasks/` and reviewed evidence paths, reproduce the P1
  endpoint tree. Scan evidence/logs for credentials and machine-specific unsafe
  content before committing.
- Reconcile root `Makefile`, `go.work`, and `go.work.sum` semantically. Retain all
  current Studio/Identity/Agents/server modules and targets, then add only P1
  Tasks wiring. Never copy the donor root files wholesale.
- Adapt dependency pins and generated sums only when current master requires it;
  record every path that differs from the donor endpoint and why.
- Preserve the standalone DB and identity boundary. Tasks must not read LMS,
  Studio, Identity, or TV databases.
- Use the existing Tasks plan Phase 1 as behavioral authority. Open one P1 PR;
  do not include P2 schema or product behavior.

## End-to-End Test Plan

- Start real services through the default host Make/non-Docker path; use real
  PostgreSQL/identity fixture and production router wiring.
- Run `make tasks-test tasks-cover tasks-clients tasks-web tasks-android` and
  `make tasks-e2e` from the repository root after root targets are integrated.
  Those targets remain the default and must not require Compose.
- Because P1 introduces the opt-in Compose vector, also run
  `primer-tasks/scripts/dev check` and `primer-tasks/scripts/dev up` as an
  additive proof.
- Run the promoted Playwright flow against the live stack for parent login,
  student creation, QR issuance, pairing, replay denial, revocation, and
  cross-tenant denial. A reviewer performs separate managed-headless exploratory
  browser verification before accepting promotion evidence.
- Run `cd primer-tasks/android && ./gradlew connectedDebugAndroidTest` when an
  emulator is available. If unavailable, report this gate blocked; JVM tests and
  screenshots do not replace connected evidence.
- Run the two-worktree reload/isolation proof from the donor plan.
- Compare the integrated P1 `primer-tasks/` tree with `62795d53`, allowing only
  documented master-adaptation differences.

## Anti-Cheating Audit

- Inspect pairing handlers/repositories for hard-coded success, in-memory token
  state, client-only tenant checks, reusable QR values, or plaintext token logs.
- Verify Playwright drives the live API/database and does not intercept pairing
  routes or seed final UI state.
- Check generated-client imports and bans for hand-written HTTP/DTO mirrors.
- Check Android secure storage and network policy; do not infer secure device
  behavior from a JVM fake.
- Inspect root reconciliation for lost Studio/Identity/Agents targets or modules.
- Reject archived screenshots as a substitute for this PR's exact-head run.

## Completion Gate

- [ ] P1 BDD scenarios pass for two tenants and fresh/replayed clients.
- [ ] Tasks Go tests, race/vet/build as applicable, and 85% coverage pass.
- [ ] Deterministic client generation, web lint/typecheck/build, Android unit/APK
      build, and generated-output policy pass.
- [ ] Managed-headless live-stack browser exploration and promoted Playwright pass.
- [ ] Connected emulator and two-worktree proofs pass or are honestly BLOCKED.
- [ ] Root modules/targets retain all newer master functionality.
- [ ] Donor endpoint deviations are documented and independently reviewed.
- [ ] PR checks are green and the exact reviewed P1 head is merged.
