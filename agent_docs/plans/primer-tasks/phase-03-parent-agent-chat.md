# Phase 3: Fantasy runtime and parent command chat

## Goal

Add a Primer-owned durable execution wrapper around Fantasy and expose the first
WebSocket agent experience: a parent command/inspect chat that can list, draft,
revise, retire, schedule, and inspect tasks through narrow tenant-scoped tools.
Text streams incrementally; safe thinking/tool progress is visible; socket loss
does not cancel the run; and destructive mutations require explicit preview and
confirmation.

This phase proves the runtime, event protocol, and authority model before any
student-facing agent can affect verification.

## BDD Success Criteria

#### Scenario: Parent creates and schedules through streamed chat

- **Given** an authenticated parent admin and existing student
- **When** the parent asks the agent to create a parent-approved task for Friday
- **Then** text appears incrementally over WebSocket with thinking/tool progress
- **And** the agent uses server tools to create a validated task revision and
  schedule visible in the ordinary UI
- **And** the final message names the human task/student, not only IDs.

#### Scenario: Ambiguous requests require clarification

- **Given** two students with similar names or an incomplete schedule request
- **When** the parent asks for an ambiguous mutation
- **Then** the agent asks a clarifying question and makes no mutation
- **And** no guessed student or timezone is selected silently.

#### Scenario: Destructive mutation requires confirmation

- **Given** an active recurring schedule
- **When** the parent asks the agent to retire or broadly change it
- **Then** the agent presents a server-generated preview and confirmation handle
- **And** no mutation occurs until the authenticated parent explicitly confirms
- **And** a stale, altered, foreign-tenant, or replayed handle is rejected.

#### Scenario: Tenant scope cannot be widened by prompts or tool arguments

- **Given** tenant A's parent and known tenant B IDs/names
- **When** the prompt or generated tool input asks to list/mutate tenant B
- **Then** server-derived scope restricts every tool to tenant A
- **And** no tenant B names, counts, records, or existence oracle are returned.

#### Scenario: Disconnect and reconnect preserve the run

- **Given** a run paused inside a scripted provider/tool barrier
- **When** the browser disconnects after receiving an early delta and reconnects
  with its durable cursor
- **Then** the run continues under a worker-owned context, reaches one terminal
  state, and the conversation replays durable progress/final messages
- **And** it does not duplicate mutations or lose a terminal error.

#### Scenario: Safe progress does not expose hidden reasoning

- **Given** a provider emits reasoning callbacks and tool input deltas
- **When** the parent watches the chat
- **Then** the UI shows generic “thinking” and safe mapped tool activity plus
  response text deltas
- **And** raw reasoning text, secret arguments, and provider metadata are absent
  from WebSocket payloads, persistence, logs, and browser state.

#### Scenario: Failure, retry, and cancellation are bounded

- **Given** provider timeout, malformed tool input, tool conflict, slow
  subscriber, or explicit parent cancel
- **When** the run executes
- **Then** finite retry/step/token/time limits apply, status becomes a safe
  terminal failure/cancel state, and no partial unconfirmed mutation is hidden
- **And** slow subscriber backpressure does not block the worker or grow without
  bound.

#### Scenario: Agent-disabled mode preserves manual UI

- **Given** Fantasy/provider configuration is disabled
- **When** the product starts and phase-2 flows run
- **Then** task/schedule/checklist/manual approval still work
- **And** chat shows an intentional unavailable state rather than fake success.

## Implementation Instructions

1. Pin `charm.land/fantasy v0.41.1` exactly in `primer-tasks/go.mod`. Add a
   dependency/import audit. Use public Fantasy APIs only: typed
   `NewAgentTool`, `Agent.Stream`, stop conditions, and callbacks. Do not import
   LMS MAF code or attempt a shared runtime abstraction in this phase.
2. Add a qualification suite around Fantasy's hardest required behavior:
   streamed text, reasoning start/end, tool input/call/result callbacks, typed
   tool schema, multi-step stop, context cancellation, provider error, and file
   support shape reserved for phase 5. Use a scripted `LanguageModel`; no live
   credentials are required for this gate.
3. Add durable tables/services for conversations, messages, runs, jobs,
   confirmation previews, and replayable run events. A DB-leased worker owns run
   contexts and resumes/reconciles abandoned jobs on restart. Document which
   states can resume from a durable step boundary rather than claiming mid-token
   model continuation.
4. Define Go WebSocket command/event tagged unions and an offline schema/type
   emitter. Include protocol version, connection hello, subscribe/unsubscribe,
   user message idempotency key, cancel, confirmation, event sequence/cursor,
   text start/delta/end, generic thinking start/end, safe tool progress, retry,
   terminal state, and typed error.
5. Implement one authenticated `/ws` upgrade path. Parent browser uses its BFF
   cookie plus CSRF/origin protection; Android support may be added later using
   bearer auth. Never accept tokens in query strings. Subscription authorization
   is checked both at subscribe and replay.
6. Separate socket and run lifetimes. Each subscriber has a bounded queue. If it
   falls behind, close it with a resumable cursor; do not cancel the durable run.
   Persist final messages, state transitions, confirmation records, and safe
   tool summaries. Live text deltas may be coalesced, but a reconnect must recover
   coherent final conversation state.
