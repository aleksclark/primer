# Phase 7: Learner snapshots and materialization runs

**Depends on:** Phase 5 (published or draft revision refs as allowed by FK)
**Duration guess:** 3–4 days
**Tables:** `learner_profiles`, `materialization_runs`

## Goal

Persist generic learner/class profiles and materialization runs that freeze complete `input_snapshot` JSON plus durable `input_fingerprint`. Implement fingerprint-based idempotency so duplicate requests do not create divergent side effects.

## BDD Success Criteria

#### Scenario: P7-S1 — Learner and class profiles

- **Given** a workspace
- **When** creating profiles kind `learner` and `class` with profile JSON objects
- **Then** rows persist; optional `integration_identity_id` links allowed
- **And** profile must be a JSON object (CHECK)

#### Scenario: P7-S2 — Run records complete snapshot and fingerprint

- **Given** workspace + plan_revision + optional learner profile
- **When** creating a run with non-empty fingerprint and object snapshot
- **Then** status defaults `requested`; indexes support lookup by `(plan_revision_id, input_fingerprint)`
- **And** empty fingerprint rejected

#### Scenario: P7-S3 — Fingerprint idempotency

- **Given** an existing run for `(plan_revision_id, input_fingerprint)`
- **When** `CreateRunIdempotent` is called with the same pair and equivalent snapshot hash
- **Then** the existing run id is returned without creating a second active divergent run
- **And** concurrent creators result in a single logical run (unique policy or transactional upsert)—document chosen uniqueness (unique constraint additive migration if required)

## Implementation Instructions

1. `LearnerProfileRepo`, `MaterializationRunRepo`.
2. Fingerprint helper: canonical JSON hash (sha256) in pure Go (T2 unit tests).
3. Idempotency policy (D11): prefer application-level “select then insert” under tx with `UNIQUE` reinforcement—if schema lacks UNIQUE on `(plan_revision_id, input_fingerprint)`, add additive migration `00005_materialization_fingerprint_unique.sql` **or** partial unique for open statuses; record decision in SCHEMA.md.
4. Status transitions `requested→running→ready|failed|cancelled` as repo methods with legal transition checks (app-level; DB CHECK is membership only).
5. Workspace isolation on all gets.
6. External-ref privacy: snapshots may include LMS mastery summaries; logging redacts raw snapshot bodies.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P7-E1 | workspace | create learner/class profiles | CHECKs; list scoped |
| P7-E2 | published revision | create run | snapshot+fingerprint stored; window check |
| P7-E3 | existing run | parallel idempotent creates | one row / same id; no double side effects |

```bash
cd curriculum-studio && go test ./internal/repo/... ./internal/fingerprint/... -count=1 -run 'Learner|Materialization|Fingerprint'
```

## Anti-Cheating Audit

- Idempotency must be proven with two concurrent goroutines and DB row count assertions.
- Snapshot must be persisted (`SELECT input_snapshot`), not recomputed empty.
- Do not store primer learner as UUID FK to LMS.

## Completion Gate

- [ ] P7-S1–P7-S3 / P7-E1–P7-E3 green
- [ ] Fingerprint helper unit tests
- [ ] SCHEMA updated if unique constraint added
- [ ] Anti-cheating audit clean

## Dependencies and rollback

- **Depends on:** Phase 5
- **Enables:** Phases 8–10
- **Rollback:** repo + any 00005 carefully on non-live only
