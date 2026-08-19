#!/usr/bin/env bash
# scripts/generate-openapi.sh
# Emit openapi.yaml (OAS 3.1) and openapi30.yaml (OAS 3.0 downgrade for codegen tools).
# Usage: bash scripts/generate-openapi.sh
# Or via Makefile: make openapi
set -euo pipefail
cd "$(dirname "$0")/.."

echo "generating openapi.yaml (OAS 3.1)..."
go run ./cmd/openapi-gen -format yaml -out openapi.yaml
echo "generating openapi30.yaml (OAS 3.0 downgrade)..."
go run ./cmd/openapi-gen -format yaml30 -out openapi30.yaml
echo "done"