7. Build parent tools over phase-2 domain services, not repositories:
   `list_students`, `list/get_task`, `draft/update/publish/retire_task`,
   `list/create/update/disable_schedule`, `list_occurrences`, and
   preview/confirm actions. Tool context contains tenant/subject/idempotency and
   finite authority. Unknown/empty active tool lists fail closed.
8. Require preview-confirm for retire, bulk schedule changes, and other declared
   destructive actions. The preview record binds tenant, actor, normalized
   action digest, expiry, and single use; confirmation executes exactly that
   digest through the same domain service.
9. Compose prompts from server-owned policy and bounded tenant context. Treat
   parent text as untrusted content. Store prompt policy version and model/provider
   metadata for audit, but never provider secrets or raw internal prompts in
   operational logs.
10. Map Fantasy reasoning callbacks to status only. Do not forward
    `OnReasoningDelta` text. Map tool names to allowlisted human labels and redact
    raw inputs/results before a progress event. Final tool effects come from
    domain records.
11. Add provider configuration for Fantasy Bedrock primary and optional
    OpenRouter/OpenAI-compatible fallback, all server-side. Scripted and disabled
    modes are explicit; production refuses scripted mode. Set finite per-run
    steps, tokens, duration, retries, active tools, and concurrent tenant limits.
12. Add the parent chat as a System C **Command/Inspect** ruled transcript beside
    ordinary task/schedule views. No chat bubbles. Include reconnect, queued,
    thinking, tool progress, confirmation, canceled, failed, and provider-disabled
    states; preserve keyboard and mobile usability.
13. Generate WebSocket message types into the TypeScript client package. The web
    app imports one client façade for reconnect/cursor/ack/error behavior and may
    not instantiate raw sockets elsewhere.

## End-to-End Test Plan

### Browser exploratory acceptance

- Against the default host Make/non-Docker Tasks path, real Postgres, and scripted Fantasy provider, ask the parent
  agent to list students, create/publish/schedule a task, clarify ambiguity, and
  preview/confirm a destructive change. Verify effects in ordinary pages and DB
  through public reads.
- Disconnect network/socket mid-stream, release a provider barrier, reconnect,
  and verify terminal output and exactly-once mutation.
- Exercise cancel, provider timeout, invalid tool input, stale confirmation,
  cross-tenant prompt injection, slow subscriber, origin/CSRF failure, server
  restart between durable steps, and disabled mode.
- Inspect WebSocket frames, browser storage, logs, and DB for absent raw reasoning
  and secrets. Capture desktop/mobile/light/dark and axe evidence.

### Promoted automation

- After exploratory PASS, add Playwright WebSocket specs for streamed delta
  ordering, progress, tool effects, ambiguity, confirmation, reconnect/cursor,
  explicit cancel, provider failure, disabled mode, and cross-tenant isolation.
- Add process E2E with scripted Fantasy barriers proving socket disconnect does
  not cancel a run, restart reconciliation, bounded subscriber behavior,
  idempotent tools, and one terminal state.
- Add protocol generation/compatibility tests and a source scan banning raw
  `WebSocket` construction outside the owned client façade.
- Run repeated concurrency scenarios under `go test -race` with leak/counter
  assertions.

Commands:

```bash
make tasks-test tasks-cover tasks-agent-compat tasks-clients tasks-web
make tasks-e2e
cd primer-tasks && go test -race ./internal/agent/... ./internal/jobs/... ./internal/api/... -count=10
```

## Anti-Cheating Audit

- Trace chat mutation from actual Fantasy tool call to phase-2 domain service and
  committed row; reject scripted handler replies, fixture-only UI effects, or
  repository bypasses.
- Verify every tool ignores model-supplied tenant/role authority and applies
  server context; inspect two-tenant negative calls and error text for oracles.
- Inspect run ownership: reject request/socket contexts as the only run lifetime,
  in-memory-only run state, fake reconnect, or retries that duplicate tools.
- Verify confirmation handles are durable, digest-bound, expiring, actor/tenant
  scoped, and single-use; reject conversational “yes” without server binding.
- Search WebSocket frames/persistence/logs for reasoning delta text, raw prompts,
  credentials, and unredacted tool inputs.
- Verify finite stop conditions, subscriber queue bounds, worker leases, and
  shutdown. Reject unbounded goroutines/queues/retries.
- Ensure disabled/provider-failure states never claim mutations succeeded and
  manual phase-2 paths remain real.
- E2E must use the real Fantasy loop with a scripted model, not a fake chat
  endpoint; scripted inference is not a live-model quality claim.

## Completion Gate

- [ ] All BDD scenarios pass through the real WebSocket/Fantasy/domain path.
- [ ] Dedicated exploratory browser agent PASS precedes Playwright promotion.
- [ ] Promoted streaming/reconnect/confirmation/tenant Playwright suite is green.
- [ ] Fantasy compatibility/pin/import audit is green.
- [ ] Repeated race, restart, slow-subscriber, and leak tests are green.
- [ ] Raw reasoning is absent from wire, DB, logs, and UI evidence.
- [ ] System C/a11y/mobile evidence passes.
- [ ] Go/web/client generation/build/coverage/diff gates pass.
- [ ] Anti-cheating audit finds no fake loop, authority widening, in-memory durability, or unsafe reasoning exposure.
