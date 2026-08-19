#!/usr/bin/env bash
# Exercise critical compatibility/policy gates against planted failures.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATES="$ROOT/../tools/contract-gates"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# OpenAPI removed-operation red: candidate removes a baseline path.
cp "$ROOT/openapi/v1/curriculum-studio.yaml" "$TMP/broken.yaml"
python3 - "$TMP/broken.yaml" <<'PY'
from pathlib import Path
p = Path(__import__('sys').argv[1])
s = p.read_text()
s = s.replace('  /health:\n', '  /health_removed:\n', 1)
p.write_text(s)
PY
if python3 "$ROOT/scripts/check-openapi-breaking.py" "$ROOT/openapi/v1/curriculum-studio.yaml" "$TMP/broken.yaml" >/dev/null 2>&1; then
  echo "FAIL: OpenAPI breaking red proof stayed green" >&2
  exit 1
fi
echo "OK: OpenAPI removed-operation planted red"

# Raw transport red and tracked-generated red are delegated to their own
# isolated self-tests, which restore any temporary index mutations.
bash "$GATES/check-raw-transport.sh" --self-test
bash "$GATES/check_no_tracked_generated.sh" --self-test

echo "OK: policy planted reds restored"
