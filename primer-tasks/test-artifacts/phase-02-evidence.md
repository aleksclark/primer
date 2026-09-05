# Primer Tasks Phase 2 — integrated evidence (not donor history)

This note indexes the **integrated P2 slice on `integration/tasks-p2-schedules`**.
It is **not** the donor-lineage report from `impl/tasks-p2-checklist` / tip
`26522b1` / endpoint `8274d217`. Donor exploratory screenshots, emulator logs,
and promotion reports are not copied here and are **not** evidence for this PR
head.

## Exact-head evidence that *is* claimed

Run these from the repository root on the reviewed SHA.

| Gate | Command | Claim |
|---|---|---|
| Focused P2 Postgres/DST/approval tests | `cd primer-tasks && go test ./internal/api ./internal/schedule ./internal/domain ./internal/verification ./internal/config -count=1` | Required |
| Tasks Go tests | `make tasks-test` | Required |
| Tasks coverage ≥85% | `make tasks-cover` | Required; floor not lowered |
| Generated clients | `make tasks-clients` | Required; generated output stays untracked |
| Web lint/typecheck/build | `make tasks-web` | Required |
| Android JVM + debug APK | `make tasks-android` | Required JVM/unit + debug assemble; release unit tests stay inert |
| Host e2e | `make tasks-e2e` | Required if the host stack can start; otherwise report blocked |

GitHub checks that actually run on this repository for the PR
(`agents-module`, `server-remoteagent-adapter`, `foundation-check` when path
filters match) are bound only to the exact pushed head.

## What is **not** claimed on this integrated head

- Connected Android emulator / `connectedDebugAndroidTest` / Photo Picker
  exploratory or promotion runs.
- Donor reports under `test-artifacts/android-phase2-*`,
  `test-artifacts/phase2-*`, or `.paseo-e2e/phase2-tasks-schedules/`.
- Physical-device CameraX live-scene pairing.
- Live model-provider evidence (Phase 2 keeps `TASKS_MODEL_PROVIDER=disabled`).

`primer-tasks/android/CONNECTED_ACCEPTANCE.md` describes how a future connected
run would be executed. It is a runner note, not a PASS for this SHA.

## Preserved P1 adaptations

- Fail-closed CSPRNG for login/session/pairing/device issuance.
- Variant-correct Android QR-frame tests (debug writes one redacted artifact;
  release writes none).
- Host/non-Compose stack as the default Tasks path; Compose remains opt-in.
- Agents, Studio, Identity, and LMS root modules/targets unchanged.

## Phase 3

Not started.
