# Phase 11: Materialization domain

**File:** `phase-11-materialization-domain.md`
**Depends on:** Phase 8
**Duration guess:** 5–7 days
**Handoff wave:** see index orchestrator map

## Goal

Implement materialization runs against published (or allowed draft policy) plan revisions: generic learner/class profiles, complete input snapshots + fingerprints, item records, lock/unlock semantics, idempotent creation, and run status lifecycle without requiring live LLM (items may be seeded by stub runner until Phase 12).

## Scope

### In scope

- learner_profiles API/storage
- materialization_runs CRUD/list/get
- input_snapshot JSON + input_fingerprint sha256
- materialized_items list/get/update
- lock/unlock endpoints
- idempotency_keys for create
- events materialization.requested enqueued
- RBAC author+ for create; viewer read

### Out of scope / YAGNI

- Full agent generation (Phase 12)
- gRPC Primer API (Phase 15)
- Bundle assembly completeness may be partial until 12/15

## BDD Success Criteria

#### Scenario: P11-S1 — Create run with snapshot

- **Given** published revision + learner profile
- **When** POST materialization with window/policy
- **Then** run id mat_* status requested|running
- **And** input_snapshot present
- **And** fingerprint stable for same input
- **And** outbox materialization.requested

#### Scenario: P11-S2 — Learner and class profiles

- **Given** workspace author
- **When** create learner and class profiles by name
- **Then** kind enum
- **And** names primary
- **And** workspace scoped

#### Scenario: P11-S3 — Lock protects body

- **Given** run with item unlocked then locked
- **When** rematerialize or runner overwrite attempt
- **Then** locked body unchanged
- **And** error or skip with supersede rules on unlocked only

#### Scenario: P11-S4 — Rematerialize respects locks

- **Given** mixed locked/unlocked items
- **When** new run or rerun policy
- **Then** locked retained
- **And** unlocked may supersede with lineage

#### Scenario: P11-S5 — Unlock explicit

- **Given** locked item
- **When** unlock as author
- **Then** editable
- **And** audit locked_by cleared
- **And** subject_ref recorded on lock history

#### Scenario: P11-S6 — Idempotent create

- **Given** same Idempotency-Key + body
- **When** POST twice
- **Then** same mat id
- **And** single run row

## Implementation Instructions

1. Fingerprint canonical JSON serialization (sorted keys).
2. Item update rejects when locked (DB trigger + app).
3. List items server-side filter by runId/kind/status.
4. Stub completer optionally marks run ready with zero items for API testing — flag `STUDIO_MAT_STUB=1` test only.
5. Integration tests for lock triggers.

## End-to-End Test Plan

#### P11-E1 — Snapshot+fingerprint

- **Setup:** published plan
- **Action:** create run twice same input
- **Assert:**
  - same fingerprint
  - snapshot JSON equal canonical
- **Command:** `make studio-test`

#### P11-E2 — Profiles

- **Setup:** API
- **Action:** create list
- **Assert:**
  - names
  - isolation
- **Command:** `go test ./internal/materialization -run Profile`

#### P11-E3 — Lock overwrite

- **Setup:** item locked
- **Action:** attempt update/rematerialize
- **Assert:**
  - body hash unchanged
- **Command:** `go test ./internal/materialization -run Lock`

#### P11-E4 — Unlock audit

- **Setup:** locked
- **Action:** unlock
- **Assert:**
  - audit_events
- **Command:** `go test ./internal/materialization -run Unlock`

#### P11-E5 — Idempotency

- **Setup:** header key
- **Action:** double POST
- **Assert:**
  - one row
- **Command:** `go test ./internal/materialization -run Idempo`

## Anti-Cheating Audit

- Fingerprint not random UUID
- Lock not UI-only
- Idempotency not ignored
- Stub completer disabled in production env
- Snapshot must include plan revision id and profile refs

## Completion Gate

- [x] OpenAPI materialization/items ops implemented
- [x] P11 tests green with real Postgres
- [x] Ready for workflow runner attachment


## Dependencies

- Upstream: Phase 8
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
