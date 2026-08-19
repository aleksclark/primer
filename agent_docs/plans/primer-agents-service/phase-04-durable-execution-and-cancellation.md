# Phase 4: Durable execution and cancellation

## Goal

Connect the durable queue to the pinned MAF engine with bounded in-process workers, database leases/fencing, persisted runtime events, and cancellation that is independent of HTTP lifetime. This phase delivers actual cross-process execution while honestly terminalizing uncheckpointed work interrupted by process death instead of claiming full distributed resume.

## BDD Success Criteria

#### Scenario: Accepted run executes through real MAF and persists its result

- **Given** an authenticated queued run, configured server-owned profile, real PostgreSQL, and a local deterministic OpenAI-compatible provider
- **When** an enabled worker claims the run
- **Then** the run transitions through `running` to one terminal state using a lease/fence
- **And** real MAF provider iteration produces persisted attributed events and safe result/error status
- **And** a second worker cannot execute the same live lease concurrently

#### Scenario: Authorized child streams with bounded least authority

- **Given** a parent/admin profile allowing a finite child count/depth/total and a parent tool grant
- **When** MAF invokes a tailored child through Primer's custom streaming child tool
- **Then** child identity/instructions differ from the parent as configured
- **And** live child start/text/tool/end events persist with run/root/parent/depth attribution before parent completion
- **And** tool authority is the fail-closed intersection and denied tools are never invoked
- **And** stock `agenttool` collection is not the required path

#### Scenario: Persisted cancellation reaches nested work

- **Given** a running parent blocked in a child MCP tool and a committed `cancel_requested` transition
- **When** the worker observes cancellation after the canceling HTTP request has ended
- **Then** it cancels the runtime-owned context
- **And** parent, child, tool, and MCP observe cancellation
- **And** the run and terminal event commit as `canceled`
- **And** repeated cancel requests remain idempotent

#### Scenario: Cancellation survives service restart

- **Given** cancellation commits while no worker is running, or while the service is stopping
- **When** a new service process starts against the same database
- **Then** it reconciles the durable request and terminalizes or cancels claimed work according to state/lease ownership
- **And** the original run ID and cancellation history remain visible

#### Scenario: Process death is recovered honestly

- **Given** one queued run and one running run whose provider invocation has begun
- **When** the worker process is killed and leases expire
- **Then** a new worker may claim the queued run exactly once
- **And** the previously running uncheckpointed run becomes terminal `interrupted` with a recovery event and safe error class
- **And** it is not silently re-executed or reported succeeded
- **And** both run IDs and prior events survive

#### Scenario: Student budget invariant cannot be bypassed in the engine

- **Given** the student execution profile with `max_children=0` and no delegated tool grant
- **When** provider output attempts child delegation or a request supplies larger budget fields
- **Then** the engine denies before child/tool invocation
- **And** persisted policy evidence identifies the denial without leaking prompt/tool arguments

#### Scenario: Event persistence backpressure is bounded and honest

- **Given** event production exceeds the configured durable writer capacity or PostgreSQL becomes unavailable
- **When** the runtime cannot durably append within its budget
- **Then** it does not silently drop source-of-truth events and claim success
- **And** the run fails/interruption-classifies safely, releases leases/child slots, and records diagnostics when the database is available
- **And** memory/goroutine growth remains bounded

## Implementation Instructions

