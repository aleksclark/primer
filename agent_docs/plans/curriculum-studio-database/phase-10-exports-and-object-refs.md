# Phase 10: Exports and object refs

**Depends on:** Phase 9
**Duration guess:** 2–3 days
**Tables:** `exports` (+ resources.artifact_ref already in Phase 4)

## Goal

Persist export jobs and their object-store references/checksums without storing file bytes in PostgreSQL. Model status transitions `requested → ready|failed` and workspace-scoped listing.

## BDD Success Criteria

#### Scenario: P10-S1 — Export requested then ready with ref

- **Given** workspace and optional run/revision
- **When** creating export format `pdf` status `requested`
- **And** completer sets `artifact_ref`, `checksum`, status `ready`, `completed_at`
- **Then** row round-trips; bytes are not in DB columns

#### Scenario: P10-S2 — Failed export path

- **Given** requested export
- **When** completer marks `failed`
- **Then** status stored; artifact_ref may remain empty
- **And** illegal format values rejected by CHECK

## Implementation Instructions

1. `ExportRepo` Create/Get/List/Complete/Fail.
2. Define `artifact_ref` convention string (e.g. `s3://bucket/key` or `studio-obj:<uuid>`) in docs—no vendor SDK required in this phase.
3. Checksum field stores hex digest of object bytes computed by export worker (worker itself may be stub writing ref).
4. Workspace isolation mandatory.
5. Do not embed PDF binaries in JSONB.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P10-E1 | workspace+run | request+complete export | ref+checksum+ready |
| P10-E2 | export | fail path + bad format | status/CHECK |

```bash
cd curriculum-studio && go test ./internal/repo/... -count=1 -run Export
```

## Anti-Cheating Audit

- Assert table columns via information_schema: no `bytea` content column on exports.
- Ready status without artifact_ref should be rejected by app rule (add CHECK in additive migration if product requires).
- No reads from LMS artifact tables.

## Completion Gate

- [ ] P10-S1–P10-S2 / P10-E1–P10-E2 green
- [ ] artifact_ref convention documented
- [ ] Anti-cheating audit clean

## Dependencies and rollback

- **Depends on:** Phase 9 (and Phase 5 for revision-only exports)
- **Peer:** object store implementation plan for actual uploads
