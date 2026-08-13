# Phase 1: Service shell

**File:** `phase-01-service-shell.md`
**Depends on:** None (consumes existing `curriculum-studio/db/migrations`)
**Duration guess:** 3–5 days
**Handoff wave:** see index orchestrator map

## Goal

Deliver a runnable Curriculum Studio process boundary: Go module under `curriculum-studio/`, `studio-server` binary, `STUDIO_` configuration, embedded goose migrations against a dedicated Postgres, liveness/readiness, structured logging, and baseline metrics hooks. After this phase the service boots, migrates, and answers health without implementing product domains.

## Scope

### In scope

- Create Go module `github.com/aleksclark/primer/curriculum-studio`
- `cmd/studio-server` main with graceful shutdown (SIGINT/SIGTERM)
- `internal/config` via envconfig, prefix `STUDIO_`
- `internal/db` connect (pgx pool) + migrate using existing SQL; goose table `studio_goose_db_version`
- `GET /studio/v1/health` (liveness) and `GET /studio/v1/ready` (DB ping)
- slog JSON logs; request_id middleware stub
- Prometheus-style `/metrics` or OTel meter noop + counter for HTTP requests
- Root/`curriculum-studio` Makefile targets: `studio-build`, `studio-test`, `dev-db-studio`, `migrate-studio`
- Integration test harness skeleton with Postgres testcontainer (mirror `server/internal/testutil`)

### Out of scope / YAGNI

- Domain CRUD, authz, SPA, gRPC
- Changing migration SQL content (db track owns design)
- Deploying to Nomad (Phase 18)

## BDD Success Criteria

#### Scenario: P1-S1 — Binary starts and serves health

- **Given** a machine with Postgres reachable via `STUDIO_DATABASE_URL` and migrations present
- **When** an operator runs `studio-server`
- **Then** process listens on `STUDIO_HOST`:`STUDIO_PORT`
- **And** `GET /studio/v1/health` returns 200 with JSON status ok
- **And** process shuts down cleanly on SIGTERM within timeout

#### Scenario: P1-S2 — Config loads from STUDIO_ env

- **Given** environment defines `STUDIO_DATABASE_URL`, `STUDIO_PORT`, `STUDIO_ENV`
- **When** the process starts
- **Then** config values are applied (observed bind port and log field env)
- **And** unknown required empty DB URL fails fast with non-zero exit

#### Scenario: P1-S3 — Migrations apply to empty database

- **Given** an empty `curriculum_studio` database
- **When** studio-server starts (or migrate command runs)
- **Then** goose version table `studio_goose_db_version` exists
- **And** schema objects from 00001–00004 exist in Studio DB only

#### Scenario: P1-S4 — Readiness fails when DB down

- **Given** studio-server running and Postgres stopped or DSN invalid after start simulation
- **When** client calls `GET /studio/v1/ready`
- **Then** ready returns non-200 when DB ping fails
- **And** health may still return 200 (liveness ≠ readiness)

#### Scenario: P1-S5 — Migrate is idempotent

- **Given** database already at latest goose version
- **When** process restarts and migrates again
- **Then** second migrate is no-op success
- **And** no destructive schema churn

#### Scenario: P1-S6 — Structured request logs emitted

- **Given** server running
- **When** client hits `/studio/v1/health`
- **Then** log line includes level, msg, request_id or method/path
- **And** no secrets (DSN passwords) printed

#### Scenario: P1-S7 — Tests use isolated Studio database

- **Given** integration test suite
- **When** suite runs
- **Then** testcontainer (or dedicated DSN) is not the LMS/TV database name
- **And** suite creates schema via Studio migrations only

## Implementation Instructions

1. Scaffold `curriculum-studio/go.mod` with Go version aligned to repo (`1.25.x` family as in `server/go.mod`).
2. Add dependencies: chi, huma/v2 (even if only health first), pgx/v5, goose/v3, envconfig, testcontainers-go, slog.
3. Implement `internal/config.Config` fields: `DatabaseURL`, `Host`, `Port`, `Env`, `LogLevel`, `ShutdownTimeout`, `ArtifactStoreDir` (optional empty), `AuthMode` default `jwks` (unused until P2).
4. Embed migrations from `curriculum-studio/db/migrations` (or shared embed path); set goose table name `studio_goose_db_version`.
5. Wire chi router mounted at `/studio/v1`; register health+ready.
6. Add `internal/testutil` Postgres container helper copying LMS patterns (transaction-per-test optional later).
7. Makefile: `studio-build` → `go build -o bin/studio-server ./cmd/studio-server`; `dev-db-studio` creates DB; document DSN example `postgres://…/curriculum_studio`.
8. Verify: `make studio-build && make studio-test`.

## End-to-End Test Plan

#### P1-E1 — Process health against real Postgres

- **Setup:** testcontainer Postgres; build/run server or httptesttest with production main wiring
- **Action:** GET /studio/v1/health and /ready
- **Assert:**
  - 200 health; 200 ready after migrate
  - migrate version ≥ 4
- **Command:** `make studio-test`

#### P1-E2 — Graceful shutdown

- **Setup:** server process started in test
- **Action:** send SIGTERM
- **Assert:**
  - exit 0
  - in-flight health completes or connection closes cleanly
- **Command:** `go test ./internal/testutil/e2e -run Shutdown`

#### P1-E3 — Fail-fast bad config

- **Setup:** unset STUDIO_DATABASE_URL in production-like ENV
- **Action:** start server
- **Assert:**
  - non-zero exit
  - error mentions database URL
- **Command:** `go test ./internal/config -run Required`

#### P1-E4 — Log hygiene

- **Setup:** capture stdout logs during request
- **Action:** hit health
- **Assert:**
  - JSON or key=value structured fields present
  - DSN password absent
- **Command:** `go test ./internal/api -run LogRedact`

#### P1-E5 — DB isolation guard

- **Setup:** integration harness
- **Action:** introspect test DSN / database name
- **Assert:**
  - name contains studio or is ephemeral container not `primer` LMS default alone without studio schema separation
- **Command:** `go test ./internal/db -run Isolation`

## Anti-Cheating Audit

Reviewer must verify:
- Health handler is not a static file server fake disconnected from main
- Migrations actually run against Postgres (query `information_schema` for `tenants` etc.)
- No import of `server/internal/db` LMS migrations
- Ready checks real ping, not hard-coded true
- Tests do not skip when Docker missing without explicit build tag documentation
- No credentials committed

## Completion Gate

- [ ] All P1-S* scenarios pass via automated tests
- [ ] `make studio-build` succeeds
- [ ] `make studio-test` green with real Postgres
- [ ] Anti-cheating audit clean
- [ ] No production domain routes claimed complete


## Dependencies

- Upstream: None (consumes existing `curriculum-studio/db/migrations`)
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
