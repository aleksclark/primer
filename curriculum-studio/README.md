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
  go.mod                 # module root (F0)
  doc.go                 # compile anchor (F0)
  Makefile               # module-local migrate / freeze / test + contract targets
  cmd/migrate/           # Studio-owned goose CLI (D1)
  cmd/openapi-gen/       # offline OpenAPI emitter (C6)
  cmd/studio-api/        # process binary hook (platform S1 owns fullness)
  internal/db/           # migrator, config, freeze gate, Connect (D1)
  internal/repo/         # Querier, WithTx, Factory (D2)
  internal/testutil/     # testcontainers harness + savepoints (D2)
  internal/api/          # Huma authoring edge
  internal/grpcapi/      # gRPC integration edge
  internal/boundary/     # wire helpers — NOT a DTO catalog
  internal/authn/        # fail-closed JWT/JWKS validator (S2)
  db/                    # standalone PostgreSQL schema + tests + embed
  contracts/             # OpenAPI (authoring) + protobuf (integration)
  clients/ts-rest/       # TS authoring client package
  clients/go-rest/       # Go authoring client package
  clients/go-grpc/       # Go gRPC client package
  tools/contract-gates/  # ownership, parity, spikes, policy gates
```

Ownership freeze: [`contracts/OWNERS.md`](contracts/OWNERS.md).
Service shell fullness lands in platform wave **S1**. The S2 auth boundary
validates Identity-issued ES256 JWTs and enforces local workspace membership
RBAC; Studio never issues tokens or sessions. Set `STUDIO_AUTH_MODE=jwks` with
`STUDIO_JWKS_URL` and `STUDIO_ISSUER` for a configured validator. `test` mode is
for external test JWKS fixtures only and is rejected in production. Persistence
repositories and the testcontainers harness land in database wave **D2**.
Contracts C1 reserves package boundaries and generation policy.

## Database

The schema lives in [`db/`](db/):

- [`db/SCHEMA.md`](db/SCHEMA.md) — table inventory, enums, invariant map
- [`db/ERD.md`](db/ERD.md) — entity-relationship diagrams
- [`db/migrations/`](db/migrations/) — PostgreSQL / goose migrations (**single SQL source**)
- [`db/baseline_manifest.json`](db/baseline_manifest.json) — frozen sha256 inventory for 00001–00004
- [`db/MIGRATION_POLICY.md`](db/MIGRATION_POLICY.md) — up/down and immutability policy
- [`db/tests/`](db/tests/) — executable schema tests

Curriculum Studio uses its own database (suggested name `curriculum_studio`)
and the dedicated goose table `studio_goose_db_version`. Domain tables live in
the `curriculum_studio` schema only.

**DSN:** `STUDIO_DATABASE_URL` only — never falls back to LMS `DATABASE_URL`.

```bash
export STUDIO_DATABASE_URL='postgres://studio:***@127.0.0.1:5432/curriculum_studio?sslmode=disable'
cd curriculum-studio
go run ./cmd/migrate up
go run ./cmd/migrate -check-freeze
make test
```

Human memberships use `subject_ref = identity:<uuid>` (Primer Identity `sub`).
Credentials are never stored here. Postgres is mandatory for durable Studio
state (no in-memory production repositories).

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
| `make dev-db-studio` | deferred — no coherent Compose surface (refuses hollow compose) |
| `make migrate-studio` | real Studio migrator (`STUDIO_DATABASE_URL` required; fail-closed) |

Foundation check: `make foundation-check`.

## Persistence foundation (D2)

| Package | Role |
| --- | --- |
| `internal/db` | Config, Connect pool, goose migrator, freeze gate |
| `internal/repo` | `Querier`, `WithTx` UoW, error mapping, `Factory` + health probe |
| `internal/testutil` | testcontainers harness (`curriculum_studio_test`), `Tx` rollback, savepoints |

Postgres is **mandatory** for durable Studio state. There is no in-memory production
repository path. Integration tests use real PostgreSQL via testcontainers or
`STUDIO_TEST_DATABASE_URL` only (never ambient bare `TEST_DATABASE_URL` / LMS DSN).

```bash
cd curriculum-studio
go test ./internal/db/... ./internal/repo/... ./internal/testutil/... -count=1
go test ./internal/repo/... -race -count=1
```

## Validate

```bash
# Module foundation
make foundation-check
make studio-test

# Disposable Postgres via Docker (preferred)
cd curriculum-studio/db && python3 -m pytest tests -q

# Or point at a throwaway local database (never a shared LMS/TV/Identity database)
STUDIO_TEST_DATABASE_URL='postgres://primer:***@127.0.0.1:5432/curriculum_studio_test' \
  go test ./internal/... -count=1
TEST_DATABASE_URL='postgres://primer:***@127.0.0.1:5432/curriculum_studio_test' \
  python3 -m pytest tests -q

# Contracts (buf / protoc / OpenAPI + protobuf generate)
cd curriculum-studio && make contracts-validate
cd curriculum-studio && make contracts-buf-generate
cd curriculum-studio && make clients-go-grpc-build
# or: cd curriculum-studio/contracts && ./scripts/validate.sh && ./scripts/generate.sh --twice
```

Parser/static checks run even when Docker and Postgres are unavailable.


## Stytch-backed identity reconciliation (authoritative)

Stytch B2B is upstream human authentication/session authority. Primer Identity is its sole SDK/API client and downstream Primer token broker: it validates opaque Stytch sessions, maps exact `(project_id, organization_id, member_id)` to a local account, and issues only short-lived single-audience Primer JWTs/JWKS. Studio, LMS, TV, MCP, product APIs, and browser JS never receive, store, forward, log, or validate a Stytch session token, SessionJWT, tuple, or role.

Stytch organization/member roles are eligibility hints only. Studio workspace membership, LMS educator roles, and TV device authentication remain local systems of record; a Stytch organization does not create a Studio tenant/workspace. Provisioning is explicit invite/admin only, and distinct cross-org tuples stay distinct personas without email merge/linking. IA is library-only and unapproved; production auth waits for IA-R and IB1–IB4, with signed webhook/cache+grant revocation as a hard BFF/MCP gate.

Required E2Es: mapped tuple to local membership; valid no-membership token denied; same-email cross-org isolation; Stytch admin-like role denied without local role; raw Stytch bearer rejected; outage fails closed/no negative cache; signed webhook forgery/replay/dedupe/out-of-order; explicit revocation bound; LMS local-role dual run; and no token/provider payload in audit logs.
