# Phase 12: Audit, retention, backup, observability

**Depends on:** Phases 1–11
**Duration guess:** 4–6 days
**Tables:** `audit_events` (+ retention markers if added)

## Goal

Close the operational loop: mutate-path audit trails, retention jobs that respect FK integrity and privacy, backup/restore and PITR drills on disposable Postgres, and observability/Makefile gates so the persistence track is operable in production without handlers or auth providers.

## BDD Success Criteria

#### Scenario: P12-S1 — Audit events for mutations

- **Given** a mutating repo operation (e.g. publish, lock item, membership change)
- **When** the operation commits
- **Then** an `audit_events` row records actor_subject_ref, action, entity, before/after JSON objects
- **And** audit write failure policy is documented (prefer same tx fail-closed for security-sensitive actions)

#### Scenario: P12-S2 — Retention without breaking integrity

- **Given** aged outbox deliveries, idempotency keys, and audit rows beyond retention windows
- **When** retention job runs
- **Then** only eligible rows delete/archive
- **And** active runs/items/published plans are never deleted by default retention
- **And** FK RESTRICT relationships do not leave orphan failures unhandled

#### Scenario: P12-S3 — Backup restore and PITR drill

- **Given** a disposable Postgres with known seed rows
- **When** logical backup (pg_dump) or configured PITR base backup is taken, data mutates, then restore/recover to target
- **Then** seed rows reappear per drill script assertions
- **And** migrate version table matches expected
- **And** drill is documented under `curriculum-studio/db/runbooks/backup-restore.md`

#### Scenario: P12-S4 — Observability and Makefile gates

- **Given** Studio persistence packages
- **When** CI/dev runs Makefile targets
- **Then** `studio-test`, `studio-migrate`, and metrics smoke (pool open connections gauge / outbox lag query) succeed
- **And** documentation lists dashboards/alerts: migrate version, outbox oldest unpublished age, failed webhook count, lease reclaim count

#### Scenario: P12-S5 — External-ref privacy in audit/ops

- **Given** integration snapshots and learner profiles
- **When** audit/log/backup docs are applied
- **Then** runbooks require redaction of sensitive snapshot fields in logs
- **And** export of audit before/after does not require storing credentials (none exist)
- **And** deletion/unlink guidance for `subject_ref` aligns with Identity account pending deletion events (consume later; document stub)

## Implementation Instructions

1. `AuditRepo` + helper `AuditSink` invoked from sensitive repos (membership, publish, lock, webhook secret_ref changes).
2. Retention job CLI `curriculum-studio/cmd/retention` with dry-run.
3. Backup drill script using Docker Postgres—no production credentials in repo.
4. Metrics: expose functions returning outbox lag SQL; wire to OTel meter when service main exists (optional no-op register).
5. Makefile finalize: `studio-test`, `studio-cover` (**≥85%** — repository `COVER_MIN`; never lower), `studio-db-pytest`, `studio-migrate`, `studio-backup-drill` (may be manual tagged). Root Makefile target wires go through delivery wave **F0** ownership.
6. Update `curriculum-studio/README.md` and `SCHEMA.md` ops sections.
7. Confirm Python + Go suites green together.
8. Still out of scope: HTTP handlers, Primer Identity token broker, contract codegen ownership.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P12-E1 | membership change / publish | inspect audit_events | row fields match |
| P12-E2 | aged rows fixture | retention dry-run then apply | counts; protected tables untouched |
| P12-E3 | docker postgres seed | dump → mutate → restore | seed assertion script exit 0 |
| P12-E4 | make studio-test | CI local | green; metrics query returns numeric lag |
| P12-E5 | logs from snapshot upsert test | capture logs | no full raw PII dump patterns; subject_ref format only |

```bash
cd curriculum-studio/db && python3 -m pytest tests -q
cd curriculum-studio && go test ./... -count=1
# drill (disposable):
# curriculum-studio/db/scripts/backup_restore_drill.sh
make studio-test studio-migrate
```

## Anti-Cheating Audit

- Backup drill must restore into a real Postgres and query tables—not compare dump files only.
- Audit tests must read DB rows.
- Retention must not `TRUNCATE materialization_runs` as a “cleanup.”
- Makefile targets must invoke real tests, not `exit 0` stubs.
- Privacy scenario must fail if logs contain planted sensitive email@ example from snapshot at info level.

## Completion Gate

- [ ] P12-S1–P12-S5 / P12-E1–P12-E5 green
- [ ] Runbooks committed
- [ ] Makefile gates documented and green locally
- [ ] Full traceability matrix satisfied for REQ-AUD/OPS/PRIV
- [ ] Anti-cheating audit clean for this phase and spot-check prior phases
- [ ] Plan completion rule in index.md satisfied for persistence track

## Dependencies and rollback

- **Depends on:** all prior phases
- **Rollback:** ops scripts only; data retention is forward-only
- **Hands off to:** API/worker tracks for continuous metrics export in process mains
