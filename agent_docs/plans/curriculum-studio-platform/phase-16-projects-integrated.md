# Phase 16: Projects integrated

**File:** `phase-16-projects-integrated.md`
**Depends on:** Phases 12–13
**Duration guess:** 6–8 days
**Handoff wave:** see index orchestrator map

## Goal

Extend plan/materialization for multi-subject projects: project blueprints, phases, project-phase materialization, off-screen activities, tool requirements, portfolio evidence specs, and cross-domain reinforcement planning — product roadmap Phase 4.

## Scope

### In scope

- Richer projects API/graph nodes (phases JSON or tables per schema)
- Materialize by project phase window
- Resource kind tool/project_supply requirements on projects
- Evidence kind portfolio/performance
- UI surfaces for project designer (Configure) + phase operate
- Validation rules for project outcome coverage
- Reading/media scheduling hooks via plan_resources + constraints

### Out of scope / YAGNI

- Physical logistics/shipping
- Onshape live integration

## BDD Success Criteria

#### Scenario: P16-S1 — Multi-subject project blueprint

- **Given** draft revision
- **When** create project spanning math+science outcomes
- **Then** project saved with name
- **And** phase list ordered
- **And** outcome roles target/prior/stretch

#### Scenario: P16-S2 — Phase materialization

- **Given** published project plan
- **When** materialize phase design
- **Then** items scoped to phase
- **And** run snapshot includes phase id

#### Scenario: P16-S3 — Off-screen + tools

- **Given** project with off-screen activity and tool resource
- **When** validate+materialize
- **Then** activity item kind project_task
- **And** tool requirement listed in item/tutor or teacher_guide

#### Scenario: P16-S4 — Portfolio evidence + reinforcement

- **Given** outcomes with portfolio evidence; reinforcement flags
- **When** validate/materialize
- **Then** evidence requirements present
- **And** reinforcement notes in plan/items without writing LMS mastery

## Implementation Instructions

1. Extend graph editor UI for projects.
2. Ensure schema fields sufficient; additive migrations via db track if gaps.
3. Scripted fixtures for project workflows.
4. Browser E2E path for project create→publish→materialize phase.

## End-to-End Test Plan

#### P16-E1 — Project blueprint API+UI

- **Setup:** stack
- **Action:** create project multi-subject
- **Assert:**
  - persist
  - names
- **Command:** `make studio-e2e --grep project`

#### P16-E2 — Phase mat

- **Setup:** published
- **Action:** materialize phase
- **Assert:**
  - snapshot phase
  - items
- **Command:** `make studio-test`

#### P16-E3 — Tools/off-screen

- **Setup:** fixture
- **Action:** mat
- **Assert:**
  - task+tool
- **Command:** `go test ./internal/plan -run ProjectTools`

#### P16-E4 — Portfolio evidence

- **Setup:** fixture
- **Action:** validate
- **Assert:**
  - findings/requirements
- **Command:** `go test ./internal/validation -run Portfolio`

## Anti-Cheating Audit

- Projects not only markdown notes without tables
- Must not write mastery to LMS
- Phase materialization not ignore locks

## Completion Gate

- [ ] Roadmap Phase 4 capabilities evidenced
- [ ] P16 tests green


## Dependencies

- Upstream: Phases 12–13
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
