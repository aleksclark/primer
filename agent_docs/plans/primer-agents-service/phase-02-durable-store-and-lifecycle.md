# Phase 2: Durable store and lifecycle

## Goal

Add the agents-owned PostgreSQL schema and repository contracts for sessions, runs, events, cancellation, worker leases, schedules, and idempotency, and freeze the lifecycle semantics before execution or public APIs depend on them. After this phase, accepted identities/statuses and ordered events are restart-safe, although no production worker executes MAF runs yet.

## BDD Success Criteria

#### Scenario: Run identity and status survive repository/process replacement

- **Given** a run and session created through the application service over real PostgreSQL
- **When** the first application/repository instance is discarded and a new instance connects to the same database
- **Then** the same server-generated IDs, owner namespace, profile, input metadata, lifecycle state, and timestamps are returned
- **And** no process-memory registry is required to find them

#### Scenario: Create retry is idempotent within the authenticated caller namespace

- **Given** one signed-principal namespace and an idempotency key
- **When** the same bounded create command is submitted concurrently or retried after response loss
- **Then** exactly one run row is created and every successful response returns that run ID
- **And** reusing the key with materially different input is rejected as a conflict
- **And** another caller namespace cannot collide with or discover that key

#### Scenario: Lifecycle transitions fail closed

- **Given** queued, running, terminal, and cancel-requested runs
- **When** valid and invalid transitions are attempted concurrently
- **Then** only the frozen state machine transitions commit
- **And** terminal state cannot be overwritten by a stale worker or repeated cancel
- **And** transition version/fencing evidence identifies the winner

#### Scenario: Cancellation survives the initiating request

- **Given** a queued or running run
- **When** cancellation is requested and the HTTP/application context ends immediately after commit
- **Then** `cancel_requested` remains visible to a newly connected repository instance
- **And** repeated cancellation is idempotent
- **And** the run is not reported `canceled` until a worker/reconciler durably acknowledges terminalization

#### Scenario: Events are durable and monotonically replayable

- **Given** concurrent event appends for one run
- **When** events are read after a repository restart using a sequence cursor
- **Then** each committed event has one strictly increasing per-run sequence
- **And** pagination after sequence N has no duplicates or gaps among committed rows
- **And** a terminal lifecycle transition and its terminal event cannot disagree

#### Scenario: Database isolation cannot be bypassed through a library call

- **Given** an LMS, TV, Studio, or Identity DSN passed directly to Connect, Migrate, or the test harness
- **When** the database operation is attempted
- **Then** it fails before applying agents migrations
- **And** agents tables/version metadata are absent from the foreign database

## Implementation Instructions

- Add embedded forward-only goose migrations under `primer-agents/internal/db/migrations` and a dedicated version table such as `agents_goose_db_version`. Use an agents-owned schema or clearly prefixed tables; never reuse LMS migrations.
- Minimum durable model:
  - `sessions`: ID, authenticated subject/client namespace, opaque caller context, execution profile, status, session revision, MAF/session state or versioned history reference, created/updated/expiry timestamps.
  - `runs`: ID, optional session ID, owner namespace, caller idempotency key/hash, profile, bounded input, state, state version, cancel-requested timestamp/reason class, attempt/lease/fencing fields, provider-started timestamp, safe result/error class, created/started/ended timestamps.
  - `run_events`: run ID, per-run sequence, event schema version, lineage IDs, agent identity/type/depth, kind, bounded payload, timestamp; unique `(run_id, sequence)`.
  - lease/attempt or equivalent fields/tables that distinguish queued reclaim from provider-started interruption.
  - schedule and schedule-firing tables reserved for Phase 6, including a uniqueness key for `(schedule_id, due_at)`.
- Do not store JWTs, provider credentials, raw Stytch material, or caller database foreign keys. Store caller-owned resource references as bounded opaque values. Enforce ownership columns and indexes on every lookup path.
- Implement repositories/application services over `pgx` with transactions, savepoint-aware integration tests, and explicit errors for not found, conflict, invalid transition, and unavailable. There is no in-memory production repository.
- Freeze the state machine from `index.md`. Use compare-and-swap version/fence predicates for worker transitions. A terminal event and terminal run update should commit in one transaction or through an invariant that cannot expose disagreement.
- Store create idempotency with request hashing scoped by validated subject + signed `client_id` + operation/profile. Never scope solely by a client-provided student/workspace reference.
- Bound input/event sizes, event counts/retention policy fields, and query page sizes. Add indexes for owner/run lookup, queue claim, cancellation observation, event replay, expired leases, schedules, and retention.
- Document migration/down policy, retention/content sensitivity, and exact restart semantics in module DB docs. Full execution recovery remains Phase 4; this phase must not mark a queued record successful.

## End-to-End Test Plan

- Use the module's real PostgreSQL testcontainer harness. Create a session/run/events, close the pool/application, open a fresh pool, and assert identity/status/sequence replay.
- Race concurrent same-key creates and state transitions against real constraints; assert one row/winner and deterministic conflict semantics.
- Commit cancellation, cancel the request context, reconstruct the service, and assert `cancel_requested` remains. Then use only the repository's explicit acknowledgment path to terminalize it.
- Append events concurrently, restart the repository, page with cursors, and assert exact ordered sequences and terminal consistency.
- Run migrations twice and inspect actual tables, constraints, FKs, delete actions, version table, and indexes. Assert all FKs remain inside the agents database/schema.
- Probe direct Connect/Migrate/test-harness calls with forbidden database names and assert no schema changes.
- Commands: `cd primer-agents && go test ./internal/db/... ./internal/repo/... ./internal/appservice/... -count=1`, relevant repository tests under `-race`, then `go test ./...`, module coverage, and existing repository gates.

## Anti-Cheating Audit

- Search production constructors for map/slice/in-memory repositories or fallback behavior when PostgreSQL is unavailable.
- Query PostgreSQL directly in tests; reject tests that only assert repository return values or mocked SQL calls.
- Inspect idempotency uniqueness/hash scope for cross-principal collisions and body-change acceptance.
- Review every state update for version/fence predicates; reject unconditional terminal overwrite or `cancel_requested` reported immediately as completed cancellation.
- Verify terminal event/status atomicity and per-run sequence allocation under concurrency; reject timestamp ordering as a substitute for sequence.
- Inspect migrations for cross-schema/foreign-DB references, shared goose table names, raw token/provider fields, unbounded text, missing indexes, or cascade deletion that destroys required audit evidence unexpectedly.
- Confirm queued/running records are not declared restart-resumable yet and no fake worker marks them complete.

## Completion Gate

- [ ] All BDD scenarios pass against real PostgreSQL, including restart/reconnect and concurrency cases.
- [ ] Schema, constraints, indexes, migration policy, and isolation tests are reviewed.
- [ ] Run/session/event/cancel state survives repository replacement with no memory dependency.
- [ ] Lifecycle, idempotency, sequence, terminal-event, and fencing invariants are enforced by transactional production paths.
- [ ] No foreign DSN, cross-database FK/read, raw credential field, or production in-memory store exists.
- [ ] Module tests/race/coverage and existing `make test`/`make cover` gates pass without threshold changes.
