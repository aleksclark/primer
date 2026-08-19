# Phase 8: Deployment, recovery, and release hardening

## Goal

Package, deploy, observe, and independently audit the completed service. This phase makes the standalone boundary operationally real: dedicated image/database/migration/Make/CI/deployment surfaces, restart and rollback drills, exact MAF dependency checks, retention/redaction, and honest release evidence. Live billable LLM and live Stytch proofs remain blocked rather than being smuggled into release acceptance.

## BDD Success Criteria

#### Scenario: Deployment artifact runs as its own service

- **Given** the `primer-agents` container image, agents-only PostgreSQL DSN, Identity issuer/JWKS, and explicit non-billable/local provider configuration
- **When** the image starts under production-like orchestration
- **Then** its own `primer-agents` process migrates/connects only to the agents database
- **And** health/readiness checks reflect process/database/worker admission accurately
- **And** LMS, Studio, Identity, and TV processes/databases are not required to be co-located

#### Scenario: Process restart preserves control-plane truth

- **Given** persisted sessions, queued/running/cancel-requested runs, and event cursors
- **When** the deployed task is force-killed and replaced using the same agents database
- **Then** IDs/status/events remain queryable
- **And** queued work, cancellation, and provider-started interruption follow the frozen recovery contract
- **And** SSE reconnect resumes by durable sequence
- **And** no duplicate provider invocation occurs for an interrupted started run

#### Scenario: Isolation and secret handling withstand production-like probes

- **Given** foreign DSNs, wrong-audience/raw-Stytch-shaped tokens, poisoned Origin/headers, oversized bodies, provider/JWKS outages, and secret marker values
- **When** config, HTTP, worker, and logs are exercised
- **Then** the service fails closed at the correct boundary
- **And** no migration touches foreign databases
- **And** logs/metrics/traces/events do not expose bearer/provider/DSN secrets or unapproved prompt/tool content

#### Scenario: MAF CONDITIONAL GO drift fails release

- **Given** an attempted MAF version/import/child-stream change
- **When** dependency and compatibility gates run
- **Then** release fails unless the exact pin, public-only imports, custom streaming-child path, budget/authority tests, Primer envelope, and stream/run split are intentionally requalified
- **And** no test/doc calls MAF workflows a distributed durable control plane

#### Scenario: Rollout and rollback do not strand callers

- **Given** a cohort rollout with remote caller flags and the service deployment
- **When** operators disable a caller flag or roll the service back
- **Then** unaccepted flows stay on legacy Fantasy/LMS behavior
- **And** accepted remote run IDs remain queryable/recoverable from the durable service database
- **And** rollback does not require sharing/deleting caller databases or lowering auth/policy checks

#### Scenario: Build, contract, coverage, and E2E gates are first-class

- **Given** a clean checkout
- **When** documented root/module CI commands run
- **Then** agents build/test/race/coverage (at least 85%), OpenAPI/client drift, migration, process E2E, restart E2E, Docker smoke, and deployment contract tests pass
- **And** existing root `make test`, `make cover`, `make build`, Identity, and Studio gates retain their thresholds/meaning

#### Scenario: Blocked external proofs remain honestly blocked

- **Given** no approved live billable provider or live Stytch acceptance phase in this plan
- **When** release evidence is reviewed
- **Then** it records local deterministic/provider and credential-free Identity proof only
- **And** no default/CI target reads ambient billable or Stytch credentials
- **And** live billable LLM, live Stytch, Studio S19, and future MCP remain explicitly BLOCKED/out of scope

## Implementation Instructions

