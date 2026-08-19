#!/usr/bin/env bash
# Run the full contract generation graph twice and compare normalized trees.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

hash_tree() {
  local dir="$1"
  (
    cd "$dir"
    find . -type f -print0 | sort -z | while IFS= read -r -d '' file; do
      printf '%s ' "$file"
      sha256sum "$file" | awk '{print $1}'
    done
  ) | sha256sum | awk '{print $1}'
}
run_once() {
  make -C "$ROOT" clients-generate >/tmp/studio-contract-generate.log 2>&1
  hash_tree "$ROOT/contracts/gen/go"
  sha256sum "$ROOT/contracts/.tmp/openapi.emitted.yaml" | awk '{print $1}'
  hash_tree "$ROOT/clients/go-rest/generated"
  sha256sum "$ROOT/clients/ts-rest/generated/schema.d.ts" | awk '{print $1}'
}
first="$(run_once)"
second="$(run_once)"
if [[ "$first" != "$second" ]]; then
  echo "FAIL: generation digest mismatch" >&2
  diff -u <(printf '%s\n' "$first") <(printf '%s\n' "$second") >&2 || true
  exit 1
fi
printf 'OK: deterministic generation\n%s\n' "$first"
