# Curriculum Studio migration policy

## Ownership

- **SQL source of truth:** `curriculum-studio/db/migrations/*.sql` only.
- **Go ownership:** `curriculum-studio/internal/db` embeds those files via
  `curriculum-studio/db` (`//go:embed`) — no duplicated SQL trees.
- **Version table:** `studio_goose_db_version` (never LMS/TV `goose_db_version`).
- **Schema:** `curriculum_studio` (domain tables must not land in `public`).
- **CLI:** `go run ./cmd/migrate` inside the `curriculum-studio` module
  (`STUDIO_DATABASE_URL` required). Root `make migrate-studio` is F0-owned and
  should be wired to this binary by a later root pass.

## Baseline freeze (00001–00004)

Migrations:

| File | Role |
| --- | --- |
| `00001_identity_and_catalogs.sql` | tenants, workspaces, catalogs |
| `00002_plan_domain.sql` | curricula / plan graph |
| `00003_materialization_and_integration.sql` | runs, items, outbox, audit |
| `00004_invariants.sql` | triggers / immutability guards |

These four files are the **immutable initial history** once the freeze inventory
`db/baseline_manifest.json` is committed.

### Pre-live

- Default: **adopt as-is** (no rewrite).
- Optional one-time squash is allowed only with dual-run proof and a regenerated
  checksum inventory **before** any environment is declared live.
- Do not casually edit baseline bytes; prefer a new `00005+` even pre-live once
  the manifest exists.

### Post-live

- **Additive only:** new numbered goose files (`00005+`).
- **Never** edit applied baseline file bytes.
- **Never** `goose fix` rewrite history on shared environments.
- Live marker: env `STUDIO_MIGRATIONS_LIVE=true` and/or file
  `db/STUDIO_MIGRATIONS_LIVE`.

## Up / down policy

| Environment | `up` | `down` |
| --- | --- | --- |
| Disposable / non-live | allowed | one step at a time (tests, local reset) |
| Live-classified | allowed (forward) | **refused** unless `STUDIO_MIGRATE_BREAK_GLASS_DOWN=true` |

Forward-fix for production mistakes is a **new migration**, not down+edit.

## Freeze gate

```bash
cd curriculum-studio
go run ./cmd/migrate -check-freeze
# regenerate only when intentionally refreshing pre-live inventory:
go run ./cmd/migrate -write-freeze
```

CI / developers must fail if a frozen baseline file’s sha256 drifts from
`baseline_manifest.json`.

## DSN isolation

| Variable | Purpose |
| --- | --- |
| `STUDIO_DATABASE_URL` | **Only** DSN Studio migrate/config reads |
| `DATABASE_URL` | LMS — ignored; never a fallback |
| Forbidden DB names | `primer`, `primer_test`, `primer_tv`, `primer_tv_test`, `tv`, `tv_test` |

Suggested database name: `curriculum_studio` (tests: `curriculum_studio_test`).

## Commands

```bash
export STUDIO_DATABASE_URL='postgres://studio:studio@127.0.0.1:5432/curriculum_studio?sslmode=disable'

cd curriculum-studio
go run ./cmd/migrate up
go run ./cmd/migrate status
go run ./cmd/migrate down   # non-live only

# Module tests (includes testcontainers migrate E2E)
go test ./internal/db/ -count=1

# Python schema suite (unchanged)
cd db && python3 -m pytest tests -q
```
