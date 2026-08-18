#!/usr/bin/env bash
# Generate Go REST models/client from emitted Huma OpenAPI.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SPEC="$ROOT/contracts/.tmp/openapi.emitted.yaml"
NORMALIZED="$ROOT/contracts/.tmp/openapi.go-client.yaml"

[[ -f "$SPEC" ]] || {
  echo "FAIL: missing emitted spec $SPEC; run make contracts-openapi-emit" >&2
  exit 1
}
python3 "$ROOT/clients/go-rest/scripts/normalize_openapi.py" "$SPEC" "$NORMALIZED"
rm -rf "$ROOT/clients/go-rest/generated"
mkdir -p "$ROOT/clients/go-rest/generated"
GOWORK=off go run github.com/ogen-go/ogen/cmd/ogen@v1.15.0 \
  --target "$ROOT/clients/go-rest/generated" \
  --package generated "$NORMALIZED"
echo "OK: generated clients/go-rest/generated"
