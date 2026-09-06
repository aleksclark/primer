# Primer Student — A16 device-owner qualification evidence

## Result

**Working supervised device-owner build. Phase 1 is NOT complete.**

Implemented and exercised a real Student DPC/launcher on the connected Samsung
Galaxy A16 5G. Device ownership, approved-app use, parent recovery, timed
maintenance, reviewed self-replacement, and non-wiping parent teardown/re-enrollment
were observed. **Silent updates remain blocked by Play Protect review** for the
tested local signing identity and enrollment/configuration. Reset-time QR
provisioning and the full escape/hardening matrix remain untested.

Source: working-tree changes based on `34a4f5c2`; no source commit was made during
this session. Exact installed binary identity is in the
[release receipt](./release-receipt.json). Current read-only OS assertions are in
[the owner/policy probe](./owner-policy-probe.json).

## Hardware and final state

- Samsung **SM-S166V**, product `a16xtfn`, Android 16 / API 36, One UI 8.
- Build `BP2A.250605.031.A3.S166VUDS6CZB6`, patch `2026-02-05`.
- Locked bootloader, verified boot green; original setup was already complete.
- Final application: **`com.aleksclark.primer.student`**, release **versionCode 13**,
  versionName **`0.1.0-qualification.13`**; non-debug, not testOnly.
- Owner: `com.aleksclark.primer.student/.admin.PrimerDeviceAdminReceiver`.
- Persistent HOME: `.StudentHome`; actual mode `LOCK_TASK_MODE_LOCKED` after unlock.
- Ordinary lock-task allowlist: Student, Samsung Calculator
  (`com.sec.android.app.popupcalculator`), Samsung Clock
  (`com.sec.android.app.clockpackage`).
- Parent maintenance closed at handoff. Settings, DocumentsUI and verifier
  packages are not permanent allowlist entries.
- Debugging intentionally remains enabled; do not give this qualification build
  to a student as finished daily-use lockdown.

No factory reset, account removal, Play publishing, verifier disablement, installer
spoofing or changes to Samsung Auto Blocker were performed. Exact-alarm permission
was granted through Samsung's actual **Alarms & reminders** UI for timed recovery.

## Public-boundary observations

| Scenario | Result / scope |
|---|---|
| Unmanaged launch | Correctly showed not enrolled and no lock task; installation alone granted no ownership. |
| ADB enrollment | Android accepted `dpm set-device-owner` on this already-set-up phone without a reset. This is development enrollment, not QR/Setup Wizard proof. |
| Signed-release identity | `apksigner` verified the APK; the installed package was pulled back and compared byte-for-byte by SHA-256 with the final build. |
| Parent bootstrap | Codes generated on the actual app UI, secured off-device, and acknowledged before enabling policy. Calculator/Clock selected through real controls. |
| Managed use | Both apps launched from Student in managed lock task; Home returned to Student. Recents did not expose an unrestricted switcher in the exercised path. |
| Settings denial | Outside maintenance, a Settings intent reached Android's **BlockedAppActivity**, not unrestricted Settings. This diagnostic is not a complete student escape matrix. |
| Wrong recovery code | Denied visibly. A valid code opened maintenance; replay after closure was denied. |
| Maintenance access | Actual top-resumed activity became Settings through the parent-authorized UI path. |
| Five-minute expiry | With Settings foregrounded, the real deadline returned to Student without a Home tap and removed temporary package exceptions. This proves production deadline reconciliation, not independently an alarm-only/process-death path. |
| Reboot with maintenance | Ownership/approved policy persisted and the lease did not survive reboot. Boot/keyguard lifecycle defects were found and corrected as described below. |
| Rotation abort | On the corrected two-phase implementation, preparing then abandoning a new batch left an old unused code usable. |
| Rotation confirmation | Explicit activation after saving the new batch rejected old codes and accepted a new one. No verifier replacement occurs merely from preparation. |
| Credential UI / logs | Canceling the recovery panel cleared its input. After actual valid recovery entry/closure on build 12, the current logcat buffer contained no tested recovery values in hyphenated or normalized form. Final build 13 additionally verified Android password IME type, not just visual masking; autocorrection is disabled. |
| Parent teardown | Real maintenance UI + explicit confirmation removed device ownership without a wipe; `dpm list-owners` returned `no owners`. Re-enrollment succeeded with fresh codes. |
| Final OS probe | Correct owner, version/release flags, HOME, actual managed lock task and exact ordinary allowlist. Intentionally wrong expected version failed closed. |

### Boot/keyguard corrections

Physical tests exposed two issues, not hidden with retries or security changes:

1. `startLockTask()` can complete asynchronously; immediately asserting mode was
   already LOCKED produced a false failure. The app now observes mode with a
   bounded check and handles asynchronous request errors visibly.
2. Android may resume HOME behind keyguard without another `onResume` at unlock.
   Student now waits while resumed for keyguard to clear, then requests lock task;
   it never dismisses keyguard itself. `onPause` cancels the pending request.

The correct test includes the parent's ordinary Samsung **Swipe to open**, not
an app-driven unlock. Corrected builds returned to managed Student after reboot
and normal unlock. A test that waited for managed lock task while still on the
preserved lock screen was not counted as a failure of post-unlock enforcement.
A PIN-protected handset and the complete lock-screen shortcut/escape matrix need
separate acceptance.

