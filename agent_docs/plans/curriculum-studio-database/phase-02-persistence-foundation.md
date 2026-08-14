# Phase 2: Persistence foundation

**Depends on:** Phase 1
**Duration guess:** 2–3 days
**Primary paths:** `curriculum-studio/internal/db`, `curriculum-studio/internal/repo`, `curriculum-studio/internal/testutil`

## Goal

Provide the shared Go persistence substrate every later phase builds on: pgx pool connect/ping, `Querier` abstraction, `WithTx` unit-of-work, repository factory, and a PostgreSQL testcontainers harness with per-test transaction rollback. Explicitly forbid in-memory production repositories.

## BDD Success Criteria

#### Scenario: P2-S1 — Pool connects and pings Studio DSN only

- **Given** a migrated Studio database
- **When** `db.Connect(ctx, studioURL)` runs
- **Then** ping succeeds and pool stats are available
- **And** closing the pool releases connections

#### Scenario: P2-S2 — WithTx commits and rolls back

- **Given** a pool Querier
- **When** `WithTx` runs a function that inserts a tenant row and returns nil
- **Then** the row is visible after commit
- **When** `WithTx` runs a function that inserts then returns an error
- **Then** the insert is not visible (full rollback)

#### Scenario: P2-S3 — Test harness isolates tests

- **Given** the Studio test harness
- **When** two tests insert conflicting unique slugs on rolled-back transactions
- **Then** both pass without cross-test pollution
- **And** harness migrates via Studio migrator (Phase 1), not LMS

#### Scenario: P2-S4 — Repository factory constructs domain repos

- **Given** a `Querier`
- **When** `repo.NewFactory(q)` is created
- **Then** it exposes constructors (or fields) for upcoming repos (tenants, plans, …) even if some return `ErrNotImplemented` stubs **only if** tests for those stubs are quarantined—prefer empty interfaces wired as nil-safe placeholders documented as Phase 3+

#### Scenario: P2-S5 — No in-memory production substitute

- **Given** production build tags / default factory
- **When** inspectors search for `map[` backed repository types registered as default
- **Then** none exist for durable entities
- **And** README states Postgres is mandatory for Studio service

## Implementation Instructions

1. Mirror `server/internal/repo/querier.go` and `tx.go` under Studio module (`Querier`, `TxBeginner`, `WithTx`).
2. Mirror `server/internal/testutil/db.go` Harness with `Migrate: studiodb.Migrate`, `DBName: "curriculum_studio_test"`.
3. Support `TEST_DATABASE_URL` override like LMS and Python suite.
4. `Connect` reuses Phase 1 helper; configure pool max conns from Studio config.
5. Factory pattern: `type Factory struct { Q Querier }` with methods added per phase; this phase lands the type + `Ping` health repo optional.
6. Add `go test` race job for `internal/repo` and `internal/db`.
7. Document developer loop in `curriculum-studio/README.md`.
8. Do not implement domain SQL yet beyond a smoke `SELECT 1` / optional tenant insert used only in foundation tests (prefer using Phase 3 tables only after Phase 3—foundation tests may insert into `tenants` since 00001 exists post-migrate).

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P2-E1 | harness DB | Connect+Ping | success; invalid URL fails |
| P2-E2 | harness | WithTx commit insert tenant; WithTx error insert | row present / absent |
| P2-E3 | parallel `t.Parallel` tests with Tx() | unique constraint ops | isolation via rollback |
| P2-E4 | NewFactory | method set non-nil factory | smoke |
| P2-E5 | static check `rg` in CI or Go test | ban list `type MemoryTenantRepo` in non-test | zero matches in prod packages |

```bash
cd curriculum-studio && go test ./internal/db/... ./internal/repo/... ./internal/testutil/... -count=1
cd curriculum-studio && go test ./internal/repo/... -race -count=1
```

## Anti-Cheating Audit

- Tests must hit real Postgres (skip only if no Docker and no `TEST_DATABASE_URL`, and CI must provide one).
- `WithTx` tests must assert durable visibility via a second connection/query, not only mock expectations.
- Factory must not swap to memory when `ENV=test` for production packages.
- Do not wrap LMS `testutil` if that forces LMS migrations.

## Completion Gate

- [ ] P2-S1–P2-S5 pass
- [ ] P2-E1–P2-E5 green
- [ ] Race detector clean on foundation packages
- [ ] README documents harness
- [ ] Anti-cheating audit clean

## Dependencies and rollback

- **Depends on:** Phase 1 migrator + schema
- **Rollback:** remove repo/testutil packages; keep migrator
- **Forward:** Phase 3 fills factory methods
