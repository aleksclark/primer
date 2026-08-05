#!/usr/bin/env bash
# Rebuild the vendored Media3 1.4.1 decoder_ffmpeg AAR (LGPL: ac3/eac3/dca).
# See README.md in this directory for provenance and license notes.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ANDROID="$(cd "${SCRIPT_DIR}/../.." && pwd)"
REPO_ROOT="$(cd "${REPO_ANDROID}/.." && pwd)"

MEDIA3_TAG="${MEDIA3_TAG:-1.4.1}"
# Commit pointed to by annotated tag 1.4.1 (not the tag object itself).
MEDIA3_COMMIT="${MEDIA3_COMMIT:-c35a9d62baec57118ea898e271ac66819399649b}"
FFMPEG_REF="${FFMPEG_REF:-release/6.0}"
# Tip of release/6.0 when this AAR was first produced; override to float.
FFMPEG_COMMIT="${FFMPEG_COMMIT:-3f92512fd1fd6f5e6d6eb45a156c352835314d69}"

NDK_PATH="${NDK_PATH:-/opt/android-sdk/ndk/26.1.10909125}"
HOST_PLATFORM="${HOST_PLATFORM:-linux-x86_64}"
ANDROID_ABI="${ANDROID_ABI:-21}"
ENABLED_DECODERS=(ac3 eac3 dca)
# Decoders that must never appear (Jellyfin full-GPL artifact / over-broad builds).
FORBIDDEN_DECODERS=(aac aac_latm alac flac mp3 mlp truehd pcm_alaw pcm_mulaw)

BUILD_ROOT="${BUILD_ROOT:-/tmp/primer-media3-ffmpeg-build}"
OUT_AAR="${OUT_AAR:-${REPO_ANDROID}/app/libs/media3-ffmpeg-decoder-1.4.1.aar}"
JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-17-openjdk}"
export JAVA_HOME
export ANDROID_HOME="${ANDROID_HOME:-/opt/android-sdk}"
export ANDROID_SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME}}"

echo "==> Build root: ${BUILD_ROOT}"
echo "==> NDK: ${NDK_PATH}"
echo "==> Decoders: ${ENABLED_DECODERS[*]}"
echo "==> Forbidden: ${FORBIDDEN_DECODERS[*]}"
echo "==> Output: ${OUT_AAR}"

if [[ ! -d "${NDK_PATH}" ]]; then
  echo "NDK not found at ${NDK_PATH}" >&2
  echo "Install with: sdkmanager \"ndk;26.1.10909125\"" >&2
  exit 1
fi

mkdir -p "${BUILD_ROOT}"
cd "${BUILD_ROOT}"

if [[ ! -d media3/.git ]]; then
  rm -rf media3
  git clone --depth 1 --branch "${MEDIA3_TAG}" https://github.com/androidx/media.git media3
fi
cd media3
git fetch --depth 1 origin "refs/tags/${MEDIA3_TAG}:refs/tags/${MEDIA3_TAG}" 2>/dev/null || true
git checkout -f "${MEDIA3_TAG}"
HEAD="$(git rev-parse HEAD)"
if [[ "${HEAD}" != "${MEDIA3_COMMIT}" ]]; then
  echo "ERROR: Media3 HEAD ${HEAD} != pinned commit ${MEDIA3_COMMIT}" >&2
  echo "Refusing to build from an unexpected Media3 revision." >&2
  exit 1
fi
echo "==> Media3 ${MEDIA3_TAG} @ ${HEAD}"

FFMPEG_MODULE_PATH="$(pwd)/libraries/decoder_ffmpeg/src/main"
mkdir -p "${BUILD_ROOT}/ffmpeg-src"
if [[ ! -d "${BUILD_ROOT}/ffmpeg-src/ffmpeg/.git" ]]; then
  rm -rf "${BUILD_ROOT}/ffmpeg-src/ffmpeg"
  git clone https://git.ffmpeg.org/ffmpeg.git "${BUILD_ROOT}/ffmpeg-src/ffmpeg"
