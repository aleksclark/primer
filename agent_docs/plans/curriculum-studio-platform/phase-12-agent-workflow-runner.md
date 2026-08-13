# Phase 12: Agent workflow runner

**File:** `phase-12-agent-workflow-runner.md`
**Depends on:** Phase 11
**Duration guess:** 7–10 days
**Handoff wave:** see index orchestrator map

## Goal

Build the staged agent workflow runner that persists workflow_stages/attempts, calls a LanguageModel provider seam, resumes after process kill, handles provider failures, and produces materialized items with provenance. Credential-free completion uses scripted model doubles only; live providers BLOCKED until Phase 18.

## Scope

### In scope

- `internal/workflow` engine + stage graph (objectives already exist → materializer stages: lessons, assessments, critic)
- LanguageModel interface; `scripted` provider from golden JSON fixtures
- Persist stage status transitions
- Resume: on boot, reclaim running stages with lease/heartbeat
- Kill test: SIGKILL worker mid-stage → restart resumes
- Assessment support linkage rubric/answer_key before item publish
- Run status ready/failed/cancelled
- Events materialization.ready/failed
- Metrics stage latency counters

### Out of scope / YAGNI

- Live Bedrock/OpenRouter quality evaluation (Phase 18)
- Fantasy SDK deep integration optional; may use plain HTTP client seam
- Multi-tenant fair scheduling beyond FIFO workspace

## BDD Success Criteria

#### Scenario: P12-S1 — Stages persist

- **Given** materialization requested with scripted provider
- **When** runner executes
- **Then** stages rows pending→running→succeeded
- **And** attempts recorded
- **And** run becomes ready with items

#### Scenario: P12-S2 — Resume after kill

- **Given** stage running
- **When** process killed hard then restarted
- **Then** stage resumes without duplicating succeeded stages
- **And** exactly-once item production per stage id or idempotent upsert

#### Scenario: P12-S3 — Scripted double generation

- **Given** `STUDIO_MODEL_PROVIDER=scripted`
- **When** run materialization
- **Then** items kinds include lesson and assessment fixtures
- **And** no outbound network to real LLM
- **And** provenance stores provider=scripted + fixture id

#### Scenario: P12-S4 — Provider failure

- **Given** scripted provider configured to fail mid-run
- **When** execute
- **Then** stage failed
- **And** run failed
- **And** outbox materialization.failed
- **And** retry API restarts failed stage only

#### Scenario: P12-S5 — Assessment supports required

- **Given** assessment item without rubric/key
- **When** attempt publish item status published
- **Then** rejected by trigger/app
- **And** with supports succeeds

## Implementation Instructions

1. Define stage names stable strings matching product agents where applicable.
2. Worker loop in-process goroutine with graceful shutdown drain.
3. Lease columns: if schema lacks leases, use workflow_attempts heartbeat fields or file db-track additive migration blocker.
4. Item writes include plan_revision_id, run_id, optional unit/outcome ids, artifact_ref optional.
5. Critic stage may only emit validation findings, not bypass deterministic engine.
6. Explicit residual risk: scripted output ≠ pedagogical quality.

## End-to-End Test Plan

#### P12-E1 — Happy scripted run

- **Setup:** published plan + scripted fixtures
- **Action:** create mat and wait
- **Assert:**
  - ready
  - items≥1
  - stages succeeded
- **Command:** `make studio-test`

#### P12-E2 — Kill resume

- **Setup:** instrumental pause in scripted provider
- **Action:** kill process; restart; continue
- **Assert:**
  - no duplicate succeeded stage side effects
  - run completes
- **Command:** `go test ./internal/workflow -run KillResume`

#### P12-E3 — Scripted no network

- **Setup:** packet filter or httptestassert
- **Action:** run
- **Assert:**
  - provider seam only touched
  - DNS to openai/bedrock not required
- **Command:** `go test ./internal/workflow -run Scripted`

#### P12-E4 — Failure+retry

- **Setup:** fail once fixture
- **Action:** run then retry
- **Assert:**
  - failed then ready
- **Command:** `go test ./internal/workflow -run Retry`

#### P12-E5 — Assessment gate

- **Setup:** assessment without supports
- **Action:** publish
- **Assert:**
  - fail then pass after supports
- **Command:** `go test ./internal/workflow -run Assessment`

## Anti-Cheating Audit

- Runner not a synchronous fake in handler without stage rows
- Resume not full restart deleting items blindly
- Live provider credentials not required for gate
- Do not mark Phase 18 live complete here
- Deterministic validation still runs

## Completion Gate

- [ ] P12-S*/E* green with scripted provider
- [ ] Kill-resume proven
- [ ] Live provider items explicitly BLOCKED open
- [ ] Provenance fields present on items


## Dependencies

- Upstream: Phase 11
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
