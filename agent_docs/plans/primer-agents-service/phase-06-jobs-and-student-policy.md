# Phase 6: Jobs and sandboxed student policy

## Goal

Complete the service's multi-audience behavior by adding durable on-demand/scheduled jobs and a distinct student tutoring API whose policy is selected server-side and fails closed. Parent/admin chat from Phase 5, machine jobs, and student tutoring share lifecycle/streaming infrastructure but never share authority defaults.

## BDD Success Criteria

#### Scenario: On-demand job uses the durable run lifecycle

- **Given** an Identity-authenticated service/human principal admitted to the job scope
- **When** it starts an on-demand job with a caller-scoped idempotency key
- **Then** one durable job run is queued and executed through the same MAF worker/status/event/cancel contract
- **And** retry returns the same run
- **And** result/error/events are available through generated clients after request or service restart

#### Scenario: Scheduled firing is durable and idempotent

- **Given** an enabled schedule with bounded cadence/time zone/profile and next due time
- **When** two scheduler instances observe the same due time or a process restarts around enqueue
- **Then** exactly one schedule-firing row and one run exist for `(schedule_id, due_at)`
- **And** the next due time advances transactionally
- **And** missed-fire/catch-up behavior follows the documented bounded policy rather than producing an unbounded burst

#### Scenario: Schedule authorization and ownership remain caller-local and isolated

- **Given** two signed caller namespaces and opaque product context authorized by the first caller
- **When** either creates/lists/disables/fires schedules
- **Then** only the owner namespace can observe or mutate its schedules/firings/runs
- **And** `primer-agents` does not query the LMS/Studio database to reinterpret that context
- **And** a request-supplied owner, role, or client ID cannot cross namespaces

#### Scenario: Student tutoring always receives the sandbox profile

- **Given** a valid Identity JWT admitted to the student endpoint/scope and an opaque student context already authorized by the caller product
- **When** a tutoring turn is created
- **Then** the service chooses the student profile server-side with `max_children=0`, `max_depth=0`, no dynamic MCP/delegation grant, finite time/token/event budgets, and student-safe provider/model configuration
- **And** the client cannot request the parent/admin or job profile
- **And** the run/session remains isolated to the signed caller namespace

#### Scenario: Student escalation attempts fail closed

- **Given** missing/ambiguous student credential admission, unknown signed client, request fields such as `profile=admin`, positive child budgets, tools, system instructions, or a model override, or provider output attempting delegation
- **When** the student endpoint is called/run executes
- **Then** the request is denied or forbidden fields are rejected before execution according to the contract
- **And** no child agent, denied tool, or MCP server is invoked
- **And** no fallback to parent/admin policy occurs

#### Scenario: Parent/admin and job authority does not leak into student sessions

- **Given** the same human can access a parent/admin UI and a student device through distinct admitted clients/scopes
- **When** each starts a run
- **Then** server-selected profiles and persisted policy snapshots differ as configured
- **And** a student token/session/run ID cannot append to or subscribe to the parent/admin namespace
- **And** profile selection remains stable on retry/restart

#### Scenario: Identity issuance gap keeps student rollout off rather than weakening auth

- **Given** a deployment without a reviewed Identity-issued credential/grant for the workstation student caller
- **When** operators enable student remote mode
- **Then** readiness/configuration or caller admission fails closed with a documented blocker
- **And** the existing Fantasy workstation path remains available
- **And** raw Stytch tokens, device-local shared secrets, or unsigned student IDs are not accepted as substitutes

## Implementation Instructions

- Define immutable server-side execution profiles for `interactive_parent_admin`, `job`, and `student`. Map signed `client_id`/subject class/scopes and route to allowed profiles using configuration or an agents-owned admission table; never accept the profile/budgets/tools/model/system instructions as authoritative request fields.
- Student invariants are code/config validation constraints, not defaults that can be overridden: `MaxChildren=0`, `MaxDepth=0`, no child factory, empty delegation/MCP grant, finite run/token/event/session limits, and a restricted tool allowlist (empty unless a separately reviewed student-safe first-party tool is named). Unknown/missing policy fails closed.
- Expose a dedicated generated-client student session/turn boundary or a discriminated operation whose server route fixes the profile. Avoid reusing an admin create DTO containing authority fields.
- Add on-demand job operations and schedule CRUD/enable/disable/list/firing history. Schedule definitions contain a named server-owned job type plus bounded, schema-validated input; they do not contain arbitrary system prompts, provider URLs, shell commands, or credentials.
- Implement scheduler claims with DB leases/fences and unique schedule-firing idempotency. Freeze time-zone/cadence parser, maximum frequency, catch-up cap, clock-skew behavior, disable/delete semantics, and cancellation interaction. Multiple API/worker processes must be safe.
- Product authorization remains in each caller. Persist opaque product references and caller-supplied correlation IDs only under the validated owner namespace; do not call caller databases or infer role from those values.
- Update OpenAPI and generated clients. Document required Identity audiences/scopes/client admissions and the current production blocker for student credentials if Identity deployment cannot issue them yet. That blocker must keep the feature flag off, not add alternate auth.
- Add metrics/logs for profile, job type, schedule firing/lag, policy denial class, child denial, and duration, without student prompts or identifiers beyond approved opaque correlation hashes.

## End-to-End Test Plan

- Through generated Go/TS clients and a real service/PostgreSQL/local provider, start/retry an on-demand job, restart the service, and retrieve terminal result/events.
- Run two scheduler processes at the same due instant, then kill one around enqueue/advance. Query schedule firing/run rows and provider invocation count to prove exactly one firing/run and bounded catch-up.
- Use two principal/client namespaces to probe schedule/run IDOR and request-supplied ownership fields.
- Mint strict Identity-profile tokens for parent/admin, job, and student client/scope combinations. Start each profile and inspect persisted policy snapshots plus actual MAF/MCP behavior.
- Send every student escalation field and provider-driven child attempt; assert API rejection/policy event and zero child/MCP invocation.
- Try student remote startup/admission without the reviewed Identity client mapping; assert fail closed and then run existing Fantasy workstation acceptance with remote flag disabled.
- Run schedule claim and student policy matrices under `-race`; include restart/process boundaries and real DB queries.

## Anti-Cheating Audit

- Trace profile selection from validated JWT client/scope and route to immutable runtime spec. Reject request-body roles/profile names or default-to-admin branches.
- Search student code paths for positive `MaxChildren`, child-tool construction, MCP grants, arbitrary tool/model/system fields, or policy lookup that falls back on errors.
- Verify denied child/tool tests assert fixture invocation count zero, not just an error string/status.
- Inspect scheduler uniqueness/leases in PostgreSQL; reject in-memory timers as the schedule source of truth or sleep-only tests.
- Verify job handlers use the common durable run path rather than hard-coded job completion or a second process-local controller.
- Confirm opaque product refs do not trigger cross-service DB/API authorization calls and cannot change owner/profile.
- Search for raw Stytch/device shared-secret fallback and ensure missing Identity issuance leaves the feature disabled.

## Completion Gate

- [ ] All BDD scenarios pass through generated clients, real processes/PostgreSQL, and real MAF/local provider paths.
- [ ] On-demand and scheduled jobs are durable/idempotent across concurrency and restart.
- [ ] Student profile is server-selected, fail-closed, zero-child/zero-delegation, bounded, and proven with no denied invocation.
- [ ] Parent/admin, job, and student namespaces/profiles remain isolated on retry/restart.
- [ ] Missing production Identity student issuance is an explicit safe blocker, not alternate authentication.
- [ ] OpenAPI/clients, race/coverage/E2E, and existing repository gates pass unchanged.
