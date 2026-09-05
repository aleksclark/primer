#!/usr/bin/env bash
# Two isolated host-stack roots: distinct source, mutation, and teardown.
set -Eeuo pipefail

ROOT=$(export CDPATH=; cd -- "$(dirname -- "$0")/.." && pwd)
PROBE_TAG="host-$(date +%s)-$$"
TMP=$(mktemp -d)
A="$TMP/a"
B="$TMP/b"
mkdir -p "$A" "$B"
rsync -a --exclude node_modules --exclude tmp --exclude build --exclude dist --exclude android/build --exclude android/.gradle "$ROOT/" "$A/"
rsync -a --exclude node_modules --exclude tmp --exclude build --exclude dist --exclude android/build --exclude android/.gradle "$ROOT/" "$B/"
mkdir -p "$A/../design-system" "$B/../design-system"
rsync -a "$ROOT/../design-system/" "$A/../design-system/"
rsync -a "$ROOT/../design-system/" "$B/../design-system/"
chmod +x "$A/scripts/host-stack" "$B/scripts/host-stack"

cleanup() {
  if [[ -f "$A/tmp/host-stack/web.log" ]]; then
    echo "prove-host: A web.log" >&2
    tail -40 "$A/tmp/host-stack/web.log" >&2 || true
  fi
  if [[ -f "$A/tmp/host-stack/api.log" ]]; then
    echo "prove-host: A api.log" >&2
    tail -20 "$A/tmp/host-stack/api.log" >&2 || true
  fi
  TASKS_HOST_NAME="${PROBE_TAG}-a" "$A/scripts/host-stack" down || true
  TASKS_HOST_NAME="${PROBE_TAG}-b" "$B/scripts/host-stack" down || true
  rm -rf -- "$TMP"
}
trap cleanup EXIT

TASKS_HOST_NAME="${PROBE_TAG}-a" TASKS_HOST_STATE_DIR="$A/tmp/host-stack" "$A/scripts/host-stack" up
TASKS_HOST_NAME="${PROBE_TAG}-b" "$B/scripts/host-stack" up
# shellcheck disable=SC1091
source "$A/tmp/host-stack/endpoints.env"
A_WEB=$PRIMER_TASKS_BASE_URL
A_API=$PRIMER_TASKS_API_URL
# shellcheck disable=SC1091
source "$B/tmp/host-stack/endpoints.env"
B_WEB=$PRIMER_TASKS_BASE_URL
B_API=$PRIMER_TASKS_API_URL
[[ "$A_WEB" != "$B_WEB" && "$A_API" != "$B_API" ]] || { echo "prove-host: FAIL shared endpoints" >&2; exit 1; }

mkdir -p "$A/tmp" "$B/tmp"
echo unique-a > "$A/tmp/marker"
echo unique-b > "$B/tmp/marker"
[[ "$(cat "$A/tmp/marker")" != "$(cat "$B/tmp/marker")" ]] || { echo "prove-host: FAIL shared source tree" >&2; exit 1; }
curl -fsS "${A_API}health" >/dev/null
curl -fsS "${B_API}health" >/dev/null
TASKS_HOST_NAME="${PROBE_TAG}-a" TASKS_HOST_STATE_DIR="$A/tmp/host-stack" "$A/scripts/host-stack" down
sleep 1
if curl -fsS --max-time 1 "${A_API}health" >/dev/null 2>&1; then
  echo "prove-host: FAIL A still answering after down" >&2
  exit 1
fi
curl -fsS "${B_API}health" >/dev/null || { echo "prove-host: FAIL B died when A stopped" >&2; exit 1; }
echo "prove-host: PASS two isolated host roots"
