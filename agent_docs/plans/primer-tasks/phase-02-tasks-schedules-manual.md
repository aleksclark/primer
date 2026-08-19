# Phase 2: Tasks, schedules, checklist, and parent approval

## Goal

Make Primer Tasks useful without any model dependency. Parents create and revise
tasks, attach a versioned verification requirement, schedule work for a student,
and see materialized occurrences. Students see a real checklist in web and
Android. The `parent_approval` driver completes the physical-task example only
when a parent explicitly approves it.

This phase establishes the immutable revision, recurrence, occurrence, attempt,
and decision contracts that every later verification driver must use.

## BDD Success Criteria

#### Scenario: Parent creates and revises a task

- **Given** an authenticated tenant admin
- **When** the parent creates “Brush your teeth” with a `parent_approval`
  requirement and later edits its instructions
- **Then** each save produces a validated immutable task revision
- **And** lists show the task title as primary identity with revision/status as
  secondary metadata
- **And** retiring/deleting the task does not rewrite occurrences already issued.

#### Scenario: One-off schedule creates one occurrence

- **Given** a published task revision and student
- **When** the parent schedules it once with a local timezone and due time
- **Then** exactly one pending occurrence appears in parent and student views
- **And** rerunning the materializer or replaying the request creates no duplicate.

#### Scenario: Recurrence is timezone-correct and durable

- **Given** a supported daily/weekly RRULE across a daylight-saving transition
- **When** the bounded schedule horizon is materialized, the service restarts,
  and the horizon advances
- **Then** nominal local times remain correct, each instance is unique, and no
  already issued occurrence is silently moved
- **And** unsupported/unbounded recurrence input is rejected visibly.

#### Scenario: Parent approval checks off the task

- **Given** the student has opened the brushing occurrence
- **When** the parent approves it from the occurrence inspector
- **Then** the requirement receives one immutable accepted decision and the
  occurrence becomes completed
- **And** student web and Android update to a checked state through normal sync
- **And** repeated approval returns the same terminal result without duplicate
  decisions.

#### Scenario: Student cannot self-approve or forge completion

- **Given** a student browser/device credential
- **When** it posts parent-decision fields, calls parent routes, alters an
  occurrence state, or replays another student's ID
- **Then** the server denies the request and completion remains unchanged
- **And** the attempt is auditable without leaking the other occurrence.

#### Scenario: Parent rejects and later retries according to policy

- **Given** a pending parent-approval attempt
- **When** the parent rejects it with a short reason
- **Then** the student sees the rejected/try-again state but not internal IDs
- **And** a permitted retry creates a new numbered attempt while preserving the
  previous decision.

#### Scenario: Schedule edits affect only future work

- **Given** a recurring schedule with already materialized occurrences
- **When** the parent changes cadence, due offset, or task revision
- **Then** the server versions the schedule and applies the change only to future
  unmaterialized nominal times according to the documented boundary
- **And** canceled/skipped existing occurrences remain explicit audit events.

#### Scenario: Collections stay server-owned

- **Given** more tasks/occurrences than one page
- **When** the parent searches, sorts, filters, paginates, and restores a URL
- **Then** the server returns the bounded matching page and total/cursor
- **And** the browser does not bulk-fetch and transform the collection locally.

## Implementation Instructions

1. Add migrations/domain/repositories for `task_templates`, immutable
   `task_revisions`, `verification_requirements`, versioned `task_schedules`,
   `task_occurrences`, `verification_attempts`, `verification_submissions`, and
   immutable `verification_decisions`.
2. Build the verification registry foundation now:
   - manifests have stable kind/config version, interaction/executor
     capabilities, config/submission schemas, and retry/timeout policy;
   - validate config on task publish and snapshot it into occurrences;
   - the engine starts attempts and commits decisions with optimistic
     concurrency; drivers do not receive raw SQL or set completion;
   - phase 2 registers only `parent_approval`, but compatibility fixtures reserve
     the later capability vocabulary.
3. Define task CRUD semantics explicitly. Drafts may be hard-deleted if
   unreferenced; published tasks are retired. Updating a published task creates a
   revision. Student/archive references use human names in API projections.
4. Implement one-off and a bounded RFC 5545 subset (daily/weekly interval,
   weekdays, count/until) with explicit IANA timezone. Reject unsupported clauses
   rather than partially interpreting them. Materialize a configurable near-term
   horizon in a durable leased job and immediately after schedule writes.
