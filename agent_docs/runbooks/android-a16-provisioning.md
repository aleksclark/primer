# A16 Student device-owner qualification

## Status and safety boundary

This is a **supervised hardware qualification build**, not the finished Student
product or certified daily-use lockdown. Tasks screens, remote management,
release hosting and fleet delivery are not implemented here. Debugging remains
enabled in both build variants on purpose; this is visible in the UI, not a
hidden debug bypass.

Initial hardware: Samsung Galaxy A16 5G **SM-S166V**, product `a16xtfn`, Android 16
(API 36), One UI 8, build `BP2A.250605.031.A3.S166VUDS6CZB6`, security patch
`2026-02-05`. Do not assume another A16/carrier firmware behaves identically.

See the [implementation plan](../plans/primer-android-platform/index.md) and
[current evidence](../../test-artifacts/android-student/device-owner-qualification.md).

**Do not reset, remove accounts, alter Samsung security controls or enroll another
person's device without approval.** ADB enrollment changes administration. A reset
is not a prerequisite to try development enrollment if Android accepts the current
state; production Setup Wizard QR provisioning is a separate reset-time test.

## Identity and build

- Application: `com.aleksclark.primer.student`
- Stable owner receiver: `.admin.PrimerDeviceAdminReceiver`
- Persistent launcher alias: `.StudentHome`
- Targets: `android/app-student`, `core-device-policy`, `core-updates`
- Baseline minSdk 28 / targetSdk 35; physical qualification is on API 36.
- No backend credential or network endpoint is bundled in this initial build.

Use JDK 17 and Android SDK 35 build tooling:

```bash
make design-system
JAVA_HOME=/usr/lib/jvm/java-17-openjdk ANDROID_HOME=/opt/android-sdk \
  ./android/gradlew -p android \
  :core-device-policy:testDebugUnitTest :core-updates:testDebugUnitTest \
  :app-student:assembleDebug :app-student:lintRelease
```

Release signing requires **all** of:

```text
PRIMER_STUDENT_KEYSTORE
PRIMER_STUDENT_STORE_PASSWORD
PRIMER_STUDENT_KEY_ALIAS
PRIMER_STUDENT_KEY_PASSWORD
```

Keep those outside the repository, with owner-only access. Student intentionally
uses separate variables from TV. Missing signing configuration must fail release
packaging rather than produce an unsigned deployment APK. Select an increasing
version explicitly; default version 1 is for a new development build only.

```bash
# Load your protected signing environment without printing its values.
JAVA_HOME=/usr/lib/jvm/java-17-openjdk ANDROID_HOME=/opt/android-sdk \
  ./android/gradlew -p android :app-student:assembleRelease \
  -PstudentVersionCode=<next-integer> \
  -PstudentVersionName=0.1.0-qualification.<next-integer>
/opt/android-sdk/build-tools/35.0.0/apksigner verify --verbose \
  android/app-student/build/outputs/apk/release/app-student-release.apk
```

For this workstation's qualification, local signing custody is under
`~/.local/share/primer/android-qualification/`, not git. Its key is explicitly a
**local qualification identity**, not an approved production distribution key.
Keep it for same-key replacement tests. Changing signer/application/receiver after
managed deployment is not a transparent update; decide production custody before
fleet enrollment. APKs, keystores, secret environment and recovery codes are not
repository test artifacts.

## Development enrollment (performed only with owner approval)

1. Select the exact connected serial using `adb devices -l`; use `-s` on every call.
2. Record model, firmware, verified boot, setup status and `dpm list-owners`.
3. Install the signed release APK and launch it. Unmanaged mode must say it is not
   owner; installing alone must never activate screen pinning or ownership.
4. Explicitly enroll:

   ```bash
   adb -s <serial> shell dpm set-device-owner \
     com.aleksclark.primer.student/.admin.PrimerDeviceAdminReceiver
   adb -s <serial> shell dpm list-owners
   ```

   If Android refuses due to accounts/provisioning/another owner, stop and report
   the reason. Do not remove accounts, fake provisioning settings or erase data.
5. Student shows **owner, parent setup required** but does not yet lock the device.
6. Grant exact-alarm permission via **Open alarm permission** if requested. It is
   required for maintenance expiry when Settings/another app is foregrounded.
   Never use `appops` to manufacture proof of a real permission flow.
7. Generate recovery codes. Save all six **off the phone**, in a password manager or
   an owner-only file, before confirming. They are 128-bit random one-use values;
   Student stores salted verifiers, never plaintext, and disables screen capture.
8. Select trusted apps and explicitly enable kiosk. Initial physical acceptance
   uses Samsung Calculator and Clock. App approval pins current signing identities;
   absent/re-signed apps are excluded with an explicit diagnostic.
9. Verify actual Android state: owner, resolved HOME, `LOCK_TASK_MODE_LOCKED`
   (not user-removable screen pinning), and the exact allowlist. Test each app and
   Home/Back/Recents behavior; a hidden launcher icon is not sufficient protection.

## Parent maintenance and recovery

- **Parent maintenance** accepts one unused recovery code and opens a five-minute
  window. The code is consumed durably before access; wrong attempts are throttled
  with persisted backoff. Reboot ends a lease regardless of its remaining time.
