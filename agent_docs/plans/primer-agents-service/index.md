# Standalone `primer-agents` Service — Phased Implementation Plan

## Outcome

Deliver a deployable `primer-agents` service that is the shared agent-runtime boundary for Primer products. It owns a Go module and process, a PostgreSQL database, Identity-JWT authentication, durable run/session/event records, detached MAF execution, cancellation, replayable Primer SSE, generated Go and TypeScript clients, parent/admin chat, jobs, and a fail-closed student tutoring profile. LMS, Curriculum Studio, workstation, and future protocol adapters consume the HTTP contract through generated clients rather than treating `primer-server` as the runtime host.

The historical [Primer MAF Go Runtime wave-1 plan](../primer-maf-runtime/index.md) and its implementation in `server/internal/agent` remain the in-process foundation and migration source. They are not the service architecture. The feasibility evidence remains [`spikes/maf-go/RUN_REPORT.md`](../../../spikes/maf-go/RUN_REPORT.md), whose verdict is **CONDITIONAL GO**, not unconditional framework approval.

## Current-state summary

### Exists now

- `server/internal/agent` contains the pinned public-MAF adapter: run/child/root lineage, direct/depth/total child budgets, fail-closed tool filtering, a public-API custom `StreamingChildTool`, bounded SSE delivery, stream/run context separation, and a process-local `Controller`.
- `server/internal/api/agent_runtime.go` exposes parent-only start/status/cancel/SSE routes under the LMS API. `server/internal/api/agent_runtime_test.go` proves parent authentication and detached completion in one LMS process.
- `server/cmd/primer-server/main.go` constructs that controller only when `AGENT_RUNTIME_ENABLED=true`; the default is false. It requires an OpenAI-compatible base URL, API key, and model and logs the pinned MAF commit.
- `server/go.mod` pins `github.com/microsoft/agent-framework-go` to pseudo-version `v0.0.0-20260813082112-00ffc8c3648c`, upstream commit `00ffc8c3648c547997eae3a3f2a3b00c28daea09`.
- Primer Identity issues short-lived, single-audience ES256 access JWTs and publishes JWKS. Curriculum Studio already demonstrates a consumer-side fail-closed JWT/JWKS validator with exact issuer/audience checks and no Stytch dependency.
- Identity and Curriculum Studio demonstrate standalone module/process, namespaced configuration, isolated PostgreSQL, health/readiness, migrations, process E2E, structured logs, and module-specific build/test/coverage conventions.

### Partial

- The LMS controller detaches work from an HTTP request, but all run records, events, cancellation handles, and session continuity are process memory only.
- Primer SSE attributes parent and child work, but retained events are bounded process memory; there is no durable event cursor or reconnect replay across restart.
- MAF sessions can be represented in-process, but Primer has no shared durable session service or explicit cross-process multi-turn policy.
- Identity can issue and verify the required JWT profile, but each caller still needs an Identity client/grant configured for audience `primer-agents`; production caller/BFF rollout is an integration prerequisite, not permission to accept Stytch material.

### Missing

- No `primer-agents/` module, `cmd/primer-agents` process, namespaced `PRIMER_AGENTS_*` configuration, service health/readiness, image, deployment job, or service runbook.
- No agents-owned PostgreSQL database or migrations; no restart-safe run/session/event identity/status.
- No public service OpenAPI contract or generated Go/TypeScript clients.
- No Identity JWT validator for audience `primer-agents`; the LMS preview instead uses LMS-local parent sessions.
- No durable queue/lease worker, persisted cancellation request, restart reconciliation, event sequence/replay, scheduler, or student-specific public boundary.
- No opt-in remote integration in LMS admin, workstation, or Studio. Existing Fantasy tutor/TUI paths remain the active paths.

Repository evidence: `server/internal/agent/`, `server/internal/api/agent_runtime.go`, `server/internal/api/agent_runtime_test.go`, `server/cmd/primer-server/main.go`, `server/internal/config/config.go`, `server/go.mod`, `primer-identity/internal/token/`, `curriculum-studio/internal/authn/`, and the two historical documents linked above.

## Scope boundaries

### In scope

