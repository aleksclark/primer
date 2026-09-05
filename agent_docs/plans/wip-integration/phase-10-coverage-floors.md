# Phase 10: Restore all coverage floors

## Goal

Raise current Studio 71.8%, Identity 79.8%, and Agents 84.9% measurements to
their unchanged 85%, 80%, and 85% floors with substantive behavioral tests.
The phase follows generation repair so measurements run from a reproducible
clean checkout. LMS/server and Tasks must retain their existing 85% floors.

## BDD Success Criteria

### Scenario: Every maintained module satisfies its real floor

- **Given** a fresh clone with generated prerequisites bootstrapped and real
  PostgreSQL available through testcontainers
- **When** the official coverage targets run
- **Then** LMS/server is at least 85%, Studio at least 85%, Identity at least
  80%, Agents at least 85%, and Tasks at least 85%
- **And** each target fails closed on missing packages, profile, parser, `bc`, or
  test dependency.

### Scenario: Coverage tests prove behavior rather than line execution

- **Given** previously uncovered public/domain failure, concurrency, retry,
  authorization, lifecycle, and observability paths
- **When** new tests exercise them
- **Then** assertions verify returned protocol outcomes plus durable state or
  emitted evidence where applicable
- **And** the same tests fail under a planted defect in the behavior they claim.

### Scenario: Stale branches are donors, not replacements

- **Given** `agents-phase1-foundation` contains alternate coverage tests
- **When** individual ideas are considered
- **Then** each is adapted to current merged production APIs and independently
  reviewed
- **And** no stale branch, old production file, or deleted current test suite is
  merged wholesale.

### Scenario: Real infrastructure failures remain visible

- **Given** PostgreSQL/notification/worker lease paths or process E2E behavior
- **When** a required dependency is unavailable or an error occurs
- **Then** the gate fails/blocks rather than substituting mocks, broad retries,
  sleeps, skipped tests, or an easier package set.

## Implementation Instructions

- Capture function/package coverage profiles on current code and prioritize
  important production seams, not generated/test helper files.
- Studio: add public-handler/domain/repository/process tests across currently
  merged S1–S12/S14/S19 and C1–C12 behavior. Use real PostgreSQL for workflow,
  materialization, outbox, and persistence; test MCP/REST/gRPC through their
  actual adapters. The 13-point gap requires systematic suites, not trivial
  getters.
- Identity: close the small gap with adversarial broker/OAuth/key/webhook/
  lifecycle behavior at the public service/repository boundary, retaining real
  PostgreSQL where durable state matters.
- Agents: exercise current SSE LISTEN/NOTIFY, worker heartbeat/cancel/fence,
  schedule GET, and replay/fallback behavior deterministically. Evaluate
  individual stale-branch tests, but retain current production fixes and suite.
- Ensure test synchronization uses channels/barriers/events rather than
  arbitrary sleeps. Keep race tests and error propagation meaningful.
- Update the stale Agents coverage runbook to current measured facts after green;
  remove obsolete claims of a 78% enforced target.
- Do not change `COVER_MIN`, `STUDIO_COVER_MIN`, `IDENTITY_COVER_MIN`,
  `AGENTS_COVER_MIN`, Tasks 85%, `coverpkg`, or package inclusion merely to
  improve the number unless a separately reviewed correctness bug in the gate
  proves the measurement invalid.

## End-to-End Test Plan

- In a clean generated checkout run, separately and then in the release matrix:
  `make cover`, `make studio-cover`, `make identity-cover`, `make agents-cover`,
  and `make tasks-cover`.
- Run corresponding full tests and races: `make test`, `make studio-test`,
  `make identity-test-oauth`, `make agents-test agents-race`, Tasks race suites,
  plus process/public-boundary E2Es associated with new coverage.
- For each new major suite, inject a temporary production defect in an isolated
  worktree (authorization check removed, fence ignored, event not persisted,
  etc.), prove the test fails, then revert and prove green.
- Repeat timing-sensitive SSE/worker/outbox tests enough to expose leaks/races;
  use `-race` and bounded timeouts.
- Record exact totals, command, SHA, environment prerequisites, and testcontainer
  use; do not round a below-floor total up.

## Anti-Cheating Audit

- Inspect for tests that assert only status, fixture JSON, collaborator calls, or
  coverage execution without state/protocol semantics.
- Check no production no-op branches, build tags, test-only environment flags,
  exported internals, or unreachable calls were added to inflate coverage.
- Check mocks/fakes do not replace required first-party APIs, PostgreSQL,
  LISTEN/NOTIFY, worker process, or durable stores.
- Check package sets, profiles, generated-code treatment, and thresholds against
  the pre-phase gate.
- Check sleeps/retries/skips for hidden flakes and swallowed errors.
- Check stale Agents code did not overwrite newer master behavior.

## Completion Gate

- [ ] LMS/server ≥85%, Studio ≥85%, Identity ≥80%, Agents ≥85%, Tasks ≥85% at
      exact head with unchanged floors.
- [ ] Full tests, races, and relevant process/public-boundary E2Es pass.
- [ ] New suites have planted-defect red proof for their substantive claims.
- [ ] No mocks/skips/test hooks/package-set changes hollow out measurements.
- [ ] Coverage profiles and current runbooks/docs agree.
- [ ] Independent review finds no stale-branch overwrite or gaming.
- [ ] Coverage PR(s) are green and merged before Phase 12.
