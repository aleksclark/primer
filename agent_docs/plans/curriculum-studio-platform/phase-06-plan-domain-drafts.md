# Phase 6: Plan domain drafts

**File:** `phase-06-plan-domain-drafts.md`
**Depends on:** Phases 4 and 5
**Duration guess:** 6–8 days
**Handoff wave:** see index orchestrator map

## Goal

Implement curriculum identity and **draft** plan revisions with full graph editing: objectives, outcomes, mappings, prerequisites, learning arcs, units/projects stubs, evidence requirements, scheduling constraints, plan resources. Editing allowed only on drafts.

## Scope

### In scope

- curricula + plan_revisions APIs
- graph get/replace and granular node/edge ops per OpenAPI
- outcome↔standard mappings; outcome prerequisites
- units and basic projects tables writes
- optimistic concurrency via revision row version / updated_at
- `curriculum.created` outbox row on create (worker may no-op until Phase 14)
- Prefixed IDs cur_, prev_, obj_, out_, …

### Out of scope / YAGNI

- Publish immutability (Phase 8)
- Materialization
- Full SPA editor (Phase 10)

## BDD Success Criteria

#### Scenario: P6-S1 — Create curriculum

- **Given** author workspace
- **When** POST curriculum with name/approach
- **Then** 201 cur_*
- **And** status mapping per crosswalk
- **And** outbox curriculum.created inserted

#### Scenario: P6-S2 — Create draft revision

- **Given** curriculum exists
- **When** POST revision brief payload
- **Then** draft status
- **And** editable graph empty or template

#### Scenario: P6-S3 — Replace graph

- **Given** draft revision
- **When** PUT graph with objectives, outcomes, arc, unit, mappings
- **Then** persisted
- **And** GET graph returns names/titles
- **And** standards refs valid or 400

#### Scenario: P6-S4 — Granular node ops

- **Given** draft with nodes
- **When** create/update/delete node and edge
- **Then** mutations persist
- **And** invalid standard ref rejected

#### Scenario: P6-S5 — Concurrent edit conflict

- **Given** two clients same draft version
- **When** both PATCH revision
- **Then** one succeeds
- **And** other 409 conflict

## Implementation Instructions

1. Domain services in `internal/plan` orchestrating multi-table writes in transactions.
2. Validate standard_ids exist and are visible.
3. Outcome prereq same-revision enforcement (DB trigger assists).
4. Graph JSON schema internal; map to tables (not only JSON blob) — tables are SoT per SCHEMA.
5. List curricula server-side with name primary.
6. Large integration fixtures for a minimal Grade 6 plan.

## End-to-End Test Plan

#### P6-E1 — Curriculum+draft create

- **Setup:** standards+resources seeded
- **Action:** create curriculum+revision
- **Assert:**
  - DB rows
  - outbox event
- **Command:** `make studio-test`

#### P6-E2 — Graph round-trip

- **Setup:** draft
- **Action:** replace then get
- **Assert:**
  - structure equal
  - titles present
- **Command:** `go test ./internal/plan -run Graph`

#### P6-E3 — Conflict

- **Setup:** two sessions
- **Action:** concurrent patch
- **Assert:**
  - 409 path works
- **Command:** `go test ./internal/plan -run Conflict`

## Anti-Cheating Audit

- Graph not stored only in opaque JSON bypassing tables
- Published path not accidentally open
- No cross-workspace curriculum access
- Outbox write in same transaction as curriculum insert

## Completion Gate

- [ ] Draft graph E2E green
- [ ] Contract operationIds for revisions/graph satisfied
- [ ] Factories support later validation phase


## Dependencies

- Upstream: Phases 4 and 5
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
