# Student and Control completion audit

## Verdict

**Neither app is fully accepted as complete.** Native implementations exist and
many component tests pass, but reachable user workflows, lifecycle correctness,
build gates, and real native/backend/device acceptance still have gaps.

Audit baseline: native branch `d9bb765add44c51d527e1143b65a12b5f0a73274`.
Do not merge the independent producer or Kotlin branches wholesale into native
work. All remediation remains on isolated branches; no push/deployment is implied.

## Parent-observed findings and dispatch

| ID | Priority | Finding / source | Remediation owner and required check |
|---|---|---|---|
| S1 | P1 | `ManagementSession.applyReleases` gives the installer `{ authorized }`, a captured Boolean rather than a live enrollment check. Approved signer/policy is also captured before dispatch. | Student lane A: binding/generation and current policy rechecks at the effect boundary; delayed download/rebind/revoke tests. |
| S2 | P1 | Separate activity/worker sessions can overlap. `applyDesired` rereads credentials after receiving a response, while staging uses fixed target filenames. | A: serialized reconciliation, scoped immutable snapshots, late-result fencing; concurrent enrollment/sync and cancellation tests. |
| S3 | P1 | `ManagedUpdater.pendingTargetId` / `pendingTargetVersion` read preferences with no observed writers; release results need durable target correlation and blocked/failure delivery. | A: persist target/session binding, read actual installed package version, report callbacks after restart without replaying blocked installs. |
| S4 | P2 | Management enrollment on Student is a paste field, not a parent-gated camera/image QR flow (`MainActivity`). | A: reuse bounded QR mechanism under current parent capability, including cancellation and lease expiry. |
| S5 | P2 | Student camera permission lacks optional hardware declaration. | A: manifest fix; lint must pass without suppression. |
| C1 | P1 | Both Control pairing and management enrollment display `qrPayload` as text, not scannable QR (`ControlScreens`, `DeviceScreens`). | Control lane B: render actual QR with quiet zone, expiry and cleanup; decode the rendered result and later scan it through Student. |
| C2 | P1 | Schedule/release selectors require raw UUIDs; loaded records are not selectable. | B: labeled, searchable/paginated server-backed choices; no external ID-copying needed for the native loop. |
| C3 | P1 | Opening a schedule editor sets IDs but not the existing date/time/timezone/recurrence/due/end values (`MainActivity`). Saving can overwrite values the parent did not edit. | B: lossless edit initialization and round-trip tests; explicit supported recurrence controls. |
| C4 | P2 | New-task navigation uses a space in the title as an editor-state sentinel. Clearing title closes the editor. | B: explicit editor state and UI tests. |
| C5 | P1 | Quick review actions reuse shared decision reason/selection; requirement-bound behavior will change under the pending P4 successor. | B: fetch/confirm the selected record and reason; bind mutations only to the reviewed producer contract, fail closed for unsupported workflows. |
| C6 | P1 | Prepared update ownership is not yet qualified for auth invalidation before prepare entry or while installation claims its file. | B: context-bound preparation generation and atomic claim/cleanup, with delayed real coordinator/ViewModel tests. |
| C7 | P1 | Failed installer state blocks all future installation, but no explicit manual retry/reset or cancel-confirmation UI is exposed. | B: explicit retry/cancel transitions through shared session APIs; no automatic failed/deferred installation. |
| C8 | P1 | Clerk sign-in only attempts password; incomplete/multi-factor states have no continuation. | B: supported SDK factor/browser flow without weaker identity policy; real instance acceptance remains operator-dependent. |
| C9 | P2 | Device UI lacks bounded remote-maintenance issuance, useful inventory selection, and recovery/install-history presentation despite backend mechanisms. | B: inventory actual endpoints first, expose existing typed operations; request missing APIs through producer ownership, never raw HTTP/DB access. |
| C10 | P2 | Long forms use non-scrolling columns; narrow screens, keyboard and large fonts need actual UI verification. | B: usable scroll/focus/insets, then parent-run screen checks. |
| B1 | P1 | Clerk 0.1.31 transitively selects Compose BOM 2026.01.00/runtime 1.10.1 against AGP 8.7.3/Kotlin 2.0.21 tooling; Compose lint crashes. Identity and Control build files also force older runtime dependencies. | Build lane D: supported pinned toolchain/runtime alignment; no lint disable/baseline or metadata-check bypass. |
| B2 | P1 | Control version is fixed at 1 and release signing is not explicitly configured/fail-closed (`app-control/build.gradle.kts`). | D: independent Control signing/version inputs and required release checks, without reading/generating private custody in the lane. |

