# Curriculum Studio

Standalone curriculum planning and lesson-production service. Studio owns plans
and the instructional materials derived from them; the LMS owns learner/mastery
state and session execution. Studio has its **own PostgreSQL database**, with no
cross-database foreign keys, views, or direct LMS/TV/Identity reads. Learner IDs
in snapshots are opaque references.

**Go module:** `github.com/aleksclark/primer/curriculum-studio`

## Implementation and acceptance

The checked-in service includes authoring/validation/publication, materialization
and a scripted workflow runner, exports with filesystem/S3 artifact adapters,
outbox/webhook delivery, project blueprints, collaborative authoring, and MCP.
It is no longer an F0 module skeleton or design-time-only database.

This is **implementation inventory, not full product acceptance**. The
[S17 browser guide](web/e2e/README.md) records later credential-free browser proof
than the [application handoff](docs/s17-application-surfaces.md) and
[browser diagnosis](docs/s17-b17-browser-fixes.md). Those earlier failed-browser
checkpoints are historical, not the latest result. Whole-phase review, coverage,
production BFF, live-provider, integration, and operations claims require their
own evidence. In particular, `internal/app`
currently uses process-local webhook secret storage; its presence is not durable
production secret custody. The workflow runner is scripted, not live generation.

Historical wave cursors in the [delivery roadmap](../agent_docs/plans/curriculum-studio-delivery/index.md)
are not current dispatch instructions. The [foundation crosswalk](../agent_docs/plans/curriculum-studio-foundation-crosswalk.md)
retains design decisions; see the [current authentication inventory](../agent_docs/authentication.md)
for implemented service boundaries rather than inferring a completed Clerk cutover.

## Layout

| Path | Role |
| --- | --- |
| `cmd/studio-server/` | Runtime process: config, migrations, HTTP, workers, optional `/mcp` |
| `cmd/studio-api/` | Reserved contract hook; not the runtime launch command |
| `cmd/migrate/`, `cmd/retention/` | Component-owned database operations |
| `cmd/openapi-gen/` | Offline Huma OpenAPI emitter |
| `internal/api/`, `internal/authn/`, `internal/authz/` | REST authoring, Identity JWT/JWKS validation, local role policy |
| `internal/bff/`, `web/` | BFF implementation and authoring SPA; independent browser/production acceptance |
| `internal/grpcapi/`, `internal/mcp/` | gRPC adapter/harness and MCP surface; not a claim of completed LMS integration |
| `internal/repo/`, `internal/db/`, `internal/testutil/` | PostgreSQL repositories, migration lifecycle, testcontainers |
| `internal/workflow/`, `internal/projects/`, `internal/artifacts/`, `internal/outbox/` | Materialization execution, project domain, bytes stores, delivery worker |
| `db/` | Embedded SQL migrations, schema/freeze tests, operational policies |
| `contracts/`, `clients/`, `tools/contract-gates/` | Contract sources, generated client facades, compatibility/exclusive-use checks |

## Clean-checkout build and tests

From the **repository root**:

```sh
# Prerequisites: pinned Go, Buf/protoc, Python, Node/npm, Docker for PostgreSQL tests.
# Generate ignored protobuf/REST packages before testing/building the module.
make -C curriculum-studio clients-generate
make studio-build
make studio-test
make studio-cover
make studio-e2e-go

# Schema and contract gates are module-local targets
make -C curriculum-studio studio-db-pytest
make -C curriculum-studio contracts-validate
make -C curriculum-studio contracts-gates
make -C curriculum-studio studio-mcp-e2e

# SPA build; client generation runs first
make studio-web
```

`make studio-cover` enforces **85%**; the module-local `make cover` only prints a
measurement and is not the enforced gate. `make studio-e2e` is currently **a shell
build smoke**, not a browser journey. For the real credential-free authoring
fixture and independent browser criteria, use the [browser handoff](docs/s17-application-surfaces.md)
and [web E2E guide](web/e2e/README.md).

`make foundation-check` is a structural prerequisite check, not full Studio
acceptance. `make dev-db-studio` still exits as unsupported; provision a disposable
Studio-only PostgreSQL separately. The opt-in [Compose workflow](../docs/dev-compose.md)
currently provisions LMS/TV databases, not Studio. Build, test, migration,
OpenAPI, and SPA targets are implemented, not deferred.

## Database and local run

- DSN: **`STUDIO_DATABASE_URL` only**, no LMS `DATABASE_URL` fallback.
- Suggested database: `curriculum_studio`; goose table: `studio_goose_db_version`.
- Domain tables use the `curriculum_studio` schema.
- Integration-test override: `STUDIO_TEST_DATABASE_URL`, pointing only at a
  disposable Studio test database; otherwise tests start PostgreSQL containers.
- Human memberships use the local projection of Primer Identity subjects, not
  provider roles or credentials.

After client generation, from the **repository root**:

```sh
export STUDIO_DATABASE_URL='postgres://studio:***@127.0.0.1:5432/curriculum_studio?sslmode=disable'
make migrate-studio
(cd curriculum-studio && go run ./cmd/migrate -check-freeze)
(cd curriculum-studio && go run ./cmd/studio-server)
```

Replace the illustrative DSN locally; never commit or print real credentials.
Protected authoring requires a configured verifier: `STUDIO_AUTH_MODE=jwks`,
`STUDIO_JWKS_URL`, `STUDIO_ISSUER`, and the intended `STUDIO_AUDIENCE`.
Test auth is rejected in production. Consult [config](internal/config/config.go)
for artifact/provider/MCP settings; starting the process alone does not prove
an authenticated browser workflow.

Schema references:
[SCHEMA.md](db/SCHEMA.md), [ERD.md](db/ERD.md),
[migrations](db/migrations/), [baseline manifest](db/baseline_manifest.json), and
[migration policy](db/MIGRATION_POLICY.md). Frozen migrations are immutable.

## Contracts

- Huma-emitted OpenAPI owns authoring REST and feeds REST client generation.
- Protobuf owns the machine/gRPC contract.
- MCP tools use their own code-defined schemas and protocol conformance gates.
- Generated Studio clients remain **untracked**. Generate them; do not commit
  missing imports or create a second handwritten DTO/transport source.

Read [contract ownership](contracts/OWNERS.md), [contract guide](contracts/README.md),
and [MCP contracts](contracts/mcp/README.md) before changing a boundary.
`X-Service-Token` is an explicitly enabled JWT migration alias, not a shared-key
replacement for Identity verification. Studio never issues access/refresh tokens.

## Operations

[Backup/restore](db/runbooks/backup-restore.md) covers the separate database and
restore drill. Module-local `make studio-backup-drill` requires disposable
`STUDIO_BACKUP_DSN` / `STUDIO_RESTORE_DSN`. Retention CLI defaults to dry run;
review eligibility before explicitly authorizing deletion.

Monitor migration version, unpublished outbox age, failed webhook deliveries,
and expired workflow leases. Do not place credentials, provider payloads, or
snapshot PII in logs, audit evidence, or metrics. Passing testcontainers or a
scripted model does not establish live Stytch, production BFF, durable secret
custody, or deployed backup/recovery acceptance.
