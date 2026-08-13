# Phase 4: Catalogs and resources

**Depends on:** Phase 3
**Duration guess:** 3–4 days
**Tables:** `standard_frameworks`, `catalog_standards`, `standard_crosswalks`, `catalog_standard_prerequisites`, `resources`

## Goal

Persist standards catalogs (global and workspace-owned), hierarchical standards, crosswalks, acyclic catalog prerequisites, and resource metadata with artifact refs only (no file bytes in Postgres).

## BDD Success Criteria

#### Scenario: P4-S1 — Global and workspace frameworks

- **Given** tenant/workspace
- **When** creating a global framework (`workspace_id` NULL) code `TN-MATH` and a workspace-custom framework
- **Then** unique indexes enforce global code uniqueness and per-workspace code uniqueness

#### Scenario: P4-S2 — Hierarchical catalog standards

- **Given** a framework
- **When** inserting parent and child standards with codes
- **Then** parent_id links work; unique `(framework_id, code)` enforced

#### Scenario: P4-S3 — Catalog prerequisite cycle rejected

- **Given** standards A→B prerequisite edge
- **When** inserting B→A
- **Then** DB trigger rejects the cycle (repo surfaces error)
- **And** concurrent conflicting inserts cannot leave a cycle committed

#### Scenario: P4-S4 — Resource metadata without bytes

- **Given** tenant/workspace
- **When** creating resource kind `book` with title and optional `artifact_ref`
- **Then** row persists; body/file bytes columns do not exist
- **And** kind CHECK enforced

## Implementation Instructions

1. Repos: `FrameworkRepo`, `CatalogStandardRepo`, `CrosswalkRepo`, `CatalogPrereqRepo`, `ResourceRepo`.
2. Batch helpers for importing a small standards set inside one `WithTx`.
3. Map trigger errors to typed `ErrPrerequisiteCycle`.
4. Resource repo forbids giant byte payloads—only text refs/metadata JSON.
5. Workspace isolation: workspace frameworks/resources not visible across workspaces; global frameworks readable by all (document policy).
6. Extend Python tests only if new migrations added; else rely on existing cycle test + Go coverage.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P4-E1 | harness+workspace | create global+ws frameworks | uniqueness conflicts |
| P4-E2 | framework | parent/child standards | tree read API |
| P4-E3 | two standards | insert cycle edges | second fails; concurrent stress optional |
| P4-E4 | tenant | create resources kinds | CHECK + list by workspace |

```bash
cd curriculum-studio && go test ./internal/repo/... -count=1 -run 'Framework|Catalog|Resource|Crosswalk|Prereq'
cd curriculum-studio/db && python3 -m pytest tests -q -k catalog_prerequisites
```

## Anti-Cheating Audit

- Cycle protection must be DB trigger (already in 00004), not only app validation—tests should insert via SQL/repo and see DB error even if app validation is bypassed in a white-box test.
- No LMS `standards` table reads.
- Resources must not base64-encode file content into JSONB as a bypass.

## Completion Gate

- [ ] P4-S1–P4-S4 / P4-E1–P4-E4 green
- [ ] Typed cycle error
- [ ] Anti-cheating audit clean

## Dependencies and rollback

- **Depends on:** Phase 3
- **Enables:** Phase 5 outcome↔standard mappings and plan_resources
