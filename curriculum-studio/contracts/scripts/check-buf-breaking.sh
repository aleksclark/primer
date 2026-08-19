#!/usr/bin/env bash
# Fail closed when the current protobuf surface breaks the C10 baseline.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASELINE="$ROOT/baselines/curriculumstudio.v1.buf.binpb"
[[ -s "$BASELINE" ]] || { echo "FAIL: missing protobuf baseline $BASELINE" >&2; exit 1; }
cd "$ROOT"
buf breaking --against "$BASELINE"
echo "OK: protobuf compatibility baseline"
