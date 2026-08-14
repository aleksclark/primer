# Phase 4: Standards catalog

**File:** `phase-04-standards-catalog.md`
**Depends on:** Phase 3
**Duration guess:** 4–6 days
**Handoff wave:** see index orchestrator map

## Goal

Implement standards catalog module: import frameworks, hierarchical catalog standards, crosswalks, and catalog-level prerequisite DAG with acyclicity enforced (DB triggers + API errors). Enables standards mapping for plan drafts.

## Scope

### In scope

- `internal/standards` + API routes per OpenAPI Standards tags
- importStandardsCatalog accepts structured JSON/YAML payload of framework + standards
- list/get catalogs and standards with server q/filter (frameworkId, parentId, code)
- crosswalk create/list
- prerequisite edges with cycle rejection
- Shared vs workspace-owned frameworks per schema
- Human-readable standard titles/codes in list primary fields

### Out of scope / YAGNI

- Automatic scrape of external standards websites
- LMS standards table sync
- AI mapping (later workflow)

## BDD Success Criteria

#### Scenario: P4-S1 — Import framework

- **Given** workspace author authenticated
- **When** POST import with TN Grade 6 Math sample catalog
- **Then** framework id returned with name
- **And** standards rows inserted hierarchical
- **And** re-import idempotent or versioned per documented policy

#### Scenario: P4-S2 — Import validation failure

- **Given** payload missing required code/title
- **When** import
- **Then** 400 with field errors
- **And** no partial dirty framework without transaction rollback

#### Scenario: P4-S3 — List standards server filters

- **Given** imported catalog with many standards
- **When** GET list with q and parent filter limit
- **Then** bounded page
- **And** titles/codes present
- **And** SQL filter

#### Scenario: P4-S4 — Crosswalk mapping

- **Given** two frameworks
- **When** create equivalent crosswalk
- **Then** persisted relationship enum
- **And** list returns both sides names/codes

#### Scenario: P4-S5 — Reject cyclic catalog prerequisites

- **Given** standards A,B
- **When** add prereq A→B and B→A
- **Then** second edge rejected
- **And** DB trigger or app error; no cycle stored

## Implementation Instructions

1. Define import DTO aligned to contract; if contract gaps, sibling PR first.
2. Transactional import; stable codes unique per framework.
3. Repo queries recursive parent path optional for breadcrumbs.
4. Map API errors for cycle to 409/400 with stable error code.
5. Seed fixture `testdata/standards/tn-math-g6-sample.json` for E2E.
6. Authorization: workspace-owned catalogs require membership; shared global readable to all authenticated authors (document).

## End-to-End Test Plan

#### P4-E1 — Import sample catalog

- **Setup:** author session
- **Action:** import fixture
- **Assert:**
  - counts match fixture
  - get catalog by id
- **Command:** `make studio-test`

#### P4-E2 — List pagination/filter

- **Setup:** large fixture
- **Action:** page through
- **Assert:**
  - no full dump
  - q on code/title
- **Command:** `go test ./internal/standards -run List`

#### P4-E3 — Cycle + crosswalk

- **Setup:** two standards
- **Action:** cycle attempt + crosswalk
- **Assert:**
  - cycle fail
  - crosswalk ok
- **Command:** `go test ./internal/standards -run Graph`

## Anti-Cheating Audit

- Import not “success” without DB rows
- Cycle check not client-only
- Shared catalog cannot be mutated by random workspace if policy forbids
- Titles not dropped in favor of only codes

## Completion Gate

- [ ] P4-S*/E* green
- [ ] Fixture committed
- [ ] Contract validate still green if touched


## Dependencies

- Upstream: Phase 3
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
