# Phase 5: Resource catalog

**File:** `phase-05-resource-catalog.md`
**Depends on:** Phase 3
**Duration guess:** 2–4 days
**Handoff wave:** see index orchestrator map

## Goal

Implement resource metadata catalog (books, media, tools, URLs) with CRUD+list and workspace isolation. No file blob storage — references and metadata only.

## Scope

### In scope

- resources API per OpenAPI
- kind enum enforced
- server-side list q/sort/filter by kind
- soft constraints on URL format
- attach-ready for plan_resources in Phase 6

### Out of scope / YAGNI

- Object store uploads for source PDFs (deferred content service)
- External ISBN APIs required

## BDD Success Criteria

#### Scenario: P5-S1 — Create resource

- **Given** author in workspace
- **When** POST resource book with title and refs
- **Then** 201 id res_*
- **And** name/title field present
- **And** row workspace scoped

#### Scenario: P5-S2 — List/filter resources

- **Given** mixed kinds
- **When** GET filter kind=book&q=
- **Then** only books
- **And** pagination

#### Scenario: P5-S3 — No blob columns

- **Given** create resource
- **When** inspect DB row / API
- **Then** no byte payload stored
- **And** only URI/metadata fields per schema

## Implementation Instructions

1. Repo + Huma handlers.
2. Delete: restrict if plan_resources references (FK) → 409.
3. Names: `title` primary in list.
4. Tests isolation across workspaces.

## End-to-End Test Plan

#### P5-E1 — Resource CRUD list

- **Setup:** author session
- **Action:** create/list/update/delete
- **Assert:**
  - visibility scoped
  - filter works
- **Command:** `make studio-test`

#### P5-E2 — Blob absence

- **Setup:** create
- **Action:** SELECT * information / API schema
- **Assert:**
  - no content byte field populated
- **Command:** `go test ./internal/resources -run NoBlob`

## Anti-Cheating Audit

- Delete must not leave orphans against RESTRICT FKs silently
- List not client-filtered
- Cross-workspace get returns 404

## Completion Gate

- [ ] P5 scenarios green
- [ ] OpenAPI Resources operations implemented


## Dependencies

- Upstream: Phase 3
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
