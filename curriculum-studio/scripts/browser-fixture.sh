#!/usr/bin/env bash
# Local credential-free test composition ONLY. No production listener/BFF changes.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
if [[ -n "${STUDIO_TEST_DATABASE_URL:-}" ]]; then
  echo 'Refusing external STUDIO_TEST_DATABASE_URL; unset it for an isolated fixture DB.' >&2; exit 2
fi
if [[ ! -f "$root/web/dist/index.html" ]]; then
  echo 'Build first: cd curriculum-studio && GOWORK=off make clients-generate && cd web && npm ci && npm run build' >&2; exit 2
fi
tmp="$(mktemp -d /tmp/primer-s17-browser.XXXXXX)"
pid=''
cleanup(){ if [[ -n "$pid" ]]; then kill -TERM "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi; rm -rf "$tmp"; }
trap cleanup EXIT
trap 'exit 130' INT TERM
cd "$root"
GOWORK=off go test -c -o "$tmp/fixture.test" ./internal/testutil/browserfixture
STUDIO_BROWSER_FIXTURE=1 STUDIO_FIXTURE_WEB_ROOT="$root/web/dist" STUDIO_FIXTURE_DIR="$tmp" "$tmp/fixture.test" -test.run '^TestBrowserFixtureHost$' -test.v -test.timeout 0 &
pid=$!
wait "$pid"
