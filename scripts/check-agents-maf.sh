#!/usr/bin/env bash
# Verify the primer-agents MAF adapter is still public-API-only and pinned.
# Supersedes scripts/check-maf-runtime.sh for the canonical agents module.
set -euo pipefail

EXPECTED='v0.0.0-20260813082112-00ffc8c3648c'
COMMIT='00ffc8c3648c547997eae3a3f2a3b00c28daea09'
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# ── 1. Exact pin in primer-agents ────────────────────────────────────────────
actual_agents="$(cd "$ROOT/primer-agents" && go list -m -f '{{.Version}}' github.com/microsoft/agent-framework-go)"
if [[ "$actual_agents" != "$EXPECTED" ]]; then
  printf 'FAIL: primer-agents MAF pin: got %s, want %s\n' "$actual_agents" "$EXPECTED" >&2
  exit 1
fi
echo "OK: primer-agents MAF pin: $actual_agents (commit $COMMIT)"

# ── 2. No import of agent-framework-go/internal in primer-agents/runtime ─────
if grep -rn --include='*.go' \
    '"github\.com/microsoft/agent-framework-go/internal/' \
    "$ROOT/primer-agents/runtime" "$ROOT/primer-agents/internal" 2>/dev/null; then
  echo 'FAIL: primer-agents imports agent-framework-go/internal' >&2
  exit 1
fi
echo "OK: no agent-framework-go/internal imports in primer-agents"

# ── 3. No stock agenttool.New / .Collect() in primer-agents/runtime ──────────
if grep -rn --include='*.go' --exclude='*_test.go' \
    'agenttool\.New(' \
    "$ROOT/primer-agents/runtime" 2>/dev/null; then
  echo 'FAIL: stock agenttool.New() in primer-agents/runtime production code' >&2
  exit 1
fi
echo "OK: no stock agenttool.New in primer-agents/runtime production code"

# ── 4. LMS seam still pins the same commit ────────────────────────────────────
if [[ -f "$ROOT/server/go.mod" ]]; then
  actual_server="$(cd "$ROOT/server" && go list -m -f '{{.Version}}' github.com/microsoft/agent-framework-go)"
  if [[ "$actual_server" != "$EXPECTED" ]]; then
    printf 'FAIL: server MAF pin: got %s, want %s\n' "$actual_server" "$EXPECTED" >&2
    exit 1
  fi
  echo "OK: server MAF pin: $actual_server"
fi

# ── 5. No live billable / Stytch / Studio MCP claims in primer-agents ─────────
forbidden_patterns=(
  'api\.stytch\.com'
  'stytch_session_token'
  'STYTCH_SECRET'
  'r\.Handle.*"/mcp\|router.*"/mcp\|Mount.*"/mcp'
  'Studio.*S19'
  'distributed.*control.*plane'
  'exactly.*once.*side.*effect'
)
for pat in "${forbidden_patterns[@]}"; do
  if grep -rn --include='*.go' -i "$pat" \
      "$ROOT/primer-agents/internal" "$ROOT/primer-agents/runtime" \
      "$ROOT/primer-agents/cmd" 2>/dev/null | grep -v '_test\.go'; then
    printf 'FAIL: forbidden pattern "%s" found in production primer-agents source\n' "$pat" >&2
    exit 1
  fi
done
echo "OK: no live-billable/Stytch/Studio-MCP/distributed-CP claims in production source"

echo ""
echo "MAF CONDITIONAL GO audit passed."
echo "  Pin:    $EXPECTED"
echo "  Commit: $COMMIT"
echo "  Live billable proof: BLOCKED (by plan — not claimed in this phase)"