A owns Student/runtime/policy/managed-updater changes; B owns Control/identity
Kotlin and UI, keeping Kotlin producer/facade work separate. D owns shared build
pins/wrapper/root build configuration and Control/identity Gradle files.
Independent read-only Student and Control reviewers have also been assigned;
additional findings must be validated and added, not silently declared resolved.

## Baseline command evidence

Executed with Java 17 and `/opt/android-sdk`:

```text
:app-student:test :feature-tasks-student:test :feature-device-management:test
:core-device-policy:test :core-updates:test :tasks-client:test
:core-parent-identity:test :feature-tasks-control:test :feature-device-control:test
:app-control:test :app-student:lintDebug :app-control:lintDebug
:app-student:assembleDebug :app-control:assembleDebug
```

- **410 unit-test variant executions, zero failures/errors/skips.** This counts
  repeated debug/release cases, not 410 distinct acceptance scenarios.
- Both debug APKs assembled; no APK was installed during this audit.
- Overall command **FAILED**: Student lint reports missing optional camera feature;
  Control/identity lint crashes with `KaSimpleVariableAccessCall` class/interface
  incompatibility in `RememberInCompositionDetector`.
- Full local log: `/tmp/primer-android-app-completion-baseline.log`.
- Dependency evidence: `/tmp/primer-control-compose-dependencies.log`.
- The full TV regression suite was not part of this baseline. Shared dependency
  changes must subsequently be checked against TV too.

## Follow-up checkpoints (still not completion)

- Control UI-first `6a9433fa` was integrated as `1de857fe`. Shared QR, picker,
  editor-state, and edit-initialization changes compile and focused tests pass.
  Review still requires scrolling/visibility of the enlarged forms, lossless
  schedule instants (seconds/fractions and DST overlap), observable invalid input,
  explicit decision reasons, and visible QR-encoding errors. Those were returned
  to B; a matrix round-trip does not establish on-device camera decoding.
- Standalone Kotlin client `55e05a9e` (including build change `ac3c3ea5`) was
  independently regenerated from the verified `535077f1…` bundle and compiled/
  tested by parent with `--rerun-tasks`. Source remained clean. This closes the
  generator-only compile gap, not consumer or runtime-constraint acceptance.
  `ContractConstraints` still needs strict JSON-type checks and demonstrated
  generated/facade wiring; helper-only tests do not establish contract conformance.
- Student `2dc64d13` / `f545e96f` were integrated as `5c675e53` / `2675dd7a`.
  Combined compilation **fails** because a suspend binding read is called from a
  non-suspending authorization callback and a public QR importer constructor
  exposes an internal source type. This is not the previously reported client
  artifact mismatch. Fixes were returned to A, along with queued-vs-blocked
  multi-package handling and strict OS-only confirmation readback.
- Student compile correction `0976c695` is integrated as `3ab1489b`. Parent's
  combined Student/module tests, `lintDebug`, and debug assembly now **PASS**
  against the committed parent client. The old compile errors are resolved.
  Authorization remains open: `ManagementAuthorization.revoke()` is wired to
  credential-denial paths but not actual enrollment replacement/cancellation.
  The helper test manually revoking a lease does not prove those lifecycle paths;
  production wiring and session-level cancellation/replacement tests were requested.
- Follow-up `050b0489` was integrated as `599e16af`; replacement/cancellation
  revocation calls now exist. Parent regression `5eced100` establishes a healthy
  desired-policy response, delays a second response, requests replacement through
  the same session, and then releases the old response. It **fails in debug and
  release** because the revoked generation still applies the old policy. The
  lease currently fences installation, not policy/recovery effects. Student lint
  and assembly pass, but this lifecycle gate remains red; remediation is assigned
  to A. The caller's original request epoch also needs preservation across waits.
