#!/usr/bin/env bash
# Install/resolve exact pinned local Buf plugins into a module-local ignored cache.
# Fail closed on missing/mismatched pins. Prefer cache reuse (offline after first install).
# Does NOT fall back to ambient host protoc-gen-go (e.g. 1.36.5).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTRACTS_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

EXPECTED_PGO_VER="${EXPECTED_PGO_VER:-v1.36.11}"
EXPECTED_PGGRPC_VER="${EXPECTED_PGGRPC_VER:-v1.5.1}"
# go install / --version for grpc plugin omits the leading "v" in some outputs.
EXPECTED_PGGRPC_VER_NUM="${EXPECTED_PGGRPC_VER#v}"

PGO_MOD="google.golang.org/protobuf/cmd/protoc-gen-go@${EXPECTED_PGO_VER}"
PGGRPC_MOD="google.golang.org/grpc/cmd/protoc-gen-go-grpc@${EXPECTED_PGGRPC_VER}"

CACHE_ROOT="${CURRICULUM_STUDIO_PLUGIN_CACHE:-$CONTRACTS_ROOT/.tmp/pinned-plugins/${EXPECTED_PGO_VER}_${EXPECTED_PGGRPC_VER}}"
BIN_DIR="${CACHE_ROOT}/bin"

fail() { echo "FAIL: $*" >&2; exit 1; }

command -v go >/dev/null || fail "go required to bootstrap pinned plugins"
mkdir -p "$BIN_DIR"

normalize_pgo_ver() {
  # Accept "protoc-gen-go v1.36.11" or "v1.36.11"
  local raw="$1"
  raw="$(printf '%s' "$raw" | tr -d '[:space:]')"
  raw="${raw#protoc-gen-go}"
  printf '%s' "$raw"
}

normalize_pggrpc_ver() {
  # Accept "protoc-gen-go-grpc 1.5.1" / "v1.5.1" / "1.5.1"
  local raw="$1"
  raw="$(printf '%s' "$raw" | tr -d '[:space:]')"
  raw="${raw#protoc-gen-go-grpc}"
  raw="${raw#v}"
  printf '%s' "$raw"
}

plugin_ok() {
  local bin="$1" kind="$2" want="$3"
  [[ -x "$bin" ]] || return 1
  local got
  got="$("$bin" --version 2>&1 || true)"
  case "$kind" in
    pgo)
      got="$(normalize_pgo_ver "$got")"
      [[ "$got" == "$want" ]]
      ;;
    pggrpc)
      got="$(normalize_pggrpc_ver "$got")"
      [[ "$got" == "${want#v}" ]]
      ;;
    *)
      return 1
      ;;
  esac
}

install_one() {
  local name="$1" mod="$2"
  local dest="$BIN_DIR/$name"
  # Remove stale/mismatched binary so go install can rewrite the pin path.
  if [[ -e "$dest" ]]; then
    rm -f -- "$dest"
  fi
  echo "bootstrap: go install $mod -> $dest"
  # Prefer module cache reuse; still allow first-time network fetch of the exact pin.
  if ! GOBIN="$BIN_DIR" GOWORK=off go install "$mod"; then
    fail "go install failed for exact pin $mod (no ambient fallback)"
  fi
  [[ -x "$dest" ]] || fail "go install did not produce $dest"
}

PGO_BIN="$BIN_DIR/protoc-gen-go"
PGGRPC_BIN="$BIN_DIR/protoc-gen-go-grpc"

if ! plugin_ok "$PGO_BIN" pgo "$EXPECTED_PGO_VER"; then
  install_one "protoc-gen-go" "$PGO_MOD"
fi
if ! plugin_ok "$PGGRPC_BIN" pggrpc "$EXPECTED_PGGRPC_VER"; then
  install_one "protoc-gen-go-grpc" "$PGGRPC_MOD"
fi

# Fail closed: re-verify after install/cache hit. Never trust ambient PATH.
if ! plugin_ok "$PGO_BIN" pgo "$EXPECTED_PGO_VER"; then
  fail "pinned protoc-gen-go version mismatch at $PGO_BIN (want $EXPECTED_PGO_VER; no ambient fallback)"
