#!/usr/bin/env bash
# Mechanical F0 foundation check for Curriculum Studio delivery.
# Proves module paths, workspace membership, Make target ownership/names,
# separate module roots, and absence of forbidden cross-module coupling.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

failures=0
pass() { printf 'PASS: %s\n' "$1"; }
fail() { printf 'FAIL: %s\n' "$1" >&2; failures=$((failures + 1)); }

STUDIO_MOD_PATH='github.com/aleksclark/primer/curriculum-studio'
IDENTITY_MOD_PATH='github.com/aleksclark/primer/identity'
SERVER_MOD_PATH='github.com/aleksclark/primer/server'

# --- Module roots and paths -------------------------------------------------

if [[ -f curriculum-studio/go.mod ]]; then
  pass 'curriculum-studio/go.mod exists'
else
  fail 'curriculum-studio/go.mod missing'
fi

if [[ -f primer-identity/go.mod ]]; then
  pass 'primer-identity/go.mod exists'
else
  fail 'primer-identity/go.mod missing'
fi

if [[ -f curriculum-studio/go.mod ]]; then
  studio_mod="$(awk '/^module / { print $2; exit }' curriculum-studio/go.mod)"
  if [[ "$studio_mod" == "$STUDIO_MOD_PATH" ]]; then
    pass "curriculum-studio module path is $STUDIO_MOD_PATH"
  else
    fail "curriculum-studio module path want $STUDIO_MOD_PATH got ${studio_mod:-<empty>}"
  fi
fi

if [[ -f primer-identity/go.mod ]]; then
  identity_mod="$(awk '/^module / { print $2; exit }' primer-identity/go.mod)"
  if [[ "$identity_mod" == "$IDENTITY_MOD_PATH" ]]; then
    pass "primer-identity module path is $IDENTITY_MOD_PATH"
  else
    fail "primer-identity module path want $IDENTITY_MOD_PATH got ${identity_mod:-<empty>}"
  fi
fi

if [[ -f server/go.mod ]]; then
  server_mod="$(awk '/^module / { print $2; exit }' server/go.mod)"
  if [[ "$server_mod" == "$SERVER_MOD_PATH" ]]; then
    pass "server module path unchanged ($SERVER_MOD_PATH)"
  else
    fail "server module path drifted: $server_mod"
  fi
else
  fail 'server/go.mod missing (unexpected inventory change)'
fi

# --- go.work membership -----------------------------------------------------

if [[ -f go.work ]]; then
  pass 'go.work exists'
  # Required use entries (order-independent).
  for use_path in ./server ./curriculum-studio ./primer-identity; do
    if awk -v p="$use_path" '
      $1 == "use" && $2 == p { found=1 }
      $1 == "use" && $2 == "(" { in_block=1; next }
      in_block && $1 == ")" { in_block=0; next }
      in_block && $1 == p { found=1 }
      END { exit found ? 0 : 1 }
    ' go.work; then
      pass "go.work uses $use_path"
    else
      fail "go.work missing use $use_path"
    fi
  done
else
  fail 'go.work missing'
fi

# --- Separate roots; no forbidden cross-module coupling ---------------------

# Studio must not require the LMS server module (or identity) in go.mod.
if [[ -f curriculum-studio/go.mod ]]; then
  if grep -E '^\s*(require\s+)?github\.com/aleksclark/primer/(server|identity)(\s|$)' curriculum-studio/go.mod >/dev/null 2>&1; then
    fail 'curriculum-studio/go.mod must not require server or identity modules'
  else
    pass 'curriculum-studio/go.mod has no server/identity require'
  fi
fi

if [[ -f primer-identity/go.mod ]]; then
  if grep -E '^\s*(require\s+)?github\.com/aleksclark/primer/(server|curriculum-studio)(\s|$)' primer-identity/go.mod >/dev/null 2>&1; then
    fail 'primer-identity/go.mod must not require server or curriculum-studio modules'
  else
    pass 'primer-identity/go.mod has no server/curriculum-studio require'
  fi
fi

