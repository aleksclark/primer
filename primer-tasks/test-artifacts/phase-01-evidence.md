# Primer Tasks Phase 1 evidence report

Base: `ec36e559abde93bd0cfaedd0a3c6dee3f410fbc0` (`ec36e55`)
Branch: `impl/tasks-p1-foundation`
Date: 2026-08-19

## Status

**BLOCKED — Phase 1 is not promoted.** The implementation establishes a separate Go module, real PostgreSQL Compose stack, tenant-scoped CRUD/pairing prototype, web shell, and Android debug shell, but mandatory independent browser and Android exploratory gates did not PASS. No Playwright or emulator automation was authored or promoted.

## Implemented paths

- `primer-tasks/`: standalone module, migrations, server/migrate/OpenAPI commands, API, web, Android, clients, Compose lifecycle.
- `go.work`, root `Makefile`: module and product forwarding targets.
- `.paseo-e2e/tasks-foundation/`: dedicated exploratory browser state and notes (blocked by Chrome DevTools profile lock).
- `test-artifacts/android-foundation-exploration.md`: dedicated emulator acceptance report (fresh APK launch PASS; pairing scenarios BLOCKED).

Generated contract/client source and build outputs remain ignored and were not intended for commit.

## Commands and observed results

- `STACKLANE_INSTANCE=phase1 primer-tasks/scripts/dev check` -> `PASS api,web`.
- Stacklane Compose start: `STACKLANE_INSTANCE=phase1 primer-tasks/scripts/dev up` -> real PostgreSQL, migration, API and Vite services healthy; direct API/web ports were `37455`/`37456` before a later web remount and `37481`/`37482` after it.
- `curl http://127.0.0.1:<api>/health` -> `{"status":"ok"}`.
- Rendered Compose JSON -> services `api,migrate,postgres,web`; publishing services use loopback ephemeral bindings and Stacklane labels; named state/cache volumes present.
- `make tasks-test` / `go test ./primer-tasks/...` -> PASS, but no product Go tests exist (vacuous).
- `go build ./primer-tasks/cmd/...` -> PASS.
- `make tasks-clients` -> PASS; offline contract emission and TypeScript generation produce ignored outputs.
- `make tasks-web` / `npm --prefix primer-tasks/web run lint` -> PASS.
- `make tasks-android` / `cd primer-tasks/android && ./gradlew assembleDebug` -> PASS; APK produced.
- API smoke through real PostgreSQL: parent A login/session, create student, issue pairing, browser pair/profile, and replay -> PASS; replay returned HTTP 410. Pairing code and device credentials are hashed server-side; QR payload contains code/expiry/ID but no returned device token.
- Vite proxy after rewrite: browser-facing `/api/auth/session` -> HTTP 401 rather than route 404.
- `make tasks-e2e` -> FAIL: no `e2e/` directory.
- `go test -race ./primer-tasks/...` and `go vet ./primer-tasks/...` -> PASS only because there are no product tests.
- `make tasks-cover` -> 0% product coverage; not a Phase 1 pass.

## Required exploratory evidence

### Browser

Dedicated agent `15495315-3829-4ed5-927c-21aaee49f9f8` loaded `paseo-e2e` and attempted exploration twice. Both `chrome-devtools_list_pages` and `chrome-devtools_new_page` failed before page creation because the MCP profile was already running at `/home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile`. Evidence is in `.paseo-e2e/tasks-foundation/exploration.md`; result is BLOCKED. No screenshots, UI assertions, cookie inspection, console/network evidence, or Playwright promotion is claimed.

### Android

Dedicated agent `036a6dcd-5ebe-43c4-9eee-72ce2b32fd6a` booted a fresh wiped `pixel` emulator, installed the real APK, cleared app state, launched it, and captured `test-artifacts/android-foundation-screen.png`, logcat, emulator log, and command log. Fresh install/unpaired screen PASS. Real QR/API pair, token exchange, bound profile, process/reboot persistence, replay, revocation, and post-pair storage/backup checks are BLOCKED; report explicitly records each result. No emulator automation was promoted.

## Independent anti-cheat review

Reviewer `3da755aa-d594-4c96-9004-0d2526281ed6` verdict: **BLOCK**. Findings include:

1. `/auth/login?principal=` is a direct development identity shortcut, not authorization-code + S256 PKCE; live issuer validation is absent.
2. BFF/API path mismatch was found and fixed for Vite `/api` proxy via path rewrite; public browser flow still lacks independent Chrome evidence.
3. Android has no implemented camera QR scanner, uses synchronous direct OkHttp rather than generated Kotlin façade, and cannot complete against Compose's ephemeral API port.
4. Pairing claim commits before session/device issuance; claim and credential creation are not one transaction.
5. Archive revocation is not one transaction and does not revoke student BFF sessions; pairing/device audit coverage is incomplete.
6. OpenAPI is manually duplicated rather than derived from production route registration; Kotlin generation is missing; the emitter contract is incomplete.
7. Pairing/device rows lack composite tenant/student ownership constraints.
8. There are no auth/tenancy/pairing/revocation/persistence tests; E2E target is broken; coverage is 0%.
9. No two-instance, hot-reload, HMR, screenshots/axe, or full Android evidence exists.

The full review output is preserved in the orchestrator session; this report records the blocking verdict rather than hiding it.

## Gate decision

Do **not** commit a claim of Phase 1 PASS. Do **not** write/promote Playwright or emulator automation. Do **not** dispatch Phase 2. Resolve the reviewer blockers and obtain independent browser exploratory PASS plus Android emulator exploratory PASS first, then follow the required promotion order.
