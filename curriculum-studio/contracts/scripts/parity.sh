#!/usr/bin/env bash
# Closed-enum parity entrypoint (C2).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STUDIO="$(cd "$ROOT/.." && pwd)"
exec python3 "$STUDIO/tools/contract-gates/enum_parity.py" \
  --studio-root "$STUDIO" \
  --openapi "${OPENAPI_PATH:-$ROOT/openapi/v1/curriculum-studio.yaml}" \
  --mappings "$STUDIO/tools/contract-gates/enum_mappings.yaml"
