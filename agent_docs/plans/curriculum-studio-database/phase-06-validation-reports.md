# Phase 6: Validation reports

**Depends on:** Phase 5
**Duration guess:** 1–2 days
**Tables:** `validation_reports`, `validation_findings`

## Goal

Persist deterministic validation outputs (coverage, graph, evidence, workload) as reports and findings attached to plan revisions, without claiming the validation *engine* is complete—only durable storage and query APIs the engine and API layers will use.

## BDD Success Criteria

#### Scenario: P6-S1 — Store passed/failed report with findings

- **Given** a plan revision
- **When** saving a report `status=failed` with multiple findings (severities error/warning/info)
- **Then** report and findings round-trip ordered by severity/stable id
- **And** workspace scoping is preserved via revision ownership

#### Scenario: P6-S2 — Report history per revision

- **Given** two validation runs on the same revision
- **When** listing reports by revision
- **Then** both appear with timestamps
- **And** latest helper returns the newest

## Implementation Instructions

1. `ValidationReportRepo.CreateReportWithFindings` in one transaction.
2. Status/severity CHECKs aligned with SCHEMA.md.
3. Do not implement full coverage algorithms here—accept structured finding inputs.
4. Optional: link publish gate interface `AssertPublishable(revisionID)` that checks latest report status—only if product wants DB-layer gate; otherwise leave to domain service in API track. Prefer repo method `Latest(ctx, revisionID)` only.
5. Tenant isolation via join to curricula/workspaces.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P6-E1 | draft revision | insert failed report+findings | round-trip |
| P6-E2 | two reports | list/latest | order correct; wrong workspace cannot read |

```bash
cd curriculum-studio && go test ./internal/repo/... -count=1 -run Validation
```

## Anti-Cheating Audit

- Findings must be in Postgres, not returned only from an in-memory validator cache.
- Do not mark publish immutable based on fake always-pass reports in tests without inserting rows.

## Completion Gate

- [ ] P6-S1–P6-S2 / P6-E1–P6-E2 green
- [ ] Anti-cheating audit clean

## Dependencies and rollback

- **Depends on:** Phase 5 revisions
- **Parallel with:** Phase 7
- **Rollback:** drop validation repo
