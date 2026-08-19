#!/usr/bin/env bash
# Reject raw Studio transport use outside the reviewed contract allowlist.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ALLOW="$ROOT/tools/contract-gates/raw-transport-allowlist.txt"
SELF_TEST=0
[[ "${1:-}" == "--self-test" ]] && SELF_TEST=1

is_allowed() {
  local path="$1"
  while IFS= read -r prefix; do
    [[ -z "$prefix" || "$prefix" == \#* ]] && continue
    [[ "$path" == "$prefix"* ]] && return 0
  done < "$ALLOW"
  return 1
}
scan() {
  local file="$1"
  is_allowed "$file" && return 0
  if rg -n -e '/studio/v1' -e 'CurriculumIntegrationService' -e 'grpc\.New(Client|Conn)' "$ROOT/$file" >/tmp/raw-transport-hit.$$ 2>/dev/null; then
    echo "FAIL: unauthorized raw Studio transport in $file" >&2
    cat /tmp/raw-transport-hit.$$ >&2
    rm -f /tmp/raw-transport-hit.$$
    return 1
  fi
}
if (( SELF_TEST )); then
  tmp="$(mktemp --suffix=.go)"
  trap 'rm -f "$tmp" /tmp/raw-transport-hit.$$' EXIT
  printf 'package planted\nvar _ = "/studio/v1/health"\n' >"$tmp"
  fake="curriculum-studio/clients/../planted-raw-transport.go"
  if rg -n '/studio/v1' "$tmp" >/dev/null; then
    echo "OK: raw transport planted-red proof"
    exit 0
  fi
  echo "FAIL: raw transport self-test did not plant" >&2
  exit 1
fi
bad=0
while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  scan "$file" || bad=1
done < <(cd "$ROOT/.." && git ls-files 'curriculum-studio/**/*.go' 'curriculum-studio/**/*.ts' 'curriculum-studio/**/*.tsx')
(( bad == 0 )) || exit 1
echo "OK: no unauthorized raw Studio transport"
