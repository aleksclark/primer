#!/usr/bin/env bash
# Rebuild the vendored Media3 1.4.1 decoder_ffmpeg AAR (LGPL: ac3/eac3/dca).
# See README.md in this directory for provenance and license notes.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ANDROID="$(cd "${SCRIPT_DIR}/../.." && pwd)"
REPO_ROOT="$(cd "${REPO_ANDROID}/.." && pwd)"

MEDIA3_TAG="${MEDIA3_TAG:-1.4.1}"
MEDIA3_COMMIT="${MEDIA3_COMMIT:-c35a9d62baec57118ea898e271ac66819399649b}"
FFMPEG_REF="${FFMPEG_REF:-release/6.0}"
# Tip of release/6.0 when this AAR was first produced; override to float.
FFMPEG_COMMIT="${FFMPEG_COMMIT:-3f92512fd1fd6f5e6d6eb45a156c352835314d69}"

NDK_PATH="${NDK_PATH:-/opt/android-sdk/ndk/26.1.10909125}"
HOST_PLATFORM="${HOST_PLATFORM:-linux-x86_64}"
ANDROID_ABI="${ANDROID_ABI:-21}"
ENABLED_DECODERS=(ac3 eac3 dca)

BUILD_ROOT="${BUILD_ROOT:-/tmp/primer-media3-ffmpeg-build}"
OUT_AAR="${OUT_AAR:-${REPO_ANDROID}/app/libs/media3-ffmpeg-decoder-1.4.1.aar}"
JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-17-openjdk}"
export JAVA_HOME
export ANDROID_HOME="${ANDROID_HOME:-/opt/android-sdk}"
export ANDROID_SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME}}"

echo "==> Build root: ${BUILD_ROOT}"
echo "==> NDK: ${NDK_PATH}"
echo "==> Decoders: ${ENABLED_DECODERS[*]}"
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
  echo "WARNING: Media3 HEAD ${HEAD} != pinned ${MEDIA3_COMMIT}" >&2
fi

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

echo "==> Assembling decoder_ffmpeg AAR..."
cd "${BUILD_ROOT}/media3"
# Point the Media3 build at the local SDK if needed.
if [[ ! -f local.properties ]]; then
  echo "sdk.dir=${ANDROID_SDK_ROOT}" > local.properties
fi
./gradlew :lib-decoder-ffmpeg:assembleRelease --no-daemon

SRC_AAR="$(pwd)/libraries/decoder_ffmpeg/build/outputs/aar/lib-decoder-ffmpeg-release.aar"
if [[ ! -f "${SRC_AAR}" ]]; then
  # Fallback name used by some AGP versions.
  SRC_AAR="$(find libraries/decoder_ffmpeg/build/outputs/aar -name '*.aar' | head -1)"
fi
if [[ ! -f "${SRC_AAR}" ]]; then
  echo "AAR not found after assembleRelease" >&2
  exit 1
fi

mkdir -p "$(dirname "${OUT_AAR}")"
cp -f "${SRC_AAR}" "${OUT_AAR}"

echo "==> Verifying ${OUT_AAR}"
python3 - <<'PY' "${OUT_AAR}"
import sys, zipfile, tempfile, os
from pathlib import Path
aar = Path(sys.argv[1])
need_so = [
    "jni/arm64-v8a/libffmpegJNI.so",
    "jni/armeabi-v7a/libffmpegJNI.so",
]
need_class = "androidx/media3/decoder/ffmpeg/FfmpegAudioRenderer.class"
need_syms = [b"ff_ac3_decoder", b"ff_eac3_decoder", b"ff_dca_decoder"]
with zipfile.ZipFile(aar) as z:
    names = set(z.namelist())
    for s in need_so:
        assert s in names, f"missing {s}"
    assert "classes.jar" in names
    data = z.read("classes.jar")
    import io, zipfile as zf
    with zf.ZipFile(io.BytesIO(data)) as jar:
        jnames = set(jar.namelist())
        assert need_class in jnames, f"missing {need_class}"
    so = z.read("jni/arm64-v8a/libffmpegJNI.so")
    for sym in need_syms:
        assert sym in so, f"missing symbol {sym!r}"
print("AAR OK:", aar)
print("size:", aar.stat().st_size)
PY

echo -n "SHA256: "
sha256sum "${OUT_AAR}"
echo "Done. If the digest changed, update android/third_party/media3-ffmpeg/README.md"
echo "and re-run: cd android && JAVA_HOME=${JAVA_HOME} ./gradlew test assembleDebug"
