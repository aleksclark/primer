#!/usr/bin/env bash
# Verify the production MAF boundary is still public-API-only and pinned.
set -euo pipefail

expected='v0.0.0-20260813082112-00ffc8c3648c'
actual="$(cd "$(dirname "$0")/../server" && go list -m -f '{{.Version}}' github.com/microsoft/agent-framework-go)"
if [[ "$actual" != "$expected" ]]; then
  printf 'MAF pin mismatch: got %s, want %s\n' "$actual" "$expected" >&2
  exit 1
fi

if rg -n --glob '*.go' '^\s*"github\.com/microsoft/agent-framework-go/internal/' server/internal/agent; then
  echo 'production agent package imports agent-framework-go/internal' >&2
  exit 1
fi

if rg -n --glob '*.go' '^\s*".*spikes/maf-go' server/internal/agent; then
  echo 'production agent package imports the feasibility spike' >&2
  exit 1
fi

echo "MAF runtime public-import audit passed ($actual)"