fi
cd "${BUILD_ROOT}/ffmpeg-src/ffmpeg"
git fetch origin "${FFMPEG_REF}"
git checkout -f "${FFMPEG_COMMIT}"
FFMPEG_PATH="$(pwd)"
echo "==> FFmpeg @ $(git rev-parse HEAD)"

cd "${FFMPEG_MODULE_PATH}/jni"
rm -f ffmpeg
ln -s "${FFMPEG_PATH}" ffmpeg

echo "==> Building FFmpeg native libs (this takes several minutes)..."
./build_ffmpeg.sh \
  "${FFMPEG_MODULE_PATH}" \
  "${NDK_PATH}" \
  "${HOST_PLATFORM}" \
  "${ANDROID_ABI}" \
  "${ENABLED_DECODERS[@]}"

# Guard LGPL scope: official build_ffmpeg.sh must not have enabled GPL/nonfree.
# FFmpeg 6.x keeps license flags in config.h and decoder enables in config_components.h.
CONFIG_H="${FFMPEG_PATH}/config.h"
CONFIG_COMPONENTS="${FFMPEG_PATH}/config_components.h"
if [[ ! -f "${CONFIG_H}" || ! -f "${CONFIG_COMPONENTS}" ]]; then
  echo "ERROR: missing FFmpeg config after build (${CONFIG_H} / ${CONFIG_COMPONENTS})" >&2
  exit 1
fi
echo "==> Checking FFmpeg config: ${CONFIG_H} + ${CONFIG_COMPONENTS}"
if grep -E '^#define CONFIG_GPL 1' "${CONFIG_H}" >/dev/null 2>&1; then
  echo "ERROR: FFmpeg was configured with GPL enabled" >&2
  exit 1
fi
if grep -E '^#define CONFIG_NONFREE 1' "${CONFIG_H}" >/dev/null 2>&1; then
  echo "ERROR: FFmpeg was configured with nonfree enabled" >&2
  exit 1
fi
# CONFIG_GPL 0 / unset nonfree is required for LGPL redistribution.
if ! grep -E '^#define CONFIG_GPL 0' "${CONFIG_H}" >/dev/null 2>&1; then
  echo "ERROR: expected CONFIG_GPL 0 in ${CONFIG_H}" >&2
  exit 1
fi
for dec in "${ENABLED_DECODERS[@]}"; do
  upper="$(echo "${dec}" | tr '[:lower:]' '[:upper:]')"
  if ! grep -E "^#define CONFIG_${upper}_DECODER 1" "${CONFIG_COMPONENTS}" >/dev/null 2>&1; then
    echo "ERROR: expected CONFIG_${upper}_DECODER 1 in ${CONFIG_COMPONENTS}" >&2
    exit 1
  fi
done
for dec in "${FORBIDDEN_DECODERS[@]}"; do
  upper="$(echo "${dec}" | tr '[:lower:]' '[:upper:]')"
  if grep -E "^#define CONFIG_${upper}_DECODER 1" "${CONFIG_COMPONENTS}" >/dev/null 2>&1; then
    echo "ERROR: forbidden CONFIG_${upper}_DECODER is enabled in ${CONFIG_COMPONENTS}" >&2
    exit 1
  fi
done
# android-libs must exist for all ABIs the AAR claims.
for abi in armeabi-v7a arm64-v8a x86 x86_64; do
  libdir="${FFMPEG_PATH}/android-libs/${abi}"
  if [[ ! -f "${libdir}/libavcodec.a" ]]; then
    echo "ERROR: missing ${libdir}/libavcodec.a after FFmpeg build" >&2
    exit 1
  fi
done
echo "==> FFmpeg config is LGPL with only ac3/eac3/dca decoders"

