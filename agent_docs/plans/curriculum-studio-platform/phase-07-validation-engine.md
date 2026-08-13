# Phase 7: Validation engine

**File:** `phase-07-validation-engine.md`
**Depends on:** Phase 6
**Duration guess:** 4–5 days
**Handoff wave:** see index orchestrator map

## Goal

Ship deterministic validation engine producing `validation_reports` / `validation_findings` for coverage gaps, prerequisite issues, missing evidence, workload caps, and superficial standards warnings — without agents.

## Scope

### In scope

- `internal/validation` pure functions + DB persistence
- POST validateRevision / GET latest report
- Finding severities error/warning/info
- Determinism: sorted inputs → stable finding codes/messages
- Integrate checks: missing evidence, unmapped outcomes, overloaded minutes vs constraints, empty units

### Out of scope / YAGNI

- LLM critic (workflow phase)
- Publish gating UI (uses this in Phase 8/10)

## BDD Success Criteria

#### Scenario: P7-S1 — Coverage gaps reported

- **Given** draft with outcome missing standard mapping
- **When** run validation
- **Then** report status failed or warning per rules
- **And** finding code stable e.g. OUTCOME_UNMAPPED
- **And** finding references outcome name

#### Scenario: P7-S2 — Clean plan passes

- **Given** well-formed fixture plan
- **When** validate
- **Then** status passed
- **And** zero error findings

#### Scenario: P7-S3 — Cyclic outcome prereq fails

- **Given** draft with cycle
- **When** validate and/or edge insert
- **Then** error finding or write rejected
- **And** no silent accept

#### Scenario: P7-S4 — Evidence and workload findings

- **Given** outcome without evidence; constraints overload
- **When** validate
- **Then** distinct finding codes
- **And** severities set

#### Scenario: P7-S5 — Deterministic rerun

- **Given** same draft unchanged
- **When** validate twice
- **Then** same finding multiset
- **And** stable ordering in API

## Implementation Instructions

1. Define finding code catalog in `internal/validation/codes.go`.
2. Run read-only queries; write report+findings transactionally.
3. Never call model providers.
4. Unit tests for each rule with table-driven cases; integration on real graph.

## End-to-End Test Plan

#### P7-E1 — Gap detection

- **Setup:** seeded bad plan
- **Action:** POST validate
- **Assert:**
  - findings persisted
  - GET returns same
- **Command:** `make studio-test`

#### P7-E2 — Cycle

- **Setup:** cyclic edges
- **Action:** validate
- **Assert:**
  - error
- **Command:** `go test ./internal/validation -run Cycle`

#### P7-E3 — Evidence/workload

- **Setup:** fixture
- **Action:** validate
- **Assert:**
  - codes present
- **Command:** `go test ./internal/validation -run Evidence`

#### P7-E4 — Determinism

- **Setup:** fixture
- **Action:** double validate
- **Assert:**
  - equal JSON canonical
- **Command:** `go test ./internal/validation -run Determinism`

## Anti-Cheating Audit

- Engine must not always return passed
- No randomness in IDs affecting equality without canonicalization
- Reports not computed only in memory without DB when API claims persistence
- Not replaced by agent critic

## Completion Gate

- [ ] Finding catalog documented
- [ ] All P7 tests green
- [ ] Used by Phase 8 publish gate


## Dependencies

- Upstream: Phase 6
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
