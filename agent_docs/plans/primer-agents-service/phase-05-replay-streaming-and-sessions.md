# Phase 5: Replay streaming and sessions

## Goal

Expose durable events as reconnectable Primer SSE and add durable, concurrency-safe multi-turn sessions for parent/admin chat. This phase preserves the proven stream/run lifetime split while replacing process-local replay with PostgreSQL sequence cursors. Afterward clients can disconnect, reconnect to any service instance, replay missed events, and continue a session without losing run identity or history.

## BDD Success Criteria

#### Scenario: SSE is incremental, attributed, and replayable

- **Given** a running parent/child run with durable events
- **When** an authenticated client subscribes to `/agents/v1/runs/{id}/events/stream`
- **Then** it receives Primer SSE frames incrementally with `id` equal to the durable event sequence
- **And** generated event data carries schema version, run/root/parent IDs, agent identity, depth, kind, and timestamp
- **And** parent/child deltas arrive before terminal completion

#### Scenario: Reconnect resumes without committed-event gaps

- **Given** a subscriber has received sequence N and disconnects while the run continues
- **When** it reconnects to another process with `Last-Event-ID: N` or the documented equivalent cursor
- **Then** the server replays every authorized committed event after N in order and continues with live events
- **And** duplicate delivery, if any around the replay/live handoff, is identifiable by sequence
- **And** terminal streams end only after the durable terminal event/status is observable

#### Scenario: Subscriber loss does not cancel or block execution

- **Given** a slow subscriber and a subscriber that disconnects after an early event
- **When** provider/child work continues
- **Then** neither subscriber controls the run-owned context or durable event writer
- **And** the run reaches its own terminal state unless budget/cancel stops it
- **And** a slow subscriber is disconnected or loses only ephemeral notification delivery and can recover from PostgreSQL replay

#### Scenario: Explicit cancellation remains distinct from disconnect

- **Given** one connected stream and a run blocked in nested MCP
- **When** an authorized generated client calls cancel
- **Then** nested work observes cancellation and the stream/replay shows cancel-requested and canceled evidence
- **And** merely closing that same SSE connection does not produce cancellation evidence

#### Scenario: Parent/admin session continues durably across process restart

- **Given** an authenticated parent/admin session with one completed turn
- **When** the service process restarts and the caller submits a second turn with the current session revision
- **Then** the same session ID and documented root-lineage policy are retained
- **And** the MAF/session state or canonical transcript required for the next turn comes from PostgreSQL
- **And** both turns and their events remain accessible in order

#### Scenario: Concurrent session turns are controlled

- **Given** two requests using the same session revision
- **When** they attempt to start turns concurrently
- **Then** at most one advances the session revision
- **And** the loser receives a conflict with the current revision rather than interleaving history
- **And** retry with a new idempotency key/current revision creates at most one next run

#### Scenario: Cross-namespace session/event access is denied

- **Given** another valid Identity principal/client namespace knows a session/run ID and event cursor
- **When** it pages, subscribes, or appends a turn
- **Then** no event/content/status is disclosed and no session revision changes

## Implementation Instructions

- Add a production SSE route documented in OpenAPI with `text/event-stream`, `Last-Event-ID` and/or query cursor semantics, heartbeat policy, terminal behavior, and error rules. Keep event schema versioned; generated clients own event types.
- Implement replay-first subscription: authorize run ownership, page PostgreSQL after cursor, establish a race-safe live notification point, catch up again, then stream. PostgreSQL remains source of truth; in-process pub/sub or `LISTEN/NOTIFY` is only a wake-up optimization.
- Bound per-subscriber buffers, concurrent streams, heartbeat frequency, replay page size, write timeout, and maximum connection lifetime. Slow/disconnected writers exit promptly without calling run cancel. Subscriber overflow may discard wake-ups, never durable events.
- Preserve SSE headers/flush behavior and IDs. Do not log data frames. Add metrics for active subscribers, replay count/lag, disconnect reason, and wake-up drops without content.
- Finalize durable session contract: authenticated owner namespace, execution profile, opaque caller context, optimistic revision, lifecycle, canonical turn history and/or versioned MAF session JSON. MAF preview serialization is internal and versioned; retain canonical Primer messages so a MAF upgrade does not make history unknowable.
- Freeze lineage policy: session ID remains stable; each turn is a new root run, and child lineage is rooted to that turn. Link turns by session/turn sequence rather than accidentally reusing a Runner's sticky root ID.
- Add create/get/list session and append-turn APIs with idempotency and expected revision. Parent/admin profile selection is server-owned from signed scope/client admission. Do not allow request instructions/tools/budgets to redefine it.
- Update generated OpenAPI/Go/TS clients. Place a reconnecting SSE helper in each generated-client package (or generate it) using generated event types; consumer apps may not hand-roll duplicate parsing/DTOs.

## End-to-End Test Plan

- Start two service processes against one PostgreSQL database and a barrier-controlled local provider. Subscribe through the generated streaming client, assert an early parent/child event arrives before provider release, disconnect after sequence N, reconnect through the second process, and assert exact sequence reconstruction through terminal.
- Use a deliberately slow client to overflow ephemeral notification capacity; assert the run succeeds, the slow connection is bounded, and a later replay retrieves committed events.
- Contrast disconnect with explicit cancel while MCP is blocked; inspect MCP context and durable event/status rows.
- Create a parent/admin session and first turn with the generated client, stop/restart the process, submit a second turn, and assert durable history, per-turn root lineage, session turn ordering, and provider-observed context.
- Race two second turns at one session revision; assert one winner and no interleaved transcript/events.
- Repeat page/stream/turn access with another principal namespace and assert no disclosure/mutation.
- Parse actual wire SSE bytes in at least one process E2E; generated helper tests supplement but do not replace wire evidence.

## Anti-Cheating Audit

- Verify SSE replay reads `run_events` from PostgreSQL; reject a process-local ring buffer presented as restart replay.
- Inspect replay/live handoff for a lost-event window and ensure sequence IDs make duplicates safe.
- Confirm stream handlers never pass request context to Runner/worker cancel paths and slow writers cannot block durable event append.
- Parse wire bytes and hold provider barriers; reject tests asserting only in-memory callbacks, event counts, or post-completion bodies.
- Restart an actual service process and ensure session context comes from persisted canonical history/state, not a test-global Runner.
- Review session revision/idempotency predicates and cross-namespace authorization on stream/page/turn routes.
- Ensure MAF Session JSON is not the only unversioned historical record and no prompt/event content enters logs/metrics.

## Completion Gate

- [ ] All BDD scenarios pass through real wire SSE, generated clients, two processes, and real PostgreSQL.
- [ ] Sequence replay/reconnect across instances has no unidentifiable gaps; terminal consistency holds.
- [ ] Disconnect, slow subscriber, and explicit cancel remain observably distinct and bounded.
- [ ] Parent/admin sessions survive restart with explicit per-turn root policy and concurrency control.
- [ ] Cross-namespace content access is denied at the server/repository boundary.
- [ ] OpenAPI/clients are current; race/coverage/process and existing repository gates pass unchanged.
