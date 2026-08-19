# Primer Tasks Phase 2 — integration evidence

Branch: `impl/tasks-p2-checklist`
Reviewed implementation tip: `4140732` (durable scheduler and decision-integrity fixes)

## Implementation checkpoints

- `602a262` — recovered immutable task/schedule/verification backend slice.
- `9360c7a` — migrations, Huma registration, generated-client façade integration, web/Android vertical slice.
- `ca60489` — real-Postgres public-boundary/concurrency/error coverage and schedule/verification tests.
- `9fbbaac` — direct-origin development auth routing.
- `ce32a2f` — Vite forwarded-host preservation.
- `a30a72c` — Alpine tzdata for explicit IANA timezone loading.
- `6b12d5a` — server-owned schedule/occurrence collection controls.
- `1f53b6b` — server-derived model-disabled and restart timestamp evidence.
- `a2d2212` — bounded 365-day development horizon for public DST evidence.
- `d2130e1` — occurrence local timezone and EDT/EST rendering.
- `37a709d` — Android Upcoming rendering.
- `38782a7` — scrollable Android Today/Upcoming checklist.
- `3e97ff2` — LazyColumn-backed accessible Today/Upcoming rows and focused section model tests.
- `4140732` — durable PostgreSQL lease worker, ON CONFLICT materialization, immutable decision replay, and legal retry checks.

## Automated gates

- `go test ./... -count=1` — PASS.
- `go vet ./...` — PASS.
- Real PostgreSQL Phase 2 public-boundary tests — PASS, including task revision, one-off/recurrence materialization, approval/rejection/retry, skip/cancel, restart-safe uniqueness, and tenant isolation.
- `PRIMER_TASKS_COVERAGE_GATE=1 ../scripts/enforce-module-cover.sh . 85 tasks` — PASS at 85.0%.
- Offline Huma OpenAPI emission and ignored TypeScript/Kotlin generation — PASS.
- Web typecheck, lint, build, and client-boundary check — PASS.
- Android `testDebugUnitTest assembleDebug` — PASS.
- `make tasks-check` and Stacklane Compose start — PASS.

## Exploratory browser evidence

Dedicated exploratory agent reached PASS in call 7, before any Playwright was written. Evidence is in `.paseo-e2e/phase2-tasks-schedules/exploration.md` and `state.json`:

- parent test auth and two-tenant negatives;
- task create/publish/retire;
- one-off/daily/weekly schedules;
- explicit IANA timezone and public DST offsets (EDT→EST and EST→EDT);
- student pairing/checklist/detail/start;
- parent reject/retry/approve and checked occurrence;
- schedule cancellation, occurrence skip/cancel;
- URL collection status/sort state;
- model provider disabled and restart timestamp change.

No Playwright suite was promoted yet because the required Android exploratory acceptance was not fully closed.

## Android exploratory evidence

`test-artifacts/android-phase2-final/acceptance-report.md` records the real CameraX-first then exact system Photo Picker pairing, server-derived identity, Today and Upcoming, detail/start/awaiting, parent rejection/retry/approval coordination, completed state, force-stop persistence, and accessible upcoming rows. The paired device façade has no direct completion/approval method.

The remaining fail-closed item is the foreign-occurrence/forged-completion negative through the generated device façade. The app UI intentionally exposes only device-owned records and Start; the Android acceptance agent refused to substitute a private/raw API call. A real Parent-B occurrence was subsequently created through public UI (`62b79178-af55-4ca7-8cb8-8f4000252d2b`) for the prescribed generated-client negative, but the final façade denial run was not completed before this evidence checkpoint.

Therefore this evidence does **not** claim Phase 2 complete and Phase 3 must not be dispatched.
