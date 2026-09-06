# Phase 04: Integrate tasks, schedules, and manual approval

## Goal

Land donor Phase 2 snapshot (`8274d217`) on the reviewed P1 service. Parents can
create immutable task revisions and schedules; the service durably materializes
student occurrences; web/Android clients show checklist state; and only the
configured parent approval completes the manual-verification example.

## BDD Success Criteria

### Scenario: Scheduled task occurrences are durable and timezone-correct

- **Given** a versioned task and RRULE schedule in a named timezone
- **When** the scheduler crosses normal, DST gap, and DST fold boundaries and is
  restarted/replayed
- **Then** each logical occurrence is materialized exactly once with stable
  schedule/revision identity
- **And** parent and student public views show the same intended local time.

### Scenario: Parent approval controls manual completion

- **Given** a student starts and checks the brushing-task occurrence
- **When** the student submits completion through web or Android
- **Then** the occurrence waits for parent approval rather than becoming
  `completed`
- **And** one accepted parent decision completes it exactly once.

### Scenario: Unauthorized or stale decisions are rejected

- **Given** tenant B, a stale occurrence version, duplicate decision, or a model-
  disabled deployment
- **When** a decision/mutation is attempted through the public API
- **Then** tenant and compare-and-set boundaries reject invalid effects
- **And** the ordinary manual path remains fully usable without a model provider.

## Implementation Instructions

- Start from merged P1. Apply the non-plan donor delta `62795d53..8274d217` and
  preserve the P2 endpoint behavior; exclude `agent_docs/plans/primer-tasks*`
  because Phase 02 already landed the final plan authority.
- Apply additive migrations in donor order. Never squash away migration history
  or mutate P1 migration bytes.
- Reconcile root targets only if P2 changed them, retaining current root owners.
- Review Android deep-link and checklist changes as the completed native P2
  baseline. Do not import later native dialogue/media work as a completion claim.
- Verify scheduler uniqueness/concurrency in PostgreSQL, not an in-memory clock
  map. Make wall-clock/timezone inputs controllable without weakening production
  code.
- Keep model-disabled operation first-class and tenant authorization server-side.
- Open a dedicated P2 PR after P1 is on master.

## End-to-End Test Plan

- Run `make tasks-test tasks-cover tasks-clients tasks-web tasks-android` and
  `make tasks-e2e` against the real standalone stack.
- Run `cd primer-tasks/android && ./gradlew connectedDebugAndroidTest` for Today,
  Upcoming, detail/start, deep-link, manual submission, and revoked/replacement
  behavior when an emulator is available.
- Through managed headless browser, create a brushing task/revision/schedule,
  observe the student occurrence, submit it, approve as parent, and assert the
  durable final state through a fresh browser session.
- Exercise DST gap/fold schedules and restart the scheduler/process; query the
  public list and database evidence for no duplicates.
- Run real-Postgres concurrent decision/schedule tests under `-race` where the
  donor plan specifies it, plus cross-tenant/stale/idempotency negatives.
- Compare integrated `primer-tasks/` against P2 endpoint `8274d217` with an
  explicit adaptation allowlist.

## Anti-Cheating Audit

- Check that occurrence generation is persisted and uniquely constrained rather
  than held in process memory.
- Check that task revisions used by existing schedules are immutable and that
  clients cannot directly set `completed`.
- Check Playwright/Android flows do not seed completed occurrences or call
  private repositories.
- Check DST tests use real timezone rules, not fixed-offset assertions only.
- Check parent approval authorization and tenant filters in handlers/domain/SQL,
  not only hidden buttons.
- Check no model fixture is required for manual completion.

## Completion Gate

- [ ] All P2 BDD scenarios, including DST, replay, restart, duplicate, stale,
      and two-tenant negatives pass.
- [ ] The brushing example completes through parent web and student web; emulator
      evidence passes or is explicitly BLOCKED.
- [ ] Real PostgreSQL concurrency and scheduler uniqueness tests pass.
- [ ] Tasks 85% coverage plus Go/client/web/Android/build/diff gates pass.
- [ ] Managed-headless exploratory and promoted browser tests pass at exact head.
- [ ] Donor comparison and anti-cheating audit have no unexplained deviation.
- [ ] The P2 PR is green, independently reviewed, and merged sequentially.
