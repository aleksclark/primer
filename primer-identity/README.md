# Primer Identity

Standalone OpenID Connect / OAuth identity service for Primer products.

Module path (frozen): `github.com/aleksclark/primer/identity`

This tree is a **separate deployable** with its own PostgreSQL database
(`primer_identity`, goose table `identity_goose_db_version`). It does not share
a database with the LMS (`server/`), TV, or Curriculum Studio.

## Layout (I1 + I2 + IB1)

```text
primer-identity/
  README.md
  go.mod
  openapi.yaml            # committed IB1 OpenAPI 3.1 baseline (generated)
  cmd/identity-server/     # HTTP process (health/ready + graceful shutdown)
  cmd/identity-migrate/    # migrate-only entry (module-local)
  cmd/openapi-gen/         # offline Huma OpenAPI 3.1 emitter (no DB/provider)
  internal/config/         # IDENTITY_* envconfig, fail-fast validation
  internal/db/             # pgx pool + embedded goose migrations
  internal/db/migrations/  # foundation + accounts/external_identities/password
  internal/db/SCHEMA.md    # schema notes
  internal/domain/         # Account, ExternalIdentity, bounds, typed errors
  internal/password/       # Argon2id PHC KDF (never stores plaintext)
  internal/repo/           # Create/Get/Lock account, external identities, password
  internal/api/            # chi+Huma /healthz /readyz /metrics + request IDs
  internal/app/            # process bootstrap
  internal/logging/        # structured JSON logs with secret redaction
  internal/testutil/       # Postgres testcontainer (primer_identity_test)
  internal/testutil/factory/ # account/identity test builders
  internal/testutil/e2e/   # process-level fail-fast + SIGTERM proofs
```

## Accounts and identities (I2)

- Stable JWT `sub` = `accounts.id` (UUID).
- External identities are unique on `(provider, provider_subject)` only.
- `primary_email` is **not** unique and is **never** used to auto-merge or
  find-or-create accounts. `ListAccountsByEmail` is non-authoritative and may
  return multiple rows.
- Password credentials use Argon2id PHC (`internal/password`); plaintext is
  never stored or logged. Disabled credentials fail closed.
- **Students are out of scope for Identity v1** — no `students` table and no
  `student` provider. LMS owns student records; Identity principals are
  educators/operators/service accounts only in later waves.

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

**Do not** point `IDENTITY_DATABASE_URL` at LMS (`primer` / `primer_test`), TV
(`primer_tv` / `primer_tv_test` / `tv` / `tv_test`), or Studio
(`curriculum_studio` / `curriculum_studio_test` / `studio`) databases. The same
pgx-parsed forbidden-name check runs in config, `db.Connect`, `db.Migrate` /
`MigrateDown` / Migrator.with, and the test harness — library callers cannot
bypass CLI gates. Missing/blank `IDENTITY_DATABASE_URL` fails before
migrate/listen.

Allowed canonical names: `primer_identity`, `primer_identity_test`. Other
non-reserved ephemeral names are permitted at the library/config boundary;
`IDENTITY_TEST_DATABASE_URL` (the only harness override; bare
`TEST_DATABASE_URL` / `DATABASE_URL` ignored) additionally requires an
Identity-safe name.

Example (local):

```bash
export IDENTITY_DATABASE_URL='postgres://primer:primer@localhost:5432/primer_identity?sslmode=disable'
export IDENTITY_ISSUER='http://localhost:8090'
export IDENTITY_ENV=development
```

## Commands (module-local, executable now)

Root `make identity-build` / `identity-test` / `identity-cover` detect this
module. Root `make identity-openapi` and `make identity-test-oauth` are the
fail-closed IB1 OpenAPI drift check and OAuth/IB1 package suite. Root
`migrate-identity`, `identity-e2e`, and `dev-db-identity` remain F0-owned
stubs — use the module-local commands below until F0 wires them.

```bash
# From repo root (F0 targets):
make identity-build    # -> bin/identity-server
make identity-test
make identity-cover
make identity-openapi      # generate to a private temp file and cmp the baseline
make identity-test-oauth   # IB1 E01..E10 relevant packages with -race (real DB)

# Module-local build
cd primer-identity
go build -o ../bin/identity-server ./cmd/identity-server
go build -o ../bin/identity-migrate ./cmd/identity-migrate

# OpenAPI (offline; no IDENTITY_DATABASE_URL / provider / network)
go run ./cmd/openapi-gen                 # stdout
go run ./cmd/openapi-gen -out /tmp/id.yaml
# Update the committed baseline (explicit; ordinary check does not write it):
go run ./cmd/openapi-gen -out openapi.yaml
go test ./cmd/openapi-gen -count=1

# Migrate (requires IDENTITY_DATABASE_URL + IDENTITY_ISSUER)
export IDENTITY_DATABASE_URL='postgres://primer:***@localhost:5432/primer_identity?sslmode=disable'
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
| `make identity-openapi` | generate IB1 OpenAPI to a private temp file and `cmp` `primer-identity/openapi.yaml` |
| `make identity-test-oauth` | IB1 E01..E10 relevant packages with `-race` against real Postgres |
| `make dev-db-identity` | deferred — no coherent Compose surface for `primer_identity` (refuses hollow compose) |

IB1 is library-only. Live Stytch / production provider traffic remains
**BLOCKED**. `openapi.yaml` is generated from handler signatures and covers
health/ready plus the IB1 authorize/broker inventory only — it does not
include `/oauth/token`, JWKS, or access/refresh/JWT/provider payload schemas.

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
