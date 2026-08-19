# Phase 3 runtime contract

This package owns the durable boundary around Fantasy `v0.41.1`. `Runtime`
creates a bounded Fantasy agent and translates callbacks to the safe protocol:
text deltas are retained, reasoning is represented only by generic thinking
start/end events, and tool callbacks become allowlisted labels and phases.
Tool arguments, tool results, reasoning deltas, provider metadata, prompts, and
credentials are never put in protocol events. `Execution` exposes final text
and usage only; it does not expose Fantasy's raw result.

`PostgresRepository` is the persistence boundary for conversations, messages,
runs, confirmation previews, and replayable events. `EventLog` persists before
publishing to the bounded ephemeral fan-out hub. The hub is allowed to vanish
on restart; replay comes from `agent_run_events` using a durable sequence/cursor.
A slow subscriber is closed rather than blocking the worker or growing a queue.

`internal/jobs.Worker` claims PostgreSQL rows with `FOR UPDATE SKIP LOCKED`,
keeps a lease independent of a browser/socket context, and requeues expired
leases on startup. Execution resumes only at a durable step boundary. A model
cannot resume in the middle of a token stream: the worker replays the last
committed event sequence and starts the next bounded step. Idempotent user
message keys and durable preview digests prevent duplicate submissions and
confirmation replay.

`schema.sql` is a schema contract, not a migration. Integration into the Tasks
migration chain, server wiring, authenticated WebSocket upgrade, and parent
commands are intentionally left to the Phase 3 orchestrator. No production
state is held in an in-memory map.

The Fantasy qualification test is a scripted provider fixture. It proves API
compatibility and safety behavior only; it makes no claim about live-model
quality.
