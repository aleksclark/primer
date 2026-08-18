#!/usr/bin/env bash
# Generate the TypeScript OpenAPI types from the emitted Huma contract.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SPEC="$ROOT/contracts/.tmp/openapi.emitted.yaml"
OUT="$ROOT/clients/ts-rest/generated/schema.d.ts"

[[ -f "$SPEC" ]] || {
  echo "FAIL: missing emitted spec $SPEC; run make contracts-openapi-emit" >&2
  exit 1
}
mkdir -p "$(dirname "$OUT")"
npx --yes openapi-typescript@7.13.0 "$SPEC" -o "$OUT"
echo "OK: generated $OUT"
