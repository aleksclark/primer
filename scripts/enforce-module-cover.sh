#!/usr/bin/env bash
# Fail-closed coverage gate for a single Go module root.
#
# Usage:
#   enforce-module-cover.sh <module-dir> <min-percent> <label>
#
# Semantics:
#   - If <module-dir>/internal does not exist → print deferred message, exit 2
#   - Otherwise run coverage in one isolated cwd with set -e, require a non-empty
#     package set, parse a numeric total, require bc, enforce min floor, cleanup
#     the temporary coverage profile, and never succeed on empty/missing totals.
#   - Tasks additionally collects qualified subprocess coverage from owned
#     instrumented server children and merges counts into the parent profile
#     without changing the statement denominator. Other modules are unchanged.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "$#" -ne 3 ]]; then
  echo "usage: $0 <module-dir> <min-percent> <label>" >&2
  exit 2
fi

MODULE_DIR="$1"
MIN_PERCENT="$2"
LABEL="$3"

if [[ ! -d "$MODULE_DIR" ]]; then
  echo "FAIL: ${LABEL}: module directory missing: $MODULE_DIR" >&2
  exit 1
fi

# Resolve to absolute path so the single-cwd recipe cannot drift.
MODULE_DIR="$(cd "$MODULE_DIR" && pwd)"

if [[ ! -d "${MODULE_DIR}/internal" ]]; then
  echo "${LABEL}-cover: deferred until ${MODULE_DIR##*/}/internal packages exist"
  exit 2
fi

if ! command -v bc >/dev/null 2>&1; then
  echo "FAIL: ${LABEL}: bc is required for coverage comparison" >&2
  exit 1
fi