5. Enforce uniqueness on `(tenant_id, schedule_id, nominal_at)`, snapshot the
   task revision/requirement set, and use compare-and-set legal state
   transitions. A materializer retry, concurrent worker, or restart must be safe.
6. Add parent REST operations for task/revision/schedule/occurrence CRUD/list,
   skip/cancel, approval/rejection, and retry. Add student-specific read/start
   routes inferred from the paired student. Never expose generic decision or
   occurrence-state CRUD to students.
7. Expand the parent SPA with **Explore/Configure** task and schedule surfaces,
   an **Operate/Inspect** occurrence queue, revision diff/retire warnings, and a
   parent approval action with confirmation. Keep q/filter/sort/page in URL and
   execute it server-side.
8. Expand student web and Android with today/upcoming sections, clear due state,
   an occurrence detail view, pending/rejected/completed states, and sync refresh.
   The list must remain usable with the model provider disabled.
9. Extend handler-derived contracts and generated clients for recurrence enums,
   discriminated requirement manifests, typed list pages/errors, and idempotency
   headers. Do not hand-copy DTOs into React or Kotlin.
10. Add scheduler metrics/logs for lease claims, generated counts, conflicts,
    skipped invalid schedules, and lag; omit task private content from operational
    logs.

## End-to-End Test Plan

### Browser exploratory acceptance

- Parent creates the brushing task, publishes it, assigns one-off and recurring
  schedules, edits a future schedule, and inspects issued occurrences.
- Student browser sees the correct task; parent approves; student list updates to
  checked. Repeat approval and retry browser refresh/reconnect.
- Exercise rejection/retry, task retirement, skip/cancel, server restart, DST
  boundary fixtures, and two-tenant IDOR attempts.
- Exercise task/occurrence list URL search/sort/filter/pagination on more than one
  server page and inspect network requests.

### Android emulator acceptance

- Use the phase-1 paired emulator to view today/upcoming, open an occurrence,
  survive process restart, and observe parent approval/rejection after refresh or
  push invalidation.
- Attempt direct completion/foreign occurrence calls with the device client and
  verify denial.

### Promoted automation

- After exploratory PASS, write Playwright specs for task publish/revision,
  one-off/recurring schedule, checklist, reject/retry, parent approve, list URL
  restoration, task retire, and two-tenant negatives.
- Add emulator-backed tests for list/detail state transitions and persistent
  student binding, against the real API.
- Add process/DB tests for concurrent schedule materializers, restart, DST,
  unsupported RRULEs, decision replay, and atomic occurrence completion.
- Run with `TASKS_MODEL_PROVIDER=disabled` to prove the manual flow has no hidden
  inference dependency.

Commands:

```bash
make tasks-test tasks-cover tasks-clients tasks-web tasks-android
make tasks-e2e
cd primer-tasks/android && ./gradlew connectedDebugAndroidTest
```

## Anti-Cheating Audit

- Inspect constraints and transaction boundaries: reject mutable published
  revisions, in-memory recurrence cursors, duplicate occurrences, or completion
  outside the verification engine.
- Confirm student routes infer student/tenant from credentials and cannot invoke
  parent approval or write state directly.
- Trace parent approval through attempt → decision → policy → occurrence in one
  durable domain path; reject handlers that merely return a checked DTO.
- Verify restart/concurrency tests use real PostgreSQL and the production worker,
  not a fake scheduler or direct private method sold as E2E.
- Confirm rejected decisions remain immutable and retries create new attempts.
- Inspect SPA list code/network traces for bulk-fetch/client filtering.
- Confirm Android/browser checked state comes from server state, not optimistic
  local completion that survives a rejected server write.
- Generated clients and OpenAPI must remain fresh/untracked and all new calls
  must use the owned client façades.

## Completion Gate

- [ ] All BDD scenarios pass, including DST, replay, restart, and two-tenant negatives.
- [ ] The brushing/parent-checkoff example works in parent web, student web, and emulator.
- [ ] Dedicated exploratory browser/Android agents PASS before test promotion.
- [ ] Promoted Playwright and emulator suites are green.
- [ ] Real-Postgres schedule/decision concurrency tests pass under race where applicable.
- [ ] Model-disabled flow passes.
- [ ] Generated clients, lint/typecheck/build, Go coverage/vet/test, Android build/tests, and diff checks pass.
- [ ] Anti-cheating audit finds no mutable revision, fake scheduler, client completion, or tenant leak.
