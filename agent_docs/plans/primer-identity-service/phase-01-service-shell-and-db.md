# Phase 1: Service shell and Identity DB

**File:** `phase-01-service-shell-and-db.md`
**Depends on:** None
**Duration guess:** 3–5 days
**Migration stage:** S1 (dark deploy foundation)
**Handoff wave:** W1

## Goal

Deliver a runnable Primer Identity process boundary: Go module under `primer-identity/`, `identity-server` binary, `IDENTITY_` configuration, embedded goose migrations against a **dedicated** Postgres (`primer_identity`, goose table `identity_goose_db_version`), liveness/readiness, structured logging, and baseline metrics hooks. After this phase the service boots, migrates, and answers health without implementing OAuth or accounts beyond empty schema scaffolding tables if needed for migrate smoke.

## Scope

### In scope

- Create Go module `github.com/aleksclark/primer/identity` (Go version aligned to `server/go.mod` 1.25.x family)
- `cmd/identity-server` with graceful SIGINT/SIGTERM shutdown
- Optional `cmd/identity-migrate` for migrate-only jobs
- `internal/config` via envconfig, prefix `IDENTITY_`
- `internal/db` pgx pool + goose migrate; embed `internal/db/migrations`
- Initial migration `00001_foundation.sql`: extensions/`pgcrypto` if needed, empty placeholder ok **or** minimal `schema_meta` — prefer real account skeleton deferred to Phase 2; Phase 1 must at least create goose version table and prove isolation
- `GET /healthz` liveness; `GET /readyz` DB ping (+ later signer — Phase 3 extends)
- slog JSON logs; request_id middleware
- Prometheus-style `/metrics` or OTel meter noop + HTTP request counter
- Root Makefile: `identity-build`, `identity-test`, `identity-cover`, `dev-db-identity`, `migrate-identity`
- `internal/testutil` Postgres testcontainer harness mirroring `server/internal/testutil` (separate DB name `primer_identity_test`)

### Out of scope / YAGNI

- OAuth, JWKS, Google, BFF, LMS edits
- Sharing LMS DSN or goose table
- Nomad deploy (Phase 14)

## BDD Success Criteria

#### Scenario: P1-S1 — Binary starts and serves health

- **Given** Postgres reachable via `IDENTITY_DATABASE_URL` and migrations present
- **When** an operator runs `identity-server`
- **Then** process listens on `IDENTITY_HOST`:`IDENTITY_PORT`
- **And** `GET /healthz` returns 200 JSON ok
- **And** process shuts down cleanly on SIGTERM within configured timeout

#### Scenario: P1-S2 — Config loads from IDENTITY_ env and fails fast

- **Given** environment defines `IDENTITY_DATABASE_URL`, `IDENTITY_PORT`, `IDENTITY_ENV`, `IDENTITY_ISSUER`
- **When** the process starts
- **Then** config values are applied (observed bind port and log field env)
- **And** empty required DB URL or issuer in production-like mode exits non-zero before listen

#### Scenario: P1-S3 — Migrations apply to empty Identity database only

- **Given** an empty `primer_identity` database
- **When** identity-server starts or migrate command runs
- **Then** goose table `identity_goose_db_version` exists
- **And** no objects are created in LMS/TV/Studio database names

#### Scenario: P1-S4 — Readiness fails when DB down

- **Given** server running and DB unreachable for ping
- **When** client calls `GET /readyz`
- **Then** ready returns non-200
- **And** health may still return 200

#### Scenario: P1-S5 — Migrate is idempotent and isolated

- **Given** database already at latest goose version
- **When** process restarts and migrates again
- **Then** second migrate is no-op success
- **And** goose table name is not `goose_db_version` shared with LMS

#### Scenario: P1-S6 — Structured logs without secrets

- **Given** server running
- **When** client hits `/healthz`
- **Then** log includes level, msg, request_id or method/path
- **And** DSN passwords, future tokens, and client secrets never appear

#### Scenario: P1-S7 — Test harness uses separate DB

- **Given** `make identity-test`
- **When** suite runs
- **Then** testcontainer DB name is Identity-specific
- **And** suite does not require LMS migrations

## Implementation Instructions

1. Scaffold `primer-identity/go.mod` with chi, huma/v2, pgx/v5, goose/v3, envconfig, testify, testcontainers-go (+ postgres module).
2. Implement `internal/config.Config`: `DatabaseURL`, `Host`, `Port`, `Env` (`development|test|production`), `LogLevel`, `ShutdownTimeout`, `Issuer` (required non-empty), `HTTPReadHeaderTimeout`, body max defaults.
3. Embed migrations; set goose table `identity_goose_db_version` explicitly.
4. Wire chi router; health/ready outside auth; do not register OAuth yet.
5. Copy LMS testutil patterns into `internal/testutil` with independent `Harness{DBName: "primer_identity_test"}`.
6. Makefile targets parallel to `tv-*` / planned `studio-*`.
7. Document example compose/DSN in `primer-identity/README.md` (short).
8. Verify: `make identity-build && make identity-test`.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P1-E1 | testcontainer Postgres; production main wiring or httptest | GET /healthz, /readyz | 200 health; 200 ready after migrate; goose version ≥1 | `make identity-test` |
| P1-E2 | start process | SIGTERM | exit 0 within timeout | `go test ./internal/testutil/e2e -run Shutdown` |
| P1-E3 | assert goose table name query | migrate twice | table `identity_goose_db_version`; no LMS tables | `go test ./internal/db -run GooseTable` |
| P1-E4 | unset IDENTITY_DATABASE_URL with ENV=production | start | non-zero exit; no listen | `go test ./internal/config -run FailFast` |

## Anti-Cheating Audit

- Health handler must not skip real server listen path in “E2E” that only unit-tests a function returning `"ok"`.
- Migrate must run real goose against real Postgres — not a fake Migrator interface for the completion gate.
- Confirm goose table name via SQL `to_regclass` / information_schema, not a hard-coded constant assertion alone.
- No `IDENTITY_DATABASE_URL` defaulting to LMS DSN in examples without loud warning.
- Logs scanned in test for password-like substrings from DSN.

## Completion Gate

- [ ] P1-S1–P1-S7 green
- [ ] P1-E1–P1-E4 green on real Postgres
- [ ] `make identity-build` produces binary
- [ ] Anti-cheat clean
- [ ] No LMS/Studio/TV production code modified

## Dependencies

- Upstream: none
- Downstream: all later Identity phases
- Sibling: none blocking

## Rollback

- Revert phase PR; drop Identity DB if never production-traffic; no product impact.
