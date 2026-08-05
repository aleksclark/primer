# Media3 FFmpeg extension (vendored)

Official AndroidX Media3 **1.4.1** `decoder_ffmpeg` module, built against
**FFmpeg 6.0** with LGPL-compatible audio decoders only. Vendored as an AAR so
CI and local builds are deterministic (no network native compile).

| | |
|---|---|
| AAR path | [`../../app/libs/media3-ffmpeg-decoder-1.4.1.aar`](../../app/libs/media3-ffmpeg-decoder-1.4.1.aar) |
| Media3 | tag `1.4.1` (`c35a9d62baec57118ea898e271ac66819399649b`) |
| FFmpeg | branch `release/6.0` (pinned at build time; see script) |
| NDK | r26b (`26.1.10909125`) — matches Media3 1.4.1 docs |
| ANDROID_ABI | `21` (must not exceed app `minSdk` 28) |
| Enabled decoders | `ac3`, `eac3`, `dca` (DTS) |
| ABIs in AAR | `arm64-v8a`, `armeabi-v7a`, `x86`, `x86_64` |
| App ABIs | `arm64-v8a`, `armeabi-v7a` only (Gradle `ndk.abiFilters`) |

## SHA256

```
dfc726c8bc9d9db02e15b160f886a03bdd6dd51e88ba2de89aa74caf5e8cd181  media3-ffmpeg-decoder-1.4.1.aar
```

Verify after rebuild:

```bash
sha256sum android/app/libs/media3-ffmpeg-decoder-1.4.1.aar
# update the digest in this file if the build environment produces a new bit-identical policy change
```

## Why vendor (not Jellyfin Maven)

`org.jellyfin.media3:media3-ffmpeg-decoder` is **GPL-3**. This tree stays on the
official Media3 extension + an LGPL FFmpeg configure (`--disable-everything` +
explicit `--enable-decoder=` for LGPL audio codecs only). No x264/x265/fdk-aac
or other GPL bits.

## Licenses / notices

| Component | License | Notice file |
|-----------|---------|-------------|
| Media3 Java/JNI (`decoder_ffmpeg`) | Apache-2.0 | [`notices/MEDIA3_APACHE-2.0.txt`](notices/MEDIA3_APACHE-2.0.txt) |
| FFmpeg (enabled audio decoders) | LGPL 2.1+ | [`notices/COPYING.LGPLv2.1`](notices/COPYING.LGPLv2.1), [`notices/COPYING.LGPLv3`](notices/COPYING.LGPLv3) |

The Android app packaging already excludes duplicate `META-INF/{AL2.0,LGPL2.1}`
resource collisions; these notice files are the human-readable source of truth
for redistribution.

## Rebuild

```bash
# From repo root. Requires JDK 17, NDK 26.1, yasm/nasm, cmake, ninja, git, curl.
./android/third_party/media3-ffmpeg/build-media3-ffmpeg.sh
```

The script:

1. Clones Media3 at tag `1.4.1` and FFmpeg `release/6.0` into a work dir
   (default `/tmp/primer-media3-ffmpeg-build`, override with `BUILD_ROOT=`).
2. Runs official `libraries/decoder_ffmpeg/src/main/jni/build_ffmpeg.sh` with
   `ENABLED_DECODERS=(ac3 eac3 dca)`.
3. Assembles `:lib-decoder-ffmpeg:assembleRelease` and copies the AAR to
   `android/app/libs/media3-ffmpeg-decoder-1.4.1.aar`.
4. Prints SHA256 and checks for `FfmpegAudioRenderer` + required decoder symbols.

## Runtime wiring

`PlayerHost` builds ExoPlayer with:

```kotlin
DefaultRenderersFactory(context)
    .setExtensionRendererMode(DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON)
```

`EXTENSION_RENDERER_MODE_ON` keeps MediaCodec preferred; FFmpeg is the fallback
when the platform has no decoder for the track (the T9 case for AC3/EAC3/DTS).

## Limits

- Software decode uses CPU — fine for typical TV bitrates on RK3318; not a
  substitute for hardware on every stream.
- TrueHD / full DTS-HD MA lossless are **not** enabled.
- Do not swap in a GPL FFmpeg build or the Jellyfin Maven artifact without a
  deliberate license review.
