# Phase 13: Exports and artifacts

**File:** `phase-13-exports-artifacts.md`
**Depends on:** Phases 11–12
**Duration guess:** 4–6 days
**Handoff wave:** see index orchestrator map

## Goal

Harden artifact object store and full export formats (PDF, Markdown, DOCX, CSV coverage, JSON bundle, iCal) with provenance linking exports to plan revision and materialization run. Object store failures fail the export visibly.

## Scope

### In scope

- `internal/artifacts` Store interface: Put/Get/Delete/SignURL
- FsStore + S3Store (MinIO in tests)
- export service all formats in SCHEMA enum
- CSV coverage from validation report
- JSON export bundle subset for download (not Primer gRPC)
- Provenance metadata file sidecar or export row fields
- Authz on download

### Out of scope / YAGNI

- LMS import push
- CDN public anonymous artifacts

## BDD Success Criteria

#### Scenario: P13-S1 — Bytes in object store

- **Given** export ready
- **When** inspect storage backend and DB
- **Then** object exists at artifact_uri/key
- **And** DB has ref not bytes

#### Scenario: P13-S2 — Formats

- **Given** published plan with items
- **When** request each format pdf/markdown/docx/csv/json/ical
- **Then** each reaches ready
- **And** content-type/magic reasonable

#### Scenario: P13-S3 — Provenance

- **Given** export from run R revision P
- **When** read export metadata/manifest
- **Then** includes plan_revision_id
- **And** materialization_run_id when applicable
- **And** created_by subject_ref

#### Scenario: P13-S4 — Store failure

- **Given** store forced error
- **When** create export
- **Then** export status failed
- **And** error surfaced to API
- **And** no ready false positive

## Implementation Instructions

1. Configure `STUDIO_ARTIFACT_STORE=fs|s3` + bucket/endpoint.
2. MinIO testcontainer or local fs under temp dir.
3. DOCX via library (e.g. bridged template); iCal minimal VEVENT from schedule constraints.
4. JSON bundle must not claim full protobuf parity; link to integration for Primer.
5. Download path authorizes workspace membership.

## End-to-End Test Plan

#### P13-E1 — FS store roundtrip

- **Setup:** fs store
- **Action:** put via export
- **Assert:**
  - get bytes equal
- **Command:** `make studio-test`

#### P13-E2 — All formats

- **Setup:** fixture plan
- **Action:** create 6 exports
- **Assert:**
  - all ready
- **Command:** `go test ./internal/export -run Formats`

#### P13-E3 — Provenance manifest

- **Setup:** export
- **Action:** read manifest
- **Assert:**
  - ids match
- **Command:** `go test ./internal/export -run Provenance`

#### P13-E4 — Failure path

- **Setup:** broken store
- **Action:** export
- **Assert:**
  - failed status
- **Command:** `go test ./internal/export -run StoreFail`

## Anti-Cheating Audit

- Exports not written only to /tmp outside interface
- S3 path tested at least with MinIO or skipped with build tag documented — prefer real MinIO
- Provenance not omitted
- Authorization on GET download

## Completion Gate

- [ ] Format enum coverage
- [ ] P13 tests green
- [ ] SPA export menu updated if needed


## Dependencies

- Upstream: Phases 11–12
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
