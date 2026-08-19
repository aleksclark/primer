#!/usr/bin/env bash
# scripts/generate-go-client.sh
# Regenerate client/go/client.gen.go from openapi30.yaml using oapi-codegen.
# Tool: github.com/oapi-codegen/oapi-codegen/v2 v2.4.1
# Usage: bash scripts/generate-go-client.sh
# Or via Makefile: make clients-go
set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f openapi30.yaml ]; then
  echo "openapi30.yaml not found; run: make openapi" >&2
  exit 1
fi

echo "generating client/go/client.gen.go..."
cd client/go
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 \
  -config oapi-codegen.yaml \
  ../../openapi30.yaml
echo "done: client/go/client.gen.go"
