# Phase 5: Wave-1 hardening and rollout

## Goal

Make the server-integrated runtime reviewable and safely deployable as a preview feature. This final phase does not add durability; it closes evidence gaps, validates resource behavior under race/reconnect/load-shaped tests, records the exact preview dependency and rollback procedure, and prevents future documentation from overstating what wave 1 provides.

## BDD Success Criteria

#### Scenario: Repeated concurrency and reconnect runs remain safe

- **Given** multiple authorized runs with concurrent child emits, slow readers, disconnects, and explicit cancels
- **When** the hardened test suite runs repeatedly under the race detector
- **Then** there are no races, panics, unbounded queue growth, or leaked active runs/children
- **And** each run reaches one terminal state and counters return to their expected baseline

#### Scenario: Provider failure is observable and bounded

- **Given** the pinned MAF provider returns an error or MCP becomes unavailable
- **When** a production run executes
- **Then** the client receives a safe terminal error event/status
- **And** logs/metrics identify run ID, agent identity, failure class, and duration without secrets or full student prompts
- **And** retries do not become an unbounded loop

#### Scenario: Preview pin drift is detected

- **Given** a dependency update or clean checkout
- **When** the runtime dependency audit and build gate run
- **Then** the exact MAF pseudo-version/commit assertion fails if changed without an intentional update
- **And** the update procedure requires rerunning the runtime compatibility tests and review

#### Scenario: Rollback leaves the legacy path usable

- **Given** runtime feature configuration is disabled or deployment is rolled back
- **When** existing tutor/API traffic is exercised
- **Then** legacy functionality remains available and no runtime run is accepted
- **And** no database migration or durable recovery step is required to roll back

#### Scenario: Documentation states the operational boundary honestly

- **Given** a maintainer reads the runtime plan/runbook
- **When** they assess restart, durability, student authority, and Fantasy/TUI compatibility
- **Then** the documents state process-local non-durability, explicit cancel/authz requirements, MAF preview status, and the fact that Fantasy remains for TUI loops
- **And** no unsupported production or learning-efficacy claim is present

## Implementation Instructions

- Add race/reconnect/concurrency tests at the highest real boundary that remains deterministic. Keep local scripted provider and MCP fixtures clearly named test-only; do not add billable credentials to CI.
- Add leak checks around controller shutdown and active-run/child counts. Use bounded test timeouts, but do not turn timeouts into success assertions.
- Add a dependency/import audit script or deterministic CI check for the exact MAF module revision and absence of `agent-framework-go/internal` imports. Re-run `go mod verify`/`go list` as appropriate.
- Add operational documentation adjacent to the runtime package or an existing runbook location describing feature flag, finite limits, SSE disconnect/cancel semantics, process restart loss, logs/metrics, rollback, and upgrade protocol. Do not label this a Studio wave or add unrelated delivery docs.
- Review configuration defaults, response redaction, authorization, shutdown behavior, and generated output. Ensure `go vet`, formatting, and diff checks are clean.
- Run full gates from the repository root: `make test`, `make cover`, `make build`; also run `cd server && go test -race ./... -count=1` if runtime cost is acceptable. Record honest results in the PR/implementation report.

Out of scope remains Ultracore/durable control plane, broad provider matrix, and TUI migration. Record those as follow-up work rather than hiding them behind this phase.

## End-to-End Test Plan

- Run an `httptest`-backed authorized server with several concurrent runs and scripted barriers, then perform stream reads, client disconnects, explicit cancels, provider failures, and status polls; assert terminal evidence and baseline counters.
- Run the same scenarios under `go test -race` repeatedly and inspect goroutine/resource cleanup after controller shutdown.
- Execute a clean-checkout dependency audit and full build/test/coverage commands; intentionally test a pin mismatch in the audit fixture if the repository convention supports it.
- Disable the feature and rerun legacy tutor API tests as the rollback test.
- Review logs/metrics captured by a test sink for redaction and useful run/failure attribution.

## Anti-Cheating Audit

- Check for tests that only assert HTTP 200, event counts, fixture strings, or terminal status without verifying actual MAF provider/MCP execution and cancellation.
- Inspect race/leak tests for skipped cases, test-only production branches, infinite/broad retries, and timeouts treated as proof of cleanup.
- Verify the dependency audit examines actual production imports and `server/go.mod`, not only a documentation string.
- Confirm rollback is the real disabled configuration path and does not delete data, hide errors, or swap in fake MAF success.
- Check docs and release notes for unsupported durable-run, multi-tenant, autonomous-student, or efficacy claims.

## Completion Gate

- [ ] Repeated race/reconnect/concurrency E2E evidence is green.
- [ ] Failure, redaction, bounded retry, and shutdown behavior are verified.
- [ ] Exact pin/import audit is automated or reproducibly documented.
- [ ] Rollback and non-durability runbook is current.
- [ ] Full `make test`, `make cover`, and `make build` gates pass unchanged.
- [ ] Independent implementation and review reports identify no blockers.
