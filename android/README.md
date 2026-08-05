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
