# Android Phase 2 exploratory acceptance rerun — FAIL CLOSED

**Reviewed commit:** `37a709da8b3c7286234dd8e82c3a5e8c67b63fb2`  
**UTC run window:** 2026-08-19T08:20:20Z–08:24:55Z  
**Fresh emulator:** AVD `pixel`, Android 15 / API 35, `emulator-5556`, booted with `-no-snapshot -no-boot-anim -gpu software`.  
**APK:** current source built successfully with `PRIMER_API_ORIGIN=http://10.0.2.2:37949`; SHA-256 is in `apk-sha256.txt`.  
**Real service:** `make tasks-endpoints` reported web `http://127.0.0.1:37949/`; real API container reported `TASKS_MODEL_PROVIDER=disabled`.

## Result

**FAIL — do not ship or promote Android connected automation.**

The mandated prerequisite legitimate paired student session could not be established. The exact caller-designated image `.paseo-e2e/phase2-tasks-schedules/android-fresh-parent-qr.png` was selected through Android's real system Photo Picker after exercising the CameraX primary path, without manual code entry, QR regeneration, direct pairing API use, private seed, or mock. The app then returned the user-visible error:

> `Pairing failed. Check the server and try again.`

This is not a claimed successful pair. Because the device remained unpaired, all server-derived identity/task/occurrence state, persistence, parent-decision, and authorization checks are **not run** rather than inferred.

## Concrete evidence

| Acceptance item | Observation | Evidence |
|---|---|---|
| Fresh current debug APK / real origin | Build passed with the required emulator origin; generated debug `BuildConfig` contains `http://10.0.2.2:37949`. | `apk-build.txt`, `apk-sha256.txt`, `run-finish-state.txt`, `fresh-adb-install.txt` |
| `TASKS_MODEL_PROVIDER=disabled` | Directly observed in the live API Compose container. | `run-start-and-service.txt` |
| CameraX is primary | Fresh app UI places **Scan pairing QR** before import. The actual system camera permission was requested and granted; CameraX scanner then visibly ran. | `fresh-unpaired-real.uia.xml`, `camerax-primary-system-permission.uia.xml`, `camerax-primary-running.png`, `camerax-primary-running.uia.xml` |
| Real CameraX runtime | Camera service recorded active client `com.aleksclark.primertasks`, Preview + ImageAnalysis output streams, and 51 produced frames. | `camerax-primary-running-dumpsys.txt`, `camerax-primary-running-logcat.txt` |
| Exact QR via Android System Photo Picker / SAF | The exact designated source image hash is `c15cb9dd16ad9eb883ac270af537a8cef47cf48e26840f406541c14a2dbadc2b`. It was pushed only to shared Pictures/media-scanned, and the system Photo Picker (`com.google.android.providers.media.module`) showed one sole Recent image and its privacy copy, then that tile was selected. | `run-start-and-service.txt`, `qr-photo-picker-commands.txt`, `system-photo-picker-exact-qr.png`, `system-photo-picker-exact-qr.uia.xml`, `system-photo-picker-exact-qr-window.txt`, `adb-commands.txt` |
| Pairing result | Selection returned to the app and produced the visible generic pairing failure. No paired identity was displayed or stored. | `after-exact-qr-selected.png`, `after-exact-qr-selected.uia.xml`, `app-files-after-failed-pair.txt`, `after-exact-qr-selected-logcat.txt` |
| Parent browser gate | Chrome DevTools MCP could not obtain its isolated profile: `The browser is already running for /home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile.` No browser was attached to or killed. | reviewer MCP action/result (not a product artifact) |

The CameraX log includes expected emulator lens-validation warnings because this image exposes only camera ID 1, but the live Camera service evidence above proves the primary CameraX Preview and ImageAnalysis streams were actually open and producing frames. No camera-frame injection or virtual-scene QR experiment was used.

## Required checks not run (fail closed)

- paired server-derived Parent-A student identity;
- Today and Upcoming rendering and occurrence detail;
- pending → start → awaiting-verification;
- public parent UI reject → student retry → public parent approve / checked durable decision;
- refresh and force-stop/relaunch paired persistence;
- skipped/cancelled occurrence visibility;
- foreign occurrence and forged-completion denial;
- parent-browser public UI decisions (also independently unavailable due to the browser MCP profile blocker).

Running direct API calls for any of these would violate the assigned exploratory boundary, so none were substituted.

## Exact blocker and recommendation

**Blocker:** On a genuinely fresh Pixel AVD, the exact designated parent-rendered QR was available in and selected from Android's real system Photo Picker, but the app's real pairing flow ended in `Pairing failed. Check the server and try again.` Consequently no authorized paired device/session existed. The exact server status is not exposed by the app's generic error and the API container emitted no request line in the collected log; this run does not speculate whether the one-use material was expired or another non-401/403 pairing failure occurred.

**Recommendation: BLOCK shipment and do not promote automation.** Provide a still-valid public-parent-rendered QR while Chrome DevTools MCP's isolated parent profile is available (or otherwise restore a legitimate paired session). Then re-run every unexecuted row through the real student and public parent UIs only. Do not use a manually transcribed payload, direct pairing endpoint, or private seed as a workaround.
