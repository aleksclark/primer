# Android foundation remediation acceptance — BLOCKED

**Role:** dedicated Android emulator acceptance reviewer (exploratory only; no promoted automation authored)

**Run:** 2026-08-19 UTC  
**Source:** current worktree `cba13993dfe8863b795babb51de543711d18ce84` plus uncommitted remediation changes present at run start (full status in `test-artifacts/android-foundation-remediation/environment.txt`).

## Result

**Overall: BLOCKED — do not treat any unobserved paired behavior as PASS.**

The real debug APK builds and launches on a freshly wiped `pixel` AVD from the explicit SDK at `$HOME/Android/Sdk`. The actual parent web flow / rendered QR could not be driven in this run because the required Chrome DevTools MCP browser profile was already exclusively occupied; its `list_pages` and `new_page` calls both failed before navigation. I did not attach to or kill that existing browser. Consequently no QR was rendered/scanned, no device token was issued, and the paired-state persistence/replay/revocation cases below are **not observed**.

The user-directed current real stack endpoints were reachable:

- Web: `http://127.0.0.1:37574/` → HTTP 200
- Direct API: `http://127.0.0.1:37578/health` → HTTP 200
- Issuer proxy: `http://127.0.0.1:37574/issuer/health` → HTTP 200 (after Vite was ready)

Evidence: `test-artifacts/android-foundation-remediation/docker-primer-tasks-ports.txt`, `web-issuer-proxy-requests.txt`, and `login-and-issuer-probe.log`.

## Phase 1 criteria status

| Required observation | Status | Evidence / limitation |
|---|---|---|
| Real APK built from current source | **PASS** | `apk-build.log`; SHA-256 in `apk-sha256.txt` (`277632…3591f`). |
| Fresh wiped Pixel emulator and real APK installation | **PASS** | Emulator was launched with `-avd pixel -wipe-data`; `emulator-fresh-boot.txt`, `adb-install-first.txt`, `fresh-unpaired.png`, and `fresh-unpaired.uia.xml`. |
| Fresh app displays a clear pairing state | **PASS** | UI XML contains “Pair this device” and “Scan the one-use QR code shown by your parent.” |
| Scanner screen / real CameraX camera session can be opened | **PASS** | Camera permission was granted through the OS dialog; live screen XML has `PreviewView`; `scanner-live-camera-state.txt` identifies `com.aleksclark.primertasks` as active client. No QR was supplied. |
| Parent browser UI signs in, creates/selects a student, and renders a real Stacklane/web QR | **BLOCKED** | Required DevTools MCP could not start because its profile was in use. No replacement or API-only claim was used. See browser MCP errors in this report’s command log note and `login-and-issuer-probe.log` for endpoint-only probes. |
| Emulator scans that **web-rendered** QR and exchanges it once | **NOT OBSERVED** | Parent web UI/QR could not be driven; no QR camera input was substituted. |
| Bound student display name and empty checklist | **NOT OBSERVED** | Requires successful real pairing. |
| Bound state survives force-stop/process death | **NOT OBSERVED** | The captured force-stop/relaunch was intentionally unpaired only: `unpaired-force-stop-relaunch.txt`, `unpaired-after-force-stop.png`, `unpaired-after-force-stop.uia.xml`. It is not evidence for bound persistence. |
| Bound state survives emulator reboot | **NOT OBSERVED** | An unpaired reboot smoke attempt/relaunch occurred (`unpaired-reboot-relaunch.txt`, `unpaired-after-reboot.*`), but no paired credential existed. It is not evidence for paired persistence. |
| Second fresh emulator is denied QR replay | **NOT OBSERVED** | No successful first redemption exists to replay. A second emulator was not launched merely to simulate a failure. |
| Parent archive/revocation rejects next device request and clears to re-pair | **NOT OBSERVED** | No device existed to revoke. |
| No plaintext token / QR / code in **paired** app storage, logcat, backup/export | **NOT OBSERVED** | No token or QR/code reached the app. Unpaired-state observations are recorded separately and must not be overgeneralized. |

