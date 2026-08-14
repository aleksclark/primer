# F0 foundation check — RED evidence (pre-change)

Captured on base `c8add3bda8d11143aa620c9916d2c65e87c4982e` before module foundations were added.

Command: `./scripts/check-f0-foundations.sh`

```text
FAIL: curriculum-studio/go.mod missing
FAIL: primer-identity/go.mod missing
PASS: server module path unchanged (github.com/aleksclark/primer/server)
FAIL: go.work missing
PASS: curriculum-studio has no forbidden cross-module Go imports
PASS: primer-identity has no forbidden cross-module Go imports
FAIL: Makefile missing target foundation-check
FAIL: Makefile missing target studio-build
FAIL: Makefile missing target studio-test
FAIL: Makefile missing target studio-cover
FAIL: Makefile missing target studio-openapi
FAIL: Makefile missing target studio-client
FAIL: Makefile missing target studio-web
FAIL: Makefile missing target studio-e2e
FAIL: Makefile missing target studio-e2e-go
FAIL: Makefile missing target dev-db-studio
FAIL: Makefile missing target migrate-studio
FAIL: Makefile missing target identity-build
FAIL: Makefile missing target identity-test
FAIL: Makefile missing target identity-cover
FAIL: Makefile missing target identity-openapi
FAIL: Makefile missing target identity-test-oauth
FAIL: Makefile missing target identity-e2e
FAIL: Makefile missing target dev-db-identity
FAIL: Makefile missing target migrate-identity
PASS: COVER_MIN remains 85
FAIL: make foundation-check is not invokable

F0 foundation check FAILED with 23 error(s)
```

Exit code: `1` (expected non-zero / RED).

Also confirmed Make targets absent on pre-F0 tree:

```text
make: *** No rule to make target 'foundation-check'.  Stop.
make: *** No rule to make target 'studio-build'.  Stop.
make: *** No rule to make target 'identity-test'.  Stop.
```