- The current Control debug APK opened successfully on the isolated API-28 AVD
  `primer-release-api28` as an ordinary app (no device owner). It displayed the
  unconfigured-Clerk screen, and no Control crash appeared. This is startup/error
  UI evidence only: no sign-in, installer dialog, APK replacement or A16 change.
  Raw local diagnostics are ignored under `.paseo-e2e/android-control-audit/`.

## Existing evidence retained, not inflated

- A16 qualification build 13: ownership, managed HOME/lock task, approved apps,
  local bounded recovery, reboot/keyguard and reviewed replacement were tested.
  Silent replacement remained blocked by Play Protect. No reset occurred.
- Managed emulator Tasks exploration: genuine selected-image pairing, Student
  start/submit, **parent web** approval, warm-link isolation, reboot and Tasks
  revocation. This is not the native Control-to-Student loop or camera decoding.
- API-28 instrumented Tink verifier: RFC/Go signatures accepted, digest-plus-zeros
  forgery rejected. This is not full app/installer acceptance.
- Isolated TV sidecar executable staging gate passed; no live TV update/playback
  acceptance follows from that check.

## Producer/client coordination remains a gate

Accepted CP3 pre-merge input remains `0e8cc6d6` / normalized digest `296b4e17…`.
Isolated additive producer `7a83b074` / `535077f1…` remains provisional.
The assigned verifier's record at
`/tmp/primer-android-p4-reconciliation/assigned-verifier/ready-record.json`
reports source-bound generated-client/public-process and focused race checks
passed from the correct workspace. This does not close full coverage/race10/CI.

The frozen CP4 candidate `89f3a593` / `940992aa…` is BLOCKED for selecting a manual
requirement without binding start/submit to it. Its successor is not implicitly
accepted merely because a newer branch tip exists. Final integration must rebind
to the exact reviewed successor, preserving both capabilities and requirement-
bound behavior. P4 first; management migrations remain `00012`–`00015` after the
immutable canonical eleven. SAME150's Kotlin/legacy hold remains.

Kotlin client commits `cdea86a8`, `867c43b7`, `ac3c3ea5` and `55e05a9e` are separate
provisional work. Standalone compilation is now observed passing; strict
constraint wiring and actual native compatibility remain separate gates.

## Closure checklist

- [ ] All findings above fixed and independently re-reviewed on exact commits.
- [ ] Clean generated client, all affected unit/lint/build checks pass, including TV
  compatibility for shared changes and signed release packaging for both apps.
- [ ] Reviewed canonical producer successor + separable compiled Kotlin handoff;
  no copied DTOs or ignored `x-*` constraints.
- [ ] Actual native Control sign-in with existing household membership and required
  factors; no MFA/policy weakening or seeded in-app JWTs.
- [ ] Native Control creates student/task/schedule, renders QR; Student camera pairs,
  starts/submits; Control approves/rejects/retries; both restore server truth.
- [ ] Independent Tasks revocation, management enrollment/policy/recovery/outbox,
  offline/restart/concurrency and real OS readback are exercised.
- [ ] Real unprivileged Control N→N+1 and cancellation/retry/background confirmation;
  real Student managed release target/receipt correlation and preserved storage.
- [ ] A16 silent gate, physical escape matrix and soak are satisfied or explicitly
  left blocked. Prompted installation is never relabeled silent.

Live test prerequisites are not present in the parent process environment:
`PRIMER_API_ORIGIN`, `PRIMER_CLERK_PUBLISHABLE_KEY`, and
`PRIMER_RELEASE_TRUST_ROOT` are unset. Use operator-approved public configuration
and a real parent test account; never put passwords/MFA/signing keys in chat or
committed files. Physical-device changes and private signing/recovery custody
remain exclusively with parent L0. No wipe, live publication, or canonical merge
is authorized by this audit.
