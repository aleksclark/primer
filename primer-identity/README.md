# Primer Identity

Standalone OpenID Connect / OAuth identity service for Primer products.

Module path (frozen): `github.com/aleksclark/primer/identity`

This tree is a **separate deployable** with its own PostgreSQL database
(`primer_identity`, goose table `identity_goose_db_version`). It does not share
a database with the LMS (`server/`), TV, or Curriculum Studio.

## Layout (I1 + I2 + IB1 + IB2 token/JWKS/revoke + hardening)

**Status tip:** dual-reviewed IB2 hardening `f5d5b5b` (`impl/IB2-hardening-remed`).
Resume: `agent_docs/plans/identity-ib2-resume.md`. Next wave: IB3/I7 BFF.

```text
primer-identity/
  README.md
  go.mod
  openapi.yaml            # committed IB1+IB2 OpenAPI 3.1 baseline (generated)
  client/                 # generated IB1+IB2 Go client (identityclient)
  cmd/identity-server/     # HTTP process (health/ready + graceful shutdown)
  cmd/identity-migrate/    # migrate-only entry (module-local)
  cmd/openapi-gen/         # offline Huma OpenAPI 3.1 emitter (no DB/provider)
  internal/config/         # IDENTITY_* envconfig, fail-fast validation
  internal/db/             # pgx pool + embedded goose migrations
  internal/db/migrations/  # 00001–00009 (IB1 broker + IB2 keys/grants/audit/retention)
  internal/db/SCHEMA.md    # schema notes
  internal/domain/         # Account, ExternalIdentity, OAuth bounds, typed errors
  internal/password/       # Argon2id PHC KDF (never stores plaintext)
  internal/repo/           # accounts, broker, oauth token/assertion repos
  internal/api/            # chi+Huma health + authorize/broker + token/revoke/JWKS/metadata
  internal/app/            # process bootstrap (prod fail-closed without active key)
  internal/broker/         # authorize/start/callback composition
  internal/oauth/          # code exchange + revoke core (sign-before-commit)
  internal/token/          # ES256 mint/verify + private_key_jwt assertion
  internal/keys/           # signing custody + TransactionSigner
  internal/stytch/         # official adapter + broker provider boundary
  internal/logging/        # structured JSON logs with secret redaction
  internal/testutil/       # Postgres testcontainer (primer_identity_test)
  internal/testutil/factory/
  internal/testutil/e2e/   # process-level token/broker proofs (credential-free)
  internal/testutil/live/  # opt-in -tags=live_stytch test-project qualification
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
fail-closed IB2 OpenAPI drift check and OAuth/keys/token package suite. Root
`migrate-identity`, `identity-e2e`, and `dev-db-identity` remain F0-owned
stubs — use the module-local commands below until F0 wires them.

```bash
# From repo root (F0 targets):
make identity-build    # -> bin/identity-server
make identity-test
make identity-cover
make identity-openapi      # generate spec+client to private temps and cmp both baselines
make identity-test-oauth   # IB2 oauth/keys/token/revoke + process tests with -race (real DB)

# Module-local build
cd primer-identity
go build -o ../bin/identity-server ./cmd/identity-server
go build -o ../bin/identity-migrate ./cmd/identity-migrate

# OpenAPI (offline; no IDENTITY_DATABASE_URL / provider / network)
go run ./cmd/openapi-gen                 # stdout
go run ./cmd/openapi-gen -out /tmp/id.yaml
# Update the committed spec baseline (explicit; ordinary check does not write it):
go run ./cmd/openapi-gen -out openapi.yaml
# Regenerate the committed IB2 Go client from the spec (pinned oapi-codegen v2 tool):
go tool oapi-codegen -package identityclient -generate types,client -o client/client.gen.go openapi.yaml
go test ./cmd/openapi-gen ./client -count=1

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
| `make identity-live-stytch` | opt-in Stytch **test-project** provider qualification (`-tags=live_stytch`); requires `IDENTITY_LIVE_STYTCH=1`; not IB8-E10 browser/webhook |
| `make identity-openapi` | generate IB2 OpenAPI + Go client to private temp files and `cmp` `openapi.yaml` and `client/client.gen.go` |
| `make identity-test-oauth` | IB2 oauth/keys/token/revoke packages plus process tests (including `./client`) with `-race` against real Postgres |
| `make dev-db-identity` | deferred — no coherent Compose surface for `primer_identity` (refuses hollow compose) |

IB2 publishes the authorization-code token contract and RFC7009 revoke.
Full IB8-E10 browser + webhook live proof remains **BLOCKED** until IB3–IB7.
Opt-in `make identity-live-stytch` may call the Stytch **test** API only through
the production adapter (fail-closed negatives; optional session token happy path).
Production/live-project credentials are refused by that harness. `openapi.yaml`
is generated from handler signatures plus documented `POST /oauth/token` and
`POST /oauth/revoke` form-urlencoded operations (Huma OpenAPI only; the live
routes are registered once on chi so Huma never reads the form body). It
covers health/ready, the IB1 authorize/broker inventory, JWKS,
authorization-server metadata (including `revocation_endpoint`),
`POST /oauth/token`, and `POST /oauth/revoke`. It does not include
refresh-token grant, `client_credentials`, or provider/private payload
schemas. `client/client.gen.go` is generated from that spec with pinned
`oapi-codegen` v2 (`go tool oapi-codegen`); do not add handwritten DTOs.
The generated client has exactly one `OauthToken` operation and exactly one
`OauthRevoke` operation.

Token and revoke process proof uses a real `app.Run` listener plus Postgres
and an active signing key. Codes are issued through production repos and a
scripted broker labelled as test-only. Do not point process tests at live
Stytch.

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
