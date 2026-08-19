# Primer Tasks Phase 1 remediation evidence

Branch: `impl/tasks-p1-foundation`
Reviewed tip: `7e7b884` (`fix(tasks): keep module tidy and stabilize dev proof`)
Status date: 2026-08-19

## Gate decision

**BLOCKED — Phase 1 is not promoted.** The browser exploratory gate remains PASS, and the implementation/build/coverage/contract/Stacklane gates now pass. The mandatory fresh Android camera acceptance did not PASS: five independent attempts were blocked by public-origin, camera-enumeration, module-runtime, or emulator-offline issues. No Android pair, persistence, replay, revoke, or post-pair storage result is claimed. No Playwright, connected Android, or Phase 2 work was started.

## Reviewed commits

- `629899f` — honest blocked checkpoint preserving the existing remediation/evidence.
- `06bc565` — stabilizes the Go watcher proof and ignores Android build products.
- `f258e7e` — derives the OpenAPI route/method set from the shared production chi router; deterministic contract tests and ignored generated Kotlin/TypeScript outputs.
- `5ad631b` — CameraX `RGBA_8888` + ZXing `RGBLuminanceSource` decoder seam with crop/stride/rotation tests using the browser QR fixture.
- `15f13f8` — Android pairing-origin policy tests.
- `9705e4c` — real PostgreSQL testcontainer integration coverage and enforcing 85% product gate.
- `7e7b884` — `go.mod` tidy normalization and final dev-proof repair.

## Focused verification

All commands below were run from the reviewed tip unless noted:

```text
go test ./primer-tasks/... -count=1                         PASS
go test -race ./primer-tasks/... -count=1                   PASS
go vet ./primer-tasks/...                                   PASS
go build ./primer-tasks/cmd/...                            PASS
make tasks-cover                                            PASS — 85.2% >= 85%
cd primer-tasks && go test ./... -count=1                    PASS
cd primer-tasks && go vet ./... && go build ./cmd/...        PASS
make tasks-clients                                           PASS
make tasks-web                                               PASS
cd primer-tasks && npm --prefix web run lint                PASS
cd primer-tasks/android && ./gradlew testDebugUnitTest       PASS
cd primer-tasks/android && ./gradlew assembleDebug           PASS
./primer-tasks/scripts/prove-dev.sh                         PASS
```

`make tasks-cover` uses `scripts/enforce-module-cover.sh`, runs `go test ./internal/... -coverpkg=./internal/...`, requires Docker-backed integration coverage (`PRIMER_TASKS_COVERAGE_GATE=1`), and fails closed below 85% or when integration infrastructure cannot run. Measured total: **85.2%**.

Offline OpenAPI emission was run twice and compared byte-for-byte:

```text
cd primer-tasks && go run ./cmd/openapi-gen -out /tmp/tasks-openapi-a.yaml
cd primer-tasks && go run ./cmd/openapi-gen -out /tmp/tasks-openapi-b.yaml
cmp ...                                                     PASS
SHA-256: 971950da1939c252a6b347bb346ce9415b6d85d314ca2f64fd506550e3d241ef
```

The contract test compares the emitted path/method inventory with the exact production chi router and checks served-document equality plus 201/204 status contracts. Generated TypeScript/Kotlin sources remain ignored; boundary/lint and Kotlin compilation passed. No generated clients/contracts are tracked.

The final dev proof verified two isolated Compose instances, loopback ephemeral ports, Stacklane labels, worktree source mounts, named volumes, Go reload without container restart, Vite HMR without navigation, source restoration, and that stopping instance A left instance B healthy. A separate manual `STACKLANE_INSTANCE=manual-check ./primer-tasks/scripts/dev up` also passed real migration/API/web health before exact destroy cleanup.

## Browser exploratory acceptance

Dedicated suite: `.paseo-e2e/tasks-foundation-remediation/`
Result: **PASS** (call 7, `state.json` phase `codify-ready`).

The fresh Chrome flow observed real Parent A and Parent B S256 PKCE authorization, HttpOnly/SameSite=Lax callback headers (redacted in `call7-auth-callback-headers.md`), student CRUD/archive, QR issuance, student-browser pairing and refresh persistence, replay denial, archive revocation, two-tenant URL/body/filter IDOR denial, empty JS cookie/storage surfaces, responsive dark/light UI, and Lighthouse accessibility 100. Relevant UI/auth behavior was unchanged by the remediation commits used after that pass. No Playwright suite was written or promoted because Android has not passed.

## Android exploratory acceptance

Dedicated reviewer/acceptance agent: `e4056e00-6e47-4e72-b709-8d28931270ab`
Result: **BLOCKED — no PASS claim.** Evidence is under `test-artifacts/android-foundation-remediation/l2-independent-call{1,2,3,4,5}-20260819/` and `.paseo-e2e/android-phase1-l2-review/`.

Observed blockers by call:

1. Browser MCP/profile and public-origin reachability prevented a fresh QR.
2. Direct web was healthy but the visible issuer URL omitted the advertised `:8091` port and callback targeted an unreachable Stacklane web origin.
3. Environment-only `/issuer`, direct callback, and `10.0.2.2` origin configuration reached a fresh QR and real UI, but CameraX reported `Available cameras: 0` / `CameraUnavailableException` before the decoder ran.
4. Reconfiguration was blocked by the then-untidy Go module graph (`go: updates to go.mod needed`). This was fixed and independently verified afterward by `go mod tidy`, real Compose startup, and `prove-dev`.
5. With the tidy module and exact known-good `-gpu swiftshader_indirect -camera-back webcam0 -port 5556` invocation, the real UI issued a fresh QR and the actual QR feed was sent to `/dev/video0`. Initial camera enumeration showed one back camera, but the emulator became unresponsive/offline when the live QR feed was introduced; bounded diagnostics recorded `pair_ui=unknown`. No bound student or downstream PASS was claimed, and the emulator/feed/stack were cleaned up.

The Android implementation itself has a pure `QrFrameDecoder` seam that consumes actual RGBA bytes with row/pixel stride, crop offsets, and 0/90/180/270-degree rotation. The exact browser-rendered QR crop and rotation/stride matrix pass JVM tests. This is implementation evidence only; it is not a substitute for the required live CameraX pairing acceptance.

## Anti-cheat and promotion status

- Pairing tests use real PostgreSQL and assert transactional replay, tenant isolation, archival revocation, credential hashes, and public HTTP behavior; no first-party in-memory substitute was added.
- Production configuration tests cover fail-fast test-auth/issuer/secret/database validation.
- Browser acceptance used the real UI and public same-origin boundaries; no private repository seeding was used.
- Android acceptance never typed a code, called a pairing endpoint directly, recreated a QR payload, or bypassed CameraX.
- The duplicate Android implementer `843df8c2` was closed before integration; its activity showed inspection only and no edits. The sole Android owner `f88fb245` produced `5ad631b`.
- No Playwright or emulator automation was promoted. Promotion is authorized only after a fresh independent Android exploratory PASS, followed by observed-flow promotion and a fresh full-phase anti-cheat review.
