#!/usr/bin/env bash
# scripts/mcp-external-e2e.sh
# External Streamable HTTP client conformance harness for E12-04.
#
# Usage:
#   STUDIO_MCP_URL=http://localhost:8080/mcp \
#   STUDIO_MCP_TOKEN=<JWT> \
#   ./scripts/mcp-external-e2e.sh
#
# To override the client binary:
#   STUDIO_MCP_EXTERNAL_CLIENT=hermes ./scripts/mcp-external-e2e.sh
#
# CI note: This script exits 2 with a BLOCKED message when the external
# client binary is not installed. The exit code is distinct from test failure
# (exit 1) so callers can differentiate a genuine failure from a missing tool.

set -euo pipefail

CLIENT="${STUDIO_MCP_EXTERNAL_CLIENT:-mcporter}"
URL="${STUDIO_MCP_URL:-}"
TOKEN="${STUDIO_MCP_TOKEN:-}"

# ── pre-flight ─────────────────────────────────────────────────────────────────

if [[ -z "$URL" ]]; then
  echo "BLOCKED(E12-04): STUDIO_MCP_URL is not set." >&2
  echo "  Set it to the Studio MCP endpoint, e.g. http://localhost:8080/mcp" >&2
  exit 2
fi

if [[ -z "$TOKEN" ]]; then
  echo "BLOCKED(E12-04): STUDIO_MCP_TOKEN is not set." >&2
  echo "  Provide a valid Studio JWT (aud=curriculum-studio, human or service principal)." >&2
  exit 2
fi

if ! command -v "$CLIENT" >/dev/null 2>&1; then
  echo "BLOCKED(E12-04): external MCP client '$CLIENT' not found in PATH." >&2
  echo "  Install mcporter: https://github.com/mcporter/mcporter" >&2
  echo "  Or set STUDIO_MCP_EXTERNAL_CLIENT to the path of an alternative Streamable HTTP MCP client." >&2
  echo "  This is an explicit external dependency per C12 plan §P12-S4." >&2
  echo "  E12-04 remains BLOCKED until the binary is available." >&2
  exit 2
fi

# ── E12-04: tools/list via external client ─────────────────────────────────────

echo "=== E12-04: external MCP client conformance ($CLIENT) ==="
echo "  URL:    $URL"
echo "  Client: $(command -v "$CLIENT")"
echo ""

echo "--- tools/list ---"
"$CLIENT" list \
  --url "$URL" \
  --header "Authorization: Bearer $TOKEN"

echo ""
echo "--- studio.workspaces.list ---"
"$CLIENT" call studio.workspaces.list \
  --url "$URL" \
  --header "Authorization: Bearer $TOKEN"

echo ""
echo "=== E12-04 PASS (list + read) ==="
