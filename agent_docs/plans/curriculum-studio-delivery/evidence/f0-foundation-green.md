# F0 foundation check — GREEN evidence (post-change)

After F0 module foundations on branch `impl/F0-root-modules`, including fail-closed
coverage gate remediation (no multi-`cd` false-green; Studio ≥85, Identity ≥80).

Command: `make foundation-check`

```text
./scripts/check-f0-foundations.sh
PASS: curriculum-studio/go.mod exists
PASS: primer-identity/go.mod exists
PASS: curriculum-studio module path is github.com/aleksclark/primer/curriculum-studio
PASS: primer-identity module path is github.com/aleksclark/primer/identity
PASS: server module path unchanged (github.com/aleksclark/primer/server)
PASS: go.work exists
PASS: go.work uses ./server
PASS: go.work uses ./curriculum-studio
PASS: go.work uses ./primer-identity
PASS: curriculum-studio/go.mod has no server/identity require
PASS: primer-identity/go.mod has no server/curriculum-studio require
PASS: curriculum-studio has no forbidden cross-module Go imports
PASS: primer-identity has no forbidden cross-module Go imports
PASS: Makefile defines target foundation-check
PASS: Makefile defines target studio-build
PASS: Makefile defines target studio-test
PASS: Makefile defines target studio-cover
PASS: Makefile defines target studio-openapi
PASS: Makefile defines target studio-client
PASS: Makefile defines target studio-web
PASS: Makefile defines target studio-e2e
PASS: Makefile defines target studio-e2e-go
PASS: Makefile defines target dev-db-studio
PASS: Makefile defines target migrate-studio
PASS: Makefile defines target identity-build
PASS: Makefile defines target identity-test
PASS: Makefile defines target identity-cover
PASS: Makefile defines target identity-openapi
PASS: Makefile defines target identity-test-oauth
PASS: Makefile defines target identity-e2e
PASS: Makefile defines target dev-db-identity
PASS: Makefile defines target migrate-identity
PASS: COVER_MIN remains 85
PASS: STUDIO_COVER_MIN is 85
PASS: IDENTITY_COVER_MIN is 80
PASS: studio-cover uses enforce-module-cover.sh
PASS: identity-cover uses enforce-module-cover.sh
PASS: scripts/enforce-module-cover.sh is executable
PASS: scripts/probe-module-cover-gates.sh is executable
PASS: studio-cover/identity-cover recipes do not chain multiple cds
PASS: studio-cover deferred exit 2 without internal/
PASS: identity-cover deferred exit 2 without internal/
PASS: module-cover gate probe OK (isolated fixtures; empty/low fail, high pass, deferred/missing-bc)
PASS: make foundation-check is invokable
PASS: curriculum-studio go test ./... succeeds
PASS: primer-identity go test ./... succeeds

F0 foundation check OK
```

Module tests:

```text
cd curriculum-studio && go test ./...
ok  	github.com/aleksclark/primer/curriculum-studio	(cached)
cd primer-identity && go test ./...
ok  	github.com/aleksclark/primer/identity	(cached)
```

`go work sync` leaves `go.work` / `go.work.sum` stable (no diff).

## Deferred targets (honest fail-closed)

```text
studio-build: deferred until curriculum-studio/cmd/studio-server exists (S1)
make: *** [Makefile:…: studio-build] Error 2
identity-build: deferred until primer-identity/cmd/identity-server exists (I1)
make: *** [Makefile:…: identity-build] Error 2
studio-cover: deferred until curriculum-studio/internal packages exist
make: *** [Makefile:…: studio-cover] Error 2
identity-cover: deferred until primer-identity/internal packages exist
make: *** [Makefile:…: identity-cover] Error 2
studio-openapi: deferred until Studio OpenAPI generator exists (S*/C*)
make: *** [Makefile:…: studio-openapi] Error 2
dev-db-studio: deferred — no additive compose surface in F0; use S1/D1 for curriculum_studio DB
make: *** [Makefile:…: dev-db-studio] Error 2
migrate-identity: deferred until Identity migrator exists (I1)
make: *** [Makefile:…: migrate-identity] Error 2
```

## Coverage gates (fail-closed)

Shared helper: `scripts/enforce-module-cover.sh` (one module cwd, non-empty
`./internal/...` package set, `set -e`, temp profile cleanup, parse validation,
requires `bc`, no success on empty total).

| Target | Floor | Deferred (no `internal/`) | Empty packages | Low coverage | High coverage |
|--------|-------|---------------------------|----------------|--------------|---------------|
| `studio-cover` | 85% (`STUDIO_COVER_MIN`) | exit 2 | fail (helper 1 / make 2) | fail | exit 0 |
| `identity-cover` | 80% (`IDENTITY_COVER_MIN`) | exit 2 | fail (helper 1 / make 2) | fail | exit 0 |

Root `COVER_MIN` remains **85** (unchanged).

Regression probe: `./scripts/probe-module-cover-gates.sh` (also invoked from
`make foundation-check`). Uses an isolated `mktemp -d` fixture root only — never
creates or deletes under live `curriculum-studio/internal` or
`primer-identity/internal`. Live trees are snapshotted before/after and must stay
byte-identical (safe after real S1/I1 packages exist). CI path filters include
both `scripts/enforce-module-cover.sh` and `scripts/probe-module-cover-gates.sh`.
See also `f0-cover-probe-nondestructive-fix.md`.

Dev-compose DB names: **deferred** in F0 — no existing coherent Compose surface
for an additive stub without false claims. S1/I1/D1 own real DB wiring.

Inventory of Go modules in `go.work`:

- `./server` — existing LMS/TV module
- `./curriculum-studio` — new Studio module
- `./primer-identity` — new Identity module