## Commands executed

All Android commands explicitly used `$HOME/Android/Sdk`:

```bash
export ANDROID_HOME="$HOME/Android/Sdk"
export ANDROID_SDK_ROOT="$HOME/Android/Sdk"
cd primer-tasks/android
./gradlew --no-daemon assembleDebug

$HOME/Android/Sdk/emulator/emulator \
  -avd pixel -wipe-data -no-snapshot -no-boot-anim \
  -gpu swiftshader_indirect -camera-back emulated -port 5556

$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 wait-for-device
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell getprop sys.boot_completed
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 install -r \
  primer-tasks/android/app/build/outputs/apk/debug/app-debug.apk
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell am start -W \
  -n com.aleksclark.primertasks/.MainActivity

# UI evidence
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 exec-out screencap -p > …png
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell uiautomator dump /sdcard/…xml
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 pull /sdcard/…xml …uia.xml

# Permission/scanner exercise, then unpaired process-death smoke check
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap …
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell am force-stop com.aleksclark.primertasks
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 reboot

# Security inspection
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell run-as \
  com.aleksclark.primertasks sh -c 'find . -type f …'
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell bmgr backupnow --monitor \
  com.aleksclark.primertasks
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 logcat -d -v threadtime
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell dumpsys package \
  com.aleksclark.primertasks
```

The exact launch/build command outputs are retained rather than redacted/reconstructed in the artifacts below.

## Captured Android evidence

- APK build and provenance: `test-artifacts/android-foundation-remediation/apk-build.log`, `apk-path.txt`, `apk-sha256.txt`, `environment.txt`
- Fresh-wipe emulator boot/install/launch: `emulator-pixel.log`, `emulator-fresh-boot.txt`, `adb-install-first.txt`, `adb-launch-first.txt`
- Fresh unpaired UI: `fresh-unpaired.png`, `fresh-unpaired.uia.xml`
- Permission and scanner UI: `camera-permission.png`, `camera-permission.uia.xml`, `camera-system-permission*.png`, `camera-system-permission*.uia.xml`, `scanner-live-no-qr.png`, `scanner-live-no-qr.uia.xml`
- Real camera connection: `camera-permission-after-grant.txt`, `scanner-live-camera-state.txt`
- Unpaired force-stop/relaunch evidence: `unpaired-force-stop-relaunch.txt`, `unpaired-after-force-stop.png`, `unpaired-after-force-stop.uia.xml`
- Unpaired reboot/relaunch evidence only: `unpaired-reboot-relaunch.txt`, `unpaired-after-reboot.png`, `unpaired-after-reboot.uia.xml`, `reboot-attempt-diagnostic.txt`
- Logcat: `fresh-unpaired.logcat.txt`, `scanner-live-no-qr.logcat.txt`, `final-logcat.txt`, `final-logcat-sensitive-scan.txt`
- Private-storage and backup inspection: `fresh-unpaired-storage-and-backup.txt`, `unpaired-reboot-storage.txt`, `package-dumpsys.txt`
- Camera-input capability review (no fixture injection used): `camera-injection-capability.txt`, `host-camera-capability.txt`

## Security observations that were actually made

1. **Unpaired private storage:** `run-as` found only empty app directories plus `files/profileInstalled`; no persisted DataStore token/code/QR payload existed in the unpaired state. See `fresh-unpaired-storage-and-backup.txt` and `unpaired-reboot-storage.txt`.
2. **Backup/export:** the installed app’s real backup-manager attempt returned `Backup is not allowed` / `BACKUP_DISABLED`. This corroborates `android:allowBackup="false"` for this installed APK. See `fresh-unpaired-storage-and-backup.txt` and `package-dumpsys.txt`.
3. **Logcat:** captured logcat was retained and a case-insensitive sensitive-marker scan was recorded without deliberately emitting a secret. It proves only that no paired secret was present because no pairing occurred; it does **not** prove that post-pair logging is clean. See `final-logcat-sensitive-scan.txt`.
4. **QR/camera:** CameraX received an actual camera client session after the OS “While using the app” permission grant. The QR decoder did not receive an actual QR, and the physical webcam feed was not replaced with a test fixture.