- New directory/module `primer-agents/`, frozen module path `github.com/aleksclark/primer/agents`, and binary `cmd/primer-agents`.
- `PRIMER_AGENTS_*` configuration only; suggested isolated database names `primer_agents` and `primer_agents_test`, with a dedicated goose version table.
- Service-owned PostgreSQL records for sessions, runs, events, cancellation, worker leases/recovery evidence, schedules, and schedule firings. References to caller resources are opaque text/UUID values with no cross-database foreign keys or reads.
- Exact Identity access-JWT validation for `aud=primer-agents`; local scope/profile checks; no raw Stytch tokens, SessionJWTs, organization tuples, or provider SDK.
- MAF Go as the engine inside the service, extracted from the LMS implementation while preserving the exact pin and all CONDITIONAL GO safeguards.
- Authenticated run/session/cancel/event/job/student APIs, Primer SSE replay, generated Go and TypeScript clients, and compatibility/cutover adapters.
- Parent/admin interactive sessions, on-demand and scheduled jobs, and a sandboxed student profile with server-owned `max_children=0`.
- Restart-safe run identity/status and event replay. A process interrupted during an uncheckpointed provider call is durably terminalized as `interrupted`; this plan does not pretend to resume model/tool side effects exactly once.
- Docker, Makefile, CI, migration, deployment, observability, recovery drills, and feature-flag rollout parallel to Identity/Studio conventions.

### Out of scope

- Ultracore or another full distributed control plane, workflow checkpoint engine, global queue, or exactly-once side-effect execution. That can replace the bounded Postgres worker model later without changing run IDs/contracts.
- Studio Streamable HTTP MCP phase S19, a Primer MCP server, or any claim that future MCP is implemented. This plan only leaves generated clients and an authenticated service contract suitable for a future adapter.
- Live Stytch integration or changes that make any component other than Primer Identity a Stytch client.
- Live billable LLM qualification. Every phase uses deterministic/scripted or explicitly local non-billable provider fixtures; live billable proof remains **BLOCKED** after this plan.
- Forking MAF, vendoring/patching it, or importing `github.com/microsoft/agent-framework-go/internal/...`.
- Reviving or copying the retired August 3 `primer-agent` prototypes.
- Immediate replacement of Fantasy-based TUI/LMS tutor flows. They remain available until each caller explicitly opts into the remote service and passes acceptance.
- Cross-service database access, shared migrations, cross-database foreign keys, or service-side reimplementation of LMS/Studio/workstation product authorization.

## Global constraints and decisions

1. **Service boundary:** production records and runtime ownership live in `primer-agents`, never the LMS database/process. No agents DSN may point at LMS, TV, Studio, or Identity databases.
2. **Authentication:** only Primer Identity ES256 access JWTs with exact issuer and `aud=primer-agents` are accepted. Production starts fail closed without JWKS/issuer configuration. Bearer material is never logged or persisted.
3. **Authorization split:** each caller remains authoritative for its own learner/workspace/job authorization before calling. `primer-agents` still enforces signed principal/client/scope, caller namespace, run ownership, and server-selected execution profile; it never trusts a request-supplied role, child budget, or policy name.
4. **Generated clients only:** OpenAPI emitted from handler signatures is the REST contract. Consumer code uses generated Go/TypeScript clients and generated event types; a streaming helper may wrap transport in the generated package but may not introduce handwritten duplicate DTOs or raw endpoint calls elsewhere.
5. **Durability honesty:** accepted run/session/event identity and status survive restart. In-flight uncheckpointed provider work is not resumed invisibly: lease expiry/restart reconciliation records `interrupted` and an event. Queued work may be reclaimed safely. Cancellation is a committed state transition, not request-context cancellation.
6. **MAF CONDITIONAL GO conditions remain hard gates:** custom public-API streaming child tool; Primer-owned child authority and finite budgets; student `max_children=0`; Primer-owned attributed event envelope; durable subscriber-independent stream/run split; exact MAF commit pin; no stock `agenttool` nested-stream claim; no claim that MAF workflows are a distributed control plane.
7. **Provider safety:** no default points at a billable endpoint. Production provider config is explicit and secret-redacted. CI and E2E use real MAF paths against local scripted/OpenAI-compatible fixtures, not fake successful handlers.
8. **Compatibility:** `AGENT_RUNTIME_ENABLED` and existing Fantasy paths are not removed before remote caller acceptance. New caller flags are disabled by default; there is no per-request silent fallback after a remote run is accepted.
9. **Testing:** existing root `make test`, `make cover` (85%), and build gates are not weakened. The new module receives its own at-least-85% coverage gate plus race, process, Postgres, contract-drift, and restart E2E gates.
10. **Sensitive content:** prompts, response text, tool arguments, JWTs, DSNs, and provider secrets are excluded from normal logs/metrics. Durable content/event retention is bounded and documented; access always rechecks the authenticated namespace.
11. **Historical boundaries:** `agent_docs/plans/primer-maf-runtime/` remains unchanged historical wave-1 authority. This directory is the authority for standalone-service implementation.

