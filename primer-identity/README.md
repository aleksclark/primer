# Primer Identity

Standalone OpenID Connect / OAuth identity service for Primer products.

Module path (frozen): `github.com/aleksclark/primer/identity`

This tree is a **separate deployable** with its own PostgreSQL database
(`primer_identity`, goose table `identity_goose_db_version`). It does not share
a database with the LMS (`server/`), TV, or Curriculum Studio.

## Layout (I1)

```text
primer-identity/
  README.md
  go.mod
  cmd/identity-server/     # HTTP process (health/ready + graceful shutdown)
  cmd/identity-migrate/    # migrate-only entry (module-local)
  internal/config/         # IDENTITY_* envconfig, fail-fast validation
  internal/db/             # pgx pool + embedded goose migrations
  internal/db/migrations/  # foundation only (no accounts/OAuth yet)
  internal/api/            # chi+Huma /healthz /readyz /metrics + request IDs
  internal/app/            # process bootstrap
  internal/logging/        # structured JSON logs with secret redaction
  internal/testutil/       # Postgres testcontainer (primer_identity_test)
  internal/testutil/e2e/   # process-level fail-fast + SIGTERM proofs
```

## Configuration

All settings use the `IDENTITY_` prefix. **`IDENTITY_DATABASE_URL` and
`IDENTITY_ISSUER` are required in every environment** (no localhost DSN
default). Bare `DATABASE_URL` is intentionally ignored so ambient LMS/host
DSNs cannot satisfy Identity config.

| Variable | Description | Default |
| --- | --- | --- |
| `IDENTITY_DATABASE_URL` | Postgres DSN for **primer_identity only** | _(required — no default)_ |
| `IDENTITY_ISSUER` | OIDC issuer URL (non-empty required) | _(required — no default)_ |
| `IDENTITY_HOST` | Bind host | `0.0.0.0` |
| `IDENTITY_PORT` | Bind port | `8090` |
| `IDENTITY_ENV` | `development` \| `test` \| `production` | `development` |
| `IDENTITY_LOG_LEVEL` | slog level | `info` |
| `IDENTITY_SHUTDOWN_TIMEOUT` | Graceful shutdown bound | `10s` |
| `IDENTITY_HTTP_READ_HEADER_TIMEOUT` | HTTP header read timeout | `10s` |

**Do not** point `IDENTITY_DATABASE_URL` at LMS (`primer`), TV (`primer_tv`), or
Studio (`curriculum_studio`) databases. Validation rejects those names in every
environment. Missing/blank `IDENTITY_DATABASE_URL` fails before migrate/listen.

Example (local):

```bash
export IDENTITY_DATABASE_URL='postgres://primer:primer@localhost:5432/primer_identity?sslmode=disable'
export IDENTITY_ISSUER='http://localhost:8090'
export IDENTITY_ENV=development
```

## Commands (module-local, executable now)

Root `make identity-build` / `identity-test` / `identity-cover` detect this
module. Root `migrate-identity`, `identity-e2e`, and `dev-db-identity` remain
F0-owned stubs — use the module-local commands below until F0 wires them.

```bash
# From repo root (F0 targets):
make identity-build    # -> bin/identity-server
make identity-test
make identity-cover

# Module-local build
cd primer-identity
go build -o ../bin/identity-server ./cmd/identity-server
go build -o ../bin/identity-migrate ./cmd/identity-migrate

# Migrate (requires IDENTITY_DATABASE_URL + IDENTITY_ISSUER)
export IDENTITY_DATABASE_URL='postgres://primer:primer@localhost:5432/primer_identity?sslmode=disable'
export IDENTITY_ISSUER='http://localhost:8090'
go run ./cmd/identity-migrate up
# or: ../bin/identity-migrate up
# down: go run ./cmd/identity-migrate down

# Process / package E2E (testcontainer; no root identity-e2e target yet)
go test ./internal/testutil/e2e/ -count=1
go test ./... -count=1

# Run server
../bin/identity-server
# GET /healthz  → 200 {"status":"ok"}  (+ structured access log)
# GET /readyz   → 200 after DB ping
# GET /metrics  → identity_http_requests_total
```

### Root Makefile (F0-owned)

| Target | Behavior |
| --- | --- |
| `make migrate-identity` | `go run ./cmd/identity-migrate up` (`IDENTITY_DATABASE_URL` + `IDENTITY_ISSUER` required; fail-closed; never prints DSN) |
| `make identity-e2e` | `go test ./internal/testutil/e2e/ -count=1` |
| `make dev-db-identity` | deferred — no coherent Compose surface for `primer_identity` (refuses hollow compose) |
## Request logging (P1-S6)

Every request (including `/healthz`) emits one JSON access line through the
redacting slog handler:

- `level`, `msg=request`
- `method`, `path` (no query string), `status`, `duration_ms`, `request_id`

No Authorization headers, bodies, or DSN passwords are logged.

## Isolation

- Goose table: `identity_goose_db_version` (not `goose_db_version`)
- Test DB name: `primer_identity_test`
- No LMS/TV/Studio migrations or shared DSN defaults
- Tests must set an explicit testcontainer `IDENTITY_DATABASE_URL` / Config.DatabaseURL
