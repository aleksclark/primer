#!/usr/bin/env bash
# Fail if generated contract/client outputs are tracked by git (REQ-OWN-4, E1-03).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Patterns that must never be tracked under curriculum-studio.
PATTERNS=(
  'curriculum-studio/contracts/gen/'
  'curriculum-studio/contracts/.tmp/'
  'curriculum-studio/clients/.*/generated/'
  'curriculum-studio/.*/openapi\.emitted\.yaml'
  'curriculum-studio/.*\.pb\.go$'
  'curriculum-studio/.*_grpc\.pb\.go$'
  'curriculum-studio/.*\.desc\.binpb$'
  'curriculum-studio/.*\.pb\.bin$'
)

tracked="$(git ls-files 'curriculum-studio/**' 2>/dev/null || true)"
if [[ -z "${tracked}" ]]; then
  # Still verify ignore rules when tree is empty of matches.
  tracked=""
fi

bad=()
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  for pat in "${PATTERNS[@]}"; do
    if [[ "$f" =~ $pat ]]; then
      bad+=("$f")
      break
    fi
  done
done <<<"$tracked"

if ((${#bad[@]} > 0)); then
  echo "FAIL: tracked generated outputs:" >&2
  printf '  %s\n' "${bad[@]}" >&2
  exit 1
fi

# Prove gitignore covers sample paths (E1-03).
samples=(
  "curriculum-studio/contracts/gen/go/curriculumstudio/v1/common.pb.go"
  "curriculum-studio/contracts/.tmp/curriculumstudio.v1.binpb"
  "curriculum-studio/clients/ts-rest/generated/schema.d.ts"
  "curriculum-studio/clients/go-rest/generated/client.go"
  "curriculum-studio/clients/go-grpc/generated/client.go"
  "curriculum-studio/contracts/openapi/v1/openapi.emitted.yaml"
  "curriculum-studio/tools/contract-gates/spikes/.tmp/proto-gen/x.go"
)

for s in "${samples[@]}"; do
  if ! git check-ignore -q "$s"; then
    # check-ignore returns 1 if not ignored
    fail "path not ignored (expected gitignore): $s"
  fi
done

echo "OK: no tracked generated outputs; sample paths ignored"
