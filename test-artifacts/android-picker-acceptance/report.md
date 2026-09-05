# Android Photo Picker pairing acceptance — PASS

**Validated base:** `b7f83f5e8a0fec746c5bb112c65136a4902737d` (device profile/checklist route alignment), after `06544ab` INTERNET permission/build assertion.

**Result:** **PASS.** Every requested picker acceptance observation completed through real public UI boundaries. No product source was edited by this acceptance leaf.

## Fallback authorization

The primary CameraX route was opened and observed first. The documented upstream Emulator 36.4.10 VirtualScene poster-propagation blocker remains at `test-artifacts/android-foundation-remediation/gateb-systematic-20260819T234500Z/report.md`; therefore this run made no VirtualScene/webcam pose or injection attempt and used the visible Android system Photo Picker fallback.

## Executed public flow

1. Started fresh isolated Stacklane-labelled Compose instance `android-picker-acceptance-final`; host forwarding exposed only the live web `/api` proxy to the emulator (`stack-transport.sanitized.txt`).
2. In a fresh Chrome DevTools MCP Parent A context, visibly completed sign-in, created **Android Picker Final Student**, opened it, and issued a new QR (`parent-login.snapshot.txt`, `first-student-created.snapshot.txt`, `first-student-detail.snapshot.txt`). The exact rendered QR image element lived only at `/dev/shm/android-picker-final-qr-1.png`; its decode output was suppressed (`qr1-validation.sanitized.txt`).
3. Built the current APK. The APK manifest inventory includes CAMERA and INTERNET, and `allowBackup=false`/`fullBackupContent=false` are present (`apk-build.txt`, `apk-permissions.sanitized.txt`, `apk-manifest-flags.sanitized.txt`).
4. Installed on a wiped Pixel. The unpaired UI exposed primary **Scan pairing QR** and accessible secondary **Import pairing QR image** (`first-unpaired.ui.xml`). The primary scanner was opened first, with real permission flow. CameraX Preview + ImageAnalysis, active `com.aleksclark.primertasks` camera client, and output streams were observed (`primary-camerax-scanner.ui.xml`, `primary-camerax-logcat.sanitized.txt`, `primary-camerax-dumpsys.sanitized.txt`).
5. Pushed the unchanged exact QR only into shared Pictures and selected it through the real system Photo Picker (`first-system-photo-picker.ui.xml`). The app completed real pair → device-profile → device-checklist, rendering **Android Picker Final Student** and **Nothing assigned yet** (`first-paired.ui.xml`).
6. Force-stop/relaunch and then full emulator reboot/relaunch each reloaded the bound profile and empty checklist through live requests (`first-after-force-stop.ui.xml`, `first-after-reboot.ui.xml`).
7. A separate writable Pixel-6 clone was wiped and booted while the first Pixel remained paired (`second-pixel-avd.sanitized.txt`, `second-emulator-boot.sanitized.txt`). It selected the same unchanged QR through its real Photo Picker and visibly received **Pairing failed. Check the server and try again.** without bound UI (`second-replay-denial.ui.xml`). This is the Android client's non-401/403 handling of the real one-use replay denial; no second usable device was created.
8. Parent A archived the original student through real browser UI (`parent-after-archive.snapshot.txt`). The first emulator’s next app launch live request visibly reported **This device pairing is no longer active. Scan a new QR code.** and cleared to the re-pair UI (`first-after-archive-revoked.ui.xml`).
9. Parent A created **Android Picker Replacement Student**, issued a new QR, and the first emulator selected its exact rendered image through the same Photo Picker flow. It re-paired successfully and rendered the replacement name plus empty checklist (`replacement-student-created.snapshot.txt`, `replacement-student-detail.snapshot.txt`, `qr2-validation.sanitized.txt`, `first-repaired.ui.xml`).
10. In the paired replacement state, run-as/private-storage, logcat, package/dumpsys, and Android backup-manager checks were executed without emitting credentials. `bmgr backupnow` returned **Backup is not allowed** and marker checks found no literal bearer/token/QR-payload/image/URI plaintext markers (`paired-private-storage.sanitized.txt`, `paired-logcat.sanitized.txt`, `paired-package-dumpsys.sanitized.txt`, `backup-manager.sanitized.txt`).

## Acceptance matrix

| Observation | Result |
|---|---|
| Parent A login, fresh student, fresh visible QR | PASS |
| Exact web-rendered QR selected through system Photo Picker | PASS |
| Unpaired primary CameraX + accessible secondary import | PASS |
| CameraX integration observed before fallback | PASS |
| Imported decode/parser and real pair API path | PASS |
| Bound student name and empty checklist | PASS |
| Force-stop/relaunch persistence through live requests | PASS |
| Emulator reboot persistence through live requests | PASS |
| Second fresh wiped Pixel same-image replay denial/no usable second device | PASS |
| Parent archive/revoke and first device next-request rejection/clear re-pair state | PASS |
| Fresh replacement student/QR and first-device picker re-pair | PASS |
| Paired private storage/logcat/dumpsys/backup checks | PASS |
| `allowBackup=false` and no plaintext secret-marker evidence | PASS |

## Exact sanitized command forms

```text
STACKLANE_INSTANCE=android-picker-acceptance-final ./primer-tasks/scripts/dev check
STACKLANE_INSTANCE=android-picker-acceptance-final ./primer-tasks/scripts/dev up
cd primer-tasks/android && PRIMER_API_ORIGIN=http://10.0.2.2:39081 ./gradlew --no-daemon assembleDebug
$HOME/Android/Sdk/emulator/emulator -avd pixel -wipe-data -no-snapshot -no-boot-anim -gpu software -port 5556
$HOME/Android/Sdk/emulator/emulator -avd pixel-picker-acceptance-second -wipe-data -no-snapshot -no-boot-anim -gpu software -port 5558
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 install -r primer-tasks/android/app/build/outputs/apk/debug/app-debug.apk
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 push /dev/shm/android-picker-final-qr-1.png /sdcard/Pictures/parent-web-pairing-qr.png
$HOME/Android/Sdk/platform-tools/adb -s emulator-5558 push /dev/shm/android-picker-final-qr-1.png /sdcard/Pictures/parent-web-pairing-qr.png
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap <visible-import-action>
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell input tap <system-photo-picker-thumbnail>
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell am force-stop com.aleksclark.primertasks
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 reboot
$HOME/Android/Sdk/platform-tools/adb -s emulator-5556 shell bmgr backupnow com.aleksclark.primertasks
```

Chrome DevTools MCP used `new_page`, `take_snapshot`, `click`, `fill`, and `take_screenshot` against the real browser parent UI. No direct pair endpoint, manual-code entry, API seeding, payload reconstruction, internal image injection, or test seam was used.

## Retention and cleanup

- Exact QR images, decoded payload output, cookies, tokens, shared-Pictures copies, and QR-thumbnail screenshots were deleted.
- `image-retention-scan.sanitized.txt` and `cleanup-verification.sanitized.txt` show no retained decodable QR image or QR tmpfs file.
- Both Pixel emulators, the temporary second-Pixel AVD clone, host forwards, and Stacklane instance were stopped/removed. No SIGKILL was used.