- Add a bounded worker loop inside `primer-agents` (API and workers may share the binary but not request contexts). Claim queued rows with transactional `FOR UPDATE SKIP LOCKED` or equivalent, lease owner/token/expiry, and monotonically changing fence/version. All updates require the current fence.
- Split `queued` claim from `provider_started`. Reclaim never-started queued/claimed work safely after lease expiry. Once provider/tool execution begins, process-loss recovery must terminalize as `interrupted`; do not auto-replay side effects.
- Instantiate MAF per run from a server-owned execution profile selected after authentication. Input may name a product mode but cannot provide agent depth, tools, child counts, system prompt, provider base URL/model, or credentials.
- Preserve the exact MAF pin and public-only adapter. Use the custom streaming child tool and fail-closed grants. Enforce finite run duration, child direct/depth/total budgets, active worker count, provider/tool timeout, and event payload bounds. Student remains hard-coded `max_children=0`; Phase 6 exposes its route.
- Replace process-local SSE bridge as source of truth with a durable event sink. Runtime events append to PostgreSQL with per-run sequence; a bounded internal writer may batch, but inability to persist must fail the run rather than silently drop lifecycle/content events. Subscriber fan-out is Phase 5 and may drop subscriber notifications because replay comes from PostgreSQL.
- Implement durable cancellation observation using bounded polling and/or PostgreSQL notification as an optimization. The committed DB state is authoritative. Cancel the run-owned context and heartbeat/lease loop; subscriber/request contexts never own execution.
- Reconcile at startup and periodically: queued/never-started expired leases become claimable; provider-started expired leases become `interrupted`; durable `cancel_requested` rows become canceled or are delivered to the current owner. Use fences so a late stale worker cannot overwrite recovery.
- Persist only safe result/error classes in run status. Event text required for client replay is sensitive content under retention/access controls; logs contain IDs, profile, state, durations, counters, and classes only.
- Provider tests use a real MAF OpenAI provider against local scripted HTTP plus official MCP transport. Production provider config is explicit; missing/unavailable provider makes execution readiness/profile unavailable, never a fake success. No live billable call is part of this phase.

## End-to-End Test Plan

- Start two real service processes/workers against one PostgreSQL database and one local OpenAI-compatible/MCP fixture. Create a run with the generated client; assert one provider invocation, attributed persisted events, and terminal success.
- Run a parent-child-tool flow and parse events from the event-page API to prove live ordering, lineage, authority intersection, and actual allowed/denied MCP effects.
- Block MCP, request cancel through generated HTTP client, close the cancel request, and assert MCP context cancellation plus durable terminal status. Repeat with cancellation committed while workers are stopped, then restart.
- Process-kill E2E: enqueue two runs, hold one after provider start, kill the worker process, start another process, advance lease time normally, and assert queued reclaim versus running `interrupted` semantics with preserved IDs/events.
- Run student-profile engine attempts with malicious budget/tool fields; assert no child/MCP invocation and durable policy evidence.
- Induce DB/event-writer outage or bounded saturation; assert no successful run with missing source events, bounded cleanup, and no goroutine/lease leaks.
- Run claim/cancel/reconcile tests under `-race` and with multiple workers. Commands include module E2E/race plus existing full gates.

## Anti-Cheating Audit

- Trace one E2E from durable queue claim through real MAF provider and MCP side effect to durable event/status; reject hard-coded worker results or direct scripted event insertion.
- Inspect leases and every worker update for fence predicates. Verify the concurrency test asserts provider invocation count, not only one terminal row.
- Confirm cancel reads committed DB state and reaches the runtime-owned context; a state-only update without nested MCP cancellation is insufficient.
- Kill an OS process, not merely cancel a goroutine, in restart tests. Query persisted records from the replacement process.
- Search for automatic retry of `provider_started` runs, MAF workflow “durability” claims, unbounded queues/goroutines, swallowed DB append errors, or success despite event loss.
- Search for `agenttool.New`, `.Collect()`, MAF internal imports, caller-controlled budget/tool/system fields, and student policy selected from a request role.
- Verify no production provider fake/test branch is enabled by environment in production and no billable endpoint/credential is used by default tests.

## Completion Gate

- [ ] All BDD scenarios pass through generated HTTP clients, real processes, PostgreSQL, real MAF adapters, and local provider/MCP fixtures.
- [ ] Multi-worker lease/fence proof shows one execution; process-kill recovery distinguishes queued reclaim from running interruption.
- [ ] Durable cancellation reaches nested MCP after request completion/restart.
- [ ] Custom child streaming, authority intersection, budgets, student zero-child invariant, and exact pin remain green.
- [ ] Event persistence cannot silently drop and still report success; resource bounds/race tests pass.
- [ ] Module coverage/race/E2E and existing root build/test/coverage gates pass unchanged.