## Durable lifecycle contract

The implementation must freeze this minimum state machine before handlers are built:

- `queued -> running -> succeeded | failed | canceled | interrupted`
- `queued | running -> cancel_requested -> canceled`
- `cancel_requested` is non-terminal and durable; workers must observe it and cancel the runtime-owned context.
- A queued run may be safely reclaimed after a worker dies. A run whose provider invocation began and whose lease expires becomes terminal `interrupted` with recovery evidence; it is not automatically replayed because tool/provider side effects are not checkpointed.
- Every transition and runtime event receives a monotonically increasing per-run sequence in PostgreSQL. Terminal status and its terminal event commit consistently.
- Retrying a create call with the same caller-scoped idempotency key returns the same run. It never starts a duplicate run.

This is “durable enough” for cross-app identity, status, cancellation, and replay while keeping full distributed execution/checkpointing explicitly later.

## Phase overview

| Phase | Goal | Depends on |
|---|---|---|
| [Phase 1: Standalone foundation and engine extraction](./phase-01-standalone-foundation-and-engine.md) | Create the module/process and make `primer-agents` the owner of the proven pinned MAF engine without breaking legacy callers. | None |
| [Phase 2: Durable store and lifecycle](./phase-02-durable-store-and-lifecycle.md) | Add the isolated PostgreSQL schema/repositories and freeze restart-safe run/session/event/cancel semantics. | Phase 1 |
| [Phase 3: Identity-authenticated API and generated clients](./phase-03-identity-api-and-generated-clients.md) | Publish authenticated control-plane contracts and generated clients over the durable store. | Phase 2 |
| [Phase 4: Durable execution and cancellation](./phase-04-durable-execution-and-cancellation.md) | Execute queued runs through MAF workers with leases, persisted events, durable cancellation, budgets, and honest restart recovery. | Phases 1–3 |
| [Phase 5: Replay streaming and sessions](./phase-05-replay-streaming-and-sessions.md) | Provide reconnectable Primer SSE and durable multi-turn parent/admin sessions without coupling stream lifetime to run lifetime. | Phases 3–4 |
| [Phase 6: Jobs and sandboxed student policy](./phase-06-jobs-and-student-policy.md) | Add on-demand/scheduled jobs and a separate fail-closed student tutoring profile. | Phases 4–5 |
| [Phase 7: Caller integration and gradual cutover](./phase-07-caller-integration-and-cutover.md) | Integrate LMS admin and workstation through generated clients while preserving opt-in Fantasy/LMS rollback; leave Studio/MCP honest and later. | Phases 3, 5–6 |
| [Phase 8: Deployment, recovery, and release hardening](./phase-08-deployment-recovery-and-hardening.md) | Ship image/Make/CI/deployment/runbooks and prove process restart, isolation, redaction, and rollback. | Phases 1–7 |
| [Phase 9: Opt-in live billable LLM qualification](./phase-09-live-billable-llm.md) | Qualify one bounded real provider request through the real process and loopback Identity boundary without changing default safety gates. | Phase 8 |

## Capability traceability

