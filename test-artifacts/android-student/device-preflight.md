# Primer Student device-owner verification — USB preflight

Historical read-only snapshot before implementation/enrollment. See
[the subsequent qualification](./device-owner-qualification.md) for the current
build, owner state and remaining blockers. The results below are not a claim
about the handset after that authorized work.

## Result

**Device connection: PASS. Student device-owner build verification: BLOCKED.**

Read-only ADB inspection at repository commit `34a4f5c2` confirmed the connected
handset and current OS state. There is no Student application target/APK in this
checkout and no Primer package installed on the handset. No ownership, kiosk,
silent-install, reboot-recovery or provisioning acceptance is claimed.

## Observed handset

| Field | ADB observation |
|---|---|
| Manufacturer / model | Samsung / `SM-S166V` |
| Product / device | `a16xtfn` / `a16x` |
| Build characteristics | `phone` |
| Android | `16`, SDK `36` |
| One UI property | `ro.build.version.oneui=80000` (One UI 8) |
| Build | `BP2A.250605.031.A3.S166VUDS6CZB6` |
| Security patch | `2026-02-05` |
| Verified boot | `green` |
| Flash locked / vbmeta state | `1` / `locked` |
| ADB | Connected and authorized over USB |
| Device provisioned | `1` |
| User setup complete | `1` |
| Device/profile owners | `dpm list-owners`: `no owners` |
| Device administration support | `android.software.device_admin` present |
| Managed users support | `android.software.managed_users` present |
| Installed Primer packages | `pm list packages primer`: no output |

Device identifiers such as the USB serial are intentionally omitted from this
repository evidence. Use the currently authorized serial explicitly for every
future ADB operation; do not assume only one device is connected.

## Samsung security observation

A narrowly filtered, read-only settings query returned:

```text
secure: rampart_strict_protection_switch_enabled=0
system: rampart_suw_main_on=0
```

These undocumented backing values **do not establish** whether all Auto Blocker
features are disabled or whether managed APK installation will be permitted.
Verify through Samsung's UI and actual signed-install tests before certification.
No Samsung setting was changed during this inspection.

## Build inventory and blocker

- `android/settings.gradle.kts` includes only `:app` (TV) and `:core` (TV domain).
- `android/app-student/` does not exist; no Student APK output was found.
- Existing APK identities are `com.aleksclark.primer.tv` and the separate prototype
  `com.aleksclark.primertasks`.
- TV declares a device-admin receiver, but its `KioskPolicy.isEligible` requires
  both television mode and device ownership. This phone reports `phone`; using
  TV as the device owner would not demonstrate Student kiosk functionality.
- The Tasks prototype does not declare a device-admin receiver.

No compile/install test was run for a nonexistent Student target. Building or
installing TV would not resolve this verification request.

## Read-only diagnostic commands

With `ADB=/opt/android-sdk/platform-tools/adb` and `SERIAL` selected from
`adb devices -l`:

```bash
$ADB devices -l
$ADB -s "$SERIAL" shell getprop ro.product.model
$ADB -s "$SERIAL" shell getprop ro.build.version.release
$ADB -s "$SERIAL" shell getprop ro.build.version.sdk
$ADB -s "$SERIAL" shell getprop ro.build.version.oneui
$ADB -s "$SERIAL" shell getprop ro.build.version.security_patch
$ADB -s "$SERIAL" shell getprop ro.build.display.id
$ADB -s "$SERIAL" shell getprop ro.boot.verifiedbootstate
$ADB -s "$SERIAL" shell getprop ro.boot.flash.locked
$ADB -s "$SERIAL" shell getprop ro.boot.vbmeta.device_state
$ADB -s "$SERIAL" shell settings get global device_provisioned
$ADB -s "$SERIAL" shell settings get secure user_setup_complete
$ADB -s "$SERIAL" shell dpm list-owners
$ADB -s "$SERIAL" shell pm list packages primer
$ADB -s "$SERIAL" shell pm list features
```

Only relevant capability/settings/owner fields were retained; no accounts,
messages, personal files or screen contents were inspected.

## Next verification gate

Implement the [Phase 1 Student DPC/launcher and recovery build](../../agent_docs/plans/primer-android-platform/phase-01-a16-ownership.md),
then build/sign and test it on this exact Android 16 firmware. Ownership enrollment
changes device administration and requires explicit execution approval. Production
Setup Wizard QR provisioning requires a reset; the fact that setup is already
complete does not by itself establish whether a development ADB enrollment would
succeed. Do not use `dpm set-device-owner` as a non-mutating eligibility probe.

No APK installation, reset, ownership change, policy restriction, account removal,
reboot, or security-setting modification was performed in this preflight.
