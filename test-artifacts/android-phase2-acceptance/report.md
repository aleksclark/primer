# Phase 2 Android exploratory acceptance — FAIL / BLOCKED

**Reviewed tip:** `37a709d` (`feat(tasks): render upcoming occurrences on Android`)  
**Real origin:** APK built with `PRIMER_API_ORIGIN=http://10.0.2.2:37949`; it used the live Vite `/api` proxy on the current Stacklane web service.  
**Model mode:** `TASKS_MODEL_PROVIDER=disabled` was retained on the real API container (`model-provider.txt`).  
**APK:** current fresh debug build, SHA-256 recorded in `attempt-2-apk-sha256.txt`.

## Verdict

**FAIL / do not ship Phase-2 Android acceptance.**

This rerun made genuine progress through the required Android/public-system boundary, but an external APK replacement cleared the newly paired app's DataStore while the reviewer was opening the first pending occurrence. That prevented the mandatory detail/start/parent decision/restart/authorization flow. The one-use QR is consequently spent; I did not manufacture another QR, manually enter a code, call the device pair endpoint, seed a credential, use MockWebServer, or access private database/repository state.

There is a second hard boundary: Chrome DevTools MCP remains unavailable to this reviewer for the required parent reject/retry/approve UI actions because another process owns its profile. The exact error is recorded below. I did not kill or attach to that browser.

## What was genuinely observed

| Required / relevant area | Result | Concrete evidence |
|---|---|---|
| Fresh current APK with Upcoming implementation | PASS | `attempt-2-apk-build.txt`; source has `studentUpcoming` at lines 97/153 and `Text("Upcoming")` at line 392 of `MainActivity.kt` |
| Exact parent-rendered QR copied unchanged to emulator-visible storage | PASS | Source is `.paseo-e2e/phase2-tasks-schedules/android-fresh-parent-qr.png`; `attempt-2-adb-push-exact-qr.txt` reports exactly that one file pushed to `/sdcard/Pictures/android-fresh-parent-qr.png` |
| CameraX remains primary | PASS | `attempt-2-unpaired.uia.xml` puts **Scan pairing QR** before **Import pairing QR image**; `attempt-2-camerax-dumpsys.txt` records the active `com.aleksclark.primertasks` camera client with Preview and ImageReader output streams producing frames |
| Documented fallback uses real Android Photo Picker/SAF | PASS | `attempt-2-system-photo-picker-indexed.uia.xml` is package `com.google.android.providers.media.module` and says **This app can only access the photos you select**; the sole exact screenshot was selected from that system picker. No CameraX/VirtualScene QR injection was attempted. |
| Pairing through actual API after Picker selection | PASS | `attempt-2-paired.uia.xml` renders server-derived student **E2E Student Call Two**, proving successful pair → profile → checklist/today live requests rather than an unpaired screen. |
| Today server state | PASS | Same paired UI shows a completed plus multiple pending/excused occurrence cards. |
| Upcoming server state / requested current fix | PASS | Same paired UI visibly renders **Upcoming** below Today; this directly verifies the newly added section against real paired API data. |
| Detail of a pending occurrence | BLOCKED | Reviewer tapped the live pending Today card, but the immediately following `uiautomator dump` command returned `Killed`; see environmental interference below. |
| Start → awaiting-parent verification | NOT RUN | Pairing data was externally cleared before a detail/start state could be captured. |
| Parent reject, retry, approve and checked state after refresh/restart | NOT RUN | No surviving paired app session; parent browser UI unavailable to this reviewer. |
| Cancellation / skip visibility | NOT RUN | No surviving paired app session for the required Android observation. |
| Foreign occurrence / forged completion denial | NOT RUN | No direct HTTP substitute was used; after pairing loss there was no valid device credential. |
| Persistence after force-stop | NOT RUN | The external package replacement/data clear invalidated any result; it would not be an honest persistence test. |

## Exact blocker evidence

### 1. External package replacement destroyed the live paired session

The app successfully reached the paired list at 03:20. During the next Android action, the requested dump command produced only:

```text
Killed
```

(`attempt-2-pending-detail.uia.xml`, eight bytes.) The resulting next UI snapshot is the unpaired screen (`attempt-2-after-start-awaiting.uia.xml`), not an awaiting verification claim.

The emulator's own log is unambiguous (`attempt-2-focused-logcat.txt`):

```text
08-19 03:21:00.813 ActivityManager: Force stopping com.aleksclark.primertasks ... installPackageLI
08-19 03:21:00.816 ActivityManager: Killing ... com.aleksclark.primertasks ... due to installPackageLI
08-19 03:21:01.092 ActivityManager: Force stopping com.aleksclark.primertasks ... clear data
08-19 03:21:01.107 ActivityManager: Force stopping com.aleksclark.primertasks ... clearApplicationUserData
```

This was not a product decision/revocation response and cannot be used to judge product persistence. It is environment interference during the exploratory run. The app must be kept exclusive and stable for the next attempt.

### 2. Parent public UI cannot be driven from this reviewer session

The required Chrome DevTools MCP tool invocation (`chrome-devtools_list_pages`) returned:

```text
The browser is already running for /home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile.
Use --isolated to run multiple browser instances.
```

I preserved the isolation boundary instead of killing/attaching to another agent's browser. The existing parent-UI QR evidence remains valid browser evidence, but it does not grant this reviewer authority to perform new parent reject/retry/approve actions invisibly.

## Exact executed commands

```text
cd primer-tasks/android && PRIMER_API_ORIGIN=http://10.0.2.2:37949 ./gradlew --no-daemon clean assembleDebug
$HOME/Android/Sdk/emulator/emulator -avd pixel -no-snapshot -no-boot-anim -gpu software -port 5556
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 uninstall com.aleksclark.primertasks
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 install primer-tasks/android/app/build/outputs/apk/debug/app-debug.apk
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 push .paseo-e2e/phase2-tasks-schedules/android-fresh-parent-qr.png /sdcard/Pictures/android-fresh-parent-qr.png
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell monkey -p com.aleksclark.primertasks 1
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap 250 525                 # Scan pairing QR (primary)
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap 220 2110               # in-app Allow camera
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap 540 1230               # system While using the app
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap 180 2275               # cancel scanner
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap 300 695                # Import pairing QR image
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell am broadcast -a android.intent.action.MEDIA_SCANNER_SCAN_FILE -d file:///sdcard/Pictures/android-fresh-parent-qr.png
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap 175 1990               # exact sole Picker thumbnail
```

The media-scanner broadcast merely indexed the already-pushed exact image so Android's real Photo Picker could show it; it did not decode, change, or inject the image/payload.

## Severity and next gate

**Severity: blocker.** The product has not failed a completed parent-decision flow; rather, the required acceptance has insufficient uninterrupted evidence. Keep the emulator exclusively owned, provide a fresh public-parent-rendered one-use QR (the previous one was consumed), and make an isolated parent browser accessible to the reviewer. Then rerun the unexecuted rows. Do not promote to connected/emulator automation from this exploration.