| Requested capability | Primary phase(s) | Required observable evidence |
|---|---|---|
| Own Go module/process, namespaced config, health/ready, structured logs | 1, 8 | Real binary process test and deployment health checks |
| Own Postgres; no shared DB/cross-DB FK | 2, 8 | Migration/isolation tests against real PostgreSQL and foreign-DSN rejection |
| Durable runs/sessions/events, restart-safe identity/status | 2, 4, 5, 8 | Kill/restart E2E retains IDs/status/events; active work becomes honestly `interrupted` |
| Public API and generated clients for LMS/Studio-later/workstation/future MCP | 3, 7 | OpenAPI drift gate plus generated Go/TS client E2E; no handwritten consumer transport |
| Identity JWT `aud=primer-agents`; no raw Stytch | 3, 8 | Real signed JWT/JWKS positive and wrong-aud/raw-Stytch-shaped negatives |
| Caller-local product authz | 3, 6, 7 | Opaque refs only; signed namespace/scope enforcement; no caller DB reads |
| MAF engine moved/extracted from LMS and exact pin retained | 1, 7, 8 | Import/pin audit and no second authoritative implementation trapped in LMS |
| Streaming child, budgets, student `max_children=0` | 4, 6 | Real nested MAF/MCP stream test; student denial before tool/child invocation |
| Primer SSE, replay/subscribe, stream/run split | 5 | Wire-level SSE with sequence replay, disconnect continuation, explicit cancel distinction |
| Durable cancellation | 2, 4 | Committed `cancel_requested`, worker context cancellation, survives canceling HTTP request/restart |
| Parent/admin chat | 5, 7 | Multi-turn session over generated client with durable revision/history |
| Scheduled/on-demand jobs | 6 | Durable schedule firing/idempotency and on-demand run E2E |
| Sandboxed student tutoring | 6, 7 | Dedicated authenticated profile/route; no escalation from request fields |
| Gradual cutover preserving Fantasy and LMS paths | 1, 7, 8 | Disabled-by-default flags, dual-path acceptance, rollback drill |
| Docker/Makefile/CI/deploy parallel to Identity/Studio | 8 | Image start, migration, service health, root/module targets and CI gates |
| CONDITIONAL GO and pinned MAF | 1, 4, 8 | Automated pin/public-import/stock-collection audit and compatibility suite |
| Live billable proof is opt-in and bounded | 9 | Explicit flag/env file, fixed cheap model, real process + loopback Identity JWT, one request, redacted SHA/model/count evidence |
| No Studio S19/live Stytch/August prototypes | 7, 8 | Scope/import/history audit; no `/mcp`, Stytch SDK, or prototype copy |
| Existing `make test`/`make cover` gates not lowered | Every phase, 8 | Existing gates plus agents-specific coverage/race/E2E pass unchanged |

## External prerequisites and blockers

- Caller deployments need an Identity OAuth client/grant capable of obtaining an access JWT whose single audience is `primer-agents` and whose scopes match the caller profile. Credential-free test Identity/JWKS is sufficient for implementation and E2E. Production live-Stytch/BFF qualification remains outside this plan and must not be implied by passing local JWT tests.
- Phase 9 is the sole live billable LLM qualification surface. It is never part of default CI or ordinary service operation; its approved provider key, explicit endpoint, and env file are external prerequisites and must not be committed.
- The workstation student path needs a reviewed Identity-issued human/service credential strategy. Until that credential exists, the student remote feature flag stays off; the service still implements and proves the fail-closed student API with Identity-signed test credentials.
- Studio runtime integration is intentionally later. Publishing generated clients does not implement Studio phase S19 or any MCP transport.

## Completion rule

The plan is complete only when every phase BDD scenario passes through its stated public boundary; a real `primer-agents` process with real PostgreSQL survives restart without losing accepted run/session/event identity or status; cancel and SSE replay are proven independently of request lifetime; parent, job, and student profiles pass their authorization/policy negatives; generated clients are current and used by opted-in callers; the exact MAF pin/public-import/streaming-child conditions pass; deployment and rollback drills are recorded; existing root `make test`, `make cover`, and build gates plus agents-specific ≥85% coverage/race/E2E gates pass unchanged; and reviewers find no in-memory substitute, hard-coded success, raw Stytch acceptance, hidden cross-DB coupling, Studio MCP claim, unbounded or ambient live billable claim, or resurrected prototype code.
