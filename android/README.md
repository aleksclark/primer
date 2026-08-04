# Primer TV — Android client

The student-facing half of [Video As Instruction](../agent_docs/plans/video-as-instruction.md).
One APK runs on the tablet and on the living-room Android TV box; the shell is
chosen at runtime from `UiModeManager.currentModeType`.

This phase covers **on-demand viewing only**. The programmed channel (a
scheduled linear stream with a fully locked player) is a later phase.

## Modules

| Module | Contents |
|--------|----------|
| `core` | Pure Kotlin/JVM: API client, domain model, playback session state machine, watch-time accounting. No Android dependencies, so all of it is unit-testable on the JVM. |
| `app`  | Android: Compose UI (tablet + leanback), ExoPlayer host, DataStore persistence. |

Keeping the interesting logic in `core` is deliberate: the grant lifecycle,
heartbeat cadence, and watch-once rules are tested without an emulator.

## Build

Requires JDK 17 (AGP 8.7 rejects newer JDKs) and the Android SDK.

```bash
cd android
./gradlew test           # unit tests, no device needed
./gradlew assembleDebug  # app/build/outputs/apk/debug/app-debug.apk
```

Production versions are supplied by `primerVersionCode` / `primerVersionName`
Gradle properties (or `PRIMER_ANDROID_VERSION_CODE` /
`PRIMER_ANDROID_VERSION_NAME`). Version codes must increase for every update.
Production signing is enabled only when all four environment variables are set:
`PRIMER_ANDROID_KEYSTORE`, `PRIMER_ANDROID_STORE_PASSWORD`,
`PRIMER_ANDROID_KEY_ALIAS`, and `PRIMER_ANDROID_KEY_PASSWORD`. The equivalent
Gradle properties are `primerSigningStoreFile`, `primerSigningStorePassword`,
`primerSigningKeyAlias`, and `primerSigningKeyPassword`; keep them in the ignored
`keystore.properties` file and pass `-PprimerSigning...` when building locally.
Devices must be provisioned initially with that same production signing identity;
Android will reject later APKs signed with another key.

If Gradle cannot find the SDK, create `android/local.properties`:

```
sdk.dir=/path/to/Android/Sdk
```

The APK ships `arm64-v8a` and `armeabi-v7a`, which covers both the tablet and
the RK3318 box.

## Sideloading to the TV box

The T9 box has no Play Store, so install over ADB. Enable **Developer options →
USB debugging / Network debugging** on the box first, then:

```bash
adb connect 192.168.1.50:5555        # the box's LAN address
adb -s 192.168.1.50:5555 install -r app/build/outputs/apk/debug/app-debug.apk
adb -s 192.168.1.50:5555 shell monkey -p com.aleksclark.primer.tv 1
```

`-r` reinstalls in place and keeps the pairing. Use `adb logcat` for
troubleshooting.

The app declares both `LAUNCHER` and `LEANBACK_LAUNCHER`, so it appears in the
tablet's app drawer and on the TV home row.

## Android 9 dedicated-device provisioning

Kiosk behavior is deliberately gated by both TV mode and device ownership, so
the same APK remains a normal app on tablets. Device ownership can only be set
on a factory-reset/unprovisioned device. For development provisioning:

```bash
adb install app/build/outputs/apk/debug/app-debug.apk
adb shell dpm set-device-owner \
  com.aleksclark.primer.tv/.app.admin.PrimerDeviceAdminReceiver
adb shell dpm list-owners
```

After provisioning, `MainActivity` allowlists only Primer TV for lock task and
enters lock task whenever it resumes. The device restrictions block factory
reset, safe boot, extra users, overlays, and removable media, but intentionally
do **not** restrict Wi-Fi/network repair, Bluetooth/audio/volume, or package
installation. Validate these APIs on the production RK3318 firmware because
some Android 9 TV ROMs have incomplete device-policy implementations.

Device-owner updates use a `PackageInstaller` session for silent same-package
replacement. Downloads are size/checksum checked and must contain the same
package, a newer published version, and a compatible signing certificate.
Unmanaged devices, and device-owner ROMs that reject silent sessions, use the
interactive system installer fallback.

## Pairing

1. In the admin SPA, open **Devices** and register the device to get a pairing
   code.
2. Launch the app. Enter the server address (`tv.local:8081`, or paste the admin
   URL — a trailing `/api/v1` is stripped automatically) and the code.
3. The device exchanges the code for a token, stored in DataStore. The code is
   single-use; re-issue one from the admin UI to pair again.

**Settings → Unpair** forgets the token but keeps the server address.

## Playback rules

Enforced by the server; the app reflects them rather than deciding them.

- **Educational / mixed** — pause and seek. This is study material, so
  re-watching a passage is expected.
- **Entertainment** — pause, but **no seeking**. These are rationed by the
  server's watch-once ledger, and scrubbing to the last minute would spend the
  single viewing without watching it. Seek commands are withdrawn from the
  player itself, so the transport controls, the D-pad, and any media session all
  obey the rule.

Progress is reported every ~30s. Backgrounding stops the heartbeats but keeps
the session open, so an interrupted film resumes on the same grant instead of
spending a second play.

## Direct play only

The app never asks Jellyfin to transcode. The RK3318 box cannot keep up with a
server-side transcode, and a NAS usually cannot either, so the library must be
in a format the box decodes in hardware:

| | Supported |
|---|---|
| Video | H.264 (AVC), H.265 (HEVC), up to 4K |
| Container | MKV, MP4 |
| Audio | AAC, MP3; AC3/EAC3 depends on the box's licensing |

The admin SPA flags items that fail this check, and the server refuses to offer
them in the catalog — an unplayable title should never reach the student as a
black screen.