echo "==> Assembling decoder_ffmpeg AAR..."
cd "${BUILD_ROOT}/media3"
# Point the Media3 build at the local SDK if needed.
if [[ ! -f local.properties ]]; then
  echo "sdk.dir=${ANDROID_SDK_ROOT}" > local.properties
fi
./gradlew :lib-decoder-ffmpeg:assembleRelease --no-daemon

# Media3 1.4.1 uses buildout/ as the Android build dir; keep older build/ as fallback.
SRC_AAR="$(pwd)/libraries/decoder_ffmpeg/buildout/outputs/aar/lib-decoder-ffmpeg-release.aar"
if [[ ! -f "${SRC_AAR}" ]]; then
  SRC_AAR="$(pwd)/libraries/decoder_ffmpeg/build/outputs/aar/lib-decoder-ffmpeg-release.aar"
fi
if [[ ! -f "${SRC_AAR}" ]]; then
  # Fallback name/path used by some AGP versions.
  SRC_AAR="$(find libraries/decoder_ffmpeg -path '*/outputs/aar/*.aar' | head -1)"
fi
if [[ ! -f "${SRC_AAR}" ]]; then
  echo "AAR not found after assembleRelease" >&2
  exit 1
fi

mkdir -p "$(dirname "${OUT_AAR}")"
cp -f "${SRC_AAR}" "${OUT_AAR}"

echo "==> Verifying ${OUT_AAR}"
python3 - <<'PY' "${OUT_AAR}"
import re
import sys
import zipfile
import io
from pathlib import Path

aar = Path(sys.argv[1])
need_so = [
    "jni/arm64-v8a/libffmpegJNI.so",
    "jni/armeabi-v7a/libffmpegJNI.so",
]
need_class = "androidx/media3/decoder/ffmpeg/FfmpegAudioRenderer.class"
need_syms = [b"ff_ac3_decoder", b"ff_eac3_decoder", b"ff_dca_decoder"]
# Must reject the Jellyfin full-decoder set and other common over-broad codecs.
forbid_syms = [
    b"ff_aac_decoder",
    b"ff_aac_latm_decoder",
    b"ff_alac_decoder",
    b"ff_flac_decoder",
    b"ff_mp3_decoder",
    b"ff_mlp_decoder",
    b"ff_truehd_decoder",
    b"ff_pcm_alaw_decoder",
    b"ff_pcm_mulaw_decoder",
]

with zipfile.ZipFile(aar) as z:
    names = set(z.namelist())
    for s in need_so:
        assert s in names, f"missing {s}"
    assert "classes.jar" in names
    data = z.read("classes.jar")
    with zipfile.ZipFile(io.BytesIO(data)) as jar:
        jnames = set(jar.namelist())
        assert need_class in jnames, f"missing {need_class}"

    so_paths = [n for n in names if n.endswith("libffmpegJNI.so")]
    assert so_paths, "no libffmpegJNI.so in AAR"
    for so_path in so_paths:
        so = z.read(so_path)
        for sym in need_syms:
            assert sym in so, f"{so_path}: missing symbol {sym!r}"
        for sym in forbid_syms:
            assert sym not in so, f"{so_path}: forbidden decoder symbol present: {sym!r}"
        found = sorted(set(re.findall(rb"ff_[a-z0-9_]+_decoder", so)))
        expected = sorted(need_syms)
        assert found == expected, (
            f"{so_path}: decoder symbol set mismatch.\n"
            f"  expected: {b', '.join(expected).decode()}\n"
            f"  found:    {b', '.join(found).decode()}"
        )
        print(f"OK {so_path}: {[s.decode() for s in found]}")

print("AAR OK:", aar)
print("size:", aar.stat().st_size)
PY

echo -n "SHA256: "
sha256sum "${OUT_AAR}"
echo "Done. If the digest changed, update android/third_party/media3-ffmpeg/README.md"
echo "and re-run: cd android && JAVA_HOME=${JAVA_HOME} ./gradlew test assembleDebug"
