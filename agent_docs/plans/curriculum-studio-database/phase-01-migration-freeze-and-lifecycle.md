# Phase 1: Migration freeze and lifecycle

**Depends on:** None
**Duration guess:** 2–4 days
**Primary paths:** `curriculum-studio/db/migrations/`, new `curriculum-studio/internal/db/`, `curriculum-studio/cmd/migrate/`, Makefile targets

## Goal

Establish Curriculum Studio as an independently migratable PostgreSQL schema owner: freeze the design-time four-migration baseline under an explicit immutability policy, ship a Studio-owned goose migrator using version table `studio_goose_db_version`, and provide dedicated DB configuration that cannot accidentally point at LMS/TV databases. After this phase, empty databases can be brought to the documented schema inventory and non-live environments can exercise controlled down migrations.

## BDD Success Criteria

#### Scenario: P1-S1 — Fresh database migrates to full schema inventory

- **Given** an empty PostgreSQL database and Studio migrate configured with DSN `STUDIO_DATABASE_URL`
- **When** an operator runs Studio migrate `up`
- **Then** goose records versions in `studio_goose_db_version` (not `goose_db_version` used by LMS default)
- **And** schema `curriculum_studio` contains every table listed in `curriculum-studio/db/SCHEMA.md`
- **And** no tables are created in `public` for Studio domain entities

#### Scenario: P1-S2 — Migrator is Studio-owned and isolated

- **Given** LMS and TV migrate tooling in `server/cmd/migrate`
- **When** Studio migrations are applied
- **Then** they are driven by a Studio-owned entrypoint under `curriculum-studio/` (binary or `go run ./curriculum-studio/cmd/migrate`)
- **And** Studio does not require the LMS binary or LMS config package to migrate

#### Scenario: P1-S3 — Baseline freeze inventory is recorded

- **Given** migrations `00001`–`00004` at plan base commit
- **When** the freeze gate runs
- **Then** a checked-in inventory lists each file name, sha256, and “baseline immutable after live” flag
- **And** CI fails if a frozen baseline file’s bytes change after the live marker exists

#### Scenario: P1-S4 — No casual rewrite after live

- **Given** an environment marked live (marker file or config `STUDIO_MIGRATIONS_LIVE=true` in ops docs)
- **When** a developer attempts to edit bytes of an already-applied baseline migration
- **Then** the freeze checker rejects the change
- **And** the only allowed path is a new numbered migration `00005+`

#### Scenario: P1-S5 — Down policy differs by environment class

- **Given** a disposable non-live database at head
- **When** migrate `down` is invoked once
- **Then** exactly one goose version rolls back and Down SQL executes
- **Given** a live-classified environment
- **When** migrate `down` is requested
- **Then** the tool refuses destructive down (exit non-zero) unless an explicit break-glass flag documented in the runbook is set

#### Scenario: P1-S6 — DSN isolation from LMS/TV

- **Given** process environment containing LMS/TV database URLs
- **When** Studio config loads
- **Then** it reads only Studio-specific config (`STUDIO_DATABASE_URL` or equivalent)
- **And** refusing to start migrate if the DSN database name equals known LMS/TV names when a guard list is configured

## Implementation Instructions

1. **Choose module layout (resolve B1):** prefer `curriculum-studio/go.mod` module (e.g. `github.com/aleksclark/primer/curriculum-studio`) so Studio does not import `server/internal`. Record choice in `curriculum-studio/db/SCHEMA.md` or README.
2. **Port migrator pattern** from `server/internal/db/db.go`:
   - `//go:embed migrations/*.sql` pointing at `curriculum-studio/db/migrations` **or** embed from `internal/db/migrations` that are the same files (single source of truth—do not duplicate SQL).
   - `NewMigrator(fsys, "studio_goose_db_version")`
   - `Up` / `Down` / `Status` helpers
3. **CLI** `curriculum-studio/cmd/migrate` mirroring `server/cmd/migrate` but only service `studio`, directions `up|down|status`.
4. **Config** minimal struct: database URL, max pool (used later), `MigrationsLive bool`. No shared LMS config.
5. **Freeze policy doc** in `curriculum-studio/db/MIGRATION_POLICY.md`:
   - Pre-live: optional one-time squash allowed with dual-run proof + new checksum inventory; default is **adopt as-is**.
   - Post-live: additive only; never edit applied files; never `goose fix` rewrite history on shared envs.
6. **Checksum inventory** script `curriculum-studio/db/scripts/freeze_inventory.py` or Go test generating `baseline_manifest.json`.
7. **Makefile** targets: `studio-migrate`, `studio-db-test` (wrap existing pytest), document beside LMS `migrate`.
8. **Keep** Python tests green: `cd curriculum-studio/db && python3 -m pytest tests -q`.
9. **Do not** implement domain repositories yet (Phase 2+).
10. **Do not** register Studio inside `server/cmd/migrate` as the permanent path.

### Focused verification

- Empty DB → up → `\dt curriculum_studio.*` matches inventory
- up is idempotent
- down on non-live undoes 00004 only first
- baseline manifest matches files

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P1-E1 | testcontainers Postgres empty | `Migrator.Up` | all SCHEMA tables present; `studio_goose_db_version` has 4 rows; no public Studio tables |
| P1-E2 | same | second `Up` | no error; version unchanged |
| P1-E3 | fixture copies of 00001–00004 | freeze inventory command | sha256 match committed manifest |
| P1-E4 | mutate 00002 bytes in temp tree with live marker | freeze checker | non-zero exit |
| P1-E5 | non-live DB at head; live DB flag on second pool | `Down` | non-live succeeds one step; live refused |
| P1-E6 | env with `DATABASE_URL` (LMS) set and `STUDIO_DATABASE_URL` unset | config load / migrate | hard fail; no silent LMS fallback |

**Commands:**

```bash
cd curriculum-studio/db && python3 -m pytest tests -q
cd curriculum-studio && go test ./internal/db/ -count=1
go run ./curriculum-studio/cmd/migrate -h
```

## Anti-Cheating Audit

- Migrator must embed/read the real SQL files under `curriculum-studio/db/migrations/`, not a hard-coded `CREATE SCHEMA` stub.
- Tests must not set search_path to LMS schema or apply `server/internal/db/migrations`.
- Freeze checker must hash file bytes, not merely count files.
- Live down refusal must be in production code path, not only a skipped test.
- No `IF NOT EXISTS` table recreation bypassing goose history as a substitute for migrate.

## Completion Gate

- [ ] P1-S1–P1-S6 scenarios pass
- [ ] P1-E1–P1-E6 automated evidence green
- [ ] `MIGRATION_POLICY.md` + baseline manifest committed
- [ ] Makefile `studio-migrate` documented
- [ ] Python schema suite still green
- [ ] Anti-cheating audit clean
- [ ] B1 module path decision recorded

## Dependencies and rollback

- **Depends on:** none
- **Rollback:** delete Studio migrator package; SQL design artifact remains; no prod envs should exist yet
- **Risk:** accidental dual ownership if someone adds Studio to LMS migrate map—gate with code review checklist
