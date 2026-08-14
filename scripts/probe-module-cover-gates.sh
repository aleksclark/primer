#!/usr/bin/env bash
# Regression probe for studio-cover / identity-cover fail-closed gates.
#
# SAFETY INVARIANT:
#   Creates and deletes ONLY an isolated mktemp -d fixture root.
#   Never creates, overwrites, moves, backs up, or deletes paths under the live
#   curriculum-studio/ or primer-identity/ trees (including internal/).
#   No rm -rf on any path derived from live module roots.
#   Safe to run after real S1/I1 internal packages exist.
#
# Proves enforce-module-cover.sh against absolute temp module paths for:
#   deferred (no internal/), empty internal/, low coverage, high coverage,
#   and missing-bc fail-closed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

failures=0
pass() { printf 'PASS: %s\n' "$1"; }
fail() { printf 'FAIL: %s\n' "$1" >&2; failures=$((failures + 1)); }

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "$got" == "$want" ]]; then
    pass "$label (got $got)"
  else
    fail "$label (want $want, got $got)"
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" label="$3"
  if grep -Fq -- "$needle" <<<"$haystack"; then
    pass "$label"
  else
    fail "$label (missing: $needle)"
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" label="$3"
  if grep -Fq -- "$needle" <<<"$haystack"; then
    fail "$label (unexpected: $needle)"
  else
    pass "$label"
  fi
}

# Isolated fixture root — the ONLY tree this probe mutates.
FIXTURE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/primer-cover-probe.XXXXXX")"
cleanup_fixture() { rm -rf "$FIXTURE_ROOT"; }
trap cleanup_fixture EXIT

HELPER="$ROOT/scripts/enforce-module-cover.sh"
if [[ ! -x "$HELPER" ]]; then
  echo "FAIL: enforce-module-cover.sh missing or not executable: $HELPER" >&2
  exit 1
fi

