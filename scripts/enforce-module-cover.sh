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
set -euo pipefail

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
if ! [[ "$MIN_PERCENT" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "FAIL: ${LABEL}: invalid min percent: $MIN_PERCENT" >&2
  exit 1
fi

PROFILE="$(mktemp "${TMPDIR:-/tmp}/primer-${LABEL}-cover.XXXXXX.out")"
cleanup() { rm -f "$PROFILE"; }
trap cleanup EXIT

# One module cwd for the entire gate — never re-cd from a nested shell state.
cd "$MODULE_DIR"

# Non-empty package set under ./internal/...
mapfile -t pkgs < <(go list ./internal/... 2>/dev/null || true)
if [[ "${#pkgs[@]}" -eq 0 ]]; then
  echo "FAIL: ${LABEL}: no packages under ./internal/... (empty coverage set)" >&2
  exit 1
fi

go test ./internal/... -count=1 -coverprofile="$PROFILE" -coverpkg=./internal/...

if [[ ! -s "$PROFILE" ]]; then
  echo "FAIL: ${LABEL}: coverage profile missing or empty" >&2
  exit 1
fi

cover_line="$(go tool cover -func="$PROFILE" | tail -1)"
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