## Blocker for rerun

Release/stop the process owning the Chrome DevTools MCP profile (or provide an unused MCP browser session). Then rerun the full real-stack sequence without API/manual-code substitution:

1. Parent browser UI login → create/select student → issue QR at `http://127.0.0.1:37574/`.
2. Scan that rendered QR on the wiped Pixel through the camera, verify bound name and empty checklist.
3. Force-stop, then reboot the same emulator; prove the bound state reloads through live profile/checklist requests.
4. Present the used QR to a second separately wiped Pixel; capture the visible denial and prove no second device exists.
5. Archive/revoke through the parent web UI; make the first device’s next live request and capture the rejection plus clear re-pair UI.
6. Repeat `run-as`, logcat, and backup/export inspection **after pairing**. Do not log or write raw bearer/code values into artifacts.

No test suite, Playwright spec, UI test, or other promoted automation was written in this review.

## Cleanup

The reviewer-owned isolated Compose project (`STACKLANE_INSTANCE=android-foundation-remediation`) was stopped with `primer-tasks/scripts/dev down`, preserving its volumes. The user-directed `primer-tasks-remed1` stack on ports 37574/37578 remained running. The emulator console’s `adb emu kill` returned HTTP 400, so it was cleanly powered down with:

```bash
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell reboot -p
```

`cleanup.log` records both the failed console command and the successful offline poll.

## Rerun 2 — current stack only (2026-08-19 UTC)

**Result: BLOCKED.** Per the rerun instruction, no reviewer-owned Compose or Stacklane instance was created. The only live product target used was the user-supplied real web/API proxy:

- `http://127.0.0.1:37574/` → HTTP 200
- `http://127.0.0.1:37574/api/health` → HTTP 200

A second genuinely fresh Pixel was started from the explicit SDK using the real host webcam (not an app test seam):

```bash
$HOME/Android/Sdk/emulator/emulator \
  -avd pixel -wipe-data -no-snapshot -no-boot-anim \
  -gpu swiftshader_indirect -camera-back webcam0 -port 5556
```

The rebuilt current APK installed successfully, the Android permission dialog was accepted, and CameraX opened the emulator camera device. `rerun-2/scanner-awaiting-real-qr.uia.xml` contains the live scanner `ViewFactoryHolder`, while `rerun-2/scanner-camera-state.txt` names `com.aleksclark.primertasks` as the active client for camera device `10` and records received preview frames. This confirms the scanner is reading a genuine camera source.

### Exact rerun blocker

Despite the instruction that Chrome MCP was accessible, this agent's available MCP calls remained unavailable before browser navigation:

```text
chrome-devtools_list_pages ->
Error: The browser is already running for
/home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile.
Use --isolated to run multiple browser instances.

Cause: The browser is already running for
/home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile.
Use a different `userDataDir` or stop the running browser first.
```

The running owner is `chrome-devtools-mcp` in this worktree; I did not attach to it, terminate it, fabricate a new parent session, directly issue a pairing code, or substitute a manual code entry. The recorded browser QR (`4454344E65`) was already redeemed/expired in the documented prior browser flow and therefore cannot be used for an Android **pair** assertion. It was not replayed as a substitute for a fresh real QR.

Because there was no accessible authenticated parent page from which to issue a fresh web-rendered QR, the webcam had no valid live QR to feed to CameraX. No scan event, `/api/device/pair` request, device token, or bound student state was created during this rerun. Therefore the following remain **NOT OBSERVED** in this rerun: Android profile/checklist, process and reboot persistence of a bound device, second-emulator replay denial, parent archive's next-device-request rejection/re-pair UI, and post-pair storage/logcat/backup leakage inspection.

