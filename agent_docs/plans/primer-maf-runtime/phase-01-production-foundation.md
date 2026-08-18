# Phase 1: Production foundation

## Goal

Establish a compilable production MAF adapter in `server/internal/agent` without wiring it into student traffic. This phase comes first because all later controller and HTTP work needs stable Primer-owned contracts, exact dependency provenance, and deterministic tests. After this phase, a server package can construct a bounded parent/child runner using public MAF APIs while the existing tutor path is unchanged.

## BDD Success Criteria

#### Scenario: Production package uses the pinned public MAF SDK

- **Given** a clean checkout with the repository `go.work`
- **When** `cd server && go list -m -json github.com/microsoft/agent-framework-go` is run and the package import graph is inspected
- **Then** the selected version resolves to the exact pseudo-version for upstream commit `00ffc8c3648c547997eae3a3f2a3b00c28daea09`
- **And** no production file imports `github.com/microsoft/agent-framework-go/internal/...`
- **And** the standalone `spikes/maf-go` module remains unchanged

#### Scenario: A permitted child gets a distinct identity and least authority

- **Given** a parent run with a named agent and an allowlist containing `shared_calc` and `parent_only`
- **When** it prepares a child request allowing only `shared_calc`
- **Then** the child provider observes the child's own ID, name, and instructions
- **And** the child surface contains `shared_calc` but not `parent_only`
- **And** the child cannot request a tool outside the parent grant

#### Scenario: Untrusted student authority is fail-closed

- **Given** an agent policy with `max_children=0` or an empty tool allowlist
- **When** a child start or tool call is attempted
- **Then** the request is denied with a typed policy error before child work/tool invocation
- **And** any provisional budget reservation is returned

#### Scenario: Existing server behavior is unchanged when runtime is disabled

- **Given** a normal server construction with no MAF runtime enablement
- **When** existing tutor API tests send a student tutor message
- **Then** the existing tutor response, policy, event record, and fallback behavior remain unchanged
- **And** no MAF provider or child run is started

## Implementation Instructions

- Add the exact MAF requirement to `server/go.mod` and update `server/go.sum` using the repository's Go toolchain. Do not copy the spike's `go.mod`; retain the server module's Go directive and dependency conventions.
- Create `server/internal/agent` with Primer-owned types for agent identity/specification, child requests, run events, tool grants, and policy errors. Keep MAF types behind the package boundary where practical so preview churn is localized.
- Port only the proven adapter concepts from `spikes/maf-go/primer`: child construction with independent instructions/identity, public `tool.FuncTool` streaming hook shape, fail-closed tool filtering, and lineage/event data. Do not port the spike's minimal HTTP handler as production controller behavior; Phase 2 owns that replacement.
- Keep provider construction injectable for deterministic tests. A local scripted provider and in-memory official MCP transport are permitted test fixtures because they exercise real MAF agent/tool/MCP paths without credentials; they must not become production branches or claim live-provider evidence.
- Add package-level documentation stating MAF is a pinned public-preview SDK and this package is non-durable/in-process. Cite the spike evidence and exact pin in code comments or package docs.
- Verify incrementally with `cd server && go test ./internal/agent/...`, `go test -race ./internal/agent/...`, `go vet ./...`, and `go build ./...`.

Required work ends when the package is independently usable and tested. Do not add HTTP routes, database tables, or student-default runtime enablement in this phase.

## End-to-End Test Plan

- Build a test harness through the production `agent` package that uses a scripted local provider and official in-memory MCP transport. Start a parent run and a permitted child through the exported runtime contract; assert provider-observed identity/instructions and actual tool invocation.
- Send denied child/tool requests through the same exported contract; assert typed denial, no denied MCP invocation, and no residual budget reservation.
- Run the existing `server/internal/api` tutor tests with runtime disabled and assert their persisted tutor event behavior; this is the compatibility boundary, not a direct private-method test.
- Run `cd server && go test ./internal/agent/... -count=1`, `go test -race ./internal/agent/...`, then `go test ./... -count=1`.

The deterministic provider/MCP fixtures are allowed supplements, not replacements for later HTTP E2E tests. This phase has no public HTTP surface yet.

## Anti-Cheating Audit

- Inspect `server/go.mod`/`go.sum` and all `server/internal/agent` imports for the exact MAF pin and absence of `internal` imports.
- Confirm child tests observe provider/MCP effects rather than merely asserting a returned string or mocked collaborator call.
- Search for hard-coded successful child/tool responses, runtime-disabled test branches, and empty-allowlist bypasses.
- Confirm `spikes/maf-go` is not deleted, rewritten into the server module, or imported as production code.
- Confirm student `max_children=0` is enforced by the production policy path and not only by a fixture setup.

## Completion Gate

- [ ] All BDD scenarios pass.
- [ ] Public MAF import and exact-version audit passes.
- [ ] Agent package unit/integration tests pass under `-race`.
- [ ] Existing server test/build gates pass with no threshold changes.
- [ ] No HTTP or student traffic is enabled accidentally.
- [ ] A reviewer has checked public-only imports and spike/production separation.
