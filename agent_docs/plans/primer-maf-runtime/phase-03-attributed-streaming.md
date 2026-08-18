# Phase 3: Attributed streaming

## Goal

Ship the required public-API streaming child tool and Primer-owned event transport on top of the controlled run lifecycle. A client can receive incremental parent and child events with root/parent/depth attribution while a slow or disconnected subscriber cannot block or cancel the underlying run. This phase is the direct production response to the spike's F3/F4/F5 conditions.

## BDD Success Criteria

#### Scenario: Nested child updates are live and attributed

- **Given** a parent MAF agent invokes an authorized child through the Primer streaming tool
- **When** the child emits incremental text and tool events
- **Then** the public event stream contains child start, incremental child events, and child end before the parent run completes
- **And** every child event carries `run_id`, `parent_run_id`, `root_run_id`, agent identity, and depth
- **And** the stream does not use stock `agenttool` collection as the nested UX path

#### Scenario: Parent updates arrive incrementally

- **Given** a provider is held behind a deterministic barrier after its first response delta
- **When** a stream subscriber reads from the Primer transport
- **Then** the first parent delta is observable before provider completion
- **And** event ordering is deterministic enough for clients to reconstruct each run's lifecycle

#### Scenario: Slow subscribers are bounded

- **Given** a subscriber consumes more slowly than the agent emits
- **When** the bounded bridge reaches capacity
- **Then** events are dropped or the subscriber is closed according to the documented policy
- **And** the runner continues without waiting forever or failing solely because of subscriber backpressure
- **And** a drop/close diagnostic is emitted without retaining unbounded event data

#### Scenario: Stream disconnect is distinct from run cancellation

- **Given** an HTTP client disconnects after receiving an early event
- **When** the SSE writer exits
- **Then** the controller-owned run continues and can reach a terminal result
- **And** an explicit controller cancel while the client remains connected reaches nested MCP work

#### Scenario: Bridge close is race-safe

- **Given** multiple producers emit while a stream is concurrently closed
- **When** the race test runs repeatedly
- **Then** there is no send-on-closed-channel panic, data race, or permanently blocked writer
- **And** queued residual events are bounded and safely discarded on stream cancellation

## Implementation Instructions

- Implement/export a Primer `StreamingChildTool` (or equivalent public contract) backed by a MAF public `tool.FuncTool`; iterate child `Run`/`ResponseUpdate` streams and publish child lifecycle/tool/text events. Do not import or fork stock `agenttool` internals.
- Extend the controller event model with explicit `root_run_id`, `parent_run_id`, `run_id`, agent type/name, depth, kind, text/tool metadata, and terminal error classification. Use stable JSON names and version the envelope if it will be public.
- Implement an asynchronous, bounded bridge with clear ownership of producer close and subscriber stream context. Emit must be non-blocking/bounded; close and cancellation must be race-safe. Decide and document whether overflow drops oldest/newest or closes a subscriber.
- Implement SSE framing/headers/flush behavior at the HTTP handler layer only after the controller contract is complete. Disconnect handling must unsubscribe the stream but must not call controller cancel.
- Add event/error observability while avoiding prompt, tool arguments, credentials, or student-sensitive text in logs by default.
- Focus verification on `cd server && go test ./internal/agent/... -race -count=20` and include a barrier test proving incremental wire arrival.

The phase may provide an internal HTTP handler for tests, but it must not yet be mounted on the production server; Phase 4 owns route/auth integration.

## End-to-End Test Plan

- Use the production controller plus deterministic provider/MCP fixtures and an `httptest` server around the production SSE handler. Read events from a real HTTP response and assert incremental parent/child attribution before releasing a provider barrier.
- Have the client close its response body after the first event, release provider work, and assert the controller terminal result is successful and the fixture did not see cancellation.
- Keep a client connected, call the exported controller cancel, and assert the blocking MCP fixture observes cancellation and the SSE terminal event/error is classified correctly.
- Run concurrent emit/close and slow-writer tests under `-race`, checking bounded memory/event count and no panic.
- Assert denied child authority produces an attributed policy error event and never invokes the denied tool.
- Exact commands: `cd server && go test ./internal/agent/... -race -count=20`, `go test ./... -count=1`.

## Anti-Cheating Audit

- Search the child tool implementation for `agenttool.New`, `.Collect()`, or a final-string-only adapter on the required nested stream path.
- Parse actual SSE bytes in tests; do not assert only in-memory sink events, HTTP status, or a fake callback.
- Confirm the barrier test proves event arrival before completion, and that child events originate from real child iteration rather than synthesized fixture output.
- Inspect bridge locking/close ownership and overflow behavior; reject recover-based panic suppression and unbounded channels/maps.
- Confirm request disconnect handlers unsubscribe only; explicit cancel tests must reach the blocking MCP server.
- Check wire IDs are generated/validated by the runtime and cannot be supplied by the student/client to broaden lineage or authority.

## Completion Gate

- [ ] Streaming child tool is public-API-only and live nested updates are proven.
- [ ] Primer SSE envelope carries root/parent/child/depth attribution.
- [ ] Async bounded bridge passes repeated race and slow/disconnect tests.
- [ ] Stream/run context split is proven at the HTTP boundary.
- [ ] No stock collection or fake event path satisfies the tests.
- [ ] Full server build/test/coverage gates remain green.