# Validate min percent is a number (integer or simple decimal).
if [[ ! "$MIN_PERCENT" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "FAIL: ${LABEL}: invalid min percent: $MIN_PERCENT" >&2
  exit 1
fi

PROFILE="$(mktemp "${TMPDIR:-/tmp}/primer-${LABEL}-cover.XXXXXX.out")"
CHILD_ROOT=""
cleanup() {
  rm -f "$PROFILE"
  if [[ -n "${MERGED_PROFILE:-}" ]]; then
    rm -f "$MERGED_PROFILE"
  fi
  if [[ -n "$CHILD_ROOT" ]]; then
    rm -rf "$CHILD_ROOT"
  fi
}
trap cleanup EXIT

# One module cwd for the entire gate — never re-cd from a nested shell state.
cd "$MODULE_DIR"

# Non-empty package set under ./internal/...
set +e
mapfile -t pkgs < <(go list ./internal/... 2>/dev/null)
list_ec=$?
set -e
if [[ "$list_ec" -ne 0 && "${#pkgs[@]}" -gt 0 ]]; then
  echo "FAIL: ${LABEL}: package discovery under ./internal/... was incomplete" >&2
  exit 1
fi
if [[ "${#pkgs[@]}" -eq 0 ]]; then
  echo "FAIL: ${LABEL}: no packages under ./internal/... (empty coverage set)" >&2
  exit 1
fi

COVER_MODE="set"
COVER_ARGS=("./internal/..." "-count=1" "-coverprofile=$PROFILE" "-coverpkg=./internal/...")
COLLECT_CHILD=0
if [[ "$LABEL" == "tasks" && -d "${MODULE_DIR}/cmd/tasks-server" ]]; then
  COVER_MODE="atomic"
  CHILD_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/primer-tasks-childcover.XXXXXX")"
  RUN_ID="$(basename "$CHILD_ROOT")"
  GO_VERSION="$(go env GOVERSION)"
  GOOS_VAL="$(go env GOOS)"
  GOARCH_VAL="$(go env GOARCH)"
  SOURCE_SHA="$(git -C "$MODULE_DIR" rev-parse HEAD 2>/dev/null || git -C "$(dirname "$MODULE_DIR")" rev-parse HEAD)"
  COVERPKG_LIST="$(printf '%s,' "${pkgs[@]}")"
  COVERPKG_LIST="${COVERPKG_LIST%,}"
  PKGS_NL="$(printf '%s\n' "${pkgs[@]}")"
  RUN_ID="$RUN_ID" SOURCE_SHA="$SOURCE_SHA" COVER_MODE="$COVER_MODE" \
    COVERPKG_LIST="$COVERPKG_LIST" GO_VERSION="$GO_VERSION" GOOS_VAL="$GOOS_VAL" \
    GOARCH_VAL="$GOARCH_VAL" PROFILE="$PROFILE" PKGS="$PKGS_NL" \
    python3 - "$CHILD_ROOT/manifest.json" <<'PY'
import json, os, sys
path = sys.argv[1]
manifest = {
    "runId": os.environ["RUN_ID"],
    "sourceSha": os.environ["SOURCE_SHA"],
    "coverMode": os.environ["COVER_MODE"],
    "coverPkg": os.environ["COVERPKG_LIST"],
    "race": False,
    "goVersion": os.environ["GO_VERSION"],
    "goos": os.environ["GOOS_VAL"],
    "goarch": os.environ["GOARCH_VAL"],
    "goflags": os.environ.get("GOFLAGS", ""),
    "gowork": os.environ.get("GOWORK", ""),
    "packages": [pkg for pkg in os.environ["PKGS"].split("\n") if pkg],
    "parentProfile": os.environ["PROFILE"],
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(manifest, handle)
    handle.write("\n")
PY
  export PRIMER_TASKS_CHILD_COVER_ROOT="$CHILD_ROOT"
  export PRIMER_TASKS_CHILD_COVER_RUN_ID="$RUN_ID"
  export PRIMER_TASKS_CHILD_COVER_MODE="$COVER_MODE"
  export PRIMER_TASKS_CHILD_COVERPKG="$COVERPKG_LIST"
  COVER_ARGS=("./internal/..." "-count=1" "-timeout=20m" "-covermode=atomic" "-coverprofile=$PROFILE" "-coverpkg=./internal/...")
  COLLECT_CHILD=1
fi

go test "${COVER_ARGS[@]}"

if [[ ! -s "$PROFILE" ]]; then
  echo "FAIL: ${LABEL}: coverage profile missing or empty" >&2
  exit 1
fi

MEASURED_PROFILE="$PROFILE"
if [[ "$COLLECT_CHILD" -eq 1 ]]; then
  MERGED_PROFILE="$(mktemp "${TMPDIR:-/tmp}/primer-${LABEL}-merged.XXXXXX.out")"
  echo "parent-only coverage before child merge:"
  go tool cover -func="$PROFILE" | tail -1
  ( cd "$SCRIPT_DIR/covermerge" && GOWORK=off go test -count=1 )
  ( cd "$SCRIPT_DIR/covermerge" && GOWORK=off go run . "$CHILD_ROOT" "$PROFILE" "$MERGED_PROFILE" )
  if [[ ! -s "$MERGED_PROFILE" ]]; then
    echo "FAIL: ${LABEL}: merged coverage profile missing or empty" >&2
    exit 1
  fi
  MEASURED_PROFILE="$MERGED_PROFILE"
fi

cover_line="$(go tool cover -func="$MEASURED_PROFILE" | tail -1)"
printf '%s\n' "$cover_line"

total="$(printf '%s\n' "$cover_line" | awk '{print $3}' | tr -d '%')"
if [[ -z "$total" || ! "$total" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "FAIL: ${LABEL}: could not parse coverage total from: ${cover_line}" >&2
  exit 1
fi

# bc returns 1 when the comparison is true.
if [[ "$(printf '%s < %s\n' "$total" "$MIN_PERCENT" | bc)" -eq 1 ]]; then
  echo "FAIL: ${LABEL} coverage ${total}% is below ${MIN_PERCENT}%" >&2
  exit 1
fi

echo "OK: ${LABEL} coverage ${total}% >= ${MIN_PERCENT}%"
exit 0
