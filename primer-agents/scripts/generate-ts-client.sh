#!/usr/bin/env bash
# scripts/generate-ts-client.sh
# Regenerate client/ts/types.gen.ts from openapi.yaml using openapi-typescript.
# Tool: openapi-typescript v7.4.0 (pinned in client/ts/package.json)
# Usage: bash scripts/generate-ts-client.sh
# Or via Makefile: make clients-ts
set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f openapi.yaml ]; then
  echo "openapi.yaml not found; run: make openapi" >&2
  exit 1
fi

echo "generating client/ts/types.gen.ts..."
cd client/ts
npm install --silent
npx openapi-typescript@7.4.0 ../../openapi.yaml -o types.gen.ts
echo "done: client/ts/types.gen.ts"