fi
if ! plugin_ok "$PGGRPC_BIN" pggrpc "$EXPECTED_PGGRPC_VER"; then
  fail "pinned protoc-gen-go-grpc version mismatch at $PGGRPC_BIN (want $EXPECTED_PGGRPC_VER; no ambient fallback)"
fi

PGO_REPORTED="$("$PGO_BIN" --version 2>&1 | tr -s '[:space:]' ' ' | sed 's/[[:space:]]*$//')"
PGGRPC_REPORTED="$("$PGGRPC_BIN" --version 2>&1 | tr -s '[:space:]' ' ' | sed 's/[[:space:]]*$//')"

# Export for callers that source this script.
export CURRICULUM_STUDIO_PLUGIN_BIN="$BIN_DIR"
export PATH="$BIN_DIR:$PATH"

# When executed (not sourced), print machine-readable pins for evidence.
# Optional: --self-test exercises fail-closed mismatch heal + nonexistent pin STOP.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  if [[ "${1:-}" == "--self-test" ]]; then
    set -euo pipefail
    SELF_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/c3-plugin-selftest.XXXXXX")"
    # shellcheck disable=SC2064
    trap "rm -rf -- '$SELF_ROOT'" EXIT
    export CURRICULUM_STUDIO_PLUGIN_CACHE="$SELF_ROOT/cache"
    # Fresh install of exact pins.
    out1="$(bash "$0")"
    bin="$(printf '%s\n' "$out1" | awk -F= '/^plugin_bin_dir=/{print substr($0,index($0,"=")+1); exit}')"
    [[ -x "$bin/protoc-gen-go" ]] || fail "self-test: missing protoc-gen-go after install"
    # Plant mismatched binary; bootstrap must replace (not accept ambient 1.36.5).
    printf '%s\n' '#!/bin/sh' 'echo "protoc-gen-go v1.36.5"' >"$bin/protoc-gen-go"
    chmod +x "$bin/protoc-gen-go"
    out2="$(bash "$0")"
    reported="$(printf '%s\n' "$out2" | awk -F= '/^plugin_protoc_gen_go_reported=/{print substr($0,index($0,"=")+1); exit}')"
    [[ "$reported" == "protoc-gen-go v1.36.11" ]] || \
      fail "self-test: expected reinstall to v1.36.11, got '$reported'"
    # Nonexistent pin must STOP (no ambient fallback).
    set +e
    bad_out="$(EXPECTED_PGO_VER=v0.0.0-not-a-real-pin CURRICULUM_STUDIO_PLUGIN_CACHE="$SELF_ROOT/bad" bash "$0" 2>&1)"
    bad_ec=$?
    set -e
    [[ "$bad_ec" -ne 0 ]] || fail "self-test: nonexistent pin must exit non-zero"
    printf '%s\n' "$bad_out" | grep -Eqi 'FAIL|go install failed|not.a.real|unknown revision|invalid version' || \
      fail "self-test: nonexistent pin output did not fail closed: $bad_out"
    echo "OK: bootstrap_local_plugins self-test (mismatch heal + missing pin STOP)"
    exit 0
  fi
  echo "plugin_bin_dir=$BIN_DIR"
  echo "plugin_protoc_gen_go_path=$PGO_BIN"
  echo "plugin_protoc_gen_go_grpc_path=$PGGRPC_BIN"
  echo "plugin_protoc_gen_go_module=$PGO_MOD"
  echo "plugin_protoc_gen_go_grpc_module=$PGGRPC_MOD"
  echo "plugin_protoc_gen_go_reported=$PGO_REPORTED"
  echo "plugin_protoc_gen_go_grpc_reported=$PGGRPC_REPORTED"
  echo "protoc_gen_go=protoc-gen-go ${EXPECTED_PGO_VER}"
  echo "protoc_gen_go_grpc=protoc-gen-go-grpc ${EXPECTED_PGGRPC_VER}"
fi