# Refuse any path that resolves inside live module roots (defense in depth).
assert_path_is_fixture() {
  local path="$1"
  local parent base abs
  parent="$(cd "$(dirname "$path")" && pwd)"
  base="$(basename "$path")"
  abs="${parent}/${base}"
  case "$abs" in
    "$ROOT/curriculum-studio"|"$ROOT/curriculum-studio"/*|"$ROOT/primer-identity"|"$ROOT/primer-identity"/*)
      echo "REFUSING: path touches live module root: $abs" >&2
      exit 1
      ;;
  esac
  case "$abs" in
    "$FIXTURE_ROOT"|"$FIXTURE_ROOT"/*) ;;
    *)
      echo "REFUSING: path outside fixture root: $abs (fixture=$FIXTURE_ROOT)" >&2
      exit 1
      ;;
  esac
}

# Create a tiny standalone Go module under the fixture root.
# mode: deferred | empty | low | high
make_fixture_module() {
  local name="$1" mode="$2"
  local mod_dir="${FIXTURE_ROOT}/${name}"
  assert_path_is_fixture "$mod_dir"

  rm -rf "$mod_dir"
  mkdir -p "$mod_dir"
  cat >"${mod_dir}/go.mod" <<EOF
module probe.example/${name}

go 1.24.0
EOF

  case "$mode" in
    deferred)
      # no internal/
      ;;
    empty)
      mkdir -p "${mod_dir}/internal"
      ;;
    high)
      mkdir -p "${mod_dir}/internal/coverprobe"
      cat >"${mod_dir}/internal/coverprobe/probe.go" <<'EOF'
package coverprobe

func Add(a, b int) int { return a + b }
EOF
      cat >"${mod_dir}/internal/coverprobe/probe_test.go" <<'EOF'
package coverprobe

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatalf("Add")
	}
}
EOF
      ;;
    low)
      mkdir -p "${mod_dir}/internal/coverprobe"
      cat >"${mod_dir}/internal/coverprobe/probe.go" <<'EOF'
package coverprobe

func Covered() int { return 1 }

func A() int { return 1 }
func B() int { return 2 }
func C() int { return 3 }
func D() int { return 4 }
func E() int { return 5 }
func F() int { return 6 }
func G() int { return 7 }
func H() int { return 8 }
func I() int { return 9 }
func J() int { return 10 }
EOF
      cat >"${mod_dir}/internal/coverprobe/probe_test.go" <<'EOF'
package coverprobe

import "testing"

func TestCovered(t *testing.T) {
	if Covered() != 1 {
		t.Fatalf("Covered")
	}
}
EOF
      ;;
    *)
      echo "unknown mode: $mode" >&2
      exit 1
      ;;
  esac

  printf '%s\n' "$mod_dir"
}

run_helper() {
  local mod="$1" min="$2" label="$3"
  assert_path_is_fixture "$mod"
  set +e
  HELPER_OUT="$("$HELPER" "$mod" "$min" "$label" 2>&1)"
  HELPER_EC=$?
  set -e
}

# ---------------------------------------------------------------------------
# Static safety: this script must never target live module internal/ trees.
# ---------------------------------------------------------------------------
# Snapshot live module trees before any fixture work so we can prove
# byte-identical restoration even when untracked S1/I1 files already exist.
LIVE_SNAP_BEFORE="${FIXTURE_ROOT}/live-snap-before.txt"
LIVE_SNAP_AFTER="${FIXTURE_ROOT}/live-snap-after.txt"
snapshot_live_modules() {
  local out="$1"
  {
    # Paths + checksums of every file under live module roots (if any).
    # Sorted for stable compare. Does not follow into fixture root.
    if [[ -d "$ROOT/curriculum-studio" ]]; then
      find "$ROOT/curriculum-studio" -type f -print0 2>/dev/null \
        | sort -z \
        | xargs -0 sha256sum 2>/dev/null || true
    fi
    if [[ -d "$ROOT/primer-identity" ]]; then
      find "$ROOT/primer-identity" -type f -print0 2>/dev/null \
        | sort -z \
        | xargs -0 sha256sum 2>/dev/null || true
    fi
    # Directory existence markers (internal/ may or may not exist).
    printf 'DIR_EXISTS curriculum-studio/internal=%s\n' \
      "$( [[ -d "$ROOT/curriculum-studio/internal" ]] && echo yes || echo no )"
    printf 'DIR_EXISTS primer-identity/internal=%s\n' \
      "$( [[ -d "$ROOT/primer-identity/internal" ]] && echo yes || echo no )"
  } >"$out"
}
snapshot_live_modules "$LIVE_SNAP_BEFORE"

echo "== probe: static non-destruction guards =="
PROBE_SRC="$ROOT/scripts/probe-module-cover-gates.sh"
# Ban destructive ops against live module roots (the C1 defect class).
# Allow rm -rf only under FIXTURE_ROOT / $mod_dir (fixture-local).
if grep -nE 'rm[[:space:]]+(-[a-zA-Z]*r[a-zA-Z]*f|-rf|--recursive)[[:space:]]+' "$PROBE_SRC" \
  | grep -E 'curriculum-studio|primer-identity' \
  | grep -vE 'FIXTURE_ROOT|fixture|mktemp|^\s*#' >/dev/null 2>&1; then
  fail "probe script still rm -rf live module roots"
else
  pass "probe script has no rm -rf of live module roots"
fi
# Ban writers that plant packages under live module internal/.
if grep -nE '(>|>>|tee).*(curriculum-studio|primer-identity)/internal' "$PROBE_SRC" >/dev/null 2>&1; then
  fail "probe script writes into live module internal/"
else
  pass "probe script does not write into live module internal/"
fi
if grep -nE 'mkdir[[:space:]].*(curriculum-studio|primer-identity)/internal' "$PROBE_SRC" >/dev/null 2>&1; then
  fail "probe script mkdir under live module internal/"
else
  pass "probe script does not mkdir under live module internal/"
fi
# Legacy live-root helpers must not exist.
if grep -nE '^(cleanup_module|write_pkg)\(\)|cleanup_all\(\)' "$PROBE_SRC" >/dev/null 2>&1; then
  fail "probe still defines live-root cleanup_module/write_pkg/cleanup_all"
else
  pass "probe has no live-root cleanup_module/write_pkg helpers"
fi

echo "== probe: deferred when internal/ is absent (temp fixture) =="
mod="$(make_fixture_module studio-deferred deferred)"
run_helper "$mod" 85 studio
assert_eq 2 "$HELPER_EC" "helper studio deferred exit 2"
assert_contains "$HELPER_OUT" "deferred" "helper studio deferred message"

mod="$(make_fixture_module identity-deferred deferred)"
run_helper "$mod" 80 identity
assert_eq 2 "$HELPER_EC" "helper identity deferred exit 2"
assert_contains "$HELPER_OUT" "deferred" "helper identity deferred message"

echo "== probe: empty internal/ must not false-green (temp fixture) =="
mod="$(make_fixture_module studio-empty empty)"
run_helper "$mod" 85 studio
assert_eq 1 "$HELPER_EC" "helper studio empty packages exit 1"
assert_contains "$HELPER_OUT" "no packages" "helper studio empty packages message"
assert_not_contains "$HELPER_OUT" "OK:" "helper studio empty is not OK"

mod="$(make_fixture_module identity-empty empty)"
run_helper "$mod" 80 identity
assert_eq 1 "$HELPER_EC" "helper identity empty packages exit 1"
assert_contains "$HELPER_OUT" "no packages" "helper identity empty packages message"
assert_not_contains "$HELPER_OUT" "OK:" "helper identity empty is not OK"

echo "== probe: low coverage must fail floors (temp fixture; studio≥85, identity≥80) =="
mod="$(make_fixture_module studio-low low)"
run_helper "$mod" 85 studio
assert_eq 1 "$HELPER_EC" "helper studio low coverage exit 1"
assert_contains "$HELPER_OUT" "below 85%" "helper studio low floor message"
assert_not_contains "$HELPER_OUT" "OK:" "helper studio low is not OK"

mod="$(make_fixture_module identity-low low)"
run_helper "$mod" 80 identity
assert_eq 1 "$HELPER_EC" "helper identity low coverage exit 1"
assert_contains "$HELPER_OUT" "below 80%" "helper identity low floor message"
assert_not_contains "$HELPER_OUT" "OK:" "helper identity low is not OK"

echo "== probe: high coverage must pass (temp fixture) =="
mod="$(make_fixture_module studio-high high)"
run_helper "$mod" 85 studio
assert_eq 0 "$HELPER_EC" "helper studio high coverage exit 0"
assert_contains "$HELPER_OUT" "OK:" "helper studio high OK message"

mod="$(make_fixture_module identity-high high)"
run_helper "$mod" 80 identity
assert_eq 0 "$HELPER_EC" "helper identity high coverage exit 0"
assert_contains "$HELPER_OUT" "OK:" "helper identity high OK message"

echo "== probe: missing bc must fail closed (temp fixture) =="
mod="$(make_fixture_module studio-nobc high)"
# Build PATH that keeps bash/env/go/core utils but excludes every directory containing bc.
BASH_DIR="$(dirname "$(command -v bash)")"
ENV_DIR="$(dirname "$(command -v env)")"
GO_DIR="$(dirname "$(command -v go)")"
FILTERED_PATH=""
IFS=':' read -ra _path_parts <<<"$PATH"
for p in "${_path_parts[@]}"; do
  [[ -z "$p" ]] && continue
  if [[ -x "${p}/bc" ]]; then
    continue
  fi
  if [[ -n "$FILTERED_PATH" ]]; then
    FILTERED_PATH="${FILTERED_PATH}:${p}"
  else
    FILTERED_PATH="${p}"
  fi
done
for keep in "$BASH_DIR" "$ENV_DIR" "$GO_DIR" /usr/bin /bin; do
  if [[ -d "$keep" && ":${FILTERED_PATH}:" != *":${keep}:"* && ! -x "${keep}/bc" ]]; then
    FILTERED_PATH="${keep}:${FILTERED_PATH}"
  fi
done
# If /usr/bin has bc, put a private bin first with bash/env/go shims and no bc.
if [[ -x /usr/bin/bc ]] || PATH="$FILTERED_PATH" command -v bc >/dev/null 2>&1; then
  NOBC_BIN="${FIXTURE_ROOT}/nobc-bin"
  mkdir -p "$NOBC_BIN"
  for tool in bash env go awk tail tr mktemp rm cat dirname basename printf; do
    src="$(command -v "$tool" 2>/dev/null || true)"
    if [[ -n "$src" && -x "$src" ]]; then
      ln -sfn "$src" "${NOBC_BIN}/${tool}"
    fi
  done
  FILTERED_PATH="${NOBC_BIN}"
fi
if PATH="$FILTERED_PATH" command -v bc >/dev/null 2>&1; then
  fail "could not construct PATH without bc; skip missing-bc assertion"
elif ! PATH="$FILTERED_PATH" command -v bash >/dev/null 2>&1; then
  fail "could not construct PATH with bash but without bc"
else
  set +e
  # Invoke via absolute bash so shebang/env resolution stays under filtered PATH.
  HELPER_OUT="$(PATH="$FILTERED_PATH" bash "$HELPER" "$mod" 85 studio 2>&1)"
  HELPER_EC=$?
  set -e
  assert_eq 1 "$HELPER_EC" "helper missing-bc exit 1"
  assert_contains "$HELPER_OUT" "bc is required" "helper missing-bc message"
  assert_not_contains "$HELPER_OUT" "OK:" "helper missing-bc is not OK"
fi

# Live module trees must be byte-identical to the pre-probe snapshot.
echo "== probe: live trees unchanged (before/after snapshot) =="
snapshot_live_modules "$LIVE_SNAP_AFTER"
if cmp -s "$LIVE_SNAP_BEFORE" "$LIVE_SNAP_AFTER"; then
  pass "live curriculum-studio/ and primer-identity/ byte-identical to pre-probe snapshot"
else
  fail "live module trees changed during probe"
  diff -u "$LIVE_SNAP_BEFORE" "$LIVE_SNAP_AFTER" >&2 || true
fi

if [[ -d "$ROOT/curriculum-studio/internal/coverprobe" ]]; then
  fail "probe created coverprobe under live curriculum-studio"
else
  pass "no probe package under live curriculum-studio/internal"
fi
if [[ -d "$ROOT/primer-identity/internal/coverprobe" ]]; then
  fail "probe created coverprobe under live primer-identity"
else
  pass "no probe package under live primer-identity/internal"
fi

if [[ -d "$FIXTURE_ROOT" ]]; then
  pass "fixture root remained isolated under mktemp"
else
  fail "fixture root disappeared early"
fi

if [[ "$failures" -ne 0 ]]; then
  printf '\nmodule-cover gate probe FAILED with %s error(s)\n' "$failures" >&2
  exit 1
fi

printf '\nmodule-cover gate probe OK (isolated fixtures only; live internals untouched)\n'
exit 0