# No Go import of foreign modules under either new tree.
# Scans only .go files for import paths (quoted), ignoring non-Go docs/contracts.
has_forbidden_go_imports() {
  local dir="$1"
  shift
  if [[ ! -d "$dir" ]]; then
    return 1 # no dir → no forbidden imports
  fi
  local go_files=()
  while IFS= read -r -d '' f; do
    go_files+=("$f")
  done < <(find "$dir" -type f -name '*.go' -print0 2>/dev/null)

  if [[ "${#go_files[@]}" -eq 0 ]]; then
    return 1 # no Go files → no forbidden imports
  fi

  local needle
  for needle in "$@"; do
    if grep -nE "\"${needle}(/|[\"])" "${go_files[@]}" >/dev/null 2>&1; then
      return 0 # found forbidden
    fi
  done
  return 1
}

if has_forbidden_go_imports curriculum-studio \
  'github.com/aleksclark/primer/server' \
  'github.com/aleksclark/primer/identity'; then
  fail 'curriculum-studio contains forbidden cross-module Go imports'
else
  pass 'curriculum-studio has no forbidden cross-module Go imports'
fi

if has_forbidden_go_imports primer-identity \
  'github.com/aleksclark/primer/server' \
  'github.com/aleksclark/primer/curriculum-studio'; then
  fail 'primer-identity contains forbidden cross-module Go imports'
else
  pass 'primer-identity has no forbidden cross-module Go imports'
fi

# --- Makefile target ownership / names --------------------------------------

if [[ ! -f Makefile ]]; then
  fail 'root Makefile missing'
else
  required_targets=(
    foundation-check
    studio-build
    studio-test
    studio-cover
    studio-openapi
    studio-client
    studio-web
    studio-e2e
    studio-e2e-go
    dev-db-studio
    migrate-studio
    identity-build
    identity-test
    identity-cover
    identity-openapi
    identity-test-oauth
    identity-e2e
    dev-db-identity
    migrate-identity
  )
  for t in "${required_targets[@]}"; do
    # Match real Make rules: target at start of line, optional .PHONY listing ok separately.
    if grep -E "^${t}([[:space:]]|:)" Makefile >/dev/null 2>&1 || \
       grep -E "^${t}:" Makefile >/dev/null 2>&1; then
      pass "Makefile defines target $t"
    else
      fail "Makefile missing target $t"
    fi
  done

  # COVER_MIN must remain 85 (never lower).
  if grep -E '^COVER_MIN[[:space:]]*:?=[[:space:]]*85[[:space:]]*$' Makefile >/dev/null 2>&1; then
    pass 'COVER_MIN remains 85'
  else
    fail 'COVER_MIN must remain 85'
  fi

  # Module floors: Studio ≥85, Identity ≥80; both via fail-closed helper.
  if grep -E '^STUDIO_COVER_MIN[[:space:]]*:?=[[:space:]]*85[[:space:]]*$' Makefile >/dev/null 2>&1; then
    pass 'STUDIO_COVER_MIN is 85'
  else
    fail 'STUDIO_COVER_MIN must be 85'
  fi
  if grep -E '^IDENTITY_COVER_MIN[[:space:]]*:?=[[:space:]]*80[[:space:]]*$' Makefile >/dev/null 2>&1; then
    pass 'IDENTITY_COVER_MIN is 80'
  else
    fail 'IDENTITY_COVER_MIN must be 80'
  fi
  if grep -E 'studio-cover:|enforce-module-cover\.sh curriculum-studio' Makefile >/dev/null 2>&1 && \
     grep -F 'enforce-module-cover.sh curriculum-studio' Makefile >/dev/null 2>&1; then
    pass 'studio-cover uses enforce-module-cover.sh'
  else
    fail 'studio-cover must call scripts/enforce-module-cover.sh'
  fi
  if grep -F 'enforce-module-cover.sh primer-identity' Makefile >/dev/null 2>&1; then
    pass 'identity-cover uses enforce-module-cover.sh'
  else
    fail 'identity-cover must call scripts/enforce-module-cover.sh'
  fi
fi

# --- Fail-closed coverage helper + regression probe ------------------------

