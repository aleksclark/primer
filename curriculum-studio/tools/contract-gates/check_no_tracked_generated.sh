#!/usr/bin/env bash
# Fail if generated contract/client outputs are tracked by git (REQ-OWN-4, E1-03).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Patterns matched against full repo-relative paths from `git ls-files`.
# Must catch module-root and nested variants (no intermediate-slash requirement).
is_forbidden_generated() {
  local f="$1"

  # Only enforce under curriculum-studio/
  [[ "$f" == curriculum-studio/* ]] || return 1

  case "$f" in
    # Generated trees / temp outputs
    curriculum-studio/contracts/gen/*) return 0 ;;
    curriculum-studio/contracts/.tmp/*) return 0 ;;
    curriculum-studio/tools/contract-gates/spikes/.tmp/*) return 0 ;;
    curriculum-studio/clients/*/generated|curriculum-studio/clients/*/generated/*) return 0 ;;
    curriculum-studio/clients/*/*/generated|curriculum-studio/clients/*/*/generated/*) return 0 ;;
  esac

  local base
  base="$(basename -- "$f")"

  case "$base" in
    openapi.emitted.yaml|*.pb.go|*_grpc.pb.go|*.desc.binpb|*.pb.bin)
      return 0
      ;;
  esac

  # Nested client generated segments anywhere under clients/
  if [[ "$f" == curriculum-studio/clients/* && "$f" == */generated/* ]]; then
    return 0
  fi
  if [[ "$f" == curriculum-studio/clients/* && "$f" == */generated ]]; then
    return 0
  fi

  return 1
}

scan_tracked() {
  local tracked bad=()
  tracked="$(git ls-files 'curriculum-studio' 2>/dev/null || true)"

  local f
  while IFS= read -r f; do
    [[ -z "$f" ]] && continue
    if is_forbidden_generated "$f"; then
      bad+=("$f")
    fi
  done <<<"$tracked"

  if ((${#bad[@]} > 0)); then
    echo "FAIL: tracked generated outputs:" >&2
    printf '  %s\n' "${bad[@]}" >&2
    return 1
  fi
  return 0
}

check_gitignore_samples() {
  # Prove gitignore covers sample paths (E1-03), including module-root emitted.
  local samples=(
    "curriculum-studio/contracts/gen/go/curriculumstudio/v1/common.pb.go"
    "curriculum-studio/contracts/.tmp/curriculumstudio.v1.binpb"
    "curriculum-studio/clients/ts-rest/generated/schema.d.ts"
    "curriculum-studio/clients/go-rest/generated/client.go"
    "curriculum-studio/clients/go-grpc/generated/client.go"
    "curriculum-studio/contracts/openapi/v1/openapi.emitted.yaml"
    "curriculum-studio/openapi.emitted.yaml"
    "curriculum-studio/tools/contract-gates/spikes/.tmp/proto-gen/x.go"
    "curriculum-studio/internal/api/openapi.emitted.yaml"
    "curriculum-studio/foo.pb.go"
  )

  local s
  for s in "${samples[@]}"; do
    if ! git check-ignore -q "$s"; then
      fail "path not ignored (expected gitignore): $s"
    fi
  done
}

# Paths that MUST remain allowed when committed (baselines / fixtures / sources).
assert_allowed_not_flagged() {
  local allowed=(
    "curriculum-studio/contracts/openapi/v1/curriculum-studio.yaml"
    "curriculum-studio/contracts/proto/curriculumstudio/v1/common.proto"
    "curriculum-studio/contracts/buf.gen.yaml"
    "curriculum-studio/tools/contract-gates/spikes/fixtures"
    "curriculum-studio/tools/contract-gates/spikes/evidence/generate_digests.txt"
    "curriculum-studio/tools/contract-gates/spikes/REPORT.md"
  )
  local p
  for p in "${allowed[@]}"; do
    if is_forbidden_generated "$p"; then
      fail "allowed baseline incorrectly classified as generated: $p"
    fi
  done
}

# Safe planted git-index self-test: force-track forbidden variants, expect fail,
# prove allowed baselines are not classified forbidden, always restore.
self_test() {
  assert_allowed_not_flagged

  local stamp plant_dir
  stamp="planted-gen-scan-$$"
  plant_dir="$(mktemp -d "${TMPDIR:-/tmp}/${stamp}.XXXXXX")"

  local plants=(
    "curriculum-studio/openapi.emitted.yaml"
    "curriculum-studio/contracts/openapi/v1/openapi.emitted.yaml"
    "curriculum-studio/internal/api/openapi.emitted.yaml"
    "curriculum-studio/contracts/gen/go/curriculumstudio/v1/planted_c4_selftest.pb.go"
    "curriculum-studio/contracts/.tmp/spike.desc.binpb"
    "curriculum-studio/clients/ts-rest/generated/schema.d.ts"
    "curriculum-studio/clients/go-grpc/generated/client.go"
    "curriculum-studio/tools/contract-gates/spikes/.tmp/proto-gen/x.pb.go"
    "curriculum-studio/scratch.pb.go"
  )

  local restore_dirs=()
  cleanup() {
    local p d
    for p in "${plants[@]}"; do
      git rm -f --cached --quiet -- "$p" 2>/dev/null || true
      rm -f -- "$p" 2>/dev/null || true
    done
    # Best-effort remove empty planted parents created under curriculum-studio.
    for d in \
      curriculum-studio/contracts/gen/go/curriculumstudio/v1 \
      curriculum-studio/contracts/gen/go/curriculumstudio \
      curriculum-studio/contracts/gen/go \
      curriculum-studio/contracts/gen \
      curriculum-studio/contracts/.tmp \
      curriculum-studio/clients/ts-rest/generated \
      curriculum-studio/clients/go-grpc/generated \
      curriculum-studio/tools/contract-gates/spikes/.tmp/proto-gen \
      curriculum-studio/tools/contract-gates/spikes/.tmp
    do
      rmdir "$d" 2>/dev/null || true
    done
    rm -rf -- "$plant_dir"
  }
  trap cleanup EXIT

  local p parent
  for p in "${plants[@]}"; do
    parent="$(dirname -- "$p")"
    mkdir -p "$parent"
    printf 'planted %s\n' "$p" >"$p"
    git add -f -- "$p"
  done

  local out ec=0
  set +e
  out="$(scan_tracked 2>&1)"
  ec=$?
  set -e

  if [[ "$ec" -eq 0 ]]; then
    fail "planted self-test expected scanner failure; got success:\n$out"
  fi

  local missing=0
  for p in "${plants[@]}"; do
    if ! grep -Fqx "  $p" <<<"$out"; then
      echo "FAIL: planted path not reported: $p" >&2
      echo "$out" >&2
      missing=1
    fi
  done
  if [[ "$missing" -ne 0 ]]; then
    fail "planted self-test missing expected failures"
  fi

  cleanup
  trap - EXIT

  # After restore, ordinary scan must be green.
  scan_tracked
  check_gitignore_samples
  assert_allowed_not_flagged

  echo "OK: planted generated-path self-test (root+nested fail; baselines allowed)"
}

main() {
  case "${1:-}" in
    --self-test)
      self_test
      ;;
    "")
      scan_tracked || exit 1
      check_gitignore_samples
      assert_allowed_not_flagged
      echo "OK: no tracked generated outputs; sample paths ignored"
      ;;
    *)
      fail "usage: $0 [--self-test]"
      ;;
  esac
}

main "$@"
