#!/usr/bin/env bash
# Production protobuf generation for Curriculum Studio (C4).
# Uses committed local Buf plugins only (no BSR remote, no ambient fallback).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BOOTSTRAP="$ROOT/scripts/bootstrap_local_plugins.sh"
EXPECTED_PGO_VER="${EXPECTED_PGO_VER:-v1.36.11}"
EXPECTED_PGGRPC_VER="${EXPECTED_PGGRPC_VER:-v1.5.1}"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

command -v buf >/dev/null || fail "buf is required"
EXPECTED_BUF_MAJOR_MINOR="1.72"
buf_version=""
if ! buf_version="$(buf --version 2>&1 | tr -d '[:space:]')"; then
  fail "unable to determine buf CLI version"
fi
[[ "$buf_version" =~ ^${EXPECTED_BUF_MAJOR_MINOR}\.[0-9]+$ ]] || \
  fail "buf CLI version '$buf_version' not in ${EXPECTED_BUF_MAJOR_MINOR}.x (pinned generation requires Buf 1.72.x)"
[[ -f "$ROOT/buf.gen.yaml" ]] || fail "missing buf.gen.yaml"
[[ -f "$BOOTSTRAP" ]] || fail "missing bootstrap_local_plugins.sh"

if grep -Eq '^[[:space:]]*-[[:space:]]*remote:' "$ROOT/buf.gen.yaml"; then
  fail "buf.gen.yaml must use local plugins only (found remote:)"
fi

digest_tree() {
  local dir="$1"
  (
    cd "$dir"
    find . -type f | LC_ALL=C sort | while read -r f; do
      printf '%s ' "$f"
      sha256sum "$f" | awk '{print $1}'
    done
  ) | sha256sum | awk '{print $1}'
}

bootstrap_plugins() {
  local out
  out="$(bash "$BOOTSTRAP")" || fail "bootstrap_local_plugins.sh failed"
  PLUGIN_BIN_DIR="$(printf '%s\n' "$out" | awk -F= '/^plugin_bin_dir=/{print substr($0,index($0,"=")+1); exit}')"
  [[ -n "$PLUGIN_BIN_DIR" && -d "$PLUGIN_BIN_DIR" ]] || fail "bootstrap did not report plugin_bin_dir"
  export PATH="$PLUGIN_BIN_DIR:$PATH"
  local which_pgo which_grpc
  which_pgo="$(command -v protoc-gen-go)"
  which_grpc="$(command -v protoc-gen-go-grpc)"
  [[ "$which_pgo" == "$PLUGIN_BIN_DIR/protoc-gen-go" ]] || \
    fail "PATH protoc-gen-go is '$which_pgo' not private pin"
  [[ "$which_grpc" == "$PLUGIN_BIN_DIR/protoc-gen-go-grpc" ]] || \
    fail "PATH protoc-gen-go-grpc is '$which_grpc' not private pin"
}

assert_generated() {
  local dest="$1"
  local count_go=0 count_grpc=0
  local f
  shopt -s nullglob
  for f in "$dest"/curriculumstudio/v1/*.pb.go; do
    [[ "$f" == *_grpc.pb.go ]] && continue
    count_go=$((count_go + 1))
    local pgo_hdr
    pgo_hdr="$(awk '/protoc-gen-go v/{print; exit}' "$f" | sed -E 's/.*protoc-gen-go[[:space:]]+//')"
    pgo_hdr="$(printf '%s' "$pgo_hdr" | tr -d '[:space:]')"
    [[ "$pgo_hdr" == "$EXPECTED_PGO_VER" ]] || \
      fail "generated header pin mismatch in $(basename "$f"): protoc-gen-go '$pgo_hdr' want $EXPECTED_PGO_VER"
  done
  for f in "$dest"/curriculumstudio/v1/*_grpc.pb.go; do
    count_grpc=$((count_grpc + 1))
    local grpc_hdr
    grpc_hdr="$(awk '/protoc-gen-go-grpc v/{print; exit}' "$f" | sed -E 's/.*protoc-gen-go-grpc[[:space:]]+//')"
    grpc_hdr="$(printf '%s' "$grpc_hdr" | tr -d '[:space:]')"
    [[ "$grpc_hdr" == "$EXPECTED_PGGRPC_VER" ]] || \
      fail "generated header pin mismatch in $(basename "$f"): protoc-gen-go-grpc '$grpc_hdr' want $EXPECTED_PGGRPC_VER"
  done
  shopt -u nullglob
  ((count_go > 0)) || fail "no *.pb.go produced under $dest"
  ((count_grpc > 0)) || fail "no *_grpc.pb.go produced under $dest"
}

generate_once() {
  env PATH="$PLUGIN_BIN_DIR:$PATH" buf generate || fail "buf generate failed (local plugins only)"
  [[ -d "$ROOT/gen/go" ]] || fail "buf generate produced no gen/go"
  assert_generated "$ROOT/gen/go"
}

bootstrap_plugins

mode="${1:-}"
case "$mode" in
  --twice)
    generate_once
    d1="$(digest_tree "$ROOT/gen/go")"
    generate_once
    d2="$(digest_tree "$ROOT/gen/go")"
    [[ "$d1" == "$d2" ]] || fail "generate twice digest mismatch: $d1 != $d2"
    echo "digest=$d1"
    echo "OK: generate twice deterministic"
    ;;
  "" )
    generate_once
    echo "OK: buf generate (local pinned plugins)"
    ;;
  *)
    fail "usage: $0 [--twice]"
    ;;
esac
