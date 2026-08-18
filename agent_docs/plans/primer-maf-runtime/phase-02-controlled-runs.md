# Phase 2: Controlled runs

## Goal

Turn the foundation adapter into a production-owned run controller. This phase addresses the spike's remaining lifecycle gaps before any HTTP stream is exposed: concurrent `PrepareRun`, explicit cancellation, run status/error ownership, sticky multi-turn root policy, and release of unused child reservations. After this phase, a caller can start, inspect, cancel, and await a run without tying runtime lifetime to an HTTP request.

## BDD Success Criteria

#### Scenario: Concurrent preparation creates isolated runs

- **Given** one configured runtime and many simultaneous start requests
- **When** callers prepare parent and child runs concurrently
- **Then** each run receives a unique run ID and coherent root/parent lineage
- **And** mutable budget, session, and event state is not shared across runs
- **And** no data race or duplicate active-run record is reported under `go test -race`

#### Scenario: Explicit cancellation reaches nested work

- **Given** a parent run blocked in a child MCP tool
- **When** an authorized caller cancels the run through the controller
- **Then** the parent, child, tool, and MCP contexts observe cancellation
- **And** the run reaches a terminal cancelled state with a stable cancellation error
- **And** active-child accounting returns to zero

#### Scenario: Subscriber loss does not cancel the runtime

- **Given** a run has a subscriber that disconnects while provider work continues
- **When** the subscriber is removed
- **Then** the run continues to its own terminal result unless explicitly cancelled or budget-exhausted
- **And** the controller retains the terminal status/error for later inspection

#### Scenario: Multi-turn root lineage follows a documented policy

- **Given** a multi-turn session with a configured root policy
- **When** turns are started and child calls are made
- **Then** root attribution either remains stable for the session or a new root is deliberately issued according to the documented policy
- **And** parent/child IDs remain coherent on every turn
- **And** tests assert the chosen policy rather than relying on MAF's Runner lifetime accident

#### Scenario: Unused child reservation is reclaimed

- **Given** a child is prepared/reserved but never invoked, or invocation fails before execution
- **When** the child handle is closed/dropped
- **Then** direct and total budget counters and `activeChildren` are released exactly once
- **And** a later authorized child can use the reclaimed capacity

## Implementation Instructions

- Add a concurrency-safe controller in `server/internal/agent` with an opaque run handle/status contract: start/prepare, cancel, subscribe/unsubscribe, await/result, and terminal state. The controller owns runtime contexts; handlers must not own the only cancellation source.
- Define authorization-neutral controller operations first; Phase 4 supplies request identity and route authorization. Cancellation must be idempotent and safe after terminal completion.
- Separate immutable run configuration from per-run mutable state. Protect registry and counters with synchronization; ensure every reservation has one release path (success, error, cancellation, explicit close, and never-invoked handle).
- Make root lineage explicit in a session/run policy. If sessions are not persisted in wave 1, state that policy is process-local and non-durable; do not imply restart continuity.
- Apply finite run/child/depth/total limits before executing work. Return structured policy/budget errors that are safe to expose to HTTP callers without leaking provider secrets.
- Add bounded lifecycle logging/metrics hooks for start, terminal state, cancel request, budget denial, dropped event, and active-run count. Hooks must not block or change correctness.
- Add focused tests for concurrent prepare and repeated cancel/close, then run `cd server && go test ./internal/agent/... -race -count=1` before broader gates.

Do not add a database durability layer, restart recovery, or distributed queue in this phase.

## End-to-End Test Plan

- Through the exported controller contract, start a run using a blocking deterministic MCP fixture, call controller cancel, and assert the fixture sees `ctx.Done()` and terminal status is cancelled.
- Start a run, subscribe, receive an early event, unsubscribe, release the provider barrier, and assert the controller reaches success and retains the result while no subscriber remains.
- Launch many concurrent `PrepareRun` operations against one runtime, await all terminal results, and run under `-race`; assert unique IDs, isolated lineage, and no leaked active children.
- Prepare and close a child without invoking it, then start a replacement child using the same finite budget and assert admission succeeds.
- Exercise two turns through the public controller/session contract and assert the selected root policy. No test may call a private lineage helper as its primary evidence.
- Exact commands: `cd server && go test ./internal/agent/... -race -count=20`, `go test ./... -count=1`, `go vet ./...`, `go build ./...`.

## Anti-Cheating Audit

- Inspect cancellation call sites to ensure they cancel the controller-owned context, not merely stop an SSE writer or mark a boolean.
- Verify the blocking MCP fixture is reached and observes cancellation; a terminal status alone is insufficient.
- Search for request-context propagation into the runtime as the sole run context, goroutines without bounded ownership, and swallowed `context.Canceled` errors.
- Inspect all reservation increments/decrements and repeated close paths for leaks/double release; test counters through observable admission behavior.
- Confirm the root policy is represented in stored/in-memory run metadata and asserted in emitted events, rather than inferred from MAF internals.
- Confirm there is no fake in-memory “durability” claim, restart recovery branch, or test-only bypass for concurrency limits.

## Completion Gate

- [ ] All BDD scenarios pass under repeated race runs.
- [ ] Explicit cancel reaches nested MCP and terminal state is retained.
- [ ] Disconnect/subscriber loss leaves a run alive.
- [ ] Multi-turn root policy and process-local durability boundary are documented and tested.
- [ ] No active-child or controller goroutine leaks remain.
- [ ] Full server build/test gates pass without lowering coverage.