Rerun evidence is in `test-artifacts/android-foundation-remediation/rerun-2/`:

- `apk-build.log`, `apk-sha256.txt`, `fresh-boot.txt`, `adb-install.txt`, `adb-launch.txt`
- `permission.uia.xml`, `scanner-awaiting-real-qr.png`, `scanner-awaiting-real-qr.uia.xml`, `scanner-camera-state.txt`, `scanner.logcat.txt`
- `current-stack-health.txt`, `emulator-pixel.log`

To unblock a valid acceptance run, release the Chrome MCP session for this leaf (or provide an MCP context explicitly usable by it), then issue a **new** QR through the visible Parent A web UI and hold/display that live rendered QR to the active physical webcam. The next run must use that camera-fed QR; it must not use text entry, direct API issuance, an expired recorded code, or a scanner fixture that bypasses CameraX.

## Rerun 3 — mandatory retry after browser PASS notification (2026-08-19 UTC)

**Result: BLOCKED before a fresh QR could be issued.** I did not accept the previous lock without retrying. I made both a direct MCP page-list request and an isolated-context new-page request to the required current web origin:

```text
chrome-devtools_list_pages -> profile-lock error
chrome-devtools_new_page(
  url=http://127.0.0.1:37574/,
  isolatedContext=android-foundation-final-retry
) -> the same profile-lock error
```

Both returned the exact error that `/home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile` was already running and requested a different user-data directory or termination of the owner. Because the browser exploratory agent had reported cleanup complete, I inspected the owner and found a stale `chrome-devtools-mcp` process with its browser child, both started in this worktree on 2026-08-18. I sent **TERM only** to that stale MCP parent; its browser exited within two seconds (`rerun-3/chrome-mcp-retry-before-cleanup.txt`). I then immediately retried `chrome-devtools_list_pages`; the exact same profile-lock error remained. Diagnostics showed that the MCP adapter had spawned a new browser using that same profile but still could not service the request.

No browser page was accessible to issue a fresh QR. I did not re-use an expired code, invoke a direct API pairing endpoint, type a pairing code, or falsely claim scanner/pairing results. Thus all Android paired-state checks remain **NOT OBSERVED**, not PASS. This is now an MCP adapter/profile lifecycle failure after both the required retry and a conservative stale-owner cleanup, rather than a refusal to retry.

## Rerun 4 — fresh browser QR and camera-injection result (2026-08-19 UTC)

**Result: BLOCKED — the Android scanner did not decode the QR and pairing did not succeed.**

The authorized Singleton cleanup succeeded after a strict process-owner check, and one MCP `new_page` retry worked. The real parent UI at `http://127.0.0.1:37574/` completed Parent A S256 PKCE sign-in, created **Android Camera Acceptance Student**, and visibly issued a new one-use pairing QR. Browser evidence is retained in `.paseo-e2e/tasks-foundation-remediation/android-rerun-*.snapshot.txt` and the real QR rendering in `android-rerun-fresh-qr-full.png`.

The QR camera experiment used the **rendered QR image itself** as a V4L2 loopback feed; no code was typed and no raw QR payload was recreated. `rerun-4/qr-loopback-sample-running.png` proves that the loopback capture device carried the rendered QR image. However, the emulator was launched with `-camera-back webcam9`, and its own log states:

```text
WARNING | Camera 'webcam9' is not found in the list of connected cameras.
Use '-webcam-list' emulator option to obtain the list of connected camera names.
```

The resulting Android facts are decisive:

- `rerun-4/after-live-qr-scan.uia.xml` shows the app back at **Pair this device**, not a bound student/profile/checklist screen.
- `rerun-4/after-live-qr-camera-state.txt` reports **Number of camera devices: 0** and no active client.
- `rerun-4/after-live-qr-scan.logcat.txt` contains no successful pairing/profile transition; no device token was persisted.

