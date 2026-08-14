# Phase 8: Workflow checkpointing and fencing

**Depends on:** Phase 7
**Duration guess:** 4–5 days
**Tables:** `workflow_stages`, `workflow_attempts` (+ optional additive lease columns)

## Goal

Make materialization workflows resumable and multi-worker safe: persist stages and attempts, checkpoint inputs/outputs, retry failed stages, reclaim stale work with fencing tokens so a restarted or slow worker cannot clobber a newer owner.

## BDD Success Criteria

#### Scenario: P8-S1 — Stage checkpoint persistence

- **Given** a materialization run
- **When** creating ordered stages with `stage_key`, `position`, status `pending`
- **Then** unique `(run_id, stage_key)` and `(run_id, position)` hold
- **And** updating stage `output` JSON checkpoints durable state

#### Scenario: P8-S2 — Resume from failed stage

- **Given** stages 1 succeeded, 2 failed, 3 pending
- **When** worker resumes run
- **Then** it loads stage 2 as next work without resetting stage 1 output

#### Scenario: P8-S3 — Retry increments attempts

- **Given** a failed stage
- **When** starting a new attempt
- **Then** `workflow_attempts.attempt_number` increments
- **And** prior attempts remain queryable

#### Scenario: P8-S4 — Stale lease writer loses

- **Given** worker A claimed stage with fence token / lease
- **When** lease expires and worker B reclaims with new fence
- **Then** worker A’s subsequent checkpoint with old fence is rejected
- **And** only B’s writes apply

#### Scenario: P8-S5 — Restart recovery mid-stage

- **Given** a stage status `running` with an open attempt and no completion
- **When** process restarts and reclaim policy runs
- **Then** stage becomes claimable after timeout
- **And** no duplicate succeeded attempts without explicit policy

## Implementation Instructions

1. Inspect schema: stages/attempts exist; **lease columns may be missing**. If missing, additive migration e.g. `00005_workflow_leases.sql`:
   - `lease_owner TEXT`, `lease_expires_at TIMESTAMPTZ`, `fence_token BIGINT` on `workflow_stages` (or attempts)
2. `WorkflowRepo` methods: `EnsureStages`, `ClaimStage`, `Checkpoint`, `CompleteAttempt`, `FailAttempt`, `ReclaimExpired`.
3. Claim uses single UPDATE … WHERE status IN (…) AND (lease_expires_at IS NULL OR lease_expires_at < now()) RETURNING with incremented fence.
4. All mutations compare fence token.
5. Time source injectable for tests.
6. Extend Python tests for new columns/constraints if migration added.
7. Out of scope: actual LLM calls—workers are test doubles writing checkpoints.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P8-E1 | run | ensure stages + checkpoint | durable output |
| P8-E2 | partial success | resume loader | starts at failed/pending |
| P8-E3 | fail then retry | attempts table | numbers 1..n |
| P8-E4 | two workers | stale write after reclaim | old fence error; final output = B |
| P8-E5 | crash fixture running stage | reclaim after TTL | claimable; recovery path |

```bash
cd curriculum-studio && go test ./internal/repo/... -count=1 -race -run 'Workflow|Lease|Fence'
cd curriculum-studio/db && python3 -m pytest tests -q
```

## Anti-Cheating Audit

- Fencing must be enforced in SQL WHERE clause, not only in process memory maps.
- Tests must use two connections/workers.
- Do not “recover” by deleting stage history.
- TTL reclaim must not succeed before expiry (fake clock ok in test).

## Completion Gate

- [ ] P8-S1–P8-S5 / P8-E1–P8-E5 green
- [ ] Migration for leases if needed + policy doc
- [ ] Race detector clean on claim tests
- [ ] Anti-cheating audit clean

## Dependencies and rollback

- **Depends on:** Phase 7 runs
- **Enables:** Phase 9 items produced by stages
- **Rollback:** non-live down of lease migration; never on live without forward fix
