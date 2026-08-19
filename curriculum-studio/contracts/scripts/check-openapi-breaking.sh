#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASELINE="$ROOT/openapi/v1/curriculum-studio.yaml"
CANDIDATE="${1:-$ROOT/.tmp/openapi.emitted.yaml}"
[[ -s "$BASELINE" ]] || { echo "FAIL: missing OpenAPI baseline" >&2; exit 1; }
[[ -s "$CANDIDATE" ]] || { echo "FAIL: missing emitted OpenAPI candidate $CANDIDATE" >&2; exit 1; }
python3 "$ROOT/scripts/check-openapi-breaking.py" "$BASELINE" "$CANDIDATE"
