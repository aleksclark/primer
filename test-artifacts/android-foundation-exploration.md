# Android foundation exploratory acceptance

**Role:** dedicated Android emulator acceptance reviewer (exploratory only; no promoted automation authored)  
**Plan read:** `agent_docs/plans/primer-tasks/phase-01-foundation-pairing.md`  
**Date:** 2026-08-19  
**APK under test:** `primer-tasks/android/app/build/outputs/apk/debug/app-debug.apk`  
**Package/version:** `com.aleksclark.primertasks` / `1.0`  
**Real device:** fresh wiped `pixel` AVD at `emulator-5554`, Android 15 (`sdk_gphone64_x86_64`)

## Exact commands and evidence

```bash
export ANDROID_HOME=$HOME/Android/Sdk PATH=$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$PATH
emulator @pixel -wipe-data -no-snapshot -no-audio -no-boot-anim -port 5554
adb -s emulator-5554 wait-for-device
adb -s emulator-5554 shell 'getprop sys.boot_completed; getprop ro.build.version.release; getprop ro.product.model'
adb -s emulator-5554 install -r primer-tasks/android/app/build/outputs/apk/debug/app-debug.apk
adb -s emulator-5554 shell pm clear com.aleksclark.primertasks
adb -s emulator-5554 shell monkey -p com.aleksclark.primertasks 1
adb -s emulator-5554 exec-out screencap -p > test-artifacts/android-foundation-screen.png
adb -s emulator-5554 shell dumpsys package com.aleksclark.primertasks | grep -E "versionName|applicationInfo"
adb -s emulator-5554 logcat -d -t 200 > test-artifacts/android-foundation-logcat.txt
```

Observed command results:

- The wiped AVD booted: `sys.boot_completed=1`; Android release `15`; model `sdk_gphone64_x86_64`.
- APK install returned `Success`.
- `pm clear` returned `Success`, providing an unpaired app data state before launch.
- `dumpsys package` returned `applicationInfo=ApplicationInfo{... com.aleksclark.primertasks}` and `versionName=1.0`.
- The launched fresh app displayed the unpaired screen: **PRIMER TASKS**, **Pair this device**, pairing-code field, and **Pair device** button. Screenshot: `test-artifacts/android-foundation-screen.png`. No fatal app-process error was observed in the collected log excerpt: `test-artifacts/android-foundation-logcat.txt`.

The real Tasks Compose stack was already running and healthy during this exploration: API published at `127.0.0.1:37455` and web at `127.0.0.1:37456`. This is not a MockWebServer substitution.

## Acceptance findings

| Phase-1 Android criterion | Result | Evidence / limitation |
|---|---|---|
| Fresh real emulator can install and launch the debug APK into the unpaired pairing state | **PASS** | Wiped `pixel` AVD, successful `adb install`, `pm clear`, launch, screenshot, and package evidence above. |
| Pair by scanning the QR displayed by the real parent web page | **BLOCKED** | No real parent-issued QR/code was supplied or successfully driven in this run. The fresh app remained unpaired. Therefore no claim is made for QR scanning or pairing. |
| Pairing uses an exchange, not a QR credential/token | **BLOCKED** | Requires an actual QR payload and a successful real API exchange. Not observed. |
| Bound single-student name and empty checklist | **BLOCKED** | Requires successful pairing and profile response. The unpaired screen has no bound-student identity. |
| Bound state persists through process death | **BLOCKED** | No bearer/device state was legitimately created, so persistence cannot be asserted. |
| Bound state persists through emulator reboot | **BLOCKED** | No bearer/device state was legitimately created, so reboot persistence cannot be asserted. |
| Second fresh emulator cannot replay the used pairing material | **BLOCKED** | No successful first redemption occurred; replay denial was not exercised. |
| Parent revocation rejects the first device on its next request and returns it to re-pair state | **BLOCKED** | No paired device or parent-side revocation was available to drive. |
| No plaintext bearer or QR credential in app-private storage/logcat/backup | **BLOCKED** | `pm clear` left no paired credential to inspect. The collected logcat is retained, but no successful exchange occurred, so it cannot prove post-pair storage, backup, or log hygiene. No plaintext bearer is claimed absent from state that was never created. |

## Blocking conditions for rerun

1. Provide/drive a real parent browser session that creates a student and renders the real one-use pairing QR against the running Compose API.
2. Ensure the Android build can reach that running API from the emulator. During exploration the Compose API was host-published on dynamic port `37455`, while the installed app’s current pairing behavior did not complete an exchange against the running stack; this must be resolved or explicitly configured before a pairing result can be accepted.
3. After a genuine first redemption, repeat on a second wiped AVD, force-stop and reboot the first AVD, revoke from the parent web UI, then inspect `run-as` app-private storage, logcat, and backup/export behavior for the actual issued bearer and QR material.

## Conclusion

**Overall: BLOCKED.** The environment did provide a real fresh Android emulator, installed debug APK, and running real Compose/API stack, and the unpaired launch acceptance passed. The required live QR → real API pairing, persistence, replay, revocation, and post-pair credential-leakage checks were not driven; they remain blocked rather than passed.
