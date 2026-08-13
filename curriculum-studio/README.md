# Curriculum Studio

Standalone curriculum planning and lesson-production service.

Curriculum Studio owns what should be taught and the concrete instructional
materials derived from it. Primer owns the live student experience, mastery
state, tutoring, and session execution.

This tree is a **separate service boundary**. It does not share a database with
the Primer LMS (`server/internal/db`) or the TV server (`server/internal/tv/db`).
There are no cross-database foreign keys, views, or direct reads. Primer learner
IDs and auth subjects appear only as opaque text identifiers inside snapshot and
integration records.

**Go module (frozen):** `github.com/aleksclark/primer/curriculum-studio`

**Authoritative cross-artifact decisions:**
[`agent_docs/plans/curriculum-studio-foundation-crosswalk.md`](../agent_docs/plans/curriculum-studio-foundation-crosswalk.md)

## Layout

```text
curriculum-studio/
  README.md
  go.mod              # module root (F0)
  doc.go              # compile anchor (F0)
  db/                 # standalone PostgreSQL schema + tests
  contracts/          # OpenAPI (authoring) + protobuf (integration)
```

Service shell (`cmd/studio-server`, `internal/*`) lands in platform wave **S1**
and later. F0 only freezes the module root.

## Database

The schema lives in [`db/`](db/):

- [`db/SCHEMA.md`](db/SCHEMA.md) — table inventory, enums, invariant map
- [`db/ERD.md`](db/ERD.md) — entity-relationship diagrams
- [`db/migrations/`](db/migrations/) — PostgreSQL / goose migrations
- [`db/tests/`](db/tests/) — executable schema tests

Curriculum Studio uses its own database (suggested name `curriculum_studio`)
and, if it ever shares a PostgreSQL instance, the dedicated goose table
`studio_goose_db_version`.

Human memberships use `subject_ref = identity:<uuid>` (Primer Identity `sub`).
Credentials are never stored here.

## Contracts

Wire contracts live in [`contracts/`](contracts/):

- OpenAPI authoring REST — browser / Studio UI
- Protobuf gRPC — Primer integration payloads and machine RPCs

Ownership split and auth presentation are documented in
[`contracts/README.md`](contracts/README.md) and the foundation crosswalk.
`X-Service-Token` is migration-only; end-state is `Authorization: Bearer` JWT.

## Root Make targets (F0 ownership)

| Target | F0 behavior |
| --- | --- |
| `make studio-test` | `go test ./...` in this module |
| `make studio-build` | deferred until `cmd/studio-server` exists (S1) |
| `make studio-cover` | deferred until internal packages exist (≥85% when active) |
| `make studio-openapi` / `studio-client` / `studio-web` / `studio-e2e*` | deferred |
| `make dev-db-studio` / `migrate-studio` | deferred (no hollow compose/DB claim) |

Foundation check: `make foundation-check`.

## Validate

```bash
# Module foundation
make foundation-check
make studio-test

# Disposable Postgres via Docker (preferred)
cd curriculum-studio/db && python3 -m pytest tests -q

# Or point at a throwaway local database (never a shared LMS/TV database)
TEST_DATABASE_URL='postgres://primer:***@127.0.0.1:5432/curriculum_studio_test' \
  python3 -m pytest tests -q

# Contracts (buf / protoc / OpenAPI)
cd curriculum-studio/contracts && ./scripts/validate.sh
```

Parser/static checks run even when Docker and Postgres are unavailable.
