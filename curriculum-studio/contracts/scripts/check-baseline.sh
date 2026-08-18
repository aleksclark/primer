#!/usr/bin/env bash
# Verify the hand-authored OpenAPI compatibility baseline has not changed.
# Updating it requires the explicit BASELINE_BOOTSTRAP=1 procedure below.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASELINE="$ROOT/openapi/v1/curriculum-studio.yaml"
DIGEST="$ROOT/openapi/v1/.baseline.sha256"

[[ -f "$BASELINE" ]] || { echo "FAIL: missing baseline $BASELINE" >&2; exit 1; }
[[ -f "$DIGEST" ]] || { echo "FAIL: missing baseline digest $DIGEST" >&2; exit 1; }
actual="$(sha256sum "$BASELINE" | awk '{print $1}')"
expected="$(awk 'NF {print $1; exit}' "$DIGEST")"
if [[ "$actual" == "$expected" ]]; then
  echo "OK: OpenAPI compatibility baseline is frozen ($actual)"
  exit 0
fi

if [[ "${BASELINE_BOOTSTRAP:-}" == "1" ]]; then
  printf '%s  %s\n' "$actual" "curriculum-studio.yaml" >"$DIGEST"
  echo "BOOTSTRAPPED: baseline digest updated to $actual"
  exit 0
fi

cat >&2 <<EOF
FAIL: OpenAPI compatibility baseline changed.
  expected: $expected
  actual:   $actual
The baseline is not live SoT. Review the emitted Huma spec, then explicitly
run BASELINE_BOOTSTRAP=1 $ROOT/scripts/check-baseline.sh and commit the digest
change together with the reviewed baseline update.
EOF
exit 1
