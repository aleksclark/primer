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

## Database

The schema lives in [`db/`](db/):

- [`db/SCHEMA.md`](db/SCHEMA.md) — table inventory, enums, invariant map
- [`db/ERD.md`](db/ERD.md) — entity-relationship diagrams
- [`db/migrations/`](db/migrations/) — PostgreSQL / goose migrations
- [`db/tests/`](db/tests/) — executable schema tests

Curriculum Studio uses its own database (suggested name `curriculum_studio`)
and, if it ever shares a PostgreSQL instance, the dedicated goose table
`studio_goose_db_version`.

## Validate

```bash
# Disposable Postgres via Docker (preferred)
cd curriculum-studio/db && python3 -m pytest tests -q

# Or point at a throwaway local database (never a shared LMS/TV database)
TEST_DATABASE_URL='postgres://primer:primer@127.0.0.1:5432/curriculum_studio_test' \
  python3 -m pytest tests -q
```

Parser/static checks run even when Docker and Postgres are unavailable.
