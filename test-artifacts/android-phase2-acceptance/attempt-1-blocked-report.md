# Phase 2 Android exploratory acceptance — BLOCKED / FAIL CLOSED

**Reviewed workspace:** `d2130e10d8e370c1f211af3246a7583f540b7092`  
**UTC start:** 2026-08-19T08:12:55Z  
**APK SHA-256:** `41350b1b791f99e637b3d101c21e5cff7b7934457fd646ac0036336e6f8be18f`  
**Emulator:** `pixel`, Android emulator at `emulator-5556`, software GPU.  
**Real service:** current Stacklane Compose instance, web `http://127.0.0.1:37949/`, API `http://127.0.0.1:37948/`; APK configured with `PRIMER_API_ORIGIN=http://10.0.2.2:37949` (the real Vite `/api` proxy). `model-provider.txt` confirms `TASKS_MODEL_PROVIDER=disabled` in the real API container.

## Result

**FAIL CLOSED — do not ship Phase 2 Android acceptance.**

This is an exploratory review only. No source was changed, no MockWebServer was used, and no private database/repository state was created or modified.

The required existing Phase-1 paired emulator state was unavailable: the booted `pixel` AVD had no installed `com.aleksclark.primertasks` package before this fresh APK install, so it could not furnish a persisted paired student session. The Phase-1 report also records that its temporary emulators were cleaned up. A fresh Photo Picker pairing would be allowed only under the Phase-1 documented exception, but requires a newly rendered one-use QR from the actual parent public UI.

Chrome DevTools MCP could not create or access an isolated parent public-browser context because another process owns its fixed profile:

```text
The browser is already running for /home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile.
Use --isolated to run multiple browser instances.
```

Per the exploratory-browser gate, I did not kill or attach to that other browser, recreate a QR/code, call a pairing endpoint directly, seed a device credential, or substitute an automated/private pairing path. Therefore no real bound student identity existed on this emulator and the required Phase-2 runtime cases cannot be honestly claimed.

## Evidence actually obtained

| Area | Result | Evidence |
|---|---|---|
| Fresh current debug APK | PASS | `apk-build.txt`, `apk-sha256.txt`, `adb-fresh-install.txt` |
| Real API-origin configuration | PASS | build command below plus `stack-endpoints.txt`; build was given `http://10.0.2.2:37949` |
| Model-disabled real stack | PASS | `model-provider.txt` contains `TASKS_MODEL_PROVIDER=disabled` |
| Unpaired screen has CameraX primary before fallback | PASS | `unpaired.uia.xml` shows **Scan pairing QR** before accessible **Import pairing QR image** |
| System camera permission / real CameraX runtime | PASS | `camera-system-permission-fresh.uia.xml`; `camerax-primary-active.uia.xml`; `primary-camerax-active-dumpsys.txt` records active `com.aleksclark.primertasks` client, a Preview surface, ImageReader, and produced frames |
| CameraX emulator limitation | OBSERVED, not a Phase-2 failure | `primary-camerax-active-logcat.txt` includes CameraX camera-validator retry warnings, but dumpsys proves device 1 opened for the app and both output streams produced frames. No QR injection or VirtualScene experiment was performed. |
| System Photo Picker / SAF fallback | NOT EXERCISED | No QR image was rendered by the public parent UI, so the documented fallback exception was not invoked. |
| Paired identity, Today/Upcoming, occurrence detail | NOT RUN | No legitimate paired device credential/session. |
| Pending → start → awaiting verification | NOT RUN | No legitimate paired device credential/session. |
| Parent reject/retry/approve after refresh or process restart | NOT RUN | No legitimate paired device credential/session. |
| Skip/cancel visibility | NOT RUN | No legitimate paired device credential/session. |
| Foreign occurrence / forged completion denial | NOT RUN | No legitimate paired device credential/session; no direct endpoint call was substituted. |
| Paired persistence after force-stop | NOT RUN | No legitimate paired device credential/session. |

## Concrete ship blocker

**Severity: BLOCKER.** The designated Phase-2 Android acceptance requires actual behavior through a paired emulator against the real API. This run could only establish the fresh APK, real origin, disabled model provider, and primary CameraX baseline. It could not obtain the prerequisite paired state without violating the public-UI/system-picker boundary, so none of the mandatory student/parent/authorization/persistence observations have evidence.

There is also a review concern to re-check once pairing is restored: the current installed UI has only a `Today` heading/list. The Kotlin client declares `studentUpcoming`, but the app entry screen uses `studentToday` and does not render an Upcoming section. This is source-correlated (`primer-tasks/android/app/src/main/java/com/aleksclark/primertasks/MainActivity.kt`) and must be confirmed on a genuinely paired emulator; it is **not** counted as a completed runtime reproduction in this blocked run.

## Exact commands

```text
mkdir -p test-artifacts/android-phase2-acceptance
cd primer-tasks/android && PRIMER_API_ORIGIN=http://10.0.2.2:37949 ./gradlew --no-daemon clean assembleDebug
$HOME/Android/Sdk/emulator/emulator -avd pixel -no-snapshot -no-boot-anim -gpu software -port 5556
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 install primer-tasks/android/app/build/outputs/apk/debug/app-debug.apk
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell monkey -p com.aleksclark.primertasks 1
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 exec-out uiautomator dump /dev/tty > test-artifacts/android-phase2-acceptance/unpaired.uia.xml
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 exec-out screencap -p > test-artifacts/android-phase2-acceptance/unpaired.png
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap 250 525
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap 220 2110
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 exec-out uiautomator dump /dev/tty > test-artifacts/android-phase2-acceptance/camera-system-permission-fresh.uia.xml
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap 540 1230
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell dumpsys media.camera > test-artifacts/android-phase2-acceptance/primary-camerax-active-dumpsys.txt
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 logcat -d -v threadtime > test-artifacts/android-phase2-acceptance/primary-camerax-active-logcat.txt
```

## Required next run

1. Restore the Phase-1 paired emulator state **or** free the Chrome DevTools MCP profile and render a fresh QR through the actual parent public UI.
2. If a fresh pairing is necessary, first observe the live primary CameraX action; then use only the documented Android system Photo Picker/SAF exact-image fallback, with no manual payload/code entry or direct device-pair API call.
3. Re-run all unexecuted rows against the real API with `TASKS_MODEL_PROVIDER=disabled`, retaining screenshots, UI XML, filtered logcat/network evidence, and the force-stop/relaunch result.
4. Do not promote to connected/emulator automation from this exploratory result.
