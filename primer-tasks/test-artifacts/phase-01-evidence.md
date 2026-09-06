# Primer Tasks Phase 1 — integrated evidence (not donor history)

This file is the product-local note for the **integrated P1 slice on
`integration/tasks-p1-foundation`**. It is **not** the donor-lineage
acceptance report from `impl/tasks-p1-foundation` / tip `23b5685`.
Those donor commits and their untracked exploratory artifacts are not
present in this worktree and must not be treated as evidence for the
current PR head.

## What this document is

A status index for reviewers of the integrated exact head. Live proof is
the public-boundary tests and Make targets in this checkout. Historical
donor screenshots, emulator logs, and review reports are not copied here.

## Exact-head evidence that *is* claimed

Run these from the repository root / `primer-tasks/` on the reviewed SHA.
Do not substitute older donor SHAs.

| Gate | Command | Claim |
|---|---|---|
| Tasks Go tests | `make tasks-test` | Required for this PR |
| Tasks coverage ≥85% | `make tasks-cover` | Required; floor not lowered |
| Generated clients | `make tasks-clients` | Required; generated output stays untracked |
| Web lint/typecheck/build | `make tasks-web` | Required |
| Opt-in Compose check | `make tasks-check` | Additive Compose/Stacklane vector only |
| Android JVM tests + debug APK | `cd primer-tasks/android && ./gradlew test assembleDebug --no-daemon` | Required JVM/unit + debug assemble. Release unit tests must pass because they assert the capture hook is **inert** and writes no artifact. |

GitHub checks that actually run on this repository for the PR (`agents-module`,
`server-remoteagent-adapter`, `foundation-check`) are bound only to the exact
pushed head. They are not a substitute for the Tasks gates above.

## What is **not** claimed on this integrated head

The following were donor-lineage or exploratory claims. They are **not**
reproduced in this worktree and are **not** evidence for the current SHA:

- Connected Android emulator / `connectedDebugAndroidTest` on a Pixel AVD.
- Physical-camera CameraX live-scene pairing.
- Photo Picker/SAF exploratory acceptance reports.
- Promoted Playwright browser PASS on this exact head (the suite exists; this
  note does not assert a fresh exploratory+promoted run).
- `make tasks-proof` / two-worktree host isolation on this exact head.
- Linked reports that do not exist in this tree:
  - `test-artifacts/phase1-final-review/report.md`
  - `test-artifacts/android-picker-acceptance/report.md`
  - `test-artifacts/android-foundation-remediation/gateb-systematic-20260819T234500Z/report.md`

If a reviewer needs the original donor exploratory material, it remains on
the immutable P1 endpoint `62795d53` and the pre-integration backup
`backup/tasks-p6-external-pre-integration-20260905`. That is history, not
this PR's exact-head evidence.

## Android capture redaction (integrated)

Release builds must not emit a QR-frame diagnostic artifact. Debug builds may
write **one** redacted metadata/digest file and never persist RGBA bytes.
Shared JVM tests cover the redacted JSON packing; variant-specific tests
assert debug writes once and release remains inert.

## Pairing / token boundary

This note does not record pairing codes, session cookies, device tokens, or
QR payloads. Credential issuance must fail closed when CSPRNG entropy is
unavailable, without consuming authorization state or pairing codes.

## Phase 2

Not started.