if [[ -x scripts/enforce-module-cover.sh ]]; then
  pass 'scripts/enforce-module-cover.sh is executable'
else
  fail 'scripts/enforce-module-cover.sh missing or not executable'
fi

if [[ -x scripts/probe-module-cover-gates.sh ]]; then
  pass 'scripts/probe-module-cover-gates.sh is executable'
else
  fail 'scripts/probe-module-cover-gates.sh missing or not executable'
fi

# Static recipe anti-patterns: no multi-cd continued shell in module cover targets.
# The helper owns one-cwd execution; recipes must not reintroduce chained cd.
if awk '
  BEGIN { in_target=0; bad=0 }
  /^studio-cover:|^identity-cover:/ { in_target=1; next }
  in_target && /^[^[:space:]#]/ { in_target=0 }
  in_target && /cd[[:space:]]+[^;&|]+&&[[:space:]]*cd[[:space:]]+/ { bad=1 }
  in_target && /cd[[:space:]]+.*\\$/ { # continued recipe with cd is suspicious if multiple
    cd_lines++
  }
  END { exit bad || cd_lines > 1 ? 0 : 1 }
' Makefile; then
  fail 'studio-cover/identity-cover recipes still chain multiple cd commands'
else
  pass 'studio-cover/identity-cover recipes do not chain multiple cds'
fi

# Live deferred semantics only when internal/ is absent (F0 shape).
# When S1/I1 packages exist, skip deferred assertions — probe owns matrix.
set +e
if [[ ! -d curriculum-studio/internal ]]; then
  make -s studio-cover >/tmp/f0-studio-cover-deferred.txt 2>&1
  studio_ec=$?
  if [[ "$studio_ec" -eq 2 ]]; then
    pass 'studio-cover deferred exit 2 without internal/'
  else
    fail "studio-cover without internal/ want exit 2 got $studio_ec"
  fi
else
  pass 'studio-cover deferred check skipped (live internal/ present; probe owns matrix)'
fi
if [[ ! -d primer-identity/internal ]]; then
  make -s identity-cover >/tmp/f0-identity-cover-deferred.txt 2>&1
  identity_ec=$?
  if [[ "$identity_ec" -eq 2 ]]; then
    pass 'identity-cover deferred exit 2 without internal/'
  else
    fail "identity-cover without internal/ want exit 2 got $identity_ec"
  fi
else
  pass 'identity-cover deferred check skipped (live internal/ present; probe owns matrix)'
fi
set -e

# Full adversarial cover-gate probe (isolated mktemp fixtures only; never
# mutates live curriculum-studio/ or primer-identity/ trees).
if [[ -x scripts/probe-module-cover-gates.sh ]]; then
  if scripts/probe-module-cover-gates.sh >/tmp/f0-cover-probe.txt 2>&1; then
    pass 'module-cover gate probe OK (isolated fixtures; empty/low fail, high pass, deferred/missing-bc)'
  else
    fail 'module-cover gate probe failed'
    sed -n '1,80p' /tmp/f0-cover-probe.txt >&2 || true
  fi
fi

# --- make foundation-check is wired -----------------------------------------

if make -n foundation-check >/dev/null 2>&1; then
  pass 'make foundation-check is invokable'
else
  fail 'make foundation-check is not invokable'
fi

# --- Module compile/test smoke (when modules exist) -------------------------

if [[ -f curriculum-studio/go.mod ]]; then
  if (cd curriculum-studio && go test ./... >/dev/null 2>&1); then
    pass 'curriculum-studio go test ./... succeeds'
  else
    fail 'curriculum-studio go test ./... failed'
  fi
fi

if [[ -f primer-identity/go.mod ]]; then
  if (cd primer-identity && go test ./... >/dev/null 2>&1); then
    pass 'primer-identity go test ./... succeeds'
  else
    fail 'primer-identity go test ./... failed'
  fi
fi

if [[ "$failures" -ne 0 ]]; then
  printf '\nF0 foundation check FAILED with %s error(s)\n' "$failures" >&2
  exit 1
fi

printf '\nF0 foundation check OK\n'
exit 0
