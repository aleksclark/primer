# Phase 5: Plan graph and publication consistency

**Depends on:** Phase 4
**Duration guess:** 5–7 days
**Tables:** `curricula`, `plan_revisions`, objectives/outcomes/prereqs/arcs/units/projects/joins/evidence/scheduling/plan_resources

## Goal

Provide transactional repositories for drafting a full plan graph under a revision and publishing it immutably. Enforce published-child locks, outcome prerequisite acyclicity and same-revision constraints, and supersession of prior published revisions—under concurrency.

## BDD Success Criteria

#### Scenario: P5-S1 — Draft graph write/read

- **Given** workspace + curriculum + draft revision
- **When** creating objectives, outcomes, mappings, arcs, units, projects, evidence, constraints, plan_resources in one or more txs
- **Then** a graph load by `plan_revision_id` returns a consistent structure scoped to that revision

#### Scenario: P5-S2 — Publish makes revision immutable

- **Given** a valid draft revision
- **When** `PublishRevision` sets status `published` with `published_at`/`published_by_subject_ref`
- **Then** subsequent updates to revision core fields fail
- **And** inserts/updates/deletes on child plan tables for that revision fail via triggers

#### Scenario: P5-S3 — Supersede path only

- **Given** a published revision
- **When** status changes `published → superseded`
- **Then** it succeeds
- **And** `published → draft` or deleting published revision fails

#### Scenario: P5-S4 — Prerequisite-cycle contention

- **Given** draft outcomes A,B
- **When** concurrent transactions attempt to create opposing prereq edges forming a cycle
- **Then** at most one cycle-creating edge set commits; the other fails
- **And** final graph remains acyclic

#### Scenario: P5-S5 — Same-revision prereq and published child lock

- **Given** outcomes on two different revisions
- **When** inserting a prereq across revisions
- **Then** trigger rejects
- **Given** published revision
- **When** attempting to add an outcome
- **Then** rejected

## Implementation Instructions

1. `CurriculumRepo`, `PlanRevisionRepo`, and either fine-grained graph repos or a `PlanGraphRepo` façade for draft edits.
2. `PublishRevision(ctx, id, subjectRef) error` in a single UoW: validate draft status → update revision → optionally write outbox stub deferred to Phase 11 (if Phase 11 not ready, publish without events but leave TODO hook; prefer landing outbox write interface no-op only if Phase 11 parallel—**must not claim events done**).
3. Map SQL exceptions for immutability and cycles to typed errors.
4. Graph load should not require N+1 catastrophic queries—document acceptable query plan (few queries by table).
5. Workspace isolation on curriculum/revision gets.
6. Concurrency tests using `errgroup` and two pools/connections (not only Tx rollback isolation).
7. Keep parity with CHECK enums in SCHEMA.md.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P5-E1 | workspace+catalog standards | build draft graph | load equals saved |
| P5-E2 | draft graph | publish | child mutation fails; published fields set |
| P5-E3 | published | supersede; illegal transitions | only allowed path works |
| P5-E4 | two connections | concurrent cycle inserts | acyclic end state |
| P5-E5 | two revisions | cross-revision prereq; post-publish child insert | both fail |

```bash
cd curriculum-studio && go test ./internal/repo/... -count=1 -run 'Plan|Curriculum|Publish|Prereq'
cd curriculum-studio/db && python3 -m pytest tests -q -k 'published_revision or outcome_prerequisites or draft_revision'
```

## Anti-Cheating Audit

- Publish immutability must fail at DB even if repo method is bypassed with raw SQL in a test.
- Do not implement publish by copying rows to a side table and leaving draft mutable as “published”.
- Concurrency test must use real concurrent sessions (two connections), not sequential calls.
- No LMS curriculum table access.

## Completion Gate

- [ ] P5-S1–P5-S5 / P5-E1–P5-E5 green
- [ ] Typed errors documented
- [ ] Race/concurrency coverage for cycles
- [ ] Anti-cheating audit clean

## Dependencies and rollback

- **Depends on:** Phase 4 catalogs for mappings/resources
- **Enables:** Phases 6–7, 11 outbox publish events
- **Rollback:** repos only; published data in dev DBs may need reset
