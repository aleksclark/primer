# Phase 1: Standalone foundation and engine extraction

## Goal

Create the `primer-agents/` module and a real `cmd/primer-agents` process, then transfer ownership of the proven MAF adapter from the LMS tree to this module. This phase comes first so all durability and API work lands at the correct service boundary. Afterward the new process has namespaced configuration, isolated database bootstrap, health/readiness, structured logs, graceful shutdown, and the exact pinned MAF engine; existing LMS/Fantasy behavior remains available and disabled-by-default behavior does not change.

## BDD Success Criteria

#### Scenario: Standalone process boots and reports real readiness

- **Given** a real agents-safe PostgreSQL database and valid `PRIMER_AGENTS_*` development/test configuration
- **When** the built `primer-agents` binary starts
- **Then** `GET /healthz` reports process liveness
- **And** `GET /readyz` succeeds only after configuration validation, migration, and a real database ping
- **And** SIGTERM produces bounded graceful shutdown and structured JSON logs without DSN credentials

#### Scenario: Configuration is isolated and fail-fast

- **Given** `PRIMER_AGENTS_DATABASE_URL` is absent, blank, or points to an LMS, TV, Studio, or Identity database
- **When** the production binary starts
- **Then** it exits nonzero before migration/listen
- **And** ambient `DATABASE_URL`, `STUDIO_DATABASE_URL`, `IDENTITY_DATABASE_URL`, or `TV_DATABASE_URL` values are ignored
- **And** no rejected DSN or provider secret is printed

#### Scenario: The service owns the exact proven MAF engine

- **Given** the new module and the historical wave-1 implementation
- **When** the runtime compatibility suite runs
- **Then** the selected MAF pseudo-version resolves to upstream commit `00ffc8c3648c547997eae3a3f2a3b00c28daea09`
- **And** child identity, authority intersection, budgets, streaming-child behavior, and stream/run context tests retain their wave-1 semantics
- **And** no production file imports `agent-framework-go/internal` or substitutes stock `agenttool` collection for nested streaming

#### Scenario: Existing callers do not switch implicitly

- **Given** normal LMS and workstation configuration with new remote flags unset
- **When** existing tutor/TUI/API acceptance tests run
- **Then** Fantasy and current LMS tutor behavior remain unchanged
- **And** no request is sent to `primer-agents`
- **And** existing `AGENT_RUNTIME_ENABLED=false` behavior remains safe

## Implementation Instructions

- Create `primer-agents/` as a separate module with frozen path `github.com/aleksclark/primer/agents`; add it to `go.work`. Use a thin `cmd/primer-agents/main.go` over an `internal/app` bootstrap, following `primer-identity/internal/app` and `curriculum-studio/internal/app` process-test patterns.
- Use the `PRIMER_AGENTS` envconfig prefix exclusively. Initial config includes database URL, host/port, environment, log level, shutdown and HTTP timeouts, body cap, provider mode/base URL/model/secret references, worker enablement, and exact Identity endpoint fields reserved for Phase 3. Production validation is fail-closed; provider credentials never receive localhost defaults or appear in formatting/logging.
- Establish an agents-only DB validator, embedded goose migrator, connection package, dedicated version table, and foundation migration. Suggested databases are `primer_agents` and `primer_agents_test`. Reuse patterns, not internal packages, from Identity/Studio; separate Go modules cannot import another module's `internal` code.
- Provide `/healthz` and `/readyz`. Health must not claim provider availability. Readiness must include DB reachability/migration compatibility; later phases may add worker/provider readiness dimensions without turning temporary provider failure into process liveness failure.
- Add redacting structured request/process logging with request ID, method, path without query, status, duration, environment, and service version. Never log Authorization, prompt/body, DSN, provider key, or response text.
- Move/extract the MAF adapter and tests from `server/internal/agent` into an agents-owned runtime package. During migration, a narrowly documented compatibility import/shim may keep the LMS preview compiling; the new module is the single source of runtime behavior and no second independently edited copy may remain. Add an import-policy test rejecting new direct runtime consumers beyond the service and temporary LMS compatibility seam.
- Preserve the exact MAF pin, custom `StreamingChildTool`, fail-closed tool intersection, depth/direct/total budgets, reservation release, attributed event envelope, bounded subscriber bridge semantics, and explicit runtime-owned cancellation context. Keep the [historical wave-1 plan](../primer-maf-runtime/index.md) and spike unchanged.
- Do not add public run routes or claim persistence beyond the foundation migration. Do not copy August 3 prototype packages/branches.
- Add focused module targets or commands usable immediately: `cd primer-agents && go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./cmd/primer-agents`. Root targets and packaging are completed in Phase 8.

## End-to-End Test Plan

- Start the real process on an injected listener with a real testcontainer PostgreSQL database; call `/healthz` and `/readyz`, verify the dedicated goose table, send SIGTERM, and inspect structured/redacted logs.
- Build and execute the binary with missing and adversarial foreign/ambient DSNs; assert nonzero exit before listening/migration and no secret leakage.
- Run the production runtime package through real MAF `Agent`/tool paths against the deterministic scripted provider and official in-memory MCP fixture inherited from wave 1. Assert actual child provider/tool effects, not only returned fixture strings.
- Run existing LMS API/tutor and workstation-focused tests with all remote flags unset; assert no remote transport invocation and unchanged behavior.
- Commands: `cd primer-agents && go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...`, `go build ./cmd/primer-agents`; then repository `make test`, `make cover`, and `make build`.

Permitted fixtures are a local deterministic provider, official in-memory MCP transport, and test Identity placeholders not yet used for auth. They prove the real runtime path without billable credentials; they do not count as live-provider evidence.

## Anti-Cheating Audit

- Inspect module ownership and imports: no active runtime implementation remains only under `server/internal/agent`, no divergent duplicate is maintained, and no consumer other than the temporary LMS compatibility seam imports the engine directly.
- Verify MAF pin from `go list -m`, production imports for `/internal/`, and child implementation for `agenttool.New`/`Collect()` substitution.
- Confirm health is liveness only and readiness performs a real DB ping/migration check rather than returning constant 200.
- Inspect config/environment tests for accidental bare `DATABASE_URL` fallback and foreign DSN bypasses in library-level Connect/Migrate calls.
- Search logs and formatting methods for DSN, API key, Authorization, prompts, tool arguments, and event text.
- Compare moved tests to historical behavior; reject deleted negative/budget/race tests disguised as extraction.
- Search the diff/history references for retired August 3 prototype code or copied prototype package names.

## Completion Gate

- [ ] All BDD scenarios pass through the real process/runtime boundaries.
- [ ] `primer-agents` is in `go.work`, builds independently, and owns the MAF adapter.
- [ ] Exact MAF pin/public-import/custom-streaming audits pass.
- [ ] Health/ready/config/shutdown/redaction process E2E is green with real PostgreSQL.
- [ ] Existing LMS/workstation paths remain unchanged with remote flags off.
- [ ] Module race/vet/build and existing `make test`, `make cover`, `make build` pass unchanged.
- [ ] No public run API, durability overclaim, live provider call, or prototype resurrection was introduced.
