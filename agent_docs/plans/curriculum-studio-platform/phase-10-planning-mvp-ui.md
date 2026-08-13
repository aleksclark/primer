# Phase 10: Planning MVP UI

**File:** `phase-10-planning-mvp-ui.md`
**Depends on:** Phases 8 and 9 (and Phase 5 resources for attachments)
**Duration guess:** 8–12 days
**Handoff wave:** see index orchestrator map

## Goal

Deliver the useful planning MVP UX: curriculum brief, objectives/outcomes review, standards map, scope-and-sequence editor, coverage report, publish action, and Markdown/PDF export — all through real APIs and browser E2E. Completes product roadmap Phase 1 standalone value.

## Scope

### In scope

- Pages: curriculum list/create (Explore), curriculum detail (Inspect), revision editor (Configure), standards picker, sequence board/list, validation report panel (Monitor/Decide hybrid → primary Inspect+Configure), export dialog
- Server-driven lists everywhere
- Names-first tables and breadcrumbs
- Wire publish + validate buttons to APIs
- Export Markdown+PDF via export API (minimal generator allowed here if Phase 13 not done — implement file export service subset)
- Browser E2E full MVP path
- Empty/loading/error/denied states

### Out of scope / YAGNI

- Lesson materialization UX (later)
- DOCX/iCal full suite (Phase 13 can extend)
- Collab comments (Phase 17)

## BDD Success Criteria

#### Scenario: P10-S1 — Brief to outcomes

- **Given** authenticated author in browser
- **When** creates curriculum from brief form and generates/adds objectives and outcomes (manual and/or scripted agent stub endpoint if exposed)
- **Then** curriculum name visible as title
- **And** outcomes list shows outcome titles
- **And** data persists after reload

#### Scenario: P10-S2 — Standards map UX

- **Given** draft with outcomes + imported standards
- **When** user maps standards to outcomes in UI
- **Then** mapping saved via API
- **And** codes+titles shown
- **And** server validation reflects mappings

#### Scenario: P10-S3 — Scope and sequence editor

- **Given** mapped outcomes
- **When** user groups into arcs/units in UI
- **Then** graph GET matches UI
- **And** prerequisite warnings surface from validation

#### Scenario: P10-S4 — Coverage report UI

- **Given** plan with known gap
- **When** user opens coverage/validation
- **Then** finding messages visible
- **And** severity indicated by icon+text not color alone

#### Scenario: P10-S5 — Markdown and PDF export

- **Given** published or draft allowed policy (document: draft export ok watermarked)
- **When** user exports MD and PDF
- **Then** download bytes non-empty
- **And** export row status ready
- **And** artifact ref set

#### Scenario: P10-S6 — Server list ops in UI

- **Given** many curricula
- **When** user searches/sorts/pages
- **Then** network shows q/sort/limit params
- **And** names primary column
- **And** IDs secondary

## Implementation Instructions

1. Feature folders under `web/src/pages/...` with house composition rules.
2. If outcome generation agent not ready, allow manual entry + optional `STUDIO_ENABLE_SCRIPT_GENERATE=1` stub that writes deterministic outcomes (not live LLM).
3. Implement minimal `internal/export` Markdown renderer + PDF (go fpdf or chromedp later — prefer pure Go MD→PDF library); store via artifact interface (filesystem).
4. Playwright journey `e2e/planning-mvp.spec.ts`.
5. Ensure CSRF on all mutations from SPA.
6. Coverage: axe on editor critical path.

## End-to-End Test Plan

#### P10-E1 — MVP journey browser

- **Setup:** full stack + fixtures standards
- **Action:** brief→outcomes→map→sequence→validate→publish→export
- **Assert:**
  - each step UI+DB evidence
  - publish immutable
- **Command:** `make studio-e2e`

#### P10-E2 — Standards+sequence

- **Setup:** browser
- **Action:** map and group
- **Assert:**
  - API graph match
- **Command:** `make studio-e2e --grep sequence`

#### P10-E3 — Coverage UI

- **Setup:** gapped plan
- **Action:** open report
- **Assert:**
  - finding visible
- **Command:** `make studio-e2e --grep coverage`

#### P10-E4 — Export downloads

- **Setup:** publish
- **Action:** export md/pdf
- **Assert:**
  - magic bytes PDF %PDF
  - md contains curriculum name
- **Command:** `make studio-e2e --grep export`

#### P10-E5 — List query network

- **Setup:** ≥25 curricula
- **Action:** search/page
- **Assert:**
  - request URLs contain q/limit
  - DOM names
- **Command:** `make studio-e2e --grep lists`

## Anti-Cheating Audit

- UI must not keep authoritative graph only in React state without PATCH
- Export not fake empty blob
- Publish button must call server publish not local flag
- No client-side filter of full curriculum list
- Scripted generate clearly labeled not “AI quality”

## Completion Gate

- [ ] Product roadmap Phase 1 capabilities observable in browser
- [ ] P10-E1 green on CI-local
- [ ] House visual review screenshots attached to PR
- [ ] Anti-cheat clean


## Dependencies

- Upstream: Phases 8 and 9 (and Phase 5 resources for attachments)
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
