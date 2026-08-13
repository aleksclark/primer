#!/usr/bin/env bash
# Offline validation for Curriculum Studio contracts.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

command -v buf >/dev/null || fail "buf is required"
command -v protoc >/dev/null || fail "protoc is required"
command -v python3 >/dev/null || fail "python3 is required"

echo "== buf lint =="
buf lint

echo "== buf format (check) =="
buf format --diff --exit-code proto

echo "== buf build (image) =="
mkdir -p .tmp
buf build -o .tmp/curriculumstudio.v1.binpb

echo "== protoc descriptor set =="
protoc \
  -I proto \
  -I /usr/include \
  --include_imports \
  --include_source_info \
  --descriptor_set_out=.tmp/curriculumstudio.v1.desc \
  proto/curriculumstudio/v1/common.proto \
  proto/curriculumstudio/v1/plan.proto \
  proto/curriculumstudio/v1/catalog.proto \
  proto/curriculumstudio/v1/materialization.proto \
  proto/curriculumstudio/v1/events.proto \
  proto/curriculumstudio/v1/integration.proto

python3 - "$ROOT" <<'PY'
from pathlib import Path
import sys

root = Path(sys.argv[1])
desc = root / ".tmp" / "curriculumstudio.v1.desc"
image = root / ".tmp" / "curriculumstudio.v1.binpb"
if desc.stat().st_size < 200:
    raise SystemExit(f"descriptor too small: {desc} ({desc.stat().st_size} bytes)")
if image.stat().st_size < 200:
    raise SystemExit(f"buf image too small: {image} ({image.stat().st_size} bytes)")
print(f"descriptor {desc.stat().st_size} bytes")
print(f"buf image {image.stat().st_size} bytes")
PY

echo "== OpenAPI 3.1 validate =="
python3 - <<'PY'
import json
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    raise SystemExit("PyYAML is required")

path = Path("openapi/v1/curriculum-studio.yaml")
doc = yaml.safe_load(path.read_text())
if doc.get("openapi") != "3.1.0":
    raise SystemExit(f"expected openapi 3.1.0, got {doc.get('openapi')!r}")

try:
    from openapi_spec_validator import OpenAPIV31SpecValidator, validate
    validate(doc, cls=OpenAPIV31SpecValidator)
    print("openapi-spec-validator: OK (3.1)")
except ImportError:
    # Fallback: structural 3.1 checks without the extra package.
    required_root = ("openapi", "info", "paths")
    missing = [k for k in required_root if k not in doc]
    if missing:
        raise SystemExit(f"missing OpenAPI keys: {missing}")
    info = doc["info"]
    if "title" not in info or "version" not in info:
        raise SystemExit("info.title/version required")
    if not doc["paths"]:
        raise SystemExit("paths must be non-empty")
    ops = 0
    for p, item in doc["paths"].items():
        if not isinstance(item, dict):
            raise SystemExit(f"path {p} is not an object")
        for method, op in item.items():
            if method.startswith("x-") or method == "parameters":
                continue
            if not isinstance(op, dict) or "operationId" not in op:
                raise SystemExit(f"{method.upper()} {p} missing operationId")
            if "responses" not in op:
                raise SystemExit(f"{method.upper()} {p} missing responses")
            ops += 1
    if ops < 10:
        raise SystemExit(f"too few operations: {ops}")
    print(f"structural OpenAPI 3.1 checks: OK ({ops} operations)")
    print("note: openapi-spec-validator not installed; install for full schema validation")
except Exception as exc:
    raise SystemExit(f"OpenAPI validation failed: {exc}") from exc
PY

echo "OK: contracts validated"
