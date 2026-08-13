# Primer Identity

Standalone OpenID Connect / OAuth identity service for Primer products.

Module path (frozen): `github.com/aleksclark/primer/identity`

This tree is a **separate deployable** with its own PostgreSQL database
(`primer_identity`, goose table `identity_goose_db_version`). It does not share
a database with the LMS (`server/`), TV, or Curriculum Studio.

## Layout (F0)

```text
primer-identity/
  README.md
  go.mod
  doc.go            # module root compile anchor
```

Service shell (`cmd/identity-server`, migrations, OAuth/OIDC packages) lands in
Identity wave **I1** and later. Do not add business or auth logic in F0.

## Root Make targets (F0 ownership)

| Target | F0 behavior |
| --- | --- |
| `make identity-test` | `go test ./...` in this module |
| `make identity-build` | deferred until `cmd/identity-server` exists (I1) |
| `make identity-cover` | deferred until packages with coverage exist |
| `make identity-openapi` / `identity-e2e` / `identity-test-oauth` | deferred |
| `make dev-db-identity` / `migrate-identity` | deferred (no hollow compose/DB claim) |

Foundation check: `make foundation-check`.