Android protected the owner process against the attempted `am kill` and `am crash`
commands; the PID did not change. **No forced-process-death proof is claimed.**
No test-only crash/backdoor was added to manufacture it.

## Update experiments — distinguish ADB, reviewed and silent

All accepted app updates used the same local qualification signing identity.
Development ADB refreshes were used to iterate code and **never counted as silent
in-app installation evidence**.

| Running installer -> candidate | Observation |
|---|---|
| 2 -> wrong-key 3 | Parent selected a real re-signed APK in the system picker with matching expected file digest/size. App rejected it before install with signing-identity mismatch. |
| 2 -> 3 | System verification rejected the install; kiosk initially blocked Play Protect's review surface. Installed version stayed 2. |
| 4 -> 5 | Verifier agents were temporarily available during parent maintenance. Play Protect showed **"App blocked to protect your device" / "Play Protect hasn't seen an app from this developer before."** Explicit **More details -> Install anyway** installed 5. Owner, approved apps and remaining maintenance lease persisted; app confirmed actual installed version. **Prompted, not silent.** |
| 5 -> 6 | Same-key next APK still showed the review. No approval was clicked. Silent test failed; the device stayed on 5. |
| 9 -> 10 | First deployed the updated installer 9, then used its actual code to install 10 with normal `UPDATE_PACKAGES_WITHOUT_USER_ACTION`, `INSTALL_REASON_POLICY`, and `PACKAGE_SOURCE_LOCAL_FILE` (API 33+). The same review appeared. Declined it with **Got it**; stayed on 9. **Silent gate remains blocked.** |

The relevant observed platform result included
`INSTALL_FAILED_VERIFICATION_FAILURE` (`-22`), and Finsky reported enterprise-device
verification with installer `com.aleksclark.primer.student`. This is distinct from
the ordinary PackageInstaller user-action eligibility gate.

Two read-only reviewers converged on the bounded metadata experiment and on
preserving the original silent-update acceptance criterion. They also identified
the recovery rotation safety issue; it was fixed and physically retested. Initial
review suggestions that overclaimed all other gates or implied Managed Google
Play satisfies Play-free distribution were explicitly rejected/corrected.

These observations do **not** prove Google's internal reputation algorithm,
permanent impossibility, or a universal requirement to publish on Play. Production
signing/distribution and real QR enrollment may need separate investigation. No
new distribution service or verification bypass was introduced here.

## Verification commands and results

### New Student modules — PASS

```bash
JAVA_HOME=/usr/lib/jvm/java-17-openjdk ANDROID_HOME=/opt/android-sdk \
  ./android/gradlew -p android \
  :core-device-policy:test :core-updates:test \
  :app-student:assembleRelease :app-student:lintRelease \
  -PstudentVersionCode=13 -PstudentVersionName=0.1.0-qualification.13
```

- Explicit protected signing environment loaded outside source control.
- **13 recovery tests + 9 APK-validation tests**, each in debug and release:
  22 distinct tests / 44 variant executions, zero failures/errors/skips.
- Full Student release lint and signed release assembly passed.
- Missing signing configuration failed release packaging with the expected error.
- Shared Gradle configuration cache stored/reused successfully.
- Read-only device probe is a promoted narrow OS assertion, not full native E2E:

```bash
python3 android/scripts/verify-student-device.py \
  --adb /opt/android-sdk/platform-tools/adb --serial <authorized-serial> \
  --version 13 \
  --approved-package com.sec.android.app.popupcalculator \
  --approved-package com.sec.android.app.clockpackage
```

### Broader Android suite — BLOCKED by unchanged TV test

`./android/gradlew -p android test assembleDebug :app-student:lintRelease` failed
in the existing `TvViewModelTest.a successful pairing stores the token and lands
on the catalog`, timing out waiting for the token. One run failed the release
variant; a full rerun failed debug. TV source was not changed.

An isolated baseline extracted with `git archive HEAD android design-system`
at **`34a4f5c2`**, without any Student changes, reproduced the same debug timeout
when running both TV app unit-test variants. An isolated class-only baseline run
passed. This is an existing asynchronous TV test failure, not a green full gate;
no assertion/timeout was weakened and no test was excluded to report success.

No Go/backend integration acceptance is claimed; this change adds no backend and
has no fake first-party service standing in for one.

## Custody, cleanup and remaining gates

- Canonical off-device recovery file:
  `~/.local/share/primer/android-qualification/recovery-codes.txt`.
  It contains **four remaining unused codes** after tests, with owner-only permissions.
  Signing environment/key and APK history are protected under the same directory.
  No code values, private keys or passwords are repository artifacts.
- The signing identity is local qualification custody, not approved production
  custody. Keep it to update this enrollment; do not silently replace the signer.
- Staged test APKs in the phone's `Download/PrimerQualification` and diagnostic XML
  are removed after verification. Only specifically created files are cleaned.
- Backup disabled; explicit Android 12+ cloud/device-transfer extraction exclusions
  cover both credential- and device-protected app storage. Actual transfer/restore
  acceptance is not claimed.
- **Still open:** silent updates, reset-time QR provisioning, forced process-death
  acceptance, full escape/keyguard/communications/accessibility matrix, production
  signing/release operations, and the pre-existing TV test failure.
- **Not part of this build:** Tasks UI migration, Control app, shared Compose library
  migration, remote management and pushed release service. Those remain later phases.
