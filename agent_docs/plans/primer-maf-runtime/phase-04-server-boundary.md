# Phase 4: Server boundary

## Goal

Integrate the runtime into the Primer server without changing the existing tutor endpoint or enabling autonomous student child authority. Expose an authenticated, authorized start/status/cancel/SSE boundary with detached run ownership. This phase makes the runtime a real server package rather than a library-only proof while preserving a reversible feature flag and existing API contracts.

## BDD Success Criteria

#### Scenario: Authorized parent starts and streams a run

- **Given** a configured runtime and an authenticated parent authorized for the relevant learner/session
- **When** the parent starts a run through the production HTTP API and opens its stream
- **Then** the server returns a server-generated run ID and the SSE response carries incremental attributed events
- **And** the run is owned by the controller rather than blocked on the request goroutine
- **And** terminal status is available after stream completion or disconnect

#### Scenario: Unauthorized callers cannot observe or cancel another run

- **Given** a run owned by learner A/parent A
- **When** a caller without that relationship requests status, stream, or cancel for the run
- **Then** the server returns the existing project authorization error shape/status
- **And** no run event, prompt, or cancellation side effect is exposed to the caller

#### Scenario: Student defaults cannot create children

- **Given** a student-authenticated request or student policy
- **When** the request attempts to start a child or ask the root to delegate
- **Then** the runtime denies it with a policy error and the MCP/tool fixture is not invoked
- **And** parent/admin policy is not inferred from client-provided fields

#### Scenario: HTTP disconnect does not abandon a run

- **Given** an authorized run whose stream client disconnects
- **When** the server finishes the stream handler
- **Then** the detached run continues to terminal status under controller ownership
- **And** a later authorized status request observes the terminal result/error

#### Scenario: Explicit cancel is authenticated and reaches nested work

- **Given** an authorized caller and a run blocked in nested MCP work
- **When** `POST`/`DELETE` to the documented cancel boundary is issued
- **Then** the server returns an idempotent cancellation acknowledgment
- **And** the nested MCP work observes cancellation
- **And** an unauthorized cancel attempt cannot affect it

#### Scenario: Feature-disabled deployment is safe

- **Given** runtime configuration is absent or disabled
- **When** the server starts and existing routes are exercised
- **Then** the server starts with the existing tutor behavior and no MAF provider initialization
- **And** runtime routes return a clear disabled/not-found response without panicking

## Implementation Instructions

- Add runtime configuration to `server/internal/config` with conservative disabled-by-default behavior, finite defaults, exact pin/version diagnostics, and no secrets in logs. Use the existing config loading conventions.
- Extend `server/internal/api.Options` only as necessary to inject a runtime/controller and register routes in the existing Huma/chi construction. Keep route types/signatures explicit so OpenAPI generation reflects the actual boundary; regenerate `web/openapi.yaml` and client only if routes are intentionally public to the admin SPA.
- Wire construction in `server/cmd/primer-server/main.go` behind the feature flag. The server should own graceful shutdown/cancel of active non-durable runs, bounded by shutdown timeout; HTTP request contexts must not be the run's only lifetime context.
- Reuse existing authentication/session/parent-student authorization helpers in `server/internal/api`; do not invent client-only authorization or accept owner/role/lineage as trusted request fields. Inspect existing student/parent route tests before choosing route placement and status codes.
- Add start/status/cancel/stream APIs with server-generated IDs, idempotent terminal handling, structured safe errors, and explicit content type/SSE behavior. Ensure status/stream access cannot read another tenant/learner's events.
- Keep the legacy `/student/sessions/{id}/tutor/messages` path on `internal/tutor` unless a later approved migration changes it; this phase is runtime integration, not a tutor rewrite.
- Verify with focused API tests first, then `make openapi`/`make client` if generated artifacts change, `make test`, `make build`, and `make cover`.

Required work is the minimal authenticated server boundary. Do not add durable run tables or claim restart recovery; process-local status disappears on process restart and that limitation must be documented in the API/runbook.

## End-to-End Test Plan

- Construct the production Huma/chi handler with a real controller and deterministic provider/MCP fixture, provision an authenticated parent/student through existing test utilities, and use HTTP start → SSE stream → status requests.
- Test parent/learner authorization across two separate fixtures: allowed owner succeeds; unrelated parent/student receives the project-standard authorization error and sees no events.
- Start a student request attempting delegation and assert the public policy denial plus no fixture invocation.
- Disconnect the SSE client after an early event, release provider work, then poll the authorized status endpoint and assert terminal success; separately issue HTTP cancel and assert nested MCP cancellation.
- Start with runtime disabled and run existing API/tutor tests, asserting no MAF initialization/route panic.
- Exact commands: focused `cd server && go test ./internal/api/... ./internal/agent/... -count=1`, `make test`, `make build`, and `make cover`.

## Anti-Cheating Audit

- Trace each route from Huma registration through controller invocation to real MAF provider/MCP execution; reject hard-coded JSON/status success.
- Inspect authz middleware/handler code and tests for server-side owner lookup; reject request-body owner IDs, role fields, or client-only checks.
- Confirm start is detached from `r.Context()` after the request has been accepted, while stream subscription still uses request lifetime.
- Confirm status is controller-backed process state, not a fabricated “complete” response or an unpersisted event replay sold as durability.
- Verify disabled mode does not silently fall back to a fake successful MAF response and legacy tutor tests remain real.
- Confirm generated OpenAPI/client artifacts, if changed, match handler signatures and no stale route claims remain.

## Completion Gate

- [ ] Start/status/cancel/stream are authenticated, authorized, and documented.
- [ ] Parent/child attribution is visible over real SSE bytes.
- [ ] Disconnect and explicit cancel have distinct observable effects.
- [ ] Student `max_children=0` is enforced at the server-owned policy boundary.
- [ ] Disabled mode preserves existing server behavior.
- [ ] OpenAPI/generated artifacts are current if applicable.
- [ ] `make test`, `make build`, and `make cover` pass without gate changes.