- Settings, system DocumentsUI, and platform-authorized package verification agents
  are temporary maintenance exceptions. They are removed at expiry/closure; no
  storefront is permanently student-approved by this mechanism.
- Use **Close maintenance** to end early. Verify the ordinary allowlist afterward.
- **Prepare replacement recovery codes** only generates a pending batch in the
  current UI session. Save it off-device, acknowledge custody, then use **Activate
  saved recovery codes** to atomically replace the old verifiers. Closing, expiry
  or reboot before activation preserves old unused codes. Renew before spending
  the last usable code. This is local physical-parent custody, not Control/backend
  enrollment or remote recovery.
- Recent recovery audit retains the last 200 sanitized events. It is a bounded
  qualification log, not the future server's complete audit ledger.
- **End qualification / remove owner**, followed by its explicit confirmation, is
  a parent-maintenance-only, non-wiping test teardown using Android's owner API.
  It removes this controller's restrictions/home binding and checks removal. If
  Android refuses, report failure; do not claim portable non-wiping decommission.
- If the DPC cannot launch, in-app recovery cannot fix it. Keep debugging available
  during qualification and rehearse the physical reset/reprovision/FRP procedure
  with the actual owner before hardening. Never bypass FRP or lose the signing key.

## APK replacement qualification

This first installer takes a parent-selected **local single APK**, not a remote
release command. Prepare the file's trusted SHA-256 and byte count off-device:

```bash
sha256sum <candidate.apk>
stat -c %s <candidate.apk>
adb -s <serial> push <candidate.apk> /sdcard/Download/PrimerQualification/
```

During maintenance enter those values and use **Select and install verified APK**.
Use the real system picker. For files staged with ADB, browse **Galaxy A16 5G ->
Download -> PrimerQualification**; the separate Downloads collection may omit
unindexed APKs. Do not replace the picker with an internal URI injection.

Student bounds the stream, verifies exact digest/length, package, newer version,
current signer set, SDK and native ABI, then uses a durable PackageInstaller
session. The current policy allows only self-replacement; third-party package
installation is not implemented by this qualification endpoint. It never launches
an unrestricted installer fallback for a student. Confirmation requires reading
Android's actual installed version, not receiving a callback or downloading bytes.

### Play Protect is an independent acceptance gate

On this handset the local qualification signer triggered **"Play Protect hasn't
seen an app from this developer before"**, including on a later same-key update.
An explicit **More details -> Install anyway** allowed one reviewed APK; that is
**prompted installation, not silent-update evidence**. Approval of one APK did not
prove future unattended updates. Do not disable verification, spoof the installer,
allow the store permanently, or count automation clicking approval as silent.

The bounded follow-up experiment first deployed updated installer build 9, then
used that running app to attempt build 10 with
`UPDATE_PACKAGES_WITHOUT_USER_ACTION`, `INSTALL_REASON_POLICY`, and truthful
`PACKAGE_SOURCE_LOCAL_FILE` (API 33+). It still showed the same verifier review.
This rules out those missing signals as a sufficient fix in that test; it does
not establish Google's internal reputation model or prove Play publication is
universally required.

A trustworthy Play-free production signing/distribution strategy and verifier
acceptance are unresolved requirements. Device ownership alone does not override
this independent system verifier. Any legitimate metadata/signing/configuration
change must be tested with a new real APK and an unchanged security baseline.

## Reusable verification probe

After public setup, with maintenance closed and Student in the foreground:

```bash
python3 android/scripts/verify-student-device.py \
  --adb /opt/android-sdk/platform-tools/adb --serial <serial> \
  --version <installed-version> \
  --approved-package com.sec.android.app.popupcalculator \
  --approved-package com.sec.android.app.clockpackage
```

The probe is read-only and fails on wrong owner/version, a debug/testOnly build,
wrong HOME, screen pinning, or extra/missing allowlisted packages. It deliberately
**does not** certify silent install, all escape paths, recovery or QR provisioning.
Use an intentionally wrong expected version to check that the probe fails closed.
After reboot, preserve keyguard: on this handset the parent uses Samsung's normal
**Swipe to open**. Student waits for real keyguard dismissal while resumed, then
requests and observes managed lock task. Do not infer a kiosk failure merely from
`LOCK_TASK_MODE_NONE` while still on keyguard, or dismiss keyguard from app code to
make a test pass. A credential-protected unlock needs the actual owner's input.

Supplement with real expiry, reboot, recovery replay, update rejection and approved-
app tests. Android may protect the owner process against ADB `am kill`/`am crash`;
if the PID did not change, do not claim a process-death test passed. No extra debug
backdoor may be added to manufacture that evidence.

## Remaining release gates

- Factory-reset/Setup Wizard QR flow on actual hardware (activities exist, unproven).
- Silent same-key updates without disabling the system verifier.
- Full boot/escape/security/accessibility/emergency/communication matrix; inspecting
  emergency access must not place a test call to emergency services.
- Production signing, all-app shared UI migration, Tasks, Control and remote policy/
  update service: subsequent phases, not this qualification build.