- Add a multi-stage `Dockerfile.agents` (or module-local equivalent) building only the agents binary/migrations/runtime artifacts and running as non-root in a minimal image. Do not bundle LMS/Studio SPAs or another service binary.
- Add honest root/module Make targets: `agents-build`, `agents-test`, `agents-race`, `agents-cover`, `agents-openapi`, `agents-client`, `agents-e2e`, `agents-restart-e2e`, `agents-migrate`, `docker-agents`, and an isolated dev-DB target only if it creates a coherent agents database. Required env checks must not print secrets.
- Add a CI workflow/path filter for `primer-agents`, generated clients, caller adapters, and shared deployment files. Enforce ≥85% internal-package coverage without lowering root/Identity/Studio floors. Add deterministic codegen/breaking-contract checks and `go mod tidy -diff`/`go vet`/`git diff --check`.
- Add a deployment job parallel to existing Nomad service patterns: independent image digest, CPU/memory, dynamic HTTP port, service registration/router, `/healthz` and `/readyz` checks, `PRIMER_AGENTS_*` env, agents-only DB secret, Identity/JWKS fields, provider secrets, and safe rolling-update/shutdown policy. Update deployment contract tests, expected service inventory, image lock, and runbook.
- Do not share a Nomad variable/DSN field with LMS/Studio/Identity. Document backup/restore, migration, retention/pruning, event/session content classification, key/provider rotation, JWKS outage, worker drain, lease recovery, interrupted-run handling, cancellation, SSE replay, and rollback.
- Add operational metrics: request/status latency, auth denials, queued/running/cancel-requested counts, queue age, lease recovery/interruption, run duration/state/error class, child/policy denial, durable event append/replay lag, subscribers, schedule lag, provider/JWKS/DB health. Labels must be bounded and content-free.
- Automate the MAF pin/public-import/custom-child audit, reusing or superseding `scripts/check-maf-runtime.sh` for the new authoritative module. Any pin update requires rerunning feasibility/compatibility/race/restart evidence and recording a new CONDITIONAL GO decision.
- Add production-like process/restart/load tests with real PostgreSQL, loopback Identity JWKS, and local non-billable provider/MCP fixtures. Inject failures without production test branches. Record commands/results in an implementation report.
- Keep live proofs blocked. If a future plan names a live billable qualification phase, it must be opt-in, credential-scope-safe, cost-bounded, and separate; this plan does not add such a target.

## End-to-End Test Plan

- Build and run the exact container image with separate disposable agents and decoy foreign databases. Migrate/start, call health/readiness/authenticated run APIs through generated clients, and inspect DB isolation.
- Run an OS/container kill/restart matrix with queued, provider-started, cancel-requested, completed, active SSE, parent session, schedule firing, and student run. Assert frozen recovery semantics and provider invocation counts.
- Run wrong-audience/raw-Stytch-shaped/oversized/foreign-DSN/provider-down/JWKS-down/DB-down probes. Capture logs/metrics and scan for planted JWT, API key, DSN password, prompt, tool argument, and student-context markers.
- Run two instances during rolling update, concurrent workers/schedulers/subscribers, then drain/terminate one. Assert lease fences, one schedule/run execution, replay, and graceful bounded shutdown.
- Exercise cohort flags off/on and service rollback with LMS/workstation acceptance; assert no dual execution and durable remote IDs remain available.
- Run all documented commands from a clean checkout, including agents module gates, generated artifact comparisons, Docker smoke, deployment contract, existing `make test`, `make cover`, `make build`, and relevant Identity/Studio contract tests.

## Anti-Cheating Audit

- Inspect image entrypoint/process list and database connections; reject an LMS binary renamed as agents or shared DSN/migration configuration.
- Verify restart tests kill the real process/container and reuse PostgreSQL; reject in-memory reconstruction, direct row patching to expected success, or goroutine cancellation as restart evidence.
- Trace readiness/metrics to real DB/worker/provider admission checks; reject constant health, unbounded/high-cardinality labels, or content leakage.
- Inspect CI/Make targets for skipped tests, lowered coverage, `|| true`, hidden network credentials, ambient env use, or generated artifacts updated without drift checks.
- Run pin/import/stock-collection scans against production source, not docs. Confirm the exact module version selected by Go.
- Review deployment variables/secrets for cross-service DB reuse, static agents API key auth, raw Stytch fields, browser-exposed provider credentials, or secrets in command arguments.
- Search docs/release notes for “exactly once,” “resumed model call,” “distributed control plane,” live provider/Stytch, Studio MCP/S19, or efficacy claims unsupported by E2E.
- Confirm retired August 3 prototype code was not restored and the historical wave-1 plan was not rewritten.

## Completion Gate

- [ ] All BDD scenarios and production-like process/container/restart/rolling-update E2E pass.
- [ ] Dedicated image, database, migrations, Make targets, CI, deployment job, health/readiness, metrics, backup/recovery/rollback runbooks are current.
- [ ] Agents coverage is ≥85%; race/vet/build/codegen/migration/deployment gates pass.
- [ ] Existing root `make test`, `make cover`, `make build` and relevant Identity/Studio gates pass with no lowered thresholds.
- [ ] Exact MAF CONDITIONAL GO audit passes and update procedure is documented.
- [ ] Isolation/redaction/security probes find no shared DB, raw Stytch, token/provider/DSN/content leak, or fake production seam.
- [ ] Live billable LLM, live Stytch, Studio S19, and MCP remain explicitly BLOCKED/out of scope.
- [ ] Independent implementation/security review reports no Critical or Important findings before rollout beyond a test cohort.