Therefore this run does **not** establish that CameraX decoded the browser-rendered QR, and pairing did not succeed. Per reviewer instruction, no further injection/diagnostic loop was attempted. The interrupted follow-up attempt is not evidence and is deliberately excluded from the result. Process/reboot persistence, second-emulator replay, archive/revocation re-pair state, and post-pair storage/logcat/backup inspection remain **NOT OBSERVED**.

## Final focused webcam0 attempt (2026-08-19 UTC)

**Result: BLOCKED — CameraX received frames but did not decode the fresh browser-rendered QR. No further scan attempts were made.**

This was the requested single focused rerun:

1. Chrome MCP visibly issued another fresh QR from the authenticated Parent A page. The test feed was cropped from the actual full-page browser screenshot (`.paseo-e2e/tasks-foundation-remediation/android-final-fresh-qr-full.png`), not recreated from a payload or entered as text.
2. The rendered QR was fed to `/dev/video9` by a V4L2 loopback writer. That loopback was bound over `/dev/video0` only for emulator camera discovery.
3. Before boot, `emulator -webcam-list` explicitly reported `Camera 'webcam0' is connected to device '/dev/video0' on channel 0 using pixel format 'YU12'` (`final-webcam0/emulator-webcam-list.txt`).
4. A freshly wiped Pixel was launched with `-camera-back webcam0`; the fresh APK was installed and the normal Android camera permission was accepted.
5. `final-webcam0/after-permission-scan-camera-state.txt` proves that CameraX opened camera device 10 for package `com.aleksclark.primertasks`, with active preview output streams and a latest received frame.
6. After 12 seconds of live frames, `final-webcam0/after-permission-scan.uia.xml` still showed **Scan pairing QR** and the live `ViewFactoryHolder`; it did not show a bound student, empty checklist, pairing error, or re-pair state. Thus the QR was not decoded and no `/device/pair`/profile success was observed.

This satisfies the requested webcam0 discovery and camera-frame verification but fails the required decode/pair outcome. The test stops here. Bound-state persistence, second-emulator replay denial, parent archive/revocation reaction, and post-pair token storage/logcat/backup inspection are **NOT OBSERVED** and are not marked PASS.

## Final rebuilt Hybrid/GlobalHistogram/ALSO_INVERTED attempt (2026-08-19 UTC)

**Result: BLOCKED — rebuilt scanner still did not decode a source image independently validated as QR-decodable. No further attempts were made.**

The requested newly rebuilt APK was assembled with the explicit SDK (`final-hybrid-global/apk-build.log`, `apk-sha256.txt`). Chrome MCP issued a new real Parent A QR. Its browser screenshot was cropped directly into the loopback feed; `zbarimg` successfully decoded that exact feed image before launch, with the payload deliberately suppressed (`final-hybrid-global/qr-validation.txt`).

The final fresh Pixel run used the same verified `webcam0` discovery gate and no text/API pairing substitute:

- `final-hybrid-global/emulator-webcam-list.txt` confirms `webcam0` maps to `/dev/video0`.
- The AVD was wiped and launched with `-camera-back webcam0`.
- `final-hybrid-global/after-scan-camera-state.txt` identifies a live CameraX client (`com.aleksclark.primertasks`) on camera device 10, active preview output streams, and a latest received frame.
- After 15 seconds, `final-hybrid-global/after-scan.uia.xml` still showed **Scan pairing QR** and the live preview holder. It did not transition to a bound profile/checklist, error, or re-pair UI.

Thus the upgraded in-app decoder did not emit a successful decode from the emulator's camera frames even though the exact source feed was independently QR-decodable and CameraX was receiving camera frames. Pairing did not occur. This is the final attempt requested; no process/reboot/replay/revocation/storage assertions are made for an unpaired device.
