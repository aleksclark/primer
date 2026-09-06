# Primer TV — Android client

This Gradle build also includes the initial **Primer Student device-owner
qualification** target (`:app-student`), with `:core-device-policy` and
`:core-updates`. See the [A16 qualification runbook](../agent_docs/runbooks/android-a16-provisioning.md)
and [physical-device evidence](../test-artifacts/android-student/device-owner-qualification.md).
It is not yet the full Student/Control platform, and silent Student updates remain
an open verification gate on the tested handset/signing configuration.

The student-facing half of [Video As Instruction](../agent_docs/plans/video-as-instruction.md).
One APK runs on the tablet and on the living-room Android TV box; the shell is
chosen at runtime from `UiModeManager.currentModeType`.

This phase covers **on-demand viewing only**. The programmed channel (a
scheduled linear stream with a fully locked player) is a later phase.

## Modules

| Module | Contents |
|--------|----------|
| `core` | Pure Kotlin/JVM: API client, domain model, playback session state machine, watch-time accounting. No Android dependencies, so all of it is unit-testable on the JVM. |
| `core-ui` | Shared System C Compose theme, fonts, and primitives (`com.aleksclark.primer.ui`). Consumed by TV and Student; Control must use this library, not a copy. |
| `app`  | Android: Compose UI (tablet + leanback), ExoPlayer host, DataStore persistence. |
| `feature-tasks-student` | Migrated Tasks pairing/checklist/start/submit for Student. Entry: `StudentTasksRoute`. |
| `app-student` | Device-owner launcher. Tasks, approved apps, and parent maintenance; recovery stays in `StudentRuntime`. |

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

Self-update from `GET /app/release` requires a signed sidecar. Unsigned
metadata (no `release-manifest.json`) is a normal legacy server and the client
fails closed rather than installing. See [operator publication](#operator-publication-signed-tv-sidecar).

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

## Operator publication (signed TV sidecar)

Do not publish anything live from this checkout. The TV server never talks to
Tasks for this path: no Tasks DB, no Tasks token, no shared credential.

The Android client accepts an update only after `TvReleaseAdapter` verifies a
canonical `ReleaseManifest` with Tink Ed25519 over the pinned
`PRIMER_RELEASE_TRUST_ROOT` (`ed25519-v1`). Outer `/app/release` fields that do
not match the verified payload are rejected.

1. Build and sign the TV APK with the same production identity already on the box.
2. Set `TV_RELEASE_SIGNING_KEY` to the 64-byte Ed25519 private key whose public
   32 bytes are the pinned trust root (hex or base64url). Optional:
   `TV_AAPT2` / `TV_APKSIGNER` if those tools are not on `PATH`.
3. Write a **new empty staging directory** on the **same filesystem** as
   `TV_RELEASE_DIR`. `-out` is refused if it already exists, is nonempty, is
   the source APK, or is the live release path. The tool snapshots the APK
   first, then inspects/hashes/signs those exact bytes:

   ```bash
   make tv-release-sidecar SIDECAR_ARGS='-apk path/to/app-release.apk -out /srv/tv-releases/rel-$(date +%s)'
   ```

   The command inspects the snapshot with `aapt2`/`apksigner`, signs canonical
   `ReleaseManifest` JSON with Go `crypto/ed25519` (same field order as the
   Tasks publisher, without opening Tasks), and refuses to talk to a database.
   No native libraries is `[]` (universal), not invented ARM ABIs. Multi-signer
   APKs and `versionCodeMajor` are rejected.
4. Confirm `packageName` is `com.aleksclark.primer.tv`, `version` matches the
   APK `versionCode`, `sha256`/`byteSize`/`signerSha256`/`minSdk` match the
   APK, and `signingKeyId` is `ed25519-v1`. Payload base64url is capped at 16KiB.
5. Point `TV_RELEASE_DIR` at a **symlink** whose target is an immutable
   directory (`primer-tv.apk`, `version`, `release-manifest.json`). Replace the
   symlink atomically on the same filesystem (`ln -sfn`). `mv current prev &&
   mv staging current` is not atomic and must not be used. One-time migration
   if the current path is a real directory:

   ```bash
   # example only; do not run against a live household from this lane
   mv "$TV_RELEASE_DIR" "$TV_RELEASE_DIR.legacy"
   ln -s "$TV_RELEASE_DIR.legacy" "$TV_RELEASE_DIR"
   ln -sfn /srv/tv-releases/rel-NEW "$TV_RELEASE_DIR"
   ```

The server resolves that symlink once per metadata or download request so one
call sees a coherent set. A later swap can still race a following download;
the Android client must fail integrity and retry, not install mixed bytes.

A missing sidecar stays the unsigned legacy shape. A present but malformed,
empty, or oversized sidecar is logged and still served unsigned; the client
will not install it. Configure `PRIMER_RELEASE_TRUST_ROOT` on the TV APK before
expecting a signed update to be offered. Full streaming/update acceptance on
hardware remains open.

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
in a format the client can decode without a server-side re-encode:

| | Supported |
|---|---|
| Video | H.264 (AVC), H.265 (HEVC), up to 4K (SoC hardware) |
| Container | MKV, MP4 |
| Audio | AAC, MP3 (hardware MediaCodec); AC3, EAC3, DTS via bundled Media3 FFmpeg software fallback |

### Bundled AC3 / EAC3 / DTS (FFmpeg)

The T9 / RK3318 (and many other Rockchip TV boxes) ship without AC3/EAC3/DTS
MediaCodec decoders. The app vendors the official AndroidX Media3 1.4.1
`decoder_ffmpeg` extension built against FFmpeg 6.0 with **LGPL-compatible**
audio decoders only (`ac3`, `eac3`, `dca`). ExoPlayer is wired with
`DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON`, so platform AAC/MP3 stay
on hardware and FFmpeg is used only when MediaCodec cannot handle the track.

**Limits**

- Software decode burns CPU. Multichannel AC3/EAC3/DTS is fine on RK3318 for
  typical TV content; very high-bitrate tracks can still hitch.
- **TrueHD / Atmos truehd is not enabled** (heavy and uncommon in this library).
- DTS-HD MA core may play via the DTS (`dca`) decoder; full lossless MA is not
  a target.
- x86 / x86_64 `.so` files are included in the AAR for emulator completeness;
  the APK still ships only `arm64-v8a` and `armeabi-v7a`.

Rebuild / provenance: see
[`third_party/media3-ffmpeg/`](third_party/media3-ffmpeg/README.md).

The admin SPA flags items that fail the direct-play allowlist, and the server
refuses to offer them in the catalog — an unplayable title should never reach
the student as a black screen.
